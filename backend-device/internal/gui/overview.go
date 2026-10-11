// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"fmt"
	"image/color"
	"net/url"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
	"github.com/lordbasex/go-link-console/backend-device/pkg/trayicon"
)

// overviewPage is the home: what the device is doing right now, and the
// computer it runs on (usage is only shown here, never twice).
type overviewPage struct {
	u        *ui
	content  fyne.CanvasObject
	dot      *canvas.Circle
	signal   *canvas.Text
	rooms    *canvas.Text
	watching *canvas.Text
	linked   *canvas.Text
	cpu      *statTile
	memory   *statTile
	network  *statTile
	stream   *statTile
	players  *statTile
	latency  *statTile
	action   *roundButton
	update   *widget.Button
	updateTo string

	platform *widget.Label
	cpuModel *widget.Label
	ram      *widget.Label
	deviceID *widget.Label
	version  *widget.Label
	web      *widget.Label

	codeDialog  dialog.Dialog
	dialogCode  *canvas.Text
	refreshesAt *time.Time
}

// logo is the app's own mark (the website's logo), for the hero tile and
// the sidebar.
func logo(size float32) fyne.CanvasObject {
	img := canvas.NewImageFromResource(fyne.NewStaticResource("go-link-logo.png", trayicon.Render(256, false)))
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(size, size))
	return img
}

func newOverviewPage(u *ui) *overviewPage {
	p := &overviewPage{u: u}
	p.dot = statusDot(textFaint)
	p.signal = text("", 12, textMuted, false)
	p.rooms = text("", 14, white, true)
	p.watching = text("", 14, white, true)
	p.linked = text("", 14, white, true)
	row := func(label string, value *canvas.Text) fyne.CanvasObject {
		return container.NewBorder(nil, nil, text(label, 14, textMuted, false), value)
	}
	info := glass(container.NewVBox(
		row(L("Rooms"), p.rooms),
		widget.NewSeparator(),
		row(L("People playing"), p.watching),
		widget.NewSeparator(),
		row(L("Linked browsers"), p.linked),
	))
	intro := wrapped(L("Your arcade, streamed. Guests play in their browser with no ROMs: the game runs here and only video, sound and controls travel."))
	header := newAdaptiveRow(640, container.NewCenter(logo(150)),
		container.NewVBox(text("go-link Console", 30, white, true), intro, info))

	p.cpu = newStatTile("cpu", L("CPU"), violet, true)
	p.memory = newStatTile("memory", L("Memory"), violet, true)
	p.network = newStatTile("network", L("Network"), violet, false)
	p.stream = newStatTile("stream", L("Streaming"), violet, false)
	p.players = newStatTile("players", L("Players"), violet, false)
	p.latency = newStatTile("latency", L("Browser latency"), violet, false)
	tiles := newResponsiveGrid(230, 12,
		p.cpu.content, p.memory.content, p.network.content,
		p.stream.content, p.players.content, p.latency.content,
	)

	mono := func() *widget.Label {
		l := widget.NewLabel("")
		l.TextStyle.Monospace = true
		l.Wrapping = fyne.TextWrapBreak
		return l
	}
	p.platform, p.cpuModel, p.ram = widget.NewLabel(""), widget.NewLabel(""), widget.NewLabel("")
	p.cpuModel.Wrapping = fyne.TextWrapWord
	p.deviceID, p.version, p.web = mono(), widget.NewLabel(""), mono()
	p.web.SetText(u.pairingURL())
	computer := glass(container.NewVBox(
		text(L("This computer"), 16, white, true),
		wrapped(L("Hardware data never goes to room guests: only to browsers linked with the code.")),
		widget.NewForm(
			widget.NewFormItem(L("System"), p.platform),
			widget.NewFormItem(L("Processor"), p.cpuModel),
			widget.NewFormItem(L("Memory"), p.ram),
			widget.NewFormItem(L("Device ID"), p.deviceID),
			widget.NewFormItem(L("Version"), p.version),
			widget.NewFormItem(L("Website"), p.web),
		),
	))

	p.action = newRoundButton(L("Open go-link"), "play", violet, p.openWebsite)
	another := widget.NewButtonWithIcon(L("Link another browser"), icon("link", white), p.showCode)
	unlink := widget.NewButtonWithIcon(L("Unlink all"), icon("unlink", white), p.confirmUnlink)
	if u.opts.Links == nil {
		another.Hide()
		unlink.Hide()
	}
	p.update = widget.NewButtonWithIcon("", icon("download", white), func() {
		if link, err := url.Parse(p.updateTo); err == nil && p.updateTo != "" {
			_ = u.app.OpenURL(link)
		}
	})
	p.update.Importance = widget.HighImportance
	p.update.Hide()
	actions := container.NewVBox(
		container.NewCenter(p.update),
		container.NewCenter(p.action),
		container.NewCenter(container.New(layout.NewCustomPaddedHBoxLayout(10), another, unlink)),
	)
	body := vstack(header, container.NewVBox(container.NewHBox(sized(p.dot, 8, 8), p.signal), tiles), actions, computer)
	p.content = container.NewVScroll(container.New(layout.NewCustomPaddedLayout(28, 24, 32, 32), body))
	return p
}

