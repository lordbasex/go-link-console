// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/lordbasex/go-link-console/backend-device/pkg/romcheck"
)

// MakerRom names the game Willy Maker sent from the website (upload
// purpose "maker"): one game at a time, kept in its own folder apart from
// the host's ROMs, so a set of the same name in the ROM folder (Willy
// Maker games are laid out as slammast) is never touched. A room plays it
// like any set.
const MakerRom = "@maker"

// makerSet is the file name the core loads Willy Maker games by.
const makerSet = "slammast"

// MakerInfo is what the website tells about the game it sent: its name and
// its buttons (the genre names them), for the room's title and controls.
type MakerInfo struct {
	Title   string   `json:"title"`
	Players int      `json:"players,omitempty"`
	Labels  []string `json:"labels,omitempty"`
}

// ErrNoMakerGame is returned when no Willy Maker game was sent yet.
var ErrNoMakerGame = errors.New("no Willy Maker game on this device yet: send one from Willy Maker first")

// SetMakerDir sets the folder of the Willy Maker game.
func (l *LibraryService) SetMakerDir(dir string) {
	l.mu.Lock()
	l.makerDir = dir
	l.mu.Unlock()
}

func (l *LibraryService) makerFolder() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.makerDir
}

func (l *LibraryService) makerPath() string {
	return filepath.Join(l.makerFolder(), makerSet+".zip")
}

// CheckMakerUpload refuses a Willy Maker game before its bytes arrive.
func (l *LibraryService) CheckMakerUpload(name string, size int64) error {
	if l.makerFolder() == "" {
		return errors.New("this device cannot keep Willy Maker games")
	}
	if name != makerSet+".zip" {
		return fmt.Errorf("a Willy Maker game is %s.zip, not %s", makerSet, name)
	}
	if size <= 0 || size > RomTestMaxSize {
		return ErrBadRom
	}
	return nil
}

// StoreMaker keeps a Willy Maker game, replacing the one before (its own
// folder holds nothing else). The file must be a zip.
func (l *LibraryService) StoreMaker(r io.Reader) error {
	dir := l.makerFolder()
	if dir == "" {
		return errors.New("this device cannot keep Willy Maker games")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	n, err := io.Copy(tmp, io.LimitReader(r, RomTestMaxSize+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if n > RomTestMaxSize {
		return ErrBadRom
	}
	head := make([]byte, 4)
	f, err := os.Open(tmp.Name())
	if err != nil {
		return err
	}
	_, err = io.ReadFull(f, head)
	f.Close()
	if err != nil || string(head) != "PK\x03\x04" {
		return ErrBadRom
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), l.makerPath())
}

// SetMakerInfo keeps the title and buttons of the Willy Maker game.
func (l *LibraryService) SetMakerInfo(info MakerInfo) error {
	dir := l.makerFolder()
	if dir == "" {
		return ErrNoMakerGame
	}
	info.Title = cleanText(info.Title, 60)
	if info.Players < 1 || info.Players > 4 {
		info.Players = 4
	}
	if len(info.Labels) > 3 {
		info.Labels = info.Labels[:3]
	}
	for i, s := range info.Labels {
		info.Labels[i] = cleanText(s, 16)
	}
	b, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "game.json"), b, 0o644)
}

// Maker returns what is known of the Willy Maker game.
func (l *LibraryService) Maker() MakerInfo {
	info := MakerInfo{Title: "Willy Maker", Players: 4}
	dir := l.makerFolder()
	if dir == "" {
		return info
	}
	if b, err := os.ReadFile(filepath.Join(dir, "game.json")); err == nil {
		var saved MakerInfo
		if json.Unmarshal(b, &saved) == nil {
			if strings.TrimSpace(saved.Title) != "" {
				info.Title = saved.Title
			}
			if saved.Players >= 1 && saved.Players <= 4 {
				info.Players = saved.Players
			}
			info.Labels = saved.Labels
		}
	}
	return info
}

// checkMaker runs the set check on the Willy Maker game: the slammast
// file names, in its own folder.
func (l *LibraryService) checkMaker() (romcheck.Result, bool) {
	cat := l.Catalog()
	if cat == nil {
		return romcheck.Result{}, false
	}
	return romcheck.NewChecker(cat, l.makerFolder()).Check(makerSet), true
}
