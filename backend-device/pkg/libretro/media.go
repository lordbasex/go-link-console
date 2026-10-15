// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package libretro

import (
	"runtime"
	"sync"
)

// FrameSizeI420 returns the byte length of a packed I420 frame.
func FrameSizeI420(w, h int) int {
	cw, ch := (w+1)/2, (h+1)/2
	return w*h + 2*cw*ch
}

func rgbOf(f Frame, x, y int) (r, g, b int) {
	switch f.Format {
	case FormatXRGB8888:
		o := y*f.Pitch + x*4
		return int(f.Data[o+2]), int(f.Data[o+1]), int(f.Data[o])
	case FormatRGB565:
		o := y*f.Pitch + x*2
		v := int(f.Data[o]) | int(f.Data[o+1])<<8
		return (v >> 11 & 31) << 3, (v >> 5 & 63) << 2, (v & 31) << 3
	default: // 0RGB1555
		o := y*f.Pitch + x*2
		v := int(f.Data[o]) | int(f.Data[o+1])<<8
		return (v >> 10 & 31) << 3, (v >> 5 & 31) << 3, (v & 31) << 3
	}
}

// ToRGB converts a frame to packed 8-bit RGB (3 bytes per pixel, rows
// without padding) with the same channel expansion ToI420 uses, so it is
// exactly the picture the encoder is given. dst must have Width*Height*3
// bytes. The video quality lab uses it as the reference picture.
func ToRGB(dst []byte, f Frame) {
	i := 0
	for y := 0; y < f.Height; y++ {
		for x := 0; x < f.Width; x++ {
			r, g, b := rgbOf(f, x, y)
			dst[i], dst[i+1], dst[i+2] = byte(r), byte(g), byte(b)
			i += 3
		}
	}
}

// ToI420 converts a frame to packed I420 (BT.601 limited range), the
// input of the VP8 encoder. dst must have FrameSizeI420 bytes. Chroma is
// taken from the top-left pixel of each 2x2 block, which is enough for
// the flat colors of arcade games and keeps the conversion cheap.
func ToI420(dst []byte, f Frame) {
	if f.Format == FormatXRGB8888 {
		toI420XRGB(dst, f)
		return
	}
	w, h := f.Width, f.Height
	cw := (w + 1) / 2
	yPlane := dst[:w*h]
	uPlane := dst[w*h : w*h+cw*((h+1)/2)]
	vPlane := dst[w*h+cw*((h+1)/2):]
	for y := 0; y < h; y++ {
		row := y * w
		for x := 0; x < w; x++ {
			r, g, b := rgbOf(f, x, y)
			yPlane[row+x] = byte((66*r+129*g+25*b+128)>>8 + 16)
			if x%2 == 0 && y%2 == 0 {
				i := (y/2)*cw + x/2
				uPlane[i] = byte((-38*r-74*g+112*b+128)>>8 + 128)
				vPlane[i] = byte((112*r-94*g-18*b+128)>>8 + 128)
			}
		}
	}
}

// parallelPixels is the size from which toI420XRGB splits a frame between
// cores (a go-link HD picture of 720p or more: about 1 ms instead of 5).
const parallelPixels = 640 * 360

// toI420XRGB is ToI420 of an XRGB8888 frame, bit for bit, reading each row
// straight and splitting a big frame's rows between up to four cores.
func toI420XRGB(dst []byte, f Frame) {
	pairs := (f.Height + 1) / 2
	parts := 1
	if f.Width*f.Height >= parallelPixels {
		parts = min(runtime.NumCPU(), 4, pairs)
	}
	if parts <= 1 {
		xrgbRows(dst, f, 0, pairs)
		return
	}
	var wg sync.WaitGroup
	for k := range parts {
		wg.Add(1)
		go func(p0, p1 int) {
			defer wg.Done()
			xrgbRows(dst, f, p0, p1)
		}(pairs*k/parts, pairs*(k+1)/parts)
	}
	wg.Wait()
}

// xrgbRows converts the row pairs p0..p1-1 (rows 2p and 2p+1, chroma from
// row 2p) of an XRGB8888 frame.
func xrgbRows(dst []byte, f Frame, p0, p1 int) {
	w, h := f.Width, f.Height
	cw, ch := (w+1)/2, (h+1)/2
	yPlane, uPlane, vPlane := dst[:w*h], dst[w*h:w*h+cw*ch], dst[w*h+cw*ch:]
	for p := p0; p < p1; p++ {
		for y := 2 * p; y < min(2*p+2, h); y++ {
			src := f.Data[y*f.Pitch : y*f.Pitch+w*4]
			out := yPlane[y*w : y*w+w]
			for x := range out {
				o := x * 4
				b, g, r := int(src[o]), int(src[o+1]), int(src[o+2])
				out[x] = byte((66*r+129*g+25*b+128)>>8 + 16)
			}
			if y == 2*p {
				u, v := uPlane[p*cw:p*cw+cw], vPlane[p*cw:p*cw+cw]
				for i := range u {
					o := i * 8
					b, g, r := int(src[o]), int(src[o+1]), int(src[o+2])
					u[i] = byte(cbOf(r, g, b))
					v[i] = byte(crOf(r, g, b))
				}
			}
		}
	}
}

