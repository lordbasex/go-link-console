// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/lordbasex/go-link-console/backend-device/pkg/emuproc"
)

// romTestWorker answers like "romtest --child": with for the run with the
// scripted input, without for the run without it.
func romTestWorker(t *testing.T, with, without emuproc.RomTestReport) func(context.Context, []string) *exec.Cmd {
	dir := t.TempDir()
	write := func(name string, r emuproc.RomTestReport) string {
		b, _ := json.Marshal(r)
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, append([]byte("core says hello\n"), append(b, '\n')...), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	a, b := write("a.json", with), write("b.json", without)
	return func(ctx context.Context, args []string) *exec.Cmd {
		i := slices.Index(args, "--script")
		if i >= 0 && args[i+1] != "" {
			return exec.CommandContext(ctx, "cat", a)
		}
		return exec.CommandContext(ctx, "cat", b)
	}
}

// goodRun is a set that boots: a picture at frame 9, moving, sound
// callbacks, and Coin changes the picture.
func goodRun(input bool) emuproc.RomTestReport {
	r := emuproc.RomTestReport{Loaded: true, Core: "MAME 2003-Plus", Width: 384, Height: 224, FPS: 60, Frames: 900, Seconds: 3,
		FirstPicture: 9, AudioCalls: 900, AudioSamples: 900 * 1600, FirstSound: -1, Shot: "iVBORw0KGgo="}
	for i := range 900 {
		h := uint32(i / 2)
		if input && i >= 310 {
			h += 100000
		}
		r.Hashes = append(r.Hashes, h)
	}
	return r
}

func zipBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("prog.rom")
	_, _ = w.Write([]byte("our program"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newTestRomTests(t *testing.T, cmd func(context.Context, []string) *exec.Cmd) (*RomTestService, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "romtest")
	core := filepath.Join(t.TempDir(), "core.dylib")
	if err := os.WriteFile(core, []byte("core"), 0o600); err != nil {
		t.Fatal(err)
	}
	return NewRomTestService(RomTestConfig{Dir: dir, CorePath: func() string { return core }, Command: cmd,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}), dir
}

func TestRomTestUploadAndRun(t *testing.T) {
	tests, dir := newTestRomTests(t, romTestWorker(t, goodRun(true), goodRun(false)))
	st := NewStatusService(deviceID, "test", "ws://x", "")
	romsDir := t.TempDir()
	lib := NewLibraryService(romsDir, st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	var mu sync.Mutex
	var replies []FileReply
	up := NewUploadService(lib, func(_ string, r FileReply) {
		mu.Lock()
		replies = append(replies, r)
		mu.Unlock()
	})
	body := zipBytes(t)
	send := func(id, name, purpose string) {
		begin, _ := json.Marshal(map[string]any{"type": "begin", "id": id, "name": name, "size": len(body), "purpose": purpose})
		up.Handle("p", true, begin)
		up.Handle("p", false, body)
		up.Handle("p", true, []byte(`{"type":"end","id":"`+id+`"}`))
	}

	// Without tests turned on, a test upload is refused.
	send("t0", "game.zip", "rom_test")
	up.SetTests(tests)
	send("t1", "game.zip", "rom_test")
	send("t2", "../evil.zip", "rom_test")
	send("bad id!", "game.zip", "rom_test")
	send("t3", "game.zip", "other")
	mu.Lock()
	if len(replies) != 5 || replies[0].OK || !replies[1].OK || replies[2].OK || replies[3].OK || replies[4].OK {
		t.Fatalf("replies %+v", replies)
	}
	mu.Unlock()
	// The set waits in the test folder, never in the ROM folder.
	if b, err := os.ReadFile(filepath.Join(dir, "t1", "game.zip")); err != nil || !bytes.Equal(b, body) {
		t.Fatalf("test upload not stored: %v", err)
	}
	if entries, _ := os.ReadDir(romsDir); len(entries) != 0 {
		t.Fatalf("the ROM folder was touched: %v", entries)
	}
	if fi, _ := os.Stat(filepath.Join(dir, "t1", "game.zip")); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode())
	}

	res := tests.Run(context.Background(), "t1", "game", 0)
	if !res.OK || res.Type != "rom_test_result" || res.ID != "t1" || res.Frames != 900 || res.Shot == "" {
		t.Fatalf("result %+v", res)
	}
	names := []string{}
	for _, s := range res.Steps {
		names = append(names, s.Name)
	}
	want := []string{"zip", "set", "identity", "core.loaded", "core.files", "video.picture", "video.alive", "audio", "input.reacts", "time.realtime"}
	if !slices.Equal(names, want) {
		t.Fatalf("steps %v", names)
	}
	if _, err := os.Stat(filepath.Join(dir, "t1")); !os.IsNotExist(err) {
		t.Fatal("the test folder must be deleted after the test")
	}
	// Run once more: the set is gone.
	if res := tests.Run(context.Background(), "t1", "game", 0); res.OK || res.Code != RomTestNotFound {
		t.Fatalf("second run %+v", res)
	}
}

func TestRomTestOneAtATime(t *testing.T) {
	tests, _ := newTestRomTests(t, romTestWorker(t, goodRun(true), goodRun(false)))
	if err := tests.StoreTest("t1", "game.zip", bytes.NewReader(zipBytes(t))); err != nil {
		t.Fatal(err)
	}
	tests.busy.Lock()
	res := tests.Run(context.Background(), "t1", "game", 0)
	tests.busy.Unlock()
	if res.OK || res.Code != RomTestBusy {
		t.Fatalf("busy %+v", res)
	}
	// The set was kept: it runs once the other test ends.
	if res := tests.Run(context.Background(), "t1", "game", 0); !res.OK {
		t.Fatalf("after %+v", res)
	}
}

func step(res RomTestResult, name string) RomTestStep {
	for _, s := range res.Steps {
		if s.Name == name {
			return s
		}
	}
	return RomTestStep{Name: name + " (missing)"}
}

func TestRomTestFailures(t *testing.T) {
	run := func(t *testing.T, with, without emuproc.RomTestReport) RomTestResult {
		tests, _ := newTestRomTests(t, romTestWorker(t, with, without))
		if err := tests.StoreTest("t1", "game.zip", bytes.NewReader(zipBytes(t))); err != nil {
			t.Fatal(err)
		}
		return tests.Run(context.Background(), "t1", "game", 900)
	}
	black := goodRun(true)
	black.FirstPicture = -1
	if res := run(t, black, goodRun(false)); res.OK || step(res, "video.picture").OK {
		t.Fatalf("black screen passed: %+v", res.Steps)
	}
	frozen := goodRun(true)
	for i := range frozen.Hashes {
		frozen.Hashes[i] = 7
	}
	if res := run(t, frozen, frozen); res.OK || step(res, "video.alive").OK || step(res, "input.reacts").OK {
		t.Fatalf("frozen passed: %+v", res.Steps)
	}
	deaf := goodRun(true)
	if res := run(t, deaf, goodRun(true)); res.OK || !strings.Contains(step(res, "input.reacts").Detail, "changed nothing") {
		t.Fatalf("inputs ignored passed: %+v", res.Steps)
	}
	missing := goodRun(true)
	missing.NotFound = []string{"prog.rom"}
	if res := run(t, missing, goodRun(false)); res.OK || step(res, "core.files").OK {
		t.Fatalf("missing file passed: %+v", res.Steps)
	}
	notLoaded := emuproc.RomTestReport{Error: "cannot load the game"}
	if res := run(t, notLoaded, notLoaded); res.OK || step(res, "core.loaded").OK {
		t.Fatalf("not loaded passed: %+v", res.Steps)
	}
	slow := goodRun(true)
	slow.Seconds = 30
	if res := run(t, slow, goodRun(false)); res.OK || step(res, "time.realtime").OK {
		t.Fatalf("slow passed: %+v", res.Steps)
	}
	mute := goodRun(true)
	mute.AudioCalls, mute.AudioSamples = 0, 0
	if res := run(t, mute, goodRun(false)); res.OK || step(res, "audio").OK {
		t.Fatalf("no audio passed: %+v", res.Steps)
	}

	// A crash in the worker.
	tests, _ := newTestRomTests(t, func(ctx context.Context, _ []string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "echo 'SIGSEGV: segmentation violation' >&2; exit 2")
	})
	if err := tests.StoreTest("t1", "game.zip", bytes.NewReader(zipBytes(t))); err != nil {
		t.Fatal(err)
	}
	res := tests.Run(context.Background(), "t1", "game", 0)
	if s := step(res, "core.run"); res.OK || s.OK || !strings.Contains(s.Detail, "crashed") {
		t.Fatalf("crash %+v", res.Steps)
	}

	// Not a zip: refused when it arrives.
	if err := tests.StoreTest("t2", "game.zip", strings.NewReader("hello")); err == nil {
		t.Fatal("not a zip was stored")
	}
}

func TestRomTestCleanupKeepsFreshRuns(t *testing.T) {
	tests, dir := newTestRomTests(t, nil)
	if err := tests.StoreTest("t1", "game.zip", bytes.NewReader(zipBytes(t))); err != nil {
		t.Fatal(err)
	}
	tests.Cleanup()
	if _, err := os.Stat(filepath.Join(dir, "t1")); err != nil {
		t.Fatal("a fresh upload was removed")
	}
	// A new upload replaces the one waiting.
	if err := tests.StoreTest("t2", "game.zip", bytes.NewReader(zipBytes(t))); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "t1")); !os.IsNotExist(err) {
		t.Fatal("the older upload must go")
	}
}
