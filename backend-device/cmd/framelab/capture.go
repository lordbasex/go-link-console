// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lordbasex/go-link-console/backend-device/internal/services"
	"github.com/lordbasex/go-link-console/backend-device/pkg/framelab"
	"github.com/lordbasex/go-link-console/backend-device/pkg/libretro"
)

// Meta describes a capture; encode reads it back.
type Meta struct {
	Rom         string  `json:"rom"` // file name only
	Core        string  `json:"core"`
	CoreVersion string  `json:"core_version"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	Aspect      float64 `json:"aspect"` // display aspect ratio the device reports
	FPS         float64 `json:"fps"`
	StreamFPS   int     `json:"stream_fps"` // frame rate the device gives the encoder
	PixelFormat string  `json:"pixel_format"`
	Frames      int     `json:"frames"`
	Script      string  `json:"script"`
	Stills      []int   `json:"stills"`
	ClipFrom    int     `json:"clip_from"`
	ClipCount   int     `json:"clip_count"`
}

// runCapture runs the game exactly like the "emulate" worker (the same
// GameCore: core options, input mapping, frame conversion), as fast as
// the machine allows, and records every frame.
func runCapture(args []string) error {
	fs := flag.NewFlagSet("capture", flag.ContinueOnError)
	core := fs.String("core", "", "the libretro core library")
	rom := fs.String("rom", "", "the game (read only)")
	out := fs.String("out", "", "output folder")
	system := fs.String("system", "", "system folder for the core (default: a fresh folder inside -out, so runs repeat)")
	frames := fs.Int("frames", 3600, "frames to run")
	script := fs.String("script", "", `scripted input, e.g. "300-305:coin 400-405:start 600-3000:play"`)
	stills := fs.String("stills", "", "comma separated frame numbers saved as PNG")
	clip := fs.String("clip", "", "FROM:COUNT consecutive frames saved as PNG")
	logCore := fs.Bool("log", false, "print the core's log to stderr (useful when a ROM does not start)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *core == "" || *rom == "" || *out == "" {
		return errors.New("capture: -core, -rom and -out are required")
	}
	sc, err := framelab.ParseScript(*script)
	if err != nil {
		return err
	}
	stillList, err := parseInts(*stills)
	if err != nil {
		return err
	}
	clipFrom, clipCount, err := parseClip(*clip)
	if err != nil {
		return err
	}
	for _, d := range []string{"stills", "clip"} {
		if err := os.MkdirAll(filepath.Join(*out, d), 0o755); err != nil {
			return err
		}
	}
	if *system == "" {
		*system = filepath.Join(*out, "system")
		if err := os.RemoveAll(*system); err != nil {
			return err
		}
	}
	run, err := framelab.CreateRun(filepath.Join(*out, "run.glrun"))
	if err != nil {
		return err
	}
	var (
		last   framelab.Image
		format libretro.PixelFormat
	)
	game, err := services.OpenGameCore(services.GameCoreConfig{
		CorePath:  *core,
		RomPath:   *rom,
		SystemDir: *system,
		Logger:    captureLogger(*logCore),
		RawVideo: func(f libretro.Frame) {
			if f.Data == nil {
				return // the core repeats the previous picture
			}
			if last.W != f.Width || last.H != f.Height {
				last = framelab.NewImage(f.Width, f.Height)
			}
			libretro.ToRGB(last.Pix, f)
			format = f.Format
		},
	})
	if err != nil {
		run.Close()
		return err
	}
	defer game.Close()
	av := game.AV()
	meta := Meta{
		Rom: filepath.Base(*rom), Core: game.Info().Name, CoreVersion: game.Info().Version,
		Aspect: game.Aspect(), FPS: av.FPS, StreamFPS: framelab.StreamFPS(av.FPS),
		Frames: *frames, Script: *script, Stills: stillList, ClipFrom: clipFrom, ClipCount: clipCount,
	}
	want := map[int]bool{}
	for _, s := range stillList {
		want[s] = true
	}
	for i := 0; i < *frames; i++ {
		game.SetPads(sc.Pads(i))
		game.Run()
		if last.Pix == nil {
			return fmt.Errorf("capture: no picture at frame %d", i)
		}
		if meta.Width == 0 {
			meta.Width, meta.Height = last.W, last.H
		} else if last.W != meta.Width || last.H != meta.Height {
			return fmt.Errorf("capture: the picture changed size at frame %d (%dx%d to %dx%d); the lab needs one size", i, meta.Width, meta.Height, last.W, last.H)
		}
		if err := run.Write(last); err != nil {
			return err
		}
		if want[i] {
			if err := framelab.WritePNG(filepath.Join(*out, "stills", frameName(i)), last); err != nil {
				return err
			}
		}
		if clipCount > 0 && i >= clipFrom && i < clipFrom+clipCount {
			if err := framelab.WritePNG(filepath.Join(*out, "clip", frameName(i)), last); err != nil {
				return err
			}
		}
	}
	if err := run.Close(); err != nil {
		return err
	}
	meta.PixelFormat = map[libretro.PixelFormat]string{libretro.FormatXRGB8888: "XRGB8888", libretro.FormatRGB565: "RGB565", libretro.Format0RGB1555: "0RGB1555"}[format]
	fmt.Printf("captured %d frames of %s: %dx%d, aspect %.4f, %.3f fps (stream %d), %s\n",
		*frames, meta.Rom, meta.Width, meta.Height, meta.Aspect, meta.FPS, meta.StreamFPS, meta.PixelFormat)
	return writeJSON(filepath.Join(*out, "meta.json"), meta)
}

// runExtract saves frames of a run file as PNG.
func runExtract(args []string) error {
	fs := flag.NewFlagSet("extract", flag.ContinueOnError)
	runPath := fs.String("run", "", "run file")
	out := fs.String("out", "", "output folder")
	list := fs.String("frames", "", "comma separated frame numbers")
	every := fs.Int("every", 0, "save every Nth frame")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *runPath == "" || *out == "" || (*list == "" && *every <= 0) {
		return errors.New("extract: -run, -out and -frames or -every are required")
	}
	frames, err := parseInts(*list)
	if err != nil {
		return err
	}
	want := map[int]bool{}
	for _, f := range frames {
		want[f] = true
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	r, err := framelab.OpenRun(*runPath)
	if err != nil {
		return err
	}
	defer r.Close()
	for i := 0; ; i++ {
		img, err := r.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if want[i] || (*every > 0 && i%*every == 0) {
			if err := framelab.WritePNG(filepath.Join(*out, frameName(i)), img); err != nil {
				return err
			}
		}
	}
}

func frameName(i int) string { return fmt.Sprintf("f%06d.png", i) }

func parseInts(s string) ([]int, error) {
	var out []int
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("bad frame number %q", p)
		}
		out = append(out, n)
	}
	return out, nil
}

func parseClip(s string) (from, count int, err error) {
	if s == "" {
		return 0, 0, nil
	}
	a, b, ok := strings.Cut(s, ":")
	from, e1 := strconv.Atoi(a)
	count, e2 := strconv.Atoi(b)
	if !ok || e1 != nil || e2 != nil || from < 0 || count <= 0 {
		return 0, 0, fmt.Errorf("bad clip %q: want FROM:COUNT", s)
	}
	return from, count, nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// captureLogger discards the core's log unless asked to print it.
func captureLogger(show bool) *slog.Logger {
	if show {
		return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
