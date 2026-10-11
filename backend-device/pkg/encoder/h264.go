// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package encoder

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// H264 encodes I420 frames to H.264 with ffmpeg running as its own process
// (go-link HD, docs/experiments/hd-streaming.md): x264 in software, the
// fastest way to 4K on Apple Silicon, or the computer's hardware encoder
// (VideoToolbox on a Mac, Media Foundation on Windows, both called
// directly, without ffmpeg). ffmpeg stays a separate program the device talks
// to through pipes, never linked into it, so go-link keeps its own license
// whatever ffmpeg was built with. Frames go in with Write; each access unit
// (one frame, Annex B, with an access unit delimiter in front) comes back to
// onFrame from a goroutine as soon as the next one starts, so the stream is
// one frame behind the encoder. The SPS and PPS ride with every keyframe,
// one every two seconds, which is how a new viewer starts the picture.
type H264 struct {
	vt      *vtEncoder // VideoToolbox called directly (macOS)
	mf      *mfEncoder // Media Foundation called directly (Windows); with neither, ffmpeg below
	cmd     *exec.Cmd
	in      io.WriteCloser
	done    chan struct{}
	mu      sync.Mutex
	closed  bool
	frameSz int
}

// H264Encoders are the encoders H264 can drive: "x264" (software, through
// ffmpeg), "videotoolbox" (the Mac's hardware: called directly on macOS, no
// ffmpeg needed), "mediafoundation" (Windows' encoders, called directly:
// the graphics card's when there is one, else Windows' software encoder),
// and on Linux, through ffmpeg, "nvenc" (an NVIDIA card's) and "vaapi"
// (Intel's and AMD's, through /dev/dri).
var H264Encoders = []string{"x264", "videotoolbox", "mediafoundation", "nvenc", "vaapi"}

// VAAPIDevice is the render node ffmpeg's VAAPI encoder opens: $VAAPI_DEVICE,
// else the first /dev/dri/renderD* there is.
func VAAPIDevice() string {
	if d := os.Getenv("VAAPI_DEVICE"); d != "" {
		return d
	}
	for n := 128; n < 136; n++ {
		d := "/dev/dri/renderD" + strconv.Itoa(n)
		if _, err := os.Stat(d); err == nil {
			return d
		}
	}
	return ""
}

// FFmpegPath finds ffmpeg: $FFMPEG, the PATH, then Homebrew's places (an
// app opened from the Finder has a short PATH).
func FFmpegPath() (string, error) {
	if p := os.Getenv("FFMPEG"); p != "" {
		return p, nil
	}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p, nil
	}
	for _, p := range []string{"/opt/homebrew/bin/ffmpeg", "/usr/local/bin/ffmpeg", "/usr/bin/ffmpeg"} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", errors.New("h264: ffmpeg not found (install it, or set FFMPEG)")
}

// NewH264 starts ffmpeg with encoder kind ("x264" or "videotoolbox").
// onFrame gets each encoded frame; it runs on the reader goroutine.
func NewH264(cfg Config, kind string, onFrame func([]byte)) (*H264, error) {
	return newH264(cfg, kind, onFrame, os.Stderr)
}

