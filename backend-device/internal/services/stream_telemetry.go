// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"encoding/json"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/internal/telemetry"
	"github.com/lordbasex/go-link-console/backend-device/pkg/input"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
)

// The room's telemetry, seen from the device: one sample per second of
// the picture (frames from the game, the longest pause between two,
// encoding) and of each participant (input, RTCP reports, voice, round
// trips), plus events for what stands out. The browsers add their own
// view with client_report.

// frameGapEvent is the pause between two frames from the game that is
// logged as an event (about 15 frames at 60 fps).
const frameGapEvent = 250 * time.Millisecond

// inputStall is how long a held control may go without a packet (the
// browser repeats it every 100 ms) before it is logged.
const inputStall = 400 * time.Millisecond

// frameTele counts the frames of the last second.
type frameTele struct {
	mu     sync.Mutex
	in     int
	last   time.Time
	gapMax time.Duration
	encN   int
	encSum time.Duration
	encMax time.Duration
	late   int // frames whose encoding took longer than a frame
}

// viewerTele counts one participant's traffic of the last second.
type viewerTele struct {
	inputPkts   atomic.Int64
	inputLost   atomic.Int64
	inputGapMax atomic.Int64 // ns, while a control was held
	pli         atomic.Int64
	nack        atomic.Int64
	rrLossPerMi atomic.Int64 // the last report's fraction lost, per mille
	rrJitterUs  atomic.Int64
	rrLost      atomic.Int64 // cumulative, from the last report
	voiceIn     atomic.Int64
	ctlRttMs    atomic.Int64 // the last ping's round trip, -1 before one
	lossWarned  atomic.Int64 // unix ms of the last loss event
	stallWarned atomic.Int64

	// under viewer.mu
	seq       [input.MaxLocalPlayers]uint16
	seqOK     [input.MaxLocalPlayers]bool
	lastInput time.Time
	held      bool
}

// SetTelemetry starts (rec) or stops (nil) recording this room's
// telemetry; host, when set, adds the computer's own numbers to each room
// sample (CPU, memory, its whole network traffic).
func (s *StreamService) SetTelemetry(rec *telemetry.Recorder, host func() telemetry.Metrics) {
	s.mu.Lock()
	s.teleHost = host
	s.mu.Unlock()
	s.tele.Store(rec)
}

// Telemetry is the room's recorder, or nil.
func (s *StreamService) Telemetry() *telemetry.Recorder { return s.tele.Load() }

func (s *StreamService) teleEvent(level telemetry.Level, kind, peer, msg string, data map[string]any) {
	s.tele.Load().Event(level, kind, peer, msg, data)
}

// noteFrame counts a frame from the source and logs a long pause before
// it: the game (or its process) sent nothing in that time.
func (s *StreamService) noteFrame(now time.Time) {
	f := &s.ftele
	f.mu.Lock()
	gap := time.Duration(0)
	if !f.last.IsZero() {
		gap = now.Sub(f.last)
	}
	f.last = now
	f.in++
	f.gapMax = max(f.gapMax, gap)
	f.mu.Unlock()
	if gap >= frameGapEvent && gap < 10*time.Minute {
		s.teleEvent(telemetry.Warn, "frame_gap", "", "no frame from the game", map[string]any{"gap_ms": gap.Milliseconds()})
	}
}

// resetFrameClock forgets the last frame (a new source starts).
func (s *StreamService) resetFrameClock() {
	s.ftele.mu.Lock()
	s.ftele.last = time.Time{}
	s.ftele.mu.Unlock()
}

// noteEncode counts the time one frame took to encode.
func (s *StreamService) noteEncode(took, frame time.Duration) {
	f := &s.ftele
	f.mu.Lock()
	f.encN++
	f.encSum += took
	f.encMax = max(f.encMax, took)
	if took > frame {
		f.late++
	}
	f.mu.Unlock()
}

