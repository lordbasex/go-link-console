// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
	"github.com/lordbasex/go-link-console/backend-device/pkg/cores"
	"github.com/lordbasex/go-link-console/backend-device/pkg/glhd"
	"github.com/lordbasex/go-link-console/backend-device/pkg/ownsets"
	"github.com/lordbasex/go-link-console/backend-device/pkg/romcheck"
	"github.com/lordbasex/go-link-console/backend-device/pkg/sysinfo"
	"github.com/lordbasex/go-link-console/backend-device/pkg/thumbnails"
)

// ErrDownloadRunning is returned when the core download is already running.
var ErrDownloadRunning = errors.New("a download is already running")

var romNameRE = regexp.MustCompile(`^[a-z0-9_]{1,16}$`)

// LibraryService manages the host's ROM folder: it lists and checks the
// sets the host put there, and installs the emulator core on request. ROMs
// never leave this folder.
type LibraryService struct {
	dirMu  sync.RWMutex
	dir    string // guarded by dirMu
	hdDir  string // go-link HD's games folder ("": none), guarded by dirMu
	onDir  func(dir string)
	onHD   func(dir string)
	status *StatusService
	log    *slog.Logger

	mu         sync.Mutex
	core       models.CoreStatus
	coresDir   string
	hdCore     string // go-link HD's core when set by SetHDCore; else in coresDir
	coreURL    string
	gameList   *romcheck.Catalog // the core's game list, loaded lazily
	badList    time.Time         // mod time of a game list that failed to load
	datURL     string
	thumbsDir  string
	thumbKind  thumbnails.Kind
	thumbCache *thumbnails.Cache
	own        *ownsets.Matcher // go-link's own sets, by their files' hashes
	makerDir   string           // the Willy Maker game's folder (maker_game.go)

	scanTimer *time.Timer // a scan after imports (scanSoon), guarded by mu

	index    atomic.Pointer[romIndex] // the last scan's sets, for roms_query
	revision atomic.Int64             // counts the scans
}

// QueryRoms returns how many sets match q and the asked page of them.
func (l *LibraryService) QueryRoms(q RomQuery) (revision int64, total int, page []models.RomInfo) {
	ix := l.index.Load()
	if ix == nil {
		return l.revision.Load(), 0, []models.RomInfo{}
	}
	total, page = ix.query(q)
	return l.revision.Load(), total, page
}

// GetRoms returns the named sets that are in the folder (at most
// RomsPageMax), in the asked order.
func (l *LibraryService) GetRoms(names []string) (revision int64, roms []models.RomInfo) {
	ix := l.index.Load()
	if ix == nil {
		return l.revision.Load(), []models.RomInfo{}
	}
	return l.revision.Load(), ix.get(names)
}

// SetCore configures the libretro core: its folder and the buildbot URL
// for this machine (empty when there is no prebuilt core).
func (l *LibraryService) SetCore(dir, url string) {
	l.mu.Lock()
	l.coresDir, l.coreURL = dir, url
	l.core.Name = cores.DefaultCore
	l.mu.Unlock()
}

// CatalogPath is where the core's compact game list lives.
func (l *LibraryService) CatalogPath() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return filepath.Join(l.coresDir, romcheck.FileName)
}

// Catalog returns the core's game list, or nil when it is not installed.
func (l *LibraryService) Catalog() *romcheck.Catalog {
	l.mu.Lock()
	cat := l.gameList
	l.mu.Unlock()
	if cat != nil {
		return cat
	}
	path := l.CatalogPath()
	fi, err := os.Stat(path)
	if err != nil {
		return nil
	}
	l.mu.Lock()
	known := l.badList.Equal(fi.ModTime())
	l.mu.Unlock()
	if known {
		return nil // already reported; wait for a new download
	}
	cat, err = romcheck.Load(path)
	if err != nil {
		l.log.Warn("cannot read the core's game list", "err", err)
		l.mu.Lock()
		l.badList = fi.ModTime()
		l.mu.Unlock()
		return nil
	}
	l.mu.Lock()
	l.gameList = cat
	l.mu.Unlock()
	return cat
}

