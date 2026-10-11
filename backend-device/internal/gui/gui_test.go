// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"archive/zip"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/test"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
	"github.com/lordbasex/go-link-console/backend-device/internal/services"
	"github.com/lordbasex/go-link-console/backend-device/pkg/romcheck"
	"github.com/lordbasex/go-link-console/backend-device/pkg/thumbnails"
)

func writeZip(t *testing.T, path, name, body string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, _ := zw.Create(name)
	w.Write([]byte(body))
	zw.Close()
	f.Close()
}

func newTestUI(t *testing.T) (*ui, *services.StatusService, string) {
	t.Helper()
	a := test.NewTempApp(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	roms, cores := t.TempDir(), t.TempDir()
	status := services.NewStatusService("7f3c2a10-1b2c-4d3e-8f90-a1b2c3d4e5f6", "test", "ws://127.0.0.1:8090/ws", roms)
	lib := services.NewLibraryService(roms, status, logger)
	lib.SetCore(cores, "")
	settings := services.NewSettingsService(lib, models.ThumbnailSettings{}, filepath.Join(t.TempDir(), "thumbnails", "MAME"), func(models.ThumbnailSettings) error { return nil })
	writeZip(t, filepath.Join(roms, "robby.zip"), "robby1.bin", "bbbb")
	writeZip(t, filepath.Join(roms, "witchgme.zip"), "a.bin", "aaaa")
	cat, err := romcheck.ParseXML(strings.NewReader(`<mame><game name="robby"><description>Robby Roto</description><year>1981</year>
		<manufacturer>Bally Midway</manufacturer><rom name="robby1.bin" size="4" crc="0f4ff68b"/></game></mame>`))
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.Save(lib.CatalogPath()); err != nil {
		t.Fatal(err)
	}
	lib.Scan()
	u := newUI(a, Options{Ctx: context.Background(), Status: status, Library: lib, Settings: settings, Version: "test", Language: "en", Logger: logger})
	u.settings.background = func(f func()) { f() }
	return u, status, roms
}

func TestWindowShowsCodeAndChecks(t *testing.T) {
	u, status, _ := newTestUI(t)
	now := time.Now()
	status.SetCode("113 134 323", now, now.Add(9*time.Minute))
	u.render(status.Snapshot())

	var code string
	for _, d := range u.onboarding.digits {
		code += d.Text
	}
	if code != "113134323" {
		t.Fatalf("code = %q", code)
	}
	test.Tap(u.onboarding.copyCode)
	// The label comes back after 2 s on another goroutine; the test driver
	// runs fyne.Do there too, so stop it before it races later renders.
	u.onboarding.copyReset.Stop()
	if got := u.app.Clipboard().Content(); got != "113134323" {
		t.Fatalf("clipboard = %q", got)
	}
	if !strings.HasPrefix(u.onboarding.refresh.Text, "A new code in 8:5") && !strings.HasPrefix(u.onboarding.refresh.Text, "A new code in 9:00") {
		t.Fatalf("refresh = %q", u.onboarding.refresh.Text)
	}
	if u.showing != "main" {
		t.Fatal("without a LinkService the window is the full one")
	}
	// Sidebar: Overview, then MAME and go-link HD under a Core heading,
	// then Settings (the computer's details live in the Overview).
	if len(u.rows) != 4 || sections[pageSettings].title != "Settings" || sections[pageMAME].title != "MAME" || sections[pageHD].title != "go-link HD" || len(u.headings) != 1 || u.headings[0].Text != "CORE" {
		t.Fatalf("sidebar: %d rows, headings %v", len(u.rows), u.headings)
	}
	test.Tap(u.rows[pageMAME])
	if u.current != pageMAME || u.mame.tab != mameTabRoms || u.page.Objects[0] != u.mame.content {
		t.Fatal("the MAME row opens the MAME page on its ROMs tab")
	}
	if u.mame.coreChip.label.Text != "Core missing" || u.mame.listChip.label.Text != "Game list" {
		t.Fatalf("chips: %q %q", u.mame.coreChip.label.Text, u.mame.listChip.label.Text)
	}
	test.Tap(u.mame.tabs[mameTabThumbs])
	if u.mame.tab != mameTabThumbs || u.mame.body.Objects[0] != u.thumbs.content {
		t.Fatal("the Thumbnails tab did not open")
	}
	u.showRoms()
	if u.mame.tab != mameTabRoms {
		t.Fatal("showRoms must open the ROMs tab")
	}
	// The ROMs tab only manages the folder: counts, not a list.
	if u.roms.total.value.Text != "2" || u.roms.runs.value.Text != "1" || u.roms.wontRun.value.Text != "1" {
		t.Fatalf("counts: %q %q %q", u.roms.total.value.Text, u.roms.runs.value.Text, u.roms.wontRun.value.Text)
	}
	// The core is not installed yet, the game list is: the ROMs tab offers
	// to download what is missing.
	if got := u.roms.core.Text; got != "mame2003-plus: not installed · game list: installed" {
		t.Fatalf("core = %q", got)
	}
	if u.roms.coreButton.Disabled() {
		t.Fatal("the core download must be enabled while the core is missing")
	}
}

func TestDropZips(t *testing.T) {
	u, status, roms := newTestUI(t)
	src := t.TempDir()
	writeZip(t, filepath.Join(src, "Gridlee.ZIP"), "gridlee.1", "cccc")
	os.WriteFile(filepath.Join(src, "notes.txt"), []byte("hi"), 0o644)
	uris := []fyne.URI{storage.NewFileURI(filepath.Join(src, "Gridlee.ZIP")), storage.NewFileURI(filepath.Join(src, "notes.txt")), storage.NewFileURI(filepath.Join(src, "Gridlee.ZIP"))}
	ok, failed := u.roms.importFiles(uris, nil)
	if ok != 1 || len(failed) != 2 {
		t.Fatalf("ok=%d failed=%v", ok, failed)
	}
	if _, err := os.Stat(filepath.Join(roms, "gridlee.zip")); err != nil {
		t.Fatal(err)
	}
	u.render(status.Snapshot())
	if len(u.roms.lib.Roms) != 3 {
		t.Fatalf("roms = %d", len(u.roms.lib.Roms))
	}
}

func TestOnlyTheCodeUntilLinked(t *testing.T) {
	a := test.NewTempApp(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	status := services.NewStatusService("7f3c2a10-1b2c-4d3e-8f90-a1b2c3d4e5f6", "test", "ws://x", t.TempDir())
	lib := services.NewLibraryService(t.TempDir(), status, logger)
	links := services.NewLinkService(services.LinkConfig{DeviceID: "7f3c2a10-1b2c-4d3e-8f90-a1b2c3d4e5f6", Logger: logger}, status)
	u := newUI(a, Options{Status: status, Library: lib, Links: links, WebURL: "https://web.example/", Language: "en", Logger: logger})
	if u.showing != "onboarding" || u.pairingURL() != "https://web.example/device" {
		t.Fatalf("showing %q, url %q", u.showing, u.pairingURL())
	}
	// A browser redeems the code and gets its token: the window opens up.
	links.PairedByCode("b1")
	links.HandleMessage("b1", []byte(`{"type":"auth"}`))
	u.render(status.Snapshot())
	if u.showing != "main" {
		t.Fatal("once a browser is remembered the full window shows")
	}
	links.UnlinkAll()
	u.render(status.Snapshot())
	if u.showing != "onboarding" {
		t.Fatal("unlinking every browser brings the code back")
	}
}

func TestResponsiveGrid(t *testing.T) {
	var tiles []fyne.CanvasObject
	for i := 0; i < 6; i++ {
		r := canvas.NewRectangle(color.White)
		r.SetMinSize(fyne.NewSize(100, 80))
		tiles = append(tiles, r)
	}
	g := &responsiveGrid{minW: 230, gap: 12}
	for _, tc := range []struct {
		width float32
		cols  int
	}{{800, 3}, {500, 2}, {250, 1}, {100, 1}} {
		g.Layout(tiles, fyne.NewSize(tc.width, 1000))
		cols := 0
		for _, o := range tiles {
			if o.Position().Y == 0 {
				cols++
			}
		}
		if cols != tc.cols {
			t.Errorf("width %v: %d columns, want %d", tc.width, cols, tc.cols)
		}
		// Cards never touch: the gap stays between them.
		if tc.cols > 1 && tiles[1].Position().X-tiles[0].Size().Width != 12 {
			t.Errorf("width %v: gap %v", tc.width, tiles[1].Position().X-tiles[0].Size().Width)
		}
	}
}

func writePNG(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 4, 3))
	img.Set(1, 1, color.White)
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func TestThumbnailsTab(t *testing.T) {
	u, status, _ := newTestUI(t)
	lib := u.opts.Library
	root := lib.ThumbnailsDir()
	// A snap named after the game title, not the set.
	writePNG(t, filepath.Join(root, "Named_Snaps", "Robby Roto.png"))
	lib.Scan()
	u.render(status.Snapshot())
	if u.thumbs.boxart.value.Text != "0 / 2" || u.thumbs.snap.value.Text != "1 / 2" {
		t.Fatalf("counts: boxart %q snap %q", u.thumbs.boxart.value.Text, u.thumbs.snap.value.Text)
	}
	if u.thumbs.dir.Text != root {
		t.Fatalf("dir = %q", u.thumbs.dir.Text)
	}

	// Drop a loose image, a pack with one sub-folder per kind and a text file.
	src := t.TempDir()
	writePNG(t, filepath.Join(src, "robby.png"))
	writePNG(t, filepath.Join(src, "pack", "Named_Titles", "witchgme.png"))
	writePNG(t, filepath.Join(src, "pack", "Named_Boxarts", "witchgme.png"))
	os.WriteFile(filepath.Join(src, "notes.txt"), []byte("hi"), 0o644)
	uris := []fyne.URI{
		storage.NewFileURI(filepath.Join(src, "robby.png")),
		storage.NewFileURI(filepath.Join(src, "pack")),
		storage.NewFileURI(filepath.Join(src, "notes.txt")),
	}
	ok, failed := u.thumbs.importThumbs(uris, nil)
	if ok != 3 || len(failed) != 1 || !strings.HasPrefix(failed[0], "notes.txt") {
		t.Fatalf("ok=%d failed=%v", ok, failed)
	}
	if got := thumbResult(ok, failed); got != "Added 3 thumbnails. Not added: notes.txt (not a PNG or JPG)." {
		t.Fatalf("result = %q", got)
	}
	for _, p := range []string{"Named_Boxarts/robby.png", "Named_Boxarts/witchgme.png", "Named_Titles/witchgme.png"} {
		if _, err := os.Stat(filepath.Join(root, p)); err != nil {
			t.Fatal(err)
		}
	}
	u.render(status.Snapshot())
	for _, tc := range []struct {
		tile *statTile
		want string
	}{{u.thumbs.boxart, "2 / 2"}, {u.thumbs.title, "1 / 2"}, {u.thumbs.snap, "1 / 2"}} {
		if tc.tile.value.Text != tc.want {
			t.Errorf("tile = %q, want %q", tc.tile.value.Text, tc.want)
		}
	}
	if got := thumbMarks(u.thumbs.lib.Roms[0].Thumbs); got != [3]bool{true, false, true} {
		t.Fatalf("robby marks = %v", got)
	}
	// The ROMs tab shows the new Boxart.
	if p, ok := lib.ThumbnailPath("robby", thumbnails.Boxart); !ok || filepath.Base(p) != "robby.png" {
		t.Fatalf("boxart = %q %v", p, ok)
	}
}

func TestSettingsPage(t *testing.T) {
	u, status, _ := newTestUI(t)
	settings, lib := u.opts.Settings, u.opts.Library
	test.Tap(u.rows[pageSettings])
	if u.current != pageSettings || u.page.Objects[0] != u.settings.content {
		t.Fatal("the Settings row opens the Settings page")
	}
	p := u.settings
	if p.cat != settingsGeneral {
		t.Fatalf("category %d", p.cat)
	}
	test.Tap(p.cats[settingsThumbnails])
	if p.kind.Selected != "Boxart" {
		t.Fatalf("kind %q", p.kind.Selected)
	}
	if p.dir.Text != settings.ThumbnailsDir() || !p.reset.Disabled() {
		t.Fatalf("dir %q (want %q), default enabled %v", p.dir.Text, settings.ThumbnailsDir(), !p.reset.Disabled())
	}

	// Choosing Snap saves it and the library shows Snaps from then on.
	writePNG(t, filepath.Join(settings.ThumbnailsDir(), "Named_Snaps", "robby.png"))
	test.Tap(p.kind.segments[2])
	if settings.Thumbnails().Kind != "snap" || lib.ThumbnailKind() != thumbnails.Snap {
		t.Fatalf("kind = %q / %q", settings.Thumbnails().Kind, lib.ThumbnailKind())
	}
	if _, ok := lib.ThumbnailPath("robby", lib.ThumbnailKind()); !ok {
		t.Fatal("the ROM list must find the Snap")
	}

	// A folder that does not exist is refused with a message.
	p.change(func(ts *models.ThumbnailSettings) { ts.Dir = filepath.Join(t.TempDir(), "missing") })
	if !p.thumbErr.Visible() || !strings.Contains(p.thumbErr.Text, "existing folder") {
		t.Fatalf("error = %q (visible %v)", p.thumbErr.Text, p.thumbErr.Visible())
	}
	other := t.TempDir()
	p.change(func(ts *models.ThumbnailSettings) { ts.Dir = other })
	if p.thumbErr.Visible() || p.dir.Text != other || p.reset.Disabled() {
		t.Fatalf("dir %q, error %q", p.dir.Text, p.thumbErr.Text)
	}
	test.Tap(p.reset)
	if settings.Thumbnails().Dir != "" || p.dir.Text == other {
		t.Fatalf("Default did not go back: %q", p.dir.Text)
	}

	test.Tap(p.cats[settingsNetwork])
	u.render(status.Snapshot())
	if p.body.Objects[0] != p.pages[settingsNetwork] || p.signal.Text != "ws://127.0.0.1:8090/ws" {
		t.Fatalf("network: signal %q", p.signal.Text)
	}
	test.Tap(p.cats[settingsRooms])
	if p.maxRooms.Text != "4 by default" || !strings.HasSuffix(p.saves.Text, filepath.Join("go-link", "saves")) {
		t.Fatalf("rooms: %q %q", p.maxRooms.Text, p.saves.Text)
	}
}

func TestWindowRemembersItsSize(t *testing.T) {
	u, _, _ := newTestUI(t)
	if got := u.savedSize(); got != defaultSize {
		t.Fatalf("first run size %v", got)
	}
	u.win.Resize(fyne.NewSize(1500, 950))
	u.rememberSize()
	if got := u.savedSize(); got.Width != 1500 || got.Height != 950 {
		t.Fatalf("remembered %v", got)
	}
	// A size too small to use is not kept.
	u.win.Resize(fyne.NewSize(300, 200))
	u.rememberSize()
	if got := u.savedSize(); got.Width != 1500 {
		t.Fatalf("kept a tiny size: %v", got)
	}
}

func TestTheWindowChangesLanguage(t *testing.T) {
	u, status, _ := newTestUI(t)
	var saved string
	u.opts.SetLanguage = func(id string) error { saved = id; return nil }
	test.Tap(u.rows[pageSettings])
	test.Tap(u.settings.language.segments[2]) // Español
	if saved != "es" || current != "es" {
		t.Fatalf("saved %q, current %q", saved, current)
	}
	u.render(status.Snapshot())
	if u.current != pageSettings || u.rows[pageOverview].label.Text != "Resumen" {
		t.Fatalf("page %d, overview row %q", u.current, u.rows[pageOverview].label.Text)
	}
	test.Tap(u.settings.language.segments[1]) // English
	if u.rows[pageOverview].label.Text != "Overview" {
		t.Fatalf("back to English: %q", u.rows[pageOverview].label.Text)
	}
	// Every text of the window has its Spanish and Portuguese version.
	for _, lang := range []string{"es", "pt"} {
		for _, key := range guiKeys(t) {
			if _, ok := catalogs[lang][key]; !ok {
				t.Errorf("%s: missing %q", lang, key)
			}
		}
	}
}

// guiKeys lists every English text of the window: the literals passed to
// L and Lf, and the texts translated through variables.
func guiKeys(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`\bLf?\(("(?:[^"\\]|\\.)*")`)
	seen := map[string]bool{}
	var keys []string
	add := func(k string) {
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || strings.HasPrefix(f, "i18n") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllSubmatch(src, -1) {
			k, err := strconv.Unquote(string(m[1]))
			if err != nil {
				t.Fatal(err)
			}
			add(k)
		}
	}
	for _, s := range sections {
		add(s.title)
		if s.group != "" {
			add(s.group)
		}
	}
	for _, c := range settingsCategories {
		add(c.title)
	}
	for _, k := range kindLabels[1:] {
		add(k)
	}
	for _, k := range hdCodecLabels {
		add(k)
	}
	add(languages[0].label)
	add(thumbHelp)
	for _, s := range []string{"connected", "connecting", "disconnected"} {
		add(s)
	}
	return keys
}

