// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

// Package gui is the device's native window, built with Fyne, in the
// visual language of MacDub: a deep gradient per section, a sidebar, glass
// cards, glossy hero tiles and a round action button. There is no web
// server on the device: the window calls the same services the linked
// website (over WebRTC) and the CLI use, in the same process.
package gui

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
	"github.com/lordbasex/go-link-console/backend-device/internal/services"
	"github.com/lordbasex/go-link-console/backend-device/pkg/trayicon"
)

// appID identifies the application to the operating system.
const appID = "org.go-link.console"

// Options wires the window to the device.
type Options struct {
	Ctx      context.Context // canceled when the device stops
	Quit     func()          // stops the device
	Status   *services.StatusService
	Library  *services.LibraryService
	Games    *services.GameService     // nil when rooms are disabled
	HDGames  *services.HDGamesService  // go-link HD's starter games; nil (tests) hides them
	Links    *services.LinkService     // remembered browsers; nil (tests) means linked
	Settings *services.SettingsService // host preferences; nil (tests) shows them read-only
	WebURL   string                    // where hosts type the pairing code
	Version  string
	// Language is the window's language (en, es, pt, or "" for the
	// computer's), and SetLanguage saves a new choice in device.json.
	Language    string
	SetLanguage func(string) error
	Logger      *slog.Logger
}

// section is one entry of the sidebar, with its own palette. Rows with a
// group sit indented under a heading with the group's name.
type section struct {
	title, icon string
	pal         palette
	group       string
}

// The pages, in sidebar order.
const (
	pageOverview = iota
	pageMAME
	pageHD
	pageSettings
)

var sections = []section{
	pageOverview: {"Overview", "device", violet, ""},
	pageMAME:     {"MAME", "arcade", magenta, "Core"},
	pageHD:       {"go-link HD", "gamepad", azure, "Core"},
	pageSettings: {"Settings", "gear", emerald, ""},
}

// ui is the window and everything in it. Its fields are only touched on
// Fyne's thread (fyne.Do).
type ui struct {
	opts       Options
	app        fyne.App
	win        fyne.Window
	bg         *canvas.LinearGradient
	rows       []*sidebarRow
	headings   []*canvas.Text
	pages      []fyne.CanvasObject
	page       *fyne.Container
	current    int
	main       fyne.CanvasObject
	onboarding *onboardingView
	overview   *overviewPage
	mame       *mamePage
	hd         *hdPage
	roms       *romsTab
	thumbs     *thumbnailsTab
	settings   *settingsPage
	panel      *trayPanel
	trayMenu   *fyne.Menu
	trayCode   *fyne.MenuItem
	showing    string // "main" or "onboarding"
	lastSize   fyne.Size
	last       models.Status
}

// Run shows the window, the tray icon and its panel, and blocks until the
// device stops or the user quits. It must run on the main goroutine.
func Run(opts Options) {
	a := app.NewWithID(appID)
	a.Settings().SetTheme(arcadeTheme{})
	a.SetIcon(appIcon())
	u := newUI(a, opts)
	u.setupTray()
	opts.Status.OnChange(func(st models.Status) {
		fyne.Do(func() { u.render(st) })
	})
	// stopped closes when the app loop ends: after that GLFW is gone and
	// any Fyne call panics (quitting from the menu used to crash here, when
	// the context was cancelled after the loop).
	stopped := make(chan struct{})
	go func() {
		defer ignoreAfterStop()
		select {
		case <-u.opts.Ctx.Done():
			fyne.Do(a.Quit)
		case <-stopped:
		}
	}()
	go func() {
		defer ignoreAfterStop()
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-u.opts.Ctx.Done():
				return
			case <-stopped:
				return
			case <-t.C:
				fyne.Do(u.tick)
			}
		}
	}()
	u.win.Show()
	styleWindow("go-link Console")
	a.Run()
	close(stopped)
	if opts.Quit != nil {
		opts.Quit()
	}
}

// ignoreAfterStop swallows the panic of a Fyne call that raced with the
// end of the app loop (the window is already gone, nothing is lost).
func ignoreAfterStop() {
	_ = recover()
}

// newUI builds the window without showing it (tests use a test app).
func newUI(a fyne.App, opts Options) *ui {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Ctx == nil {
		opts.Ctx = context.Background()
	}
	if opts.WebURL == "" {
		opts.WebURL = models.DefaultWebURL
	}
	u := &ui{opts: opts, app: a}
	setLanguage(opts.Language)
	u.win = a.NewWindow("go-link Console")
	// The gradient reaches the window edges (Fyne pads by default).
	u.win.SetPadded(false)
	u.bg = background(violet)
	u.build()
	u.win.SetContent(container.NewStack(u.bg, u.main))
	u.win.Resize(u.savedSize())
	u.win.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) {
		if u.showing == "main" {
			u.dropped(uris)
		}
	})
	u.show(pageOverview)
	u.render(opts.Status.Snapshot())
	if opts.Settings != nil {
		opts.Settings.OnChange(func() { fyne.Do(u.settingsChanged) })
	}
	return u
}