// RefreshCatalog downloads the core's game list again when the saved one
// has an older layout (for example, from before games carried their
// controls). A missing list is left alone: it comes with the core.
func (l *LibraryService) RefreshCatalog(ctx context.Context) {
	path := l.CatalogPath()
	if _, err := romcheck.Load(path); !errors.Is(err, romcheck.ErrOutdated) {
		return
	}
	l.mu.Lock()
	url := l.datURL
	l.mu.Unlock()
	if url == "" {
		url = romcheck.DatURL
	}
	cat, err := romcheck.Download(ctx, nil, url, path)
	if err != nil {
		l.log.Warn("cannot update the core's game list", "err", err)
		return
	}
	l.mu.Lock()
	l.gameList, l.badList = cat, time.Time{}
	l.mu.Unlock()
	l.log.Info("core game list updated", "games", len(cat.Games))
	l.Scan()
}

// CheckRom tells whether the core can run a set, without running it. ok
// is false when the core's game list is not installed.
func (l *LibraryService) CheckRom(name string) (res romcheck.Result, ok bool) {
	if name == MakerRom {
		return l.checkMaker()
	}
	if l.IsHD(name) {
		if _, ok := l.demo(name); ok {
			return romcheck.Result{Status: romcheck.StatusOK}, true
		}
		if _, err := glhd.Read(l.RomPath(name)); err != nil {
			return romcheck.Result{Status: romcheck.StatusBadZip}, true
		}
		return romcheck.Result{Status: romcheck.StatusOK}, true
	}
	cat := l.Catalog()
	if cat == nil {
		return romcheck.Result{}, false
	}
	return romcheck.NewChecker(cat, l.Dir()).Check(name), true
}

// CorePath is where the core library lives.
func (l *LibraryService) CorePath() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return filepath.Join(l.coresDir, cores.FileName(cores.DefaultCore, runtime.GOOS))
}

// SetHDCore sets where go-link HD's engine library is (the device's
// --hd-core); without it the one shipped with the device is used, else
// the one in the cores folder.
func (l *LibraryService) SetHDCore(path string) {
	l.mu.Lock()
	l.hdCore = path
	l.mu.Unlock()
}

// HDCorePath is where go-link HD's engine library lives.
func (l *LibraryService) HDCorePath() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.hdCore != "" {
		return l.hdCore
	}
	if p := glhd.BundledLibrary(runtime.GOOS); p != "" {
		return p
	}
	return filepath.Join(l.coresDir, glhd.LibraryFile(runtime.GOOS))
}

// IsHD reports whether a game is go-link HD's, a package of the folder
// (name.glhd) or one of the engine's built-in games, rather than a MAME
// set. A .zip of the same name wins.
func (l *LibraryService) IsHD(name string) bool {
	if !romNameRE.MatchString(name) {
		return false
	}
	if _, err := os.Stat(filepath.Join(l.Dir(), name+".zip")); err == nil {
		return false
	}
	if l.hdPackage(name) != "" {
		return true
	}
	_, ok := glhd.FindDemo(name)
	return ok
}

// HDPackage is the file of a go-link HD package of the folders, or "".
func (l *LibraryService) HDPackage(name string) string {
	if !romNameRE.MatchString(name) {
		return ""
	}
	return l.hdPackage(name)
}

