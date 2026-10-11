// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package inputhud draws the players' controllers over a game's frames
// (I420), like the test card's gamepad: one cell per seat along the bottom
// edge with a small pad that lights up with what the device received, and
// a beacon, a white square lit while that seat presses anything. A browser
// that knows where its seat's beacon is watches the video for it and
// measures the time from its own press to the picture: the whole way
// there and back (input, game, encoder, network, decoder).
package inputhud

import (
	"github.com/lordbasex/go-link-console/backend-device/pkg/input"
)

// Rect is a part of the frame in fractions of its size (0 to 1).
type Rect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

type box struct{ x, y, w, h int }

// stickActive is how far a stick moves before its seat's beacon lights.
const stickActive = 64

// Active reports whether a pad presses anything: a button or a stick past
// half way. The beacon shows it.
func Active(p input.Pad) bool {
	if p.Buttons != 0 {
		return true
	}
	for _, a := range p.Axes {
		if a >= stickActive || a <= -stickActive {
			return true
		}
	}
	return false
}

// layout is where the cells of n seats go on a w x h frame: side by side
// along the bottom, each about a seventh of the frame high and never wider
// than the frame allows.
func layout(w, h, n int) (cells []box, unit int) {
	if n < 1 || w < 16 || h < 16 {
		return nil, 0
	}
	ch := max(h/7, 16)
	cw := ch * 2
	if total := cw*n + (n+1)*2; total > w {
		cw = (w - (n+1)*2) / n
		ch = min(ch, cw/2)
	}
	unit = max(ch/8, 2)
	y := h - ch - 2
	for i := 0; i < n; i++ {
		cells = append(cells, box{2 + i*(cw+2), y, cw, ch})
	}
	return cells, unit
}

// beaconOf is the beacon's square inside a cell: on its left, below the
// seat's label.
func beaconOf(c box, unit int) box {
	s := c.h - 4*unit
	return box{c.x + unit, c.y + 3*unit, s, s}
}

// Beacons returns where each of n seats' beacons is on a w x h frame, in
// fractions of the frame, so the browser finds them at any size the video
// is sent or shown.
func Beacons(w, h, n int) []Rect {
	cells, unit := layout(w, h, n)
	out := make([]Rect, 0, len(cells))
	for _, c := range cells {
		b := beaconOf(c, unit)
		out = append(out, Rect{float64(b.x) / float64(w), float64(b.y) / float64(h), float64(b.w) / float64(w), float64(b.h) / float64(h)})
	}
	return out
}

// Draw paints the cells of len(pads) seats on an I420 frame of w x h, in
// place.
func Draw(i420 []byte, w, h int, pads []input.Pad) {
	if len(i420) < w*h+2*((w+1)/2)*((h+1)/2) {
		return
	}
	cells, unit := layout(w, h, len(pads))
	f := frame{buf: i420, w: w, h: h, cw: (w + 1) / 2, ch: (h + 1) / 2}
	for i, c := range cells {
		f.cell(c, unit, i+1, pads[i])
	}
}

type rgb struct{ r, g, b float64 }

var (
	panel   = rgb{12, 12, 14}
	lampOff = rgb{60, 60, 64}
	lampOn  = rgb{242, 163, 58} // the design's accent
	label   = rgb{200, 200, 200}
	beamOff = rgb{0, 0, 0}
	beamOn  = rgb{255, 255, 255}
)

// yuv converts RGB to BT.601 limited-range YUV.
func yuv(c rgb) [3]byte {
	y := 16 + (65.481*c.r+128.553*c.g+24.966*c.b)/255
	u := 128 + (-37.797*c.r-74.203*c.g+112*c.b)/255
	v := 128 + (112*c.r-93.786*c.g-18.214*c.b)/255
	return [3]byte{byte(y), byte(u), byte(v)}
}

type frame struct {
	buf          []byte
	w, h, cw, ch int
}

