// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package testpattern draws a TV test card, inspired by the Philips
// PM5544, and generates the 1 kHz reference tone. Together they work like
// an echo test call: they prove the whole video and audio path works.
//
// The card is drawn in code (no image files). Like the broadcast cards, a
// running clock sits in the top box, which also shows that the picture is
// live. The bottom box draws a gamepad that lights up with the keyboard
// or a real controller, so the card also tests input.
package testpattern

import (
	"fmt"
	"math"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/pkg/input"
)

type rgb struct{ r, g, b float64 }

var (
	black      = rgb{0, 0, 0}
	white      = rgb{235, 235, 235}
	gridGray   = rgb{110, 110, 110}
	lightGray  = rgb{180, 180, 180}
	lampOff    = rgb{55, 55, 55}
	padLine    = rgb{120, 120, 120}
	fpsGreen   = rgb{120, 230, 150}
	lampOn     = rgb{242, 163, 58} // design accent
	colorBars  = []rgb{{190, 190, 40}, {0, 180, 180}, {0, 180, 50}, {190, 0, 190}, {190, 0, 20}, {20, 20, 170}}
	grayScale  = []rgb{{0, 0, 0}, {51, 51, 51}, {102, 102, 102}, {153, 153, 153}, {204, 204, 204}, {255, 255, 255}}
	sideTop    = []rgb{{19, 147, 112}, {65, 105, 225}} // left outer, left inner
	sideBottom = []rgb{{190, 82, 110}, {155, 105, 35}}
	rightTop   = []rgb{{107, 142, 35}, {65, 105, 225}} // right outer, right inner
	rightBot   = []rgb{{123, 95, 230}, {155, 105, 35}}
)

// yuv converts RGB to BT.601 limited-range YUV.
func yuv(c rgb) [3]byte {
	y := 16 + (65.481*c.r+128.553*c.g+24.966*c.b)/255
	u := 128 + (-37.797*c.r-74.203*c.g+112*c.b)/255
	v := 128 + (112*c.r-93.786*c.g-18.214*c.b)/255
	return [3]byte{byte(y), byte(u), byte(v)}
}

// Pattern renders frames of a fixed size (4:3 looks right, e.g. 640x480).
type Pattern struct {
	w, h, cw, ch int
	base         []byte // the static card, drawn once
	frame        []byte

	clockBox, padBox box
}

// Controls is what the card shows about the players' input and the
// stream.
type Controls struct {
	Buttons input.State // union of every player
	Axes    input.Axes  // the stick that moves the most
	Players [input.MaxLocalPlayers]bool
	FPS     float64 // frames per second actually sent by the device
}

type box struct{ x, y, w, h int }

// New creates a pattern. Sizes must be even.
func New(w, h int) *Pattern {
	cw, ch := w/2, h/2
	size := w*h + 2*cw*ch
	p := &Pattern{w: w, h: h, cw: cw, ch: ch, base: make([]byte, size), frame: make([]byte, size)}
	p.drawCard()
	return p
}

// Size returns the frame dimensions.
func (p *Pattern) Size() (w, h int) { return p.w, p.h }

// Frame draws the card at time now with the given controls. The
// returned slice is reused by the next call.
func (p *Pattern) Frame(now time.Time, c Controls) []byte {
	copy(p.frame, p.base)
	p.drawClock(now)
	p.drawPad(c)
	return p.frame
}

func (p *Pattern) set(buf []byte, x, y int, c [3]byte) {
	buf[y*p.w+x] = c[0]
	if x%2 == 0 && y%2 == 0 {
		i := (y/2)*p.cw + x/2
		buf[p.w*p.h+i] = c[1]
		buf[p.w*p.h+p.cw*p.ch+i] = c[2]
	}
}

func (p *Pattern) fill(buf []byte, b box, c rgb) {
	v := yuv(c)
	for y := max(b.y, 0); y < min(b.y+b.h, p.h); y++ {
		for x := max(b.x, 0); x < min(b.x+b.w, p.w); x++ {
			p.set(buf, x, y, v)
		}
	}
}

