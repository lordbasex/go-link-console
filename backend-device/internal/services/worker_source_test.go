// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/pkg/emuproc"
	"github.com/lordbasex/go-link-console/backend-device/pkg/input"
	"github.com/lordbasex/go-link-console/backend-device/pkg/libretro"
	"github.com/lordbasex/go-link-console/backend-device/pkg/testpattern"
)

// fakeWorkerSink records what a source sends.
type fakeWorkerSink struct {
	mu      sync.Mutex
	frames  [][2]byte // first two bytes of every frame
	samples int
	aspect  float64
	buttons atomic.Uint32 // pad of port 1
	scale   int           // told by SetVideoScale
	scales  []int         // every change
	sizes   [][2]int      // of the frames, when it changes
}

func (f *fakeWorkerSink) VideoFrame(i420 []byte, w, h int, _ time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	scale := max(f.scale, 1)
	if w != 4*scale || h != 2*scale || len(i420) != emuproc.FrameSizeI420(w, h) {
		panic(fmt.Sprintf("bad frame %dx%d with %d bytes at scale %d", w, h, len(i420), scale))
	}
	if n := len(f.sizes); n == 0 || f.sizes[n-1] != [2]int{w, h} {
		f.sizes = append(f.sizes, [2]int{w, h})
	}
	f.frames = append(f.frames, [2]byte{i420[0], i420[1]})
}

func (f *fakeWorkerSink) SetVideoScale(scale int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scale = scale
	f.scales = append(f.scales, scale)
}

func (f *fakeWorkerSink) AudioSamples(pcm []int16) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.samples += len(pcm)
}

func (f *fakeWorkerSink) SetAspect(aspect float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.aspect = aspect
}

func (f *fakeWorkerSink) Controls() testpattern.Controls { return testpattern.Controls{} }

func (f *fakeWorkerSink) PortPad(port int) input.Pad {
	if port != 1 {
		return input.Pad{}
	}
	return input.Pad{Buttons: input.State(f.buttons.Load())}
}

func (f *fakeWorkerSink) lastFrames(n int) [][2]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.frames) < n {
		return nil
	}
	return append([][2]byte(nil), f.frames[len(f.frames)-n:]...)
}

