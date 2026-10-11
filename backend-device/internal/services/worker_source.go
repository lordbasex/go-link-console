// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/pkg/emuproc"
	"github.com/lordbasex/go-link-console/backend-device/pkg/libretro"
)

// WorkerConfig describes the game a WorkerSource runs.
type WorkerConfig struct {
	// Exe is the go-link device binary that runs "emulate"; the default is
	// the running executable.
	Exe                          string
	CorePath, RomPath, SystemDir string
	// StatePath is an optional save state loaded right after the game.
	StatePath string
	// Native runs go-link HD's engine (CorePath its library) instead of a libretro core.
	Native bool
	// Language is the texts' language of go-link HD's games (en, es, pt;
	// empty leaves the engine's English).
	Language string
	// Upscale enlarges the picture that many times (go-link HD's 640x360
	// screen: 2 for 720p, 3 for 1080p); 0 or 1 keeps the video mode.
	Upscale int
	Logger  *slog.Logger
	// OnReady runs once the game is loaded, before the first frame.
	OnReady func(libretro.AVInfo)
	// Command builds the worker process from its arguments; nil runs Exe.
	// Tests use it to start a fake worker.
	Command func(ctx context.Context, args []string) *exec.Cmd
}

// workerQuitDelay is how long a worker gets to exit after Quit before it
// is killed.
const workerQuitDelay = 3 * time.Second

// stderrLines is how many of the worker's last log lines an error keeps.
const stderrLines = 12

// errWorkerStopped is returned by commands sent while no worker runs.
var errWorkerStopped = errors.New("emulator worker is not running")

// WorkerSource runs a game in a child process (the "emulate" worker, see
// pkg/emuproc) and streams it. Unlike EmulatorSource, any number of them
// can run at once, and a crashing core only takes its own process down.
type WorkerSource struct {
	cfg     WorkerConfig
	log     *slog.Logger
	paused  atomic.Bool
	running atomic.Bool
	// video is the emuproc.VideoMode the worker converts frames with.
	video atomic.Uint32
	// savesIncomplete: the emulator does not save this game whole.
	savesIncomplete atomic.Bool

	wmu  sync.Mutex // guards w and done, and serializes writes to the worker
	w    *emuproc.Writer
	done chan struct{} // closed when the current Run ends

	stateMu sync.Mutex // one save or load at a time
	replyMu sync.Mutex
	reply   *pendingState

	sinkMu   sync.Mutex // serializes sink calls and guards the last frame
	last     []byte
	lw, lh   int
	lscale   int // the scale of last, as told to the sink
	frameDur atomic.Int64
}

// pendingState waits for the answer to a SaveState or LoadState.
type pendingState struct {
	typ  emuproc.Type
	path string
	ch   chan emuproc.StateResult
}

// NewWorkerSource prepares the source; the worker is started by Run.
func NewWorkerSource(cfg WorkerConfig) *WorkerSource {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	s := &WorkerSource{cfg: cfg, log: cfg.Logger}
	s.frameDur.Store(int64(time.Second / 60))
	return s
}

// Running reports whether the worker process is up.
func (s *WorkerSource) Running() bool { return s.running.Load() }

// SavesIncomplete reports whether the emulator saves this game only in
// part: such a game never resumes from a save (known once it is ready).
func (s *WorkerSource) SavesIncomplete() bool { return s.savesIncomplete.Load() }

// SetPaused holds or resumes the game. While paused the last picture is
// repeated and no sound is sent. It can be called before Run.
func (s *WorkerSource) SetPaused(paused bool) {
	s.paused.Store(paused)
	if err := s.send(func(w *emuproc.Writer) error { return w.WritePause(paused) }); err != nil && !errors.Is(err, errWorkerStopped) {
		s.log.Debug("pause", "err", err)
	}
}

