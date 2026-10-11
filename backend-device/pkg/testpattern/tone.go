// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package testpattern

import (
	"math"

	"github.com/lordbasex/go-link-console/backend-device/pkg/encoder"
)

// Tone generates the 1 kHz reference tone that accompanied test cards on
// broadcast TV: a pure sine wave, here at -18 dBFS, mono, 48 kHz.
type Tone struct {
	phase float64
	step  float64
	amp   float64
	frame []int16
}

// NewTone creates a tone of the given frequency.
func NewTone(hz float64) *Tone {
	return &Tone{
		step:  2 * math.Pi * hz / encoder.OpusSampleRate,
		amp:   32767 * math.Pow(10, -18.0/20), // -18 dBFS
		frame: make([]int16, encoder.OpusFrameSamples),
	}
}

// Samples returns the next n samples (mono). The slice is reused.
func (t *Tone) Samples(n int) []int16 {
	if cap(t.frame) < n {
		t.frame = make([]int16, n)
	}
	out := t.frame[:n]
	for i := range out {
		out[i] = int16(t.amp * math.Sin(t.phase))
		t.phase += t.step
	}
	t.phase = math.Mod(t.phase, 2*math.Pi)
	return out
}

// Frame returns the next 20 ms of samples. The phase continues across
// frames, so there are no clicks. The slice is reused.
func (t *Tone) Frame() []int16 { return t.Samples(encoder.OpusFrameSamples) }