// fakeWorker runs this test binary as a worker (see TestHelperProcess).
func fakeWorker(mode string, gotArgs *[]string) func(context.Context, []string) *exec.Cmd {
	return func(ctx context.Context, args []string) *exec.Cmd {
		if gotArgs != nil {
			*gotArgs = args
		}
		cmd := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestHelperProcess$", "--"}, args...)...)
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "FAKE_WORKER_MODE="+mode)
		return cmd
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestWorkerSourceStreamsAndSavesState(t *testing.T) {
	dir := t.TempDir()
	var args []string
	var readyAV atomic.Pointer[libretro.AVInfo]
	src := NewWorkerSource(WorkerConfig{
		CorePath: "/cores/mame.dylib", RomPath: "/roms/robby.zip", SystemDir: "/system",
		StatePath: filepath.Join(dir, "missing-is-fine-for-the-fake"),
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		OnReady:   func(av libretro.AVInfo) { readyAV.Store(&av) },
		Command:   fakeWorker("ok", &args),
	})
	sink := &fakeWorkerSink{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- src.Run(ctx, sink) }()

	waitFor(t, "ready", func() bool { return readyAV.Load() != nil })
	if av := readyAV.Load(); av.FPS != 100 || av.BaseWidth != 4 {
		t.Fatalf("ready %+v", av)
	}
	want := []string{"emulate", "--core", "/cores/mame.dylib", "--rom", "/roms/robby.zip", "--system", "/system", "--state", filepath.Join(dir, "missing-is-fine-for-the-fake")}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Fatalf("args %q", args)
	}
	if !src.Running() {
		t.Fatal("not running")
	}

	// The fake writes the buttons of port 1 into the first byte.
	sink.buttons.Store(uint32(input.Button1 | input.Button3))
	waitFor(t, "pads to reach the worker", func() bool {
		f := sink.lastFrames(1)
		return f != nil && f[0][0] == byte(input.Button1|input.Button3)
	})
	sink.mu.Lock()
	if sink.aspect != 4.0/3 || sink.samples == 0 {
		t.Fatalf("aspect %v samples %d", sink.aspect, sink.samples)
	}
	sink.mu.Unlock()

	// Paused: frames keep coming, but always the same picture (the fake
	// counts frames in the second byte).
	src.SetPaused(true)
	time.Sleep(50 * time.Millisecond)
	sink.mu.Lock()
	before := len(sink.frames)
	sink.mu.Unlock()
	time.Sleep(100 * time.Millisecond)
	held := sink.lastFrames(5)
	sink.mu.Lock()
	after := len(sink.frames)
	sink.mu.Unlock()
	if after-before < 5 {
		t.Fatalf("only %d frames while paused", after-before)
	}
	for _, f := range held {
		if f != held[0] {
			t.Fatalf("the picture changed while paused: %v", held)
		}
	}
	src.SetPaused(false)
	waitFor(t, "the game to resume", func() bool {
		f := sink.lastFrames(1)
		return f != nil && f[0][1] != held[0][1]
	})

	// Save and load states.
	state := filepath.Join(dir, "robby.state")
	sctx, scancel := context.WithTimeout(ctx, 2*time.Second)
	defer scancel()
	if err := src.SaveState(sctx, state); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(state); err != nil || string(b) != "fake state" {
		t.Fatalf("state file %q %v", b, err)
	}
	if err := src.LoadState(sctx, state); err != nil {
		t.Fatal(err)
	}
	if err := src.LoadState(sctx, filepath.Join(dir, "nope.state")); err == nil || !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("missing state: %v", err)
	}

	// Stopping asks the worker to quit, well before the kill delay.
	start := time.Now()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return")
	}
	if d := time.Since(start); d >= workerQuitDelay {
		t.Fatalf("the worker was killed instead of quitting (%v)", d)
	}
	if src.Running() {
		t.Fatal("still running")
	}
	if err := src.SaveState(context.Background(), state); !errors.Is(err, errWorkerStopped) {
		t.Fatalf("save after stop: %v", err)
	}
}

func TestWorkerSourceReportsCrash(t *testing.T) {
	var ready atomic.Bool
	src := NewWorkerSource(WorkerConfig{
		CorePath: "c", RomPath: "looping.zip", SystemDir: "s",
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		OnReady: func(libretro.AVInfo) { ready.Store(true) },
		Command: fakeWorker("crash", nil),
	})
	err := src.Run(context.Background(), &fakeWorkerSink{})
	if err == nil || !strings.Contains(err.Error(), "the core could not load looping.zip") || !strings.Contains(err.Error(), "exit status 3") || !strings.Contains(err.Error(), "loading core c") {
		t.Fatalf("crash: %v", err)
	}
	if ready.Load() || src.Running() {
		t.Fatal("ready or running after a crash")
	}
}

func TestLineTail(t *testing.T) {
	tail := &lineTail{max: 2, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	fmt.Fprint(tail, "one\ntw")
	fmt.Fprint(tail, "o\r\n\nthree\nfour")
	if got := tail.String(); got != "two\nthree\nfour" {
		t.Fatalf("tail %q", got)
	}
}

// TestHelperProcess is the fake emulator worker; it only runs as a child
// of the tests above.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		t.Skip("helper process")
	}
	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+1:]
			break
		}
	}
	os.Exit(fakeWorkerMain(os.Getenv("FAKE_WORKER_MODE"), args))
}

