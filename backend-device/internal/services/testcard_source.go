// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/pkg/testpattern"
)

// TestCardSource streams the test card and the 1 kHz reference tone: the
// room's echo test.
type TestCardSource struct {
	Width, Height, FPS int
	// Paused, when set and true, holds the card: the last picture repeats
	// and the tone stops (the test room's pause, for testing that flow).
	Paused func() bool
}

// Run produces frames at FPS until ctx ends.
func (t *TestCardSource) Run(ctx context.Context, sink MediaSink) error {
	pattern := testpattern.New(t.Width, t.Height)
	tone := testpattern.NewTone(1000)
	sink.SetAspect(float64(t.Width) / float64(t.Height))
	interval := time.Second / time.Duration(t.FPS)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	stereo := make([]int16, 0, 2*48000/t.FPS)
	var last []byte
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		if t.Paused != nil && t.Paused() && last != nil {
			sink.VideoFrame(last, t.Width, t.Height, interval)
			continue
		}
		last = pattern.Frame(time.Now(), sink.Controls())
		sink.VideoFrame(last, t.Width, t.Height, interval)
		stereo = stereo[:0]
		for _, v := range tone.Samples(48000 / t.FPS) {
			stereo = append(stereo, v, v)
		}
		sink.AudioSamples(stereo)
	}
}
