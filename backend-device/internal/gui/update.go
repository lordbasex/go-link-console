// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
	"github.com/lordbasex/go-link-console/backend-device/internal/updater"
)

// updates says in a window of its own that a newer go-link Console is out
// (found by the device's check every 6 hours, or by Check for Updates…)
// and installs it: the download is checked against the release's signed
// list, the new version takes this one's place and opens. Its fields are
// only touched on Fyne's thread, except those under mu.
type updates struct {
	u  *ui
	up *updater.Updater
	// check asks GitHub now (the device's UpdateService; a test's stand-in).
	check func(context.Context) (models.UpdateInfo, error)
	// background runs the check off the UI thread (tests run it inline).
	background func(func())

	mu      sync.Mutex
	known   models.UpdateInfo // the newest release found (Latest "" when none)
	shown   string            // the release whose window opened by itself
	pending bool              // found while someone played: shown when they stop
	busy    bool              // downloading or installing

	win         fyne.Window
	title, text *widget.Label
	bar         *widget.ProgressBar
	spin        *widget.ProgressBarInfinite
	buttons     *fyne.Container
	trayItem    *fyne.MenuItem
}

// checkTimeout bounds Check for Updates….
const checkTimeout = 20 * time.Second

func (u *ui) newUpdates() *updates {
	p := &updates{u: u, up: updater.New(), background: func(f func()) { go f() }}
	if s := u.opts.Updates; s != nil {
		p.check = s.Check
	}
	return p
}

// found: the device saw a newer release (Status.Update). Its window opens
// by itself once, never while someone plays (then when nobody does).
func (p *updates) found(info models.UpdateInfo) {
	if info.Latest == "" {
		return
	}
	p.mu.Lock()
	p.known = info
	show := info.Latest != p.shown && !p.busy
	if show {
		p.shown = info.Latest
	}
	if show && playing(p.u.opts.Status.Snapshot()) {
		p.pending, show = true, false
	}
	p.mu.Unlock()
	p.renderTray()
	if show {
		p.showAvailable()
	}
}

// playEnded shows an update found while someone played.
func (p *updates) playEnded() {
	p.mu.Lock()
	show := p.pending && p.known.Latest != "" && !p.busy
	p.pending = false
	p.mu.Unlock()
	if show {
		p.showAvailable()
	}
}

// playing: someone holds a seat in a room, so an update waits.
func playing(st models.Status) bool { return roomsSummary(st).seated > 0 }

// ask is Check for Updates…: it says what it found either way.
func (p *updates) ask() {
	p.mu.Lock()
	busy, known := p.busy, p.known
	p.mu.Unlock()
	if busy {
		p.window().Show()
		p.win.RequestFocus()
		return
	}
	if known.Latest != "" {
		p.showAvailable()
		return
	}
	p.render(L("Checking for updates…"), "", true)
	p.setButtons(widget.NewButton(L("Close"), p.win.Hide))
	check := p.check
	p.background(func() {
		ctx, cancel := context.WithTimeout(p.u.opts.Ctx, checkTimeout)
		defer cancel()
		info, err := models.UpdateInfo{}, errors.New("no update check")
		if check != nil {
			info, err = check(ctx)
		}
		fyne.Do(func() {
			switch {
			case err != nil:
				p.render(L("Could not check for updates"), L("There is no connection to GitHub, where the versions are. Try again later."), false)
				p.setButtons(widget.NewButton(L("Close"), p.win.Hide))
			case info.Latest == "":
				p.render(L("go-link Console is up to date"), Lf("You have %s, the newest version.", bare(p.u.opts.Version)), false)
				p.setButtons(widget.NewButton(L("OK"), p.win.Hide))
			default:
				p.mu.Lock()
				p.known, p.shown = info, info.Latest
				p.mu.Unlock()
				// the website's notice and the Overview's button too
				p.u.opts.Status.SetUpdate(&info)
				p.renderTray()
				p.showAvailable()
			}
		})
	})
}

// canInstall: this copy replaces itself (a release unpacked or installed
// as published, not a development build).
func (p *updates) canInstall() bool {
	return isRelease(p.u.opts.Version) && p.up.CanInstall()
}

// showAvailable shows the newest release with Update.
func (p *updates) showAvailable() {
	p.mu.Lock()
	info := p.known
	p.mu.Unlock()
	if info.Latest == "" {
		return
	}
	text := Lf("You have %s.", bare(p.u.opts.Version))
	switch {
	case !p.canInstall():
		text += " " + L("This copy cannot replace itself: download the new one from its page and install it as before.")
	case playing(p.u.opts.Status.Snapshot()):
		text += " " + L("Updating closes the rooms for a moment: each game is saved and comes back where it was.")
	}
	p.render(Lf("go-link Console %s is available", bare(info.Latest)), text, false)
	news := widget.NewButton(L("What's new"), func() { p.openPage(info.URL) })
	later := widget.NewButton(L("Later"), p.win.Hide)
	if !p.canInstall() {
		get := widget.NewButton(L("Download"), func() { p.openPage(info.URL); p.win.Hide() })
		get.Importance = widget.HighImportance
		p.setButtons(news, later, get)
		return
	}
	update := widget.NewButton(L("Update"), func() { p.install(info) })
	update.Importance = widget.HighImportance
	p.setButtons(news, later, update)
}