func fakeWorkerMain(mode string, args []string) int {
	w := emuproc.NewWriter(os.Stdout)
	if mode == "crash" {
		fmt.Fprintf(os.Stderr, "loading core %s\n", args[2])
		_ = w.Write(emuproc.TypeError, []byte("libretro: the core could not load "+args[4]))
		_ = w.Flush()
		return 3
	}
	_ = w.WriteJSON(emuproc.TypeReady, emuproc.Ready{BaseWidth: 4, BaseHeight: 2, AspectRatio: 4.0 / 3, FPS: 100, SampleRate: 48000})
	_ = w.Flush()

	var buttons atomic.Uint32
	var paused atomic.Bool
	var video atomic.Uint32
	for i, a := range args {
		if a == "--video" && i+1 < len(args) {
			m, _ := emuproc.ParseVideoMode(args[i+1])
			video.Store(uint32(m))
		}
	}
	replies := make(chan func(), 4)
	quit := make(chan struct{})
	go func() {
		defer close(quit)
		r := emuproc.NewReader(os.Stdin)
		for {
			t, p, err := r.Next()
			if err != nil {
				return
			}
			switch t {
			case emuproc.TypePads:
				pads, _ := emuproc.DecodePads(p)
				buttons.Store(uint32(pads[0].Buttons))
			case emuproc.TypePause:
				v, _ := emuproc.DecodePause(p)
				paused.Store(v)
			case emuproc.TypeVideoMode:
				m, _ := emuproc.DecodeVideoMode(p)
				video.Store(uint32(m))
			case emuproc.TypeSaveState:
				path := string(p)
				err := os.WriteFile(path, []byte("fake state"), 0o644)
				replies <- func() { _ = w.WriteJSON(emuproc.TypeStateSaved, stateResult(path, err)) }
			case emuproc.TypeLoadState:
				path := string(p)
				_, err := os.ReadFile(path)
				replies <- func() { _ = w.WriteJSON(emuproc.TypeStateLoaded, stateResult(path, err)) }
			case emuproc.TypeQuit:
				return
			}
		}
	}()
	var count byte
	ticker := time.NewTicker(10 * time.Millisecond)
	for {
		select {
		case <-quit:
			return 0
		case reply := <-replies:
			reply()
		case <-ticker.C:
			if paused.Load() {
				continue
			}
			count++
			scale := emuproc.VideoMode(video.Load()).Scale()
			frame := make([]byte, emuproc.FrameSizeI420(4*scale, 2*scale))
			frame[0], frame[1] = byte(buttons.Load()), count
			_ = w.WriteVideo(4*scale, 2*scale, scale, 10*time.Millisecond, frame)
			_ = w.WriteAudio(make([]int16, 960))
		}
		if err := w.Flush(); err != nil {
			return 1
		}
	}
}

func stateResult(path string, err error) emuproc.StateResult {
	if err != nil {
		return emuproc.StateResult{Path: path, Error: err.Error()}
	}
	return emuproc.StateResult{OK: true, Path: path}
}

func TestWorkerSourceVideoModes(t *testing.T) {
	var args []string
	src := NewWorkerSource(WorkerConfig{
		CorePath: "/c", RomPath: "/r.zip", SystemDir: "/s",
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Command: fakeWorker("ok", &args),
	})
	src.SetVideoMode(emuproc.VideoDouble) // before Run: the worker's flag
	sink := &fakeWorkerSink{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- src.Run(ctx, sink) }()
	scaleIs := func(want int) func() bool {
		return func() bool {
			sink.mu.Lock()
			defer sink.mu.Unlock()
			return sink.scale == want && len(sink.sizes) > 0 && sink.sizes[len(sink.sizes)-1] == [2]int{4 * want, 2 * want}
		}
	}
	waitFor(t, "2x frames", scaleIs(2))
	if !strings.Contains(strings.Join(args, " "), "--video double") {
		t.Fatalf("args %q", args)
	}
	// While running: a message, and the frames that follow change size.
	src.SetVideoMode(emuproc.VideoBox)
	waitFor(t, "frames at the game's size", scaleIs(1))
	src.SetVideoMode(emuproc.VideoDouble)
	waitFor(t, "2x frames again", scaleIs(2))
	sink.mu.Lock()
	scales := slices.Clone(sink.scales)
	sink.mu.Unlock()
	if !slices.Equal(scales, []int{2, 1, 2}) {
		t.Fatalf("scales told %v", scales)
	}
	cancel()
	<-done
}
