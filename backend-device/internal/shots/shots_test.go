// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package shots

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lordbasex/go-link-console/backend-device/pkg/romcheck"
)

func TestSeedMakesTheInventedLibrary(t *testing.T) {
	base := t.TempDir()
	if err := Seed(base, false); err != nil {
		t.Fatal(err)
	}
	cat, err := romcheck.Load(filepath.Join(base, "cores", romcheck.FileName))
	if err != nil {
		t.Fatal(err)
	}
	checker := romcheck.NewChecker(cat, filepath.Join(base, "roms"))
	for _, g := range Games {
		want := romcheck.StatusOK
		if g.Missing {
			want = romcheck.StatusMissing
		}
		if got := checker.Check(g.Set).Status; got != want {
			t.Errorf("%s: %s, want %s", g.Set, got, want)
		}
		if cat.Game(g.Set).Title != g.Title {
			t.Errorf("%s: title %q", g.Set, cat.Game(g.Set).Title)
		}
		if _, err := os.Stat(filepath.Join(base, "thumbnails", "MAME", "Named_Boxarts", g.Set+".png")); err != nil {
			t.Errorf("%s: no cover: %v", g.Set, err)
		}
	}
	if _, err := os.Stat(filepath.Join(base, "cores")); err != nil {
		t.Fatal(err)
	}
}
