// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package libretro_test

import (
	"bytes"
	"hash/crc32"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/lordbasex/go-link-console/backend-device/pkg/cores"
	"github.com/lordbasex/go-link-console/backend-device/pkg/libretro"
)

// TestSaveStateWithRealCore needs the mame2003-plus core and the free ROM
// robby.zip in ~/go-link; it is skipped otherwise.
func TestSaveStateWithRealCore(t *testing.T) {
	home, _ := os.UserHomeDir()
	corePath := filepath.Join(home, "go-link", "cores", cores.FileName(cores.DefaultCore, runtime.GOOS))
	romPath := filepath.Join(home, "go-link", "roms", "robby.zip")
	if _, err := os.Stat(corePath); err != nil {
		t.Skip("mame2003-plus core not installed")
	}
	if _, err := os.Stat(romPath); err != nil {
		t.Skip("robby.zip not in the ROM folder")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var sum uint32
	system := t.TempDir()
	core, err := libretro.Open(corePath, libretro.Config{
		SystemDir: system,
		SaveDir:   system,
		Options: map[string]string{
			"mame2003-plus_skip_disclaimer": "enabled",
			"mame2003-plus_skip_warnings":   "enabled",
		},
		Handlers: libretro.Handlers{
			Video: func(f libretro.Frame) {
				if f.Data != nil {
					sum = crc32.ChecksumIEEE(f.Data)
				}
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	if _, err := core.Load(romPath); err != nil {
		t.Fatal(err)
	}
	run := func(frames int) uint32 {
		for i := 0; i < frames; i++ {
			core.Run()
		}
		return sum
	}
	start := run(600)

	size := core.StateSize()
	if size <= 0 {
		t.Fatalf("state size %d", size)
	}
	state, err := core.SaveState()
	if err != nil {
		t.Fatal(err)
	}
	if len(state) != size {
		t.Fatalf("state is %d bytes, size said %d", len(state), size)
	}
	first := run(90)
	if first == start {
		t.Log("the picture did not change in 90 frames")
	}

	// Going back to the snapshot replays the same frames.
	if err := core.LoadState(state); err != nil {
		t.Fatal(err)
	}
	if again := run(90); again != first {
		t.Fatalf("after LoadState the picture is %08x, want %08x", again, first)
	}
	again, err := core.SaveState()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(again, state) {
		t.Log("the machine state did not change in 90 frames")
	}
	if err := core.LoadState(nil); err == nil {
		t.Fatal("an empty state was accepted")
	}
}
