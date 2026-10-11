// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
)

func TestMigrateLegacy(t *testing.T) {
	root := t.TempDir()
	old, cur := filepath.Join(root, legacyDataDir), filepath.Join(root, "go-link-console")
	os.MkdirAll(filepath.Join(old, "cores"), 0o755)
	os.WriteFile(filepath.Join(old, "cores", "core.dylib"), []byte("x"), 0o644)
	if !migrateLegacy(old, cur) {
		t.Fatal("the old folder must move")
	}
	if _, err := os.Stat(filepath.Join(cur, "cores", "core.dylib")); err != nil {
		t.Fatal(err)
	}
	// Once the new folder exists, nothing moves again.
	os.MkdirAll(old, 0o755)
	if migrateLegacy(old, cur) {
		t.Fatal("an existing new folder is never replaced")
	}
}

func TestMovePaths(t *testing.T) {
	old, base := filepath.Join("/home", "go-link"), filepath.Join("/home", "go-link-console")
	c := models.Config{RomsDir: filepath.Join(old, "roms"), HD: models.HDSettings{GamesDir: filepath.Join(old, "hd-games")}}
	c.Thumbnails.Dir = filepath.Join("/elsewhere", "thumbs")
	if !movePaths(&c, old, base) {
		t.Fatal("folders moved")
	}
	if c.RomsDir != filepath.Join(base, "roms") || c.HD.GamesDir != filepath.Join(base, "hd-games") {
		t.Fatalf("folders inside the old one move: %q %q", c.RomsDir, c.HD.GamesDir)
	}
	if c.Thumbnails.Dir != filepath.Join("/elsewhere", "thumbs") {
		t.Fatalf("a folder elsewhere stays: %q", c.Thumbnails.Dir)
	}
	if movePaths(&c, old, base) {
		t.Fatal("nothing moves twice")
	}
}
