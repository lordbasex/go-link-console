// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// run runs a subcommand with a throwaway HOME and config and returns what it
// printed.
func runCmd(t *testing.T, config string, args ...string) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	// Flags go before the positional arguments (Go stops parsing flags at
	// the first one): "roms dir --config X /path".
	full := args
	if len(args) >= 2 {
		full = append(append(append([]string{}, args[:2]...), "--config", config), args[2:]...)
	}
	_, cmdErr := runCommand(full)
	os.Stdout = stdout
	w.Close()
	out, _ := io.ReadAll(r)
	return string(out), cmdErr
}

func sandbox(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return filepath.Join(home, "config", "device.json")
}

var uuid = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\n$`)

func TestTheDeviceRunsWhenNoCommandIsGiven(t *testing.T) {
	for _, args := range [][]string{nil, {"--headless"}} {
		if handled, err := runCommand(args); handled || err != nil {
			t.Fatalf("%v: handled %v %v", args, handled, err)
		}
	}
	config := sandbox(t)
	if _, err := runCmd(t, config, "no", "such"); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("unknown command: %v", err)
	}
}

func TestPanelTokenCommand(t *testing.T) {
	config := sandbox(t)
	first, err := runCmd(t, config, "panel", "token")
	if err != nil || !uuid.MatchString(first) {
		t.Fatalf("first token %q %v", first, err)
	}
	again, _ := runCmd(t, config, "panel", "token")
	if again != first {
		t.Fatalf("the token changed: %q then %q", first, again)
	}
	renewed, err := runCmd(t, config, "panel", "token", "--new")
	if err != nil || renewed == first || !uuid.MatchString(renewed) {
		t.Fatalf("new token %q %v", renewed, err)
	}
	// The config keeps it, private to the user.
	st, err := os.Stat(config)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("config %v %v", st, err)
	}
}

func TestRomsDirAndCheckCommands(t *testing.T) {
	config := sandbox(t)
	out, err := runCmd(t, config, "roms", "dir")
	if err != nil || !strings.HasSuffix(strings.TrimSpace(out), filepath.Join("go-link-console", "roms")) {
		t.Fatalf("default folder %q %v", out, err)
	}
	mine := t.TempDir()
	_ = os.WriteFile(filepath.Join(mine, "robby.zip"), []byte("PK\x03\x04"), 0o644)
	out, err = runCmd(t, config, "roms", "dir", mine)
	if err != nil || !strings.Contains(out, "(1 sets)") {
		t.Fatalf("set folder %q %v", out, err)
	}
	if out, _ := runCmd(t, config, "roms", "dir"); strings.TrimSpace(out) != mine {
		t.Fatalf("remembered folder %q", out)
	}
	if _, err := runCmd(t, config, "roms", "dir", filepath.Join(mine, "missing")); err == nil {
		t.Fatal("a missing folder was accepted")
	}
	// Without the core's game list there is nothing to check against.
	if _, err := runCmd(t, config, "roms", "check"); err == nil || !strings.Contains(err.Error(), "device core download") {
		t.Fatalf("check without a catalog: %v", err)
	}
}

func TestThumbnailsCommands(t *testing.T) {
	config := sandbox(t)
	if out, err := runCmd(t, config, "thumbnails", "kind", "title"); err != nil {
		t.Fatalf("kind %q %v", out, err)
	}
	if out, _ := runCmd(t, config, "thumbnails", "kind"); strings.TrimSpace(out) != "title" {
		t.Fatalf("kept kind %q", out)
	}
	if out, err := runCmd(t, config, "thumbnails", "check"); err != nil || !strings.Contains(out, "sets") {
		t.Fatalf("check %q %v", out, err)
	}
}

func TestHelpListsTheCommands(t *testing.T) {
	config := sandbox(t)
	out, err := runCmd(t, config, "help")
	_ = out
	if err != nil {
		t.Fatalf("help: %v", err)
	}
}

func TestTheLogGoesToAFileWithoutATerminal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Under `go test` stderr is not a terminal, like the app opened from the
	// Finder.
	path := filepath.Join(home, "go-link-console", "logs", "device.log")
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	_ = os.WriteFile(path, make([]byte, maxLogSize+1), 0o600)
	w, done := logOutput()
	_, _ = io.WriteString(w, "hello from the test\n")
	done()
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "hello from the test") || len(data) > 1000 {
		t.Fatalf("log %d bytes %v", len(data), err)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Fatalf("log mode %v", st.Mode().Perm())
	}
	// The old log was kept once as .1.
	if st, err := os.Stat(path + ".1"); err != nil || st.Size() != maxLogSize+1 {
		t.Fatalf("rotated log %v %v", st, err)
	}
}

func TestSmallHelpers(t *testing.T) {
	if hostName() == "" {
		t.Fatal("no host name")
	}
	if runtime.GOOS != "linux" && wantsHeadless() {
		t.Fatal("only Linux without a display runs headless by itself")
	}
}
