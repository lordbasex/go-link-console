// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
	"github.com/lordbasex/go-link-console/backend-device/pkg/emuproc"
)

// videoGame is a fakeGame that takes a video mode, like WorkerSource.
type videoGame struct {
	*fakeGame
	vmu   sync.Mutex
	mode  emuproc.VideoMode
	modes []emuproc.VideoMode
}

func (g *videoGame) SetVideoMode(m emuproc.VideoMode) {
	g.vmu.Lock()
	defer g.vmu.Unlock()
	g.mode = m
	g.modes = append(g.modes, m)
}

func (g *videoGame) VideoMode() emuproc.VideoMode {
	g.vmu.Lock()
	defer g.vmu.Unlock()
	return g.mode
}

func TestPlanFor(t *testing.T) {
	cases := map[string]VideoPlan{
		"high":   {Quality: "high", Mode: emuproc.VideoDouble, Kbps: 3500},
		"normal": {Quality: "normal", Mode: emuproc.VideoDouble, Kbps: 2500},
		"saver":  {Quality: "saver", Mode: emuproc.VideoBox, Kbps: 2500},
		"":       {Quality: "high", Mode: emuproc.VideoDouble, Kbps: 3500},
		"ultra":  {Quality: "high", Mode: emuproc.VideoDouble, Kbps: 3500},
	}
	for q, want := range cases {
		if got := PlanFor(q); got != want {
			t.Errorf("PlanFor(%q) = %+v, want %+v", q, got, want)
		}
	}
}

func TestEncodeProbeDecides(t *testing.T) {
	frame := time.Second / 60 // 16.7 ms: the limit is 10 ms
	run := func(took func(i int) time.Duration) (frames int, p95 time.Duration, slow bool) {
		p := newEncodeProbe(frame)
		now := time.Unix(0, 0)
		for i := 0; i < 1000; i++ {
			done, p95, slow := p.add(took(i), now)
			if done {
				return i + 1, p95, slow
			}
			now = now.Add(frame)
		}
		t.Fatal("the probe never finished")
		return 0, 0, false
	}
	// Fast: 3 ms per frame, with a slow first keyframe that is left out.
	n, p95, slow := run(func(i int) time.Duration {
		if i == 0 {
			return 40 * time.Millisecond
		}
		return 3 * time.Millisecond
	})
	if slow || p95 != 3*time.Millisecond {
		t.Fatalf("fast encoder: p95 %v slow %v", p95, slow)
	}
	// Warm-up plus two seconds of frames.
	if n < probeWarmup+120 || n > probeWarmup+123 {
		t.Fatalf("finished after %d frames", n)
	}
	// Slow: most frames take 12 ms.
	if _, p95, slow := run(func(i int) time.Duration { return 12 * time.Millisecond }); !slow || p95 != 12*time.Millisecond {
		t.Fatalf("slow encoder: p95 %v slow %v", p95, slow)
	}
	// A few spikes (under 5%) do not count; more than that do.
	if _, _, slow := run(func(i int) time.Duration {
		if i%50 == 0 {
			return 30 * time.Millisecond
		}
		return 4 * time.Millisecond
	}); slow {
		t.Fatal("2% spikes made it slow")
	}
	if _, _, slow := run(func(i int) time.Duration {
		if i%10 == 0 {
			return 30 * time.Millisecond
		}
		return 4 * time.Millisecond
	}); !slow {
		t.Fatal("10% slow frames passed")
	}
	if tooSlow(9*time.Millisecond, frame) || !tooSlow(11*time.Millisecond, frame) || tooSlow(time.Second, 0) {
		t.Fatal("tooSlow limits")
	}
}

func TestStreamStatsTellTheScale(t *testing.T) {
	s, err := NewStreamService(StreamConfig{IncludeLoopback: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}, NewICEStore())
	if err != nil {
		t.Fatal(err)
	}
	if s.Bitrate() != SaverKbps {
		t.Fatalf("default bitrate %d", s.Bitrate())
	}
	s.mu.Lock()
	if st := s.streamStatsLocked(); st.Video != nil {
		t.Fatalf("video before any frame: %+v", st.Video)
	}
	s.mu.Unlock()
	s.SetVideoScale(2)
	s.SetVideoInfo(models.VideoHigh, "")
	s.mu.Lock()
	s.vp8W, s.vp8H, s.aspect = 768, 448, 4.0/3
	b, _ := json.Marshal(s.streamStatsLocked())
	s.mu.Unlock()
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	v, _ := got["video"].(map[string]any)
	if got["type"] != "stream_stats" || got["width"] != 768.0 || v == nil || v["scale"] != 2.0 || v["width"] != 384.0 || v["height"] != 224.0 || v["quality"] != "high" {
		t.Fatalf("stream_stats %s", b)
	}
	if _, ok := v["fallback"]; ok {
		t.Fatalf("fallback without one: %s", b)
	}
	s.SetVideoScale(7) // anything else is the game's size
	if s.VideoScale() != 1 {
		t.Fatal("scale 7 kept")
	}
}

