// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package glhd

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePackage(t *testing.T, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "game.glhd")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for name, body := range files {
		w, _ := z.Create(name)
		w.Write([]byte(body))
	}
	z.Close()
	f.Close()
	return path
}

func TestRead(t *testing.T) {
	ok := writePackage(t, map[string]string{
		"manifest.json": `{"format": 1, "title": "Neon Run", "players": 2, "level": "level.json", "extra": {"later": true}}`,
		"level.json":    `{}`,
	})
	m, err := Read(ok)
	if err != nil || m.Title != "Neon Run" || m.Players != 2 {
		t.Fatalf("got %+v, %v", m, err)
	}
	// format 3's levels, in place of one level
	several := writePackage(t, map[string]string{
		"manifest.json": `{"format": 3, "title": "Body", "levels": [{"level": "a.json"}, {"level": "b.json"}]}`,
		"a.json":        `{}`,
		"b.json":        `{}`,
	})
	if m, err := Read(several); err != nil || len(m.Levels) != 2 {
		t.Fatalf("several levels: got %+v, %v", m, err)
	}
	// a game coded in the engine has no levels
	coded := writePackage(t, map[string]string{"manifest.json": `{"format": 3, "title": "Remote", "game": "remote", "players": 1}`})
	if m, err := Read(coded); err != nil || m.Game != "remote" || m.Players != 1 {
		t.Fatalf("coded game: got %+v, %v", m, err)
	}
	for name, c := range map[string]struct {
		files map[string]string
		want  string
	}{
		"newer":           {map[string]string{"manifest.json": `{"format": 9, "title": "x", "level": "l"}`, "l": ""}, "newer go-link HD"},
		"no title":        {map[string]string{"manifest.json": `{"format": 1, "level": "l"}`, "l": ""}, "no title"},
		"no level":        {map[string]string{"manifest.json": `{"format": 1, "title": "x", "level": "l"}`}, "no level"},
		"none":            {map[string]string{"level.json": ""}, "no manifest.json"},
		"a level missing": {map[string]string{"manifest.json": `{"format": 3, "title": "x", "levels": [{"level": "a"}, {"level": "b"}]}`, "a": ""}, "missing one of its levels"},
	} {
		if _, err := Read(writePackage(t, c.files)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", name, err, c.want)
		}
	}
	bad := filepath.Join(t.TempDir(), "bad.glhd")
	os.WriteFile(bad, []byte("not a zip"), 0o644)
	if _, err := Read(bad); err == nil {
		t.Error("a file that is not a zip was read")
	}
}

// A package's buttons: jump and run without a weapon; a weapon's fire and super buttons name theirs.
func TestLabels(t *testing.T) {
	for name, c := range map[string]struct {
		manifest string
		want     string
	}{
		"no weapon":      {`{}`, "Jump Jump Run Run"},
		"fire with run":  {`{"weapon": {}}`, "Jump Jump Fire Fire"},
		"fire and super": {`{"weapon": {"button": "x", "super": {"button": "y"}}}`, "Jump Jump Special Fire"},
		"super on R":     {`{"weapon": {"button": "x", "super": {}}}`, "Jump Jump Fire Fire"},
		"fire on Y":      {`{"weapon": {"button": "y"}}`, "Jump Jump Fire Fire"},
		"coded game":     {`{"game": "remote"}`, "Use Use Run Run"},
	} {
		var m Manifest
		if err := json.Unmarshal([]byte(c.manifest), &m); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(m.Labels(), " "); got != c.want {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
}

func TestPicture(t *testing.T) {
	with := writePackage(t, map[string]string{
		"manifest.json": `{"format": 3, "title": "Body", "level": "l", "screens": {"title": "start.png"}}`,
		"l":             "",
		"start.png":     "picture",
	})
	if b, err := Picture(with, 1<<20); err != nil || string(b) != "picture" {
		t.Fatalf("got %q, %v", b, err)
	}
	if _, err := Picture(with, 3); err == nil {
		t.Error("a picture bigger than the limit was read")
	}
	without := writePackage(t, map[string]string{"manifest.json": `{"format": 3, "title": "Body", "level": "l"}`, "l": ""})
	missing := writePackage(t, map[string]string{"manifest.json": `{"format": 3, "title": "Body", "level": "l", "screens": {"title": "start.png"}}`, "l": ""})
	notImage := writePackage(t, map[string]string{"manifest.json": `{"format": 3, "title": "Body", "level": "l", "screens": {"title": "l"}}`, "l": ""})
	for name, p := range map[string]string{"without": without, "missing": missing, "not an image": notImage} {
		if _, err := Picture(p, 1<<20); err != ErrNoPicture {
			t.Errorf("%s: got %v, want ErrNoPicture", name, err)
		}
	}
}
