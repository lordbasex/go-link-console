// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/lordbasex/go-link-console/backend-device/pkg/framelab"
)

// runCompare prints the metrics of B against the reference A, as JSON
// lines. A and B are PNG files, or folders compared file by file. -down N
// first averages B down by N (for a stream sent N times larger).
func runCompare(args []string) error {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	down := fs.Int("down", 1, "scale B down by this integer factor (2x2 average) before comparing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 || *down < 1 {
		return errors.New("compare: want [-down N] and two PNG files or two folders")
	}
	a, b := fs.Arg(0), fs.Arg(1)
	st, err := os.Stat(a)
	if err != nil {
		return err
	}
	pairs := [][2]string{{a, b}}
	if st.IsDir() {
		pairs = nil
		names, err := filepath.Glob(filepath.Join(a, "*.png"))
		if err != nil {
			return err
		}
		slices.Sort(names)
		for _, n := range names {
			other := filepath.Join(b, filepath.Base(n))
			if _, err := os.Stat(other); err == nil {
				pairs = append(pairs, [2]string{n, other})
			}
		}
	}
	enc := json.NewEncoder(os.Stdout)
	for _, p := range pairs {
		ra, err := framelab.ReadPNG(p[0])
		if err != nil {
			return err
		}
		rb, err := framelab.ReadPNG(p[1])
		if err != nil {
			return err
		}
		if *down > 1 {
			rb = framelab.BoxDown(rb, *down)
		}
		m, err := framelab.Compare(ra, rb)
		if err != nil {
			return fmt.Errorf("%s: %w", p[1], err)
		}
		if err := enc.Encode(struct {
			A, B string
			framelab.Metrics
		}{p[0], p[1], m}); err != nil {
			return err
		}
	}
	return nil
}

// runCrop cuts a rectangle and zooms it with nearest neighbour, to look
// at single pixels.
func runCrop(args []string) error {
	fs := flag.NewFlagSet("crop", flag.ContinueOnError)
	in := fs.String("in", "", "input PNG")
	rect := fs.String("rect", "", "X,Y,W,H")
	zoom := fs.Int("zoom", 4, "integer zoom")
	out := fs.String("out", "", "output PNG")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var r [4]int
	parts := strings.Split(*rect, ",")
	if *in == "" || *out == "" || len(parts) != 4 || *zoom < 1 {
		return errors.New("crop: -in, -out, -rect X,Y,W,H and a positive -zoom are required")
	}
	for i, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return fmt.Errorf("crop: bad rect %q", *rect)
		}
		r[i] = n
	}
	img, err := framelab.ReadPNG(*in)
	if err != nil {
		return err
	}
	return framelab.WritePNG(*out, framelab.Nearest(framelab.Crop(img, r[0], r[1], r[2], r[3]), *zoom))
}

// runHotspot prints the X,Y,W,H window where IMG differs most from REF
// (IMG is averaged down first with -down), for crop -rect.
func runHotspot(args []string) error {
	fs := flag.NewFlagSet("hotspot", flag.ContinueOnError)
	ref := fs.String("ref", "", "reference PNG")
	img := fs.String("img", "", "processed PNG")
	down := fs.Int("down", 1, "scale IMG down by this integer factor first")
	w := fs.Int("w", 64, "window width")
	h := fs.Int("h", 48, "window height")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ref == "" || *img == "" || *down < 1 || *w < 1 || *h < 1 {
		return errors.New("hotspot: -ref and -img are required")
	}
	a, err := framelab.ReadPNG(*ref)
	if err != nil {
		return err
	}
	b, err := framelab.ReadPNG(*img)
	if err != nil {
		return err
	}
	if *down > 1 {
		b = framelab.BoxDown(b, *down)
	}
	if a.W != b.W || a.H != b.H {
		return fmt.Errorf("hotspot: size %dx%d vs %dx%d", a.W, a.H, b.W, b.H)
	}
	x, y := framelab.Hotspot(a, b, *w, *h)
	fmt.Printf("%d,%d,%d,%d\n", x, y, min(*w, a.W), min(*h, a.H))
	return nil
}

// VMAFResult holds libvmaf's pooled scores.
type VMAFResult struct {
	VMAF    float64 `json:"vmaf"`
	MSSSIM  float64 `json:"ms_ssim"`
	Scaling string  `json:"scaling"`
}

// runVMAF scores the decoded clip against the reference clip with ffmpeg's
// libvmaf. Both are scaled to 1080 lines with nearest neighbour first (the
// VMAF model expects a 1080p viewing size, and nearest keeps the pixel art
// as a sharp display shows it).
func runVMAF(refDir, decDir, logPath string) (*VMAFResult, error) {
	cmd := exec.Command("ffmpeg", "-v", "error",
		"-framerate", "60", "-pattern_type", "glob", "-i", filepath.Join(decDir, "*.png"),
		"-framerate", "60", "-pattern_type", "glob", "-i", filepath.Join(refDir, "*.png"),
		"-lavfi", "[0:v]scale=-2:1080:flags=neighbor,format=yuv420p[d];[1:v]scale=-2:1080:flags=neighbor,format=yuv420p[r];"+
			"[d][r]libvmaf=feature=name=float_ms_ssim:log_fmt=json:log_path="+logPath,
		"-f", "null", "-")
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, out)
	}
	b, err := os.ReadFile(logPath)
	if err != nil {
		return nil, err
	}
	var log struct {
		Pooled map[string]struct {
			Mean float64 `json:"mean"`
		} `json:"pooled_metrics"`
	}
	if err := json.Unmarshal(b, &log); err != nil {
		return nil, err
	}
	return &VMAFResult{VMAF: log.Pooled["vmaf"].Mean, MSSSIM: log.Pooled["float_ms_ssim"].Mean, Scaling: "nearest to 1080 lines"}, nil
}
