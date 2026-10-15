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
	"sync"
	"sync/atomic"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/internal/services"
	"github.com/lordbasex/go-link-console/backend-device/pkg/emuproc"
	"github.com/lordbasex/go-link-console/backend-device/pkg/input"
)

// emulateConfig is what the "emulate" worker runs.
type emulateConfig struct {
	CorePath, RomPath, SystemDir string
	StatePath                    string // optional save state loaded at start
	// Video is how frames are converted (--video: native, box or double);
	// the parent can change it later with a VideoMode message.
	Video emuproc.VideoMode
	// Upscale enlarges every frame that many times instead (--upscale,
	// go-link HD's 640x360 screen).
	Upscale int
	// Language is go-link HD's texts' language (--language).
	Language string
	// Native runs go-link HD's engine through its own API (--golinkhd): --core is its library, --rom a .glhd.
	Native bool
}

// runEmulate is the hidden "emulate" subcommand: an emulator worker that
// runs one game and speaks the pkg/emuproc protocol, commands on stdin and
// media on stdout, with its log on stderr. The device starts one per game
// (services.WorkerSource), so several games can run at once.
//
//	device emulate --core PATH --rom PATH --system DIR [--state PATH] [--video native|box|double] [--upscale N]
func runEmulate(args []string) error {
	fs := flag.NewFlagSet("emulate", flag.ContinueOnError)
	var cfg emulateConfig
	fs.StringVar(&cfg.CorePath, "core", "", "libretro core library")
	fs.StringVar(&cfg.RomPath, "rom", "", "ROM set to run")
	fs.StringVar(&cfg.SystemDir, "system", "", "system folder: BIOS, samples, NVRAM")
	fs.StringVar(&cfg.StatePath, "state", "", "save state to load after the game starts")
	fs.BoolVar(&cfg.Native, "golinkhd", false, "--core is go-link HD's engine (libgolinkhd), run through its own API, and --rom a game package (.glhd)")
	fs.StringVar(&cfg.Language, "language", "", "go-link HD's texts and dialogs in en, es or pt (with --golinkhd)")
	fs.IntVar(&cfg.Upscale, "upscale", 0, "enlarge every frame this many times with nearest neighbour (go-link HD), instead of --video")
	video := fs.String("video", "native", "how frames are converted: native, box (2x2 averaged color) or double (2x nearest neighbour)")
	probe := fs.String("probe", "", `test the game's saves instead of streaming it: "save" or "load" (see runProbe)`)
	probeFile := fs.String("probe-file", "", "the save state the probe writes (save) or reads (load)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if cfg.CorePath == "" || cfg.RomPath == "" || cfg.SystemDir == "" {
		return errors.New("emulate: --core, --rom and --system are required")
	}
	mode, err := emuproc.ParseVideoMode(*video)
	if err != nil {
		return err
	}
	cfg.Video = mode
	if *probe != "" {
		return runProbe(cfg, *probe, *probeFile, os.Stdout)
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil)).With("rom", filepath.Base(cfg.RomPath))
	// Cores may print to stdout; keep the protocol on a private copy of
	// it and send everything else written to stdout to stderr.
	out, err := protocolStdout()
	if err != nil {
		return err
	}
	defer out.Close()
	return emulate(cfg, os.Stdin, out, log)
}