// hdPackage is the file of a go-link HD package (name.glhd), or "": the
// HD games folder's first, then the ROM folder's (where packages lived
// before the HD folder).
func (l *LibraryService) hdPackage(name string) string {
	for _, dir := range []string{l.HDDir(), l.Dir()} {
		if dir == "" {
			continue
		}
		p := filepath.Join(dir, name+glhd.Ext)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return ""
}

// demo returns the engine's built-in game of a name, when no file of the
// folder has that name.
func (l *LibraryService) demo(name string) (glhd.Demo, bool) {
	d, ok := glhd.FindDemo(name)
	if !ok {
		return d, false
	}
	if _, err := os.Stat(filepath.Join(l.Dir(), name+".zip")); err == nil {
		return d, false
	}
	if l.hdPackage(name) != "" {
		return d, false
	}
	return d, true
}

// CoreFor is the core that plays a game: go-link HD's for its packages,
// the MAME core for everything else.
func (l *LibraryService) CoreFor(name string) string {
	if l.IsHD(name) {
		return l.HDCorePath()
	}
	return l.CorePath()
}

// HasCoreFor reports whether the core that plays a game is installed.
func (l *LibraryService) HasCoreFor(name string) bool {
	_, err := os.Stat(l.CoreFor(name))
	return err == nil
}

// HD returns a go-link HD package's manifest, or nil for any other game.
func (l *LibraryService) HD(name string) *glhd.Manifest {
	if !l.IsHD(name) {
		return nil
	}
	if d, ok := l.demo(name); ok {
		return &glhd.Manifest{Format: glhd.Format, Title: d.Title, Players: d.Players}
	}
	m, err := glhd.Read(l.RomPath(name))
	if err != nil {
		return nil
	}
	return &m
}

// HasCore reports whether the core is installed.
func (l *LibraryService) HasCore() bool {
	_, err := os.Stat(l.CorePath())
	return err == nil
}

// HasRom reports whether a ROM set is in the folder.
func (l *LibraryService) HasRom(name string) bool {
	if name == MakerRom {
		_, err := os.Stat(l.makerPath())
		return l.makerFolder() != "" && err == nil
	}
	if !romNameRE.MatchString(name) {
		return false
	}
	_, err := os.Stat(filepath.Join(l.Dir(), name+".zip"))
	return err == nil || l.IsHD(name)
}

// RomPath returns the file of a ROM set.
func (l *LibraryService) RomPath(name string) string {
	if name == MakerRom {
		return l.makerPath()
	}
	if d, ok := l.demo(name); ok {
		return glhd.DemoPath(d)
	}
	if l.IsHD(name) {
		if p := l.hdPackage(name); p != "" {
			return p
		}
		return filepath.Join(l.Dir(), name+glhd.Ext)
	}
	return filepath.Join(l.Dir(), name+".zip")
}

// Own returns the go-link set a ROM of the folder is, or nil. It is decided
// by the SHA-256 of every file inside the zip, never by the name: a real
// set with the same name is the original game.
func (l *LibraryService) Own(name string) *ownsets.Set {
	if !romNameRE.MatchString(name) || l.IsHD(name) {
		return nil
	}
	return l.own.Match(l.RomPath(name))
}

// Title returns the catalog title of a set, or its short name.
func (l *LibraryService) Title(name string) string {
	if name == MakerRom {
		return l.Maker().Title
	}
	if lib := l.status.Snapshot().Library; lib != nil {
		for _, r := range lib.Roms {
			if r.Name == name && r.Title != "" {
				return r.Title
			}
		}
	}
	return name
}

// DownloadCore installs the emulator core from the libretro buildbot and
// the core's game list (to check ROMs), whichever is missing, in the
// background.
func (l *LibraryService) DownloadCore(ctx context.Context) error {
	if err := l.startCoreDownload(); err != nil {
		return err
	}
	go l.installCore(ctx)
	return nil
}

// InstallCore is DownloadCore, waiting until it ends (for the CLI).
func (l *LibraryService) InstallCore(ctx context.Context) error {
	if err := l.startCoreDownload(); err != nil {
		return err
	}
	return l.installCore(ctx)
}

func (l *LibraryService) startCoreDownload() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.core.Downloading {
		return ErrDownloadRunning
	}
	l.core.Downloading, l.core.Error = true, ""
	return nil
}

func (l *LibraryService) installCore(ctx context.Context) error {
	l.Scan()
	var errs []error
	if !l.HasCore() {
		l.mu.Lock()
		url, dir := l.coreURL, l.coresDir
		l.mu.Unlock()
		if url == "" {
			errs = append(errs, errors.New("no prebuilt emulator core for this system"))
		} else if _, err := cores.Download(ctx, nil, url, cores.FileName(cores.DefaultCore, runtime.GOOS), dir); err != nil {
			errs = append(errs, err)
		} else {
			l.log.Info("emulator core installed", "core", cores.DefaultCore)
		}
	}
	if l.Catalog() == nil {
		l.mu.Lock()
		url := l.datURL
		l.mu.Unlock()
		if url == "" {
			url = romcheck.DatURL
		}
		if cat, err := romcheck.Download(ctx, nil, url, l.CatalogPath()); err != nil {
			errs = append(errs, fmt.Errorf("core game list: %w", err))
		} else {
			l.mu.Lock()
			l.gameList = cat
			l.mu.Unlock()
			l.log.Info("core game list installed", "games", len(cat.Games))
		}
	}
	err := errors.Join(errs...)
	l.mu.Lock()
	l.core.Downloading = false
	if err != nil {
		l.core.Error = err.Error()
		l.log.Warn("core download failed", "err", err)
	}
	l.mu.Unlock()
	l.Scan()
	return err
}