// SetVideoMode picks how the worker converts frames: before Run it is
// the worker's --video flag, after it a VideoMode message (the next frame
// uses it).
func (s *WorkerSource) SetVideoMode(m emuproc.VideoMode) {
	if emuproc.VideoMode(s.video.Swap(uint32(m))) == m {
		return
	}
	if err := s.send(func(w *emuproc.Writer) error { return w.WriteVideoMode(m) }); err != nil && !errors.Is(err, errWorkerStopped) {
		s.log.Debug("video mode", "err", err)
	}
}

// VideoMode is the mode the worker converts frames with.
func (s *WorkerSource) VideoMode() emuproc.VideoMode { return emuproc.VideoMode(s.video.Load()) }

// SaveState asks the worker to write a save state to path and waits for
// the answer.
func (s *WorkerSource) SaveState(ctx context.Context, path string) error {
	return s.stateCommand(ctx, emuproc.TypeSaveState, emuproc.TypeStateSaved, path)
}

// LoadState asks the worker to restore the save state at path and waits
// for the answer.
func (s *WorkerSource) LoadState(ctx context.Context, path string) error {
	return s.stateCommand(ctx, emuproc.TypeLoadState, emuproc.TypeStateLoaded, path)
}

func (s *WorkerSource) stateCommand(ctx context.Context, cmd, reply emuproc.Type, path string) error {
	if path == "" {
		return errors.New("no save state path")
	}
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.wmu.Lock()
	done := s.done
	s.wmu.Unlock()
	if done == nil {
		return errWorkerStopped
	}
	p := &pendingState{typ: reply, path: path, ch: make(chan emuproc.StateResult, 1)}
	s.replyMu.Lock()
	s.reply = p
	s.replyMu.Unlock()
	defer func() {
		s.replyMu.Lock()
		if s.reply == p {
			s.reply = nil
		}
		s.replyMu.Unlock()
	}()
	if err := s.send(func(w *emuproc.Writer) error { return w.Write(cmd, []byte(path)) }); err != nil {
		return err
	}
	select {
	case res := <-p.ch:
		if !res.OK {
			return fmt.Errorf("%v: %s", cmd, res.Error)
		}
		return nil
	case <-done:
		return errWorkerStopped
	case <-ctx.Done():
		return ctx.Err()
	}
}

// send writes one command to the worker and flushes it.
func (s *WorkerSource) send(write func(*emuproc.Writer) error) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if s.w == nil {
		return errWorkerStopped
	}
	if err := write(s.w); err != nil {
		return err
	}
	return s.w.Flush()
}

func (s *WorkerSource) args() []string {
	args := []string{"emulate", "--core", s.cfg.CorePath, "--rom", s.cfg.RomPath, "--system", s.cfg.SystemDir}
	if m := s.VideoMode(); m != emuproc.VideoNative {
		args = append(args, "--video", m.String())
	}
	if s.cfg.StatePath != "" {
		args = append(args, "--state", s.cfg.StatePath)
	}
	if s.cfg.Native && s.cfg.Language != "" {
		args = append(args, "--language", s.cfg.Language)
	}
	if s.cfg.Upscale > 1 {
		args = append(args, "--upscale", strconv.Itoa(s.cfg.Upscale))
	}
	if s.cfg.Native {
		args = append(args, "--golinkhd")
	}
	return args
}

func (s *WorkerSource) command(ctx context.Context) (*exec.Cmd, error) {
	if s.cfg.Command != nil {
		return s.cfg.Command(ctx, s.args()), nil
	}
	exe := s.cfg.Exe
	if exe == "" {
		var err error
		if exe, err = os.Executable(); err != nil {
			return nil, err
		}
	}
	cmd := exec.CommandContext(ctx, exe, s.args()...)
	// Its own process group: Ctrl-C in the device's terminal must reach
	// only the device, which saves the game before stopping the worker.
	detachSignals(cmd)
	return cmd, nil
}

