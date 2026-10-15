// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package updater

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// testExe is a program inside an app bundle in a folder the test writes.
func testExe(t *testing.T) string {
	exe := filepath.Join(t.TempDir(), "go-link-console.app", "Contents", "MacOS", "go-link-console")
	_ = os.MkdirAll(filepath.Dir(exe), 0o755)
	return exe
}

func TestMacPlan(t *testing.T) {
	u := &Updater{Exe: testExe(t)}
	p, err := u.plan()
	if err != nil {
		t.Fatal(err)
	}
	if got := p.asset("v0.4.3"); got != "go-link-console-v0.4.3-macos-universal.dmg" {
		t.Errorf("asset %s", got)
	}
	if _, err := (&Updater{Exe: "/usr/local/bin/go-link-console"}).plan(); !errors.Is(err, ErrManual) {
		t.Errorf("a program outside an app: %v", err)
	}
}
