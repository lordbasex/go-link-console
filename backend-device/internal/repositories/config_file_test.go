// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package repositories

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
)

func TestFirstRunCreatesPersistentID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "device.json")
	f := NewConfigFile(path)

	cfg, created, err := f.LoadOrCreate()
	if err != nil || !created || cfg.DeviceID == "" {
		t.Fatalf("first run: %+v %v %v", cfg, created, err)
	}
	again, created, err := f.LoadOrCreate()
	if err != nil || created || again.DeviceID != cfg.DeviceID {
		t.Fatalf("second run: %+v %v %v", again, created, err)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(path)
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("mode %v", info.Mode().Perm())
		}
	}
}

func TestSaveRoundTrip(t *testing.T) {
	f := NewConfigFile(filepath.Join(t.TempDir(), "device.json"))
	cfg, _, _ := f.LoadOrCreate()
	cfg.RomsDir = "/roms"
	cfg.SignalURL = "wss://signal.example/ws"
	cfg.WebURL = "https://web.example"
	cfg.VideoQuality = models.VideoSaver
	now := time.Now().UTC().Truncate(time.Second)
	cfg.Links = []models.Link{{ID: "3f9c0a1b2c3d4e5f", TokenHash: "ab", CreatedAt: now, LastSeen: now}}
	if err := f.Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, _, err := f.LoadOrCreate()
	if err != nil || !reflect.DeepEqual(got, cfg) {
		t.Fatalf("got %+v %v", got, err)
	}
	if got.EffectiveSignalURL() != cfg.SignalURL {
		t.Fatalf("effective values: %+v", got)
	}
	raw, err := os.ReadFile(f.Path())
	if err != nil || !strings.Contains(string(raw), `"video_quality": "saver"`) {
		t.Fatalf("device.json lacks the video quality: %s %v", raw, err)
	}
}

func TestLooseModeIsTightened(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions")
	}
	path := filepath.Join(t.TempDir(), "device.json")
	f := NewConfigFile(path)
	_, _, _ = f.LoadOrCreate()
	_ = os.Chmod(path, 0o644)
	_, _, _ = f.LoadOrCreate()
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode().Perm())
	}
}

func TestCorruptConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "device.json")
	for _, content := range []string{"{not json", `{"device_id":"nope"}`} {
		_ = os.WriteFile(path, []byte(content), 0o600)
		if _, _, err := NewConfigFile(path).LoadOrCreate(); err == nil {
			t.Errorf("accepted %q", content)
		}
	}
}