// emulate loads the game and runs it at its own frame rate until Quit or
// the end of in (the parent died). The core runs on this goroutine, pinned
// to one OS thread; commands are read by another goroutine and the ones
// that touch the core are handed over between frames.
func emulate(cfg emulateConfig, in io.Reader, out io.Writer, log *slog.Logger) error {
	w := emuproc.NewWriter(out)
	var (
		writeErr error
		hidden   bool // warm-up frames are not sent
	)
	fail := func(err error) error {
		_ = w.Write(emuproc.TypeError, []byte(err.Error()))
		_ = w.Flush()
		return err
	}
	var game *services.GameCore // set before the first frame runs
	game, err := services.OpenGameCore(services.GameCoreConfig{
		CorePath:  cfg.CorePath,
		RomPath:   cfg.RomPath,
		SystemDir: cfg.SystemDir,
		Logger:    log,
		VideoMode: cfg.Video,
		Upscale:   cfg.Upscale,
		Language:  cfg.Language,
		Native:    cfg.Native,
		Video: func(i420 []byte, width, height int, dur time.Duration) {
			if writeErr == nil && !hidden {
				writeErr = w.WriteVideo(width, height, game.VideoScale(), dur, i420)
			}
		},
		Audio: func(pcm []int16) {
			if writeErr == nil && !hidden {
				writeErr = w.WriteAudio(pcm)
			}
		},
	})
	if err != nil {
		return fail(err)
	}
	defer game.Close()
	complete := game.SavesComplete()
	if !complete && cfg.StatePath != "" {
		// Loading it would crash the game (it restarts, or hangs on its
		// RAM/ROM check): start it from power on instead.
		log.Warn("this game cannot resume from a save in this emulator: starting it over", "state", cfg.StatePath)
		cfg.StatePath = ""
	}
	if cfg.StatePath != "" {
		hidden = true
		err := loadStartState(game, cfg.StatePath)
		hidden = false
		if err != nil {
			return fail(err)
		}
	}
	av, info := game.AV(), game.Info()
	if err := w.WriteJSON(emuproc.TypeReady, emuproc.Ready{
		Core:        info.Name,
		CoreVersion: info.Version,
		BaseWidth:   av.BaseWidth,
		BaseHeight:  av.BaseHeight,
		MaxWidth:    av.MaxWidth,
		MaxHeight:   av.MaxHeight,
		AspectRatio: game.Aspect(),
		FPS:         av.FPS,
		SampleRate:  av.SampleRate,

		SavesIncomplete: !complete,
	}); err != nil {
		return err
	}
	if err := w.Flush(); err != nil {
		return err
	}
	log.Info("game running", "core", info.Name, "size", [2]int{av.BaseWidth, av.BaseHeight}, "fps", av.FPS, "state", cfg.StatePath != "", "video", cfg.Video)

	cmds := newWorkerCommands()
	cmds.video.Store(uint32(cfg.Video))
	go cmds.read(in, log)

	pace := newPacer(game.FrameDuration(), time.Now())
	timer := time.NewTimer(0)
	defer timer.Stop()
	shown := false // a frame went out since the game started
	for {
		select {
		case <-cmds.quit:
			log.Info("worker stopping")
			return nil
		case c := <-cmds.state:
			if err := answerState(w, game, c); err != nil {
				return err
			}
			continue
		case <-timer.C:
		}
		now := time.Now()
		// The parent repeats the last picture while the game is paused. A
		// game paused before its first frame (a room saved paused comes back
		// paused as soon as it is ready) runs that one frame, so it shows
		// where it stopped instead of a black screen.
		if cmds.paused.Load() && shown {
			pace.rest(now)
			timer.Reset(pace.wait(now))
			continue
		}
		shown = true
		game.SetPads(cmds.pads())
		game.SetVideoMode(emuproc.VideoMode(cmds.video.Load()))
		// Behind the clock, a go-link HD game runs this frame without its
		// picture: its sound keeps coming at the game's speed (see pacer).
		if pace.frame(now, game.CanSkip()) {
			game.RunSkip()
		} else {
			game.Run()
		}
		timer.Reset(pace.wait(time.Now()))
		if writeErr == nil {
			writeErr = w.Flush()
		}
		if writeErr != nil {
			return fmt.Errorf("emulate: writing to the device: %w", writeErr)
		}
	}
}

// stateCommand is a SaveState or LoadState request.
type stateCommand struct {
	typ  emuproc.Type
	path string
}

// workerCommands is what the command reader shares with the game loop.
type workerCommands struct {
	mu      sync.Mutex
	current [emuproc.Ports]input.Pad
	paused  atomic.Bool
	video   atomic.Uint32 // the emuproc.VideoMode for the next frames
	state   chan stateCommand
	quit    chan struct{}
}

func newWorkerCommands() *workerCommands {
	return &workerCommands{state: make(chan stateCommand, 8), quit: make(chan struct{})}
}

func (c *workerCommands) pads() [emuproc.Ports]input.Pad {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.current
}

// read handles commands until Quit or the end of the stream, then closes
// quit.
func (c *workerCommands) read(in io.Reader, log *slog.Logger) {
	defer close(c.quit)
	r := emuproc.NewReader(in)
	for {
		t, p, err := r.Next()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Warn("reading commands", "err", err)
			}
			return
		}
		switch t {
		case emuproc.TypePads:
			pads, err := emuproc.DecodePads(p)
			if err != nil {
				log.Warn("bad pads", "err", err)
				continue
			}
			c.mu.Lock()
			c.current = pads
			c.mu.Unlock()
		case emuproc.TypePause:
			if paused, err := emuproc.DecodePause(p); err == nil {
				c.paused.Store(paused)
			}
		case emuproc.TypeSaveState, emuproc.TypeLoadState:
			c.state <- stateCommand{typ: t, path: string(p)}
		case emuproc.TypeVideoMode:
			m, err := emuproc.DecodeVideoMode(p)
			if err != nil {
				log.Warn("bad video mode", "err", err)
				continue
			}
			if old := emuproc.VideoMode(c.video.Swap(uint32(m))); old != m {
				log.Info("video mode", "from", old, "to", m)
			}
		case emuproc.TypeQuit:
			return
		default:
			log.Warn("unknown command", "type", t)
		}
	}
}

