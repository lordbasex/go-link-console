// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
	"github.com/lordbasex/go-link-console/backend-device/internal/services"
	"github.com/lordbasex/go-link-console/backend-device/pkg/thumbnails"
)

// The categories of the Settings page, in order.
const (
	settingsGeneral = iota
	settingsThumbnails
	settingsRooms
	settingsNetwork
)

var settingsCategories = []struct{ title, icon string }{
	settingsGeneral:    {"General", "gear"},
	settingsThumbnails: {"Thumbnails", "image"},
	settingsRooms:      {"Rooms", "players"},
	settingsNetwork:    {"Network", "signal"},
}

// Labels of the thumbnail choices, in the order of thumbnails.Kinds.
var kindLabels = []string{"Boxart", "Title", "Snap"}

// Labels of the video qualities, in the order of models.VideoQualities.
var qualityLabels = []string{"High", "Normal", "Saver"}

// qualityHelp explains the video qualities.
const qualityHelp = "High and Normal send the picture at twice the game's size, so every pixel keeps its own color (High uses more bandwidth). Saver sends the game's own size, for slow computers or slow uplinks. A room whose computer cannot keep up with twice the size switches to Saver by itself. Changes apply to running rooms at once."

// thumbHelp explains the thumbnails folder. It never names a website.
const thumbHelp = "One folder per type: Named_Boxarts, Named_Titles, Named_Snaps. Images are matched to games by set name or title. Thumbnail packs for MAME can be found on the internet."

// settingsPage holds the host's preferences: a column of categories on
// the left and the chosen category's cards on the right. Changes are
// saved at once through SettingsService.
type settingsPage struct {
	u        *ui
	content  fyne.CanvasObject
	cats     []*sidebarRow
	body     *fyne.Container
	pages    []fyne.CanvasObject
	cat      int
	kind     *segmented
	language *segmented
	autoUp   *widget.Check
	dir      *widget.Label
	thumbErr *widget.Label
	choose   *widget.Button
	reset    *widget.Button
	quality  *segmented
	maxRooms *widget.Label
	saves    *widget.Label
	signal   *widget.Label
	state    *widget.Label
	ice      *widget.Label
	// background runs slow work off the UI thread (tests run it inline).
	background func(func())
}

func newSettingsPage(u *ui) *settingsPage {
	p := &settingsPage{u: u, background: func(f func()) { go f() }}
	mono := func() *widget.Label {
		l := widget.NewLabel("")
		l.TextStyle.Monospace = true
		l.Wrapping = fyne.TextWrapBreak
		return l
	}
	p.pages = []fyne.CanvasObject{
		settingsGeneral:    p.generalCards(),
		settingsThumbnails: p.thumbnailsCards(mono),
		settingsRooms:      p.roomsCards(mono),
		settingsNetwork:    p.networkCards(mono),
	}
	p.body = container.NewStack()

	cats := container.New(layout.NewCustomPaddedVBoxLayout(4))
	for i, c := range settingsCategories {
		i := i
		row := newSidebarRow(L(c.title), c.icon, emerald, func() { p.showCategory(i) })
		p.cats = append(p.cats, row)
		cats.Add(row)
	}

	subtitle := text(L("How this device shows and runs your games"), 14, textMuted, false)
	header := newAdaptiveRow(420, heroTile(emerald, "gear", 64),
		container.NewVBox(text(L("Settings"), 30, white, true), subtitle))
	left := container.New(layout.NewCustomPaddedLayout(0, 0, 0, 16), fixedWidth(190, cats))
	// A little room on the right keeps the cards clear of the scroll bar.
	right := container.NewVScroll(container.New(layout.NewCustomPaddedLayout(0, 0, 0, 8), p.body))
	p.content = container.New(layout.NewCustomPaddedLayout(24, 20, 28, 28),
		container.NewBorder(container.New(layout.NewCustomPaddedLayout(0, 18, 0, 0), header), nil, left, nil, right))
	p.showCategory(settingsGeneral)
	p.refresh()
	return p
}

// showCategory switches the cards on the right.
func (p *settingsPage) showCategory(i int) {
	p.cat = i
	for k, r := range p.cats {
		r.SetSelected(k == i)
	}
	p.body.Objects = []fyne.CanvasObject{p.pages[i]}
	p.body.Refresh()
}

