// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package hdgames

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pkgBytes is a tiny game package (a zip with its manifest).
func pkgBytes(t *testing.T, title string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, body := range map[string]string{"manifest.json": `{"format": 3, "title": "` + title + `", "resolution": "720p", "levels": [{"level": "l.json"}]}`, "l.json": "{}"} {
		w, _ := z.Create(name)
		w.Write([]byte(body))
	}
	z.Close()
	return b.Bytes()
}

func game(url string, data []byte) Game {
	sum := sha256.Sum256(data)
	return Game{Name: "demo", Title: "Demo", Files: map[string]File{"720p": {URL: url, Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}}}
}

func TestDownloadChecksTheFile(t *testing.T) {
	good := pkgBytes(t, "Demo")
	serve := good
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(serve) }))
	defer srv.Close()
	dir := t.TempDir()
	g := game(srv.URL, good)
	var last int64
	path, err := Download(context.Background(), srv.Client(), g, "720p", dir, func(done, total int64) { last = done })
	if err != nil || path != filepath.Join(dir, "demo.glhd") || last != int64(len(good)) {
		t.Fatalf("download %q %v (progress %d)", path, err, last)
	}
	// A changed file is refused and the installed one stays.
	serve = pkgBytes(t, "Other")
	if _, err := Download(context.Background(), srv.Client(), g, "720p", dir, nil); !errors.Is(err, ErrChanged) {
		t.Fatalf("a changed file: %v", err)
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, good) {
		t.Fatal("the installed game was replaced by a bad download")
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".*")); len(left) != 0 {
		t.Fatalf("partial files left: %v", left)
	}
	if _, err := Download(context.Background(), srv.Client(), g, "1080p", dir, nil); err == nil {
		t.Fatal("a size the game does not have")
	}
}

func TestStarterGamesArePinned(t *testing.T) {
	for _, g := range Starter {
		for size, f := range g.Files {
			if f.Size <= 0 || len(f.SHA256) != 64 || f.URL == "" || !strings.Contains(f.URL, "-v"+g.Version+"/") {
				t.Errorf("%s %s: %+v", g.Name, size, f)
			}
		}
		if _, ok := Find(g.Name); !ok {
			t.Errorf("Find(%q)", g.Name)
		}
	}
}

func TestOlderVersions(t *testing.T) {
	for _, c := range []struct {
		installed, version string
		older              bool
	}{
		{"0.1.0", "0.1.1", true}, {"0.1.1", "0.1.1", false}, {"0.2.0", "0.1.1", false},
		{"0.9", "0.10", true}, {"1", "1.0.0", false}, {"", "0.1.1", true}, {"dev", "0.1.1", true}, {"0.1.0", "", false},
	} {
		if got := Older(c.installed, c.version); got != c.older {
			t.Errorf("Older(%q, %q) = %v", c.installed, c.version, got)
		}
	}
}
