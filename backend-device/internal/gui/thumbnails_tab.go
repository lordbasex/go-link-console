// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
	"github.com/lordbasex/go-link-console/backend-device/pkg/thumbnails"
)

// thumbColumn is the width of the Boxart, Title and Snap columns.
const thumbColumn = 64

// namingExample is the folder layout shown in "How to name them".
const namingExample = `thumbnails/
  Named_Boxarts/galaga.png
  Named_Titles/Galaga (Namco rev. B).png
  Named_Snaps/galaga.png`

// thumbnailsTab is MAME's Thumbnails tab: which of the host's own images
// each set has, and a drop target to add more. The images are the host's
// files; go-link never downloads them.
type thumbnailsTab struct {
	u       *ui
	content fyne.CanvasObject
	boxart  *statTile
	title   *statTile
	snap    *statTile
	list    *widget.List
	dir     *widget.Label
	message *widget.Label
	lib     models.Library
}

// thumbFile is one image found in what the host dropped.
type thumbFile struct {
	kind thumbnails.Kind
	path string
}

func newThumbnailsTab(u *ui) *thumbnailsTab {
	t := &thumbnailsTab{u: u}
	t.boxart = newStatTile("image", "Boxart", magenta, true)
	t.title = newStatTile("image", L("Title"), magenta, true)
	t.snap = newStatTile("image", L("Snap"), magenta, true)
	stats := newResponsiveGrid(150, 12, t.boxart.content, t.title.content, t.snap.content)

	t.list = widget.NewList(
		func() int { return len(t.lib.Roms) },
		func() fyne.CanvasObject { return newThumbRow() },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			if i < len(t.lib.Roms) {
				o.(*thumbRow).fill(t.lib.Roms[i])
			}
		},
	)
	column := func(s string) fyne.CanvasObject {
		l := text(s, 12, textMuted, true)
		l.Alignment = fyne.TextAlignCenter
		return sized(container.NewCenter(l), thumbColumn, 20)
	}
	head := container.NewBorder(nil, nil, nil,
		container.NewHBox(column("Boxart"), column(L("Title")), column(L("Snap"))),
		container.New(layout.NewCustomPaddedLayout(0, 0, 8, 0), text(L("Game"), 12, textMuted, true)))
	list := glassPadded(container.NewBorder(head, nil, nil, nil, t.list), 8, 6)

	t.message = wrapped("")
	t.message.Hide()
	drop := glass(container.NewVBox(
		container.NewHBox(iconImage("download", magenta.accent, 18), text(L("Drop images or a folder here"), 14, white, true)),
		wrapped(L("PNG or JPG. go-link matches each image to its game by file name.")),
		t.message,
	))

	example := widget.NewLabel(namingExample)
	example.TextStyle.Monospace = true
	example.SizeName = theme.SizeNameCaptionText
	howto := glass(container.NewVBox(
		text(L("How to name them"), 14, white, true),
		wrapped(L("One folder per type, file named after the set or the game title:")),
		glassPadded(example, 2, 4),
		wrapped(L("Thumbnail packs for MAME can be found on the internet. You are responsible for having the right to use them.")),
	))

	t.dir = widget.NewLabel("")
	t.dir.TextStyle.Monospace = true
	t.dir.Wrapping = fyne.TextWrapBreak
	open := widget.NewButtonWithIcon(L("Open folder"), icon("folder", white), t.openFolder)
	folder := glass(container.NewVBox(text(L("Thumbnails folder"), 14, white, true), t.dir, container.NewHBox(open)))

	// A little room on the right keeps the cards clear of the scroll bar.
	side := container.NewVScroll(container.New(layout.NewCustomPaddedLayout(0, 0, 0, 8), vstack(drop, howto, folder)))
	body := container.NewBorder(nil, nil, nil, container.New(layout.NewCustomPaddedLayout(0, 0, 12, 0), fixedWidth(340, side)), list)
	t.content = container.NewBorder(container.New(layout.NewCustomPaddedLayout(0, 12, 0, 0), stats), nil, nil, nil, body)
	return t
}

