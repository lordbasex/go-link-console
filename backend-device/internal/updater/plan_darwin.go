// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package updater

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// macPlan replaces the app bundle with the one in the release's disk
// image, once its signature is checked to be by the same developer as
// this one's.
type macPlan struct{ bundle string }

func (u *Updater) plan() (plan, error) {
	exe, err := u.exe()
	if err != nil {
		return nil, err
	}
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(exe))) // .app/Contents/MacOS/x
	if !strings.HasSuffix(bundle, ".app") {
		return nil, ErrManual
	}
	if !writable(filepath.Dir(bundle)) {
		return nil, ErrManual // Applications of another user: the person drags it
	}
	return &macPlan{bundle: bundle}, nil
}

func (p *macPlan) asset(tag string) string { return "go-link-console-" + tag + "-macos-universal.dmg" }

func (p *macPlan) install(ctx context.Context, dmg string) (func() error, error) {
	mnt, err := os.MkdirTemp("", "go-link-console-dmg-")
	if err != nil {
		return nil, err
	}
	defer os.Remove(mnt)
	if out, err := exec.CommandContext(ctx, "hdiutil", "attach", "-nobrowse", "-readonly", "-noautoopen", "-mountpoint", mnt, dmg).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("updater: cannot open the disk image: %v: %s", err, strings.TrimSpace(string(out)))
	}
	defer exec.Command("hdiutil", "detach", mnt, "-force").Run()
	apps, _ := filepath.Glob(filepath.Join(mnt, "*.app"))
	if len(apps) != 1 {
		return nil, errors.New("updater: the disk image has no app")
	}
	newApp := apps[0]
	mine, err := team(p.bundle)
	if err != nil || mine == "" {
		return nil, fmt.Errorf("updater: this copy is not signed by a developer, so nothing can be checked against it: %w", ErrManual)
	}
	if out, err := exec.CommandContext(ctx, "codesign", "--verify", "--deep", "--strict", newApp).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("%w: its signature is not valid: %s", ErrSignature, strings.TrimSpace(string(out)))
	}
	if theirs, err := team(newApp); err != nil || theirs != mine {
		return nil, fmt.Errorf("%w: signed by %q, not %q", ErrSignature, theirs, mine)
	}
	staged, old := p.bundle+".new", p.bundle+oldSuffix
	_ = os.RemoveAll(staged)
	_ = os.RemoveAll(old)
	if out, err := exec.CommandContext(ctx, "ditto", newApp, staged).CombinedOutput(); err != nil {
		_ = os.RemoveAll(staged)
		return nil, fmt.Errorf("updater: cannot copy the app: %v: %s", err, strings.TrimSpace(string(out)))
	}
	if err := os.Rename(p.bundle, old); err != nil {
		_ = os.RemoveAll(staged)
		return nil, err
	}
	if err := os.Rename(staged, p.bundle); err != nil {
		_ = os.Rename(old, p.bundle)
		_ = os.RemoveAll(staged)
		return nil, err
	}
	_ = os.RemoveAll(old)
	return afterExit(os.Getpid(), "/usr/bin/open", p.bundle), nil
}

var teamLine = regexp.MustCompile(`(?m)^TeamIdentifier=(\S+)$`)

// team is the developer team that signed an app ("" when none).
func team(app string) (string, error) {
	out, err := exec.Command("codesign", "-dv", "--verbose=2", app).CombinedOutput()
	if err != nil {
		return "", err
	}
	m := teamLine.FindSubmatch(out)
	if m == nil || string(m[1]) == "not" {
		return "", nil
	}
	return string(m[1]), nil
}
