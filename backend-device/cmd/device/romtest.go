// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash/fnv"
	"image"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/internal/services"
	"github.com/lordbasex/go-link-console/backend-device/pkg/cores"
	"github.com/lordbasex/go-link-console/backend-device/pkg/emuproc"
	"github.com/lordbasex/go-link-console/backend-device/pkg/framelab"
	"github.com/lordbasex/go-link-console/backend-device/pkg/libretro"
	"github.com/lordbasex/go-link-console/backend-device/pkg/romcheck"
)

// cmdRomTest is "device romtest": the power-on test of a ROM set with the
// exact core (validation level 4), from the command line. With --child it
// is instead the test's worker process, which the device (or this command)
// starts for each run, like a game room's "emulate" worker.
//
//	device romtest [--frames N] [--json] [--shot FILE] [--core PATH] ZIP
//
// With --input it replays an input script instead (experiment 1's
// harness, docs/experiments/harness.md): the JSON script of the simulator
// runner, frame by frame on the exact core, saving the --checkpoints
// frames (and every --png-every frame) as PNG in --frames-dir and, with
// --mp4, every frame as a video.
//
//	device romtest --input FILE [--frames N] [--checkpoints 300,600]
//	    [--frames-dir DIR] [--png-every K] [--mp4 FILE] [--json] ZIP
func cmdRomTest(args []string) error {
	if slices.Contains(args, "--child") {
		return runRomTestChild(args)
	}
	fs := flag.NewFlagSet("device romtest", flag.ContinueOnError)
	frames := fs.Int("frames", services.RomTestFrames, "frames to run (600 to 3600)")
	asJSON := fs.Bool("json", false, "print JSON")
	shot := fs.String("shot", "", "save the last frame as this PNG")
	corePath := fs.String("core", "", "the libretro core (default: the one in ~/go-link/cores)")
	inputFile := fs.String("input", "", "replay this input script (JSON) instead of the power-on test")
	checkpoints := fs.String("checkpoints", "", "replay: frames to save as PNG, e.g. 300,600 (default: the script's)")
	framesDir := fs.String("frames-dir", "", "replay: the folder for the PNG frames")
	pngEvery := fs.Int("png-every", 0, "replay: also save every K-th frame")
	mp4 := fs.String("mp4", "", "replay: record every frame into this MP4 (needs ffmpeg)")
	wav := fs.String("wav", "", "replay: record the core's stereo sound into this WAV (48 kHz)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	zips := fs.Args()
	if len(zips) != 1 {
		return errors.New("usage: device romtest [--frames N] [--json] [--shot FILE] [--core PATH] ZIP\n       device romtest --input FILE [--frames N] [--checkpoints LIST] [--frames-dir DIR] [--png-every K] [--mp4 FILE] [--wav FILE] [--json] ZIP")
	}
	zipPath := zips[0]
	base, err := dataDir()
	if err != nil {
		return err
	}
	coresDir := filepath.Join(base, "cores")
	if *corePath == "" {
		*corePath = filepath.Join(coresDir, cores.FileName(cores.DefaultCore, runtime.GOOS))
	} else {
		coresDir = filepath.Dir(*corePath)
	}
	if _, err := os.Stat(*corePath); err != nil {
		return errors.New("the emulator core is not installed; run: device core download")
	}
	if *inputFile != "" {
		o := replayOptions{Input: *inputFile, FramesDir: *framesDir, PNGEvery: *pngEvery, MP4: *mp4, WAV: *wav, Core: *corePath,
			Work: filepath.Join(base, "tmp", fmt.Sprintf("romreplay-%d", os.Getpid()))}
		frameSet := false
		fs.Visit(func(f *flag.Flag) { frameSet = frameSet || f.Name == "frames" })
		if frameSet {
			o.Frames = *frames
		}
		if *checkpoints != "" {
			cps, err := parseCheckpoints(*checkpoints)
			if err != nil {
				return err
			}
			o.Checkpoints = cps
		}
		rep, err := runReplay(zipPath, o)
		if err != nil {
			rep.Error = err.Error()
		}
		if *asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(rep); err != nil {
				return err
			}
		} else if err == nil {
			fmt.Printf("Replay of %s on %s: %d frames (%dx%d at %.2f Hz) in %.1f s, %d PNG\n", filepath.Base(zipPath), rep.Core, rep.Frames, rep.Width, rep.Height, rep.FPS, rep.Seconds, rep.PNGs)
			for _, c := range rep.Checkpoints {
				fmt.Printf("  frame %d  %s  %s\n", c.Frame, c.Hash, c.PNG)
			}
		}
		return err
	}
	cat, _ := romcheck.Load(filepath.Join(coresDir, romcheck.FileName)) // nil: the set check is skipped
	tests := services.NewRomTestService(services.RomTestConfig{
		Dir:      filepath.Join(base, "tmp", "romtest"),
		CorePath: func() string { return *corePath },
		Catalog:  func() *romcheck.Catalog { return cat },
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)), // the report says it all
	})
	ctx, stop := interruptible()
	defer stop()
	res := tests.RunFile(ctx, zipPath, *frames)
	if *shot != "" && res.Shot != "" {
		if b, err := base64.StdEncoding.DecodeString(res.Shot); err == nil {
			if err := os.WriteFile(*shot, b, 0o644); err != nil {
				return err
			}
		}
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(res); err != nil {
			return err
		}
	} else {
		printRomTest(os.Stdout, zipPath, res)
	}
	if !res.OK {
		return errors.New("the ROM test failed")
	}
	return nil
}

