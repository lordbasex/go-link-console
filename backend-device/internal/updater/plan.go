// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package updater

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// plan is how this copy of go-link Console is updated: which release file
// it takes and how that file goes in.
type plan interface {
	asset(tag string) string
	install(ctx context.Context, path string) (restart func() error, err error)
}

// bare is a version without its v: 0.4.3.
func bare(tag string) string { return strings.TrimPrefix(tag, "v") }

// writable reports whether this process may make files in dir.
func writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".go-link-console-")
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(f.Name())
	return true
}

// Cleanup removes what an update left next to this program: the files it
// replaced, which Windows keeps while they run. The device calls it when
// it starts.
func Cleanup() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return
	}
	old, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), "*"+oldSuffix))
	for _, f := range old {
		_ = os.Remove(f)
	}
}

func pid() int { return os.Getpid() }
