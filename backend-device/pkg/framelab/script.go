// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package framelab

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/lordbasex/go-link-console/backend-device/pkg/input"
)

// Script is scripted controller input for an emulator run. Each step
// holds buttons on one port for a range of frames, in the device's input
// bits, so the core sees exactly what a guest's controller would send.
//
// Text form, steps separated by spaces:
//
//	FROM-TO:BUTTONS[@PORT]
//
// FROM and TO are frame numbers (TO inclusive), BUTTONS are input names
// joined by "+" (up, down, left, right, b1..b6, start, coin), and PORT is
// 1-4 (default 1). The special button "play" plays on its own: it walks
// in changing directions and taps attack buttons, the same every run.
// Example: "300-305:coin 400-405:start 600-3000:play".
type Script []Step

// Step is one line of a Script.
type Step struct {
	From, To int
	Port     int // 0-3
	Buttons  input.State
	Play     bool
}

var buttonNames = map[string]input.Button{
	"up": input.Up, "down": input.Down, "left": input.Left, "right": input.Right,
	"b1": input.Button1, "b2": input.Button2, "b3": input.Button3,
	"b4": input.Button4, "b5": input.Button5, "b6": input.Button6,
	"start": input.Start, "coin": input.Coin,
}

// ParseScript reads the text form.
func ParseScript(text string) (Script, error) {
	var s Script
	for _, f := range strings.Fields(text) {
		rng, rest, ok := strings.Cut(f, ":")
		if !ok {
			return nil, fmt.Errorf("script step %q: want FROM-TO:BUTTONS", f)
		}
		a, b, ok := strings.Cut(rng, "-")
		from, err1 := strconv.Atoi(a)
		to, err2 := strconv.Atoi(b)
		if !ok || err1 != nil || err2 != nil || from < 0 || to < from {
			return nil, fmt.Errorf("script step %q: bad frame range", f)
		}
		st := Step{From: from, To: to}
		names, port, hasPort := strings.Cut(rest, "@")
		if hasPort {
			p, err := strconv.Atoi(port)
			if err != nil || p < 1 || p > 4 {
				return nil, fmt.Errorf("script step %q: port must be 1-4", f)
			}
			st.Port = p - 1
		}
		for _, n := range strings.Split(names, "+") {
			if n == "play" {
				st.Play = true
				continue
			}
			bt, ok := buttonNames[n]
			if !ok {
				return nil, fmt.Errorf("script step %q: unknown button %q", f, n)
			}
			st.Buttons |= input.State(bt)
		}
		s = append(s, st)
	}
	return s, nil
}

// Pads returns the controllers of the four ports at a frame.
func (s Script) Pads(frame int) [4]input.Pad {
	var pads [4]input.Pad
	for _, st := range s {
		if frame < st.From || frame > st.To {
			continue
		}
		b := st.Buttons
		if st.Play {
			b |= playPattern(frame - st.From)
		}
		pads[st.Port].Buttons |= b
	}
	return pads
}

// playPattern is a deterministic sequence of moves: every 24-48 frames a
// new direction (mostly forward), and an attack button tapped every few
// frames, like a beginner mashing buttons.
func playPattern(t int) input.State {
	dirs := []input.State{
		input.State(input.Right), input.State(input.Right | input.Up), input.State(input.Right),
		input.State(input.Left), input.State(input.Right | input.Down), input.State(input.Up),
		input.State(input.Right), input.State(input.Down),
	}
	seed := uint32(2463534242)
	seg, at := 0, 0
	for {
		seed ^= seed << 13
		seed ^= seed >> 17
		seed ^= seed << 5
		length := 24 + int(seed%25)
		if t < at+length {
			break
		}
		at += length
		seg++
	}
	st := dirs[seg%len(dirs)]
	if phase := t % 12; phase < 3 {
		buttons := []input.Button{input.Button1, input.Button2, input.Button1, input.Button3, input.Button4}
		st |= input.State(buttons[(t/12)%len(buttons)])
	}
	return st
}
