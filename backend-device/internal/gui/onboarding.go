// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"net/url"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
)

// onboardingView is all a new host sees: the pairing code and where to
// type it. Once a browser is linked the window shows everything else.
type onboardingView struct {
	u           *ui
	content     fyne.CanvasObject
	digits      []*canvas.Text
	refresh     *canvas.Text
	dot         *canvas.Circle
	signal      *canvas.Text
	refreshesAt *time.Time
	code        string
	copyCode    *widget.Button
	copyReset   *time.Timer // puts "Copy code" back after "Copied"
}

func newOnboardingView(u *ui) *onboardingView {
	v := &onboardingView{u: u}
	v.refresh = text("", 12, textFaint, false)
	v.dot = statusDot(textFaint)
	v.signal = text("", 12, textMuted, false)

	// Step 1: where.
	address := text(strings.TrimPrefix(u.pairingURL(), "https://"), 15, violet.accent, true)
	copyURL := widget.NewButtonWithIcon(L("Copy"), icon("copy", white), func() {
		u.app.Clipboard().SetContent(u.pairingURL())
	})
	step1 := container.NewBorder(nil, nil, stepBadge("1"), copyURL,
		container.NewVBox(text(L("Open this address in your browser"), 14, white, true), address))

	// Step 2: the code, one box per digit like the website.
	groups := make([]fyne.CanvasObject, 0, 3)
	for g := 0; g < 3; g++ {
		boxes := make([]fyne.CanvasObject, 0, 3)
		for k := 0; k < 3; k++ {
			d := text("-", 30, white, true)
			d.TextStyle = fyne.TextStyle{Monospace: true}
			d.Alignment = fyne.TextAlignCenter
			v.digits = append(v.digits, d)
			bg := canvas.NewRectangle(withAlpha(white, 0x1c))
			bg.CornerRadius = 10
			bg.StrokeColor = withAlpha(white, 0x2e)
			bg.StrokeWidth = 1
			boxes = append(boxes, sized(container.NewStack(bg, container.NewCenter(d)), 44, 56))
		}
		groups = append(groups, container.New(layout.NewCustomPaddedHBoxLayout(6), boxes...))
	}
	code := container.NewCenter(container.New(layout.NewCustomPaddedHBoxLayout(18), groups...))
	step2 := container.NewBorder(nil, nil, stepBadge("2"), nil,
		container.NewVBox(text(L("Type this code"), 14, white, true), text(L("It works once. After that, the browser comes back on its own."), 12, textMuted, false)))
	// Copies the nine digits: the website's code field accepts a paste.
	v.copyCode = widget.NewButtonWithIcon(L("Copy code"), icon("copy", white), func() {
		if v.code == "" {
			return
		}
		u.app.Clipboard().SetContent(v.code)
		v.copyCode.SetText(L("Copied"))
		if v.copyReset != nil {
			v.copyReset.Stop()
		}
		v.copyReset = time.AfterFunc(2*time.Second, func() { fyne.Do(func() { v.copyCode.SetText(L("Copy code")) }) })
	})
	// The countdown on the left, the button on the right with a fixed
	// width: they never overlap, and neither moves when the label changes.
	footer := container.NewBorder(nil, nil, container.NewCenter(v.refresh), fixedWidth(132, v.copyCode))
	card := glassPadded(container.New(layout.NewCustomPaddedVBoxLayout(14),
		step1,
		widget.NewSeparator(),
		step2,
		code,
		footer,
	), 20, 22)

	intro := container.New(layout.NewCustomPaddedVBoxLayout(2),
		text(L("Manage this device from any browser: pick games, open rooms,"), 15, textMuted, false),
		text(L("add ROMs and see who is playing. Link it once and it remembers."), 15, textMuted, false),
	)
	right := container.New(layout.NewCustomPaddedVBoxLayout(18),
		text(L("Link this device"), 34, white, true),
		intro,
		card,
	)
	status := container.NewCenter(container.NewHBox(container.NewCenter(sized(v.dot, 8, 8)), v.signal))
	hero := container.New(layout.NewCustomPaddedVBoxLayout(4), heroTile(violet, "link", 200), status)
	row := container.New(layout.NewCustomPaddedHBoxLayout(44), container.NewCenter(hero), fixedWidth(500, right))

	open := newCapsuleButton(L("Open in browser"), "link", violet, 240, func() {
		if link, err := url.Parse(u.pairingURL()); err == nil {
			_ = u.app.OpenURL(link)
		}
	})
	body := container.NewBorder(nil, container.New(layout.NewCustomPaddedLayout(0, 40, 0, 0), container.NewCenter(open)), nil, nil,
		container.NewCenter(row))
	v.content = container.New(layout.NewCustomPaddedLayout(titleBarInset+24, 0, 40, 40), body)
	return v
}

// stepBadge is a small numbered circle.
func stepBadge(n string) fyne.CanvasObject {
	c := canvas.NewCircle(withAlpha(violet.accent, 0xd0))
	t := text(n, 13, white, true)
	t.Alignment = fyne.TextAlignCenter
	return container.New(layout.NewCustomPaddedLayout(2, 0, 0, 12), sized(container.NewStack(c, container.NewCenter(t)), 26, 26))
}

func (v *onboardingView) render(st models.Status) {
	code := strings.ReplaceAll(st.Pairing.Code, " ", "")
	v.code = code
	if code == "" {
		v.copyCode.Disable()
	} else {
		v.copyCode.Enable()
	}
	for i, d := range v.digits {
		s := "-"
		if i < len(code) {
			s = code[i : i+1]
		}
		setText(d, s)
	}
	v.refreshesAt = st.Pairing.RefreshesAt
	v.tick()
	setSignal(v.dot, v.signal, st)
}

func (v *onboardingView) tick() {
	s := ""
	if v.refreshesAt != nil {
		s = Lf("A new code in %s", countdown(time.Until(*v.refreshesAt)))
	}
	setText(v.refresh, s)
}

// setSignal paints the signaling state as a dot and a line.
func setSignal(dot *canvas.Circle, line *canvas.Text, st models.Status) {
	switch st.Signal.State {
	case models.SignalConnected:
		dot.FillColor = colorOK
		line.Text = L("Connected to the signaling server")
	case models.SignalConnecting:
		dot.FillColor = colorWarn
		line.Text = L("Connecting to the signaling server…")
	default:
		dot.FillColor = colorDanger
		line.Text = L("No connection to the signaling server")
		if st.Signal.Error != "" {
			line.Text += ": " + st.Signal.Error
		}
	}
	dot.Refresh()
	line.Refresh()
}