// decodeRow expands row y of a frame to packed 8-bit RGB (3 bytes per
// pixel) with the channel expansion rgbOf uses. The pixel format is
// checked once per row, so the loops stay tight.
func decodeRow(row []byte, f Frame, y int) {
	src := f.Data[y*f.Pitch:]
	w := f.Width
	switch f.Format {
	case FormatXRGB8888:
		for x := 0; x < w; x++ {
			o, i := x*4, x*3
			row[i], row[i+1], row[i+2] = src[o+2], src[o+1], src[o]
		}
	case FormatRGB565:
		for x := 0; x < w; x++ {
			v := int(src[x*2]) | int(src[x*2+1])<<8
			i := x * 3
			row[i], row[i+1], row[i+2] = byte((v>>11&31)<<3), byte((v>>5&63)<<2), byte((v&31)<<3)
		}
	default: // 0RGB1555
		for x := 0; x < w; x++ {
			v := int(src[x*2]) | int(src[x*2+1])<<8
			i := x * 3
			row[i], row[i+1], row[i+2] = byte((v>>10&31)<<3), byte((v>>5&31)<<3), byte((v&31)<<3)
		}
	}
}

// rowStack is the widest row the converters keep on the stack; wider
// frames get a buffer from the heap.
const rowStack = 1024

// BT.601 limited range, the integer formulas of ToI420.
func lumaOf(r, g, b int) byte { return byte((66*r+129*g+25*b+128)>>8 + 16) }
func cbOf(r, g, b int) int    { return (-38*r-74*g+112*b+128)>>8 + 128 }
func crOf(r, g, b int) int    { return (112*r-94*g-18*b+128)>>8 + 128 }

// ToI420Double scales a frame 2x with nearest neighbour and converts it to
// packed I420 in one pass: dst must have FrameSizeI420(2*Width, 2*Height)
// bytes. The result is exactly ToI420 of the frame upscaled first, bit for
// bit, without ever building the upscaled picture: each source pixel is
// one 2x2 block of luma and, since chroma is taken from the top-left pixel
// of each block, its own chroma sample. So pixel art keeps one color per
// pixel instead of one per 2x2 block.
func ToI420Double(dst []byte, f Frame) {
	w, h := f.Width, f.Height
	ww := 2 * w
	luma := ww * 2 * h
	uPlane := dst[luma : luma+w*h]
	vPlane := dst[luma+w*h : luma+2*w*h]
	var stack [rowStack * 3]byte
	row := stack[:]
	if w > rowStack {
		row = make([]byte, w*3)
	}
	row = row[:w*3]
	for y := 0; y < h; y++ {
		decodeRow(row, f, y)
		y0 := dst[2*y*ww : 2*y*ww+ww]
		u := uPlane[y*w : y*w+w]
		v := vPlane[y*w : y*w+w]
		for x := range u {
			p := row[x*3 : x*3+3 : x*3+3]
			r, g, b := int(p[0]), int(p[1]), int(p[2])
			l := lumaOf(r, g, b)
			o := y0[2*x : 2*x+2 : 2*x+2]
			o[0], o[1] = l, l
			u[x] = byte(cbOf(r, g, b))
			v[x] = byte(crOf(r, g, b))
		}
		copy(dst[(2*y+1)*ww:(2*y+2)*ww], y0)
	}
}