// Run starts the worker and streams its game until ctx ends (the worker
// is asked to quit, then killed if it does not) or the worker exits, which
// returns an error with its last log lines.
func (s *WorkerSource) Run(ctx context.Context, sink MediaSink) error {
	cmd, err := s.command(ctx)
	if err != nil {
		return err
	}
	tail := &lineTail{max: stderrLines, log: s.log}
	cmd.Stderr = tail
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	// When ctx ends, ask the worker to quit; exec kills it after WaitDelay.
	cmd.Cancel = func() error {
		_ = s.send(func(w *emuproc.Writer) error { return w.Write(emuproc.TypeQuit) })
		return stdin.Close()
	}
	cmd.WaitDelay = workerQuitDelay
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("cannot start the emulator worker: %w", err)
	}
	done := make(chan struct{})
	s.wmu.Lock()
	s.w, s.done = emuproc.NewWriter(stdin), done
	s.wmu.Unlock()
	s.running.Store(true)
	defer func() {
		s.wmu.Lock()
		s.w, s.done = nil, nil
		s.wmu.Unlock()
		s.running.Store(false)
		close(done)
	}()

	s.sinkMu.Lock()
	s.lscale = 0 // the first frame tells the sink its scale
	s.sinkMu.Unlock()
	pads := portPads(sink)
	if err := s.send(func(w *emuproc.Writer) error {
		if err := w.WritePause(s.paused.Load()); err != nil {
			return err
		}
		return w.WritePads(pads)
	}); err != nil {
		s.log.Debug("worker commands", "err", err)
	}

	var workerErr atomic.Pointer[string]
	ready := make(chan struct{}, 1)
	readDone := make(chan error, 1)
	go func() { readDone <- s.readLoop(stdout, sink, ready, &workerErr) }()

	ticker := time.NewTicker(time.Duration(s.frameDur.Load()))
	defer ticker.Stop()
	var readErr error
loop:
	for {
		select {
		case <-ctx.Done():
			readErr = <-readDone // exec quits or kills the worker
			break loop
		case readErr = <-readDone:
			break loop
		case <-ready:
			ticker.Reset(time.Duration(s.frameDur.Load()))
		case <-ticker.C:
			if s.paused.Load() {
				s.repeatLast(sink)
				continue
			}
			if now := portPads(sink); now != pads {
				pads = now
				if err := s.send(func(w *emuproc.Writer) error { return w.WritePads(pads) }); err != nil {
					s.log.Debug("worker pads", "err", err)
				}
			}
		}
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var msg []string
	if e := workerErr.Load(); e != nil {
		msg = append(msg, *e)
	}
	if waitErr != nil {
		msg = append(msg, waitErr.Error())
	} else if readErr != nil && !errors.Is(readErr, io.EOF) {
		msg = append(msg, readErr.Error())
	}
	if lines := tail.String(); lines != "" {
		msg = append(msg, "log:\n"+lines)
	}
	if len(msg) == 0 {
		msg = append(msg, "no reason given")
	}
	return fmt.Errorf("emulator worker stopped: %s", strings.Join(msg, "; "))
}

