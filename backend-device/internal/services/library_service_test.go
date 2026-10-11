// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
	"github.com/lordbasex/go-link-console/backend-device/pkg/ownsets"
	"github.com/lordbasex/go-link-console/backend-device/pkg/romcheck"
	"github.com/lordbasex/go-link-console/backend-device/pkg/thumbnails"
)

func TestLibraryScan(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "mygame.zip"), []byte("PK mine"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "robby.zip"), []byte("PK"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644)
	st := NewStatusService(deviceID, "test", "ws://x", "")
	lib := NewLibraryService(dir, st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	thumbs := t.TempDir()
	lib.SetThumbnailsDir(thumbs)
	if err := lib.ImportThumbnail(thumbnails.Boxart, "robby.png", bytes.NewReader(tinyPNG(t))); err != nil {
		t.Fatal(err)
	}

	lib.Scan()
	got := st.Snapshot().Library
	if got == nil || len(got.Roms) != 2 || got.Roms[0].Name != "mygame" || got.Roms[1].Name != "robby" || got.Dir != dir || got.ThumbnailsDir != thumbs {
		t.Fatalf("scan %+v", got)
	}
	if got.Roms[0].Thumbs.Boxart || !got.Roms[1].Thumbs.Boxart || got.Roms[1].Thumbs.Snap {
		t.Fatalf("thumbs %+v", got.Roms)
	}
	if p, ok := lib.ThumbnailPath("robby", thumbnails.Boxart); !ok || filepath.Base(p) != "robby.png" {
		t.Fatal("thumbnail path")
	}
	if _, ok := lib.ThumbnailPath("../secret", thumbnails.Boxart); ok {
		t.Fatal("path traversal accepted")
	}
	if b, err := lib.Thumbnail("robby", thumbnails.Boxart, 64, 86, 3000); err != nil || len(b) == 0 {
		t.Fatalf("small thumbnail: %v", err)
	}
	if err := lib.ImportThumbnail(thumbnails.Boxart, "notes.txt", strings.NewReader("x")); err == nil {
		t.Fatal("a text file is not a thumbnail")
	}
}

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 8, 10))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestSetDirAndImport(t *testing.T) {
	st := NewStatusService(deviceID, "test", "ws://x", "")
	lib := NewLibraryService(t.TempDir(), st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	var saved string
	lib.OnDirChange(func(d string) { saved = d })

	mine := t.TempDir()
	_ = os.WriteFile(filepath.Join(mine, "sf2.zip"), []byte("PK\x03\x04 street fighter"), 0o644)
	if err := lib.SetDir("relative/path"); !errors.Is(err, ErrBadFolder) {
		t.Fatalf("relative: %v", err)
	}
	if err := lib.SetDir(filepath.Join(mine, "missing")); !errors.Is(err, ErrBadFolder) {
		t.Fatalf("missing: %v", err)
	}
	if err := lib.SetDir(mine); err != nil || saved != mine || lib.Dir() != mine {
		t.Fatalf("set: %v saved=%q", err, saved)
	}
	if got := st.Snapshot().Library; len(got.Roms) != 1 || got.Roms[0].Name != "sf2" || got.Dir != mine {
		t.Fatalf("scan after switch %+v", got)
	}

	zip := "PK\x03\x04 data"
	if err := lib.Import("/Users/x/Downloads/Robby.ZIP", strings.NewReader(zip)); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(filepath.Join(mine, "robby.zip")); err != nil || fi.Mode().Perm() != 0o644 {
		t.Fatalf("imported file must be readable by all: %v", err)
	}
	if err := placeNoOverwrite(filepath.Join(mine, "robby.zip"), filepath.Join(mine, "robby.zip")); err == nil {
		t.Fatal("placing onto an existing file must fail")
	}
	if _, err := os.Stat(filepath.Join(mine, "robby.zip")); err != nil {
		t.Fatal("import not written")
	}
	for name, body := range map[string]string{
		"robby.zip":                      zip,          // already there
		"notes.txt":                      "PK\x03\x04", // not a zip name
		"../../evil.zip":                 "nope",       // not a zip file
		"way_too_long_name_for_mame.zip": zip,
	} {
		if err := lib.Import(name, strings.NewReader(body)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(mine), "evil.zip")); err == nil {
		t.Fatal("import escaped the folder")
	}
}

func TestLibraryChecksRoms(t *testing.T) {
	st := NewStatusService(deviceID, "test", "ws://x", "")
	roms, coresDir := t.TempDir(), t.TempDir()
	lib := NewLibraryService(roms, st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	lib.SetCore(coresDir, "")
	writeTestZip(t, filepath.Join(roms, "robby.zip"), "robby1.bin", "bbbb")
	writeTestZip(t, filepath.Join(roms, "newgame.zip"), "a.bin", "aaaa")

	lib.Scan()
	if lib := st.Snapshot().Library; lib.Core.Catalog || lib.Roms[1].Check != nil {
		t.Fatal("without the game list there is no check")
	}
	if _, ok := lib.CheckRom("robby"); ok {
		t.Fatal("CheckRom must report that the list is missing")
	}

	cat, err := romcheck.ParseXML(strings.NewReader(`<mame><game name="robby"><description>Robby Roto</description><year>1981</year>
		<rom name="robby1.bin" size="4" crc="0f4ff68b"/></game></mame>`))
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.Save(lib.CatalogPath()); err != nil {
		t.Fatal(err)
	}
	lib.Scan()
	got := st.Snapshot().Library
	if !got.Core.Catalog {
		t.Fatal("the game list must be reported")
	}
	byName := map[string]models.RomInfo{}
	for _, r := range got.Roms {
		byName[r.Name] = r
	}
	if r := byName["robby"]; r.Check == nil || r.Check.Status != romcheck.StatusOK || r.Title != "Robby Roto" || r.Year != "1981" {
		t.Fatalf("robby = %+v", r)
	}
	if r := byName["newgame"]; r.Check == nil || r.Check.Status != romcheck.StatusUnsupported {
		t.Fatalf("newgame = %+v", r)
	}
}

func writeTestZip(t *testing.T, path, name, body string) {
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

func TestImportNeverFillsTheDisk(t *testing.T) {
	st := NewStatusService(deviceID, "test", "ws://x", "")
	lib := NewLibraryService(t.TempDir(), st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	free := uint64(MinFreeSpace / 2)
	saved := diskFree
	diskFree = func(string) (uint64, bool) { return free, true }
	t.Cleanup(func() { diskFree = saved })

	// Already below the reserve: refused before copying anything.
	if err := lib.Import("robby.zip", strings.NewReader("PK\x03\x04 data")); !errors.Is(err, ErrDiskFull) {
		t.Fatalf("full disk: %v", err)
	}
	// Room for 10 bytes above the reserve: a bigger set stops mid-copy.
	free = MinFreeSpace + 10
	if err := lib.Import("robby.zip", strings.NewReader("PK\x03\x04"+strings.Repeat("x", 100))); !errors.Is(err, ErrDiskFull) {
		t.Fatalf("too big for the disk: %v", err)
	}
	if lib.HasRom("robby") {
		t.Fatal("a half copied set was kept")
	}
	// A small one fits.
	if err := lib.Import("robby.zip", strings.NewReader("PK\x03\x04 ok")); err != nil {
		t.Fatalf("small set: %v", err)
	}
}

func TestLibraryOwnSets(t *testing.T) {
	dir := t.TempDir()
	prog := []byte("our own program")
	writeOwnZip(t, filepath.Join(dir, "robby.zip"), map[string][]byte{"robby.1": prog})
	sum := sha256.Sum256(prog)
	list := []ownsets.Set{{ID: "ours", Set: "robby", Title: "Our Game", Description: "Made by us", Year: "2026", Maker: "go-link",
		Players: 4, Buttons: 3, Control: "joy8way", Labels: []string{"Jump", "Fire", "Special"}, Art: "willy-proto.png",
		Files: []ownsets.File{{Name: "robby.1", Size: int64(len(prog)), SHA256: hex.EncodeToString(sum[:])}}}}
	st := NewStatusService(deviceID, "test", "ws://x", "")
	lib := NewLibraryService(dir, st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	lib.own = ownsets.NewMatcher(list)
	thumbs := t.TempDir()
	lib.SetThumbnailsDir(thumbs)
	// The host's picture of the original game must not show for ours.
	if err := lib.ImportThumbnail(thumbnails.Snap, "robby.png", bytes.NewReader(tinyPNG(t))); err != nil {
		t.Fatal(err)
	}
	lib.Scan()
	r := st.Snapshot().Library.Roms[0]
	if !r.Own || r.Title != "Our Game" || r.Maker != "go-link" || r.Description != "Made by us" || r.Controls == nil || r.Controls.Buttons != 3 || r.Controls.Labels[2] != "Special" {
		t.Fatalf("own set %+v", r)
	}
	if !r.Thumbs.Boxart || !r.Thumbs.Snap || lib.Title("robby") != "Our Game" {
		t.Fatalf("own picture %+v", r.Thumbs)
	}
	if b, err := lib.Thumbnail("robby", thumbnails.Snap, 96, 128, 0); err != nil || len(b) < 100 {
		t.Fatalf("own picture: %v", err)
	}

	// The same name with other bytes: the original set, never ours.
	prog[0] ^= 1
	writeOwnZip(t, filepath.Join(dir, "robby.zip"), map[string][]byte{"robby.1": prog})
	lib.Scan()
	r = st.Snapshot().Library.Roms[0]
	if r.Own || r.Title == "Our Game" || r.Controls != nil || r.Thumbs.Boxart || !r.Thumbs.Snap {
		t.Fatalf("a name match was trusted: %+v", r)
	}
}

func writeOwnZip(t *testing.T, path string, files map[string][]byte) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(data)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	// A new modification time, so caches see the change.
	later := time.Now().Add(time.Duration(len(buf.Bytes())) * time.Millisecond)
	_ = os.Chtimes(path, later, later)
}

// A Willy Maker project or AI pack dropped as a ROM gets its own answer,
// by its name or by what is inside a renamed one (T-21).
func TestImportRefusesProjectsAndAiPacks(t *testing.T) {
	st := NewStatusService(deviceID, "test", "ws://x", "")
	lib := NewLibraryService(t.TempDir(), st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	withFile := func(name string) []byte {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte("{}"))
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	for _, name := range []string{"dead-air.willy.zip", "Dead-Air.AI-PACK.zip"} {
		if _, err := lib.CheckImportName(name); !errors.Is(err, ErrNotRom) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	for _, inside := range []string{"project.json", "PROMPT.md"} {
		if err := lib.Import("deadair.zip", bytes.NewReader(withFile(inside))); !errors.Is(err, ErrNotRom) {
			t.Fatalf("%s inside: %v", inside, err)
		}
	}
	if lib.HasRom("deadair") {
		t.Fatal("a project was kept as a ROM")
	}
	if err := lib.Import("robby.zip", bytes.NewReader(withFile("robby.ic1"))); err != nil {
		t.Fatalf("a set: %v", err)
	}
	if errorCode(ErrNotRom) != "not_rom" || errorCode(ErrBadRom) != "" {
		t.Fatal("error codes")
	}
}