// answerState runs a save or load on the core's thread and replies.
func answerState(w *emuproc.Writer, game *services.GameCore, c stateCommand) error {
	reply := emuproc.TypeStateSaved
	var err error
	if c.typ == emuproc.TypeLoadState {
		reply = emuproc.TypeStateLoaded
		err = loadStateFile(game, c.path)
	} else {
		err = saveStateFile(game, c.path)
	}
	res := emuproc.StateResult{OK: err == nil, Path: c.path}
	if err != nil {
		res.Error = err.Error()
	}
	if err := w.WriteJSON(reply, res); err != nil {
		return err
	}
	return w.Flush()
}

// saveStateFile writes a save state atomically: a temporary file in the
// same folder renamed over the target, so a crash never leaves half a
// state behind.
func saveStateFile(game *services.GameCore, path string) error {
	if path == "" {
		return errors.New("no save state path")
	}
	state, err := game.SaveState()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after the rename
	if _, err := tmp.Write(state); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// startStateFrames bounds the warm-up frames loadStartState runs.
const startStateFrames = 10

// loadStartState restores a save state right after the game loads.
// mame2003-plus refuses a state until its first frame has run, so a few
// frames are run first; the caller keeps them off the stream.
func loadStartState(game *services.GameCore, path string) error {
	state, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read the save state: %w", err)
	}
	for i := 0; ; i++ {
		game.Run()
		err := game.LoadState(state)
		if err == nil {
			return nil
		}
		if i+1 >= startStateFrames {
			return fmt.Errorf("cannot load the save state: %w", err)
		}
	}
}

func loadStateFile(game *services.GameCore, path string) error {
	state, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read the save state: %w", err)
	}
	if err := game.LoadState(state); err != nil {
		return fmt.Errorf("cannot load the save state: %w", err)
	}
	return nil
}

// The save probe runs the game without pacing (a minute of game time takes
// a second or two) and without showing it.
const (
	probeWindow   = 300  // frames per measurement (5 s of game time)
	probeMinStart = 600  // frames before the save point at the earliest
	probeMaxStart = 3600 // frames to wait for the game to make a sound
	probeSound    = 1000 // samples that are not silence: "it made a sound"
)

// probePads plays like someone at the cabinet: a coin and then Start every
// four seconds (some boards take long to boot), so a game whose demo is
// silent makes its sounds and music.
func probePads(frame int) (pads [4]input.Pad) {
	switch t := frame % 240; {
	case frame < 240:
	case t < 8:
		pads[0].Buttons = input.State(input.Coin)
	case t >= 60 && t < 68:
		pads[0].Buttons = input.State(input.Start)
	}
	return pads
}

// runProbe is one half of services.ProbeSaves. "save" runs the game until it
// makes a sound (or probeMaxStart frames), saves it to file and counts the
// sound of the next probeWindow frames. "load" starts the game, loads file
// (as a room resuming does) and counts the sound of its first probeWindow
// frames. Both print an emuproc.ProbeResult.
func runProbe(cfg emulateConfig, mode, file string, out io.Writer) error {
	if file == "" || (mode != "save" && mode != "load") {
		return errors.New(`emulate: --probe needs "save" or "load" and --probe-file`)
	}
	sound, changes := 0, 0
	var last uint64
	game, err := services.OpenGameCore(services.GameCoreConfig{
		CorePath: cfg.CorePath, RomPath: cfg.RomPath, SystemDir: cfg.SystemDir,
		Video: func(i420 []byte, w, h int, _ time.Duration) {
			// A sample of the luma plane is enough to see the picture move.
			var sum uint64
			for i := 0; i < w*h && i < len(i420); i += 7 {
				sum = sum*31 + uint64(i420[i])
			}
			if sum != last {
				changes++
				last = sum
			}
		},
		Audio: func(pcm []int16) {
			for _, v := range pcm {
				if v != 0 {
					sound++
				}
			}
		},
	})
	if err != nil {
		return err
	}
	defer game.Close()
	var res emuproc.ProbeResult
	res.Complete, res.Modules = game.SaveContents()
	frame := 0
	window := func() int {
		sound, changes = 0, 0
		for range probeWindow {
			game.SetPads(probePads(frame))
			game.Run()
			frame++
		}
		return sound
	}
	if mode == "save" {
		for frames := 0; frames < probeMaxStart; frames += probeWindow {
			if window() >= probeSound && frames+probeWindow >= probeMinStart {
				break
			}
		}
		state, err := game.SaveState()
		if err != nil {
			// The core saves nothing for this game (the Midway T-Unit ones).
			res.Complete = false
			return json.NewEncoder(out).Encode(res)
		}
		if err := os.WriteFile(file, state, 0o600); err != nil {
			return err
		}
	} else if err := loadStartState(game, file); err != nil {
		return err
	}
	res.Sound = window()
	res.Changes = changes
	return json.NewEncoder(out).Encode(res)
}