func (p *overviewPage) render(st models.Status) {
	setSignal(p.dot, p.signal, st)
	sum := roomsSummary(st)
	live, paused, seated, watching := sum.live, sum.paused, sum.seated, sum.watching
	setText(p.rooms, Lf("%d live · %d paused", live, paused))
	setText(p.watching, Lf("%d playing · %d watching", seated, watching))
	setText(p.linked, Lf("%d online · %d remembered", len(st.Peers), st.SavedLinks))

	if sys := st.System; sys != nil {
		u, hw := sys.Usage, sys.Hardware
		p.cpu.Set(fmt.Sprintf("%.0f %%", u.CPUPercent), Lf("go-link %.0f %% of one core", u.ProcessCPUPercent))
		p.cpu.meter.SetValue(u.CPUPercent/100, loadColor(u.CPUPercent/100))
		if hw.MemTotal > 0 {
			frac := float64(u.MemUsed) / float64(hw.MemTotal)
			p.memory.Set(fmt.Sprintf("%.0f %%", frac*100), Lf("%s of %s · go-link %s", formatBytes(u.MemUsed), formatBytes(hw.MemTotal), formatBytes(u.ProcessRSS)))
			p.memory.meter.SetValue(frac, loadColor(frac))
		}
		p.network.Set(formatRate(u.NetSentBps)+" "+L("sent"), formatRate(u.NetRecvBps)+" "+L("received · whole computer"))
		p.platform.SetText(fmt.Sprintf("%s · %s", hw.Platform, hw.Arch))
		p.cpuModel.SetText(Lf("%s · %d cores", hw.CPUModel, hw.Cores))
		p.ram.SetText(formatBytes(hw.MemTotal))
	} else {
		p.platform.SetText(L("Measuring…"))
	}
	if s := st.Stream; s.VideoViewers > 0 {
		p.stream.Set(fmt.Sprintf("%.0f fps", s.FPS), fmt.Sprintf("%.0f kbps · %d×%d · VP8", s.VideoKbps, s.Width, s.Height))
	} else {
		p.stream.Set(L("Idle"), L("Nothing is encoded until someone watches"))
	}
	p.players.Set(fmt.Sprintf("%d", seated), Lf("%d watching or in queue", watching))
	best := -1
	for _, peer := range st.Peers {
		if peer.LatencyMs != nil && (best < 0 || *peer.LatencyMs < best) {
			best = *peer.LatencyMs
		}
	}
	if best >= 0 {
		p.latency.Set(fmt.Sprintf("%d ms", best), L("Round trip over WebRTC"))
	} else {
		p.latency.Set("—", L("No browser online"))
	}

	if st.Update != nil && st.Update.Latest != "" {
		p.updateTo = st.Update.URL
		p.update.SetText(Lf("go-link %s is available: download it", st.Update.Latest))
		p.update.Show()
	} else {
		p.update.Hide()
	}
	p.deviceID.SetText(st.DeviceID)
	p.version.SetText(st.Version)
	p.refreshesAt = st.Pairing.RefreshesAt
	if p.dialogCode != nil {
		setText(p.dialogCode, codeOrDashes(st.Pairing.Code))
	}
}

func (p *overviewPage) tick() {}

// rooms is what every running room adds up to.
type rooms struct{ live, paused, seated, watching int }

// roomsSummary counts the game rooms and the people in them (the test
// pattern room included).
func roomsSummary(st models.Status) rooms {
	var r rooms
	for _, m := range st.Rooms {
		switch m.State {
		case models.RoomLive:
			r.live++
		case models.RoomPaused:
			r.paused++
		default:
			continue
		}
		r.seated += m.Players
		r.watching += m.Spectators + m.Queue
	}
	if t := st.Room; t != nil {
		r.seated += t.Players
		r.watching += t.Spectators + t.Queue
	}
	return r
}

// openWebsite opens the rooms on the website, where games are started.
func (p *overviewPage) openWebsite() {
	web := strings.TrimSuffix(p.u.opts.WebURL, "/") + "/rooms"
	if link, err := url.Parse(web); err == nil {
		_ = p.u.app.OpenURL(link)
	}
}

// showCode shows the pairing code to link one more browser.
func (p *overviewPage) showCode() {
	p.dialogCode = text(codeOrDashes(p.u.last.Pairing.Code), 40, white, true)
	p.dialogCode.TextStyle = fyne.TextStyle{Monospace: true}
	p.dialogCode.Alignment = fyne.TextAlignCenter
	msg := wrapped(Lf("On the other browser, open %s and type this code. Each code works once.", p.u.pairingURL()))
	p.codeDialog = dialog.NewCustom(L("Link another browser"), L("Close"), container.NewVBox(msg, p.dialogCode), p.u.win)
	p.codeDialog.SetOnClosed(func() { p.dialogCode = nil })
	p.codeDialog.Resize(fyne.NewSize(460, 220))
	p.codeDialog.Show()
}

func (p *overviewPage) confirmUnlink() {
	dialog.ShowConfirm(L("Unlink all browsers?"),
		L("Every linked browser is disconnected and will need a new code. The window goes back to the pairing code."),
		func(ok bool) {
			if ok && p.u.opts.Links != nil {
				p.u.opts.Links.UnlinkAll()
			}
		}, p.u.win)
}

func setText(t *canvas.Text, s string) {
	if t.Text != s {
		t.Text = s
		t.Refresh()
	}
}

// loadColor is green, then yellow, then red, like a level meter.
func loadColor(frac float64) color.Color {
	switch {
	case frac > 0.85:
		return colorDanger
	case frac > 0.6:
		return colorWarn
	default:
		return colorOK
	}
}