// readLoop handles the worker's messages until its stdout closes.
func (s *WorkerSource) readLoop(stdout io.Reader, sink MediaSink, ready chan<- struct{}, workerErr *atomic.Pointer[string]) error {
	r := emuproc.NewReader(stdout)
	var pcm []int16
	for {
		t, p, err := r.Next()
		if err != nil {
			return err
		}
		switch t {
		case emuproc.TypeVideo:
			v, err := emuproc.DecodeVideo(p)
			if err != nil {
				s.log.Warn("worker video", "err", err)
				continue
			}
			s.sinkMu.Lock()
			if len(v.I420) > 0 {
				s.last = append(s.last[:0], v.I420...)
				s.lw, s.lh = v.Width, v.Height
				if v.Scale != s.lscale {
					s.lscale = v.Scale
					if vs, ok := sink.(videoScaler); ok {
						vs.SetVideoScale(v.Scale)
					}
				}
			}
			if s.last != nil {
				sink.VideoFrame(s.last, s.lw, s.lh, v.Duration)
			}
			s.sinkMu.Unlock()
		case emuproc.TypeAudio:
			if pcm, err = emuproc.DecodeAudio(pcm[:0], p); err != nil {
				s.log.Warn("worker audio", "err", err)
				continue
			}
			s.sinkMu.Lock()
			sink.AudioSamples(pcm)
			s.sinkMu.Unlock()
		case emuproc.TypeReady:
			info, err := emuproc.DecodeReady(p)
			if err != nil {
				return fmt.Errorf("bad ready message: %w", err)
			}
			s.savesIncomplete.Store(info.SavesIncomplete)
			if info.FPS > 0 {
				s.frameDur.Store(int64(float64(time.Second) / info.FPS))
			}
			if info.AspectRatio > 0 {
				sink.SetAspect(info.AspectRatio)
			}
			select {
			case ready <- struct{}{}:
			default:
			}
			if s.cfg.OnReady != nil {
				s.cfg.OnReady(libretro.AVInfo{
					BaseWidth: info.BaseWidth, BaseHeight: info.BaseHeight,
					MaxWidth: info.MaxWidth, MaxHeight: info.MaxHeight,
					AspectRatio: info.AspectRatio, FPS: info.FPS, SampleRate: info.SampleRate,
				})
			}
			s.log.Info("game running", "core", info.Core, "rom", s.cfg.RomPath, "size", [2]int{info.BaseWidth, info.BaseHeight}, "fps", info.FPS, "sample_rate", info.SampleRate)
		case emuproc.TypeStateSaved, emuproc.TypeStateLoaded:
			res, err := emuproc.DecodeStateResult(p)
			if err != nil {
				s.log.Warn("worker state reply", "err", err)
				continue
			}
			s.replyMu.Lock()
			if s.reply != nil && s.reply.typ == t && s.reply.path == res.Path {
				s.reply.ch <- res
				s.reply = nil
			}
			s.replyMu.Unlock()
		case emuproc.TypeError:
			msg := string(p)
			workerErr.Store(&msg)
			s.log.Warn("emulator worker failed", "err", msg)
		case emuproc.TypeLog:
			s.log.Info("emulator worker", "msg", string(p))
		default:
			s.log.Debug("unknown worker message", "type", t)
		}
	}
}

// repeatLast sends the last picture again, so viewers keep an image while
// the game is paused.
func (s *WorkerSource) repeatLast(sink MediaSink) {
	s.sinkMu.Lock()
	defer s.sinkMu.Unlock()
	if s.last != nil {
		sink.VideoFrame(s.last, s.lw, s.lh, time.Duration(s.frameDur.Load()))
	}
}

// lineTail keeps the last lines written to it (the worker's stderr) and
// forwards each one to the log.
type lineTail struct {
	mu      sync.Mutex
	max     int
	log     *slog.Logger
	partial []byte
	lines   []string
}

func (t *lineTail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.partial = append(t.partial, p...)
	for {
		i := bytes.IndexByte(t.partial, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimRight(string(t.partial[:i]), "\r")
		t.partial = t.partial[i+1:]
		if line == "" {
			continue
		}
		t.log.Info("emulator worker", "log", line)
		t.lines = append(t.lines, line)
		if len(t.lines) > t.max {
			t.lines = t.lines[len(t.lines)-t.max:]
		}
	}
	return len(p), nil
}

// String returns the kept lines, including an unterminated last one.
func (t *lineTail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	lines := t.lines
	if rest := strings.TrimSpace(string(t.partial)); rest != "" {
		lines = append(lines[:len(lines):len(lines)], rest)
	}
	return strings.Join(lines, "\n")
}

// videoScaler is a sink that wants to know when frames are the game's
// picture enlarged (StreamService).
type videoScaler interface {
	SetVideoScale(scale int)
}

// videoModeSetter is a source that can send the picture enlarged 2x or
// with averaged color (WorkerSource).
type videoModeSetter interface {
	SetVideoMode(m emuproc.VideoMode)
	VideoMode() emuproc.VideoMode
}

var (
	_ MediaSource     = (*WorkerSource)(nil)
	_ videoModeSetter = (*WorkerSource)(nil)
	_ videoScaler     = (*StreamService)(nil)
)
