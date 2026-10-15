// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"fyne.io/fyne/v2/widget"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
	"github.com/lordbasex/go-link-console/backend-device/internal/updater"
)

var testRelease = models.UpdateInfo{Latest: "v0.4.3", URL: "https://github.com/lordbasex/go-link-console/releases/tag/v0.4.3"}

// A release the device found opens its window once; while someone plays
// it waits until nobody does.
func TestAnUpdateShowsOnceAndNotWhileSomeonePlays(t *testing.T) {
	u, st, _ := newTestUI(t)
	u.opts.Version = "v0.4.2"
	st.SetRooms([]models.ManagedRoom{{SavedRoom: models.SavedRoom{ID: "r1", State: models.RoomLive}, Players: 2}})
	u.render(st.Snapshot())
	st.SetUpdate(&testRelease)
	u.render(st.Snapshot())
	if u.upd.win != nil && u.upd.title.Text != "" {
		t.Fatalf("shown while two played: %q", u.upd.title.Text)
	}
	if got := u.upd.trayLabel(); got != "Update to 0.4.3…" {
		t.Errorf("tray %q", got)
	}
	if !u.overview.update.Visible() || u.overview.update.Text != "Update to go-link Console 0.4.3…" {
		t.Errorf("overview button %q", u.overview.update.Text)
	}

	st.SetRooms([]models.ManagedRoom{{SavedRoom: models.SavedRoom{ID: "r1", State: models.RoomLive}}})
	u.render(st.Snapshot())
	if u.upd.win == nil || u.upd.title.Text != "go-link Console 0.4.3 is available" {
		t.Fatal("not shown when nobody played any more")
	}
	if !strings.Contains(u.upd.text.Text, "0.4.2") {
		t.Errorf("text %q", u.upd.text.Text)
	}

	u.upd.win.Hide()
	u.upd.title.SetText("")
	u.render(st.Snapshot()) // the next check: the same release
	if u.upd.title.Text != "" {
		t.Fatal("the same release opened its window again")
	}
	u.upd.ask() // asked for: it shows
	if u.upd.title.Text != "go-link Console 0.4.3 is available" {
		t.Fatalf("Check for Updates… showed %q", u.upd.title.Text)
	}
}

// A development build is never replaced: it offers the download.
func TestADevelopmentBuildDownloadsByHand(t *testing.T) {
	u, _, _ := newTestUI(t)
	u.opts.Version = "v0.4.2-3-gabcdef0"
	u.upd.known = testRelease
	u.upd.showAvailable()
	last := u.upd.buttons.Objects[len(u.upd.buttons.Objects)-1].(*widget.Button)
	if last.Text != "Download" {
		t.Fatalf("the last button is %q", last.Text)
	}
	for v, want := range map[string]bool{"v0.4.2": true, "v10.0.1": true, "0.4.2": false, "v0.4.2-dirty": false, "test": false, "v0.4": false} {
		if isRelease(v) != want {
			t.Errorf("isRelease(%q) = %v", v, !want)
		}
	}
}

// Check for Updates… says when there is nothing newer, or no connection.
func TestCheckForUpdatesSaysWhatItFound(t *testing.T) {
	u, st, _ := newTestUI(t)
	u.opts.Version = "v0.4.2"
	u.upd.background = func(f func()) { f() }
	for _, c := range []struct {
		info  models.UpdateInfo
		err   error
		title string
	}{
		{models.UpdateInfo{}, nil, "go-link Console is up to date"},
		{models.UpdateInfo{}, errors.New("offline"), "Could not check for updates"},
		{testRelease, nil, "go-link Console 0.4.3 is available"},
	} {
		u.upd.check = func(context.Context) (models.UpdateInfo, error) { return c.info, c.err }
		u.upd.ask()
		if u.upd.title.Text != c.title {
			t.Errorf("%v %v: %q", c.info, c.err, u.upd.title.Text)
		}
	}
	if got := st.Snapshot().Update; got == nil || got.Latest != "v0.4.3" {
		t.Errorf("the status has %v", got)
	}
}

// A refused download says why and leaves Close where Update was.
func TestAFailedUpdateSaysWhy(t *testing.T) {
	u, _, _ := newTestUI(t)
	u.opts.Version = "v0.4.2"
	u.upd.known = testRelease
	u.upd.failed(testRelease, fmt.Errorf("%w: test", updater.ErrSignature))
	if u.upd.title.Text != "The update was not installed" || !strings.Contains(u.upd.text.Text, "signature") {
		t.Fatalf("%q: %q", u.upd.title.Text, u.upd.text.Text)
	}
	last := u.upd.buttons.Objects[len(u.upd.buttons.Objects)-1].(*widget.Button)
	if last.Text != "Close" {
		t.Fatalf("the last button is %q", last.Text)
	}
	u.upd.failed(testRelease, context.Canceled) // Cancel while downloading
	if u.upd.title.Text != "go-link Console 0.4.3 is available" {
		t.Fatalf("after a cancel: %q", u.upd.title.Text)
	}
}

// Settings › General turns the automatic check off and on.
func TestSettingsTurnTheUpdateCheckOff(t *testing.T) {
	u, _, _ := newTestUI(t)
	var saved []bool
	u.opts.AutoUpdate = true
	u.opts.SetAutoUpdate = func(on bool) error { saved = append(saved, on); return nil }
	u.settings = newSettingsPage(u)
	u.settings.generalCards()
	u.settings.autoUp.Enable()
	if !u.settings.autoUp.Checked {
		t.Fatal("not checked")
	}
	u.settings.autoUp.SetChecked(false)
	if len(saved) != 1 || saved[0] || u.opts.AutoUpdate {
		t.Fatalf("saved %v", saved)
	}
}
