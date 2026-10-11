// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package glhd reads the manifest of a go-link HD game package (.glhd): a
// zip with manifest.json, a level and PNG pictures, played by go-link HD's
// engine (the golink-hd repository, whose README documents the format), run
// through its own API by package golinkhd. The device only reads what it
// shows and checks that the package is one the engine can open; the engine
// validates the rest when it loads the game.
package glhd

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Format is the newest package format the engine reads.
const Format = 3

// MaxBytes is the largest package the core loads.
const MaxBytes = 256 << 20

// LibraryFile is the engine library's file name on an OS: libgolinkhd.dylib,
// libgolinkhd.so or golinkhd.dll.
func LibraryFile(goos string) string {
	switch goos {
	case "darwin":
		return "libgolinkhd.dylib"
	case "windows":
		return "golinkhd.dll"
	}
	return "libgolinkhd.so"
}

// Ext is a package's file extension.
const Ext = ".glhd"

// Manifest is what the device shows of a game package.
type Manifest struct {
	Format  int    `json:"format"`
	Title   string `json:"title"`
	Version string `json:"version"`
	Genre   string `json:"genre"`
	Players int    `json:"players"`
	// Resolution is format 3's screen the pictures were painted for:
	// 360p (empty), 720p or 1080p.
	Resolution string `json:"resolution"`
	Level      string `json:"level"`
	// Levels is format 3's game of several levels, in place of Level.
	Levels []struct {
		Level string `json:"level"`
	} `json:"levels"`
	// Weapon is format 3's weapon: its button fires and its super's button
	// throws the super attack (the engine reads the rest).
	Weapon *struct {
		Button string `json:"button"`
		Super  *struct {
			Button string `json:"button"`
		} `json:"super"`
	} `json:"weapon"`
	// Screens are the package's own pictures: its title screen is the
	// game's picture in the library and the rooms.
	Screens struct {
		Title string `json:"title"`
	} `json:"screens"`
}

// Labels names the buttons of the built-in games (the platformer): buttons
// 1 and 2 (B and A) jump, 3 and 4 (Y and X) run.
var Labels = []string{"Jump", "Jump", "Run", "Run"}

// Labels names a package's buttons 1 to 4 (B, A, Y and X on a pad): they
// jump and run, unless its weapon fires ("Fire") or throws its super
// ("Special") with one of them. With a weapon the run button no longer runs.
func (m Manifest) Labels() []string {
	if m.Weapon == nil {
		return Labels
	}
	l := []string{"Jump", "Jump", "", ""}
	// the engine's button names: "run" is Y and X together, "b" and "a" also jump
	set := func(button, label string) {
		switch button {
		case "", "run":
			l[2], l[3] = label, label
		case "y":
			l[2] = label
		case "x":
			l[3] = label
		}
	}
	set(m.Weapon.Button, "Fire")
	if m.Weapon.Super != nil {
		b := m.Weapon.Super.Button
		if b == "" {
			b = "r" // the engine's default: a shoulder button, not one of these four
		}
		set(b, "Special")
	}
	// Y or X left with nothing to do (a weapon turns running off) shows what the other one does
	if l[2] == "" {
		l[2] = l[3]
	}
	if l[3] == "" {
		l[3] = l[2]
	}
	return l
}

// ErrNoPicture is a package without a title screen.
var ErrNoPicture = errors.New("the game package has no title screen")

// Picture returns a package's title screen (a PNG or JPEG of at most
// maxBytes), the picture the library and the rooms show for the game.
func Picture(path string, maxBytes int64) ([]byte, error) {
	m, err := Read(path)
	if err != nil {
		return nil, err
	}
	name := m.Screens.Title
	ext := strings.ToLower(filepath.Ext(name))
	if name == "" || (ext != ".png" && ext != ".jpg" && ext != ".jpeg") {
		return nil, ErrNoPicture
	}
	z, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	for _, f := range z.File {
		if f.Name != name {
			continue
		}
		if f.UncompressedSize64 > uint64(maxBytes) {
			return nil, fmt.Errorf("%s is bigger than %d bytes", name, maxBytes)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(io.LimitReader(rc, maxBytes))
	}
	return nil, ErrNoPicture
}

// Read opens a package and returns its manifest, or why the core cannot
// play it.
func Read(path string) (Manifest, error) {
	var m Manifest
	st, err := os.Stat(path)
	if err != nil {
		return m, err
	}
	if st.Size() > MaxBytes {
		return m, errors.New("the game package is bigger than 256 MB")
	}
	z, err := zip.OpenReader(path)
	if err != nil {
		return m, errors.New("the game package is not a valid zip")
	}
	defer z.Close()
	files := map[string]*zip.File{}
	for _, f := range z.File {
		files[f.Name] = f
	}
	f := files["manifest.json"]
	if f == nil {
		return m, errors.New("the game package has no manifest.json")
	}
	rc, err := f.Open()
	if err != nil {
		return m, err
	}
	data, err := io.ReadAll(io.LimitReader(rc, 1<<20))
	rc.Close()
	if err != nil {
		return m, fmt.Errorf("manifest.json: %w", err)
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("manifest.json: %w", err)
	}
	switch {
	case m.Format > Format:
		return m, errors.New("this game was made for a newer go-link HD: update go-link HD")
	case m.Format < 1:
		return m, errors.New("manifest.json has no format number")
	case strings.TrimSpace(m.Title) == "":
		return m, errors.New("manifest.json has no title")
	case len(m.Levels) == 0 && (m.Level == "" || files[m.Level] == nil):
		return m, errors.New("the game package has no level")
	}
	for _, l := range m.Levels {
		if l.Level == "" || files[l.Level] == nil {
			return m, errors.New("the game package is missing one of its levels")
		}
	}
	if m.Players < 1 || m.Players > 4 {
		m.Players = 4
	}
	return m, nil
}
