// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
	"github.com/lordbasex/go-link-console/backend-device/internal/services"
	"github.com/lordbasex/go-link-console/backend-device/pkg/instancelock"
	"github.com/lordbasex/go-link-console/backend-device/pkg/ownsets"
	"github.com/lordbasex/go-link-console/backend-device/pkg/romcheck"
)

// usage prints the help of the device and its subcommands.
func usage() {
	out := flag.CommandLine.Output()
	fmt.Fprintf(out, `go-link device %s

Usage:
  device [flags]                         run the device
  device core download                   download the emulator core and its game list
  device roms dir [PATH]                 show or change the ROM folder
  device roms check [--dir D] [--json]   check which ROM sets the core can run
  device roms saves [--json]             test which games can resume from a save (a few seconds each)
  device romtest [--frames N] [--json] [--shot FILE] ZIP
                                         power a set on with the exact core: picture, sound,
                                         inputs (the website's "Test on my go-link")
  device hdbench --far PICTURE [--play PICTURE] [--res 720p,1080p,2160p] [--encoder vp8|videotoolbox|x264|mediafoundation]
  device hdprobe --far PICTURE [--play PICTURE] [--json]   (what go-link HD streams here at 60 fps)
                                         go-link HD's experiment: encode an HD test scene and
                                         measure the time per frame, bitrate and CPU
  device thumbnails check [--json]       count the thumbnails of the ROM sets
  device thumbnails dir [PATH|default]   show or change the thumbnails folder
  device thumbnails kind [boxart|title|snap]
                                         show or change which thumbnail is shown
  device video quality [high|normal|saver]
                                         show or change the video quality of game rooms
  device hd dir [PATH|default]           show or change go-link HD's games folder
  device hd screen [720p|1080p] [auto|vp8]
                                         show or change go-link HD rooms' size and codec
  device hd games                        list go-link HD's free starter games
  device hd download GAME [--size 720p|1080p]
                                         download a starter game into the games folder
  device panel token [--new]             show (or replace) the web panel token
  device rec list [--json]               list the recordings of game rooms (~/go-link-console/rec)
  device rec rm ID...|--all              delete recordings
  device telemetry rooms                 list the rooms with telemetry (~/go-link-console/telemetry.db)
  device telemetry runs ROOM             the times a room was on, with their game ids
  device telemetry events ROOM|#GAME [--warn] [--since 30m]
                                         a room's (or one game's) log of events
  device telemetry incidents ROOM|#GAME [--json]
                                         every freeze, and where it most likely came from
  device telemetry export ROOM|#GAME [--since 2h]
                                         everything recorded, as JSON lines
  device reset --yes                     factory reset (device stopped): rooms, saved games,
                                         history, recordings, links and settings go;
                                         ROMs, thumbnails and the emulator stay

Every subcommand accepts --config PATH.

Flags:
`, version)
	flag.PrintDefaults()
}

// runCommand runs a subcommand. handled is false when args start with a
// flag or are empty: then the device itself runs.
func runCommand(args []string) (handled bool, err error) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return false, nil
	}
	// A game room's emulator: a child process the device starts itself
	// (no window, config or single-instance lock).
	if args[0] == "emulate" {
		return true, runEmulate(args[1:])
	}
	// The ROM test (and its worker process): no config either.
	if args[0] == "romtest" {
		return true, cmdRomTest(args[1:])
	}
	// go-link HD's streaming experiment (T-31): no config either.
	if args[0] == "hdbench" {
		return true, cmdHDBench(args[1:])
	}
	if args[0] == "hdprobe" {
		return true, cmdHDProbe(args[1:])
	}
	key := args[0]
	if len(args) > 1 {
		key += " " + args[1]
	}
	cmds := map[string]func([]string) error{
		"core download":       cmdCoreDownload,
		"roms dir":            cmdRomsDir,
		"roms check":          cmdRomsCheck,
		"roms saves":          cmdRomsSaves,
		"thumbnails check":    cmdThumbnailsCheck,
		"thumbnails dir":      cmdThumbnailsDir,
		"thumbnails kind":     cmdThumbnailsKind,
		"panel token":         cmdPanelToken,
		"video quality":       cmdVideoQuality,
		"hd dir":              cmdHDDir,
		"hd screen":           cmdHDScreen,
		"hd games":            cmdHDGames,
		"hd download":         cmdHDDownload,
		"rec list":            cmdRecList,
		"rec rm":              cmdRecRm,
		"telemetry rooms":     cmdTelemetryRooms,
		"telemetry runs":      cmdTelemetryRuns,
		"telemetry export":    cmdTelemetryExport,
		"telemetry events":    cmdTelemetryEvents,
		"telemetry incidents": cmdTelemetryIncidents,
	}
	if args[0] == "reset" {
		return true, cmdReset(args[1:])
	}
	if args[0] == "help" {
		flag.CommandLine.SetOutput(os.Stdout)
		usage()
		return true, nil
	}
	cmd, ok := cmds[key]
	if !ok {
		usage()
		return true, fmt.Errorf("unknown command %q", strings.Join(args, " "))
	}
	return true, cmd(args[2:])
}

