// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/pkg/encoder"
	"github.com/lordbasex/go-link-console/backend-device/pkg/framelab"
)

// FrameResult is the quality of one frame as players see it.
type FrameResult struct {
	Frame int `json:"frame"`
	// Native compares at the game's resolution (a 2x picture is averaged
	// back down 2x2), with chroma spread bilinearly like a GPU does.
	Native framelab.Metrics `json:"native"`
	// Replicate is the same with chroma repeated over 2x2 blocks (libyuv).
	Replicate framelab.Metrics `json:"replicate"`
	// Scaled, for upscaled variants, compares the decoded picture with the
	// reference scaled up the same way (nearest neighbour).
	Scaled *framelab.Metrics `json:"scaled,omitempty"`
}

// Result is what encode writes to result.json.
type Result struct {
	Variant       framelab.Variant `json:"variant"`
	Width         int              `json:"width"`
	Height        int              `json:"height"`
	FPS           int              `json:"fps"`
	Frames        int              `json:"frames"`
	Packets       int              `json:"packets"`
	Keyframes     int              `json:"keyframes"`
	Dropped       int              `json:"dropped"` // frames the rate control skipped
	KbpsAvg       float64          `json:"kbps_avg"`
	KbpsPeak      float64          `json:"kbps_peak"` // highest one second window
	PrepMs        float64          `json:"prep_ms"`   // scaling + I420 per frame
	EncodeMs      float64          `json:"encode_ms"` // wall time per frame
	EncodeP50Ms   float64          `json:"encode_p50_ms"`
	EncodeP95Ms   float64          `json:"encode_p95_ms"`
	EncodeCPUMs   float64          `json:"encode_cpu_ms"` // CPU time per frame (all encoder threads)
	RunPSNR       float64          `json:"run_psnr"`      // native PSNR over every frame of the run
	Stills        []FrameResult    `json:"stills"`
	Clip          []FrameResult    `json:"clip"`
	StillsMean    FrameResult      `json:"stills_mean"`
	ClipMean      FrameResult      `json:"clip_mean"`
	VMAF          *VMAFResult      `json:"vmaf,omitempty"`
	DecodedFrames int              `json:"decoded_frames"`
}

func runEncode(args []string) error {
	fs := flag.NewFlagSet("encode", flag.ContinueOnError)
	capDir := fs.String("capture", "", "a folder written by capture")
	spec := fs.String("variant", "", "variant, e.g. name=B,scale=2,kbps=5000 (empty = the device's settings)")
	out := fs.String("out", "", "output folder")
	vmaf := fs.Bool("vmaf", false, "also run VMAF and MS-SSIM (ffmpeg libvmaf) on the clip")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *capDir == "" || *out == "" {
		return errors.New("encode: -capture and -out are required")
	}
	v, err := framelab.ParseVariant(*spec)
	if err != nil {
		return err
	}
	var meta Meta
	if err := readJSON(filepath.Join(*capDir, "meta.json"), &meta); err != nil {
		return err
	}
	for _, d := range []string{"stills", "clip"} {
		if err := os.MkdirAll(filepath.Join(*out, d), 0o755); err != nil {
			return err
		}
	}
	res := &Result{Variant: v, Width: meta.Width * v.Scale, Height: meta.Height * v.Scale, FPS: meta.StreamFPS}
	ivf := filepath.Join(*out, "stream.ivf")
	start := time.Now()
	if v.Codec == "vp9" {
		err = encodeFFmpegVP9(*capDir, ivf, v, res)
	} else {
		err = encodeVP8(*capDir, ivf, v, res)
	}
	if err != nil {
		return err
	}
	fmt.Printf("%s: encoded %d frames in %s, %.0f kbps, %.2f ms/frame\n", v.Name, res.Frames, time.Since(start).Round(time.Millisecond), res.KbpsAvg, res.EncodeMs)
	if err := measure(*capDir, *out, ivf, meta, v, res); err != nil {
		return err
	}
	if *vmaf && meta.ClipCount > 0 {
		vm, err := runVMAF(filepath.Join(*capDir, "clip"), filepath.Join(*out, "clip"), filepath.Join(*out, "vmaf.json"))
		if err != nil {
			fmt.Fprintln(os.Stderr, "vmaf:", err)
		}
		res.VMAF = vm
	}
	fmt.Printf("%s: stills PSNR %.2f dB SSIM-Y %.4f, clip PSNR %.2f dB SSIM-Y %.4f, run PSNR %.2f dB\n", v.Name,
		res.StillsMean.Native.PSNR, res.StillsMean.Native.SSIMY, res.ClipMean.Native.PSNR, res.ClipMean.Native.SSIMY, res.RunPSNR)
	return writeJSON(filepath.Join(*out, "result.json"), res)
}

