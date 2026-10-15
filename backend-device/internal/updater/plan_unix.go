// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !windows

package updater

import (
	"os/exec"
	"strconv"
	"syscall"
)

// afterExit runs a command once this process has quit, from a shell of
// its own.
func afterExit(pid int, name string, args ...string) func() error {
	return func() error {
		script := `while kill -0 ` + strconv.Itoa(pid) + ` 2>/dev/null; do sleep 0.2; done; exec "$@"`
		cmd := exec.Command("/bin/sh", append([]string{"-c", script, "sh", name}, args...)...)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		return cmd.Start()
	}
}
