// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"fmt"
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/lordbasex/go-link/backend-device/internal/models"
	"github.com/lordbasex/go-link/backend-device/pkg/hdgames"
)

// The tabs of go-link HD's page.
const (
	hdTabGames = iota
	hdTabScreen
)

// The choices of the Screen tab, in models.HDRoomSizes and HDCodecs order.
var (
	hdSizeLabels  = []string{"720p", "1080p"}
	hdCodecLabels = []string{"Automatic", "VP8"}
)

// hdPage is go-link HD's engine, under Core in the sidebar: a header with
// the engine's state and two tabs, its games (folder and free starter
// games) and the screen of its rooms.
type hdPage struct {
	u          *ui
	content    fyne.CanvasObject
	engineChip *statusChip
	tabs       []*tabButton
	body       *fyne.Container
	pages      []fyne.CanvasObject

	dir     *widget.Label
	games   *statTile
	size    *statTile
	message *widget.Label
	starter map[string]*starterRow

	roomSize *segmented
	codec    *segmented
}

// starterRow is one free starter game and its download buttons.
type starterRow struct {
	state *widget.Label
	err   *widget.Label
	sizes map[string]*widget.Button
}

func newHDPage(u *ui) *hdPage {
	h := &hdPage{u: u, engineChip: newStatusChip(), starter: map[string]*starterRow{}}
	h.pages = []fyne.CanvasObject{hdTabGames: h.gamesTab(), hdTabScreen: h.screenTab()}
	h.body = container.NewStack()
	for i, name := range []string{hdTabGames: L("Games"), hdTabScreen: L("Screen")} {
		i := i
		h.tabs = append(h.tabs, newTabButton(name, azure, func() { h.showTab(i) }))
	}
	subtitle := text(L("go-link's own engine · HD games made for go-link"), 14, textMuted, false)
	header := newAdaptiveRow(420, textTile(azure, "HD", 64),
		container.NewVBox(text("go-link HD", 30, white, true), subtitle, container.NewHBox(h.engineChip.content)))
	var tabs []fyne.CanvasObject
	for _, b := range h.tabs {
		tabs = append(tabs, b)
	}
	rule := canvas.NewRectangle(cardStroke)
	rule.SetMinSize(fyne.NewSize(1, 1))
	bar := container.NewBorder(nil, rule, nil, nil, container.New(layout.NewCustomPaddedHBoxLayout(18), tabs...))
	top := container.New(layout.NewCustomPaddedVBoxLayout(10), header, bar)
	h.content = container.New(layout.NewCustomPaddedLayout(24, 20, 28, 28),
		container.NewBorder(container.New(layout.NewCustomPaddedLayout(0, 14, 0, 0), top), nil, nil, nil, h.body))
	h.showTab(hdTabGames)
	return h
}

// showTab switches between Games and Screen.
func (h *hdPage) showTab(i int) {
	for k, b := range h.tabs {
		b.SetSelected(k == i)
	}
	h.body.Objects = []fyne.CanvasObject{h.pages[i]}
	h.body.Refresh()
}

// gamesTab is the games folder and the free starter games.
func (h *hdPage) gamesTab() fyne.CanvasObject {
	h.dir = widget.NewLabel("")
	h.dir.TextStyle.Monospace = true
	h.dir.Truncation = fyne.TextTruncateEllipsis
	choose := widget.NewButtonWithIcon(L("Choose folder…"), icon("folder", white), h.chooseFolder)
	open := widget.NewButtonWithIcon(L("Open folder"), icon("folder", white), h.openFolder)
	folder := glass(container.NewVBox(
		settingTitle(L("Games folder"), L("go-link HD's games (.glhd files) live here, apart from the MAME ROMs.")),
		container.NewBorder(nil, nil, nil, container.NewHBox(choose, open), h.dir),
	))
	h.games = newStatTile("gamepad", L("Games"), azure, false)
	h.size = newStatTile("folder", L("Folder size"), azure, false)
	stats := newResponsiveGrid(190, 12, h.games.content, h.size.content)

	rows := container.NewVBox(settingTitle(L("Free games"), L("Games made with go-link HD that you can download for free. The first time a browser links, the device downloads them in the size of its rooms and opens a room to play them.")))
	for _, g := range hdgames.Starter {
		row := &starterRow{state: widget.NewLabel(""), err: widget.NewLabel(""), sizes: map[string]*widget.Button{}}
		row.err.Importance = widget.DangerImportance
		row.err.Wrapping = fyne.TextWrapWord
		row.err.Hide()
		var buttons []fyne.CanvasObject
		for _, size := range models.HDRoomSizes {
			name, size := g.Name, size
			b := widget.NewButtonWithIcon(Lf("Download %s", size), icon("download", white), func() { h.download(name, size) })
			row.sizes[size] = b
			buttons = append(buttons, b)
		}
		h.starter[g.Name] = row
		rows.Add(widget.NewSeparator())
		rows.Add(container.NewBorder(nil, nil, text(g.Title, 15, white, true), container.NewHBox(buttons...), row.state))
		rows.Add(row.err)
	}
	h.message = widget.NewLabel("")
	h.message.Wrapping = fyne.TextWrapWord
	h.message.Hide()
	note := wrapped(L("To remove a game, delete its file from the folder; its rooms are managed on the website, in My device › Rooms (archive or delete them there)."))
	note.Importance = widget.LowImportance
	note.SizeName = theme.SizeNameCaptionText
	body := vstack(folder, stats, glass(rows), h.message, note)
	return container.NewVScroll(container.New(layout.NewCustomPaddedLayout(0, 0, 0, 8), body))
}