// drawCard paints everything that never changes.
func (p *Pattern) drawCard() {
	w, h := float64(p.w), float64(p.h)
	cell := p.w / 16 // grid cell (40 px at 640 wide)
	line := max(p.w/320, 2)
	cx, cy := w/2, h/2
	r := h * 0.448
	top := cy - r
	d := 2 * r

	for y := 0; y < p.h; y++ {
		for x := 0; x < p.w; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			var c rgb
			if (fx-cx)*(fx-cx)+(fy-cy)*(fy-cy) <= r*r {
				c = p.circlePixel(fx, fy, cx, cy, r, top, d, cell, line)
			} else {
				c = p.outsidePixel(x, y, cell, line)
			}
			p.set(p.base, x, y, yuv(c))
		}
	}

	// Black boxes for the clock (top) and the gamepad (bottom).
	p.clockBox = box{int(cx - r*0.52), int(top + d*0.03), int(r * 1.04), int(d * 0.085)}
	p.padBox = box{int(cx - r*0.74), int(top + d*0.615), int(r * 1.48), int(d * 0.27)}
	p.fill(p.base, p.clockBox, black)
	p.fill(p.base, p.padBox, black)
}

func (p *Pattern) outsidePixel(x, y, cell, line int) rgb {
	col, row := x/cell, y/cell
	cols, rows := p.w/cell, p.h/cell
	edge := cell / 5
	// Border castellation: black and white ticks along the edges.
	if y < edge || y >= p.h-edge {
		if (x/(cell/2))%2 == 0 {
			return black
		}
		return white
	}
	if x < edge || x >= p.w-edge {
		if row%2 == 0 {
			return black
		}
		return white
	}
	// Side color columns.
	inTop := row >= 1 && row <= rows/2-1
	inBottom := row >= rows/2 && row <= rows-2
	switch {
	case col == 1 && inTop:
		return sideTop[0]
	case col == 1 && inBottom:
		return sideBottom[0]
	case col == 2 && row >= 1 && row <= 2:
		return sideTop[1]
	case col == 2 && row >= rows-3 && row <= rows-2:
		return sideBottom[1]
	case col == cols-2 && inTop:
		return rightTop[0]
	case col == cols-2 && inBottom:
		return rightBot[0]
	case col == cols-3 && row >= 1 && row <= 2:
		return rightTop[1]
	case col == cols-3 && row >= rows-3 && row <= rows-2:
		return rightBot[1]
	}
	// Gray background with a white grid.
	if x%cell < line || y%cell < line {
		return white
	}
	return gridGray
}

func (p *Pattern) circlePixel(fx, fy, cx, cy, r, top, d float64, cell, line int) rgb {
	t := (fy - top) / d // 0 at the top of the circle, 1 at the bottom
	left := cx - r
	switch {
	case t < 0.12:
		return white
	case t < 0.19: // castellation
		if int((fx-left)/(d/16))%2 == 0 {
			return black
		}
		return lightGray
	case t < 0.33: // color bars
		i := min(int((fx-left)/(d/6)), 5)
		return colorBars[max(i, 0)]
	case t < 0.40: // black band with the center cross and grid lines
		if math.Abs(fy-cy) < float64(line) || math.Abs(fx-cx) < float64(line) {
			return white
		}
		if int(fx)%cell < line {
			return white
		}
		return black
	case t < 0.52: // frequency gratings, coarse to fine
		periods := []float64{16, 10, 8, 6, 4, 3}
		i := min(max(int((fx-left)/(d/6)), 0), 5)
		if math.Abs(fx-cx) < float64(line) {
			return white
		}
		l := 127.5 + 127.5*math.Sin(2*math.Pi*fx/periods[i])
		return rgb{l, l, l}
	case t < 0.60: // gray scale
		i := min(max(int((fx-left)/(d/6)), 0), 5)
		return grayScale[i]
	case t < 0.885:
		return white
	default: // yellow with a red center stripe
		if math.Abs(fx-cx) < d*0.05 {
			return colorBars[4]
		}
		return colorBars[0]
	}
}

// drawClock writes HH:MM:SS in the top box.
func (p *Pattern) drawClock(now time.Time) {
	scale := max(p.clockBox.h/10, 1)
	p.textCentered(now.Format("15:04:05"), p.clockBox.x+p.clockBox.w/2, p.clockBox.y+p.clockBox.h/2, scale, white)
}

