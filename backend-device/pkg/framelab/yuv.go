// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package framelab

import "github.com/lordbasex/go-link-console/backend-device/pkg/libretro"

// ToI420 converts a picture exactly like the device does before encoding
// (libretro.ToI420: BT.601 limited range, chroma from the top-left pixel
// of each 2x2 block).
func ToI420(img Image) []byte {
	dst := make([]byte, libretro.FrameSizeI420(img.W, img.H))
	libretro.ToI420(dst, libretro.Frame{Data: XRGB(img), Width: img.W, Height: img.H, Pitch: img.W * 4, Format: libretro.FormatXRGB8888})
	return dst
}

// ToI420Box converts like ToI420 but each chroma sample is the average of
// its 2x2 block, centered on it (what most video tools do).
func ToI420Box(img Image) []byte {
	dst := ToI420(img)
	w, h := img.W, img.H
	cw, ch := (w+1)/2, (h+1)/2
	u := dst[w*h : w*h+cw*ch]
	v := dst[w*h+cw*ch:]
	for cy := 0; cy < ch; cy++ {
		for cx := 0; cx < cw; cx++ {
			var su, sv, n int
			for dy := 0; dy < 2; dy++ {
				for dx := 0; dx < 2; dx++ {
					x, y := cx*2+dx, cy*2+dy
					if x >= w || y >= h {
						continue
					}
					o := (y*w + x) * 3
					r, g, b := int(img.Pix[o]), int(img.Pix[o+1]), int(img.Pix[o+2])
					su += (-38*r-74*g+112*b+128)>>8 + 128
					sv += (112*r-94*g-18*b+128)>>8 + 128
					n++
				}
			}
			u[cy*cw+cx] = byte((su + n/2) / n)
			v[cy*cw+cx] = byte((sv + n/2) / n)
		}
	}
	return dst
}

// Upsample is how a player spreads each chroma sample over its 2x2 block
// when it turns the decoded picture back into RGB.
type Upsample int

const (
	// Bilinear samples the chroma plane with bilinear filtering, each
	// sample at the center of its 2x2 block: GPU texture sampling, the
	// usual path for a playing video.
	Bilinear Upsample = iota
	// Replicate repeats each chroma sample over its 2x2 block (libyuv's
	// software conversion).
	Replicate
)

// FromI420 converts a decoded I420 frame back to RGB the way a browser
// draws it: BT.601 limited range, chroma spread by mode.
func FromI420(i420 []byte, w, h int, mode Upsample) Image {
	cw, ch := (w+1)/2, (h+1)/2
	yp := i420[:w*h]
	up := i420[w*h : w*h+cw*ch]
	vp := i420[w*h+cw*ch:]
	img := NewImage(w, h)
	chroma := func(plane []byte, x, y int) float64 {
		if mode == Replicate {
			return float64(plane[(y/2)*cw+x/2])
		}
		// Position in chroma samples, centers at (i+0.5)*2 luma pixels.
		fx := (float64(x)+0.5)/2 - 0.5
		fy := (float64(y)+0.5)/2 - 0.5
		x0, y0 := floor(fx), floor(fy)
		ax, ay := fx-float64(x0), fy-float64(y0)
		at := func(xx, yy int) float64 {
			xx, yy = max(0, min(xx, cw-1)), max(0, min(yy, ch-1))
			return float64(plane[yy*cw+xx])
		}
		top := at(x0, y0)*(1-ax) + at(x0+1, y0)*ax
		bot := at(x0, y0+1)*(1-ax) + at(x0+1, y0+1)*ax
		return top*(1-ay) + bot*ay
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			yy := 1.164 * (float64(yp[y*w+x]) - 16)
			u := chroma(up, x, y) - 128
			v := chroma(vp, x, y) - 128
			o := (y*w + x) * 3
			img.Pix[o] = clamp8(yy + 1.596*v)
			img.Pix[o+1] = clamp8(yy - 0.391*u - 0.813*v)
			img.Pix[o+2] = clamp8(yy + 2.018*u)
		}
	}
	return img
}

func floor(f float64) int {
	i := int(f)
	if f < 0 && float64(i) != f {
		i--
	}
	return i
}

func clamp8(f float64) byte {
	switch {
	case f <= 0:
		return 0
	case f >= 255:
		return 255
	}
	return byte(f + 0.5)
}
