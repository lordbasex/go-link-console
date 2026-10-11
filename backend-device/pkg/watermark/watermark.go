// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package watermark draws the go-link icon on video frames (I420) while a
// game is recorded: it sits in a corner, beats slowly and changes corner
// now and then. It is drawn before the frame is encoded, so it is part of
// the picture everyone sees and of the recording, at no extra cost.
package watermark

import (
	"bytes"
	"image"
	"image/png"
	"math"
	"math/rand/v2"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/pkg/trayicon"
)

// icon is the mark at one size, ready to blend into I420.
type icon struct {
	size    int
	y, a    []uint8 // luma and alpha, size*size
	u, v, c []uint8 // chroma and its alpha, (size/2)*(size/2)
}

type step struct {
	at     time.Duration
	corner int // 0 top left, 1 top right, 2 bottom right, 3 bottom left
}

// Stamper draws the mark on the frames of one recording.
type Stamper struct {
	rnd     *rand.Rand
	plan    []step
	icons   map[int]*icon
	scratch []byte
}

// New starts a mark with its own random corners.
func New() *Stamper {
	return NewWithSeed(rand.Uint64())
}

// NewWithSeed makes the corners repeatable (tests).
func NewWithSeed(seed uint64) *Stamper {
	return &Stamper{rnd: rand.New(rand.NewPCG(seed, seed^0x9E3779B97F4A7C15)), icons: map[int]*icon{}}
}

// Corner returns which corner the mark is in at t: it changes every 8 to
// 15 seconds, never to the same corner.
func (s *Stamper) Corner(t time.Duration) int {
	if len(s.plan) == 0 {
		s.plan = append(s.plan, step{0, s.rnd.IntN(4)})
	}
	for last := s.plan[len(s.plan)-1]; last.at <= t; last = s.plan[len(s.plan)-1] {
		gap := 8*time.Second + time.Duration(s.rnd.Int64N(int64(7*time.Second)))
		s.plan = append(s.plan, step{last.at + gap, (last.corner + 1 + s.rnd.IntN(3)) % 4})
	}
	corner := s.plan[0].corner
	for _, p := range s.plan {
		if p.at <= t {
			corner = p.corner
		}
	}
	return corner
}

// Heartbeat is the mark's scale at t: two soft beats and a long rest every
// 3 seconds, 1 at rest and up to 1 + depth.
func Heartbeat(t time.Duration, depth float64) float64 {
	x := math.Mod(t.Seconds(), 3) / 3
	beat := func(center, width, height float64) float64 {
		d := (x - center) / width
		return height * math.Exp(-d*d)
	}
	return 1 + depth*max(beat(0.12, 0.06, 1), beat(0.32, 0.06, 0.7))
}

// Size of the mark for a picture this high: about 8 % of it.
func Size(height int) int {
	return max(14, int(math.Round(float64(height)*0.08)))
}

// Draw returns the frame with the mark, t after the recording started.
// The frame itself is not changed (a source may reuse its buffer).
func (s *Stamper) Draw(i420 []byte, w, h int, t time.Duration) []byte {
	if len(i420) < w*h*3/2 || w < 64 || h < 64 {
		return i420
	}
	base := Size(h)
	// The beat grows the mark by whole pixels, even sizes keep the chroma aligned.
	size := int(math.Round(float64(base)*Heartbeat(t, 0.12)/2)) * 2
	ic := s.icon(size)
	margin := max(4, h*3/100) &^ 1
	room := int(math.Ceil(float64(base)*1.12/2)) * 2 // the biggest beat
	corner := s.Corner(t)
	// The mark grows around the center of its spot.
	x0 := margin + (room-size)/2
	y0 := margin + (room-size)/2
	if corner == 1 || corner == 2 {
		x0 = w - margin - room + (room-size)/2
	}
	if corner == 2 || corner == 3 {
		y0 = h - margin - room + (room-size)/2
	}
	x0, y0 = x0&^1, y0&^1
	if cap(s.scratch) < len(i420) {
		s.scratch = make([]byte, len(i420))
	}
	out := s.scratch[:len(i420)]
	copy(out, i420)
	blend := func(dst, src uint8, a uint8) uint8 {
		return uint8((int(dst)*(255-int(a)) + int(src)*int(a) + 127) / 255)
	}
	for yy := 0; yy < ic.size; yy++ {
		py := y0 + yy
		if py < 0 || py >= h {
			continue
		}
		for xx := 0; xx < ic.size; xx++ {
			px := x0 + xx
			if px < 0 || px >= w {
				continue
			}
			if a := ic.a[yy*ic.size+xx]; a > 0 {
				out[py*w+px] = blend(out[py*w+px], ic.y[yy*ic.size+xx], a)
			}
		}
	}
	cw, ch, half := w/2, h/2, ic.size/2
	uPlane, vPlane := out[w*h:w*h+cw*ch], out[w*h+cw*ch:]
	for yy := 0; yy < half; yy++ {
		py := y0/2 + yy
		if py < 0 || py >= ch {
			continue
		}
		for xx := 0; xx < half; xx++ {
			px := x0/2 + xx
			if px < 0 || px >= cw {
				continue
			}
			if a := ic.c[yy*half+xx]; a > 0 {
				uPlane[py*cw+px] = blend(uPlane[py*cw+px], ic.u[yy*half+xx], a)
				vPlane[py*cw+px] = blend(vPlane[py*cw+px], ic.v[yy*half+xx], a)
			}
		}
	}
	return out
}

// icon renders the brand mark at this size once and keeps it.
func (s *Stamper) icon(size int) *icon {
	if ic := s.icons[size]; ic != nil {
		return ic
	}
	ic := &icon{size: size}
	img, err := png.Decode(bytes.NewReader(trayicon.Render(size, false)))
	if err != nil {
		img = image.NewNRGBA(image.Rect(0, 0, size, size))
	}
	half := size / 2
	ic.y, ic.a = make([]uint8, size*size), make([]uint8, size*size)
	ic.u, ic.v, ic.c = make([]uint8, half*half), make([]uint8, half*half), make([]uint8, half*half)
	var su, sv, sa [][3]int
	su, sv, sa = make([][3]int, half*half), make([][3]int, half*half), make([][3]int, half*half)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			// Straight (not premultiplied) 8 bit color.
			R, G, B := float64(r)*255/float64(a), float64(g)*255/float64(a), float64(b)*255/float64(a)
			A := uint8(a >> 8)
			ic.y[y*size+x] = clamp(16 + 0.257*R + 0.504*G + 0.098*B)
			ic.a[y*size+x] = A
			if x/2 < half && y/2 < half {
				i := (y/2)*half + x/2
				su[i][0] += int(clamp(128-0.148*R-0.291*G+0.439*B)) * int(A)
				sv[i][0] += int(clamp(128+0.439*R-0.368*G-0.071*B)) * int(A)
				sa[i][0] += int(A)
				sa[i][1]++
			}
		}
	}
	for i := range sa {
		if sa[i][0] == 0 {
			continue
		}
		ic.u[i] = uint8(su[i][0] / sa[i][0])
		ic.v[i] = uint8(sv[i][0] / sa[i][0])
		ic.c[i] = uint8(sa[i][0] / 4)
	}
	s.icons[size] = ic
	return ic
}

func clamp(v float64) uint8 {
	return uint8(math.Max(0, math.Min(255, math.Round(v))))
}