// cliEnv is what a subcommand needs.
type cliEnv struct {
	cfg      models.Config
	status   *services.StatusService
	library  *services.LibraryService
	settings *services.SettingsService
}

// newFlags returns the flag set of a subcommand, with --config.
func newFlags(name string) (*flag.FlagSet, *string) {
	fs := flag.NewFlagSet("device "+name, flag.ContinueOnError)
	return fs, fs.String("config", "", "path to device.json (default: user config dir)")
}

// parseFlags parses args with flags before or after the positional
// arguments (device telemetry export #ID --since 2h), which flag.Parse alone
// stops reading at the first positional one. Everything after "--" stays
// positional.
func parseFlags(fs *flag.FlagSet, args []string) error {
	var pos []string
	for rest := args; ; {
		if err := fs.Parse(rest); err != nil {
			return err
		}
		left := fs.Args()
		if len(left) == 0 {
			break
		}
		if n := len(rest) - len(left); n > 0 && rest[n-1] == "--" {
			pos = append(pos, left...)
			break
		}
		pos = append(pos, left[0])
		rest = left[1:]
	}
	return fs.Parse(append([]string{"--"}, pos...))
}

func openEnv(configPath string) (*cliEnv, error) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	store, cfg, _, err := loadConfig(configPath)
	if err != nil {
		return nil, err
	}
	env := &cliEnv{}
	env.status, env.library, env.settings, _, err = openLibrary(store, &cfg, "", "", logger)
	env.cfg = cfg
	return env, err
}

// interruptible returns a context canceled by Ctrl-C.
func interruptible() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func cmdCoreDownload(args []string) error {
	fs, config := newFlags("core download")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	env, err := openEnv(*config)
	if err != nil {
		return err
	}
	lib := env.library
	if lib.HasCore() && lib.Catalog() != nil {
		fmt.Println("The emulator core and its game list are already installed in", filepath.Dir(lib.CorePath()))
		return nil
	}
	fmt.Println("Downloading the emulator core and its game list...")
	ctx, stop := interruptible()
	defer stop()
	if err := lib.InstallCore(ctx); err != nil {
		return err
	}
	fmt.Println("Installed in", filepath.Dir(lib.CorePath()))
	return nil
}

func cmdRomsDir(args []string) error {
	fs, config := newFlags("roms dir")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	env, err := openEnv(*config)
	if err != nil {
		return err
	}
	if fs.NArg() == 0 {
		fmt.Println(env.library.Dir())
		return nil
	}
	dir, err := filepath.Abs(fs.Arg(0))
	if err != nil {
		return err
	}
	if err := env.library.SetDir(dir); err != nil {
		return err
	}
	n := 0
	if lib := env.status.Snapshot().Library; lib != nil {
		n = len(lib.Roms)
	}
	fmt.Printf("ROM folder set to %s (%d sets). A running device picks it up on its next start.\n", dir, n)
	return nil
}

// romReport is one line of "roms check".
type romReport struct {
	Name  string          `json:"name"`
	Title string          `json:"title,omitempty"`
	Check romcheck.Result `json:"check"`
	// Own is set for a set go-link made itself, verified by the SHA-256 of
	// every file inside the zip; Title is then go-link's game.
	Own bool `json:"own,omitempty"`
}

func cmdRomsCheck(args []string) error {
	fs, config := newFlags("roms check")
	dirFlag := fs.String("dir", "", "folder to check (default: the ROM folder)")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	env, err := openEnv(*config)
	if err != nil {
		return err
	}
	cat := env.library.Catalog()
	if cat == nil {
		return errors.New("the core's game list is not installed; run: device core download")
	}
	dir := env.library.Dir()
	if *dirFlag != "" {
		dir = *dirFlag
	}
	reports, err := checkFolder(cat, dir)
	if err != nil {
		return err
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(reports)
	}
	printReports(os.Stdout, dir, reports)
	return nil
}

// saveReport is one line of "roms saves".
type saveReport struct {
	Name  string `json:"name"`
	Title string `json:"title,omitempty"`
	Saves bool   `json:"saves"`
	Error string `json:"error,omitempty"`
}

