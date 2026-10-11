// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/pkg/input"
	"github.com/lordbasex/go-link-console/backend-device/pkg/libretro"
)

// EmulatorConfig describes the game to run.
type EmulatorConfig struct {
	CorePath string // e.g. ~/go-link-console/cores/mame2003_plus_libretro.dylib
	RomPath  string // e.g. ~/go-link-console/roms/robby.zip; empty starts the core with no content
	// Native runs go-link HD's engine instead of a libretro core (GameCoreConfig.Native).
	Native bool
	// Upscale enlarges the picture that many times (GameCoreConfig.Upscale).
	Upscale   int
	SystemDir string // BIOS, samples, hiscores, NVRAM
	Logger    *slog.Logger
	// OnReady runs once the game is loaded, before the first frame.
	OnReady func(libretro.AVInfo)
	// Paused, when it returns true, holds the game: the core does not run,
	// the last picture is repeated and no sound is sent.
	Paused func() bool
}

// EmulatorSource runs a libretro core in the device process and streams
// the game. Only one can run per process; WorkerSource runs the game in a
// child process instead.
type EmulatorSource struct {
	cfg EmulatorConfig
	log *slog.Logger
}

// NewEmulatorSource prepares the source; the core is loaded by Run.
func NewEmulatorSource(cfg EmulatorConfig) *EmulatorSource {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &EmulatorSource{cfg: cfg, log: cfg.Logger}
}

// Run loads the core and the game and emulates at the game's own frame
// rate until ctx ends. All core calls happen on this goroutine, pinned to
// one OS thread.
func (e *EmulatorSource) Run(ctx context.Context, sink MediaSink) error {
	game, err := OpenGameCore(GameCoreConfig{
		CorePath:  e.cfg.CorePath,
		RomPath:   e.cfg.RomPath,
		SystemDir: e.cfg.SystemDir,
		Upscale:   e.cfg.Upscale,
		Native:    e.cfg.Native,
		Logger:    e.log,
		Video:     sink.VideoFrame,
		Audio:     sink.AudioSamples,
	})
	if err != nil {
		return err
	}
	defer game.Close()
	av := game.AV()
	if aspect := game.Aspect(); aspect > 0 {
		sink.SetAspect(aspect)
	}
	if e.cfg.OnReady != nil {
		e.cfg.OnReady(av)
	}
	e.log.Info("game running", "core", game.Info().Name, "rom", e.cfg.RomPath, "size", [2]int{av.BaseWidth, av.BaseHeight}, "fps", av.FPS, "sample_rate", av.SampleRate)

	frameDur := game.FrameDuration()
	ticker := time.NewTicker(frameDur)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		if e.cfg.Paused != nil && e.cfg.Paused() {
			// Keep the video alive with the last picture; the game waits.
			if frame, w, h := game.LastFrame(); frame != nil {
				sink.VideoFrame(frame, w, h, frameDur)
			}
			continue
		}
		game.SetPads(portPads(sink))
		game.Run()
	}
}

// portPads reads the controllers of the players seated at ports 1-4.
func portPads(sink MediaSink) (pads [4]input.Pad) {
	for port := range pads {
		pads[port] = sink.PortPad(port + 1)
	}
	return pads
}
