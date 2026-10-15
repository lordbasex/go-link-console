// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package libretro

import (
	"bytes"
	"math"
	"testing"
)

func TestToI420Colors(t *testing.T) {
	// 2x2 XRGB8888: white, black / red, blue.
	f := Frame{Width: 2, Height: 2, Pitch: 8, Format: FormatXRGB8888, Data: []byte{
		255, 255, 255, 0, 0, 0, 0, 0,
		0, 0, 255, 0, 255, 0, 0, 0,
	}}
	dst := make([]byte, FrameSizeI420(2, 2))
	ToI420(dst, f)
	if dst[0] != 235 || dst[1] != 16 {
		t.Fatalf("luma white/black = %d/%d", dst[0], dst[1])
	}
	if dst[4] != 128 || dst[5] != 128 {
		t.Fatalf("white chroma = %d/%d", dst[4], dst[5])
	}

	// RGB565 pure red.
	f565 := Frame{Width: 1, Height: 1, Pitch: 2, Format: FormatRGB565, Data: []byte{0x00, 0xF8}}
	d := make([]byte, FrameSizeI420(1, 1))
	ToI420(d, f565)
	if d[0] < 75 || d[0] > 85 || d[2] < 235 { // Y≈81, V≈240
		t.Fatalf("red = %v", d)
	}
}

func TestToRGBMatchesToI420Input(t *testing.T) {
	// RGB565: pure red, then pure green, with the same expansion ToI420 uses.
	f := Frame{Width: 2, Height: 1, Pitch: 4, Format: FormatRGB565, Data: []byte{0x00, 0xF8, 0xE0, 0x07}}
	dst := make([]byte, 6)
	ToRGB(dst, f)
	if want := []byte{248, 0, 0, 0, 252, 0}; string(dst) != string(want) {
		t.Fatalf("rgb565 = %v, want %v", dst, want)
	}
	x := Frame{Width: 1, Height: 1, Pitch: 4, Format: FormatXRGB8888, Data: []byte{3, 2, 1, 0}}
	ToRGB(dst[:3], x)
	if dst[0] != 1 || dst[1] != 2 || dst[2] != 3 {
		t.Fatalf("xrgb8888 = %v", dst[:3])
	}
}

func TestResamplerRateAndContinuity(t *testing.T) {
	r := NewResampler(22050, 48000)
	total := 0
	var last int16
	for block := 0; block < 50; block++ {
		in := make([]int16, 441*2) // 20 ms at 22.05 kHz
		for i := 0; i < 441; i++ {
			v := int16(10000 * math.Sin(2*math.Pi*1000*float64(block*441+i)/22050))
			in[i*2], in[i*2+1] = v, v
		}
		out := r.Process(in)
		if block > 0 && math.Abs(float64(out[0]-last)) > 4000 {
			t.Fatalf("discontinuity at block %d: %d -> %d", block, last, out[0])
		}
		last = out[len(out)-2]
		total += len(out) / 2
	}
	// 1 second of input gives about 48000 output frames.
	if total < 47900 || total > 48100 {
		t.Fatalf("got %d frames for one second", total)
	}
	same := NewResampler(48000, 48000)
	if out := same.Process([]int16{1, 2, 3, 4}); len(out) != 4 {
		t.Fatalf("passthrough %v", out)
	}
}

func TestDefaultOption(t *testing.T) {
	for decl, want := range map[string]string{
		"Gamma correction; 1.0|0.5|1.2":     "1.0",
		"Skip disclaimer; disabled|enabled": "disabled",
		"Brightness; 1.0":                   "1.0",
	} {
		if got, ok := DefaultOption(decl); !ok || got != want {
			t.Errorf("DefaultOption(%q) = %q, %v", decl, got, ok)
		}
	}
	if _, ok := DefaultOption("no values"); ok {
		t.Error("a declaration without values has no default")
	}
}

// randomFrame fills a frame of the given format with pseudo-random pixels
// and some padding at the end of each row, like a core's pitch.
func randomFrame(format PixelFormat, w, h int, seed uint32) Frame {
	bpp := 2
	if format == FormatXRGB8888 {
		bpp = 4
	}
	pitch := w*bpp + 6
	data := make([]byte, pitch*h)
	for i := range data {
		seed = seed*1664525 + 1013904223
		data[i] = byte(seed >> 24)
	}
	return Frame{Data: data, Width: w, Height: h, Pitch: pitch, Format: format}
}