// cmdRomsSaves tests every set that runs: can a room resume it from a save,
// or does the emulator leave something out (then it always starts over)?
func cmdRomsSaves(args []string) error {
	fs, config := newFlags("roms saves")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	env, err := openEnv(*config)
	if err != nil {
		return err
	}
	ctx, cancel := interruptible()
	defer cancel()
	env.library.RefreshCatalog(ctx) // a game list from an older device is fetched again
	cat := env.library.Catalog()
	if cat == nil {
		return errors.New("the core's game list is not installed; run: device core download")
	}
	reports, err := checkFolder(cat, env.library.Dir())
	if err != nil {
		return err
	}
	probes := saveProbes(env.library)
	var out []saveReport
	for _, r := range reports {
		if r.Check.Status != romcheck.StatusOK {
			continue
		}
		rep := saveReport{Name: r.Name, Title: r.Title}
		if rep.Saves, err = probes.SavesWork(ctx, r.Name); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			rep.Error = err.Error()
		}
		if !*asJSON {
			state := "resumes"
			switch {
			case rep.Error != "":
				state = "error: " + rep.Error
			case !rep.Saves:
				state = "starts over (the emulator does not save it whole)"
			}
			fmt.Printf("%-12s %s — %s\n", rep.Name, rep.Title, state)
		}
		out = append(out, rep)
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	n := 0
	for _, r := range out {
		if r.Saves && r.Error == "" {
			n++
		}
	}
	fmt.Printf("\n%d of %d games resume from a save; the others always start over.\n", n, len(out))
	return nil
}

// checkFolder checks every set in dir without running anything.
func checkFolder(cat *romcheck.Catalog, dir string) ([]romReport, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	checker := romcheck.NewChecker(cat, dir)
	var out []romReport
	for _, e := range entries {
		name, ok := strings.CutSuffix(strings.ToLower(e.Name()), ".zip")
		if !ok || e.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		r := romReport{Name: name, Check: checker.Check(name)}
		if g := cat.Game(name); g != nil {
			r.Title = g.Title
		}
		if own, _ := ownsets.Match(filepath.Join(dir, e.Name()), ownsets.All()); own != nil {
			r.Title, r.Own = own.Title, true
		}
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b romReport) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

func printReports(w io.Writer, dir string, reports []romReport) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SET\tSTATUS\tGAME")
	counts := map[romcheck.Status]int{}
	for _, r := range reports {
		counts[r.Check.Status]++
		line := r.Title
		if reason := r.Check.Reason(); reason != "" {
			if line != "" {
				line += ": "
			}
			line += reason
		} else if r.Check.Driver != "" {
			line += " (driver " + r.Check.Driver + ": may not run well)"
		}
		if r.Own {
			line += " [go-link set, verified]"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", r.Name, r.Check.Status, line)
	}
	tw.Flush()
	fmt.Fprintf(w, "\n%s: %d of %d sets run on mame2003-plus", dir, counts[romcheck.StatusOK], len(reports))
	var rest []string
	for _, s := range []romcheck.Status{romcheck.StatusMissing, romcheck.StatusUnsupported, romcheck.StatusBIOS, romcheck.StatusBadZip} {
		if counts[s] > 0 {
			rest = append(rest, fmt.Sprintf("%d %s", counts[s], s))
		}
	}
	if len(rest) > 0 {
		fmt.Fprintf(w, " (%s)", strings.Join(rest, ", "))
	}
	fmt.Fprintln(w, ".")
}

// thumbReport is the result of "thumbnails check".
type thumbReport struct {
	ThumbnailsDir string   `json:"thumbnails_dir"`
	RomsDir       string   `json:"roms_dir"`
	Sets          int      `json:"sets"`
	Boxart        int      `json:"boxart"`
	Title         int      `json:"title"`
	Snap          int      `json:"snap"`
	WithoutAny    []string `json:"without_thumbnails"`
}

func cmdThumbnailsCheck(args []string) error {
	fs, config := newFlags("thumbnails check")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	env, err := openEnv(*config)
	if err != nil {
		return err
	}
	// openEnv scanned the ROM folder, which also looks for thumbnails.
	var lib models.Library
	if l := env.status.Snapshot().Library; l != nil {
		lib = *l
	}
	report := thumbnailReport(lib)
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	printThumbReport(os.Stdout, report)
	return nil
}

// thumbnailReport counts which thumbnails the library's sets have.
func thumbnailReport(lib models.Library) thumbReport {
	r := thumbReport{ThumbnailsDir: lib.ThumbnailsDir, RomsDir: lib.Dir, Sets: len(lib.Roms), WithoutAny: []string{}}
	for _, rom := range lib.Roms {
		t := rom.Thumbs
		if t.Boxart {
			r.Boxart++
		}
		if t.Title {
			r.Title++
		}
		if t.Snap {
			r.Snap++
		}
		if !t.Boxart && !t.Title && !t.Snap {
			r.WithoutAny = append(r.WithoutAny, rom.Name)
		}
	}
	return r
}

func printThumbReport(w io.Writer, r thumbReport) {
	fmt.Fprintf(w, "Thumbnails in %s for the %d sets in %s:\n", r.ThumbnailsDir, r.Sets, r.RomsDir)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, row := range []struct {
		kind string
		n    int
	}{{"Boxart", r.Boxart}, {"Title", r.Title}, {"Snap", r.Snap}} {
		fmt.Fprintf(tw, "  %s\t%d / %d\n", row.kind, row.n, r.Sets)
	}
	tw.Flush()
	if len(r.WithoutAny) == 0 {
		fmt.Fprintln(w, "\nEvery set has at least one thumbnail.")
		return
	}
	fmt.Fprintf(w, "\nSets without any thumbnail (%d):\n", len(r.WithoutAny))
	for _, name := range r.WithoutAny {
		fmt.Fprintln(w, "  "+name)
	}
}

func cmdThumbnailsDir(args []string) error {
	fs, config := newFlags("thumbnails dir")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	env, err := openEnv(*config)
	if err != nil {
		return err
	}
	return thumbnailsDir(os.Stdout, env.settings, fs.Args())
}

// thumbnailsDir shows the thumbnails folder, or sets it ("default" goes
// back to ~/go-link-console/thumbnails/MAME).
func thumbnailsDir(w io.Writer, settings *services.SettingsService, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(w, settings.ThumbnailsDir())
		return nil
	}
	t := settings.Thumbnails()
	if args[0] == "default" {
		t.Dir = ""
	} else {
		dir, err := filepath.Abs(args[0])
		if err != nil {
			return err
		}
		t.Dir = dir
	}
	if err := settings.SetThumbnails(t); err != nil {
		if errors.Is(err, services.ErrBadSetting) {
			return fmt.Errorf("%s is not an existing folder", t.Dir)
		}
		return err
	}
	fmt.Fprintf(w, "Thumbnails folder set to %s. A running device picks it up on its next start.\n", settings.ThumbnailsDir())
	return nil
}

