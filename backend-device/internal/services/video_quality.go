// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"slices"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
	"github.com/lordbasex/go-link-console/backend-device/pkg/emuproc"
)

// Target bitrates of the video qualities. Measured on real games, a 2x
// picture at 3,500 kbps averages about 2,600 kbps (VP8's rate control
// spends less on the flat parts of pixel art).
const (
	HighKbps   = 3500 // 2x
	NormalKbps = 2500 // 2x
	SaverKbps  = 2500 // the game's size, like the test card
)

// VideoPlan is how a game room sends its picture.
type VideoPlan struct {
	Quality string            // models.VideoHigh, VideoNormal or VideoSaver
	Mode    emuproc.VideoMode // how the worker converts the frames
	Kbps    int               // VP8 target bitrate
}

// PlanFor returns the plan of a video quality (the default for an
// unknown one).
func PlanFor(quality string) VideoPlan {
	switch models.CleanVideoQuality(quality) {
	case models.VideoNormal:
		return VideoPlan{Quality: models.VideoNormal, Mode: emuproc.VideoDouble, Kbps: NormalKbps}
	case models.VideoSaver:
		return VideoPlan{Quality: models.VideoSaver, Mode: emuproc.VideoBox, Kbps: SaverKbps}
	}
	return VideoPlan{Quality: models.VideoHigh, Mode: emuproc.VideoDouble, Kbps: HighKbps}
}

// The encoder check of a 2x room: the first probeWarmup frames are left
// out (the first keyframe and libvpx settling), then the encode time of
// every frame over probeWindow is kept. When its 95th percentile takes
// more than slowShare of the time a frame lasts, 2x does not fit this
// computer next to what else it runs (other rooms included, since they
// share its cores), and the room falls back to saver.
const (
	probeWarmup     = 10
	probeWindow     = 2 * time.Second
	probeMinSamples = 30
	slowShare       = 0.6
)

// encodeProbe measures a room's encoder at the start of a 2x stream. It
// belongs to the goroutine that encodes.
type encodeProbe struct {
	frame   time.Duration // how long one frame lasts
	seen    int
	samples []time.Duration
	started time.Time
}

func newEncodeProbe(frame time.Duration) *encodeProbe {
	return &encodeProbe{frame: frame}
}

// add records one frame's encode time at now. It returns done once the
// window is full, with the 95th percentile and whether it is too slow.
func (p *encodeProbe) add(took time.Duration, now time.Time) (done bool, p95 time.Duration, slow bool) {
	p.seen++
	if p.seen <= probeWarmup {
		return false, 0, false
	}
	if p.started.IsZero() {
		p.started = now
	}
	p.samples = append(p.samples, took)
	if len(p.samples) < probeMinSamples || now.Sub(p.started) < probeWindow {
		return false, 0, false
	}
	p95 = percentile(p.samples, 0.95)
	return true, p95, tooSlow(p95, p.frame)
}

// tooSlow says whether an encode time leaves too little of the frame's
// time for everything else.
func tooSlow(p95, frame time.Duration) bool {
	return frame > 0 && float64(p95) > slowShare*float64(frame)
}

// percentile returns the q quantile (0-1) of the samples, nearest rank.
func percentile(samples []time.Duration, q float64) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	s := slices.Clone(samples)
	slices.Sort(s)
	i := int(q*float64(len(s))+0.5) - 1
	return s[max(0, min(i, len(s)-1))]
}
