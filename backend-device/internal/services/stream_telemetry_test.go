// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/internal/telemetry"
	"github.com/lordbasex/go-link-console/backend-device/pkg/input"
	"github.com/pion/rtcp"
)

func teleStream(t *testing.T) (*StreamService, *telemetry.Store) {
	t.Helper()
	s, err := NewStreamService(StreamConfig{IncludeLoopback: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}, NewICEStore())
	if err != nil {
		t.Fatal(err)
	}
	store, err := telemetry.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	s.SetTelemetry(store.Room("room"), func() telemetry.Metrics { return telemetry.Metrics{"cpu_pct": 12} })
	return s, store
}

func TestTheStreamLogsAPauseOfTheGamesFramesAndSamplesTheRoom(t *testing.T) {
	s, store := teleStream(t)
	now := time.Now()
	s.noteFrame(now)
	s.noteFrame(now.Add(16 * time.Millisecond))
	s.noteFrame(now.Add(2 * time.Second)) // the game stopped for ~2 s
	s.noteEncode(4*time.Millisecond, 16*time.Millisecond)
	s.noteEncode(30*time.Millisecond, 16*time.Millisecond) // late
	s.sampleTelemetry(s.Telemetry(), 1, false)
	store.Sync()

	events, _ := store.Events("room", now.Add(-time.Minute), time.Now().Add(time.Minute), 0, telemetry.Warn)
	if len(events) != 1 || events[0].Kind != "frame_gap" {
		t.Fatalf("events %+v", events)
	}
	series, _ := store.Series("room", now.Add(-time.Minute), time.Now().Add(time.Minute), time.Hour, []string{"room.gap_max_ms", "room.enc_late", "room.cpu_pct", "room.fps_in"})
	got := map[string]float64{}
	for _, sr := range series {
		got[sr.Metric] = float64(sr.Max[0])
	}
	if got["gap_max_ms"] < 1900 || got["enc_late"] != 1 || got["cpu_pct"] != 12 || got["fps_in"] != 3 {
		t.Fatalf("room sample %v", got)
	}
}

func TestAViewersInputLossAndReportsAreCounted(t *testing.T) {
	s, store := teleStream(t)
	v := &viewer{id: "peerA", kind: KindViewer}
	v.tm.ctlRttMs.Store(-1)
	v.videoSSRC.Store(1234)
	s.mu.Lock()
	s.viewers[v.id] = v
	s.mu.Unlock()

	now := time.Now()
	held := input.Pad{Buttons: input.State(input.Button1)}
	send := func(seq uint16, at time.Time) {
		v.mu.Lock()
		s.noteInput(v, input.Encode(input.Packet{Seq: seq, Pad: held}), at)
		v.mu.Unlock()
	}
	send(1, now)
	send(2, now.Add(100*time.Millisecond))
	send(5, now.Add(700*time.Millisecond)) // two lost, and 600 ms of silence while held
	s.noteRTCP(v, []rtcp.Packet{
		// The browser bundles the reports on every stream: only the
		// video's counts (another stream's 94 % loss after it is ignored).
		&rtcp.ReceiverReport{Reports: []rtcp.ReceptionReport{{SSRC: 1234, FractionLost: 26, TotalLost: 40, Jitter: 900}, {SSRC: 999, FractionLost: 240, TotalLost: 5000, Jitter: 9000}}},
		&rtcp.PictureLossIndication{},
	})
	s.noteClientReport(v, []byte(`{"type":"client_report","rtt_ms":31,"freeze_ms":900,"video_loss_pct":4,"bogus":5,"fps":-3}`))
	// A hidden tab's freezes are not counted: the browser stops drawing it.
	s.noteClientReport(v, []byte(`{"type":"client_report","rtt_ms":31,"freeze_ms":5000,"freezes":1,"hidden":1}`))
	s.sampleTelemetry(s.Telemetry(), 1, false)
	store.Sync()

	series, _ := store.Series("room", now.Add(-time.Minute), time.Now().Add(time.Minute), time.Hour, nil)
	got := map[string]float64{}
	for _, sr := range series {
		got[sr.Kind+"."+sr.Metric] = float64(sr.Max[0])
	}
	if got["peer.input_lost"] != 2 || got["peer.input_gap_max_ms"] < 590 || got["peer.pli"] != 1 || got["peer.rr_loss_pct"] != 10.1 || got["peer.rr_jitter_ms"] != 10 {
		t.Fatalf("peer sample %v", got)
	}
	if got["client.rtt_ms"] != 31 || got["client.freeze_ms"] != 900 || got["client.bogus"] != 0 || got["client.fps"] != 0 {
		t.Fatalf("client sample %v", got)
	}
	events, _ := store.Events("room", now.Add(-time.Minute), time.Now().Add(time.Minute), 0, telemetry.Info)
	kinds := map[string]bool{}
	for _, e := range events {
		kinds[e.Kind] = true
	}
	for _, k := range []string{"input_gap", "video_loss", "client_freeze", "client_hidden"} {
		if !kinds[k] {
			t.Fatalf("no %s event: %+v", k, events)
		}
	}
}