// upscale2 repeats every pixel of f over a 2x2 block, in f's own format:
// the separate upscale the fused converter replaces.
func upscale2(f Frame) Frame {
	bpp := 2
	if f.Format == FormatXRGB8888 {
		bpp = 4
	}
	pitch := 2 * f.Width * bpp
	out := Frame{Data: make([]byte, pitch*2*f.Height), Width: 2 * f.Width, Height: 2 * f.Height, Pitch: pitch, Format: f.Format}
	for y := 0; y < out.Height; y++ {
		for x := 0; x < out.Width; x++ {
			src := f.Data[(y/2)*f.Pitch+(x/2)*bpp:]
			copy(out.Data[y*pitch+x*bpp:], src[:bpp])
		}
	}
	return out
}

var formats = []PixelFormat{FormatXRGB8888, FormatRGB565, Format0RGB1555}

func TestToI420DoubleEqualsUpscaleThenConvert(t *testing.T) {
	sizes := [][2]int{{1, 1}, {2, 2}, {3, 5}, {7, 4}, {17, 9}, {384, 224}, {1030, 3}}
	for _, format := range formats {
		for i, sz := range sizes {
			f := randomFrame(format, sz[0], sz[1], uint32(i+1)*7919)
			want := make([]byte, FrameSizeI420(2*sz[0], 2*sz[1]))
			ToI420(want, upscale2(f))
			got := make([]byte, len(want))
			ToI420Double(got, f)
			if string(got) != string(want) {
				t.Fatalf("format %d %dx%d: the fused 2x conversion differs from upscale then ToI420", format, sz[0], sz[1])
			}
		}
	}
}

// upscaleN repeats every pixel of f over an n x n block, in f's own format.
func upscaleN(f Frame, n int) Frame {
	bpp := 2
	if f.Format == FormatXRGB8888 {
		bpp = 4
	}
	pitch := n * f.Width * bpp
	out := Frame{Data: make([]byte, pitch*n*f.Height), Width: n * f.Width, Height: n * f.Height, Pitch: pitch, Format: f.Format}
	for y := 0; y < out.Height; y++ {
		for x := 0; x < out.Width; x++ {
			src := f.Data[(y/n)*f.Pitch+(x/n)*bpp:]
			copy(out.Data[y*pitch+x*bpp:], src[:bpp])
		}
	}
	return out
}

func TestToI420ScaledEqualsUpscaleThenConvert(t *testing.T) {
	sizes := [][2]int{{1, 1}, {2, 2}, {3, 5}, {7, 4}, {17, 9}, {64, 36}}
	for _, format := range formats {
		for n := 1; n <= 6; n++ {
			for i, sz := range sizes {
				f := randomFrame(format, sz[0], sz[1], uint32(i+n)*7919)
				want := make([]byte, FrameSizeI420(n*sz[0], n*sz[1]))
				ToI420(want, upscaleN(f, n))
				got := make([]byte, len(want))
				ToI420Scaled(got, f, n)
				if string(got) != string(want) {
					t.Fatalf("format %d %dx%d x%d: the fused conversion differs from upscale then ToI420", format, sz[0], sz[1], n)
				}
			}
		}
	}
}

// boxReference is the plain definition of ToI420Box: ToI420's luma, and
// each chroma sample the rounded mean of its block's per-pixel chroma.
func boxReference(f Frame) []byte {
	w, h := f.Width, f.Height
	dst := make([]byte, FrameSizeI420(w, h))
	ToI420(dst, f)
	cw, ch := (w+1)/2, (h+1)/2
	for cy := 0; cy < ch; cy++ {
		for cx := 0; cx < cw; cx++ {
			var su, sv, n int
			for y := 2 * cy; y < min(2*cy+2, h); y++ {
				for x := 2 * cx; x < min(2*cx+2, w); x++ {
					r, g, b := rgbOf(f, x, y)
					su += (-38*r-74*g+112*b+128)>>8 + 128
					sv += (112*r-94*g-18*b+128)>>8 + 128
					n++
				}
			}
			dst[w*h+cy*cw+cx] = byte((su + n/2) / n)
			dst[w*h+cw*ch+cy*cw+cx] = byte((sv + n/2) / n)
		}
	}
	return dst
}