func (t *thumbnailsTab) render(st models.Status) {
	var lib models.Library
	if st.Library != nil {
		lib = *st.Library
	}
	if reflect.DeepEqual(lib, t.lib) {
		return // metrics tick: nothing in the library changed
	}
	t.lib = lib
	boxart, title, snap := thumbCounts(lib.Roms)
	n := len(lib.Roms)
	setCount(t.boxart, boxart, n, "the box or flyer")
	setCount(t.title, title, n, "the title screen")
	setCount(t.snap, snap, n, "a moment of play")
	t.dir.SetText(lib.ThumbnailsDir)
	t.list.Refresh()
}

// thumbCounts counts the sets that have each kind of thumbnail.
func thumbCounts(roms []models.RomInfo) (boxart, title, snap int) {
	for _, r := range roms {
		if r.Thumbs.Boxart {
			boxart++
		}
		if r.Thumbs.Title {
			title++
		}
		if r.Thumbs.Snap {
			snap++
		}
	}
	return boxart, title, snap
}

func setCount(tile *statTile, have, total int, detail string) {
	tile.Set(fmt.Sprintf("%d / %d", have, total), detail)
	frac := 0.0
	if total > 0 {
		frac = float64(have) / float64(total)
	}
	tile.meter.SetValue(frac, magenta.accent)
}

// openFolder shows the thumbnails folder in the file manager, creating it
// first so there is always something to open.
func (t *thumbnailsTab) openFolder() {
	dir := t.u.opts.Library.ThumbnailsDir()
	if dir == "" {
		t.u.showError(errors.New("there is no thumbnails folder"))
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.u.showError(err)
		return
	}
	u, err := url.Parse(storage.NewFileURI(dir).String())
	if err == nil {
		err = t.u.app.OpenURL(u)
	}
	if err != nil {
		t.u.showError(err)
	}
}

// dropped imports images or folders dropped while this tab is open.
func (t *thumbnailsTab) dropped(uris []fyne.URI) {
	t.showMessage(L("Reading…"))
	go func() {
		ok, failed := t.importThumbs(uris, func(i, n int, name string) {
			fyne.Do(func() { t.showMessage(Lf("Copying %d / %d · %s", i, n, name)) })
		})
		fyne.Do(func() { t.showMessage(thumbResult(ok, failed)) })
	}()
}

func (t *thumbnailsTab) showMessage(s string) {
	t.message.SetText(s)
	t.message.Show()
}

// importThumbs copies the dropped images into the thumbnails folder and
// rescans the library once at the end, so a whole pack is quick.
func (t *thumbnailsTab) importThumbs(uris []fyne.URI, progress func(i, n int, name string)) (ok int, failed []string) {
	lib := t.u.opts.Library
	root := lib.ThumbnailsDir()
	if root == "" {
		return 0, []string{"there is no thumbnails folder"}
	}
	files, failed := collectThumbs(uris)
	for i, f := range files {
		name := filepath.Base(f.path)
		if progress != nil {
			progress(i+1, len(files), name)
		}
		r, err := os.Open(f.path)
		if err == nil {
			_, err = thumbnails.Import(root, f.kind, name, r)
			r.Close()
		}
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", name, err))
			continue
		}
		ok++
	}
	if ok > 0 {
		lib.Scan()
	}
	return ok, failed
}

