// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"
)

type file struct {
	name, body string
}

func tarGz(t *testing.T, files ...file) string {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range files {
		_ = tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o755, Size: int64(len(f.body)), Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte(f.body))
	}
	_ = tw.Close()
	_ = gz.Close()
	path := filepath.Join(t.TempDir(), "release.tar.gz")
	_ = os.WriteFile(path, buf.Bytes(), 0o600)
	return path
}

func zipOf(t *testing.T, files ...file) string {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, _ := zw.Create(f.name)
		_, _ = w.Write([]byte(f.body))
	}
	_ = zw.Close()
	path := filepath.Join(t.TempDir(), "release.zip")
	_ = os.WriteFile(path, buf.Bytes(), 0o600)
	return path
}

// folder is a copy unpacked from a release: the program, the engine and a
// file of the person's that is not in releases.
func folder(t *testing.T) (dir string, p *folderPlan, restarted *string) {
	dir = t.TempDir()
	for name, body := range map[string]string{"go-link-console": "old exe", "libgolinkhd.so": "old lib", "LICENSE": "old license", "notes.txt": "mine"} {
		_ = os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755)
	}
	restarted = new(string)
	pl, err := folderOf(filepath.Join(dir, "go-link-console"), "go-link-console", "libgolinkhd.so", "go-link-console-%s-linux-amd64.tar.gz",
		func(exe string) func() error { return func() error { *restarted = exe; return nil } })
	if err != nil {
		t.Fatal(err)
	}
	return dir, pl.(*folderPlan), restarted
}

func read(t *testing.T, path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "(none)"
	}
	return string(b)
}

func TestAFolderTakesTheReleasesFiles(t *testing.T) {
	for _, kind := range []string{"tar.gz", "zip"} {
		t.Run(kind, func(t *testing.T) {
			dir, p, restarted := folder(t)
			files := []file{{"go-link-console", "new exe"}, {"libgolinkhd.so", "new lib"}, {"LICENSE", "new license"}, {"THIRD_PARTY_NOTICES.md", "notices"}}
			archive := tarGz(t, files...)
			if kind == "zip" {
				archive = zipOf(t, files...)
			}
			restart, err := p.install(context.Background(), archive)
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range files {
				if got := read(t, filepath.Join(dir, f.name)); got != f.body {
					t.Errorf("%s = %q", f.name, got)
				}
			}
			if got := read(t, filepath.Join(dir, "notes.txt")); got != "mine" {
				t.Errorf("a file of the person's changed: %q", got)
			}
			left, _ := filepath.Glob(filepath.Join(dir, "*"))
			if len(left) != 5 {
				t.Errorf("the folder has %v", left)
			}
			if _ = restart(); *restarted != p.exe {
				t.Errorf("restarted %q", *restarted)
			}
		})
	}
}

func TestABadArchiveChangesNothing(t *testing.T) {
	cases := map[string][]file{
		"no program":  {{"libgolinkhd.so", "new lib"}},
		"no engine":   {{"go-link-console", "new exe"}},
		"leaves it":   {{"go-link-console", "new exe"}, {"libgolinkhd.so", "new lib"}, {"../evil", "x"}},
		"in a folder": {{"go-link-console", "new exe"}, {"libgolinkhd.so", "new lib"}, {"sub/x", "x"}},
		"an old file": {{"go-link-console", "new exe"}, {"libgolinkhd.so", "new lib"}, {"x.old", "x"}},
	}
	for what, files := range cases {
		t.Run(what, func(t *testing.T) {
			dir, p, restarted := folder(t)
			if _, err := p.install(context.Background(), tarGz(t, files...)); err == nil {
				t.Fatal("installed")
			}
			if read(t, filepath.Join(dir, "go-link-console")) != "old exe" || read(t, filepath.Join(dir, "libgolinkhd.so")) != "old lib" {
				t.Fatal("the program changed")
			}
			if left, _ := filepath.Glob(filepath.Join(dir, "*")); len(left) != 4 {
				t.Errorf("the folder has %v", left)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "evil")); err == nil {
				t.Error("a file was written outside the folder")
			}
			if *restarted != "" {
				t.Error("restarted")
			}
		})
	}
}