// newH264 is NewH264 with ffmpeg's messages going to stderr (nil drops them).
func newH264(cfg Config, kind string, onFrame func([]byte), stderr io.Writer) (*H264, error) {
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width%2 != 0 || cfg.Height%2 != 0 {
		return nil, fmt.Errorf("h264: bad size %dx%d", cfg.Width, cfg.Height)
	}
	if cfg.FPS <= 0 {
		cfg.FPS = 60
	}
	if cfg.BitrateKbps <= 0 {
		cfg.BitrateKbps = 8000
	}
	if kind == "videotoolbox" && hasVT {
		gop := cfg.FPS * 2
		if cfg.GOPFrames > 0 {
			gop = cfg.GOPFrames
		}
		vt, err := newVT(cfg, gop, onFrame)
		if err != nil {
			return nil, err
		}
		return &H264{vt: vt}, nil
	}
	if kind == "mediafoundation" && !hasMF {
		return nil, errors.New("h264: mediafoundation is only on Windows")
	}
	if kind == "mediafoundation" {
		gop := cfg.FPS * 2
		if cfg.GOPFrames > 0 {
			gop = cfg.GOPFrames
		}
		mf, err := newMF(cfg, gop, true, onFrame)
		if err != nil {
			// no graphics card encoder takes it: Windows' own software encoder
			mf, err = newMF(cfg, gop, false, onFrame)
		}
		if err != nil {
			return nil, err
		}
		return &H264{mf: mf}, nil
	}
	path, err := FFmpegPath()
	if err != nil {
		return nil, err
	}
	rate := strconv.Itoa(cfg.BitrateKbps) + "k"
	gop := strconv.Itoa(cfg.FPS * 2)
	if cfg.GOPFrames > 0 {
		gop = strconv.Itoa(cfg.GOPFrames)
	}
	var codec, pre []string
	switch kind {
	case "x264":
		codec = []string{"-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency", "-profile:v", "baseline", "-x264-params", "aud=1:repeat-headers=1"}
	case "videotoolbox":
		codec = []string{"-c:v", "h264_videotoolbox", "-realtime", "1", "-prio_speed", "1", "-profile:v", "baseline", "-bsf:v", "h264_metadata=aud=insert,dump_extra=freq=keyframe"}
	case "nvenc":
		// the fastest preset, ultra low latency, constant bitrate, no B-frames
		codec = []string{"-c:v", "h264_nvenc", "-preset", "p1", "-tune", "ull", "-zerolatency", "1", "-rc", "cbr", "-profile:v", "baseline", "-bf", "0", "-bsf:v", "h264_metadata=aud=insert,dump_extra=freq=keyframe"}
	case "vaapi":
		dev := VAAPIDevice()
		if dev == "" {
			return nil, errors.New("h264: vaapi: no /dev/dri/renderD* (in Docker: --device /dev/dri)")
		}
		// the frames go up to the card as NV12; one frame in flight at a time
		pre = []string{"-vaapi_device", dev}
		codec = []string{"-vf", "format=nv12,hwupload", "-c:v", "h264_vaapi", "-profile:v", "constrained_baseline", "-rc_mode", "CBR", "-bf", "0", "-async_depth", "1", "-bsf:v", "h264_metadata=aud=insert,dump_extra=freq=keyframe"}
	default:
		return nil, fmt.Errorf("h264: unknown encoder %q (x264, videotoolbox, mediafoundation, nvenc or vaapi)", kind)
	}
	args := append([]string{"-hide_banner", "-loglevel", "error"}, pre...)
	args = append(args, "-f", "rawvideo", "-pix_fmt", "yuv420p", "-s", fmt.Sprintf("%dx%d", cfg.Width, cfg.Height), "-r", strconv.Itoa(cfg.FPS), "-i", "pipe:0")
	args = append(args, codec...)
	args = append(args, "-b:v", rate, "-maxrate", rate, "-bufsize", strconv.Itoa(cfg.BitrateKbps/2)+"k", "-g", gop, "-f", "h264", "-flush_packets", "1", "pipe:1")
	cmd := exec.Command(path, args...)
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("h264: ffmpeg: %w", err)
	}
	e := &H264{cmd: cmd, in: in, done: make(chan struct{}), frameSz: cfg.Width*cfg.Height + 2*(cfg.Width/2)*(cfg.Height/2)}
	go func() {
		defer close(e.done)
		splitAccessUnits(bufio.NewReaderSize(out, 1<<20), onFrame)
	}()
	return e, nil
}

// Write hands one I420 frame to the encoder; it blocks while ffmpeg is busy.
func (e *H264) Write(i420 []byte) error {
	return e.WriteKey(i420, false)
}

// WriteKey is Write, with key asking for a keyframe now (a viewer just
// joined); ffmpeg only makes them on its own schedule (every GOPFrames).
func (e *H264) WriteKey(i420 []byte, key bool) error {
	if e.vt != nil {
		return e.vt.write(i420, key)
	}
	if e.mf != nil {
		return e.mf.write(i420, key)
	}
	if len(i420) < e.frameSz {
		return fmt.Errorf("h264: frame is %d bytes, want %d", len(i420), e.frameSz)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return errors.New("h264: closed")
	}
	_, err := e.in.Write(i420[:e.frameSz])
	return err
}

// Encoder names the encoder in use: Media Foundation's (its graphics card
// encoder, or Windows' software one), else the kind it was asked for.
func (e *H264) Encoder() string {
	if e.mf != nil {
		if e.mf.hardware {
			return "mediafoundation (" + e.mf.name + ")"
		}
		return "mediafoundation (software: " + e.mf.name + ")"
	}
	if e.vt != nil {
		return "videotoolbox"
	}
	return "ffmpeg"
}

// Close stops ffmpeg and waits for the last frame.
func (e *H264) Close() {
	if e.vt != nil {
		e.vt.close()
		return
	}
	if e.mf != nil {
		e.mf.close()
		return
	}
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return
	}
	e.closed = true
	_ = e.in.Close()
	e.mu.Unlock()
	<-e.done
	_ = e.cmd.Wait()
}

