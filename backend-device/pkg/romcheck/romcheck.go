// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package romcheck tells whether a ROM set will run on a MAME core without
// running it. It compares the file list of each zip (read from the zip's
// central directory, nothing is decompressed) with the core's game list,
// using the same rules as the MAME 0.78 ROM loader.
package romcheck

import (
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/gob"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// DatURL is the game list published by the mame2003-plus project.
const DatURL = "https://raw.githubusercontent.com/libretro/mame2003-plus-libretro/master/metadata/mame2003-plus.xml"

// FileName is the compact catalog saved next to the core.
const FileName = "mame2003_plus.romcheck"

// maxDat bounds the XML download (about 22 MB today).
const maxDat = 96 << 20

// maxMissing bounds the missing file names reported per game.
const maxMissing = 10

// ErrOutdated means the saved catalog has an older layout: download the
// game list again.
var ErrOutdated = errors.New("romcheck: outdated catalog, download it again")

// formatVersion changes whenever the saved catalog layout changes.
const formatVersion = 4

// Status is the verdict for one ROM set.
type Status string

// Verdicts.
const (
	StatusOK          Status = "ok"          // every required file is there
	StatusMissing     Status = "missing"     // required files are missing
	StatusUnsupported Status = "unsupported" // the core does not know this game
	StatusBIOS        Status = "bios"        // a BIOS set, not a game
	StatusBadZip      Status = "bad_zip"     // the file is not a readable zip
)

// Rom is one file a game needs.
type Rom struct {
	Name   string // lower case
	Size   int64
	CRC    uint32
	NoDump bool // no known dump: never required
	Disk   bool // a hard disk image (.chd), outside the zip
}

// Game is one entry of the core's game list.
type Game struct {
	Name   string
	Title  string
	Year   string
	Maker  string
	RomOf  string // parent set or BIOS whose zip is searched too
	BIOS   bool
	Driver string // good, preliminary or protection
	Roms   []Rom
	Input  Input
	// SaveModules are the save state modules a whole save of this game must
	// contain, for chips some builds of the core do not save (see
	// chipModules). A save without one of them is incomplete.
	SaveModules []string
}

// chipModules maps chips to the save state module that saves them.
// MAME 2003-Plus (MAME 0.78) left both out, as its source shows: the
// Konami CPU core's save code is disabled (a loaded save runs on garbage
// registers and the game crashes) and the QSound chip of Capcom's CPS2
// boards registers nothing (the game resumes silent). A core that saves
// them (go-link's patches, or upstream once they are merged) lists the
// module, and the game resumes. Other gaps are found by trying
// (services.ProbeSaves).
var chipModules = map[string]string{"cpu/KONAMI": "konami", "audio/QSound": "QSound"}

// Input is the control panel of a game, as MAME describes it.
type Input struct {
	Players int    // players at once
	Buttons int    // action buttons per player
	Control string // joy4way, joy8way, stick, dial, trackball, lightgun... empty when unknown
}

// Catalog is the core's game list, by set name.
type Catalog struct {
	Version int
	Games   map[string]*Game
}

// Result is the verdict for one ROM set.
type Result struct {
	Status  Status   `json:"status"`
	Missing []string `json:"missing,omitempty"`
	Needs   []string `json:"needs,omitempty"`
	Driver  string   `json:"driver,omitempty"` // set when not "good"
}

// Reason explains a verdict that is not ok, in one line.
func (r Result) Reason() string {
	switch r.Status {
	case StatusOK:
		return ""
	case StatusUnsupported:
		return "this game is not in the mame2003-plus core (it is from a newer MAME)"
	case StatusBIOS:
		return "this is a BIOS, not a game"
	case StatusBadZip:
		return "the file is not a valid zip"
	}
	var b strings.Builder
	b.WriteString("files missing for this core")
	if len(r.Missing) > 0 {
		b.WriteString(": " + strings.Join(r.Missing, ", "))
	}
	if len(r.Needs) > 0 {
		b.WriteString(" (needs " + strings.Join(r.Needs, ".zip, ") + ".zip)")
	}
	return b.String()
}

// Game returns a game of the list, or nil.
func (c *Catalog) Game(name string) *Game {
	if c == nil {
		return nil
	}
	return c.Games[name]
}

type xmlGame struct {
	Name     string `xml:"name,attr"`
	RomOf    string `xml:"romof,attr"`
	Runnable string `xml:"runnable,attr"`
	Title    string `xml:"description"`
	Year     string `xml:"year"`
	Maker    string `xml:"manufacturer"`
	BIOSSets []struct {
		Name    string `xml:"name,attr"`
		Default string `xml:"default,attr"`
	} `xml:"biosset"`
	Roms []struct {
		Name   string `xml:"name,attr"`
		BIOS   string `xml:"bios,attr"`
		Size   string `xml:"size,attr"`
		CRC    string `xml:"crc,attr"`
		Status string `xml:"status,attr"`
	} `xml:"rom"`
	Disks []struct {
		Name string `xml:"name,attr"`
	} `xml:"disk"`
	Driver struct {
		Status string `xml:"status,attr"`
	} `xml:"driver"`
	Input struct {
		Players string `xml:"players,attr"`
		Buttons string `xml:"buttons,attr"`
		Control string `xml:"control,attr"`
	} `xml:"input"`
	Chips []struct {
		Type string `xml:"type,attr"`
		Name string `xml:"name,attr"`
	} `xml:"chip"`
}

// ParseXML reads a MAME game list (the -listxml format).
func ParseXML(r io.Reader) (*Catalog, error) {
	cat := &Catalog{Version: formatVersion, Games: map[string]*Game{}}
	dec := xml.NewDecoder(r)
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("romcheck: %w", err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "game" {
			continue
		}
		var xg xmlGame
		if err := dec.DecodeElement(&xg, &start); err != nil {
			return nil, fmt.Errorf("romcheck: %w", err)
		}
		g := &Game{
			Name: xg.Name, Title: strings.TrimSpace(xg.Title), Year: xg.Year, Maker: strings.TrimSpace(xg.Maker),
			RomOf: xg.RomOf, BIOS: xg.Runnable == "no", Driver: xg.Driver.Status,
		}
		g.Input.Players, _ = strconv.Atoi(xg.Input.Players)
		g.Input.Buttons, _ = strconv.Atoi(xg.Input.Buttons)
		g.Input.Control = xg.Input.Control
		for _, c := range xg.Chips {
			if m := chipModules[c.Type+"/"+c.Name]; m != "" && !slices.Contains(g.SaveModules, m) {
				g.SaveModules = append(g.SaveModules, m)
			}
		}
		// Only the default BIOS is loaded; the other BIOS versions of a
		// set (Neo Geo lists fifteen) are optional.
		defaultBIOS := ""
		for i, b := range xg.BIOSSets {
			if i == 0 || b.Default == "yes" {
				defaultBIOS = b.Name
			}
			if b.Default == "yes" {
				break
			}
		}
		seen := map[string]bool{}
		for _, xr := range xg.Roms {
			if xr.BIOS != "" && xr.BIOS != defaultBIOS {
				continue
			}
			name := strings.ToLower(xr.Name)
			// The same file can be loaded twice at different offsets.
			if seen[name] {
				continue
			}
			seen[name] = true
			size, _ := strconv.ParseInt(xr.Size, 10, 64)
			crc, _ := strconv.ParseUint(xr.CRC, 16, 32)
			g.Roms = append(g.Roms, Rom{Name: name, Size: size, CRC: uint32(crc), NoDump: xr.Status == "nodump" || xr.CRC == ""})
		}
		for _, d := range xg.Disks {
			name := strings.TrimSuffix(strings.ToLower(d.Name), ".chd")
			g.Roms = append(g.Roms, Rom{Name: name, Disk: true})
		}
		cat.Games[g.Name] = g
	}
	if len(cat.Games) == 0 {
		return nil, errors.New("romcheck: the game list is empty")
	}
	return cat, nil
}

// Save writes the catalog in a compact form, atomically.
func (c *Catalog) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".romcheck-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	zw := gzip.NewWriter(tmp)
	err = gob.NewEncoder(zw).Encode(c)
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Load reads a catalog written by Save.
func Load(path string) (*Catalog, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("romcheck: %w", err)
	}
	var c Catalog
	if err := gob.NewDecoder(zr).Decode(&c); err != nil {
		return nil, fmt.Errorf("romcheck: %w", err)
	}
	if c.Version != formatVersion || len(c.Games) == 0 {
		return nil, ErrOutdated
	}
	return &c, nil
}

