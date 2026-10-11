// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
	"github.com/lordbasex/go-link-console/backend-device/internal/services"
)

func TestTheRecordingsAndTheFactoryResetWorkFromTheCLI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	config := filepath.Join(home, "config", "device.json")
	store, cfg, _, err := loadConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	identity := cfg.DeviceID
	cfg.SignalURL, cfg.UDPPort, cfg.Language = "wss://own.example/ws", 50000, "es"
	cfg.Links = []models.Link{{ID: "l1", TokenHash: "x"}}
	cfg.Rooms = []models.SavedRoom{{ID: "ab", Rom: "robby", Name: "Night"}}
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}

	// One finished recording and one saved game.
	recs, history, base, err := openRecordings()
	if err != nil {
		t.Fatal(err)
	}
	rec, err := recs.Start("ab", "Night", nil)
	if err != nil {
		t.Fatal(err)
	}
	rec.Video([]byte{0x10, 0, 0}, 64, 64)
	time.Sleep(20 * time.Millisecond)
	info, err := recs.Finish(rec, "ab", "Night", services.RecStopped)
	if err != nil {
		t.Fatal(err)
	}
	if err := history.Add(services.HistoryEntry{RoomID: "ab", Recordings: []services.RecordingInfo{info}}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "saves", "ab"), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := cmdRecList([]string{"--json"}); err != nil {
		t.Fatal(err)
	}
	if err := cmdRecRm(nil); err == nil {
		t.Fatal("rm without an id")
	}
	if err := cmdRecRm([]string{info.ID}); err != nil {
		t.Fatal(err)
	}
	if len(recs.List()) != 0 || len(services.NewHistoryService(filepath.Join(base, "history.json")).List()[0].Recordings) != 0 {
		t.Fatal("the recording is still listed")
	}

	if err := cmdReset([]string{"--config", config}); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("reset without --yes: %v", err)
	}
	if err := cmdReset([]string{"--config", config, "--yes"}); err != nil {
		t.Fatal(err)
	}
	_, after, _, err := loadConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	if after.DeviceID != identity || after.DeviceSecret != cfg.DeviceSecret || after.SignalURL != cfg.SignalURL || after.UDPPort != 50000 {
		t.Fatalf("the reset lost the device's identity or network: %+v", after)
	}
	if len(after.Links) != 0 || len(after.Rooms) != 0 || after.Language != "" {
		t.Fatalf("the reset kept the host's data: %+v", after)
	}
	if _, err := os.Stat(filepath.Join(base, "saves")); !os.IsNotExist(err) {
		t.Fatal("the saved games stayed")
	}
	if _, err := os.Stat(filepath.Join(base, "history.json")); !os.IsNotExist(err) {
		t.Fatal("the history stayed")
	}
}
