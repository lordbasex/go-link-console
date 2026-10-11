// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"errors"
	"fmt"
	"image/color"
	"net/url"
	"os"
	"reflect"
	"strings"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
	"github.com/lordbasex/go-link-console/backend-device/internal/services"
	"github.com/lordbasex/go-link-console/backend-device/pkg/romcheck"
)

// romsTab is MAME's ROMs tab, for managing the ROM folder: which folder,
// adding sets by dropping them, and how many there are. The library itself
// (every game, its check, playing it) is on the website, where there is
// room to browse it.
type romsTab struct {
	u          *ui
	content    fyne.CanvasObject
	core       *widget.Label
	coreErr    *widget.Label
	coreButton *widget.Button
	dir        *widget.Label
	total      *statTile
	runs       *statTile
	wontRun    *statTile
	added      *statTile
	size       *statTile
	free       *statTile
	message    *widget.Label
	lib        models.Library
	// uploaded counts the sets added from this window since it opened.
	uploaded int
}

func newRomsTab(u *ui) *romsTab {
	t := &romsTab{u: u}
	t.core = widget.NewLabel("")
	t.core.Truncation = fyne.TextTruncateEllipsis
	t.coreErr = widget.NewLabel("")
	t.coreErr.Importance = widget.DangerImportance
	t.coreErr.Wrapping = fyne.TextWrapWord
	t.coreErr.Hide()
	t.coreButton = widget.NewButtonWithIcon(L("Download what is missing"), icon("download", white), func() {
		if err := u.opts.Library.DownloadCore(u.opts.Ctx); err != nil {
			u.showError(err)
		}
	})
	t.dir = widget.NewLabel("")
	t.dir.TextStyle.Monospace = true
	t.dir.Truncation = fyne.TextTruncateEllipsis
	choose := widget.NewButtonWithIcon(L("Choose folder…"), icon("folder", white), t.chooseFolder)
	open := widget.NewButtonWithIcon(L("Open folder"), icon("folder", white), t.openFolder)
	t.message = widget.NewLabel("")
	t.message.Wrapping = fyne.TextWrapWord
	t.message.Hide()

	setup := glass(container.NewVBox(
		container.NewBorder(nil, nil, text(L("Emulator"), 13, textMuted, false), t.coreButton, t.core),
		t.coreErr,
		widget.NewSeparator(),
		container.NewBorder(nil, nil, text(L("Folder"), 13, textMuted, false), container.NewHBox(choose, open), t.dir),
	))

	t.total = newStatTile("rom", L("ROM sets"), magenta, false)
	t.runs = newStatTile("play", L("Run"), magenta, false)
	t.wontRun = newStatTile("rom", L("Will not run"), magenta, false)
	t.added = newStatTile("download", L("Added here"), magenta, false)
	t.size = newStatTile("folder", L("Folder size"), magenta, false)
	t.free = newStatTile("folder", L("Free on the disk"), magenta, false)
	stats := newResponsiveGrid(190, 12,
		t.total.content, t.runs.content, t.wontRun.content,
		t.added.content, t.size.content, t.free.content)

	body := vstack(setup, t.dropZone(), stats, t.message,
		wrapped(L("Every game, its check and Play are on the website, in My device › ROMs.")))
	// The MAME page pads this tab and puts its header above it.
	// A little room on the right keeps the cards clear of the scroll bar.
	t.content = container.NewVScroll(container.New(layout.NewCustomPaddedLayout(0, 0, 0, 8), body))
	return t
}