// NewLibraryService builds the service for dir.
func NewLibraryService(dir string, status *StatusService, logger *slog.Logger) *LibraryService {
	if logger == nil {
		logger = slog.Default()
	}
	return &LibraryService{dir: dir, status: status, log: logger, thumbCache: thumbnails.NewCache(512), own: ownsets.NewMatcher(nil)}
}

// Dir returns the ROM folder.
func (l *LibraryService) Dir() string {
	l.dirMu.RLock()
	defer l.dirMu.RUnlock()
	return l.dir
}

// OnDirChange registers a callback that persists a new folder.
func (l *LibraryService) OnDirChange(fn func(dir string)) { l.onDir = fn }

// HDDir returns go-link HD's games folder ("" when there is none).
func (l *LibraryService) HDDir() string {
	l.dirMu.RLock()
	defer l.dirMu.RUnlock()
	return l.hdDir
}

// OnHDDirChange registers a callback that persists a new HD games folder.
func (l *LibraryService) OnHDDirChange(fn func(dir string)) { l.onHD = fn }

// UseHDDir sets go-link HD's games folder without checking or saving it
// (at start: the folder is made if it is missing).
func (l *LibraryService) UseHDDir(dir string) {
	l.dirMu.Lock()
	l.hdDir = dir
	l.dirMu.Unlock()
}

// SetHDDir switches go-link HD's games folder and rescans. Nothing is
// copied or moved.
func (l *LibraryService) SetHDDir(dir string) error {
	dir = filepath.Clean(strings.TrimSpace(dir))
	if !filepath.IsAbs(dir) {
		return ErrBadFolder
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return ErrBadFolder
	}
	l.UseHDDir(dir)
	if l.onHD != nil {
		l.onHD(dir)
	}
	l.log.Info("go-link HD games folder changed", "dir", dir)
	l.Scan()
	return nil
}

// Errors returned when choosing a folder or importing ROMs.
var (
	ErrBadFolder = errors.New("the folder must be an absolute path to an existing directory")
	ErrBadRom    = errors.New("only MAME ROM sets (.zip files with a short name, e.g. robby.zip) can be added")
	ErrDiskFull  = errors.New("not enough free space on the device's disk for this ROM")
	// ErrNotRom is a Willy Maker project or AI pack dropped as a ROM (T-21):
	// they are the game's sources, and Create ROM makes the ROM from them.
	ErrNotRom = errors.New("this is a Willy Maker project or AI pack, not a ROM: open it in Willy Maker and use Create ROM in the Export tab to make the ROM")
)

// notRomName reports a Willy Maker project (.willy.zip) or AI pack (.ai-pack.zip) by its name.
func notRomName(base string) bool {
	return strings.HasSuffix(base, ".willy.zip") || strings.HasSuffix(base, ".ai-pack.zip")
}

// notRomZip reports a zip with a Willy Maker project or AI pack inside
// (project.json or PROMPT.md at its top), whatever it is called. No ROM
// set has them.
func notRomZip(path string) bool {
	z, err := zip.OpenReader(path)
	if err != nil {
		return false
	}
	defer z.Close()
	for _, f := range z.File {
		if f.Name == "project.json" || f.Name == "PROMPT.md" {
			return true
		}
	}
	return false
}

// MinFreeSpace is what an import always leaves free on the disk, so a
// drop never fills it (the device, its saves and the system need room).
const MinFreeSpace = 1 << 30

// diskFree reports the free bytes on the disk of path (tests replace it).
var diskFree = func(path string) (uint64, bool) {
	d, ok := sysinfo.DiskOf(path)
	return d.Free, ok
}

// SetDir switches to another ROM folder (for example the host's own
// collection) and rescans. Nothing is copied or moved.
func (l *LibraryService) SetDir(dir string) error {
	dir = filepath.Clean(strings.TrimSpace(dir))
	if !filepath.IsAbs(dir) {
		return ErrBadFolder
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return ErrBadFolder
	}
	l.dirMu.Lock()
	l.dir = dir
	l.dirMu.Unlock()
	if l.onDir != nil {
		l.onDir(dir)
	}
	l.log.Info("ROM folder changed", "dir", dir)
	l.Scan()
	return nil
}

