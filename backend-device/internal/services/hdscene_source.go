// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"image"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/pkg/hdscene"
)

// HDSceneSource streams go-link HD's test scene (experiment T-31,
// docs/experiments/hd-streaming.md) instead of the test card: two pictures
// scrolling at different speeds and a bouncing ball, at an HD size, with
// silence for sound. The test room uses it with --test-room-hd.
type HDSceneSource struct {
	Width, Height, FPS int
	Far, Play          image.Image
}

func (h *HDSceneSource) Run(ctx context.Context, sink MediaSink) error {
	scene := hdscene.New(h.Width, h.Height, h.Far, h.Play)
	sink.SetAspect(float64(h.Width) / float64(h.Height))
	interval := time.Second / time.Duration(h.FPS)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	silence := make([]int16, 2*48000/h.FPS)
	for t := 0; ; t++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		sink.VideoFrame(scene.Draw(t), h.Width, h.Height, interval)
		sink.AudioSamples(silence)
	}
}
