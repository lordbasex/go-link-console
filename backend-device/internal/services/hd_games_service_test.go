// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lordbasex/go-link/backend-device/internal/models"
	"github.com/lordbasex/go-link/backend-device/pkg/hdgames"
)

func TestTheHDFolderListsItsGamesFirst(t *testing.T) {
	roms, hd := t.TempDir(), t.TempDir()
	writeGLHD(t, filepath.Join(roms, "neon.glhd"), `{"format": 1, "title": "Old Neon", "level": "level.json"}`)
	writeGLHD(t, filepath.Join(roms, "kept.glhd"), `{"format": 1, "title": "Kept", "level": "level.json"}`)
	writeGLHD(t, filepath.Join(hd, "neon.glhd"), `{"format": 1, "title": "Neon Run", "level": "level.json"}`)
	st := NewStatusService(deviceID, "test", "ws://x", "")
	lib := NewLibraryService(roms, st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	lib.SetCore(t.TempDir(), "")
	lib.UseHDDir(hd)
	lib.Scan()
	got := st.Snapshot().Library
	titles := map[string]string{}
	for _, r := range got.Roms {
		titles[r.Name] = r.Title
	}
	if titles["neon"] != "Neon Run" || titles["kept"] != "Kept" || got.HDDir != hd || got.HDGames != 1 {
		t.Fatalf("titles %v, hd %q %d", titles, got.HDDir, got.HDGames)
	}
	if lib.RomPath("neon") != filepath.Join(hd, "neon.glhd") || lib.RomPath("kept") != filepath.Join(roms, "kept.glhd") {
		t.Fatalf("paths %q %q", lib.RomPath("neon"), lib.RomPath("kept"))
	}
	var saved string
	lib.OnHDDirChange(func(d string) { saved = d })
	other := t.TempDir()
	if err := lib.SetHDDir(other); err != nil || saved != other || lib.HDDir() != other {
		t.Fatalf("SetHDDir: %v, saved %q", err, saved)
	}
	if err := lib.SetHDDir("relative"); err == nil {
		t.Fatal("a relative folder")
	}
}

// starterGame serves a tiny package as a starter game.
func starterGame(t *testing.T) (hdgames.Game, *atomic.Int32) {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, body := range map[string]string{"manifest.json": `{"format": 3, "title": "Demo", "resolution": "720p", "levels": [{"level": "l.json"}]}`, "l.json": "{}"} {
		w, _ := z.Create(name)
		w.Write([]byte(body))
	}
	z.Close()
	data := b.Bytes()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write(data)
	}))
	t.Cleanup(srv.Close)
	sum := sha256.Sum256(data)
	f := hdgames.File{URL: srv.URL, Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}
	return hdgames.Game{Name: "demo", Title: "Demo", Files: map[string]hdgames.File{"720p": f, "1080p": f}}, &hits
}

func TestStarterGamesAreOfferedOnce(t *testing.T) {
	g, hits := starterGame(t)
	roms, hd := t.TempDir(), t.TempDir()
	st := NewStatusService(deviceID, "test", "ws://x", "")
	lib := NewLibraryService(roms, st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	lib.SetCore(t.TempDir(), "")
	engine := filepath.Join(t.TempDir(), "libgolinkhd")
	_ = os.WriteFile(engine, []byte("engine"), 0o644)
	lib.SetHDCore(engine)
	lib.UseHDDir(hd)
	settings := NewSettingsService(lib, models.ThumbnailSettings{}, t.TempDir(), nil)
	var mu sync.Mutex
	var rooms []string
	saved := make(chan struct{}, 2)
	h := NewHDGamesService(HDGamesConfig{
		Library: lib, Status: st, Settings: settings, Games: []hdgames.Game{g},
		SaveOffered: func() error { saved <- struct{}{}; return nil },
		OpenRoom: func(rom string) error {
			mu.Lock()
			rooms = append(rooms, rom)
			mu.Unlock()
			return nil
		},
	})
	h.OfferStarter(context.Background())
	select {
	case <-saved:
	case <-time.After(5 * time.Second):
		t.Fatal("the starter games were not offered")
	}
	h.OfferStarter(context.Background()) // once only
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	n := len(rooms)
	mu.Unlock()
	if hits.Load() != 1 || n != 1 || rooms[0] != "demo" || lib.HDPackage("demo") != filepath.Join(hd, "demo.glhd") {
		t.Fatalf("hits %d, rooms %v, package %q", hits.Load(), rooms, lib.HDPackage("demo"))
	}
	sg := st.Snapshot().HD.Starter[0]
	if !sg.Installed || sg.Size != "720p" || sg.Downloading != "" {
		t.Fatalf("status %+v", sg)
	}
	// A factory reset offers them again: the game is there, so only its room.
	h.ResetOffered()
	h.OfferStarter(context.Background())
	select {
	case <-saved:
	case <-time.After(5 * time.Second):
		t.Fatal("not offered after a reset")
	}
	mu.Lock()
	defer mu.Unlock()
	if hits.Load() != 1 || len(rooms) != 2 {
		t.Fatalf("after a reset: hits %d, rooms %v", hits.Load(), rooms)
	}
}

func TestStarterGamesWaitForTheEngine(t *testing.T) {
	g, hits := starterGame(t)
	st := NewStatusService(deviceID, "test", "ws://x", "")
	lib := NewLibraryService(t.TempDir(), st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	lib.SetCore(t.TempDir(), "")
	lib.SetHDCore(filepath.Join(t.TempDir(), "missing"))
	lib.UseHDDir(t.TempDir())
	settings := NewSettingsService(lib, models.ThumbnailSettings{}, t.TempDir(), nil)
	h := NewHDGamesService(HDGamesConfig{Library: lib, Status: st, Settings: settings, Games: []hdgames.Game{g}})
	if h.offer(context.Background()) || hits.Load() != 0 {
		t.Fatal("offered without the engine")
	}
	// The host can still download one by hand, in either size.
	if err := h.DownloadWait(context.Background(), "demo", "1080p"); err != nil {
		t.Fatal(err)
	}
	if err := h.DownloadWait(context.Background(), "nope", "720p"); err != ErrUnknownGame {
		t.Fatalf("unknown game: %v", err)
	}
	if err := h.DownloadWait(context.Background(), "demo", "4k"); err != ErrBadSetting {
		t.Fatalf("unknown size: %v", err)
	}
}

func TestSettingsKeepTheHDScreen(t *testing.T) {
	s := NewSettingsService(nil, models.ThumbnailSettings{}, "", nil)
	var saved models.HDSettings
	s.UseHD(models.HDSettings{RoomSize: "4k"}, func(h models.HDSettings) error { saved = h; return nil })
	if got := s.HD(); got.RoomSize != "720p" || got.Codec != "auto" {
		t.Fatalf("defaults %+v", got)
	}
	if err := s.SetHDScreen("1080p", "vp8"); err != nil || saved.RoomSize != "1080p" || saved.Codec != "vp8" {
		t.Fatalf("set: %v %+v", err, saved)
	}
	if err := s.SetHDScreen("2160p", "auto"); err != ErrBadSetting {
		t.Fatalf("a size it does not have: %v", err)
	}
}
