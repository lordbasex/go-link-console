// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"testing"

	"github.com/lordbasex/go-link-console/backend-device/pkg/input"
)

func TestPanelStartPressesAnotherPortsStart(t *testing.T) {
	seats := map[string]int{"ana": 1, "bob": 2}
	s := &StreamService{
		pads: map[string][input.MaxLocalPlayers]input.Pad{},
		room: RoomHooks{PortOf: func(peer string, local uint8) (int, bool) {
			p, ok := seats[peer]
			return p, ok && local == 0
		}},
	}
	// Ana (P1) presses the start button of player 2.
	s.pads["ana"] = [input.MaxLocalPlayers]input.Pad{{Buttons: input.State(input.Start2)}}
	if !s.PortPad(2).Buttons.Pressed(input.Start) {
		t.Fatal("P2 start is not down while P1 presses Start 2")
	}
	if s.PortPad(1).Buttons.Pressed(input.Start) {
		t.Fatal("Start 2 pressed P1's own start")
	}
	// A guest without a seat cannot press it.
	s.pads = map[string][input.MaxLocalPlayers]input.Pad{"eve": {{Buttons: input.State(input.Start3)}}}
	if s.PortPad(3).Buttons.Pressed(input.Start) {
		t.Fatal("a spectator pressed Start 3")
	}
}

func TestStartOf(t *testing.T) {
	if input.StartOf(1) != input.Start1 || input.StartOf(4) != input.Start4 || input.StartOf(0) != 0 || input.StartOf(5) != 0 {
		t.Fatal("StartOf maps ports 1-4 only")
	}
}