func TestStreamEncoderProbeFallsBack(t *testing.T) {
	s, err := NewStreamService(StreamConfig{IncludeLoopback: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}, NewICEStore())
	if err != nil {
		t.Fatal(err)
	}
	slow := make(chan time.Duration, 1)
	s.ArmEncodeProbe(func(p95, frame time.Duration) { slow <- p95 })
	frame := time.Second / 60
	// Frames at the game's size are never measured.
	for range 300 {
		s.measure(50*time.Millisecond, frame, 384, 224)
	}
	if s.probe != nil {
		t.Fatal("a 1x frame was measured")
	}
	s.SetVideoScale(2)
	for i := 0; s.probeArmed.Load(); i++ {
		if i > 10000 {
			t.Fatal("the probe never finished")
		}
		s.measure(15*time.Millisecond, frame, 768, 448)
		time.Sleep(20 * time.Microsecond)
		if i > probeWarmup+probeMinSamples {
			// Speed the window up: pretend two seconds went by.
			if s.probe != nil {
				s.probe.started = time.Now().Add(-probeWindow)
			}
		}
	}
	select {
	case p95 := <-slow:
		if p95 != 15*time.Millisecond {
			t.Fatalf("p95 %v", p95)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no fallback")
	}
	// Once decided, it measures no more.
	s.measure(15*time.Millisecond, frame, 768, 448)
	if s.probe != nil {
		t.Fatal("measured after the verdict")
	}
}

func TestRoomsVideoQuality(t *testing.T) {
	h := newRoomsHarness(t, 4, nil, func(h *roomsHarness) { h.video, h.quality = true, models.VideoNormal })
	h.create("robby", "room-1")
	id := h.rooms.List()[0].ID
	src := func() *videoGame {
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.sources[0].(*videoGame)
	}
	if v := h.room(id).Video; v == nil || *v != (models.RoomVideo{Quality: "normal", Scale: 2}) {
		t.Fatalf("video %+v", v)
	}
	h.rooms.mu.Lock()
	gr := h.rooms.rooms[0]
	stream := gr.stream
	h.rooms.mu.Unlock()
	if src().VideoMode() != emuproc.VideoDouble || stream.Bitrate() != NormalKbps || !stream.probeArmed.Load() {
		t.Fatalf("mode %v bitrate %d probe %v", src().VideoMode(), stream.Bitrate(), stream.probeArmed.Load())
	}

	// Live: a new quality reaches the running room.
	h.rooms.SetVideoQuality(models.VideoSaver)
	if src().VideoMode() != emuproc.VideoBox || stream.Bitrate() != SaverKbps || stream.probeArmed.Load() {
		t.Fatalf("saver: mode %v bitrate %d probe %v", src().VideoMode(), stream.Bitrate(), stream.probeArmed.Load())
	}
	if v := h.room(id).Video; v.Quality != "saver" || v.Scale != 1 || v.Fallback != "" {
		t.Fatalf("saver video %+v", v)
	}
	h.rooms.SetVideoQuality(models.VideoHigh)
	if src().VideoMode() != emuproc.VideoDouble || stream.Bitrate() != HighKbps || h.rooms.VideoQuality() != "high" {
		t.Fatalf("high: mode %v bitrate %d", src().VideoMode(), stream.Bitrate())
	}

	// The encoder check says 2x does not fit: the room goes to saver and
	// the owner sees why.
	h.rooms.encoderTooSlow(gr, stream, 14*time.Millisecond, time.Second/60)
	if v := h.room(id).Video; v == nil || *v != (models.RoomVideo{Quality: "saver", Fallback: "cpu", Scale: 1}) {
		t.Fatalf("fallback video %+v", v)
	}
	if src().VideoMode() != emuproc.VideoBox || stream.Bitrate() != SaverKbps {
		t.Fatalf("fallback: mode %v bitrate %d", src().VideoMode(), stream.Bitrate())
	}
	st := h.status.Snapshot()
	if len(st.Rooms) != 1 || st.Rooms[0].Video == nil || st.Rooms[0].Video.Fallback != "cpu" {
		t.Fatalf("device_status rooms %+v", st.Rooms)
	}
	// The host picks a quality again: the room tries 2x again.
	h.rooms.SetVideoQuality(models.VideoHigh)
	if v := h.room(id).Video; v.Quality != "high" || v.Fallback != "" || !stream.probeArmed.Load() {
		t.Fatalf("after a new choice %+v", v)
	}
	// A stopped room tells nothing.
	if _, err := h.rooms.Action(t.Context(), id, "archive", ""); err != nil {
		t.Fatal(err)
	}
	if v := h.room(id).Video; v != nil {
		t.Fatalf("archived room video %+v", v)
	}
}

func TestSettingsVideoQuality(t *testing.T) {
	s := NewSettingsService(nil, models.ThumbnailSettings{}, t.TempDir(), nil)
	if s.VideoQuality() != "high" {
		t.Fatalf("default %q", s.VideoQuality())
	}
	s.UseVideoQuality("bogus", nil)
	if s.VideoQuality() != "high" {
		t.Fatalf("a bad saved value %q", s.VideoQuality())
	}
	var saved, told string
	s.UseVideoQuality("normal", func(q string) error { saved = q; return nil })
	s.OnVideoQuality(func(q string) { told = q })
	if s.VideoQuality() != "normal" {
		t.Fatalf("saved value %q", s.VideoQuality())
	}
	if err := s.SetVideoQuality("4k"); err != ErrBadSetting {
		t.Fatalf("bad quality: %v", err)
	}
	if err := s.SetVideoQuality("saver"); err != nil || saved != "saver" || told != "saver" {
		t.Fatalf("set: %v saved %q told %q", err, saved, told)
	}
}
