// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"github.com/lordbasex/go-link-console/backend-device/internal/models"
	"testing"

	"github.com/lordbasex/go-link-console/backend-device/pkg/libretro"
)

func TestMAMEsOwnButtonsNeverReachTheCore(t *testing.T) {
	// mame2003-plus: L3 turns the sound off, R2 opens MAME's menu.
	for _, id := range []int{libretro.JoypadL2, libretro.JoypadR2, libretro.JoypadL3, libretro.JoypadR3} {
		if _, ok := retroButton[id]; ok {
			t.Errorf("RetroPad id %d reaches the core", id)
		}
	}
	for _, id := range []int{libretro.JoypadB, libretro.JoypadA, libretro.JoypadY, libretro.JoypadX, libretro.JoypadL, libretro.JoypadR, libretro.JoypadStart, libretro.JoypadSelect} {
		if _, ok := retroButton[id]; !ok {
			t.Errorf("game button %d is missing", id)
		}
	}
}

func TestSavesCompleteNeedsEveryCPUsRegisters(t *testing.T) {
	line := func(item string) string { return "[MAME 2003+]     " + item + ": b003..b022" }
	generic := []string{line("cpu.0.irq enable"), line("cpu.0.watchdog count"), line("memory.0.ram"), line("bank.1.bank")}
	for _, c := range []struct {
		name  string
		items []string
		want  bool
	}{
		// X-Men vs. Street Fighter: 68000 (CPU 0) and Z80 (CPU 1) both save.
		{"xmvsf", []string{line("m68000.0.D"), line("m68000.0.PC"), line("z80.1.AF"), line("z80.1.PC")}, true},
		// The Simpsons, Aliens: the Konami CPU 0 saves nothing of its own.
		{"simpsons", []string{line("z80.1.AF"), line("z80.1.PC"), line("k052109.0.ram")}, false},
		// One CPU that saves nothing.
		{"none", nil, false},
		// A core that logs no list: nothing to go by.
		{"no list", []string{"[MAME 2003+] Git Version 3141930"}, true},
	} {
		lines := c.items
		if c.name != "no list" {
			lines = append(append([]string{}, generic...), c.items...)
		}
		if got := savesComplete(lines); got != c.want {
			t.Errorf("%s: savesComplete = %v, want %v", c.name, got, c.want)
		}
	}
}

// A go-link HD frame reaches the room's size: a 360p game enlarged, a game drawn at 720p or 1080p as it comes.
func TestHDUpscale(t *testing.T) {
	for _, c := range []struct{ up, w, h, want int }{
		{2, 640, 360, 2}, {3, 640, 360, 3}, {3, 360, 640, 3}, {2, 480, 360, 2},
		{2, 1280, 720, 1}, {3, 1280, 720, 1}, {3, 1920, 1080, 1}, {2, 1920, 1080, 1},
		{0, 640, 360, 1}, {3, 0, 0, 1},
	} {
		if got := hdUpscale(c.up, c.w, c.h); got != c.want {
			t.Errorf("hdUpscale(%d, %d, %d) = %d, want %d", c.up, c.w, c.h, got, c.want)
		}
	}
}

// A go-link HD room's bitrate: its own at high, less for normal and saver.
func TestHDPlanKbps(t *testing.T) {
	if hdPlanKbps(8000, models.VideoHigh) != 8000 || hdPlanKbps(8000, models.VideoNormal) != 5333 || hdPlanKbps(8000, models.VideoSaver) != 4000 {
		t.Error(hdPlanKbps(8000, models.VideoHigh), hdPlanKbps(8000, models.VideoNormal), hdPlanKbps(8000, models.VideoSaver))
	}
}
