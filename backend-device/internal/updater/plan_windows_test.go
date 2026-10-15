// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package updater

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// testExe is a program in the folder its .zip was unpacked in.
func testExe(t *testing.T) string {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "golinkhd.dll"), []byte("lib"), 0o644)
	exe := filepath.Join(dir, "go-link-console.exe")
	_ = os.WriteFile(exe, []byte("exe"), 0o755)
	return exe
}

func TestWindowsPlan(t *testing.T) {
	p, err := (&Updater{Exe: testExe(t)}).plan()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := p.asset("v0.4.3"), "go-link-console-v0.4.3-windows-"+runtime.GOARCH+".zip"; got != want {
		t.Errorf("asset %s, want %s", got, want)
	}
	if _, err := (&Updater{Exe: filepath.Join(t.TempDir(), "go-link-console.exe")}).plan(); !errors.Is(err, ErrManual) {
		t.Errorf("a program without its engine next to it: %v", err)
	}
}