func (p *settingsPage) thumbnailsCards(mono func() *widget.Label) fyne.CanvasObject {
	p.kind = newSegmented(translated(kindLabels), emerald, func(label string) {
		kind := string(thumbnails.Kinds[labelIndex(translated(kindLabels), label)])
		p.change(func(t *models.ThumbnailSettings) { t.Kind = kind })
	})
	p.thumbErr = widget.NewLabel("")
	p.thumbErr.Importance = widget.DangerImportance
	p.thumbErr.Wrapping = fyne.TextWrapWord
	p.thumbErr.Hide()

	p.dir = mono()
	p.choose = widget.NewButtonWithIcon(L("Choose…"), icon("folder", white), p.chooseFolder)
	p.reset = widget.NewButton(L("Default"), func() {
		p.change(func(t *models.ThumbnailSettings) { t.Dir = "" })
	})
	open := widget.NewButtonWithIcon(L("Open folder"), icon("folder", white), p.u.thumbs.openFolder)
	help := wrapped(L(thumbHelp))
	help.Importance = widget.LowImportance
	help.SizeName = theme.SizeNameCaptionText

	show := glass(container.NewVBox(
		settingTitle(L("Show"), L("Which image the device and the browsers show for each game.")),
		container.NewHBox(p.kind.content),
	))
	folder := glass(container.NewVBox(
		settingTitle(L("Folder"), L("Where the device looks for your thumbnails.")),
		p.dir,
		container.NewHBox(p.choose, p.reset, open),
		help,
	))
	return vstack(p.thumbErr, show, folder)
}

// generalCards holds the window's language.
func (p *settingsPage) generalCards() fyne.CanvasObject {
	labels := make([]string, len(languages))
	for i, l := range languages {
		labels[i] = l.label
		if l.id == "" {
			labels[i] = L(l.label)
		}
	}
	p.language = newSegmented(labels, emerald, func(label string) {
		id := languages[labelIndex(labels, label)].id
		if p.u.opts.SetLanguage != nil {
			if err := p.u.opts.SetLanguage(id); err != nil {
				p.u.showError(err)
				return
			}
		}
		p.u.opts.Language = id
		fyne.Do(p.u.relanguage)
	})
	for i, l := range languages {
		if l.id == p.u.opts.Language {
			p.language.SetSelected(labels[i])
		}
	}
	p.autoUp = widget.NewCheck(L("Check for updates automatically"), func(on bool) {
		if p.u.opts.SetAutoUpdate == nil || on == p.u.opts.AutoUpdate {
			return
		}
		if err := p.u.opts.SetAutoUpdate(on); err != nil {
			p.u.showError(err)
			p.autoUp.SetChecked(p.u.opts.AutoUpdate)
			return
		}
		p.u.opts.AutoUpdate = on
	})
	p.autoUp.SetChecked(p.u.opts.AutoUpdate)
	check := widget.NewButtonWithIcon(L("Check for Updates…"), icon("download", white), func() { p.u.upd.ask() })
	uhelp := wrapped(L("Every 6 hours go-link asks GitHub whether a newer version is out, and offers it. An update is installed only when its download carries go-link Console's signature."))
	uhelp.Importance = widget.LowImportance
	uhelp.SizeName = theme.SizeNameCaptionText
	if p.u.opts.Updates == nil {
		p.autoUp.Disable()
	}
	return vstack(glass(container.NewVBox(
		settingTitle(L("Language"), L("The language of this window. Automatic follows your computer. The website has its own language switch.")),
		container.NewHBox(p.language.content),
	)), glass(container.NewVBox(
		settingTitle(L("Updates"), Lf("This is go-link Console %s.", bare(p.u.opts.Version))),
		p.autoUp,
		container.NewHBox(check),
		uhelp,
	)))
}

func (p *settingsPage) roomsCards(mono func() *widget.Label) fyne.CanvasObject {
	p.quality = newSegmented(translated(qualityLabels), emerald, func(label string) {
		q := models.VideoQualities[labelIndex(translated(qualityLabels), label)]
		s := p.u.opts.Settings
		if s == nil {
			return
		}
		p.background(func() {
			err := s.SetVideoQuality(q)
			fyne.Do(func() {
				if err != nil {
					p.u.showError(err)
				}
				p.refresh()
			})
		})
	})
	qhelp := wrapped(L(qualityHelp))
	qhelp.Importance = widget.LowImportance
	qhelp.SizeName = theme.SizeNameCaptionText
	video := glass(container.NewVBox(
		settingTitle(L("Video quality"), L("How game rooms send the picture to the players.")),
		container.NewHBox(p.quality.content),
		qhelp,
	))
	p.maxRooms = widget.NewLabel(Lf("%d by default", models.DefaultMaxRooms))
	p.saves = mono()
	p.saves.SetText(savesDir())
	note := wrapped(L("To change the limit, set max_rooms in device.json and restart the device."))
	note.Importance = widget.LowImportance
	note.SizeName = theme.SizeNameCaptionText
	return vstack(video, glass(container.NewVBox(
		widget.NewForm(
			widget.NewFormItem(L("Rooms at once"), p.maxRooms),
			widget.NewFormItem(L("Saved games"), p.saves),
		),
		note,
	)))
}