// splitAccessUnits reads an Annex B stream and calls onFrame with each
// access unit, cut where the next access unit delimiter (NAL type 9) starts.
func splitAccessUnits(r io.ByteReader, onFrame func([]byte)) {
	var au []byte
	zeros := 0
	for {
		b, err := r.ReadByte()
		if err != nil {
			if len(au) > 0 {
				onFrame(au)
			}
			return
		}
		au = append(au, b)
		if b == 0 {
			zeros++
			continue
		}
		if b == 1 && zeros >= 2 {
			// a start code: peek at the NAL type
			t, err := r.ReadByte()
			if err != nil {
				if len(au) > 0 {
					onFrame(au)
				}
				return
			}
			if t&0x1f == 9 {
				start := len(au) - min(zeros, 3) - 1
				if start > 0 {
					onFrame(au[:start:start])
					rest := append([]byte(nil), au[start:]...)
					au = rest
				}
			}
			au = append(au, t)
		}
		zeros = 0
	}
}

// H264Support says which H.264 encoders the installed ffmpeg has (none
// without ffmpeg). A listed hardware encoder can still refuse a size or be
// busy: the device tries an encoder before it relies on it.
type H264Support struct {
	FFmpeg       string `json:"ffmpeg,omitempty"`
	X264         bool   `json:"x264"`
	VideoToolbox bool   `json:"videotoolbox"`
	NVENC        bool   `json:"nvenc"`
	VAAPI        bool   `json:"vaapi"`
}

// ProbeH264 asks ffmpeg for its encoders.
func ProbeH264() H264Support {
	path, err := FFmpegPath()
	if err != nil {
		return H264Support{}
	}
	out, err := exec.Command(path, "-hide_banner", "-encoders").Output()
	if err != nil {
		return H264Support{FFmpeg: path}
	}
	s := string(out)
	return H264Support{FFmpeg: path, X264: strings.Contains(s, " libx264 "), VideoToolbox: strings.Contains(s, " h264_videotoolbox "),
		NVENC: strings.Contains(s, " h264_nvenc "), VAAPI: strings.Contains(s, " h264_vaapi ")}
}

// TryH264 encodes a short clip with the encoder kind and says whether it
// works: ffmpeg lists NVENC and VAAPI whether or not there is a card (or
// the container was given it), so only a real encode tells.
func TryH264(kind string) error {
	var mu sync.Mutex
	n := 0
	// quiet: an encoder that is not there is not an error (ffmpeg's messages kept for the reason)
	var said bytes.Buffer
	e, err := newH264(Config{Width: 640, Height: 360, FPS: 30, BitrateKbps: 1000}, kind, func([]byte) {
		mu.Lock()
		n++
		mu.Unlock()
	}, &said)
	if err != nil {
		return err
	}
	// why ffmpeg stopped (read once it has exited): the line that names the cause
	// (a driver library it cannot load, no capable device), else its last line
	why := func(err error) error {
		lines := strings.Split(strings.TrimSpace(said.String()), "\n")
		pick := strings.TrimSpace(lines[len(lines)-1])
		for _, l := range lines {
			if strings.Contains(l, "Cannot load") || strings.Contains(l, "No capable") || strings.Contains(l, "No such") || strings.Contains(l, "Failed to initialise") {
				pick = strings.TrimSpace(l)
				break
			}
		}
		if pick != "" {
			return fmt.Errorf("h264: %s: %s", kind, pick)
		}
		return err
	}
	frame := make([]byte, 640*360*3/2)
	for i := 0; i < 10; i++ {
		for k := range frame[:640*360] {
			frame[k] = byte(k + i*7)
		}
		if err := e.Write(frame); err != nil {
			e.Close()
			return why(err)
		}
	}
	e.Close()
	mu.Lock()
	defer mu.Unlock()
	if n < 5 {
		return why(fmt.Errorf("h264: %s gave %d frames for 10", kind, n))
	}
	return nil
}

// HardwareH264 is the computer's hardware H.264 encoder that works, "" when
// there is none: VideoToolbox on macOS; on Windows the graphics card's,
// through Media Foundation; on Linux NVENC, else VAAPI, through ffmpeg,
// each tried with a short clip.
func HardwareH264(goos string) string {
	switch goos {
	case "darwin":
		return "videotoolbox"
	case "windows":
		if MFHardware() != "" {
			return "mediafoundation"
		}
	case "linux":
		s := ProbeH264()
		if s.NVENC && TryH264("nvenc") == nil {
			return "nvenc"
		}
		if s.VAAPI && VAAPIDevice() != "" && TryH264("vaapi") == nil {
			return "vaapi"
		}
	}
	return ""
}
