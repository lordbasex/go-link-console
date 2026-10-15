// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package updater

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// A Windows copy is the folder its .zip was unpacked in.
func (u *Updater) plan() (plan, error) {
	exe, err := u.exe()
	if err != nil {
		return nil, err
	}
	return folderOf(exe, "go-link-console.exe", "golinkhd.dll", "go-link-console-%s-windows-"+arch()+".zip",
		func(exe string) func() error { return afterExit(pid(), exe) })
}

// afterExit opens a program once this process has quit (only one device
// runs at a time), from a hidden PowerShell of its own.
func afterExit(pid int, exe string) func() error {
	return func() error {
		quoted := "'" + strings.ReplaceAll(exe, "'", "''") + "'"
		script := "Wait-Process -Id " + strconv.Itoa(pid) + " -ErrorAction SilentlyContinue; Start-Process -FilePath " + quoted
		cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", script)
		const detached, newGroup = 0x00000008, 0x00000200
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detached | newGroup, HideWindow: true}
		return cmd.Start()
	}
}