// MaxImportSize bounds one imported ROM set.
const MaxImportSize = 512 << 20

// CheckImportName validates a dropped file name and returns the set name.
func (l *LibraryService) CheckImportName(fileName string) (string, error) {
	if isImage(fileName) {
		if filepath.Base(fileName) != fileName {
			return "", thumbnails.ErrNotImage
		}
		return fileName, nil // a thumbnail (Boxart), see Import
	}
	base := strings.ToLower(filepath.Base(fileName))
	if notRomName(base) {
		return "", ErrNotRom
	}
	name, ok := strings.CutSuffix(base, ".zip")
	if !ok || !romNameRE.MatchString(name) {
		return "", ErrBadRom
	}
	if l.HasRom(name) {
		return "", fmt.Errorf("%s.zip is already in the folder", name)
	}
	return name, nil
}

// Import copies one ROM set into the current folder (drag and drop). The
// data is streamed to a temporary file, so large sets do not sit in
// memory. It never overwrites an existing set.
func (l *LibraryService) Import(fileName string, r io.Reader) error {
	// Images dropped with the ROMs are the host's Boxart thumbnails.
	if isImage(fileName) {
		return l.ImportThumbnail(thumbnails.Boxart, fileName, r)
	}
	name, err := l.CheckImportName(fileName)
	if err != nil {
		return err
	}
	dir := l.Dir()
	// Never fill the disk: at most what leaves MinFreeSpace free.
	limit := int64(MaxImportSize)
	if free, ok := diskFree(dir); ok {
		if free <= MinFreeSpace {
			return ErrDiskFull
		}
		limit = min(limit, int64(free-MinFreeSpace))
	}
	tmp, err := os.CreateTemp(dir, ".import-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	head := make([]byte, 4)
	if _, err := io.ReadFull(r, head); err != nil || !bytes.Equal(head, []byte("PK\x03\x04")) {
		tmp.Close()
		return ErrBadRom
	}
	n, err := io.Copy(tmp, io.MultiReader(bytes.NewReader(head), io.LimitReader(r, limit)))
	if err == nil && n > limit {
		err = ErrBadRom
		if limit < MaxImportSize {
			err = ErrDiskFull
		}
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if notRomZip(tmp.Name()) {
		return ErrNotRom
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	if err := placeNoOverwrite(tmp.Name(), filepath.Join(dir, name+".zip")); err != nil {
		return err
	}
	l.log.Info("ROM imported", "rom", name)
	l.scanSoon()
	return nil
}

// ScanNow scans the folder at once, in place of a scan scanSoon planned.
func (l *LibraryService) ScanNow() {
	l.mu.Lock()
	if l.scanTimer != nil && l.scanTimer.Stop() {
		l.scanTimer = nil
	}
	l.mu.Unlock()
	l.Scan()
}

// importScanDelay is how long scanSoon waits for the next imported set.
const importScanDelay = 750 * time.Millisecond

// scanSoon scans the folder once a burst of imports is over: a scan reads
// every set (a full folder takes seconds), and dropping a hundred zips would
// otherwise scan a hundred times.
func (l *LibraryService) scanSoon() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.scanTimer != nil {
		l.scanTimer.Reset(importScanDelay)
		return
	}
	l.scanTimer = time.AfterFunc(importScanDelay, func() {
		l.mu.Lock()
		l.scanTimer = nil
		l.mu.Unlock()
		l.Scan()
	})
}

// placeNoOverwrite moves src to dst and fails if dst already exists. A hard
// link is atomic and refuses an existing target; filesystems without links
// (exFAT, some network shares) fall back to a checked rename.
func placeNoOverwrite(src, dst string) error {
	err := os.Link(src, dst)
	if err == nil {
		return os.Remove(src)
	}
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("%s is already in the folder", filepath.Base(dst))
	}
	if _, serr := os.Stat(dst); serr == nil {
		return fmt.Errorf("%s is already in the folder", filepath.Base(dst))
	}
	return os.Rename(src, dst)
}

// Scan lists the .zip sets and the go-link HD packages (.glhd) and
// publishes them in the status.
func (l *LibraryService) Scan() {
	var roms []models.RomInfo
	files, _ := os.ReadDir(l.Dir())
	zips := map[string]bool{}
	for _, f := range files {
		if name, ok := strings.CutSuffix(f.Name(), ".zip"); ok {
			zips[name] = true
		}
	}
	// go-link HD's games folder comes first: its package wins over one of
	// the same name in the ROM folder.
	type entry struct {
		dir string
		f   os.DirEntry
	}
	var entries []entry
	hdDir := l.HDDir()
	var hdGames int
	var hdBytes int64
	seen := map[string]bool{}
	if hdDir != "" && hdDir != l.Dir() {
		hdFiles, _ := os.ReadDir(hdDir)
		for _, f := range hdFiles {
			if name, ok := strings.CutSuffix(f.Name(), glhd.Ext); ok && !f.IsDir() && romNameRE.MatchString(name) {
				entries = append(entries, entry{hdDir, f})
				if info, err := f.Info(); err == nil {
					hdGames++
					hdBytes += info.Size()
				}
				seen[name] = true
			}
		}
	}
	for _, f := range files {
		if name, ok := strings.CutSuffix(f.Name(), glhd.Ext); ok && seen[name] {
			continue
		}
		entries = append(entries, entry{l.Dir(), f})
	}
	for _, e := range entries {
		f := e.f
		name, ok := strings.CutSuffix(f.Name(), ".zip")
		hd := false
		if !ok {
			name, hd = strings.CutSuffix(f.Name(), glhd.Ext)
			if !hd || zips[name] {
				continue // a .zip of the same name wins
			}
		}
		if f.IsDir() || !romNameRE.MatchString(name) {
			continue
		}
		info, err := f.Info()
		if err != nil {
			continue
		}
		rom := models.RomInfo{Name: name, Size: info.Size()}
		if hd {
			// go-link HD's packages tell their own title and players
			rom.Kind = models.KindHD
			m, err := glhd.Read(filepath.Join(e.dir, f.Name()))
			if err != nil {
				rom.Check = &romcheck.Result{Status: romcheck.StatusBadZip}
			} else {
				rom.Check = &romcheck.Result{Status: romcheck.StatusOK}
				rom.Title, rom.Description = m.Title, "go-link HD"
				rom.Controls = &models.RomControls{Players: m.Players, Buttons: len(m.Labels()), Labels: m.Labels()}
			}
		}
		roms = append(roms, rom)
	}
	// The engine's built-in games, whenever the engine is installed.
	if _, err := os.Stat(l.HDCorePath()); err == nil {
		for _, d := range glhd.Demos {
			if _, ok := l.demo(d.Name); !ok {
				continue
			}
			roms = append(roms, models.RomInfo{
				Name: d.Name, Kind: models.KindHD, Title: d.Title, Description: "go-link HD",
				Check:    &romcheck.Result{Status: romcheck.StatusOK},
				Controls: &models.RomControls{Players: d.Players, Buttons: len(glhd.Labels), Labels: glhd.Labels},
			})
		}
	}
	// Check each set against the core's game list, without running it.
	if cat := l.Catalog(); cat != nil {
		checker := romcheck.NewChecker(cat, l.Dir())
		for i := range roms {
			if roms[i].Kind == models.KindHD {
				continue
			}
			res := checker.Check(roms[i].Name)
			roms[i].Check = &res
			if g := cat.Game(roms[i].Name); g != nil {
				roms[i].Title, roms[i].Year, roms[i].Maker = g.Title, g.Year, g.Maker
			}
		}
	}
	// go-link's own sets show go-link's game, never the original set's
	// title or the host's pictures of it.
	for i := range roms {
		if roms[i].Kind == models.KindHD {
			continue
		}
		if s := l.Own(roms[i].Name); s != nil {
			roms[i].Own = true
			roms[i].Title, roms[i].Year, roms[i].Maker, roms[i].Description = s.Title, s.Year, s.Maker, s.Description
			roms[i].Controls = &models.RomControls{Players: s.Players, Buttons: s.Buttons, Labels: s.Labels}
		}
	}
	// Thumbnails can be named after the set or the title, so look after
	// the titles are known.
	thumbsDir := l.ThumbnailsDir()
	for i := range roms {
		if roms[i].Own {
			_, art := l.Own(roms[i].Name).Picture()
			roms[i].Thumbs = models.Thumbs{Boxart: art, Title: art, Snap: art}
			continue
		}
		has := thumbnails.Has(thumbsDir, roms[i].Name, roms[i].Title)
		roms[i].Thumbs = models.Thumbs{Boxart: has[thumbnails.Boxart], Title: has[thumbnails.Title], Snap: has[thumbnails.Snap]}
	}
	slices.SortFunc(roms, func(a, b models.RomInfo) int { return strings.Compare(a.Name, b.Name) })
	if roms == nil {
		roms = []models.RomInfo{}
	}
	installed := l.HasCore()
	l.mu.Lock()
	core := l.core
	l.mu.Unlock()
	core.Installed = installed
	core.Catalog = l.Catalog() != nil
	if _, err := os.Stat(l.HDCorePath()); err == nil {
		core.HDInstalled = true
	}
	ix := newRomIndex(roms)
	l.index.Store(ix)
	lib := models.Library{Dir: l.Dir(), Roms: roms, Summary: ix.summary(l.revision.Add(1)), Core: core, ThumbnailsDir: thumbsDir, ThumbKind: string(l.ThumbnailKind()), ThumbnailsBytes: thumbnails.Size(thumbsDir),
		HDDir: hdDir, HDGames: hdGames, HDBytes: hdBytes}
	if d, ok := sysinfo.DiskOf(l.Dir()); ok {
		lib.Disk = &d
	}
	l.status.SetLibrary(lib)
}

// SetThumbnailsDir sets the folder of the host's own thumbnails (one
// sub-folder per kind, libretro style).
func (l *LibraryService) SetThumbnailsDir(dir string) {
	l.mu.Lock()
	l.thumbsDir = dir
	l.mu.Unlock()
}

// SetThumbnailKind picks which kind of thumbnail the device shows (and
// tells browsers to show); an unknown kind means Boxart.
func (l *LibraryService) SetThumbnailKind(kind thumbnails.Kind) {
	if !kind.Valid() {
		kind = thumbnails.Boxart
	}
	l.mu.Lock()
	l.thumbKind = kind
	l.mu.Unlock()
}

// ThumbnailKind returns the kind of thumbnail to show.
func (l *LibraryService) ThumbnailKind() thumbnails.Kind {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.thumbKind.Valid() {
		return thumbnails.Boxart
	}
	return l.thumbKind
}

// ThumbnailsDir returns the thumbnails folder.
func (l *LibraryService) ThumbnailsDir() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.thumbsDir
}

