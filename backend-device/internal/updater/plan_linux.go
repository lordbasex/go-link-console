// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package updater

// A Linux copy is the folder its .tar.gz was unpacked in (the build with
// the window; the window-less one has no update window).
func (u *Updater) plan() (plan, error) {
	exe, err := u.exe()
	if err != nil {
		return nil, err
	}
	return folderOf(exe, "go-link-console", "libgolinkhd.so", "go-link-console-%s-linux-"+arch()+".tar.gz",
		func(exe string) func() error { return afterExit(pid(), exe) })
}