// noteInput counts an input packet: lost ones are the gaps in its
// sequence, and a held control that goes quiet longer than inputStall is
// logged. Called with v.mu held.
func (s *StreamService) noteInput(v *viewer, raw []byte, now time.Time) {
	t := &v.tm
	t.inputPkts.Add(1)
	p, err := input.Decode(raw)
	if err != nil {
		return
	}
	i := p.Player
	if t.seqOK[i] {
		if d := int16(p.Seq - t.seq[i]); d > 0 {
			if d > 1 {
				t.inputLost.Add(int64(d - 1))
			}
			t.seq[i] = p.Seq
		}
	} else {
		t.seq[i], t.seqOK[i] = p.Seq, true
	}
	if t.held && !t.lastInput.IsZero() {
		gap := now.Sub(t.lastInput)
		if int64(gap) > t.inputGapMax.Load() {
			t.inputGapMax.Store(int64(gap))
		}
		if gap >= inputStall && now.UnixMilli()-t.stallWarned.Load() > 5000 {
			t.stallWarned.Store(now.UnixMilli())
			s.teleEvent(telemetry.Warn, "input_gap", v.id, "a held control went quiet", map[string]any{"gap_ms": gap.Milliseconds()})
		}
	}
	t.lastInput = now
	t.held = p.Pad.Buttons != 0 || p.Pad.Axes != (input.Axes{})
}

// noteRTCP reads what a participant's browser reports about the video:
// receiver reports (loss, jitter), retransmission and keyframe requests.
func (s *StreamService) noteRTCP(v *viewer, pkts []rtcp.Packet) {
	for _, p := range pkts {
		switch r := p.(type) {
		case *rtcp.ReceiverReport:
			for _, rep := range r.Reports {
				// The browser bundles its reports on every stream (video,
				// game sound, voices): keep the video's.
				if ssrc := v.videoSSRC.Load(); ssrc != 0 && rep.SSRC != ssrc {
					continue
				}
				perMille := int64(rep.FractionLost) * 1000 / 256
				v.tm.rrLossPerMi.Store(perMille)
				v.tm.rrJitterUs.Store(int64(rep.Jitter) * 1000 / 90) // 90 kHz video clock
				v.tm.rrLost.Store(int64(rep.TotalLost))
				now := time.Now().UnixMilli()
				if perMille >= 50 && now-v.tm.lossWarned.Load() > 5000 {
					v.tm.lossWarned.Store(now)
					s.teleEvent(telemetry.Warn, "video_loss", v.id, "the participant lost video packets", map[string]any{"pct": float64(perMille) / 10, "total": rep.TotalLost})
				}
			}
		case *rtcp.TransportLayerNack:
			v.tm.nack.Add(int64(len(r.Nacks)))
		case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
			v.tm.pli.Add(1)
		}
	}
}

// clientMetrics are the numbers a browser may send in client_report, with
// the highest value each may take.
var clientMetrics = map[string]float64{
	"rtt_ms": 60000, "jitter_ms": 60000, "buffer_ms": 60000, "decode_ms": 10000,
	"video_loss_pct": 100, "audio_loss_pct": 100, "voice_loss_pct": 100,
	"fps": 1000, "dropped": 100000, "freeze_ms": 60000, "freezes": 1000,
	"e2e_ms": 60000, "e2e_last_ms": 60000, "kbps": 1e6, "hidden": 1, "input_hz": 1000,
}

// noteClientReport stores what a participant's browser measured over its
// last two seconds, logs its freezes and moves its playout delay
// (playout_delay.go). A hidden tab's freezes are left out: the browser
// stops drawing it on purpose.
func (s *StreamService) noteClientReport(v *viewer, data []byte) {
	var raw map[string]any
	if json.Unmarshal(data, &raw) != nil {
		return
	}
	m := telemetry.Metrics{}
	for k, top := range clientMetrics {
		if f, ok := raw[k].(float64); ok && !math.IsNaN(f) && f >= 0 && f <= top {
			m[k] = f
		}
	}
	if m["hidden"] == 1 {
		delete(m, "freeze_ms")
		delete(m, "freezes")
	}
	s.movePlayout(v, m)
	s.moveQuality(v, m)
	rec := s.tele.Load()
	if rec == nil {
		return
	}
	rec.Sample(v.id, "client", m)
	if f := m["freeze_ms"]; f >= float64(frameGapEvent.Milliseconds()) {
		rec.Event(telemetry.Warn, "client_freeze", v.id, "the participant's picture froze", map[string]any{"freeze_ms": f, "video_loss_pct": m["video_loss_pct"], "rtt_ms": m["rtt_ms"]})
	}
	hidden := m["hidden"] == 1
	v.mu.Lock()
	changed := v.hidden != hidden
	v.hidden = hidden
	v.mu.Unlock()
	if changed {
		msg := "the participant's tab is visible again"
		if hidden {
			msg = "the participant's tab is hidden (the browser may slow it down)"
		}
		rec.Event(telemetry.Info, "client_hidden", v.id, msg, map[string]any{"hidden": hidden})
	}
}