// ThumbnailPath returns the image of a kind for a set, if the host has it.
func (l *LibraryService) ThumbnailPath(name string, kind thumbnails.Kind) (string, bool) {
	if !romNameRE.MatchString(name) || !kind.Valid() {
		return "", false
	}
	return thumbnails.Find(l.ThumbnailsDir(), kind, name, l.Title(name))
}

// Thumbnail returns a small JPEG of a set's thumbnail that fits in
// maxW x maxH (and in maxBytes when it is above 0).
func (l *LibraryService) Thumbnail(name string, kind thumbnails.Kind, maxW, maxH, maxBytes int) ([]byte, error) {
	// A go-link set has its own picture (any kind), or none: the host's
	// thumbnails for its name are the original game's.
	if s := l.Own(name); s != nil {
		art, ok := s.Picture()
		if !ok || !kind.Valid() {
			return nil, os.ErrNotExist
		}
		return l.thumbCache.SmallBytes(s.PictureKey(), art, maxW, maxH, maxBytes)
	}
	p, ok := l.ThumbnailPath(name, kind)
	if !ok {
		return nil, os.ErrNotExist
	}
	return l.thumbCache.Small(p, maxW, maxH, maxBytes)
}

// ImportThumbnail saves an image the host dropped and rescans, so the
// library shows it.
func (l *LibraryService) ImportThumbnail(kind thumbnails.Kind, name string, r io.Reader) error {
	if _, err := thumbnails.Import(l.ThumbnailsDir(), kind, name, r); err != nil {
		return err
	}
	l.Scan()
	return nil
}

// isImage reports whether a dropped file is a PNG or JPEG picture.
func isImage(fileName string) bool {
	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".png", ".jpg", ".jpeg":
		return true
	}
	return false
}
