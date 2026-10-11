// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"archive/zip"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
	"github.com/lordbasex/go-link-console/backend-device/pkg/glhd"
	"github.com/lordbasex/go-link-console/backend-device/pkg/romcheck"
)

func writeGLHD(t *testing.T, path, manifest string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for name, body := range map[string]string{"manifest.json": manifest, "level.json": "{}"} {
		w, _ := z.Create(name)
		w.Write([]byte(body))
	}
	z.Close()
	f.Close()
}

func TestLibraryListsGoLinkHDPackages(t *testing.T) {
	dir, coresDir := t.TempDir(), t.TempDir()
	writeGLHD(t, filepath.Join(dir, "neon.glhd"), `{"format": 1, "title": "Neon Run", "players": 2, "level": "level.json"}`)
	writeGLHD(t, filepath.Join(dir, "future.glhd"), `{"format": 9, "title": "Later", "level": "level.json"}`)
	_ = os.WriteFile(filepath.Join(dir, "robby.zip"), []byte("PK"), 0o644)
	writeGLHD(t, filepath.Join(dir, "robby.glhd"), `{"format": 1, "title": "Shadow", "level": "level.json"}`) // the .zip wins
	st := NewStatusService(deviceID, "test", "ws://x", "")
	lib := NewLibraryService(dir, st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	lib.SetCore(coresDir, "")
	lib.Scan()
	got := st.Snapshot().Library
	if got == nil || len(got.Roms) != 3 {
		t.Fatalf("scan %+v", got)
	}
	byName := map[string]models.RomInfo{}
	for _, r := range got.Roms {
		byName[r.Name] = r
	}
	neon := byName["neon"]
	if neon.Kind != models.KindHD || neon.Title != "Neon Run" || neon.Check == nil || neon.Check.Status != romcheck.StatusOK || neon.Controls == nil || neon.Controls.Players != 2 {
		t.Fatalf("neon %+v", neon)
	}
	if f := byName["future"]; f.Kind != models.KindHD || f.Check == nil || f.Check.Status != romcheck.StatusBadZip {
		t.Fatalf("a package of a newer format is playable: %+v", f)
	}
	if byName["robby"].Kind != "" || lib.IsHD("robby") {
		t.Fatal("a package took the place of the .zip of the same name")
	}
	if !lib.HasRom("neon") || filepath.Base(lib.RomPath("neon")) != "neon.glhd" || !lib.IsHD("neon") {
		t.Fatal("the package is not the game's file")
	}
	if res, ok := lib.CheckRom("neon"); !ok || res.Status != romcheck.StatusOK {
		t.Fatalf("check %+v", res)
	}
	if want := filepath.Join(coresDir, glhd.LibraryFile(runtime.GOOS)); lib.CoreFor("neon") != want || lib.CoreFor("robby") != lib.CorePath() {
		t.Fatalf("cores: %s and %s", lib.CoreFor("neon"), lib.CoreFor("robby"))
	}
	if lib.HasCoreFor("neon") {
		t.Fatal("go-link HD's core is not installed")
	}
	lib.SetHDCore("/elsewhere/golink_hd_libretro.so")
	if lib.CoreFor("neon") != "/elsewhere/golink_hd_libretro.so" {
		t.Fatal("--hd-core is not used")
	}
	if lib.HD("robby") != nil || lib.HD("neon").Title != "Neon Run" {
		t.Fatal("manifests")
	}
}

func TestGoLinkHDRoomsNeedTheirCoreAndShowTheirButtons(t *testing.T) {
	h := newRoomsHarness(t, 4, nil)
	writeGLHD(t, filepath.Join(h.lib.Dir(), "neon.glhd"), `{"format": 1, "title": "Neon Run", "players": 2, "level": "level.json"}`)
	h.lib.Scan()
	if err := h.rooms.Create(GameRequest{Rom: "neon"}, func(GameReply) {}); err != ErrNoCore {
		t.Fatalf("a package started without go-link HD's core: %v", err)
	}
	core := filepath.Join(t.TempDir(), "golink_hd_libretro.so")
	_ = os.WriteFile(core, []byte("core"), 0o644)
	h.lib.SetHDCore(core)
	c := h.rooms.controlsOf("neon")
	if c.Players != 2 || !slices.Equal(c.Labels, glhd.Labels) {
		t.Fatalf("controls %+v", c)
	}
	if err := h.rooms.Create(GameRequest{Rom: "neon"}, func(GameReply) {}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the game's source", func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		return len(h.games) == 1 && h.games[0].rom == "neon"
	})
}

