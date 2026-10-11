// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"

	"github.com/lordbasex/go-link-console/backend-device/pkg/emuproc"
)

// probeSound is how many samples that are not silence mean "the game made a
// sound" (the worker counts the same way).
const probeSound = 1000

// SaveProbeConfig describes the game ProbeSaves tests.
type SaveProbeConfig struct {
	// Exe is the go-link device binary; the default is the running one.
	Exe               string
	CorePath, RomPath string
	// Needs are the save modules this game's chips need (the catalog's
	// SaveModules): a save without one of them is incomplete.
	Needs []string
	// Command builds the worker process from its arguments (tests).
	Command func(ctx context.Context, args []string) *exec.Cmd
}

// ProbeSaves tells whether the emulator saves a game whole, so a room can
// resume it. MAME 2003-Plus (MAME 0.78) leaves out some CPUs (the Konami
// CPU of The Simpsons or Aliens: the game crashes) and some sound chips
// (the QSound of Capcom's CPS2 games: the game resumes silent). It runs
// the game twice, each in a worker process with a throwaway system folder,
// the way a room resumes: once to save it at a moment it makes sound, and
// once from power on loading that save. The save is incomplete when a CPU
// is missing from it or the sound is gone after loading. It takes a few
// seconds. A game that freezes after loading fails too.
func ProbeSaves(ctx context.Context, cfg SaveProbeConfig) (bool, error) {
	dir, err := os.MkdirTemp("", "go-link-probe-")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(dir)
	system, file := filepath.Join(dir, "system"), filepath.Join(dir, "probe.state")
	run := func(mode string) (emuproc.ProbeResult, error) {
		args := []string{"emulate", "--core", cfg.CorePath, "--rom", cfg.RomPath, "--system", system, "--probe", mode, "--probe-file", file}
		var cmd *exec.Cmd
		if cfg.Command != nil {
			cmd = cfg.Command(ctx, args)
		} else {
			exe := cfg.Exe
			if exe == "" {
				if exe, err = os.Executable(); err != nil {
					return emuproc.ProbeResult{}, err
				}
			}
			cmd = exec.CommandContext(ctx, exe, args...)
		}
		out, err := cmd.Output()
		if err != nil {
			return emuproc.ProbeResult{}, fmt.Errorf("save probe (%s): %w", mode, err)
		}
		return lastProbeResult(out)
	}
	saved, err := run("save")
	if err != nil {
		return false, err
	}
	if !saved.Complete {
		return false, nil
	}
	for _, m := range cfg.Needs {
		if !slices.Contains(saved.Modules, m) {
			return false, nil // this core does not save that chip
		}
	}
	loaded, err := run("load")
	if err != nil {
		return false, err
	}
	soundLost := saved.Sound >= probeSound && loaded.Sound == 0
	// A game that hangs after loading keeps one picture (a core that saves
	// a CPU but not a board's banks: the CPU runs the wrong code).
	frozen := saved.Changes >= probeMoving && loaded.Changes <= 1
	return !soundLost && !frozen, nil
}

// probeMoving is how many picture changes in the probe window mean "the
// game was moving" (out of 300 frames).
const probeMoving = 30

// lastProbeResult reads the worker's answer: its last JSON line (a core may
// print other things on stdout).
func lastProbeResult(out []byte) (emuproc.ProbeResult, error) {
	var res emuproc.ProbeResult
	found := false
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		var r emuproc.ProbeResult
		if json.Unmarshal(sc.Bytes(), &r) == nil {
			res, found = r, true
		}
	}
	if !found {
		return res, errors.New("save probe: no answer from the worker")
	}
	return res, nil
}