// dropZone invites to drop zips on the window: they are copied into the
// ROM folder (never replacing a set), as dropping them anywhere does.
func (t *romsTab) dropZone() fyne.CanvasObject {
	border := canvas.NewRectangle(color.Transparent)
	border.StrokeColor = withAlpha(white, 0x55)
	border.StrokeWidth = 1.5
	border.CornerRadius = 16
	title := text(L("Drop .zip ROM sets here"), 18, white, true)
	title.Alignment = fyne.TextAlignCenter
	hint := text(L("They are copied into the folder above. An existing set is never replaced."), 13, textMuted, false)
	hint.Alignment = fyne.TextAlignCenter
	add := widget.NewButtonWithIcon(L("Choose a file…"), icon("download", white), t.chooseFile)
	inner := container.New(layout.NewCustomPaddedLayout(26, 26, 20, 20), container.NewVBox(
		container.NewCenter(iconImage("rom", white, 40)), title, hint, container.NewCenter(add)))
	return container.NewStack(border, inner)
}

func (t *romsTab) render(st models.Status) {
	var lib models.Library
	if st.Library != nil {
		lib = *st.Library
	}
	if reflect.DeepEqual(lib, t.lib) {
		return // metrics tick: nothing in the library changed
	}
	t.lib = lib
	t.renderCore(lib)
	t.dir.SetText(lib.Dir)
	runs, wontRun := 0, 0
	var size int64
	for _, r := range lib.Roms {
		size += r.Size
		switch {
		case playable(r):
			runs++
		case r.Check != nil && r.Check.Status != romcheck.StatusBIOS:
			wontRun++
		}
	}
	t.total.Set(fmt.Sprintf("%d", len(lib.Roms)), L("zip files in the folder"))
	if lib.Core.Catalog {
		t.runs.Set(fmt.Sprintf("%d", runs), L("ready to play"))
		t.wontRun.Set(fmt.Sprintf("%d", wontRun), L("missing files or another MAME version"))
	} else {
		t.runs.Set("—", L("download the game list to check them"))
		t.wontRun.Set("—", "")
	}
	t.added.Set(fmt.Sprintf("%d", t.uploaded), L("since this window opened"))
	t.size.Set(formatBytes(uint64(size)), L("ROM sets"))
	if lib.Disk != nil && lib.Disk.Total > 0 {
		t.free.Set(formatBytes(lib.Disk.Free), Lf("of %s", formatBytes(lib.Disk.Total)))
	} else {
		t.free.Set("—", "")
	}
}

// renderCore shows whether the emulator core and its game list are
// installed. Nothing is bundled: both are downloaded only on request.
func (t *romsTab) renderCore(lib models.Library) {
	state := func(installed bool) string {
		switch {
		case installed:
			return L("installed")
		case lib.Core.Downloading:
			return L("downloading…")
		default:
			return L("not installed")
		}
	}
	t.core.SetText(Lf("mame2003-plus: %s · game list: %s", state(lib.Core.Installed), state(lib.Core.Catalog)))
	t.coreErr.SetText(lib.Core.Error)
	if lib.Core.Error == "" {
		t.coreErr.Hide()
	} else {
		t.coreErr.Show()
	}
	if lib.Core.Downloading || (lib.Core.Installed && lib.Core.Catalog) {
		t.coreButton.Disable()
	} else {
		t.coreButton.Enable()
	}
}

func (t *romsTab) chooseFolder() {
	d := dialog.NewFolderOpen(func(folder fyne.ListableURI, err error) {
		if err != nil {
			t.u.showError(err)
			return
		}
		if folder != nil {
			t.setFolder(folder.Path())
		}
	}, t.u.win)
	if dir := t.u.opts.Library.Dir(); dir != "" {
		if l, err := storage.ListerForURI(storage.NewFileURI(dir)); err == nil {
			d.SetLocation(l)
		}
	}
	d.Show()
}

// chooseFile adds one zip picked in a dialog, like dropping it.
func (t *romsTab) chooseFile() {
	d := dialog.NewFileOpen(func(f fyne.URIReadCloser, err error) {
		if err != nil {
			t.u.showError(err)
			return
		}
		if f != nil {
			uri := f.URI()
			f.Close()
			t.dropped([]fyne.URI{uri})
		}
	}, t.u.win)
	d.SetFilter(storage.NewExtensionFileFilter([]string{".zip", ".ZIP"}))
	d.Show()
}

