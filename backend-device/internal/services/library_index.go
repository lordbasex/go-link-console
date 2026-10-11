// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
	"github.com/lordbasex/go-link-console/backend-device/pkg/romcheck"
)

// A ROM folder can hold thousands of sets (a full MAME 0.78 collection is
// about 5,000), far more than one WebRTC message carries (Chrome takes at
// most 256 KiB). So device_status carries only the library's summary, and
// browsers ask for pages of sets with roms_query, or for some sets by name
// with roms_get. Both are answered from this index, built once per scan:
// the four orders are sorted then, so a page is one pass over a slice.

// RomsPageMax is the most sets one roms_query or roms_get reply carries.
const RomsPageMax = 100

// RomQuery asks for one page of the library.
type RomQuery struct {
	Q      string // words to find in the set name, title or maker
	Filter string // all, playable, unplayable, or a kind (models.RomKinds)
	Sort   string // name (title A-Z), size and year (largest and newest first), status
	Offset int
	Limit  int
}

type indexedRom struct {
	rom  *models.RomInfo
	hay  string // lower case name, title and maker, for the search
	kind string
}

type romIndex struct {
	roms   []indexedRom
	orders map[string][]int // sort name -> positions in roms
}

// RomKind is the state of a set, as the website groups it.
func RomKind(r *models.RomInfo) string {
	if r.Check == nil {
		return models.RomUnchecked
	}
	switch r.Check.Status {
	case romcheck.StatusOK:
		return models.RomRuns
	case romcheck.StatusMissing:
		return models.RomMissing
	case romcheck.StatusUnsupported:
		return models.RomUnsupported
	case romcheck.StatusBIOS:
		return models.RomBIOS
	default:
		return models.RomBroken
	}
}

// RomPlayable tells whether a room can be opened with the set: the core's
// check passed, or there is no check yet.
func RomPlayable(r *models.RomInfo) bool {
	return r.Check == nil || r.Check.Status == romcheck.StatusOK
}

func title(r *models.RomInfo) string {
	if r.Title != "" {
		return r.Title
	}
	return r.Name
}

func newRomIndex(roms []models.RomInfo) *romIndex {
	ix := &romIndex{roms: make([]indexedRom, len(roms)), orders: map[string][]int{}}
	for i := range roms {
		r := &roms[i]
		ix.roms[i] = indexedRom{rom: r, kind: RomKind(r), hay: strings.ToLower(r.Name + " " + r.Title + " " + r.Maker)}
	}
	byTitle := func(a, b int) int {
		ta, tb := strings.ToLower(title(ix.roms[a].rom)), strings.ToLower(title(ix.roms[b].rom))
		return cmp.Or(strings.Compare(ta, tb), strings.Compare(ix.roms[a].rom.Name, ix.roms[b].rom.Name))
	}
	year := func(i int) int {
		y, _ := strconv.Atoi(ix.roms[i].rom.Year) // "199?" and empty sort last
		return y
	}
	kindPos := func(i int) int { return slices.Index(models.RomKinds, ix.roms[i].kind) }
	sorts := map[string]func(a, b int) int{
		"name": byTitle,
		"size": func(a, b int) int {
			return cmp.Or(cmp.Compare(ix.roms[b].rom.Size, ix.roms[a].rom.Size), byTitle(a, b))
		},
		"year":   func(a, b int) int { return cmp.Or(cmp.Compare(year(b), year(a)), byTitle(a, b)) },
		"status": func(a, b int) int { return cmp.Or(cmp.Compare(kindPos(a), kindPos(b)), byTitle(a, b)) },
	}
	for name, f := range sorts {
		order := make([]int, len(roms))
		for i := range order {
			order[i] = i
		}
		slices.SortFunc(order, f)
		ix.orders[name] = order
	}
	return ix
}

// query returns the number of sets that match and the asked page of them.
func (ix *romIndex) query(q RomQuery) (int, []models.RomInfo) {
	order := ix.orders[q.Sort]
	if order == nil {
		order = ix.orders["name"]
	}
	limit := q.Limit
	if limit <= 0 || limit > RomsPageMax {
		limit = RomsPageMax
	}
	offset := max(q.Offset, 0)
	words := strings.Fields(strings.ToLower(q.Q))
	page := []models.RomInfo{}
	total := 0
	for _, i := range order {
		r := &ix.roms[i]
		if !matchFilter(r, q.Filter) || !matchWords(r.hay, words) {
			continue
		}
		if total >= offset && len(page) < limit {
			page = append(page, *r.rom)
		}
		total++
	}
	return total, page
}

func matchFilter(r *indexedRom, filter string) bool {
	switch filter {
	case "", "all":
		return true
	case "playable":
		return RomPlayable(r.rom)
	case "unplayable":
		return !RomPlayable(r.rom)
	}
	return r.kind == filter
}

func matchWords(hay string, words []string) bool {
	for _, w := range words {
		if !strings.Contains(hay, w) {
			return false
		}
	}
	return true
}

// summary counts the library for device_status.
func (ix *romIndex) summary(revision int64) models.LibrarySummary {
	s := models.LibrarySummary{Revision: revision, Kinds: map[string]int{}, Biggest: []models.RomBrief{}}
	for _, k := range models.RomKinds {
		s.Kinds[k] = 0
	}
	for _, r := range ix.roms {
		s.Total++
		s.Bytes += r.rom.Size
		s.Kinds[r.kind]++
		if RomPlayable(r.rom) {
			s.Playable++
		}
		th := r.rom.Thumbs
		s.Thumbs.Boxart += b2i(th.Boxart)
		s.Thumbs.Title += b2i(th.Title)
		s.Thumbs.Snap += b2i(th.Snap)
	}
	for _, i := range ix.orders["size"] {
		if len(s.Biggest) == 5 {
			break
		}
		r := ix.roms[i].rom
		s.Biggest = append(s.Biggest, models.RomBrief{Name: r.Name, Title: r.Title, Size: r.Size})
	}
	return s
}

// get returns the named sets that are in the library, in the asked order.
func (ix *romIndex) get(names []string) []models.RomInfo {
	out := []models.RomInfo{}
	for _, n := range names {
		if len(out) == RomsPageMax {
			break
		}
		// roms are sorted by name (Scan), so a binary search finds each
		if i, ok := slices.BinarySearchFunc(ix.roms, n, func(r indexedRom, n string) int { return strings.Compare(r.rom.Name, n) }); ok {
			out = append(out, *ix.roms[i].rom)
		}
	}
	return out
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