// Download fetches the core's game list and saves it compacted at path.
func Download(ctx context.Context, client *http.Client, url, path string) (*Catalog, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "go-link-console")
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("romcheck: %s: HTTP %d", url, resp.StatusCode)
	}
	cat, err := ParseXML(io.LimitReader(resp.Body, maxDat))
	if err != nil {
		return nil, err
	}
	if err := cat.Save(path); err != nil {
		return nil, err
	}
	return cat, nil
}

// zipIndex is the file list of one zip.
type zipIndex struct {
	sizes map[string]int64 // by lower case base name
	crcs  map[uint32]bool
}

func readZip(path string) (*zipIndex, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	idx := &zipIndex{sizes: map[string]int64{}, crcs: map[uint32]bool{}}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := strings.ToLower(f.Name)
		if i := strings.LastIndexByte(name, '/'); i >= 0 {
			name = name[i+1:]
		}
		idx.sizes[name] = int64(f.UncompressedSize64)
		idx.crcs[f.CRC32] = true
	}
	return idx, nil
}

// Checker checks the sets of one folder. It reads each zip once, so a
// parent shared by many clones (a BIOS like neogeo) is read only once.
type Checker struct {
	cat  *Catalog
	dir  string
	zips map[string]*zipIndex // nil value: absent or unreadable
	bad  map[string]bool
}

