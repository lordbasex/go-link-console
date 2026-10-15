// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"testing"
	"time"
)

func TestPacerOnTimeDrawsEveryFrame(t *testing.T) {
	start := time.Unix(1000, 0)
	dur := time.Second / 60
	p := newPacer(dur, start)
	now := start
	for i := range 120 {
		if p.frame(now, true) {
			t.Fatalf("frame %d skipped its picture while on time", i)
		}
		// each frame takes a third of its time
		now = now.Add(dur / 3)
		now = now.Add(p.wait(now))
	}
	if got := now.Sub(start); got < 2*time.Second-dur || got > 2*time.Second+dur {
		t.Fatalf("120 frames took %v, want about 2s", got)
	}
}

func TestPacerSlowFramesSkipPicturesAndKeepTheRate(t *testing.T) {
	start := time.Unix(1000, 0)
	dur := time.Second / 60
	p := newPacer(dur, start)
	now := start
	drawn, run := 0, 0
	for now.Sub(start) < 10*time.Second {
		skip := p.frame(now, true)
		run++
		if skip {
			now = now.Add(dur / 10) // the game and its sound only
		} else {
			drawn++
			now = now.Add(dur * 3 / 2) // a picture takes 25 ms
		}
		now = now.Add(p.wait(now))
	}
	// every frame of the 10 seconds ran (the sound is whole) ...
	if run < 590 || run > 610 {
		t.Fatalf("ran %d frames in 10s, want about 600", run)
	}
	// ... and the picture went down to what the computer can draw
	if drawn < 250 || drawn > 450 {
		t.Fatalf("drew %d pictures in 10s", drawn)
	}
}

func TestPacerWithoutSkipNeverSkips(t *testing.T) {
	start := time.Unix(1000, 0)
	dur := time.Second / 60
	p := newPacer(dur, start)
	now := start
	for range 100 {
		if p.frame(now, false) {
			t.Fatal("a game that cannot skip skipped")
		}
		now = now.Add(dur * 2)
	}
	// far behind, the clock started again instead of running frames in a burst
	if p.next.Before(now.Add(-pacerGiveUp - 2*dur)) {
		t.Fatalf("the pacer is %v behind", now.Sub(p.next))
	}
}

func TestPacerSkipsAtMostTwoInARow(t *testing.T) {
	start := time.Unix(1000, 0)
	dur := time.Second / 60
	p := newPacer(dur, start)
	now := start.Add(200 * time.Millisecond) // well behind
	row := 0
	for range 30 {
		if p.frame(now, true) {
			if row++; row > pacerSkips {
				t.Fatal("more than pacerSkips frames in a row without a picture")
			}
		} else {
			row = 0
		}
	}
}

func TestPacerRestDoesNotRush(t *testing.T) {
	start := time.Unix(1000, 0)
	dur := time.Second / 60
	p := newPacer(dur, start)
	p.frame(start, true)
	later := start.Add(5 * time.Second) // paused for 5 s
	p.rest(later)
	if w := p.wait(later); w != dur {
		t.Fatalf("after a pause the next frame waits %v, want %v", w, dur)
	}
	if p.frame(later.Add(dur), true) {
		t.Fatal("the first frame after a pause skipped its picture")
	}
}