func TestToI420BoxAveragesEachBlock(t *testing.T) {
	sizes := [][2]int{{1, 1}, {2, 2}, {3, 5}, {7, 4}, {17, 9}, {288, 224}, {1031, 3}}
	for _, format := range formats {
		for i, sz := range sizes {
			f := randomFrame(format, sz[0], sz[1], uint32(i+3)*104729)
			want := boxReference(f)
			got := make([]byte, len(want))
			ToI420Box(got, f)
			if string(got) != string(want) {
				t.Fatalf("format %d %dx%d: box chroma differs from the reference", format, sz[0], sz[1])
			}
		}
	}
	// A red pixel next to three white ones: the block's color is their mean.
	f := Frame{Width: 2, Height: 2, Pitch: 8, Format: FormatXRGB8888, Data: []byte{
		0, 0, 255, 0, 255, 255, 255, 0,
		255, 255, 255, 0, 255, 255, 255, 0,
	}}
	dst := make([]byte, FrameSizeI420(2, 2))
	ToI420Box(dst, f)
	if v := dst[5]; v < 150 || v > 160 { // (240 + 3*128) / 4 = 156
		t.Fatalf("mean Cr = %d", v)
	}
}

func benchFrame(b *testing.B, convert func([]byte, Frame), scale int) {
	f := randomFrame(FormatRGB565, 384, 224, 42)
	dst := make([]byte, FrameSizeI420(384*scale, 224*scale))
	b.SetBytes(int64(len(dst)))
	b.ResetTimer()
	for range b.N {
		convert(dst, f)
	}
}

func BenchmarkToI420(b *testing.B)       { benchFrame(b, ToI420, 1) }
func BenchmarkToI420Box(b *testing.B)    { benchFrame(b, ToI420Box, 1) }
func BenchmarkToI420Double(b *testing.B) { benchFrame(b, ToI420Double, 2) }
func BenchmarkToI420Scaled3(b *testing.B) {
	benchFrame(b, func(dst []byte, f Frame) { ToI420Scaled(dst, f, 3) }, 3)
}

// BenchmarkUpscaleThenToI420 is the separate path the fused one replaces.
func BenchmarkUpscaleThenToI420(b *testing.B) {
	benchFrame(b, func(dst []byte, f Frame) { ToI420(dst, upscale2(f)) }, 2)
}

// toI420Reference is ToI420 pixel by pixel through rgbOf, as it was before
// the XRGB8888 fast path.
func toI420Reference(dst []byte, f Frame) {
	w, h := f.Width, f.Height
	cw := (w + 1) / 2
	yPlane, uPlane, vPlane := dst[:w*h], dst[w*h:w*h+cw*((h+1)/2)], dst[w*h+cw*((h+1)/2):]
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b := rgbOf(f, x, y)
			yPlane[y*w+x] = lumaOf(r, g, b)
			if x%2 == 0 && y%2 == 0 {
				uPlane[(y/2)*cw+x/2] = byte(cbOf(r, g, b))
				vPlane[(y/2)*cw+x/2] = byte(crOf(r, g, b))
			}
		}
	}
}

func TestToI420XRGBMatchesPixelByPixel(t *testing.T) {
	for _, size := range [][2]int{{1, 1}, {3, 5}, {384, 224}, {641, 361}, {1280, 720}} {
		f := randomFrame(FormatXRGB8888, size[0], size[1], 7) // rows with padding
		got, want := make([]byte, FrameSizeI420(size[0], size[1])), make([]byte, FrameSizeI420(size[0], size[1]))
		ToI420(got, f)
		toI420Reference(want, f)
		if !bytes.Equal(got, want) {
			t.Fatalf("%dx%d: the fast conversion differs from the reference", size[0], size[1])
		}
	}
}

func BenchmarkToI420XRGB720p(b *testing.B) {
	f := randomFrame(FormatXRGB8888, 1280, 720, 42)
	dst := make([]byte, FrameSizeI420(1280, 720))
	b.SetBytes(int64(len(dst)))
	b.ResetTimer()
	for range b.N {
		ToI420(dst, f)
	}
}