// text draws s with its top-left corner at (x, y).
func (p *Pattern) text(s string, x, y, scale int, color rgb) {
	c := yuv(color)
	for _, ch := range s {
		glyph := font[ch]
		for row := 0; row < 7; row++ {
			for col := 0; col < 5; col++ {
				if glyph[row]&(1<<(4-col)) == 0 {
					continue
				}
				for dy := 0; dy < scale; dy++ {
					for dx := 0; dx < scale; dx++ {
						px, py := x+col*scale+dx, y+row*scale+dy
						if px >= 0 && px < p.w && py >= 0 && py < p.h {
							p.set(p.frame, px, py, c)
						}
					}
				}
			}
		}
		x += 6 * scale
	}
}

// textCentered draws s centered on (cx, cy).
func (p *Pattern) textCentered(s string, cx, cy, scale int, color rgb) {
	w := len(s)*6*scale - scale
	p.text(s, cx-w/2, cy-7*scale/2, scale, color)
}

func (p *Pattern) circle(cx, cy, r int, color rgb) {
	c := yuv(color)
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			if (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r && x >= 0 && x < p.w && y >= 0 && y < p.h {
				p.set(p.frame, x, y, c)
			}
		}
	}
}

func (p *Pattern) fillFrame(b box, c rgb) { p.fill(p.frame, b, c) }

func lamp(on bool) rgb {
	if on {
		return lampOn
	}
	return lampOff
}

func textOn(lit bool) rgb {
	if lit {
		return black
	}
	return white
}

// drawPad draws a gamepad (Nintendo layout: X top, Y left, A right,
// B bottom) whose parts light up while pressed, the local player lamps,
// and the frames per second being sent.
func (p *Pattern) drawPad(c Controls) {
	b := p.padBox
	u := max(b.h/8, 4) // layout unit
	on := c.Buttons.Pressed
	at := func(fx, fy float64) (int, int) { return b.x + int(fx*float64(b.w)), b.y + int(fy*float64(b.h)) }

	// Shoulders: ZL L ... R ZR.
	sw, sh := 2*u, u
	for _, s := range []struct {
		fx    float64
		btn   input.Button
		label string
	}{{0.09, input.L2, "ZL"}, {0.23, input.Button5, "L"}, {0.77, input.Button6, "R"}, {0.91, input.R2, "ZR"}} {
		x, y := at(s.fx, 0.08)
		p.fillFrame(box{x - sw/2, y, sw, sh}, lamp(on(s.btn)))
		p.textCentered(s.label, x, y+sh/2, 1, textOn(on(s.btn)))
	}

	// Player lamps between the shoulders: lit while that seat plays, and
	// while anyone presses its start button (1P to 4P).
	for i := 0; i < input.MaxLocalPlayers; i++ {
		x, y := at(0.38+float64(i)*0.08, 0.08)
		lit := c.Players[i] || on(input.StartOf(i+1))
		p.fillFrame(box{x - u/2, y, u, sh}, lamp(lit))
		p.textCentered(string(rune('1'+i)), x, y+sh/2, 1, textOn(lit))
	}

	// D-pad.
	dx, dy := at(0.17, 0.5)
	arm := u + u/3
	p.fillFrame(box{dx - arm/2, dy - arm/2, arm, arm}, padLine)
	p.fillFrame(box{dx - arm/2, dy - arm*3/2, arm, arm}, lamp(on(input.Up)))
	p.fillFrame(box{dx - arm/2, dy + arm/2, arm, arm}, lamp(on(input.Down)))
	p.fillFrame(box{dx - arm*3/2, dy - arm/2, arm, arm}, lamp(on(input.Left)))
	p.fillFrame(box{dx + arm/2, dy - arm/2, arm, arm}, lamp(on(input.Right)))

	// Face buttons.
	fx, fy := at(0.83, 0.5)
	fr := u*2/3 + 1
	off := u + u/2
	for _, f := range []struct {
		x, y  int
		btn   input.Button
		label string
	}{{fx, fy - off, input.Button4, "X"}, {fx - off, fy, input.Button3, "Y"}, {fx + off, fy, input.Button2, "A"}, {fx, fy + off, input.Button1, "B"}} {
		p.circle(f.x, f.y, fr, lamp(on(f.btn)))
		p.textCentered(f.label, f.x+1, f.y, 1, textOn(on(f.btn)))
	}

	// Center: minus (coin), plus (start), capture, home.
	mx, my := at(0.40, 0.36)
	px, py := at(0.60, 0.36)
	p.circle(mx, my, u/2+1, lamp(on(input.Coin)))
	p.textCentered("-", mx+1, my, 1, textOn(on(input.Coin)))
	p.circle(px, py, u/2+1, lamp(on(input.Start)))
	p.textCentered("+", px+1, py, 1, textOn(on(input.Start)))
	cx, cy := at(0.44, 0.55)
	p.fillFrame(box{cx - u/2, cy - u/2, u, u}, lamp(on(input.Capture)))
	hx, hy := at(0.56, 0.55)
	p.circle(hx, hy, u/2+1, lamp(on(input.Home)))

	// Sticks: ring plus a dot at the stick position.
	for _, s := range []struct {
		fx    float64
		ax    int
		click input.Button
	}{{0.33, 0, input.L3}, {0.67, 2, input.R3}} {
		sx, sy := at(s.fx, 0.8)
		ring := u + u/4
		p.circle(sx, sy, ring, padLine)
		p.circle(sx, sy, ring-2, black)
		ox := int(c.Axes[s.ax]) * (ring - u/3) / 127
		oy := int(c.Axes[s.ax+1]) * (ring - u/3) / 127
		dot := lampOff
		if on(s.click) || ox != 0 || oy != 0 {
			dot = lampOn
		}
		p.circle(sx+ox, sy+oy, u/2, dot)
	}

	// Frames per second sent by the device.
	fpsX, fpsY := at(0.5, 0.82)
	p.textCentered(fmt.Sprintf("%.1f FPS", c.FPS), fpsX, fpsY, 1, fpsGreen)
}