// encodeVP8 is the device's path: the same conversion (libretro.ToI420)
// and pkg/encoder, a keyframe first, at the device's stream frame rate.
func encodeVP8(capDir, ivfPath string, v framelab.Variant, res *Result) error {
	r, err := framelab.OpenRun(filepath.Join(capDir, "run.glrun"))
	if err != nil {
		return err
	}
	defer r.Close()
	enc, err := encoder.NewVP8(encoder.Config{Width: res.Width, Height: res.Height, FPS: res.FPS, BitrateKbps: v.BitrateKbps,
		MinQuantizer: v.MinQ, MaxQuantizer: v.MaxQ, CPUUsed: v.CPUUsed})
	if err != nil {
		return err
	}
	defer enc.Close()
	w, err := framelab.CreateIVF(ivfPath, "VP80", res.Width, res.Height, res.FPS)
	if err != nil {
		return err
	}
	var (
		prep, wall time.Duration
		cpu        time.Duration
		times      []float64
		sizes      []int
		buf        []byte
	)
	for i := 0; ; i++ {
		img, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			w.Close()
			return err
		}
		// The device converts the core's frame in one pass (the worker);
		// variants it has no conversion for go through the lab's own.
		frame := framelab.Frame(img)
		t0 := time.Now()
		i420, _, _, ok := v.PrepareFrame(buf, frame)
		if ok {
			buf = i420
		} else {
			i420, _, _ = v.Prepare(img)
		}
		t1 := time.Now()
		c0 := cpuTime()
		data, key, err := enc.Encode(i420, i == 0)
		cpu += cpuTime() - c0
		t2 := time.Now()
		if err != nil {
			w.Close()
			return err
		}
		prep += t1.Sub(t0)
		wall += t2.Sub(t1)
		times = append(times, float64(t2.Sub(t1).Microseconds())/1000)
		res.Frames++
		sizes = append(sizes, len(data))
		if len(data) == 0 {
			res.Dropped++
			continue
		}
		if key {
			res.Keyframes++
		}
		res.Packets++
		if err := w.Write(data, int64(i)); err != nil {
			w.Close()
			return err
		}
	}
	if err := w.Close(); err != nil {
		return err
	}
	n := float64(res.Frames)
	res.PrepMs = float64(prep.Microseconds()) / 1000 / n
	res.EncodeMs = float64(wall.Microseconds()) / 1000 / n
	res.EncodeCPUMs = float64(cpu.Microseconds()) / 1000 / n
	sort.Float64s(times)
	res.EncodeP50Ms = times[int(0.50*float64(len(times)-1))]
	res.EncodeP95Ms = times[int(0.95*float64(len(times)-1))]
	res.KbpsAvg, res.KbpsPeak = bitrates(sizes, res.FPS)
	return nil
}