func TestWorkerUpscalesGoLinkHD(t *testing.T) {
	src := NewWorkerSource(WorkerConfig{CorePath: "/c", RomPath: "/r.glhd", SystemDir: "/s", Upscale: 2})
	if args := src.args(); !slices.Contains(args, "--upscale") || args[slices.Index(args, "--upscale")+1] != "2" {
		t.Fatalf("args %v", args)
	}
	if args := NewWorkerSource(WorkerConfig{CorePath: "/c", RomPath: "/r.zip", SystemDir: "/s"}).args(); slices.Contains(args, "--upscale") || slices.Contains(args, "--golinkhd") {
		t.Fatalf("a MAME game is upscaled or run as go-link HD: %v", args)
	}
	if args := NewWorkerSource(WorkerConfig{CorePath: "/c", RomPath: "/r.glhd", SystemDir: "/s", Upscale: 2, Native: true}).args(); !slices.Contains(args, "--golinkhd") {
		t.Fatalf("a go-link HD game is not run by its engine: %v", args)
	}
}

func TestLibraryListsGoLinkHDBuiltInGames(t *testing.T) {
	dir, coresDir := t.TempDir(), t.TempDir()
	st := NewStatusService(deviceID, "test", "ws://x", "")
	lib := NewLibraryService(dir, st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	lib.SetCore(coresDir, "")
	lib.Scan()
	if got := st.Snapshot().Library; len(got.Roms) != 0 {
		t.Fatalf("built-in games listed without the engine: %+v", got.Roms)
	}
	_ = os.WriteFile(filepath.Join(coresDir, glhd.LibraryFile(runtime.GOOS)), []byte("engine"), 0o644)
	// a file of the folder with a built-in game's name wins
	_ = os.WriteFile(filepath.Join(dir, "glhd_platformer.zip"), []byte("PK"), 0o644)
	lib.Scan()
	got := st.Snapshot().Library
	var names []string
	for _, r := range got.Roms {
		names = append(names, r.Name)
	}
	if !slices.Equal(names, []string{"glhd_platformer", "glhd_showcase"}) || got.Roms[0].Kind != "" {
		t.Fatalf("roms %v", got.Roms)
	}
	show := got.Roms[1]
	if show.Kind != models.KindHD || show.Title != "go-link HD: Showcase" || show.Check.Status != romcheck.StatusOK || show.Controls.Players != 4 {
		t.Fatalf("showcase %+v", show)
	}
	if !lib.IsHD("glhd_showcase") || !lib.HasRom("glhd_showcase") || !lib.HasCoreFor("glhd_showcase") {
		t.Fatal("the showcase cannot be played")
	}
	if n, ok := glhd.DemoIndex(lib.RomPath("glhd_showcase")); !ok || n != 1 {
		t.Fatalf("path %q", lib.RomPath("glhd_showcase"))
	}
	if m := lib.HD("glhd_showcase"); m == nil || m.Players != 4 {
		t.Fatalf("manifest %+v", m)
	}
	if res, ok := lib.CheckRom("glhd_showcase"); !ok || res.Status != romcheck.StatusOK {
		t.Fatalf("check %+v", res)
	}
	if lib.IsHD("glhd_platformer") {
		t.Fatal("the folder's zip lost to the built-in game")
	}
}
