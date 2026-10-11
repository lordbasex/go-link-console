// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
	"github.com/lordbasex/go-link-console/backend-device/pkg/romcheck"
)

func testRoms() []models.RomInfo {
	ok := &romcheck.Result{Status: romcheck.StatusOK}
	missing := &romcheck.Result{Status: romcheck.StatusMissing, Missing: []string{"a.bin"}}
	return []models.RomInfo{ // sorted by name, as Scan leaves them
		{Name: "aliens", Title: "Aliens", Year: "1990", Maker: "Konami", Size: 300, Check: ok},
		{Name: "galaga", Title: "Galaga", Year: "1981", Maker: "Namco", Size: 50, Check: missing},
		{Name: "pacman", Title: "Pac-Man", Year: "1980", Maker: "Namco", Size: 40, Check: ok},
		{Name: "robby", Size: 10},
		{Name: "simpsons", Title: "The Simpsons", Year: "1991", Maker: "Konami", Size: 900, Check: ok},
	}
}

func names(roms []models.RomInfo) string {
	s := ""
	for _, r := range roms {
		s += r.Name + " "
	}
	return s
}

func TestRomIndexQuery(t *testing.T) {
	ix := newRomIndex(testRoms())
	for _, c := range []struct {
		q     RomQuery
		total int
		page  string
	}{
		{RomQuery{}, 5, "aliens galaga pacman robby simpsons "}, // by title: robby has none, so its name
		{RomQuery{Sort: "size"}, 5, "simpsons aliens galaga pacman robby "},
		{RomQuery{Sort: "year"}, 5, "simpsons aliens galaga pacman robby "},
		{RomQuery{Sort: "status"}, 5, "aliens pacman simpsons galaga robby "},
		{RomQuery{Q: "namco"}, 2, "galaga pacman "},
		{RomQuery{Q: "KONAMI simp"}, 1, "simpsons "}, // every word, any case
		{RomQuery{Filter: "runs"}, 3, "aliens pacman simpsons "},
		{RomQuery{Filter: "missing"}, 1, "galaga "},
		{RomQuery{Filter: "unchecked"}, 1, "robby "},
		{RomQuery{Filter: "playable"}, 4, "aliens pacman robby simpsons "}, // unchecked sets can be tried
		{RomQuery{Filter: "unplayable"}, 1, "galaga "},
		{RomQuery{Offset: 1, Limit: 2}, 5, "galaga pacman "},
		{RomQuery{Offset: 9}, 5, ""},
	} {
		total, page := ix.query(c.q)
		if total != c.total || names(page) != c.page {
			t.Errorf("%+v: %d %q, want %d %q", c.q, total, names(page), c.total, c.page)
		}
	}
	if got := names(ix.get([]string{"simpsons", "nope", "aliens"})); got != "simpsons aliens " {
		t.Errorf("get %q", got)
	}
	s := ix.summary(7)
	if s.Revision != 7 || s.Total != 5 || s.Bytes != 1300 || s.Playable != 4 || s.Kinds["runs"] != 3 || s.Kinds["missing"] != 1 || s.Kinds["unchecked"] != 1 || s.Kinds["bios"] != 0 {
		t.Errorf("summary %+v", s)
	}
	if len(s.Biggest) != 5 || s.Biggest[0].Name != "simpsons" || s.Biggest[1].Name != "aliens" {
		t.Errorf("biggest %+v", s.Biggest)
	}
}

// A full MAME collection is about 5,000 sets. device_status must stay far
// below what one WebRTC message carries (Chrome: 256 KiB), and a page must
// too, whatever the folder holds.
func TestLargeLibraryFitsMessages(t *testing.T) {
	var roms []models.RomInfo
	for i := range 6000 {
		roms = append(roms, models.RomInfo{
			Name: fmt.Sprintf("set%05d", i), Title: fmt.Sprintf("A long game title number %d (World, revision B)", i),
			Year: "1994", Maker: "Some Company", Size: int64(1000 + i),
			Check: &romcheck.Result{Status: romcheck.StatusMissing, Missing: []string{"file1.bin", "file2.bin", "file3.bin"}},
		})
	}
	ix := newRomIndex(roms)
	status := NewDeviceStatusMessage(models.Status{Library: &models.Library{Dir: "/roms", Roms: roms, Summary: ix.summary(1)}})
	b, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > 16<<10 {
		t.Errorf("device_status is %d bytes with 6000 sets", len(b))
	}
	total, page := ix.query(RomQuery{Limit: 1000})
	if total != 6000 || len(page) != RomsPageMax {
		t.Fatalf("page of %d of %d", len(page), total)
	}
	if b, _ := json.Marshal(map[string]any{"type": "roms_page", "roms": page}); len(b) > 64<<10 {
		t.Errorf("a page is %d bytes", len(b))
	}
}

func TestScanFillsSummaryAndPages(t *testing.T) {
	st := NewStatusService(deviceID, "test", "ws://x", "")
	dir := t.TempDir()
	for _, n := range []string{"robby", "pacman", "aliens"} {
		writeTestZip(t, filepath.Join(dir, n+".zip"), n+".bin", "data")
	}
	lib := NewLibraryService(dir, st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	lib.Scan()
	got := st.Snapshot().Library
	if got.Summary.Total != 3 || got.Summary.Revision == 0 {
		t.Fatalf("summary %+v", got.Summary)
	}
	rev, total, page := lib.QueryRoms(RomQuery{Q: "pac"})
	if rev != got.Summary.Revision || total != 1 || page[0].Name != "pacman" {
		t.Fatalf("query: %d %d %+v", rev, total, page)
	}
	lib.Scan()
	if st.Snapshot().Library.Summary.Revision == rev {
		t.Fatal("a scan must change the revision")
	}
}
