// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package framelab

import (
	"encoding/binary"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/lordbasex/go-link-console/backend-device/pkg/input"
)

// pattern draws flat color blocks with hard edges, like an arcade screen.
func pattern(w, h int) Image {
	img := NewImage(w, h)
	colors := [][3]byte{{248, 0, 0}, {0, 252, 0}, {0, 0, 248}, {248, 248, 248}, {0, 0, 0}, {248, 200, 0}}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := colors[((x/5)+(y/3))%len(colors)]
			copy(img.Pix[(y*w+x)*3:], c[:])
		}
	}
	return img
}

func TestNearestBoxDownRoundTrip(t *testing.T) {
	src := pattern(13, 7)
	up := Nearest(src, 2)
	if up.W != 26 || up.H != 14 {
		t.Fatalf("size %dx%d", up.W, up.H)
	}
	if string(BoxDown(up, 2).Pix) != string(src.Pix) {
		t.Fatal("box down does not undo nearest")
	}
	c := Crop(src, 10, 5, 10, 10)
	if c.W != 3 || c.H != 2 || c.Pix[0] != src.Pix[(5*13+10)*3] {
		t.Fatalf("crop %dx%d", c.W, c.H)
	}
}

func TestMetrics(t *testing.T) {
	a := pattern(40, 30)
	m, err := Compare(a, a)
	if err != nil || m.PSNR != MaxPSNR || math.Abs(m.SSIMY-1) > 1e-9 || math.Abs(m.SSIMRGB-1) > 1e-9 {
		t.Fatalf("identical pictures: %+v %v", m, err)
	}
	b := a.Clone()
	for i := range b.Pix {
		if b.Pix[i] < 255 {
			b.Pix[i]++
		}
	}
	// Every sample off by one: MSE 1, PSNR = 10*log10(255^2) = 48.13 dB.
	if p := PSNR(a, b); math.Abs(p-48.13) > 0.01 {
		t.Fatalf("PSNR %.3f", p)
	}
	noisy := a.Clone()
	seed := uint32(7)
	for i := range noisy.Pix {
		seed = seed*1664525 + 1013904223
		noisy.Pix[i] = byte(int(noisy.Pix[i]) + int(seed>>28) - 8)
	}
	mn, _ := Compare(a, noisy)
	if mn.SSIMY >= 0.99 || mn.SSIMY <= 0 || mn.PSNR >= 48 {
		t.Fatalf("noisy picture scored too high: %+v", mn)
	}
	if _, err := Compare(a, pattern(4, 4)); err == nil {
		t.Fatal("different sizes compared")
	}
	avg := Mean([]Metrics{{PSNR: 30, SSIMY: 0.5}, {PSNR: 40, SSIMY: 1}})
	if avg.PSNR != 35 || avg.SSIMY != 0.75 {
		t.Fatalf("mean %+v", avg)
	}
}

func TestI420RoundTrip(t *testing.T) {
	// A 2x nearest picture has one color per 2x2 block, so 4:2:0 loses no
	// color and the round trip with replicated chroma is close to exact.
	src := Nearest(pattern(20, 12), 2)
	if p := PSNR(src, FromI420(ToI420(src), src.W, src.H, Replicate)); p < 40 {
		t.Fatalf("2x round trip PSNR %.1f", p)
	}
	// Bilinear chroma blends half a source pixel at each color edge.
	back := FromI420(ToI420(src), src.W, src.H, Bilinear)
	// Both chroma modes agree when every 2x2 block is flat.
	if string(ToI420(src)) != string(ToI420Box(src)) {
		t.Fatal("box chroma differs on flat 2x2 blocks")
	}
	// At native size the color edges bleed: worse than the 2x picture.
	nat := pattern(20, 12)
	natBack := FromI420(ToI420(nat), nat.W, nat.H, Bilinear)
	if PSNR(nat, natBack) >= PSNR(src, back) {
		t.Fatal("native 4:2:0 should lose color detail")
	}
	gray := NewImage(4, 4)
	for i := range gray.Pix {
		gray.Pix[i] = 128
	}
	g := FromI420(ToI420(gray), 4, 4, Bilinear)
	for _, v := range g.Pix {
		if v < 126 || v > 130 {
			t.Fatalf("gray came back as %d", v)
		}
	}
}

func TestRunFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.glrun")
	w, err := CreateRun(path)
	if err != nil {
		t.Fatal(err)
	}
	frames := []Image{pattern(8, 6), pattern(8, 6), pattern(10, 4)}
	frames[1].Pix[0] = 17
	for _, f := range frames {
		if err := w.Write(f); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil || w.Frames() != 3 {
		t.Fatal(err)
	}
	r, err := OpenRun(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for i, want := range frames {
		got, err := r.Next()
		if err != nil || got.W != want.W || string(got.Pix) != string(want.Pix) {
			t.Fatalf("frame %d: %v", i, err)
		}
	}
	if _, err := r.Next(); err != io.EOF {
		t.Fatalf("want EOF, got %v", err)
	}
	bad := filepath.Join(t.TempDir(), "bad")
	os.WriteFile(bad, []byte("nope"), 0o644)
	if _, err := OpenRun(bad); err == nil {
		t.Fatal("garbage opened as a run")
	}
}

func TestIVF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.ivf")
	w, err := CreateIVF(path, "VP80", 320, 224, 59)
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte{1, 2, 3}, 0)
	w.Write([]byte{4}, 2)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if string(b[:4]) != "DKIF" || string(b[8:12]) != "VP80" || binary.LittleEndian.Uint16(b[12:]) != 320 ||
		binary.LittleEndian.Uint32(b[16:]) != 59 || binary.LittleEndian.Uint32(b[24:]) != 2 || len(b) != 32+12+3+12+1 {
		t.Fatalf("bad IVF: % x", b)
	}
	if binary.LittleEndian.Uint64(b[32+12+3+4:]) != 2 {
		t.Fatal("second frame pts")
	}
}

func TestScript(t *testing.T) {
	s, err := ParseScript("10-12:coin 20-21:start+b1@2 30-100:play")
	if err != nil {
		t.Fatal(err)
	}
	if !s.Pads(11)[0].Buttons.Pressed(input.Coin) || s.Pads(13)[0].Buttons != 0 {
		t.Fatal("coin range")
	}
	if p := s.Pads(20); p[1].Buttons != input.State(input.Start|input.Button1) || p[0].Buttons != 0 {
		t.Fatalf("port 2: %v", p)
	}
	moved := map[input.State]bool{}
	for f := 30; f <= 100; f++ {
		moved[s.Pads(f)[0].Buttons] = true
		if s.Pads(f) != s.Pads(f) {
			t.Fatal("play is not deterministic")
		}
	}
	if len(moved) < 4 {
		t.Fatalf("play pattern too dull: %d states", len(moved))
	}
	for _, bad := range []string{"1:coin", "5-2:coin", "1-2:jump", "1-2:coin@5", "x-2:coin"} {
		if _, err := ParseScript(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}

func TestVariant(t *testing.T) {
	v, err := ParseVariant("")
	if err != nil || v.Scale != 1 || v.BitrateKbps != DefaultBitrateKbps || v.MinQ != 4 || v.MaxQ != 56 || v.CPUUsed != 8 || v.Chroma != "topleft" {
		t.Fatalf("defaults %+v %v", v, err)
	}
	v, err = ParseVariant("name=B,scale=2,kbps=5000,minq=2")
	if err != nil || v.Name != "B" || v.Scale != 2 || v.BitrateKbps != 5000 || v.MinQ != 2 {
		t.Fatalf("parsed %+v %v", v, err)
	}
	i420, w, h := v.Prepare(pattern(10, 6))
	if w != 20 || h != 12 || len(i420) != 20*12*3/2 {
		t.Fatalf("prepared %dx%d (%d bytes)", w, h, len(i420))
	}
	for _, bad := range []string{"scale=0", "codec=h264", "minq=10,maxq=5", "kbps=x", "foo=1", "chroma=odd"} {
		if _, err := ParseVariant(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
	if StreamFPS(59.637) != 59 || StreamFPS(60) != 60 || StreamFPS(0) != 60 {
		t.Fatalf("stream fps %d %d", StreamFPS(59.637), StreamFPS(60))
	}
}

func TestHotspot(t *testing.T) {
	a := NewImage(64, 48)
	b := a.Clone()
	for y := 30; y < 34; y++ {
		for x := 40; x < 44; x++ {
			b.Pix[(y*64+x)*3] = 255
		}
	}
	if x, y := Hotspot(a, b, 16, 16); x > 40 || x+16 < 44 || y > 30 || y+16 < 34 {
		t.Fatalf("hotspot at %d,%d misses the difference", x, y)
	}
}

func TestPrepareFrameMatchesPrepare(t *testing.T) {
	img := NewImage(37, 21)
	seed := uint32(99)
	for i := range img.Pix {
		seed = seed*1664525 + 1013904223
		img.Pix[i] = byte(seed >> 24)
	}
	for _, spec := range []string{"scale=1", "chroma=box", "scale=2", "scale=2,chroma=box", "scale=3"} {
		v, err := ParseVariant(spec)
		if err != nil {
			t.Fatal(err)
		}
		want, ww, wh := v.Prepare(img)
		got, gw, gh, ok := v.PrepareFrame(nil, Frame(img))
		if !ok {
			if spec == "scale=1" || spec == "chroma=box" || spec == "scale=2" {
				t.Fatalf("%s: the device has a one-pass conversion", spec)
			}
			continue
		}
		if gw != ww || gh != wh || string(got) != string(want) {
			t.Fatalf("%s: the device's conversion differs from the lab's", spec)
		}
	}
}