// screenTab is the size and codec of go-link HD's rooms.
func (h *hdPage) screenTab() fyne.CanvasObject {
	apply := func() {
		s := h.u.opts.Settings
		if s == nil {
			return
		}
		size := models.HDRoomSizes[labelIndex(hdSizeLabels, h.roomSize.Selected)]
		codec := models.HDCodecs[labelIndex(translated(hdCodecLabels), h.codec.Selected)]
		h.u.settings.background(func() {
			err := s.SetHDScreen(size, codec)
			fyne.Do(func() {
				if err != nil {
					h.u.showError(err)
				}
			})
		})
	}
	h.roomSize = newSegmented(hdSizeLabels, azure, func(string) { apply() })
	h.codec = newSegmented(translated(hdCodecLabels), azure, func(string) { apply() })
	sizeHelp := wrapped(L("720p suits most computers and connections. 1080p needs a computer that encodes it at 60 frames a second (a Mac's hardware does) and more upload bandwidth; download the 1080p version of a game to see it in full detail."))
	sizeHelp.Importance = widget.LowImportance
	sizeHelp.SizeName = theme.SizeNameCaptionText
	codecHelp := wrapped(L("Automatic sends H.264 from the computer's hardware when it has an encoder, and VP8 when it does not. VP8 always works but uses more of the processor."))
	codecHelp.Importance = widget.LowImportance
	codecHelp.SizeName = theme.SizeNameCaptionText
	note := wrapped(L("Rooms started from now on use these settings; a running room keeps its own until it starts again."))
	note.Importance = widget.LowImportance
	note.SizeName = theme.SizeNameCaptionText
	body := vstack(
		glass(container.NewVBox(settingTitle(L("Room size"), L("The picture go-link HD's rooms send to the players.")), container.NewHBox(h.roomSize.content), sizeHelp)),
		glass(container.NewVBox(settingTitle(L("Codec"), L("How the picture is compressed.")), container.NewHBox(h.codec.content), codecHelp)),
		note,
	)
	return container.NewVScroll(container.New(layout.NewCustomPaddedLayout(0, 0, 0, 8), body))
}

func (h *hdPage) render(st models.Status) {
	var lib models.Library
	if st.Library != nil {
		lib = *st.Library
	}
	if lib.Core.HDInstalled {
		h.engineChip.Set(L("Engine installed"), colorOK)
	} else {
		h.engineChip.Set(L("Engine missing"), colorDanger)
	}
	h.dir.SetText(lib.HDDir)
	h.games.Set(fmt.Sprintf("%d", lib.HDGames), L("games in the folder"))
	h.size.Set(formatBytes(uint64(lib.HDBytes)), L("go-link HD games"))
	hd := st.HD
	if hd == nil {
		return
	}
	if h.roomSize.Selected != hd.RoomSize {
		h.roomSize.SetSelected(hd.RoomSize)
	}
	codec := L(hdCodecLabels[0])
	if hd.Codec == models.HDVP8 {
		codec = L(hdCodecLabels[1])
	}
	if h.codec.Selected != codec {
		h.codec.SetSelected(codec)
	}
	h.roomSize.SetDisabled(h.u.opts.Settings == nil)
	h.codec.SetDisabled(h.u.opts.Settings == nil)
	for _, g := range hd.Starter {
		row := h.starter[g.Name]
		if row == nil {
			continue
		}
		switch {
		case g.Downloading != "" && g.Total > 0:
			row.state.SetText(Lf("Downloading %s · %d %% (%s of %s)", g.Downloading, g.Done*100/g.Total, formatBytes(uint64(g.Done)), formatBytes(uint64(g.Total))))
		case g.Downloading != "":
			row.state.SetText(Lf("Downloading %s…", g.Downloading))
		case g.Installed && g.Size != "":
			row.state.SetText(Lf("Installed · %s", g.Size))
		case g.Installed:
			row.state.SetText(L("Installed"))
		default:
			row.state.SetText(L("Not installed"))
		}
		row.err.SetText(g.Error)
		if g.Error == "" {
			row.err.Hide()
		} else {
			row.err.Show()
		}
		for size, b := range row.sizes {
			if g.Downloading != "" || h.u.opts.HDGames == nil || (g.Installed && g.Size == size) {
				b.Disable()
			} else {
				b.Enable()
			}
		}
	}
}

// download starts a starter game's download (its progress comes with the
// status).
func (h *hdPage) download(name, size string) {
	if h.u.opts.HDGames == nil {
		return
	}
	if err := h.u.opts.HDGames.Download(h.u.opts.Ctx, name, size); err != nil {
		h.u.showError(err)
	}
}

func (h *hdPage) chooseFolder() {
	d := dialog.NewFolderOpen(func(folder fyne.ListableURI, err error) {
		if err != nil {
			h.u.showError(err)
			return
		}
		if folder != nil {
			h.setFolder(folder.Path())
		}
	}, h.u.win)
	if dir := h.u.opts.Library.HDDir(); dir != "" {
		if l, err := storage.ListerForURI(storage.NewFileURI(dir)); err == nil {
			d.SetLocation(l)
		}
	}
	d.Show()
}

// openFolder shows the games folder in the Finder (or the file manager).
func (h *hdPage) openFolder() {
	if dir := h.u.opts.Library.HDDir(); dir != "" {
		_ = h.u.app.OpenURL(&url.URL{Scheme: "file", Path: dir})
	}
}

// setFolder changes the games folder off the UI thread.
func (h *hdPage) setFolder(dir string) {
	go func() {
		err := h.u.opts.Library.SetHDDir(dir)
		fyne.Do(func() {
			if err != nil {
				h.u.showError(err)
				return
			}
			h.message.SetText(L("Games folder changed."))
			h.message.Show()
		})
	}()
}