func (p *settingsPage) networkCards(mono func() *widget.Label) fyne.CanvasObject {
	p.signal, p.state, p.ice = mono(), widget.NewLabel(""), mono()
	note := wrapped(L("The signaling server comes from --server-signaling or signal_url in device.json. STUN and TURN servers come from the signaling server and are kept in memory only. A fixed UDP port and announced addresses are set with udp_port and announce_ips in device.json."))
	note.Importance = widget.LowImportance
	note.SizeName = theme.SizeNameCaptionText
	return vstack(glass(container.NewVBox(
		widget.NewForm(
			widget.NewFormItem(L("Signaling"), p.signal),
			widget.NewFormItem(L("State"), p.state),
			widget.NewFormItem(L("STUN / TURN"), p.ice),
		),
		note,
	)))
}

// settingTitle is a setting's name with a short explanation under it.
func settingTitle(title, help string) fyne.CanvasObject {
	h := wrapped(help)
	h.Importance = widget.LowImportance
	h.SizeName = theme.SizeNameCaptionText
	return container.NewVBox(text(title, 14, white, true), h)
}

func labelIndex(labels []string, label string) int {
	for i, l := range labels {
		if l == label {
			return i
		}
	}
	return 0
}

// savesDir is where saved games live: ~/go-link-console/saves.
func savesDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("~", "go-link-console", "saves")
	}
	return filepath.Join(home, "go-link-console", "saves")
}

// refresh shows the current thumbnail settings.
func (p *settingsPage) refresh() {
	s := p.u.opts.Settings
	if s == nil {
		p.kind.SetDisabled(true)
		p.quality.SetDisabled(true)
		p.choose.Disable()
		p.reset.Disable()
		p.dir.SetText(p.u.opts.Library.ThumbnailsDir())
		return
	}
	p.quality.SetSelected(translated(qualityLabels)[labelIndex(models.VideoQualities, s.VideoQuality())])
	t := s.Thumbnails()
	p.kind.SetSelected(translated(kindLabels)[labelIndex(kindValues(), t.Kind)])
	p.dir.SetText(s.ThumbnailsDir())
	if t.Dir == "" {
		p.reset.Disable()
	} else {
		p.reset.Enable()
	}
}

func kindValues() []string {
	out := make([]string, len(thumbnails.Kinds))
	for i, k := range thumbnails.Kinds {
		out[i] = string(k)
	}
	return out
}

// change edits the thumbnail settings and saves them off the UI thread:
// the library is scanned again with the new kind or folder.
func (p *settingsPage) change(edit func(*models.ThumbnailSettings)) {
	s := p.u.opts.Settings
	if s == nil {
		return
	}
	t := s.Thumbnails()
	edit(&t)
	p.showError("")
	p.background(func() {
		err := s.SetThumbnails(t)
		fyne.Do(func() {
			if err != nil {
				p.showError(settingsError(err, t))
			}
			p.refresh()
		})
	})
}

func settingsError(err error, t models.ThumbnailSettings) string {
	if errors.Is(err, services.ErrBadSetting) && t.Dir != "" {
		return L("That folder cannot be used: choose an existing folder.")
	}
	return L("The setting could not be saved:") + " " + err.Error()
}

func (p *settingsPage) showError(s string) {
	p.thumbErr.SetText(s)
	if s == "" {
		p.thumbErr.Hide()
	} else {
		p.thumbErr.Show()
	}
}

func (p *settingsPage) chooseFolder() {
	d := dialog.NewFolderOpen(func(folder fyne.ListableURI, err error) {
		if err != nil {
			p.showError(err.Error())
			return
		}
		if folder != nil {
			dir := folder.Path()
			p.change(func(t *models.ThumbnailSettings) { t.Dir = dir })
		}
	}, p.u.win)
	if s := p.u.opts.Settings; s != nil {
		if l, err := storage.ListerForURI(storage.NewFileURI(s.ThumbnailsDir())); err == nil {
			d.SetLocation(l)
		}
	}
	d.Show()
}

// render shows the connection details from the status.
func (p *settingsPage) render(st models.Status) {
	p.signal.SetText(st.Signal.URL)
	state := st.Signal.State
	if st.Signal.Error != "" {
		state += " · " + st.Signal.Error
	}
	p.state.SetText(capitalize(L(state)))
	if len(st.Signal.ICEURLs) == 0 {
		p.ice.SetText(L("Not received yet (they come from the signaling server)"))
	} else {
		p.ice.SetText(strings.Join(st.Signal.ICEURLs, "\n"))
	}
}

// translated returns labels in the window's language.
func translated(labels []string) []string {
	out := make([]string, len(labels))
	for i, l := range labels {
		out[i] = L(l)
	}
	return out
}

// settingsChanged repaints what depends on the settings. It runs on
// Fyne's thread.
func (u *ui) settingsChanged() {
	u.settings.refresh()
}