// collectThumbs finds the images in what was dropped. Loose images are
// Boxarts. A folder named like a kind (Named_Snaps) holds that kind; a
// folder with such sub-folders is a pack with one kind per sub-folder; any
// other folder holds Boxarts.
func collectThumbs(uris []fyne.URI) (files []thumbFile, skipped []string) {
	for _, uri := range uris {
		path := uri.Path()
		fi, err := os.Stat(path)
		switch {
		case err != nil:
			skipped = append(skipped, fmt.Sprintf("%s (%v)", uri.Name(), err))
		case !fi.IsDir():
			if isImageFile(path) {
				files = append(files, thumbFile{thumbnails.Boxart, path})
			} else {
				skipped = append(skipped, uri.Name()+" ("+L("not a PNG or JPG")+")")
			}
		default:
			if kind, ok := kindOfFolder(filepath.Base(path)); ok {
				files = append(files, imagesIn(path, kind)...)
				continue
			}
			found := false
			entries, _ := os.ReadDir(path)
			for _, e := range entries {
				if kind, ok := kindOfFolder(e.Name()); ok && e.IsDir() {
					files = append(files, imagesIn(filepath.Join(path, e.Name()), kind)...)
					found = true
				}
			}
			if !found {
				files = append(files, imagesIn(path, thumbnails.Boxart)...)
			}
		}
	}
	return files, skipped
}

// kindOfFolder maps a folder name (Named_Boxarts, Named_Titles,
// Named_Snaps, any case) to its kind.
func kindOfFolder(name string) (thumbnails.Kind, bool) {
	for _, k := range thumbnails.Kinds {
		if strings.EqualFold(name, k.Folder()) {
			return k, true
		}
	}
	return "", false
}

// imagesIn lists the images directly inside dir, skipping hidden files.
func imagesIn(dir string, kind thumbnails.Kind) []thumbFile {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []thumbFile
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") || !isImageFile(e.Name()) {
			continue
		}
		out = append(out, thumbFile{kind, filepath.Join(dir, e.Name())})
	}
	return out
}

func isImageFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg":
		return true
	}
	return false
}

// thumbResult is the short message after a drop.
func thumbResult(ok int, failed []string) string {
	msg := Lf("Added %d thumbnails.", ok)
	if ok == 1 {
		msg = L("Added 1 thumbnail.")
	}
	if len(failed) > 0 {
		shown := failed
		if len(shown) > 5 {
			shown = shown[:5]
		}
		msg += " " + L("Not added:") + " " + strings.Join(shown, ", ")
		if more := len(failed) - len(shown); more > 0 {
			msg += " " + Lf("and %d more", more)
		}
		msg += "."
	}
	return msg
}

// thumbRow is one line of the thumbnails list.
type thumbRow struct {
	widget.BaseWidget
	title  *widget.Label
	detail *widget.Label
	marks  [3]*canvas.Image // Boxart, Title, Snap
}

func newThumbRow() *thumbRow {
	r := &thumbRow{title: widget.NewLabel(""), detail: widget.NewLabel("")}
	r.title.TextStyle.Bold = true
	r.title.Truncation = fyne.TextTruncateEllipsis
	r.detail.Truncation = fyne.TextTruncateEllipsis
	r.detail.Importance = widget.LowImportance
	r.detail.SizeName = theme.SizeNameCaptionText
	for i := range r.marks {
		r.marks[i] = iconImage("dash", textFaint, 18)
	}
	r.ExtendBaseWidget(r)
	return r
}

func (r *thumbRow) CreateRenderer() fyne.WidgetRenderer {
	var cols []fyne.CanvasObject
	for _, m := range r.marks {
		cols = append(cols, sized(container.NewCenter(m), thumbColumn, 24))
	}
	right := container.NewHBox(cols...)
	return widget.NewSimpleRenderer(container.NewBorder(nil, nil, nil, container.NewCenter(right), container.New(&tightVBox{}, r.title, r.detail)))
}

func (r *thumbRow) fill(rom models.RomInfo) {
	title := rom.Title
	if title == "" {
		title = rom.Name
	}
	r.title.SetText(title)
	r.detail.SetText(rom.Name)
	for i, has := range thumbMarks(rom.Thumbs) {
		if has {
			r.marks[i].Resource = icon("check", colorOK)
		} else {
			r.marks[i].Resource = icon("dash", textFaint)
		}
		r.marks[i].Refresh()
	}
}

// thumbMarks lists which kinds a set has, in column order.
func thumbMarks(t models.Thumbs) [3]bool {
	return [3]bool{t.Boxart, t.Title, t.Snap}
}