func printRomTest(w io.Writer, zip string, res services.RomTestResult) {
	fmt.Fprintf(w, "ROM test of %s (%s)\n", filepath.Base(zip), res.Set)
	for _, s := range res.Steps {
		mark := "ok  "
		if !s.OK {
			mark = "FAIL"
		}
		line := fmt.Sprintf("  %s  %-14s", mark, s.Name)
		if s.Detail != "" {
			line += "  " + s.Detail
		}
		fmt.Fprintln(w, strings.TrimRight(line, " "))
	}
	if res.Error != "" {
		fmt.Fprintln(w, "  error:", res.Error)
	}
	verdict := "PASSED: the set powers on in this core."
	if !res.OK {
		verdict = "FAILED."
	}
	fmt.Fprintf(w, "%s %d frames in %.1f s.\n", verdict, res.Frames, res.Seconds)
}

// runRomTestChild is the worker: it powers the set on, runs it unpaced
// for --frames frames with the scripted input and prints one
// emuproc.RomTestReport line on stdout.
func runRomTestChild(args []string) error {
	fs := flag.NewFlagSet("romtest --child", flag.ContinueOnError)
	_ = fs.Bool("child", true, "run as the test's worker process")
	core := fs.String("core", "", "libretro core library")
	rom := fs.String("rom", "", "ROM set to run")
	system := fs.String("system", "", "a fresh system folder")
	frames := fs.Int("frames", services.RomTestFrames, "frames to run")
	script := fs.String("script", "", "scripted input (framelab syntax)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *core == "" || *rom == "" || *system == "" || *frames <= 0 {
		return errors.New("romtest --child: --core, --rom, --system and --frames are required")
	}
	sc, err := framelab.ParseScript(*script)
	if err != nil {
		return err
	}
	rep := romTestRun(*core, *rom, *system, *frames, sc)
	return json.NewEncoder(os.Stdout).Encode(rep)
}

