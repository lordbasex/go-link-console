// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"image"
	"image/png"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/internal/services"
	"github.com/lordbasex/go-link-console/backend-device/pkg/framelab"
	"github.com/lordbasex/go-link-console/backend-device/pkg/libretro"
)

// replayScript is an input script of experiment 1's harness
// (docs/experiments/harness.md): the same JSON the simulator runner
// (rom/tools/lab/run.mjs) reads and writes as inputs.json.
type replayScript struct {
	Frames      int   `json:"frames"`
	Checkpoints []int `json:"checkpoints"`
	Steps       []struct {
		From    int      `json:"from"`
		To      int      `json:"to"`
		Port    int      `json:"port"`
		Buttons []string `json:"buttons"`
	} `json:"steps"`
}

// replayOptions are romtest's replay flags.
type replayOptions struct {
	Input       string
	Frames      int // 0: the script's
	Checkpoints []int
	FramesDir   string
	PNGEvery    int
	MP4         string
	WAV         string // the core's stereo sound, 48 kHz 16-bit (T-26: checking QSound)
	Core        string
	Work        string // a scratch folder for the set copy and the system folder
}

// replayReport is what a replay prints with --json.
type replayReport struct {
	Type        string             `json:"type"`
	Set         string             `json:"set"`
	Core        string             `json:"core"`
	FPS         float64            `json:"fps"`
	Width       int                `json:"width"`
	Height      int                `json:"height"`
	Frames      int                `json:"frames"`
	Seconds     float64            `json:"seconds"`
	Checkpoints []replayCheckpoint `json:"checkpoints"`
	PNGs        int                `json:"pngs"`
	MP4         string             `json:"mp4,omitempty"`
	WAV         string             `json:"wav,omitempty"`
	Error       string             `json:"error,omitempty"`
}

type replayCheckpoint struct {
	Frame int    `json:"frame"`
	PNG   string `json:"png"`
	Hash  string `json:"hash"`
}

// parseCheckpoints reads "300,600,900".
func parseCheckpoints(s string) ([]int, error) {
	var out []int
	for f := range strings.SplitSeq(s, ",") {
		if f = strings.TrimSpace(f); f == "" {
			continue
		}
		n, err := strconv.Atoi(f)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("bad checkpoint %q", f)
		}
		out = append(out, n)
	}
	return out, nil
}

// loadReplayScript reads the JSON script and turns it into framelab's.
func loadReplayScript(path string) (replayScript, framelab.Script, error) {
	var js replayScript
	b, err := os.ReadFile(path)
	if err != nil {
		return js, nil, err
	}
	if err := json.Unmarshal(b, &js); err != nil {
		return js, nil, fmt.Errorf("%s: %w", path, err)
	}
	var text []string
	for _, st := range js.Steps {
		if len(st.Buttons) == 0 {
			continue
		}
		port := st.Port
		if port == 0 {
			port = 1
		}
		text = append(text, fmt.Sprintf("%d-%d:%s@%d", st.From, st.To, strings.Join(st.Buttons, "+"), port))
	}
	sc, err := framelab.ParseScript(strings.Join(text, " "))
	if err != nil {
		return js, nil, fmt.Errorf("%s: %w", path, err)
	}
	return js, sc, nil
}

// runReplay powers the set on with the exact core, applies the script
// frame by frame (unpaced) and saves the checkpoint frames as PNG. Frame
// N is the picture after N frames have run, as in the simulator runner.
func runReplay(zip string, o replayOptions) (rep replayReport, err error) {
	rep = replayReport{Type: "rom_replay", Checkpoints: []replayCheckpoint{}}
	js, sc, err := loadReplayScript(o.Input)
	if err != nil {
		return rep, err
	}
	frames := o.Frames
	if frames <= 0 {
		frames = js.Frames
	}
	if frames <= 0 {
		return rep, errors.New("the number of frames is missing: pass --frames or put \"frames\" in the script")
	}
	cps := o.Checkpoints
	if cps == nil {
		cps = js.Checkpoints
	}
	set, err := replaySetName(zip)
	if err != nil {
		return rep, err
	}
	rep.Set = set
	if err := os.MkdirAll(o.Work, 0o755); err != nil {
		return rep, err
	}
	defer os.RemoveAll(o.Work)
	// The core reads the set by its name, from a folder holding only it.
	rom := filepath.Join(o.Work, set+".zip")
	if err := copyFile(zip, rom); err != nil {
		return rep, err
	}
	if o.FramesDir != "" {
		if err := os.MkdirAll(o.FramesDir, 0o755); err != nil {
			return rep, err
		}
	}

	var last framelab.Image
	var pcm []int16
	game, err := services.OpenGameCore(services.GameCoreConfig{
		Audio: func(s []int16) {
			if o.WAV != "" {
				pcm = append(pcm, s...)
			}
		},
		CorePath: o.Core, RomPath: rom, SystemDir: filepath.Join(o.Work, "system"),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		RawVideo: func(f libretro.Frame) {
			if f.Data == nil {
				return
			}
			if last.W != f.Width || last.H != f.Height {
				last = framelab.NewImage(f.Width, f.Height)
			}
			libretro.ToRGB(last.Pix, f)
		},
	})
	if err != nil {
		return rep, err
	}
	defer game.Close()
	rep.Core = strings.TrimSpace(game.Info().Name + " " + game.Info().Version)
	rep.FPS = game.AV().FPS

	var video *replayVideo
	start := time.Now()
	for frame := 0; frame < frames; frame++ {
		game.SetPads(sc.Pads(frame))
		game.Run()
		n := frame + 1
		if last.Pix == nil {
			continue
		}
		if o.MP4 != "" && video == nil {
			if video, err = startReplayVideo(o.MP4, last.W, last.H, rep.FPS); err != nil {
				return rep, err
			}
		}
		if video != nil {
			if err := video.write(last.Pix); err != nil {
				return rep, err
			}
		}
		isCP := slices.Contains(cps, n)
		if o.FramesDir != "" && (isCP || (o.PNGEvery > 0 && n%o.PNGEvery == 0)) {
			file := filepath.Join(o.FramesDir, fmt.Sprintf("f%06d.png", n))
			if err := writeRGBPNG(file, last); err != nil {
				return rep, err
			}
			rep.PNGs++
			if isCP {
				h := fnv.New32a()
				_, _ = h.Write(last.Pix)
				rep.Checkpoints = append(rep.Checkpoints, replayCheckpoint{Frame: n, PNG: file, Hash: fmt.Sprintf("%08x", h.Sum32())})
			}
		}
	}
	rep.Seconds = time.Since(start).Seconds()
	rep.Frames = frames
	rep.Width, rep.Height = last.W, last.H
	if video != nil {
		if err := video.close(); err != nil {
			return rep, err
		}
		rep.MP4 = o.MP4
	}
	if o.WAV != "" {
		if err := writeWAV(o.WAV, pcm); err != nil {
			return rep, err
		}
		rep.WAV = o.WAV
	}
	return rep, nil
}