// NewChecker checks sets in dir against the catalog.
func NewChecker(cat *Catalog, dir string) *Checker {
	return &Checker{cat: cat, dir: dir, zips: map[string]*zipIndex{}, bad: map[string]bool{}}
}

func (c *Checker) zip(name string) *zipIndex {
	if idx, ok := c.zips[name]; ok {
		return idx
	}
	idx, err := readZip(filepath.Join(c.dir, name+".zip"))
	if err != nil {
		idx = nil
		if !errors.Is(err, os.ErrNotExist) {
			c.bad[name] = true
		}
	}
	c.zips[name] = idx
	return idx
}

// Check returns the verdict for the set name (the zip's base name).
func (c *Checker) Check(name string) Result {
	if c.zip(name) == nil {
		if c.bad[name] {
			return Result{Status: StatusBadZip}
		}
		return Result{Status: StatusMissing, Needs: []string{name}}
	}
	g := c.cat.Game(name)
	if g == nil {
		return Result{Status: StatusUnsupported}
	}
	if g.BIOS {
		return Result{Status: StatusBIOS}
	}
	res := Result{Status: StatusOK}
	if g.Driver != "" && g.Driver != "good" {
		res.Driver = g.Driver
	}
	// The loader searches the set's zip, then its parent, then the
	// parent's parent (a clone of a BIOS game).
	chain := []string{g.Name}
	for p := g.RomOf; p != "" && len(chain) < 8; {
		chain = append(chain, p)
		pg := c.cat.Game(p)
		if pg == nil {
			break
		}
		p = pg.RomOf
	}
	missing := 0
	for _, r := range g.Roms {
		if r.NoDump || c.has(chain, r) {
			continue
		}
		missing++
		if len(res.Missing) < maxMissing {
			if r.Disk {
				res.Missing = append(res.Missing, r.Name+".chd")
			} else {
				res.Missing = append(res.Missing, r.Name)
			}
		}
	}
	if missing > 0 {
		res.Status = StatusMissing
		for _, p := range chain[1:] {
			if c.zip(p) == nil {
				res.Needs = append(res.Needs, p)
			}
		}
	}
	return res
}

// has reports whether a file is found as the MAME 0.78 loader finds it:
// by name (a wrong size or CRC only gives a warning), or by CRC under any
// name. A disk is <folder>/<set>/<disk>.chd.
func (c *Checker) has(chain []string, r Rom) bool {
	for _, set := range chain {
		if r.Disk {
			if _, err := os.Stat(filepath.Join(c.dir, set, r.Name+".chd")); err == nil {
				return true
			}
			continue
		}
		idx := c.zip(set)
		if idx == nil {
			continue
		}
		if _, ok := idx.sizes[r.Name]; ok {
			return true
		}
		if idx.crcs[r.CRC] {
			return true
		}
	}
	return false
}
