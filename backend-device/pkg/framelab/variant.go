// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package framelab

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/pkg/encoder"
	"github.com/lordbasex/go-link-console/backend-device/pkg/libretro"
)

// Variant is one way of preparing and encoding the picture. The zero
// tuning values are the device's current streaming settings.
type Variant struct {
	Name  string `json:"name"`
	Codec string `json:"codec"` // "vp8" (pkg/encoder, the device's path) or "vp9" (ffmpeg, for reference)
	Scale int    `json:"scale"` // integer nearest-neighbour upscale before encoding (1 = native)
	// Chroma is "topleft" (the device's conversion) or "box" (2x2 average).
	Chroma      string `json:"chroma"`
	BitrateKbps int    `json:"kbps"`
	MinQ        int    `json:"minq"`
	MaxQ        int    `json:"maxq"`
	CPUUsed     int    `json:"cpu"`
}

// DefaultBitrateKbps is the lab's baseline bitrate: the device's stream
// at the game's size (StreamConfig default, the saver quality).
const DefaultBitrateKbps = 2500

// ParseVariant reads "name=A,scale=2,kbps=5000,minq=4,maxq=56,cpu=8,
// chroma=topleft,codec=vp8". Missing keys keep the device's settings.
func ParseVariant(text string) (Variant, error) {
	v := Variant{Name: "baseline", Codec: "vp8", Scale: 1, Chroma: "topleft", BitrateKbps: DefaultBitrateKbps,
		MinQ: encoder.DefaultMinQuantizer, MaxQ: encoder.DefaultMaxQuantizer, CPUUsed: encoder.DefaultCPUUsed}
	for _, kv := range strings.Split(text, ",") {
		if kv == "" {
			continue
		}
		k, val, ok := strings.Cut(kv, "=")
		if !ok {
			return v, fmt.Errorf("variant %q: want key=value", kv)
		}
		num := func() (int, error) {
			n, err := strconv.Atoi(val)
			if err != nil {
				return 0, fmt.Errorf("variant %s: %q is not a number", k, val)
			}
			return n, nil
		}
		var err error
		switch k {
		case "name":
			v.Name = val
		case "codec":
			v.Codec = val
		case "chroma":
			v.Chroma = val
		case "scale":
			v.Scale, err = num()
		case "kbps":
			v.BitrateKbps, err = num()
		case "minq":
			v.MinQ, err = num()
		case "maxq":
			v.MaxQ, err = num()
		case "cpu":
			v.CPUUsed, err = num()
		default:
			return v, fmt.Errorf("variant: unknown key %q", k)
		}
		if err != nil {
			return v, err
		}
	}
	switch {
	case v.Codec != "vp8" && v.Codec != "vp9":
		return v, fmt.Errorf("variant %s: codec must be vp8 or vp9", v.Name)
	case v.Chroma != "topleft" && v.Chroma != "box":
		return v, fmt.Errorf("variant %s: chroma must be topleft or box", v.Name)
	case v.Scale < 1 || v.Scale > 4:
		return v, fmt.Errorf("variant %s: scale must be 1-4", v.Name)
	case v.BitrateKbps <= 0:
		return v, fmt.Errorf("variant %s: kbps must be positive", v.Name)
	case v.MinQ < 1 || v.MaxQ > 63 || v.MinQ > v.MaxQ:
		return v, fmt.Errorf("variant %s: quantizers must satisfy 1 <= minq <= maxq <= 63", v.Name)
	}
	return v, nil
}

// Prepare turns a reference picture into the encoder's I420 input.
func (v Variant) Prepare(ref Image) (i420 []byte, w, h int) {
	img := ref
	if v.Scale > 1 {
		img = Nearest(ref, v.Scale)
	}
	if v.Chroma == "box" {
		return ToI420Box(img), img.W, img.H
	}
	return ToI420(img), img.W, img.H
}

// PrepareFrame converts a core frame with the device's own one-pass
// conversions when the variant has one: the game's size with top-left
// (libretro.ToI420) or averaged chroma (libretro.ToI420Box), or 2x with
// top-left chroma (libretro.ToI420Double). ok is false for the others,
// which only Prepare makes. Both give the same bytes.
func (v Variant) PrepareFrame(dst []byte, f libretro.Frame) (i420 []byte, w, h int, ok bool) {
	var convert func([]byte, libretro.Frame)
	switch {
	case v.Scale == 1 && v.Chroma == "topleft":
		convert = libretro.ToI420
	case v.Scale == 1 && v.Chroma == "box":
		convert = libretro.ToI420Box
	case v.Scale == 2 && v.Chroma == "topleft":
		convert = libretro.ToI420Double
	default:
		return nil, 0, 0, false
	}
	w, h = f.Width*v.Scale, f.Height*v.Scale
	if size := libretro.FrameSizeI420(w, h); len(dst) != size {
		dst = make([]byte, size)
	}
	convert(dst, f)
	return dst, w, h, true
}

// Frame wraps a picture as the core would hand it over (XRGB8888).
func Frame(img Image) libretro.Frame {
	return libretro.Frame{Data: XRGB(img), Width: img.W, Height: img.H, Pitch: img.W * 4, Format: libretro.FormatXRGB8888}
}

// StreamFPS is the frame rate the device gives the encoder for a game
// running at fps: StreamService takes whole frames per second from the
// frame duration.
func StreamFPS(fps float64) int {
	if fps <= 0 {
		return 60
	}
	dur := time.Duration(float64(time.Second) / fps)
	return max(int(time.Second/dur), 1)
}