// encodeFFmpegVP9 encodes with ffmpeg's libvpx-vp9 in real-time mode with
// the variant's rate settings, for reference only: the device has no VP9
// encoder.
func encodeFFmpegVP9(capDir, ivfPath string, v framelab.Variant, res *Result) error {
	r, err := framelab.OpenRun(filepath.Join(capDir, "run.glrun"))
	if err != nil {
		return err
	}
	defer r.Close()
	kbps := fmt.Sprintf("%dk", v.BitrateKbps)
	cmd := exec.Command("ffmpeg", "-v", "error", "-y", "-f", "rawvideo", "-pix_fmt", "yuv420p",
		"-s", fmt.Sprintf("%dx%d", res.Width, res.Height), "-r", fmt.Sprint(res.FPS), "-i", "pipe:0",
		"-c:v", "libvpx-vp9", "-deadline", "realtime", "-cpu-used", fmt.Sprint(v.CPUUsed), "-row-mt", "1", "-threads", "2",
		"-lag-in-frames", "0", "-error-resilient", "1", "-b:v", kbps, "-minrate", kbps, "-maxrate", kbps,
		"-qmin", fmt.Sprint(v.MinQ), "-qmax", fmt.Sprint(v.MaxQ), "-g", fmt.Sprint(res.FPS*3), "-f", "ivf", ivfPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	bw := bufio.NewWriterSize(stdin, 1<<20)
	var prep time.Duration
	start := time.Now()
	for {
		img, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			stdin.Close()
			cmd.Wait()
			return err
		}
		t0 := time.Now()
		i420, _, _ := v.Prepare(img)
		prep += time.Since(t0)
		if _, err := bw.Write(i420); err != nil {
			stdin.Close()
			cmd.Wait()
			return err
		}
		res.Frames++
	}
	bw.Flush()
	stdin.Close()
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg vp9: %w", err)
	}
	n := float64(res.Frames)
	res.PrepMs = float64(prep.Microseconds()) / 1000 / n
	res.EncodeMs = float64(time.Since(start).Microseconds())/1000/n - res.PrepMs
	st := cmd.ProcessState
	res.EncodeCPUMs = float64((st.UserTime() + st.SystemTime()).Microseconds()) / 1000 / n
	res.EncodeP95Ms = res.EncodeMs
	pts, sizes, keys, err := readIVF(ivfPath)
	if err != nil {
		return err
	}
	res.Packets, res.Keyframes = len(pts), keys
	res.Dropped = res.Frames - len(pts)
	perFrame := make([]int, res.Frames)
	for i, p := range pts {
		if int(p) < len(perFrame) {
			perFrame[p] = sizes[i]
		}
	}
	res.KbpsAvg, res.KbpsPeak = bitrates(perFrame, res.FPS)
	return nil
}

// bitrates returns the average and the highest one second bitrate.
func bitrates(sizes []int, fps int) (avg, peak float64) {
	total := 0
	for _, s := range sizes {
		total += s
	}
	if len(sizes) == 0 {
		return 0, 0
	}
	avg = float64(total) * 8 / 1000 / (float64(len(sizes)) / float64(fps))
	win := 0
	for i, s := range sizes {
		win += s
		if i >= fps {
			win -= sizes[i-fps]
		}
		if i >= fps-1 {
			peak = max(peak, float64(win)*8/1000)
		}
	}
	return avg, peak
}

// readIVF lists the pts and size of each frame of an IVF file and counts
// the VP8/VP9 keyframes.
func readIVF(path string) (pts []int64, sizes []int, keys int, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, 0, err
	}
	if len(b) < 32 || string(b[:4]) != "DKIF" {
		return nil, nil, 0, fmt.Errorf("%s: not IVF", path)
	}
	vp9 := string(b[8:12]) == "VP90"
	for o := int(binary.LittleEndian.Uint16(b[6:])); o+12 <= len(b); {
		n := int(binary.LittleEndian.Uint32(b[o:]))
		p := int64(binary.LittleEndian.Uint64(b[o+4:]))
		o += 12
		if o+n > len(b) {
			return nil, nil, 0, fmt.Errorf("%s: truncated", path)
		}
		frame := b[o : o+n]
		if len(frame) > 0 {
			if vp9 {
				// uncompressed header: frame marker (2), profile (2 bits
				// for profile 0), show_existing (1), frame_type (1, 0 = key).
				if frame[0]&0x04 == 0 && frame[0]&0x08 == 0 {
					keys++
				}
			} else if frame[0]&1 == 0 {
				keys++
			}
		}
		pts = append(pts, p)
		sizes = append(sizes, n)
		o += n
	}
	return pts, sizes, keys, nil
}

// job is one frame to measure.
type job struct {
	frame int
	ref   framelab.Image
	i420  []byte // decoded frame (never reused)
	w, h  int    // encoded size
	full  bool   // stills and clip: every metric; else PSNR only
	still bool
	clip  bool
}

