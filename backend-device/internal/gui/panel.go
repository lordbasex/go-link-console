// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
)

// panelTitle names the borderless panel window, so the native code can
// find it (it is never shown: the window has no title bar).
const panelTitle = "go-link status"

// trayPanel is the small window a left click on the tray icon toggles,
// like the menu bar panels of MacDub and CleanMyMac.
type trayPanel struct {
	u        *ui
	win      fyne.Window
	body     *fyne.Container
	header   fyne.CanvasObject
	grid     fyne.CanvasObject
	open     fyne.CanvasObject
	withCode bool
	state    *canvas.Text
	line     *canvas.Text
	codeBox  fyne.CanvasObject
	code     *canvas.Text
	cpu      *statTile
	memory   *statTile
	network  *statTile
	stream   *statTile
	players  *statTile
	browsers *statTile
}

func newTrayPanel(u *ui, win fyne.Window) *trayPanel {
	p := &trayPanel{u: u, win: win}
	win.SetPadded(false)
	win.SetTitle(panelTitle)
	p.state = text("", 17, violet.accent, true)
	p.line = text("", 12, textMuted, false)
	p.code = text("--- --- ---", 26, white, true)
	p.code.TextStyle = fyne.TextStyle{Monospace: true}
	p.code.Alignment = fyne.TextAlignCenter

	compact := func(t *statTile) *statTile {
		t.value.TextSize = 16
		return t
	}
	p.cpu = compact(newStatTile("cpu", L("CPU"), violet, true))
	p.memory = compact(newStatTile("memory", L("Memory"), violet, true))
	p.network = compact(newStatTile("network", L("Network"), violet, false))
	p.stream = compact(newStatTile("stream", L("Streaming"), violet, false))
	p.players = compact(newStatTile("players", L("Players"), violet, false))
	p.browsers = compact(newStatTile("link", L("Browsers"), violet, false))

	title := container.NewHBox(text("go-link Console", 17, white, true), p.state)
	header := container.NewBorder(nil, nil, nil, sized(heroTileSmall(), 48, 48), container.NewVBox(title, p.line))
	p.codeBox = glass(container.NewVBox(
		text(Lf("Link at %s", u.pairingURL()), 12, textMuted, false),
		p.code,
	))
	grid := newResponsiveGrid(160, 10,
		p.cpu.content, p.memory.content,
		p.network.content, p.stream.content,
		p.players.content, p.browsers.content,
	)
	open := widget.NewButtonWithIcon(L("Open go-link"), icon("device", white), func() {
		win.Hide()
		u.showWindow()
	})
	open.Importance = widget.HighImportance
	p.header, p.grid, p.open = header, grid, open
	p.body = container.NewVBox()
	p.layout(true)
	win.SetContent(container.NewStack(background(violet), container.New(layout.NewCustomPaddedLayout(14, 14, 14, 14), p.body)))
	win.SetFixedSize(true)
	attachPanel(panelTitle, func() { fyne.Do(win.Hide) })
	return p
}

// layout shows the code card only while no browser is linked, and fits
// the window to the content.
func (p *trayPanel) layout(withCode bool) {
	if p.body.Objects != nil && withCode == p.withCode {
		return
	}
	p.withCode = withCode
	if withCode {
		p.body.Objects = []fyne.CanvasObject{p.header, p.codeBox, p.grid, p.open}
	} else {
		p.body.Objects = []fyne.CanvasObject{p.header, p.grid, p.open}
	}
	p.body.Layout = layout.NewCustomPaddedVBoxLayout(10)
	p.body.Refresh()
	p.win.Resize(fyne.NewSize(380, p.body.MinSize().Height+28))
}

// heroTileSmall is the panel's corner tile.
func heroTileSmall() fyne.CanvasObject {
	tile := canvas.NewImageFromImage(tileImage(violet, 96))
	tile.SetMinSize(fyne.NewSize(48, 48))
	return container.NewStack(tile, container.NewCenter(iconImage("joystick", white, 24)))
}

func (p *trayPanel) render(st models.Status) {
	linked := p.u.linked(st)
	switch {
	case st.Signal.State != models.SignalConnected:
		p.state.Color, p.state.Text = colorDanger, L("Offline")
	case !linked:
		p.state.Color, p.state.Text = colorWarn, L("Waiting to link")
	case roomsSummary(st).seated > 0:
		p.state.Color, p.state.Text = colorOK, L("Playing")
	default:
		p.state.Color, p.state.Text = colorOK, L("Ready")
	}
	p.state.Refresh()
	sum := roomsSummary(st)
	setText(p.line, Lf("%d live · %d paused", sum.live, sum.paused))
	setText(p.code, codeOrDashes(st.Pairing.Code))
	p.layout(!linked)

	if sys := st.System; sys != nil {
		u, hw := sys.Usage, sys.Hardware
		p.cpu.Set(fmt.Sprintf("%.0f %%", u.CPUPercent), Lf("go-link %.0f %%", u.ProcessCPUPercent))
		p.cpu.meter.SetValue(u.CPUPercent/100, loadColor(u.CPUPercent/100))
		if hw.MemTotal > 0 {
			frac := float64(u.MemUsed) / float64(hw.MemTotal)
			p.memory.Set(fmt.Sprintf("%.0f %%", frac*100), Lf("%s used", formatBytes(u.MemUsed)))
			p.memory.meter.SetValue(frac, loadColor(frac))
		}
		p.network.Set(formatRate(u.NetSentBps)+" "+L("sent"), formatRate(u.NetRecvBps)+" "+L("received"))
	}
	if s := st.Stream; s.VideoViewers > 0 {
		p.stream.Set(fmt.Sprintf("%.0f fps", s.FPS), fmt.Sprintf("%.0f kbps", s.VideoKbps))
	} else {
		p.stream.Set(L("Idle"), L("no one watching"))
	}
	p.players.Set(fmt.Sprintf("%d", sum.seated), Lf("%d watching or in queue", sum.watching))
	p.browsers.Set(fmt.Sprintf("%d online", len(st.Peers)), fmt.Sprintf("%d remembered", st.SavedLinks))
}
