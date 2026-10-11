// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"github.com/lordbasex/go-link-console/backend-device/internal/telemetry"
)

// The quality ladder moves one player between the video tiers of their
// size (video_tiers.go): the full bitrate, then half, then a quarter.
// Lost packets or a frozen picture in their browser's report step down at
// once, so a struggling connection gets a picture it can carry before it
// freezes again; 20 clean seconds step back up, one at a time, so the
// quality does not swing. It only changes what that player gets: the
// others keep their own tier.

// qualityCleanReports is how many clean reports (2 s each) step back up.
const qualityCleanReports = 10

// qualityLossPct is the video loss, in percent, that steps down.
const qualityLossPct = 3

type qualityLadder struct {
	clean int
}

// note takes one client report and returns the new drop (0 to
// qualitySteps) and why it changed, or the same drop and "".
func (l *qualityLadder) note(drop int, m map[string]float64) (int, string) {
	if m["hidden"] == 1 {
		return drop, "" // a hidden tab measures nothing useful
	}
	why := ""
	switch {
	case m["video_loss_pct"] >= qualityLossPct:
		why = "lost video packets"
	case m["freeze_ms"] >= float64(frameGapEvent.Milliseconds()):
		why = "a freeze"
	}
	if why != "" {
		l.clean = 0
		if drop < qualitySteps {
			return drop + 1, why
		}
		return drop, ""
	}
	l.clean++
	if l.clean >= qualityCleanReports && drop > 0 {
		l.clean = 0
		return drop - 1, "a clean spell"
	}
	return drop, ""
}

// moveQuality steps a player's video down or up from one client report, in
// a tiered stream, and logs each step with its reason. The new tier starts
// with a keyframe.
func (s *StreamService) moveQuality(v *viewer, m telemetry.Metrics) {
	if s.tiers == nil {
		return
	}
	s.mu.Lock()
	from := s.tierOfLocked(v)
	drop, why := v.quality.note(v.drop, m)
	v.drop = drop
	to := s.tierOfLocked(v)
	sizes := max(s.tierLevels, 1)
	kbps := s.Bitrate()
	s.mu.Unlock()
	if from == to {
		return
	}
	s.tiers[to].keyframe.Store(true)
	s.teleEvent(telemetry.Info, "video_quality", v.id, "the participant's video quality changed after "+why, map[string]any{
		"from_kbps": stepKbps(kbps, from, sizes), "to_kbps": stepKbps(kbps, to, sizes), "step": drop,
	})
	go s.sendStreamStats()
}