// teleLoop samples the room and its participants every second while the
// stream runs.
func (s *StreamService) teleLoop(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	last := time.Now()
	tick := 0
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			tick++
			elapsed := now.Sub(last).Seconds()
			last = now
			rec := s.tele.Load()
			if rec == nil || elapsed <= 0 {
				continue
			}
			s.sampleTelemetry(rec, elapsed, tick%2 == 0)
		}
	}
}

func (s *StreamService) sampleTelemetry(rec *telemetry.Recorder, elapsed float64, withICE bool) {
	f := &s.ftele
	f.mu.Lock()
	room := telemetry.Metrics{
		"fps_in":     float64(f.in) / elapsed,
		"gap_max_ms": float64(f.gapMax.Microseconds()) / 1000,
		"enc_late":   float64(f.late),
	}
	if f.encN > 0 {
		room["enc_avg_ms"] = float64(f.encSum.Microseconds()) / 1000 / float64(f.encN)
		room["enc_max_ms"] = float64(f.encMax.Microseconds()) / 1000
	}
	f.in, f.gapMax, f.encN, f.encSum, f.encMax, f.late = 0, 0, 0, 0, 0, 0
	f.mu.Unlock()

	s.mu.Lock()
	room["fps_sent"] = s.sentFPS
	room["kbps"] = s.videoKbps
	host := s.teleHost
	viewers := make([]*viewer, 0, len(s.viewers))
	for _, v := range s.viewers {
		if v.kind == KindViewer {
			viewers = append(viewers, v)
		}
	}
	s.mu.Unlock()
	room["viewers"] = float64(len(viewers))
	room["enc_load_pct"] = s.EncodeLoad() * 100
	if host != nil {
		for k, v := range host() {
			room[k] = v
		}
	}
	if s.hud.Load() > 0 {
		room["hud"] = 1
	}
	rec.Sample("", "room", room)

	for _, v := range viewers {
		t := &v.tm
		m := telemetry.Metrics{
			"input_pps":        float64(t.inputPkts.Swap(0)) / elapsed,
			"input_lost":       float64(t.inputLost.Swap(0)),
			"input_gap_max_ms": float64(time.Duration(t.inputGapMax.Swap(0)).Microseconds()) / 1000,
			"pli":              float64(t.pli.Swap(0)),
			"nack":             float64(t.nack.Swap(0)),
			"rr_loss_pct":      float64(t.rrLossPerMi.Load()) / 10,
			"rr_jitter_ms":     float64(t.rrJitterUs.Load()) / 1000,
			"rr_lost":          float64(t.rrLost.Load()),
			"voice_in_pps":     float64(t.voiceIn.Swap(0)) / elapsed,
		}
		if pi := playoutDelays.forPC(v.pc); pi != nil {
			m["playout_max_ms"] = float64(pi.Max())
		}
		if s.tiers != nil {
			s.mu.Lock()
			l := s.tierOfLocked(v)
			m["video_kbps_target"] = float64(stepKbps(s.Bitrate(), l, max(s.tierLevels, 1)))
			s.mu.Unlock()
		}
		if rtt := t.ctlRttMs.Load(); rtt >= 0 {
			m["ctl_rtt_ms"] = float64(rtt)
		}
		if withICE {
			if rtt, ok := iceRTT(v.pc); ok {
				m["ice_rtt_ms"] = rtt
			}
		}
		rec.Sample(v.id, "peer", m)
	}
}

// iceRTT is the round trip of the pair in use, from Pion's ICE checks.
func iceRTT(pc *webrtc.PeerConnection) (float64, bool) {
	if pc == nil {
		return 0, false
	}
	for _, st := range pc.GetStats() {
		if p, ok := st.(webrtc.ICECandidatePairStats); ok && p.Nominated && p.State == webrtc.StatsICECandidatePairStateSucceeded && p.CurrentRoundTripTime > 0 {
			return math.Round(p.CurrentRoundTripTime*10000) / 10, true
		}
	}
	return 0, false
}

// movePlayout climbs or lowers a viewer's playout delay from one client
// report, and logs each change with its reason.
func (s *StreamService) movePlayout(v *viewer, m telemetry.Metrics) {
	pi := playoutDelays.forPC(v.pc)
	if pi == nil {
		return
	}
	v.mu.Lock()
	from := v.playout.maxMs()
	changed, why := v.playout.note(m)
	to := v.playout.maxMs()
	v.mu.Unlock()
	if !changed {
		return
	}
	pi.SetMax(to)
	s.teleEvent(telemetry.Info, "playout_delay", v.id, "the participant's video wait changed after "+why, map[string]any{"from_ms": from, "to_ms": to})
}