func TestSettingsChangeTheVideoQuality(t *testing.T) {
	u, _, _ := newTestUI(t)
	if u.settings.quality.Selected != "High" {
		t.Fatalf("selected %q", u.settings.quality.Selected)
	}
	test.Tap(u.settings.quality.segments[2]) // Saver
	if q := u.opts.Settings.VideoQuality(); q != "saver" {
		t.Fatalf("quality %q", q)
	}
	test.Tap(u.settings.quality.segments[1]) // Normal
	if q := u.opts.Settings.VideoQuality(); q != "normal" || u.settings.quality.Selected != "Normal" {
		t.Fatalf("quality %q, selected %q", q, u.settings.quality.Selected)
	}
}

func TestTheGoLinkHDPageChangesTheScreen(t *testing.T) {
	u, status, _ := newTestUI(t)
	hd := services.NewHDGamesService(services.HDGamesConfig{Library: u.opts.Library, Status: status, Settings: u.opts.Settings, Offered: true})
	u.opts.HDGames = hd
	u.render(status.Snapshot())
	test.Tap(u.rows[pageHD])
	if u.current != pageHD || u.page.Objects[0] != u.hd.content {
		t.Fatal("the go-link HD row opens its page")
	}
	if u.hd.roomSize.Selected != "720p" || u.hd.codec.Selected != "Automatic" {
		t.Fatalf("defaults %q %q", u.hd.roomSize.Selected, u.hd.codec.Selected)
	}
	row := u.hd.starter["antidoto"]
	if row == nil || row.state.Text != "Not installed" || row.sizes["720p"].Disabled() {
		t.Fatalf("starter row %+v", row)
	}
	test.Tap(u.hd.tabs[hdTabScreen])
	test.Tap(u.hd.roomSize.segments[1]) // 1080p
	test.Tap(u.hd.codec.segments[1])    // VP8
	if got := u.opts.Settings.HD(); got.RoomSize != "1080p" || got.Codec != "vp8" {
		t.Fatalf("saved %+v", got)
	}
	u.render(status.Snapshot())
	if st := status.Snapshot().HD; st == nil || st.RoomSize != "1080p" || st.Codec != "vp8" {
		t.Fatalf("status %+v", st)
	}
}