func cmdThumbnailsKind(args []string) error {
	fs, config := newFlags("thumbnails kind")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	env, err := openEnv(*config)
	if err != nil {
		return err
	}
	return thumbnailsKind(os.Stdout, env.settings, fs.Args())
}

// thumbnailsKind shows or sets which thumbnail the device and the
// browsers show.
func thumbnailsKind(w io.Writer, settings *services.SettingsService, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(w, settings.Thumbnails().Kind)
		return nil
	}
	t := settings.Thumbnails()
	t.Kind = strings.ToLower(args[0])
	if err := settings.SetThumbnails(t); err != nil {
		if errors.Is(err, services.ErrBadSetting) {
			return fmt.Errorf("unknown kind %q (use boxart, title or snap)", args[0])
		}
		return err
	}
	fmt.Fprintf(w, "Thumbnails shown: %s. A running device picks it up on its next start.\n", t.Kind)
	return nil
}

func cmdVideoQuality(args []string) error {
	fs, config := newFlags("video quality")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	env, err := openEnv(*config)
	if err != nil {
		return err
	}
	return videoQuality(os.Stdout, env.settings, fs.Args())
}

// videoQuality shows or sets the video quality of game rooms.
func videoQuality(w io.Writer, settings *services.SettingsService, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(w, settings.VideoQuality())
		return nil
	}
	q := strings.ToLower(args[0])
	if err := settings.SetVideoQuality(q); err != nil {
		if errors.Is(err, services.ErrBadSetting) {
			return fmt.Errorf("unknown quality %q (use high, normal or saver)", args[0])
		}
		return err
	}
	fmt.Fprintf(w, "Video quality set to %s. A running device picks it up on its next start.\n", q)
	return nil
}

// cmdPanelToken shows the token the local web panel asks for, making one
// the first time. --new replaces it; that needs the device stopped, so the
// running device never writes the old one back.
func cmdPanelToken(args []string) error {
	fs, config := newFlags("panel token")
	renew := fs.Bool("new", false, "replace the token: browsers that know the old one must type the new one")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	store, cfg, _, err := loadConfig(*config)
	if err != nil {
		return err
	}
	if *renew || cfg.PanelToken == "" {
		lock, err := instancelock.Acquire(filepath.Join(filepath.Dir(store.Path()), "device.lock"))
		if err != nil {
			return errors.New("the device is running: stop it first to make a new panel token")
		}
		defer lock.Release()
	}
	token, err := panelToken(store, &cfg, *renew)
	if err != nil {
		return err
	}
	fmt.Println(token)
	return nil
}
