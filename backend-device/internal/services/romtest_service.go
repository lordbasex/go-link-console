// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/pkg/emuproc"
	"github.com/lordbasex/go-link-console/backend-device/pkg/ownsets"
	"github.com/lordbasex/go-link-console/backend-device/pkg/romcheck"
)

// The ROM test (validation level 4, docs/willy-maker/validation.md): the
// owner's browser sends a set on the files channel with purpose "rom_test"
// and asks for rom_test; the device powers it on with the exact core in a
// worker process ("device romtest --child", isolated like a game room's
// "emulate" worker), twice from power on (once with scripted Coin, Start
// and buttons, once without), and answers rom_test_result. The set never
// goes near the ROM folder and is deleted when the test ends.
const (
	// RomTestFrames is how many frames a test runs by default (15 s of
	// game time at 60 Hz, unpaced: a few seconds).
	RomTestFrames = 900
	// RomTestMinFrames and RomTestMaxFrames bound the frames asked for.
	RomTestMinFrames = 600
	RomTestMaxFrames = 3600
	// RomTestMaxSize bounds a set sent for a test.
	RomTestMaxSize = 16 << 20
	// RomTestTimeout bounds a whole test (both runs).
	RomTestTimeout = 60 * time.Second
	// RomTestMaxShot bounds the PNG of the last frame: in base64, with the
	// steps, it must fit in one control message (64 KB).
	RomTestMaxShot = 36 << 10
	// romTestCoin is the frame of the first scripted press: the pictures
	// of both runs must be the same before it.
	romTestCoin = 300
	// romTestScript is the scripted input (framelab syntax): Coin, Start,
	// right held, then each button, on player 1.
	romTestScript = "300-307:coin 360-367:start 420-539:right 560-565:b1 600-605:b2 640-645:b3"
	// romTestNoInput keeps the second run's controls idle.
	romTestNoInput = ""
	// romTestPictureBy is when the first picture must be on screen.
	romTestPictureBy = 300
	// romTestAliveWindow and romTestAliveMin: at least this many different
	// pictures in the last window of frames (no freeze).
	romTestAliveWindow = 300
	romTestAliveMin    = 10
)

// Error codes of rom_test_result.
const (
	RomTestBusy     = "busy"      // another test is running
	RomTestNotFound = "not_found" // no set was uploaded with this id
)

// RomTestStep is one check of a ROM test.
type RomTestStep struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// RomTestOwn names the go-link set the tested zip is, when it is one.
type RomTestOwn struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// RomTestResult is the answer to rom_test (and what "device romtest"
// prints with --json).
type RomTestResult struct {
	Type    string        `json:"type"` // "rom_test_result"
	ID      string        `json:"id"`
	Set     string        `json:"set"`
	OK      bool          `json:"ok"`
	Steps   []RomTestStep `json:"steps"`
	Frames  int           `json:"frames"`
	Seconds float64       `json:"seconds"`
	Shot    string        `json:"shot,omitempty"` // PNG of the last frame, base64
	Own     *RomTestOwn   `json:"own,omitempty"`
	Error   string        `json:"error,omitempty"`
	Code    string        `json:"code,omitempty"`
}

// RomTestConfig sets up the ROM tests.
type RomTestConfig struct {
	// Dir holds each test's files: Dir/<id>/<set>.zip and its system
	// folders (~/go-link-console/tmp/romtest).
	Dir      string
	CorePath func() string
	Catalog  func() *romcheck.Catalog // nil or returning nil: the set check is skipped
	// Exe is the go-link device binary; the default is the running one.
	Exe string
	// Command builds a worker process from its arguments (tests).
	Command func(ctx context.Context, args []string) *exec.Cmd
	Timeout time.Duration // default RomTestTimeout
	Logger  *slog.Logger
}

// RomTestService receives sets for tests and runs one test at a time.
type RomTestService struct {
	cfg  RomTestConfig
	busy sync.Mutex
}

