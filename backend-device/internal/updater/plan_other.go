// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !darwin && !windows && !linux

package updater

func (u *Updater) plan() (plan, error) { return nil, ErrManual }
