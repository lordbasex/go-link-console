// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

// go-link HD, the experiment of task T-31 (docs/experiments/hd-streaming.md):
// how far can the device stream a 2D game drawn above the CPS-1's 384 x 224?
// An HD test scene (two pictures scrolling at different speeds, like a far
// and a play layer, and a bouncing ball) is drawn frame by frame at 720p,
// 1080p and 4K and encoded at 60 fps, with the stream's own VP8 encoder
// (libvpx, software) or, through ffmpeg, the computer's hardware encoder
// (VideoToolbox on a Mac) or x264. It reports the encode time per frame,
// the frame rate the encoder can keep, the bitrate and the process CPU.

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"io"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	_ "image/jpeg"
	_ "image/png"

	"github.com/lordbasex/go-link-console/backend-device/pkg/encoder"
	"github.com/lordbasex/go-link-console/backend-device/pkg/hdscene"
)

type hdSize struct {
	Name string
	W, H int
	Kbps int
}

var hdSizes = map[string]hdSize{
	"720p":  {"720p", 1280, 720, 4000},
	"1080p": {"1080p", 1920, 1080, 8000},
	"2160p": {"2160p", 3840, 2160, 25000},
}

type hdResult struct {
	Size        string  `json:"size"`
	Encoder     string  `json:"encoder"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	Frames      int     `json:"frames"`
	TargetKbps  int     `json:"target_kbps"`
	EncodeAvgMs float64 `json:"encode_avg_ms"`
	EncodeP95Ms float64 `json:"encode_p95_ms"`
	EncodeMaxMs float64 `json:"encode_max_ms"`
	DrawAvgMs   float64 `json:"draw_avg_ms"`
	// MaxFPS is how many frames a second the encoder alone could keep (1000 / average).
	MaxFPS   float64 `json:"max_fps"`
	Realtime bool    `json:"realtime_60"` // the 95th percentile fits a 60 fps frame with room for the rest (60 %)
	Kbps     float64 `json:"kbps"`
	CPU      float64 `json:"cpu_percent"` // process CPU over the run (100 = one core)
	Note     string  `json:"note,omitempty"`
}

func cmdHDBench(args []string) error {
	fs := flag.NewFlagSet("hdbench", flag.ContinueOnError)
	far := fs.String("far", "", "far layer picture (PNG or JPEG), scrolling at half speed")
	play := fs.String("play", "", "play layer picture, scrolling at full speed (#FF00FF is transparent)")
	sizes := fs.String("res", "720p,1080p,2160p", "sizes: 720p, 1080p, 2160p")
	enc := fs.String("encoder", "vp8", "vp8 (libvpx, the stream's), videotoolbox (the Mac's hardware, through ffmpeg) or x264 (through ffmpeg)")
	seconds := fs.Int("seconds", 10, "seconds of game to encode at each size")
	fps := fs.Int("fps", 60, "frames per second")
	threads := fs.Int("threads", 0, "vp8: libvpx threads (0 = the stream's 2; try the cores)")
	cpuUsed := fs.Int("cpu-used", 0, "vp8: libvpx speed (0 = the stream's 8; up to 16 is faster)")
	asJSON := fs.Bool("json", false, "print JSON")
	raw := fs.String("raw", "", "write the scene's frames as raw I420 to this file (one size) instead of encoding, to time other encoders on their own")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *far == "" {
		return errors.New("usage: device hdbench --far PICTURE [--play PICTURE] [--res 720p,1080p,2160p] [--encoder vp8|videotoolbox|x264] [--seconds N] [--threads N] [--json]")
	}
	farImg, err := loadPicture(*far)
	if err != nil {
		return err
	}
	var playImg image.Image
	if *play != "" {
		if playImg, err = loadPicture(*play); err != nil {
			return err
		}
	}
	if *raw != "" {
		size, ok := hdSizes[strings.TrimSpace(*sizes)]
		if !ok {
			return fmt.Errorf("hdbench: --raw takes one size, not %q", *sizes)
		}
		return hdRaw(*raw, size, farImg, playImg, *seconds**fps)
	}
	var results []hdResult
	for _, name := range strings.Split(*sizes, ",") {
		size, ok := hdSizes[strings.TrimSpace(name)]
		if !ok {
			return fmt.Errorf("hdbench: unknown size %q", name)
		}
		r, err := hdRun(size, *enc, farImg, playImg, *seconds**fps, *fps, *threads, *cpuUsed)
		if err != nil {
			return err
		}
		results = append(results, r)
		if !*asJSON {
			fmt.Printf("%-6s %-12s %4dx%-4d encode %6.2f ms avg, %6.2f p95, %6.2f max  draw %5.2f ms  max %6.1f fps  realtime 60: %-5v  %7.0f kbps (target %d)  CPU %4.0f%%%s\n",
				r.Size, r.Encoder, r.Width, r.Height, r.EncodeAvgMs, r.EncodeP95Ms, r.EncodeMaxMs, r.DrawAvgMs, r.MaxFPS, r.Realtime, r.Kbps, r.TargetKbps, r.CPU, noteOf(r))
		}
	}
	if *asJSON {
		e := json.NewEncoder(os.Stdout)
		e.SetIndent("", "  ")
		return e.Encode(map[string]any{"type": "hd_bench", "cpus": runtime.NumCPU(), "goos": runtime.GOOS, "goarch": runtime.GOARCH, "results": results})
	}
	return nil
}

func noteOf(r hdResult) string {
	if r.Note == "" {
		return ""
	}
	return "  (" + r.Note + ")"
}

func loadPicture(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

func hdRun(size hdSize, enc string, far, play image.Image, frames, fps, threads, cpuUsed int) (hdResult, error) {
	r := hdResult{Size: size.Name, Encoder: enc, Width: size.W, Height: size.H, Frames: frames, TargetKbps: size.Kbps}
	scene := hdscene.New(size.W, size.H, far, play)
	var i420 []byte
	var encodeTimes []time.Duration
	var drawTotal time.Duration
	var bytes int64
	frameDur := time.Second / time.Duration(fps)

	var encodeFrame func(force bool) error
	var finish func() error
	switch enc {
	case "vp8":
		v, err := encoder.NewVP8(encoder.Config{Width: size.W, Height: size.H, FPS: fps, BitrateKbps: size.Kbps, Threads: threads, CPUUsed: cpuUsed})
		if err != nil {
			return r, err
		}
		defer v.Close()
		if threads > 0 || cpuUsed > 0 {
			r.Encoder = fmt.Sprintf("vp8 t%d c%d", max(threads, 2), cpuUsed)
		}
		encodeFrame = func(force bool) error {
			start := time.Now()
			data, _, err := v.Encode(i420, force)
			encodeTimes = append(encodeTimes, time.Since(start))
			bytes += int64(len(data))
			return err
		}
		finish = func() error { return nil }
	case "videotoolbox", "x264":
		codec := []string{"-c:v", "h264_videotoolbox", "-realtime", "1", "-b:v", fmt.Sprintf("%dk", size.Kbps), "-g", strconv.Itoa(fps * 3)}
		if enc == "x264" {
			codec = []string{"-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency", "-b:v", fmt.Sprintf("%dk", size.Kbps), "-g", strconv.Itoa(fps * 3)}
		}
		cmdArgs := append([]string{"-hide_banner", "-loglevel", "error", "-f", "rawvideo", "-pix_fmt", "yuv420p", "-s", fmt.Sprintf("%dx%d", size.W, size.H), "-r", strconv.Itoa(fps), "-i", "pipe:0"}, codec...)
		cmdArgs = append(cmdArgs, "-f", "h264", "pipe:1")
		cmd := exec.Command(ffmpegPath(), cmdArgs...)
		in, err := cmd.StdinPipe()
		if err != nil {
			return r, err
		}
		out, err := cmd.StdoutPipe()
		if err != nil {
			return r, err
		}
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			return r, fmt.Errorf("hdbench: ffmpeg: %w", err)
		}
		counted := make(chan int64, 1)
		go func() {
			n, _ := io.Copy(io.Discard, out)
			counted <- n
		}()
		// a pipe write is how long ffmpeg takes to accept the frame: it paces at the encoder's speed
		encodeFrame = func(bool) error {
			start := time.Now()
			_, err := in.Write(i420)
			encodeTimes = append(encodeTimes, time.Since(start))
			return err
		}
		finish = func() error {
			_ = in.Close()
			bytes = <-counted
			return cmd.Wait()
		}
		r.Note = "through ffmpeg: the time per frame is how long it takes to accept one"
	default:
		return r, fmt.Errorf("hdbench: unknown encoder %q", enc)
	}

	cpu0 := cpuTime()
	wall0 := time.Now()
	for t := 0; t < frames; t++ {
		d0 := time.Now()
		i420 = scene.Draw(t)
		drawTotal += time.Since(d0)
		if err := encodeFrame(t == 0); err != nil {
			return r, err
		}
	}
	if err := finish(); err != nil {
		return r, err
	}
	wall := time.Since(wall0)
	cpu := cpuTime() - cpu0

	ms := func(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
	sorted := slices.Clone(encodeTimes)
	slices.Sort(sorted)
	var sum time.Duration
	for _, d := range sorted {
		sum += d
	}
	// the first frame (a keyframe, the encoder settling) is left out of the average
	avg := (sum - sorted[len(sorted)-1]) / time.Duration(max(1, len(sorted)-1))
	r.EncodeAvgMs = ms(avg)
	r.EncodeP95Ms = ms(sorted[len(sorted)*95/100])
	r.EncodeMaxMs = ms(sorted[len(sorted)-1])
	r.DrawAvgMs = ms(drawTotal / time.Duration(frames))
	if avg > 0 {
		r.MaxFPS = float64(time.Second) / float64(avg)
	}
	r.Realtime = sorted[len(sorted)*95/100] <= frameDur*6/10
	r.Kbps = float64(bytes*8) / (float64(frames) / float64(fps)) / 1000
	r.CPU = 100 * float64(cpu) / float64(wall)
	return r, nil
}

// hdRaw writes the scene's frames as raw I420, for timing an encoder on its
// own (ffmpeg -benchmark), apart from drawing the scene.
func hdRaw(file string, size hdSize, far, play image.Image, frames int) error {
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()
	scene := hdscene.New(size.W, size.H, far, play)
	for t := 0; t < frames; t++ {
		if _, err := f.Write(scene.Draw(t)); err != nil {
			return err
		}
	}
	return f.Close()
}

// hdChoice is what go-link HD streams on this computer: a size and a codec.
type hdChoice struct {
	Size   string  `json:"size"`
	Codec  string  `json:"codec"`          // vp8 or h264
	H264   string  `json:"h264,omitempty"` // with h264: x264 or videotoolbox
	MaxFPS float64 `json:"max_fps"`        // what the check measured
	P95Ms  float64 `json:"p95_ms"`
}

// hdCheck is one candidate's try.
type hdCheck struct {
	hdChoice
	Fits bool   `json:"fits"`
	Why  string `json:"why,omitempty"`
}

// hdCandidates are the choices to try on this computer, best first: 4K
// with x264 on Apple Silicon (the experiment's 4K60), 1080p with the
// hardware encoder (half a core), 1080p with x264 or VP8 with enough cores,
// and 720p VP8, which every host can stream.
func hdCandidates(h264 encoder.H264Support, cores int, appleSilicon bool) []hdChoice {
	var c []hdChoice
	if h264.X264 && appleSilicon {
		c = append(c, hdChoice{Size: "2160p", Codec: "h264", H264: "x264"})
	}
	if h264.VideoToolbox {
		c = append(c, hdChoice{Size: "1080p", Codec: "h264", H264: "videotoolbox"})
	}
	if h264.X264 && cores >= 8 {
		c = append(c, hdChoice{Size: "1080p", Codec: "h264", H264: "x264"})
	}
	if cores >= 8 {
		c = append(c, hdChoice{Size: "1080p", Codec: "vp8"})
	}
	return append(c, hdChoice{Size: "720p", Codec: "vp8"})
}

// hdFits says whether a check's numbers stream at 60 fps: VP8 in the
// device's own process needs its 95th percentile within 60 % of a frame
// (the device's rule for a room's encoder); ffmpeg, in a process of its own,
// needs room for 66 frames a second.
func hdFits(r hdResult, codec string) (bool, string) {
	if codec == "vp8" {
		if r.Realtime {
			return true, ""
		}
		return false, fmt.Sprintf("p95 %.1f ms is over 10 ms", r.EncodeP95Ms)
	}
	if r.MaxFPS >= 66 {
		return true, ""
	}
	return false, fmt.Sprintf("%.0f fps is under 66", r.MaxFPS)
}

// hdAuto tries the candidates for two seconds each, in order, and returns
// the first that fits (720p VP8 when none does), with every try.
func hdAuto(far, play image.Image) (hdChoice, []hdCheck) {
	appleSilicon := runtime.GOOS == "darwin" && runtime.GOARCH == "arm64"
	var tries []hdCheck
	for _, c := range hdCandidates(encoder.ProbeH264(), runtime.NumCPU(), appleSilicon) {
		enc, threads := c.Codec, 8
		if c.Codec == "h264" {
			enc = c.H264
		}
		r, err := hdRun(hdSizes[c.Size], enc, far, play, 120, 60, threads, 0)
		check := hdCheck{hdChoice: c}
		if err != nil {
			check.Why = err.Error()
		} else {
			check.MaxFPS, check.P95Ms = r.MaxFPS, r.EncodeP95Ms
			check.Fits, check.Why = hdFits(r, c.Codec)
		}
		tries = append(tries, check)
		if check.Fits {
			return check.hdChoice, tries
		}
	}
	return hdChoice{Size: "720p", Codec: "vp8"}, tries
}

// cmdHDProbe prints what this computer can stream for go-link HD and what
// `--test-room-hd auto` would choose.
func cmdHDProbe(args []string) error {
	fs := flag.NewFlagSet("hdprobe", flag.ContinueOnError)
	far := fs.String("far", "", "the HD scene's far picture (PNG or JPEG)")
	play := fs.String("play", "", "the HD scene's play picture (#FF00FF is transparent)")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *far == "" {
		return errors.New("usage: device hdprobe --far PICTURE [--play PICTURE] [--json]")
	}
	farImg, err := loadPicture(*far)
	if err != nil {
		return err
	}
	var playImg image.Image
	if *play != "" {
		if playImg, err = loadPicture(*play); err != nil {
			return err
		}
	}
	h264 := encoder.ProbeH264()
	choice, tries := hdAuto(farImg, playImg)
	if *asJSON {
		e := json.NewEncoder(os.Stdout)
		e.SetIndent("", "  ")
		return e.Encode(map[string]any{"type": "hd_probe", "cpus": runtime.NumCPU(), "goos": runtime.GOOS, "goarch": runtime.GOARCH, "h264": h264, "tries": tries, "choice": choice})
	}
	fmt.Printf("computer: %s/%s, %d CPUs; ffmpeg: %s (x264 %v, VideoToolbox %v)\n", runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), orNone(h264.FFmpeg), h264.X264, h264.VideoToolbox)
	for _, t := range tries {
		fmt.Printf("  %-5s %-4s %-12s max %6.1f fps, p95 %5.1f ms  %s\n", t.Size, t.Codec, t.H264, t.MaxFPS, t.P95Ms, map[bool]string{true: "fits", false: "no: " + t.Why}[t.Fits])
	}
	fmt.Printf("choice: %s %s %s\n", choice.Size, choice.Codec, choice.H264)
	return nil
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
