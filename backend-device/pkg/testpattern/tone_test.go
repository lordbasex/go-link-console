// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package testpattern

import (
	"math"
	"testing"

	"github.com/lordbasex/go-link-console/backend-device/pkg/encoder"
)

func TestToneIsOneKilohertz(t *testing.T) {
	tone := NewTone(1000)
	var samples []int16
	for i := 0; i < 5; i++ { // 100 ms
		samples = append(samples, tone.Frame()...)
	}
	// Count upward zero crossings: 1 kHz gives 100 in 100 ms.
	crossings := 0
	peak := 0
	for i := 1; i < len(samples); i++ {
		if samples[i-1] < 0 && samples[i] >= 0 {
			crossings++
		}
		peak = max(peak, int(math.Abs(float64(samples[i]))))
	}
	if crossings < 99 || crossings > 101 {
		t.Fatalf("%d crossings in 100 ms", crossings)
	}
	// -18 dBFS is about 4125.
	if peak < 4000 || peak > 4200 {
		t.Fatalf("peak %d", peak)
	}
	if len(tone.Frame()) != encoder.OpusFrameSamples {
		t.Fatal("frame size")
	}
}
