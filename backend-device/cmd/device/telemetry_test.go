// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/internal/telemetry"
)

func TestATelemetryAnswerAlwaysFitsOneMessage(t *testing.T) {
	store, err := telemetry.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	r := store.Room("room")
	r.Start("abc123", "Game")
	for i := 0; i < 3000; i++ {
		for p := 0; p < 4; p++ {
			r.Sample(fmt.Sprintf("peer%d", p), "client", telemetry.Metrics{"rtt_ms": float64(i % 90), "video_loss_pct": 1.5, "freeze_ms": 0, "fps": 59.9})
		}
		r.Event(telemetry.Info, "peer_state", "peer1", "connection connected with a long enough message to fill the page", map[string]any{"i": i})
	}
	store.Sync()

	ask := func(q string) map[string]any {
		b := answerTelemetry(store, []byte(q))
		if len(b) == 0 || len(b) > maxAnswer {
			t.Fatalf("%s: %d bytes", q, len(b))
		}
		var res map[string]any
		if err := json.Unmarshal(b, &res); err != nil {
			t.Fatal(err)
		}
		if res["error"] != nil {
			t.Fatalf("%s: %v", q, res["error"])
		}
		return res
	}
	series := ask(`{"type":"telemetry_series","req":1,"id":"room"}`)
	if len(series["series"].([]any)) != 16 {
		t.Fatalf("%d series", len(series["series"].([]any)))
	}
	events := ask(`{"type":"telemetry_events","req":2,"id":"room","limit":300}`)
	if events["more"] != true || len(events["events"].([]any)) == 0 {
		t.Fatalf("events page: more %v, %d events", events["more"], len(events["events"].([]any)))
	}
	if run := ask(`{"type":"telemetry_find","req":3,"run":"#abc"}`); run["id"] != "room" {
		t.Fatalf("find %v", run)
	}
}

func TestTelemetryTargetSince(t *testing.T) {
	store, err := telemetry.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.Room("room").Start("abc123", "Game")
	store.Sync()
	for _, arg := range []string{"room", "#abc"} {
		_, from, to, err := telemetryTarget(store, arg, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if d := to.Sub(from); d > time.Minute {
			t.Errorf("%s --since 1m covers %v", arg, d)
		}
	}
}