// NewRomTestService builds the service.
func NewRomTestService(cfg RomTestConfig) *RomTestService {
	if cfg.Timeout <= 0 {
		cfg.Timeout = RomTestTimeout
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &RomTestService{cfg: cfg}
}

var romTestIDRE = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Cleanup removes what earlier tests left behind (the device stopped in
// the middle of one). Folders younger than a minute may belong to a test
// running in another process ("device romtest") and stay.
func (s *RomTestService) Cleanup() {
	entries, err := os.ReadDir(s.cfg.Dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if fi, err := e.Info(); err == nil && time.Since(fi.ModTime()) < time.Minute {
			continue
		}
		_ = os.RemoveAll(filepath.Join(s.cfg.Dir, e.Name()))
	}
}

// CheckTestUpload validates a test upload before its bytes arrive.
func (s *RomTestService) CheckTestUpload(id, fileName string, size int64) error {
	if !romTestIDRE.MatchString(id) {
		return errors.New("bad test id")
	}
	if _, err := romTestSetName(fileName); err != nil {
		return err
	}
	if size <= 0 || size > RomTestMaxSize {
		return fmt.Errorf("a set for a test must be a zip of at most %d MB", RomTestMaxSize>>20)
	}
	return nil
}

// StoreTest writes an uploaded set to Dir/<id>/<set>.zip (0600). Only one
// set waits for its test at a time: older uploads are removed.
func (s *RomTestService) StoreTest(id, fileName string, r io.Reader) error {
	if err := s.CheckTestUpload(id, fileName, 1); err != nil {
		return err
	}
	set, _ := romTestSetName(fileName)
	s.dropOthers(id)
	dir := filepath.Join(s.cfg.Dir, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, set+".zip"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	head := make([]byte, 4)
	if _, err := io.ReadFull(r, head); err != nil || !bytes.Equal(head, []byte("PK\x03\x04")) {
		f.Close()
		_ = os.RemoveAll(dir)
		return ErrBadRom
	}
	n, err := io.Copy(f, io.MultiReader(bytes.NewReader(head), io.LimitReader(r, RomTestMaxSize)))
	if err == nil && n > RomTestMaxSize {
		err = ErrBadRom
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.RemoveAll(dir)
	}
	return err
}

// dropOthers removes the uploads waiting for a test, except keep's and the
// CLI's runs ("run-*", see RunFile).
func (s *RomTestService) dropOthers(keep string) {
	entries, err := os.ReadDir(s.cfg.Dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.Name() == keep || strings.HasPrefix(e.Name(), "run-") {
			continue
		}
		_ = os.RemoveAll(filepath.Join(s.cfg.Dir, e.Name()))
	}
}

func romTestSetName(fileName string) (string, error) {
	base := strings.ToLower(filepath.Base(fileName))
	name, ok := strings.CutSuffix(base, ".zip")
	if !ok || !romNameRE.MatchString(name) || base != strings.ToLower(fileName) {
		return "", ErrBadRom
	}
	return name, nil
}

// Run tests the set uploaded with id. One test runs at a time; the
// uploaded files are deleted when it ends.
func (s *RomTestService) Run(ctx context.Context, id, set string, frames int) RomTestResult {
	res := RomTestResult{Type: "rom_test_result", ID: id, Set: set, Steps: []RomTestStep{}}
	if !romTestIDRE.MatchString(id) || !romNameRE.MatchString(set) {
		res.Error, res.Code = "bad test id or set name", RomTestNotFound
		return res
	}
	dir := filepath.Join(s.cfg.Dir, id)
	if !s.busy.TryLock() {
		res.Error, res.Code = "a ROM test is already running on this device", RomTestBusy
		return res
	}
	defer s.busy.Unlock()
	defer os.RemoveAll(dir)
	zipPath := filepath.Join(dir, set+".zip")
	if _, err := os.Stat(zipPath); err != nil {
		res.Error, res.Code = "send the set first (files channel, purpose rom_test)", RomTestNotFound
		return res
	}
	out := s.run(ctx, zipPath, set, frames)
	out.ID = id
	return out
}

// RunFile tests a zip from the disk (the CLI): it is copied to a test
// folder first, so the original is never touched.
func (s *RomTestService) RunFile(ctx context.Context, path string, frames int) RomTestResult {
	set := strings.ToLower(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	res := RomTestResult{Type: "rom_test_result", Set: set, Steps: []RomTestStep{}}
	if !romNameRE.MatchString(set) || !strings.EqualFold(filepath.Ext(path), ".zip") {
		res.Error = "the file must be a set: a .zip with a short name, e.g. robby.zip"
		return res
	}
	if !s.busy.TryLock() {
		res.Error, res.Code = "a ROM test is already running", RomTestBusy
		return res
	}
	defer s.busy.Unlock()
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	dir := filepath.Join(s.cfg.Dir, "run-"+hex.EncodeToString(b))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		res.Error = err.Error()
		return res
	}
	defer os.RemoveAll(dir)
	if err := copyLimited(path, filepath.Join(dir, set+".zip"), RomTestMaxSize); err != nil {
		res.Error = err.Error()
		return res
	}
	return s.run(ctx, filepath.Join(dir, set+".zip"), set, frames)
}

func copyLimited(src, dst string, limit int64) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if fi, err := in.Stat(); err != nil || fi.Size() > limit {
		return fmt.Errorf("a set for a test must be a zip of at most %d MB", limit>>20)
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, io.LimitReader(in, limit))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

// run does the checks on a set in its test folder.
func (s *RomTestService) run(ctx context.Context, zipPath, set string, frames int) (res RomTestResult) {
	res = RomTestResult{Type: "rom_test_result", Set: set, Steps: []RomTestStep{}}
	if frames <= 0 {
		frames = RomTestFrames
	}
	frames = min(max(frames, RomTestMinFrames), RomTestMaxFrames)
	start := time.Now()
	defer func() {
		res.Seconds = time.Since(start).Seconds()
		res.OK = res.Error == "" && len(res.Steps) > 0
		for _, st := range res.Steps {
			res.OK = res.OK && st.OK
		}
	}()
	step := func(name string, ok bool, detail string, args ...any) bool {
		res.Steps = append(res.Steps, RomTestStep{Name: name, OK: ok, Detail: fmt.Sprintf(detail, args...)})
		return ok
	}

	core := ""
	if s.cfg.CorePath != nil {
		core = s.cfg.CorePath()
	}
	if _, err := os.Stat(core); err != nil {
		res.Error = "the emulator core is not installed on this device"
		return res
	}

	// 1. A readable zip.
	files, err := zipFiles(zipPath)
	if err != nil {
		step("zip", false, "not a valid zip: %v", err)
		return res
	}
	step("zip", true, "%d files", files)

	// 2. The core knows the set and every file it needs is there (the
	// same check as the ROM library, without running anything).
	if cat := s.catalog(); cat != nil {
		check := romcheck.NewChecker(cat, filepath.Dir(zipPath)).Check(set)
		if check.Status != romcheck.StatusOK {
			step("set", false, "%s", check.Reason())
			return res
		}
		title := set
		if g := cat.Game(set); g != nil {
			title = g.Title
		}
		step("set", true, "the core's %s (%s layout)", set, title)
	} else {
		step("set", true, "skipped: the core's game list is not installed")
	}

	// 3. Is it a go-link set? Decided by the files' hashes only.
	if own, _ := ownsets.Match(zipPath, ownsets.All()); own != nil {
		res.Own = &RomTestOwn{ID: own.ID, Title: own.Title}
		step("identity", true, "go-link set %q, verified by the SHA-256 of its %d files", own.Title, len(own.Files))
	} else {
		step("identity", true, "not a go-link set: it shows as the core's original game")
	}

	// 4. Power on, twice: with the scripted input and without it.
	ctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()
	work := filepath.Dir(zipPath)
	a, err := s.child(ctx, zipPath, filepath.Join(work, "system-a"), frames, romTestScript)
	if err != nil {
		step("core.run", false, "%s", s.childError(ctx, err))
		return res
	}
	if !a.Loaded {
		step("core.loaded", false, "the core did not load the set: %s", a.Error)
		return res
	}
	res.Frames, res.Shot = a.Frames, a.Shot
	step("core.loaded", true, "%s %s, %dx%d at %.2f Hz", a.Core, strings.TrimSpace(a.CoreVersion), a.Width, a.Height, a.FPS)

	switch {
	case len(a.NotFound) > 0:
		step("core.files", false, "the core did not find: %s", strings.Join(a.NotFound, ", "))
	case len(a.BadLength) > 0:
		step("core.files", false, "wrong size: %s", strings.Join(a.BadLength, ", "))
	case len(a.WrongChecksum) > 0 && res.Own == nil:
		step("core.files", true, "%d files differ from the original set (the core warns, and runs them)", len(a.WrongChecksum))
	case len(a.WrongChecksum) > 0:
		step("core.files", true, "every file found with its size; %d differ from the original set, as expected for a go-link set", len(a.WrongChecksum))
	default:
		step("core.files", true, "every file found, with its checksum")
	}

	switch {
	case a.FirstPicture < 0:
		step("video.picture", false, "the screen stayed black for all %d frames", a.Frames)
	case a.FirstPicture >= romTestPictureBy:
		step("video.picture", false, "the first picture came at frame %d; it must come before frame %d", a.FirstPicture, romTestPictureBy)
	default:
		step("video.picture", true, "first picture at frame %d", a.FirstPicture)
	}

	distinct := distinctTail(a.Hashes, romTestAliveWindow)
	step("video.alive", distinct >= romTestAliveMin, "%d different pictures in the last %d frames (at least %d)", distinct, romTestAliveWindow, romTestAliveMin)

	expected := int(float64(a.Frames) / max(a.FPS, 1) * 48000 * 2)
	switch {
	case a.AudioCalls == 0 || a.AudioSamples*2 < expected:
		step("audio", false, "the core sent %d of about %d sound samples", a.AudioSamples, expected)
	case a.Sound == 0:
		step("audio", true, "sound runs, silent in these %d frames", a.Frames)
	default:
		step("audio", true, "sound from frame %d", a.FirstSound)
	}

	b, err := s.child(ctx, zipPath, filepath.Join(work, "system-b"), frames, romTestNoInput)
	switch {
	case err != nil:
		step("input.reacts", false, "the run without input stopped: %s", s.childError(ctx, err))
	case !b.Loaded || len(b.Hashes) != len(a.Hashes):
		step("input.reacts", false, "the run without input did not run the same frames")
	default:
		ok, detail := compareRuns(a.Hashes, b.Hashes, romTestCoin)
		step("input.reacts", ok, "%s", detail)
	}

	speed := 0.0
	if a.Seconds > 0 && a.FPS > 0 {
		speed = float64(a.Frames) / a.Seconds / a.FPS
	}
	step("time.realtime", speed >= 1, "%d frames in %.1f s: %.1fx real time", a.Frames, a.Seconds, speed)
	return res
}

func (s *RomTestService) catalog() *romcheck.Catalog {
	if s.cfg.Catalog == nil {
		return nil
	}
	return s.cfg.Catalog()
}

// child runs one power-on in a worker process and reads its report.
func (s *RomTestService) child(ctx context.Context, rom, system string, frames int, script string) (emuproc.RomTestReport, error) {
	core := ""
	if s.cfg.CorePath != nil {
		core = s.cfg.CorePath()
	}
	args := []string{"romtest", "--child", "--core", core, "--rom", rom, "--system", system, "--frames", fmt.Sprint(frames), "--script", script}
	var cmd *exec.Cmd
	if s.cfg.Command != nil {
		cmd = s.cfg.Command(ctx, args)
	} else {
		exe := s.cfg.Exe
		if exe == "" {
			var err error
			if exe, err = os.Executable(); err != nil {
				return emuproc.RomTestReport{}, err
			}
		}
		cmd = exec.CommandContext(ctx, exe, args...)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &limitedBuffer{buf: &stderr, max: 4096}
	out, err := cmd.Output()
	if err != nil {
		// The first line says why (a crash: "SIGSEGV: segmentation violation").
		first, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n")
		s.cfg.Logger.Warn("rom test worker stopped", "err", err, "why", first)
		if first != "" {
			err = &workerError{err: err, why: first}
		}
		return emuproc.RomTestReport{}, err
	}
	var rep emuproc.RomTestReport
	found := false
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	for sc.Scan() {
		var r emuproc.RomTestReport
		if json.Unmarshal(sc.Bytes(), &r) == nil && (r.Loaded || r.Error != "") {
			rep, found = r, true
		}
	}
	if !found {
		return rep, errors.New("no answer from the worker")
	}
	return rep, nil
}

func (s *RomTestService) childError(ctx context.Context, err error) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Sprintf("the test took longer than %s and was stopped", s.cfg.Timeout)
	}
	if ctx.Err() != nil {
		return "the test was canceled"
	}
	var why *workerError
	if errors.As(err, &why) && strings.HasPrefix(why.why, "SIG") {
		return fmt.Sprintf("the set crashed the emulator (%s)", why.why)
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return fmt.Sprintf("the emulator stopped (%v): the set crashed the core", exit)
	}
	return err.Error()
}

// workerError is a worker that stopped, with the first line it printed.
type workerError struct {
	err error
	why string
}

func (e *workerError) Error() string { return e.err.Error() + ": " + e.why }
func (e *workerError) Unwrap() error { return e.err }

// compareRuns tells whether the inputs changed the picture: both runs
// must show the same pictures before the first press, and some picture
// after it must differ.
func compareRuns(with, without []uint32, press int) (bool, string) {
	press = min(press, len(with))
	for i := range press {
		if with[i] != without[i] {
			return false, fmt.Sprintf("the game does not run the same twice from power on (frame %d differs before any input), so its reaction cannot be told", i)
		}
	}
	for i := press; i < len(with); i++ {
		if with[i] != without[i] {
			return true, fmt.Sprintf("the picture changed at frame %d, after Coin at frame %d (the run without input stayed the same until then)", i, press)
		}
	}
	return false, "Coin, Start, the stick and the buttons changed nothing on screen"
}

// distinctTail counts the different pictures in the last n frames.
func distinctTail(hashes []uint32, n int) int {
	seen := map[uint32]bool{}
	for _, h := range hashes[max(len(hashes)-n, 0):] {
		seen[h] = true
	}
	return len(seen)
}

// zipFiles counts the files of a zip (its directory only).
func zipFiles(path string) (int, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return 0, err
	}
	defer zr.Close()
	n := 0
	for _, f := range zr.File {
		if !f.FileInfo().IsDir() {
			n++
		}
	}
	if n == 0 {
		return 0, errors.New("it is empty")
	}
	return n, nil
}

// limitedBuffer keeps the first max bytes written to it.
type limitedBuffer struct {
	buf *bytes.Buffer
	max int
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if room := l.max - l.buf.Len(); room > 0 {
		l.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}