// romTestRun runs one power-on of the set and reports what it saw.
func romTestRun(core, rom, system string, frames int, sc framelab.Script) emuproc.RomTestReport {
	rep := emuproc.RomTestReport{FirstPicture: -1, FirstSound: -1}
	var (
		last  framelab.Image
		frame int
		lit   bool // the last picture is not black
	)
	game, err := services.OpenGameCore(services.GameCoreConfig{
		CorePath: core, RomPath: rom, SystemDir: system,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		LogLine: func(_ int, msg string) {
			name, verdict := loaderLine(msg)
			switch verdict {
			case "not_found":
				rep.NotFound = appendOnce(rep.NotFound, name)
			case "bad_length":
				rep.BadLength = appendOnce(rep.BadLength, name)
			case "wrong_checksum":
				rep.WrongChecksum = appendOnce(rep.WrongChecksum, name)
			}
		},
		RawVideo: func(f libretro.Frame) {
			if f.Data == nil {
				return // the core repeats the previous picture
			}
			if last.W != f.Width || last.H != f.Height {
				last = framelab.NewImage(f.Width, f.Height)
			}
			libretro.ToRGB(last.Pix, f)
			lit = !black(last)
		},
		Audio: func(pcm []int16) {
			rep.AudioCalls++
			rep.AudioSamples += len(pcm)
			for _, v := range pcm {
				if v != 0 {
					rep.Sound++
					if rep.FirstSound < 0 {
						rep.FirstSound = frame
					}
				}
			}
		},
	})
	if err != nil {
		rep.Error = err.Error()
		return rep
	}
	defer game.Close()
	rep.Loaded = true
	rep.Core, rep.CoreVersion = game.Info().Name, game.Info().Version
	rep.FPS = game.AV().FPS
	rep.Hashes = make([]uint32, 0, frames)
	start := time.Now()
	for frame = 0; frame < frames; frame++ {
		game.SetPads(sc.Pads(frame))
		game.Run()
		if last.Pix == nil {
			rep.Hashes = append(rep.Hashes, 0)
			continue
		}
		if lit && rep.FirstPicture < 0 {
			rep.FirstPicture = frame
		}
		h := fnv.New32a()
		_, _ = h.Write(last.Pix)
		rep.Hashes = append(rep.Hashes, h.Sum32())
	}
	rep.Seconds = time.Since(start).Seconds()
	rep.Frames = frames
	rep.Width, rep.Height = last.W, last.H
	if last.Pix != nil {
		rep.Shot = shotPNG(last)
	}
	return rep
}

// loaderLine reads the core's ROM loader verdict on one file, e.g.
// "[MAME 2003+] mbe_23e.rom  WRONG CHECKSUMS:".
func loaderLine(msg string) (name, verdict string) {
	msg = strings.TrimSpace(msg)
	if i := strings.Index(msg, "] "); strings.HasPrefix(msg, "[") && i > 0 {
		msg = strings.TrimSpace(msg[i+2:])
	}
	upper := strings.ToUpper(msg)
	switch {
	case strings.Contains(upper, "NOT FOUND"):
		verdict = "not_found"
	case strings.Contains(upper, "LENGTH") && (strings.Contains(upper, "WRONG") || strings.Contains(upper, "INCORRECT")):
		verdict = "bad_length"
	case strings.Contains(upper, "WRONG CHECKSUM"):
		verdict = "wrong_checksum"
	default:
		return "", ""
	}
	fields := strings.Fields(msg)
	if len(fields) == 0 || strings.ContainsAny(fields[0], ":()") {
		return "", ""
	}
	// The core's own data files (hiscore.dat, cheat.dat...) are optional
	// and not part of a set.
	if strings.EqualFold(filepath.Ext(fields[0]), ".dat") {
		return "", ""
	}
	return strings.ToLower(fields[0]), verdict
}

func appendOnce(list []string, s string) []string {
	if s == "" || slices.Contains(list, s) || len(list) >= 64 {
		return list
	}
	return append(list, s)
}

// black reports whether a picture has (almost) nothing on it: fewer than
// 1 in 200 pixels brighter than a dark grey.
func black(img framelab.Image) bool {
	lit := 0
	for i := 0; i+2 < len(img.Pix); i += 3 {
		if img.Pix[i] > 24 || img.Pix[i+1] > 24 || img.Pix[i+2] > 24 {
			lit++
		}
	}
	return lit*200 < img.W*img.H
}

// shotPNG is a PNG of the picture, halved when it is large, so it fits in
// one control message.
func shotPNG(img framelab.Image) string {
	for range 3 {
		rgba := image.NewNRGBA(image.Rect(0, 0, img.W, img.H))
		for i, j := 0, 0; i+2 < len(img.Pix); i, j = i+3, j+4 {
			rgba.Pix[j], rgba.Pix[j+1], rgba.Pix[j+2], rgba.Pix[j+3] = img.Pix[i], img.Pix[i+1], img.Pix[i+2], 255
		}
		var buf bytes.Buffer
		if png.Encode(&buf, rgba) != nil {
			return ""
		}
		if buf.Len() <= services.RomTestMaxShot {
			return base64.StdEncoding.EncodeToString(buf.Bytes())
		}
		img = framelab.BoxDown(img, 2)
	}
	return ""
}