func (f frame) fill(b box, c rgb) {
	v := yuv(c)
	x0, y0 := max(b.x, 0), max(b.y, 0)
	x1, y1 := min(b.x+b.w, f.w), min(b.y+b.h, f.h)
	for y := y0; y < y1; y++ {
		row := f.buf[y*f.w : y*f.w+f.w]
		for x := x0; x < x1; x++ {
			row[x] = v[0]
		}
	}
	// Chroma covers 2x2 pixels: paint every block the box touches.
	for y := y0 / 2; y < (y1+1)/2; y++ {
		for x := x0 / 2; x < (x1+1)/2; x++ {
			i := y*f.cw + x
			f.buf[f.w*f.h+i] = v[1]
			f.buf[f.w*f.h+f.cw*f.ch+i] = v[2]
		}
	}
}

func lamp(on bool) rgb {
	if on {
		return lampOn
	}
	return lampOff
}

// cell draws one seat: its label, beacon, D-pad, six buttons, Coin and
// Start.
func (f frame) cell(c box, u, port int, p input.Pad) {
	f.fill(c, panel)
	on := p.Buttons.Pressed
	f.text(c.x+u, c.y+u/2, max(u/3, 1), []byte{'P', byte('0' + port)})
	beacon := beaconOf(c, u)
	if Active(p) {
		f.fill(beacon, beamOn)
	} else {
		f.fill(beacon, beamOff)
	}

	// D-pad, right of the beacon; the stick lights it too.
	left := p.Axes[0] <= -stickActive
	right := p.Axes[0] >= stickActive
	up := p.Axes[1] <= -stickActive
	down := p.Axes[1] >= stickActive
	cx := beacon.x + beacon.w + 3*u
	cy := c.y + c.h/2 + u/2
	f.fill(box{cx - u/2, cy - u/2, u, u}, lampOff)
	f.fill(box{cx - u/2, cy - u - u/2, u, u}, lamp(on(input.Up) || up))
	f.fill(box{cx - u/2, cy + u/2, u, u}, lamp(on(input.Down) || down))
	f.fill(box{cx - u - u/2, cy - u/2, u, u}, lamp(on(input.Left) || left))
	f.fill(box{cx + u/2, cy - u/2, u, u}, lamp(on(input.Right) || right))

	// Buttons 1-3 on the bottom row and 4-6 above, like an arcade panel.
	bx := cx + 2*u + u/2
	rows := [2][3]input.Button{{input.Button4, input.Button5, input.Button6}, {input.Button1, input.Button2, input.Button3}}
	for r, row := range rows {
		for i, btn := range row {
			f.fill(box{bx + i*(u+u/2), cy - u - u/2 + r*(u+u/2) + u/4, u, u}, lamp(on(btn)))
		}
	}

	// Coin and Start (any start button of the panel counts for Start).
	start := on(input.Start) || on(input.Start1) || on(input.Start2) || on(input.Start3) || on(input.Start4)
	f.fill(box{c.x + c.w - 2*u - u/2, c.y + u/2, u, u / 2}, lamp(on(input.Coin)))
	f.fill(box{c.x + c.w - u - u/4, c.y + u/2, u, u / 2}, lamp(start))
}

// glyphs is a 3x5 font for the seat labels; each byte is one row, bit 2 =
// left.
var glyphs = map[byte][5]byte{
	'P': {7, 5, 7, 4, 4},
	'1': {2, 6, 2, 2, 7},
	'2': {7, 1, 7, 4, 7},
	'3': {7, 1, 3, 1, 7},
	'4': {5, 5, 7, 1, 1},
}

func (f frame) text(x, y, scale int, s []byte) {
	for _, ch := range s {
		g, ok := glyphs[ch]
		if !ok {
			x += 4 * scale
			continue
		}
		for row := 0; row < 5; row++ {
			for col := 0; col < 3; col++ {
				if g[row]&(4>>col) != 0 {
					f.fill(box{x + col*scale, y + row*scale, scale, scale}, label)
				}
			}
		}
		x += 4 * scale
	}
}