// ToI420Scaled scales a frame n times (n >= 1) with nearest neighbour and
// converts it to packed I420 in one pass: dst must have
// FrameSizeI420(n*Width, n*Height) bytes. Like ToI420Double, the result is
// exactly ToI420 of the frame upscaled first, bit for bit: each chroma
// sample comes from the source pixel under the top-left pixel of its 2x2
// block. go-link HD's 640 x 360 screen uses n = 3 for 1080p and 6 for 4K.
func ToI420Scaled(dst []byte, f Frame, n int) {
	w, h := f.Width, f.Height
	ww, hh := n*w, n*h
	cw := (ww + 1) / 2
	uPlane := dst[ww*hh : ww*hh+cw*((hh+1)/2)]
	vPlane := dst[ww*hh+cw*((hh+1)/2):]
	// each source pixel is converted once: its luma and chroma
	row := make([]byte, w*3)
	lum := make([]byte, w)
	cb := make([]byte, w)
	cr := make([]byte, w)
	// the source column under each chroma sample's top-left pixel
	cmap := make([]int32, cw)
	for cx := range cmap {
		cmap[cx] = int32(2 * cx / n)
	}
	for y := 0; y < h; y++ {
		decodeRow(row, f, y)
		for x := 0; x < w; x++ {
			r, g, b := int(row[x*3]), int(row[x*3+1]), int(row[x*3+2])
			lum[x] = lumaOf(r, g, b)
			cb[x] = byte(cbOf(r, g, b))
			cr[x] = byte(crOf(r, g, b))
		}
		first := dst[n*y*ww : n*y*ww+ww]
		for x, l := range lum {
			o := first[n*x : n*x+n : n*x+n]
			for i := range o {
				o[i] = l
			}
		}
		for k := 1; k < n; k++ {
			copy(dst[(n*y+k)*ww:(n*y+k+1)*ww], first)
		}
		// the chroma rows whose top-left pixel row falls in this source row
		for yy := n * y; yy < n*y+n; yy++ {
			if yy%2 != 0 {
				continue
			}
			u := uPlane[(yy/2)*cw : (yy/2)*cw+cw]
			v := vPlane[(yy/2)*cw : (yy/2)*cw+cw]
			for cx, sx := range cmap {
				u[cx] = cb[sx]
				v[cx] = cr[sx]
			}
		}
	}
}

// ToI420Box converts a frame to packed I420 like ToI420, but each chroma
// sample is the rounded average of the chroma of its 2x2 block (the
// pixels inside the picture), instead of the top-left pixel's. It keeps
// small color details from bleeding: the native path of the "saver"
// video quality.
func ToI420Box(dst []byte, f Frame) {
	w, h := f.Width, f.Height
	cw := (w + 1) / 2
	ch := (h + 1) / 2
	uPlane := dst[w*h : w*h+cw*ch]
	vPlane := dst[w*h+cw*ch:]
	var stack [rowStack * 3]byte
	var sums [2 * rowStack]int32
	row := stack[:]
	acc := sums[:]
	if w > rowStack {
		row = make([]byte, w*3)
		acc = make([]int32, 2*cw)
	}
	row = row[:w*3]
	su, sv := acc[:cw], acc[cw:2*cw]
	for cy := 0; cy < ch; cy++ {
		clear(su)
		clear(sv)
		n := int32(2)
		if 2*cy+1 >= h {
			n = 1
		}
		for k := int32(0); k < n; k++ {
			y := 2*cy + int(k)
			decodeRow(row, f, y)
			out := dst[y*w : y*w+w]
			for x := range out {
				r, g, b := int(row[x*3]), int(row[x*3+1]), int(row[x*3+2])
				out[x] = lumaOf(r, g, b)
				su[x>>1] += int32(cbOf(r, g, b))
				sv[x>>1] += int32(crOf(r, g, b))
			}
		}
		u := uPlane[cy*cw : cy*cw+cw]
		v := vPlane[cy*cw : cy*cw+cw]
		full := 2 * n
		for cx := range u {
			cnt := full
			if 2*cx+1 >= w {
				cnt = n
			}
			u[cx] = byte((su[cx] + cnt/2) / cnt)
			v[cx] = byte((sv[cx] + cnt/2) / cnt)
		}
	}
}

// Resampler converts interleaved stereo 16-bit audio between sample rates
// with linear interpolation, keeping state between calls so there are no
// clicks at buffer boundaries.
type Resampler struct {
	step float64 // input samples per output sample
	pos  float64 // position of the next output sample, relative to prev
	prev [2]int16
	out  []int16
}

// NewResampler converts from rate `from` to rate `to`.
func NewResampler(from, to float64) *Resampler {
	return &Resampler{step: from / to}
}

// Process consumes interleaved stereo input and returns resampled output.
// The returned slice is reused by the next call.
func (r *Resampler) Process(in []int16) []int16 {
	r.out = r.out[:0]
	frames := len(in) / 2
	if r.step == 1 {
		return append(r.out, in[:frames*2]...)
	}
	sample := func(i int, ch int) float64 {
		if i < 0 {
			return float64(r.prev[ch])
		}
		return float64(in[i*2+ch])
	}
	// Positions are measured from prev (index -1) to the last input frame.
	for r.pos < float64(frames) {
		i := int(r.pos) - 1
		frac := r.pos - float64(int(r.pos))
		for ch := 0; ch < 2; ch++ {
			a, b := sample(i, ch), sample(i+1, ch)
			r.out = append(r.out, int16(a+(b-a)*frac))
		}
		r.pos += r.step
	}
	r.pos -= float64(frames)
	if frames > 0 {
		r.prev = [2]int16{in[(frames-1)*2], in[(frames-1)*2+1]}
	}
	return r.out
}
