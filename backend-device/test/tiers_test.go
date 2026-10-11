// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/lordbasex/go-link-console/backend-device/internal/services"
)

// Two guests of a tiered stream (go-link HD) show the video at different
// sizes: each gets the picture that fills it, told in its stream_stats, and
// both keep receiving video.
func TestTiersGiveEachViewerItsSize(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	stream, err := services.NewStreamService(services.StreamConfig{Width: 1280, Height: 720, FPS: 30, IncludeLoopback: true, Logger: logger, Tiers: true}, services.NewICEStore())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = stream.Run(ctx) }()

	big := newBrowser(t, "big", stream)
	small := newBrowser(t, "small", stream)
	stream.SetSender(multiRelay{"big": big, "small": small})

	type stats struct {
		Type  string `json:"type"`
		Width int    `json:"width"`
	}
	var backwards atomic.Int64 // RTP sequence numbers that went back (a switch must not restart them)
	controls := map[string]*webrtc.DataChannel{}
	var cmu sync.Mutex
	watch := func(b *browserPeer, wantW, wantH int, width *atomic.Int64, packets *atomic.Int64) {
		b.pc.OnTrack(func(tr *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
			if tr.Kind() != webrtc.RTPCodecTypeVideo {
				return
			}
			go func() {
				var last uint16
				for n := 0; ; n++ {
					p, _, err := tr.ReadRTP()
					if err != nil {
						return
					}
					if n > 0 && int16(p.SequenceNumber-last) < 0 {
						backwards.Add(1)
					}
					last = p.SequenceNumber
					packets.Add(1)
				}
			}()
		})
		b.pc.OnDataChannel(func(dc *webrtc.DataChannel) {
			if dc.Label() != "control" {
				return
			}
			cmu.Lock()
			controls[b.id] = dc
			cmu.Unlock()
			dc.OnOpen(func() {
				msg, _ := json.Marshal(map[string]any{"type": "video_want", "width": wantW, "height": wantH})
				_ = dc.SendText(string(msg))
			})
			dc.OnMessage(func(m webrtc.DataChannelMessage) {
				var s stats
				if json.Unmarshal(m.Data, &s) == nil && s.Type == "stream_stats" && s.Width > 0 {
					width.Store(int64(s.Width))
				}
			})
		})
	}
	var bigW, smallW, bigRTP, smallRTP atomic.Int64
	watch(big, 1280, 720, &bigW, &bigRTP)
	watch(small, 600, 340, &smallW, &smallRTP)
	if err := stream.AddViewer("big"); err != nil {
		t.Fatal(err)
	}
	if err := stream.AddViewer("small"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) && (bigW.Load() != 1280 || smallW.Load() != 640 || bigRTP.Load() < 50 || smallRTP.Load() < 50) {
		time.Sleep(50 * time.Millisecond)
	}
	if bigW.Load() != 1280 || smallW.Load() != 640 {
		t.Fatalf("widths: big %d, small %d; want 1280 and 640", bigW.Load(), smallW.Load())
	}
	if bigRTP.Load() < 50 || smallRTP.Load() < 50 {
		t.Fatalf("video packets: big %d, small %d", bigRTP.Load(), smallRTP.Load())
	}
	// the small guest goes full screen: it moves up a tier and its video goes on
	cmu.Lock()
	dc := controls["small"]
	cmu.Unlock()
	msg, _ := json.Marshal(map[string]any{"type": "video_want", "width": 1280, "height": 720})
	if err := dc.SendText(string(msg)); err != nil {
		t.Fatal(err)
	}
	before := smallRTP.Load()
	deadline = time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) && (smallW.Load() != 1280 || smallRTP.Load() < before+50) {
		time.Sleep(50 * time.Millisecond)
	}
	if smallW.Load() != 1280 || smallRTP.Load() < before+50 {
		t.Fatalf("after the switch: width %d, %d new packets", smallW.Load(), smallRTP.Load()-before)
	}
	if n := backwards.Load(); n > 0 {
		t.Fatalf("%d RTP sequence numbers went back", n)
	}
}
