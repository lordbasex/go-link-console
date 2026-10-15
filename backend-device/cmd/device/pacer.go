// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import "time"

// pacer keeps the emulator worker at the game's frame rate by the clock.
// Every frame is run, so the game and its sound keep their speed; when the
// computer falls behind (a frame took longer than its time), a game that can
// (go-link HD) runs the next frames without drawing them, up to pacerSkips
// in a row, and catches up. The sound then never runs short, which a viewer
// hears as gaps; the picture shows fewer frames for a moment instead.
type pacer struct {
	dur     time.Duration
	next    time.Time // when the next frame is due
	skipped int       // frames in a row run without a picture
}

const (
	pacerSkips  = 2                      // a picture at least every third frame (20 a second at 60)
	pacerGiveUp = 250 * time.Millisecond // further behind, the clock starts again from now
)

func newPacer(dur time.Duration, now time.Time) *pacer {
	return &pacer{dur: dur, next: now}
}

// frame is called when a frame runs at now: whether it skips its picture
// (canSkip: the game can).
func (p *pacer) frame(now time.Time, canSkip bool) (skip bool) {
	skip = canSkip && now.Sub(p.next) >= p.dur && p.skipped < pacerSkips
	if skip {
		p.skipped++
	} else {
		p.skipped = 0
	}
	p.next = p.next.Add(p.dur)
	if now.Sub(p.next) > pacerGiveUp {
		p.next = now
	}
	return skip
}

// rest is a frame not run (the game is paused): the clock goes on from now,
// so resuming does not rush to catch up.
func (p *pacer) rest(now time.Time) {
	p.next = now.Add(p.dur)
	p.skipped = 0
}

// wait is how long until the next frame is due (0 when it is late).
func (p *pacer) wait(now time.Time) time.Duration {
	return max(0, p.next.Sub(now))
}
