// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"

	"github.com/lordbasex/go-link/backend-device/internal/models"
)

// The tabs of the MAME page.
const (
	mameTabRoms = iota
	mameTabThumbs
)

// mamePage is the MAME emulator, under Core in the sidebar: a header
// with the core's state and two tabs, the ROM sets and their thumbnails.
type mamePage struct {
	u        *ui
	content  fyne.CanvasObject
	coreChip *statusChip
	listChip *statusChip
	tabs     []*tabButton
	body     *fyne.Container
	pages    []fyne.CanvasObject
	tab      int
}

func newMamePage(u *ui) *mamePage {
	m := &mamePage{u: u, coreChip: newStatusChip(), listChip: newStatusChip()}
	m.pages = []fyne.CanvasObject{mameTabRoms: u.roms.content, mameTabThumbs: u.thumbs.content}
	m.body = container.NewStack()
	for i, name := range []string{mameTabRoms: L("ROMs"), mameTabThumbs: L("Thumbnails")} {
		i := i
		m.tabs = append(m.tabs, newTabButton(name, magenta, func() { m.showTab(i) }))
	}

	subtitle := text(L("Engine mame2003-plus · plays MAME 0.78 sets"), 14, textMuted, false)
	chips := container.New(layout.NewCustomPaddedHBoxLayout(8), m.coreChip.content, m.listChip.content)
	header := newAdaptiveRow(420, textTile(magenta, "MAME", 64),
		container.NewVBox(text("MAME", 30, white, true), subtitle, container.NewHBox(chips)))
	var tabs []fyne.CanvasObject
	for _, b := range m.tabs {
		tabs = append(tabs, b)
	}
	rule := canvas.NewRectangle(cardStroke)
	rule.SetMinSize(fyne.NewSize(1, 1))
	bar := container.NewBorder(nil, rule, nil, nil, container.New(layout.NewCustomPaddedHBoxLayout(18), tabs...))
	top := container.New(layout.NewCustomPaddedVBoxLayout(10), header, bar)
	m.content = container.New(layout.NewCustomPaddedLayout(24, 20, 28, 28),
		container.NewBorder(container.New(layout.NewCustomPaddedLayout(0, 14, 0, 0), top), nil, nil, nil, m.body))
	m.showTab(mameTabRoms)
	return m
}

// showTab switches between ROMs and Thumbnails.
func (m *mamePage) showTab(i int) {
	m.tab = i
	for k, b := range m.tabs {
		b.SetSelected(k == i)
	}
	m.body.Objects = []fyne.CanvasObject{m.pages[i]}
	m.body.Refresh()
}

func (m *mamePage) render(st models.Status) {
	var lib models.Library
	if st.Library != nil {
		lib = *st.Library
	}
	switch {
	case lib.Core.Installed:
		m.coreChip.Set(L("Core installed"), colorOK)
	case lib.Core.Downloading:
		m.coreChip.Set(L("Downloading core…"), colorWarn)
	default:
		m.coreChip.Set(L("Core missing"), colorDanger)
	}
	if lib.Core.Catalog {
		m.listChip.Set(L("Game list"), colorOK)
	} else {
		m.listChip.Set(L("No game list"), colorDanger)
	}
}
