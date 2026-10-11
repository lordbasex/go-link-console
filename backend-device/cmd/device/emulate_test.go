// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/internal/services"
	"github.com/lordbasex/go-link-console/backend-device/pkg/cores"
	"github.com/lordbasex/go-link-console/backend-device/pkg/input"
	"github.com/lordbasex/go-link-console/backend-device/pkg/libretro"
	"github.com/lordbasex/go-link-console/backend-device/pkg/testpattern"
)

// countingSink counts frames and audio samples.
type countingSink struct {
	mu      sync.Mutex
	frames  int
	samples int
	w, h    int
}

func (c *countingSink) VideoFrame(i420 []byte, w, h int, _ time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(i420) == libretro.FrameSizeI420(w, h) {
		c.frames++
		c.w, c.h = w, h
	}
}

func (c *countingSink) AudioSamples(pcm []int16) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.samples += len(pcm)
}

func (c *countingSink) SetAspect(float64)              {}
func (c *countingSink) Controls() testpattern.Controls { return testpattern.Controls{} }
func (c *countingSink) PortPad(int) input.Pad          { return input.Pad{} }

func (c *countingSink) counts() (frames, samples int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.frames, c.samples
}

// emulateWorker runs this test binary as the "emulate" worker.
func emulateWorker(ctx context.Context, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestEmulateHelper$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "GO_WANT_EMULATE_WORKER=1")
	return cmd
}

// TestEmulateHelper is the worker process of TestEmulateWorkerWithRealCore.
func TestEmulateHelper(t *testing.T) {
	if os.Getenv("GO_WANT_EMULATE_WORKER") != "1" {
		t.Skip("helper process")
	}
	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+2:] // drop "emulate"
			break
		}
	}
	if err := runEmulate(args); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

// TestEmulateWorkerWithRealCore runs robby in a worker process and saves
// and restores its state. It needs the mame2003-plus core and robby.zip in
// ~/go-link and is skipped otherwise.
func TestEmulateWorkerWithRealCore(t *testing.T) {
	home, _ := os.UserHomeDir()
	corePath := filepath.Join(home, "go-link", "cores", cores.FileName(cores.DefaultCore, runtime.GOOS))
	romPath := filepath.Join(home, "go-link", "roms", "robby.zip")
	if _, err := os.Stat(corePath); err != nil {
		t.Skip("mame2003-plus core not installed")
	}
	if _, err := os.Stat(romPath); err != nil {
		t.Skip("robby.zip not in the ROM folder")
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	dir := t.TempDir()
	system := filepath.Join(dir, "system")
	state := filepath.Join(dir, "states", "robby.state")

	run := func(statePath string, body func(*services.WorkerSource, *countingSink)) {
		t.Helper()
		ready := make(chan libretro.AVInfo, 1)
		src := services.NewWorkerSource(services.WorkerConfig{
			CorePath: corePath, RomPath: romPath, SystemDir: system, StatePath: statePath,
			Logger:  logger,
			OnReady: func(av libretro.AVInfo) { ready <- av },
			Command: emulateWorker,
		})
		sink := &countingSink{}
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() { result <- src.Run(ctx, sink) }()
		select {
		case av := <-ready:
			if av.FPS <= 0 || av.BaseWidth == 0 {
				t.Fatalf("ready %+v", av)
			}
		case err := <-result:
			t.Fatalf("worker stopped: %v", err)
		case <-time.After(20 * time.Second):
			t.Fatal("the game never started")
		}
		body(src, sink)
		cancel()
		if err := <-result; err != context.Canceled {
			t.Fatalf("run: %v", err)
		}
	}

	run("", func(src *services.WorkerSource, sink *countingSink) {
		deadline := time.Now().Add(10 * time.Second)
		for {
			if frames, samples := sink.counts(); frames >= 30 && samples > 0 {
				break
			}
			if time.Now().After(deadline) {
				frames, samples := sink.counts()
				t.Fatalf("%d frames and %d samples", frames, samples)
			}
			time.Sleep(20 * time.Millisecond)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := src.SaveState(ctx, state); err != nil {
			t.Fatal(err)
		}
		if fi, err := os.Stat(state); err != nil || fi.Size() == 0 || fi.Mode().Perm() != 0o600 { // saved games are private
			t.Fatalf("state file %v %v", fi, err)
		}
		if err := src.LoadState(ctx, state); err != nil {
			t.Fatal(err)
		}
	})
	// A new worker starts from the saved state.
	run(state, func(*services.WorkerSource, *countingSink) {})

	// A room saved paused comes back paused: it is paused as soon as the
	// game is ready, before its first frame, and still shows the game
	// where it stopped (one frame, which the device repeats).
	var paused *services.WorkerSource
	paused = services.NewWorkerSource(services.WorkerConfig{
		CorePath: corePath, RomPath: romPath, SystemDir: system, StatePath: state, Logger: logger,
		OnReady: func(libretro.AVInfo) { paused.SetPaused(true) },
		Command: emulateWorker,
	})
	sink := &countingSink{}
	pctx, pcancel := context.WithCancel(context.Background())
	presult := make(chan error, 1)
	go func() { presult <- paused.Run(pctx, sink) }()
	deadline := time.Now().Add(15 * time.Second)
	for frames, _ := sink.counts(); frames < 10; frames, _ = sink.counts() {
		if time.Now().After(deadline) {
			t.Fatalf("a game started paused shows no picture: %d frames", frames)
		}
		time.Sleep(20 * time.Millisecond)
	}
	pcancel()
	if err := <-presult; err != context.Canceled {
		t.Fatalf("paused run: %v", err)
	}

	// A ROM the core cannot load is reported with the worker's reason.
	bad := filepath.Join(dir, "bogus.zip")
	if err := os.WriteFile(bad, []byte("not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := services.NewWorkerSource(services.WorkerConfig{CorePath: corePath, RomPath: bad, SystemDir: system, Logger: logger, Command: emulateWorker})
	err := src.Run(context.Background(), &countingSink{})
	if err == nil || !strings.Contains(err.Error(), "could not load") {
		t.Fatalf("bogus ROM: %v", err)
	}
}