// font is a 5x7 bitmap for the clock; each byte is one row, bit 4 = left.
var font = map[rune][7]byte{
	'0': {0x0E, 0x11, 0x13, 0x15, 0x19, 0x11, 0x0E},
	'1': {0x04, 0x0C, 0x04, 0x04, 0x04, 0x04, 0x0E},
	'2': {0x0E, 0x11, 0x01, 0x02, 0x04, 0x08, 0x1F},
	'3': {0x1F, 0x02, 0x04, 0x02, 0x01, 0x11, 0x0E},
	'4': {0x02, 0x06, 0x0A, 0x12, 0x1F, 0x02, 0x02},
	'5': {0x1F, 0x10, 0x1E, 0x01, 0x01, 0x11, 0x0E},
	'6': {0x06, 0x08, 0x10, 0x1E, 0x11, 0x11, 0x0E},
	'7': {0x1F, 0x01, 0x02, 0x04, 0x08, 0x08, 0x08},
	'8': {0x0E, 0x11, 0x11, 0x0E, 0x11, 0x11, 0x0E},
	'9': {0x0E, 0x11, 0x11, 0x0F, 0x01, 0x02, 0x0C},
	':': {0x00, 0x0C, 0x0C, 0x00, 0x0C, 0x0C, 0x00},
	'.': {0x00, 0x00, 0x00, 0x00, 0x00, 0x0C, 0x0C},
	' ': {0, 0, 0, 0, 0, 0, 0},
	'A': {0x0E, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11},
	'B': {0x1E, 0x11, 0x11, 0x1E, 0x11, 0x11, 0x1E},
	'F': {0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x10},
	'L': {0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x1F},
	'P': {0x1E, 0x11, 0x11, 0x1E, 0x10, 0x10, 0x10},
	'R': {0x1E, 0x11, 0x11, 0x1E, 0x14, 0x12, 0x11},
	'S': {0x0F, 0x10, 0x10, 0x0E, 0x01, 0x01, 0x1E},
	'X': {0x11, 0x11, 0x0A, 0x04, 0x0A, 0x11, 0x11},
	'Y': {0x11, 0x11, 0x0A, 0x04, 0x04, 0x04, 0x04},
	'Z': {0x1F, 0x01, 0x02, 0x04, 0x08, 0x10, 0x1F},
	'+': {0x00, 0x04, 0x04, 0x1F, 0x04, 0x04, 0x00},
	'-': {0x00, 0x00, 0x00, 0x1F, 0x00, 0x00, 0x00},
}