// build makes every page, the sidebar and the pairing view, in the
// current language.
func (u *ui) build() {
	u.overview = newOverviewPage(u)
	u.roms = newRomsTab(u)
	u.thumbs = newThumbnailsTab(u)
	u.mame = newMamePage(u)
	u.hd = newHDPage(u)
	u.settings = newSettingsPage(u)
	u.pages = []fyne.CanvasObject{pageOverview: u.overview.content, pageMAME: u.mame.content, pageHD: u.hd.content, pageSettings: u.settings.content}
	u.page = container.NewStack()
	u.rows, u.headings = nil, nil

	side := container.New(layout.NewCustomPaddedVBoxLayout(4))
	group := ""
	for i, s := range sections {
		i := i
		if s.group != "" && s.group != group {
			heading, label := sidebarHeading(L(s.group))
			u.headings = append(u.headings, label)
			side.Add(heading)
		}
		group = s.group
		row := newSidebarRow(L(s.title), s.icon, s.pal, func() { u.show(i) })
		u.rows = append(u.rows, row)
		if s.group != "" {
			side.Add(indented(row))
		} else {
			side.Add(row)
		}
	}
	brand := container.New(layout.NewCustomPaddedLayout(4, 18, 10, 0), container.NewHBox(logo(24), text("go-link Console", 15, white, true)))
	footer := container.New(layout.NewCustomPaddedLayout(0, 0, 10, 0), text(L("Device")+" "+u.opts.Version, 11, textFaint, false))
	sideBg := canvas.NewRectangle(withAlpha(white, 0x0a))
	divider := canvas.NewRectangle(cardStroke)
	sidebar := container.NewStack(sideBg, container.New(layout.NewCustomPaddedLayout(18+titleBarInset, 18, 16, 16),
		container.NewBorder(brand, footer, nil, nil, container.NewVBox(side))))
	sidebar = container.NewBorder(nil, nil, nil, fixedWidth(1, divider), fixedWidth(228, sidebar))
	u.main = container.NewBorder(nil, nil, sidebar, nil, container.New(layout.NewCustomPaddedLayout(titleBarInset, 0, 0, 0), u.page))
	u.onboarding = newOnboardingView(u)
}

// relanguage rebuilds the window in the new language, on the same page.
func (u *ui) relanguage() {
	setLanguage(u.opts.Language)
	page := u.current
	u.build()
	u.settings.showCategory(settingsGeneral)
	u.showing = ""
	u.current = page
	u.render(u.last)
	u.show(page)
	if u.panel != nil {
		u.panel = newTrayPanel(u, u.panel.win)
	}
	if u.trayMenu != nil {
		u.trayMenu.Items[2].Label = L("Open go-link")
		u.trayCode.Label = ""
		u.renderTray(u.last)
	}
}

// show switches the section.
func (u *ui) show(i int) {
	u.current = i
	for k, r := range u.rows {
		r.SetSelected(k == i)
	}
	p := sections[i].pal
	u.bg.StartColor, u.bg.EndColor = p.top, p.bottom
	u.bg.Refresh()
	u.page.Objects = []fyne.CanvasObject{u.pages[i]}
	u.page.Refresh()
}

// showRoms opens MAME on its ROMs tab.
func (u *ui) showRoms() {
	u.show(pageMAME)
	u.mame.showTab(mameTabRoms)
}

// dropped routes files and folders dropped on the window. Zips always go
// to the ROMs; images and folders go to the thumbnails while that tab is
// open, and to the ROMs (a folder becomes the ROM folder) otherwise.
func (u *ui) dropped(uris []fyne.URI) {
	if u.current != pageMAME || u.mame.tab != mameTabThumbs {
		u.showRoms()
		u.roms.dropped(uris)
		return
	}
	var zips, rest []fyne.URI
	for _, uri := range uris {
		if strings.EqualFold(uri.Extension(), ".zip") {
			zips = append(zips, uri)
		} else {
			rest = append(rest, uri)
		}
	}
	if len(rest) > 0 {
		u.thumbs.dropped(rest)
	}
	if len(zips) > 0 {
		u.showRoms()
		u.roms.dropped(zips)
	}
}

// linked reports whether a browser is remembered. Until then the window
// only shows the pairing code, so a new host is not overwhelmed.
func (u *ui) linked(st models.Status) bool {
	return u.opts.Links == nil || st.SavedLinks > 0
}