// openFolder shows the ROM folder in the Finder (or the file manager).
func (t *romsTab) openFolder() {
	if dir := t.u.opts.Library.Dir(); dir != "" {
		_ = t.u.app.OpenURL(&url.URL{Scheme: "file", Path: dir})
	}
}

// setFolder changes the ROM folder off the UI thread: the new folder is
// scanned and checked.
func (t *romsTab) setFolder(dir string) {
	t.say(Lf("Reading %s…", dir))
	go func() {
		err := t.u.opts.Library.SetDir(dir)
		fyne.Do(func() {
			if err != nil {
				t.say("")
				t.u.showError(err)
				return
			}
			t.say(L("ROM folder changed."))
		})
	}()
}

// dropped handles files or a folder dropped on the window.
func (t *romsTab) dropped(uris []fyne.URI) {
	if len(uris) == 1 {
		if fi, err := os.Stat(uris[0].Path()); err == nil && fi.IsDir() {
			t.setFolder(uris[0].Path())
			return
		}
	}
	go func() {
		ok, failed := t.importFiles(uris, func(i, n int, name string) {
			fyne.Do(func() { t.say(Lf("Copying %d / %d · %s", i, n, name)) })
		})
		fyne.Do(func() {
			t.uploaded += ok
			t.lib = models.Library{} // repaint the counters
			t.render(t.u.opts.Status.Snapshot())
			t.say(importResult(ok, len(uris), failed))
		})
	}()
}

// importFiles copies dropped zips into the ROM folder. The library checks
// each name and file and never replaces an existing set.
func (t *romsTab) importFiles(uris []fyne.URI, progress func(i, n int, name string)) (ok int, failed []string) {
	for i, uri := range uris {
		name := uri.Name()
		if progress != nil {
			progress(i+1, len(uris), name)
		}
		if !strings.EqualFold(uri.Extension(), ".zip") {
			failed = append(failed, name+" ("+L("not a .zip")+")")
			continue
		}
		f, err := os.Open(uri.Path())
		if err == nil {
			err = t.u.opts.Library.Import(name, f)
			f.Close()
		}
		if errors.Is(err, services.ErrNotRom) {
			// a Willy Maker project or AI pack: its sources, not the ROM (T-21)
			failed = append(failed, name+" ("+L("a Willy Maker project or AI pack, not a ROM: make the ROM with Create ROM in Willy Maker")+")")
			continue
		}
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", name, err))
			continue
		}
		ok++
	}
	if ok > 0 {
		t.u.opts.Library.ScanNow() // the window counts the new sets at once
	}
	return ok, failed
}

func importResult(ok, n int, failed []string) string {
	msg := Lf("Added %d of %d.", ok, n)
	if len(failed) > 0 {
		msg += " " + L("Not added:") + " " + strings.Join(failed, ", ")
	}
	return msg
}

// say shows a short message, or hides it when empty.
func (t *romsTab) say(s string) {
	t.message.SetText(s)
	if s == "" {
		t.message.Hide()
	} else {
		t.message.Show()
	}
}

// playable reports whether the core can run a set.
func playable(r models.RomInfo) bool {
	return r.Check == nil || r.Check.Status == romcheck.StatusOK
}

// capitalize upper-cases the first letter.
func capitalize(s string) string {
	for i, r := range s {
		return string(unicode.ToUpper(r)) + s[i+len(string(r)):]
	}
	return s
}

// tightVBox stacks labels with less padding between them.
type tightVBox struct{}

func (*tightVBox) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var s fyne.Size
	for _, o := range objects {
		m := o.MinSize()
		s.Width = max(s.Width, m.Width)
		s.Height += m.Height - 2*theme.Padding()
	}
	return s.AddWidthHeight(0, 2*theme.Padding())
}

func (*tightVBox) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	y := float32(0)
	for _, o := range objects {
		h := o.MinSize().Height
		o.Move(fyne.NewPos(0, y))
		o.Resize(fyne.NewSize(size.Width, h))
		y += h - 2*theme.Padding()
	}
}