// writeWAV writes interleaved stereo 16-bit samples at 48 kHz as a WAV file.
func writeWAV(file string, pcm []int16) error {
	data := make([]byte, 44+len(pcm)*2)
	le := binary.LittleEndian
	copy(data[0:], "RIFF")
	le.PutUint32(data[4:], uint32(36+len(pcm)*2))
	copy(data[8:], "WAVEfmt ")
	le.PutUint32(data[16:], 16)
	le.PutUint16(data[20:], 1) // PCM
	le.PutUint16(data[22:], 2)
	le.PutUint32(data[24:], 48000)
	le.PutUint32(data[28:], 48000*4)
	le.PutUint16(data[32:], 4)
	le.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	le.PutUint32(data[40:], uint32(len(pcm)*2))
	for i, v := range pcm {
		le.PutUint16(data[44+i*2:], uint16(v))
	}
	return os.WriteFile(file, data, 0o644)
}

// replaySetName is the set a zip is, from its file name.
func replaySetName(zip string) (string, error) {
	base := strings.ToLower(filepath.Base(zip))
	name, ok := strings.CutSuffix(base, ".zip")
	if !ok || name == "" || strings.ContainsAny(name, `/\ `) {
		return "", fmt.Errorf("%s: the zip's name must be the set's (e.g. slammast.zip)", zip)
	}
	return name, nil
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o600)
}

func writeRGBPNG(file string, img framelab.Image) error {
	rgba := image.NewNRGBA(image.Rect(0, 0, img.W, img.H))
	for i, j := 0, 0; i+2 < len(img.Pix); i, j = i+3, j+4 {
		rgba.Pix[j], rgba.Pix[j+1], rgba.Pix[j+2], rgba.Pix[j+3] = img.Pix[i], img.Pix[i+1], img.Pix[i+2], 255
	}
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	if err := png.Encode(f, rgba); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// replayVideo pipes raw RGB frames into ffmpeg, doubled with nearest
// neighbour like the simulator runner's run.mp4.
type replayVideo struct {
	cmd *exec.Cmd
	in  io.WriteCloser
}

func ffmpegPath() string {
	if p := os.Getenv("FFMPEG"); p != "" {
		return p
	}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p
	}
	return "/usr/local/bin/ffmpeg"
}

func startReplayVideo(file string, w, h int, fps float64) (*replayVideo, error) {
	if fps <= 0 {
		fps = 60
	}
	cmd := exec.Command(ffmpegPath(), "-y", "-loglevel", "error", "-f", "rawvideo", "-pix_fmt", "rgb24",
		"-s", fmt.Sprintf("%dx%d", w, h), "-r", strconv.FormatFloat(fps, 'f', 3, 64), "-i", "-",
		"-vf", fmt.Sprintf("scale=%d:%d:flags=neighbor", w*2, h*2), "-c:v", "libx264", "-preset", "veryfast",
		"-crf", "18", "-pix_fmt", "yuv420p", "-movflags", "+faststart", file)
	cmd.Stderr = os.Stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w", err)
	}
	return &replayVideo{cmd: cmd, in: in}, nil
}

func (v *replayVideo) write(pix []byte) error {
	_, err := v.in.Write(pix)
	return err
}

func (v *replayVideo) close() error {
	_ = v.in.Close()
	return v.cmd.Wait()
}