// install downloads, checks and installs the release, and quits for the
// new version to open (the device saves every running game first).
func (p *updates) install(info models.UpdateInfo) {
	ctx, cancel := context.WithCancel(p.u.opts.Ctx)
	p.mu.Lock()
	p.busy = true
	p.mu.Unlock()
	p.render(Lf("Downloading go-link Console %s…", bare(info.Latest)), "", false)
	p.bar.Show()
	p.bar.SetValue(0)
	p.setButtons(widget.NewButton(L("Cancel"), cancel))
	done := func() {
		p.mu.Lock()
		p.busy = false
		p.mu.Unlock()
		cancel()
	}
	go func() {
		path, err := p.up.Download(ctx, info.Latest, func(n, total int64) {
			if total > 0 {
				fyne.Do(func() { p.bar.SetValue(float64(n) / float64(total)) })
			}
		})
		if err != nil {
			done()
			fyne.Do(func() { p.failed(info, err) })
			return
		}
		fyne.Do(func() {
			p.render(L("Installing…"), L("go-link Console opens again by itself."), true)
			p.setButtons()
		})
		restart, err := p.up.Install(ctx, path)
		if err != nil {
			done()
			fyne.Do(func() { p.failed(info, err) })
			return
		}
		p.u.opts.Logger.Info("update installed", "from", p.u.opts.Version, "to", info.Latest)
		if restart != nil {
			if err := restart(); err != nil {
				p.u.opts.Logger.Warn("cannot open the new version", "err", err)
			}
		}
		if p.u.opts.Quit != nil {
			p.u.opts.Quit()
		} else {
			fyne.Do(p.u.app.Quit)
		}
	}()
}

// failed says why the update did not go in, with the release's page.
func (p *updates) failed(info models.UpdateInfo, err error) {
	switch {
	case errors.Is(err, context.Canceled):
		p.showAvailable()
		return
	case errors.Is(err, updater.ErrSignature):
		p.render(L("The update was not installed"), L("The download is not the one go-link Console published (its signature or its contents differ), so it was thrown away."), false)
	case errors.Is(err, updater.ErrManual):
		p.render(L("The update was not installed"), L("This copy cannot replace itself: download the new one from its page and install it as before."), false)
	default:
		p.render(L("The update was not installed"), L("Something went wrong while downloading or installing it. Try again, or download it from its page."), false)
	}
	p.u.opts.Logger.Warn("update failed", "to", info.Latest, "err", err)
	page := widget.NewButton(L("Open its page"), func() { p.openPage(info.URL); p.win.Hide() })
	// Close where Update was: a second click only closes.
	p.setButtons(page, widget.NewButton(L("Close"), p.win.Hide))
}

// window builds the update window once.
func (p *updates) window() fyne.Window {
	if p.win != nil {
		return p.win
	}
	w := p.u.app.NewWindow(L("Software update"))
	p.title = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	p.text = widget.NewLabel("")
	p.text.Wrapping = fyne.TextWrapWord
	p.bar = widget.NewProgressBar()
	p.spin = widget.NewProgressBarInfinite()
	p.buttons = container.NewHBox()
	w.SetContent(container.NewPadded(container.NewVBox(p.title, p.text, p.bar, p.spin, container.NewHBox(layout.NewSpacer(), p.buttons))))
	w.Resize(fyne.NewSize(460, 0))
	w.SetFixedSize(true)
	w.SetCloseIntercept(func() {
		p.mu.Lock()
		busy := p.busy
		p.mu.Unlock()
		if !busy {
			w.Hide()
		}
	})
	p.win = w
	return w
}

// render puts a state in the window and shows it.
func (p *updates) render(title, text string, spinning bool) {
	w := p.window()
	p.title.SetText(title)
	p.text.SetText(text)
	if text == "" {
		p.text.Hide()
	} else {
		p.text.Show()
	}
	p.bar.Hide()
	if spinning {
		p.spin.Show()
		p.spin.Start()
	} else {
		p.spin.Stop()
		p.spin.Hide()
	}
	w.Show()
	w.CenterOnScreen()
	w.RequestFocus()
}

func (p *updates) setButtons(b ...fyne.CanvasObject) {
	p.buttons.Objects = b
	p.buttons.Refresh()
}

func (p *updates) openPage(link string) {
	if u, err := url.Parse(link); err == nil && u.Scheme == "https" {
		_ = p.u.app.OpenURL(u)
	}
}

// trayLabel is the tray's item: "Update to 0.4.3…" once one is known.
func (p *updates) trayLabel() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.known.Latest != "" {
		return Lf("Update to %s…", bare(p.known.Latest))
	}
	return L("Check for Updates…")
}

func (p *updates) renderTray() {
	if p.trayItem == nil {
		return
	}
	if label := p.trayLabel(); p.trayItem.Label != label {
		p.trayItem.Label = label
		p.u.trayMenu.Refresh()
	}
}

// relanguage puts the window's texts in the new language.
func (p *updates) relanguage() {
	if p.win != nil {
		p.win.SetTitle(L("Software update"))
		p.win.Hide()
	}
	p.renderTray()
}

// bare is a version without its v: 0.4.3.
func bare(v string) string { return strings.TrimPrefix(v, "v") }

// isRelease: a version a release has (v0.4.3), not a development build's.
func isRelease(v string) bool {
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 || !strings.HasPrefix(v, "v") {
		return false
	}
	for _, s := range parts {
		if s == "" || strings.Trim(s, "0123456789") != "" {
			return false
		}
	}
	return true
}