// measure decodes the stream with libvpx (through ffmpeg), as a browser
// would, rebuilds what each frame shows (a skipped frame keeps showing the
// last decoded picture) and compares it with the reference.
func measure(capDir, outDir, ivfPath string, meta Meta, v framelab.Variant, res *Result) error {
	pts, _, _, err := readIVF(ivfPath)
	if err != nil {
		return err
	}
	decoderName := map[string]string{"vp8": "libvpx", "vp9": "libvpx-vp9"}[v.Codec]
	cmd := exec.Command("ffmpeg", "-v", "error", "-c:v", decoderName, "-i", ivfPath,
		"-f", "rawvideo", "-pix_fmt", "yuv420p", "-fps_mode", "passthrough", "pipe:1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() {
		stdout.Close()
		_ = cmd.Wait()
	}()
	dec := bufio.NewReaderSize(stdout, 1<<22)
	frameBytes := encoder.FrameSize(res.Width, res.Height)
	r, err := framelab.OpenRun(filepath.Join(capDir, "run.glrun"))
	if err != nil {
		return err
	}
	defer r.Close()

	stills := map[int]bool{}
	for _, s := range meta.Stills {
		stills[s] = true
	}
	jobs := make(chan job, runtime.NumCPU())
	var (
		mu      sync.Mutex
		results = map[int]FrameResult{}
		runSum  float64
		runN    int
		saveErr error
		wg      sync.WaitGroup
	)
	for range runtime.NumCPU() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				fr, psnr, err := measureFrame(j, v.Scale, outDir)
				mu.Lock()
				runSum += psnr
				runN++
				if j.full {
					results[j.frame] = fr
				}
				if err != nil && saveErr == nil {
					saveErr = err
				}
				mu.Unlock()
			}
		}()
	}

	var (
		cur     []byte // latest decoded frame
		decoded int    // frames read from the decoder
		pi      int    // next packet
	)
	for i := 0; ; i++ {
		ref, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			close(jobs)
			wg.Wait()
			return err
		}
		for pi < len(pts) && pts[pi] <= int64(i) {
			buf := make([]byte, frameBytes)
			if _, err := io.ReadFull(dec, buf); err != nil {
				close(jobs)
				wg.Wait()
				return fmt.Errorf("decoder gave %d frames for %d packets: %w", decoded, len(pts), err)
			}
			cur = buf
			decoded++
			pi++
		}
		if cur == nil {
			continue // nothing decoded yet
		}
		inClip := meta.ClipCount > 0 && i >= meta.ClipFrom && i < meta.ClipFrom+meta.ClipCount
		full := stills[i] || inClip
		jobs <- job{frame: i, ref: ref, full: full, still: stills[i], clip: inClip, i420: cur, w: res.Width, h: res.Height}
	}
	close(jobs)
	wg.Wait()
	if saveErr != nil {
		return saveErr
	}
	res.DecodedFrames = decoded
	if runN > 0 {
		res.RunPSNR = runSum / float64(runN)
	}
	keys := make([]int, 0, len(results))
	for k := range results {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		fr := results[k]
		if stills[k] {
			res.Stills = append(res.Stills, fr)
		}
		if meta.ClipCount > 0 && k >= meta.ClipFrom && k < meta.ClipFrom+meta.ClipCount {
			res.Clip = append(res.Clip, fr)
		}
	}
	res.StillsMean = meanFrames(res.Stills)
	res.ClipMean = meanFrames(res.Clip)
	return nil
}

// measureFrame computes the metrics of one frame and saves the decoded
// picture of stills and clip frames.
func measureFrame(j job, scale int, outDir string) (FrameResult, float64, error) {
	decoded := framelab.FromI420(j.i420, j.w, j.h, framelab.Bilinear)
	native := decoded
	if scale > 1 {
		native = framelab.BoxDown(decoded, scale)
	}
	if !j.full {
		return FrameResult{}, framelab.PSNR(j.ref, native), nil
	}
	fr := FrameResult{Frame: j.frame}
	fr.Native, _ = framelab.Compare(j.ref, native)
	rep := framelab.FromI420(j.i420, j.w, j.h, framelab.Replicate)
	if scale > 1 {
		rep = framelab.BoxDown(rep, scale)
		m, _ := framelab.Compare(framelab.Nearest(j.ref, scale), decoded)
		fr.Scaled = &m
	}
	fr.Replicate, _ = framelab.Compare(j.ref, rep)
	var err error
	if j.still {
		err = framelab.WritePNG(filepath.Join(outDir, "stills", frameName(j.frame)), decoded)
	}
	if j.clip && err == nil {
		err = framelab.WritePNG(filepath.Join(outDir, "clip", frameName(j.frame)), decoded)
	}
	return fr, fr.Native.PSNR, err
}

func meanFrames(frs []FrameResult) FrameResult {
	var nat, rep, sc []framelab.Metrics
	for _, f := range frs {
		nat = append(nat, f.Native)
		rep = append(rep, f.Replicate)
		if f.Scaled != nil {
			sc = append(sc, *f.Scaled)
		}
	}
	out := FrameResult{Frame: -1, Native: framelab.Mean(nat), Replicate: framelab.Mean(rep)}
	if len(sc) > 0 {
		m := framelab.Mean(sc)
		out.Scaled = &m
	}
	return out
}