// render paints a status snapshot. It runs on Fyne's thread.
func (u *ui) render(st models.Status) {
	u.last = st
	want := "onboarding"
	if u.linked(st) {
		want = "main"
	}
	if want != u.showing {
		u.showing = want
		if want == "main" {
			u.win.SetContent(withDragStrip(u.win.Title(), container.NewStack(u.bg, u.main)))
			u.show(u.current)
		} else {
			u.bg.StartColor, u.bg.EndColor = violet.top, violet.bottom
			u.bg.Refresh()
			u.win.SetContent(withDragStrip(u.win.Title(), container.NewStack(u.bg, u.onboarding.content)))
		}
	}
	u.onboarding.render(st)
	u.overview.render(st)
	u.mame.render(st)
	u.hd.render(st)
	u.roms.render(st)
	u.thumbs.render(st)
	u.settings.render(st)
	if u.panel != nil {
		u.panel.render(st)
	}
	u.renderTray(st)
}

// tick updates countdowns once a second.
func (u *ui) tick() {
	u.onboarding.tick()
	u.overview.tick()
	u.rememberSize()
}

// setupTray adds the tray icon: a left click toggles the status panel, a
// right click opens the menu. Closing the window only hides it.
func (u *ui) setupTray() {
	desk, ok := u.app.(desktop.App)
	if !ok {
		return
	}
	u.trayCode = fyne.NewMenuItem("", nil)
	u.trayCode.Disabled = true
	show := fyne.NewMenuItem(L("Open go-link"), u.showWindow)
	u.trayMenu = fyne.NewMenu("go-link Console", u.trayCode, fyne.NewMenuItemSeparator(), show)
	desk.SetSystemTrayMenu(u.trayMenu)
	desk.SetSystemTrayIcon(trayResource())
	u.win.SetCloseIntercept(u.win.Hide)
	if drv, ok := u.app.Driver().(desktop.Driver); ok {
		u.panel = newTrayPanel(u, drv.CreateSplashWindow())
		desk.SetSystemTrayWindow(u.panel.win)
	}
	u.renderTray(u.opts.Status.Snapshot())
}

func (u *ui) showWindow() {
	u.win.Show()
	u.win.RequestFocus()
}

func (u *ui) renderTray(st models.Status) {
	if u.trayCode == nil {
		return
	}
	label := L("Pairing code:") + " " + codeOrDashes(st.Pairing.Code)
	if u.linked(st) {
		label = linkedSummary(st)
	}
	if u.trayCode.Label != label {
		u.trayCode.Label = label
		u.trayMenu.Refresh()
	}
}

func linkedSummary(st models.Status) string {
	switch n := len(st.Peers); n {
	case 0:
		return L("Linked · no browser online")
	case 1:
		return L("Linked · 1 browser online")
	default:
		return Lf("Linked · %d browsers online", n)
	}
}

// showError reports an action that failed.
func (u *ui) showError(err error) {
	dialog.ShowError(err, u.win)
}

func (u *ui) pairingURL() string {
	return strings.TrimRight(u.opts.WebURL, "/") + "/device"
}

func codeOrDashes(code string) string {
	if code == "" {
		return "--- --- ---"
	}
	return code
}

func appIcon() fyne.Resource { return fyne.NewStaticResource("go-link.png", trayicon.AppPNG()) }

func trayResource() fyne.Resource {
	if isWindows {
		return fyne.NewStaticResource("go-link.ico", trayicon.ICO())
	}
	return fyne.NewStaticResource("go-link-tray.png", trayicon.PNG())
}

// The window's size is kept between runs (on macOS the system also keeps
// its position, see panel_darwin.m).
const (
	prefWidth  = "window.width"
	prefHeight = "window.height"
)

// defaultSize fits the sidebar, a list and a side column comfortably.
var defaultSize = fyne.NewSize(1240, 800)

// savedSize is the size the host left the window in, or the default.
func (u *ui) savedSize() fyne.Size {
	p := u.app.Preferences()
	w, h := p.FloatWithFallback(prefWidth, 0), p.FloatWithFallback(prefHeight, 0)
	if w < 900 || h < 600 {
		return defaultSize
	}
	return fyne.NewSize(float32(w), float32(h))
}

// rememberSize saves the window's size when the host changed it. It runs
// every second on Fyne's thread.
func (u *ui) rememberSize() {
	size := u.win.Canvas().Size()
	if size.Width < 900 || size.Height < 600 || size == u.lastSize {
		return
	}
	u.lastSize = size
	p := u.app.Preferences()
	p.SetFloat(prefWidth, float64(size.Width))
	p.SetFloat(prefHeight, float64(size.Height))
}
