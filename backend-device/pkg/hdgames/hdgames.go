// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package hdgames lists the free starter games of go-link HD and downloads
// them. Each game is a package (.glhd) published with go-link HD's engine,
// in a screen size per room size; a download is kept only when its size
// and SHA-256 match the ones written here, so a changed file is refused.
package hdgames

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/pkg/glhd"
)

// File is one screen size of a game.
type File struct {
	URL    string
	Size   int64
	SHA256 string
}

// Game is a starter game: its package name (name.glhd), title, version
// (its manifest's) and files by screen size (720p, 1080p).
type Game struct {
	Name    string
	Title   string
	Version string
	Files   map[string]File
}

// release is where go-link HD's engine publishes the starter games.
const release = "https://github.com/lordbasex/golink-hd/releases/download/antidoto-v0.1.1/"

// Starter are the games a device downloads on its first link, and that
// go-link HD's page can download again.
var Starter = []Game{{
	Name:    "antidoto",
	Title:   "ANTÍDOTO",
	Version: "0.1.1",
	Files: map[string]File{
		"720p": {URL: release + "antidoto-720p.glhd", Size: 117426392,
			SHA256: "aab91aff3d6960575867805cb84f41b9ad3e116165c4a24d2dc114adb3eb3afc"},
		"1080p": {URL: release + "antidoto-1080p.glhd", Size: 173130796,
			SHA256: "fe97b1be1a19c9a6787567c500cfe7b4f72183a799ed5ab018070c186451fa2b"},
	},
}}

// Older reports whether an installed package's version (its manifest's,
// "0.1.0") comes before the game's: a newer one can be downloaded. An
// unknown version counts as older, an equal or newer one does not.
func Older(installed, version string) bool {
	if version == "" {
		return false
	}
	a, b := strings.Split(installed, "."), strings.Split(version, ".")
	for i := 0; i < max(len(a), len(b)); i++ {
		x, y := 0, 0
		if i < len(a) {
			n, err := strconv.Atoi(a[i])
			if err != nil {
				return true
			}
			x = n
		}
		if i < len(b) {
			y, _ = strconv.Atoi(b[i])
		}
		if x != y {
			return x < y
		}
	}
	return false
}

// Find returns the starter game of a name.
func Find(name string) (Game, bool) {
	for _, g := range Starter {
		if g.Name == name {
			return g, true
		}
	}
	return Game{}, false
}

// ErrChanged is a download whose size or SHA-256 is not the expected one.
var ErrChanged = errors.New("hdgames: the downloaded game is not the published one")

// Download fetches one screen size of a game into dir as name.glhd,
// replacing the one there only once the new file is complete and checked.
// progress (may be nil) is called with the bytes received so far.
func Download(ctx context.Context, client *http.Client, g Game, size, dir string, progress func(done, total int64)) (string, error) {
	f, ok := g.Files[size]
	if !ok {
		return "", fmt.Errorf("hdgames: %s has no %s package", g.Name, size)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "go-link-console")
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Minute}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("hdgames: %s: HTTP %d", f.URL, resp.StatusCode)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, "."+g.Name+"-*.part")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	hash := sha256.New()
	w := io.MultiWriter(tmp, hash)
	var done int64
	buf := make([]byte, 256<<10)
	// never more than the published size: a bigger answer is not the game
	body := io.LimitReader(resp.Body, f.Size+1)
	for {
		n, rerr := body.Read(buf)
		if n > 0 {
			if _, err := w.Write(buf[:n]); err != nil {
				tmp.Close()
				return "", err
			}
			done += int64(n)
			if progress != nil {
				progress(done, f.Size)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			tmp.Close()
			return "", rerr
		}
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if done != f.Size || hex.EncodeToString(hash.Sum(nil)) != f.SHA256 {
		return "", ErrChanged
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return "", err
	}
	if _, err := glhd.Read(tmp.Name()); err != nil {
		return "", fmt.Errorf("hdgames: %w", err)
	}
	dst := filepath.Join(dir, g.Name+glhd.Ext)
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return "", err
	}
	return dst, nil
}
