// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"

	"github.com/lordbasex/go-link-console/backend-device/internal/services"
	"github.com/lordbasex/go-link-console/backend-device/pkg/encoder"
	"github.com/lordbasex/go-link-console/backend-device/pkg/input"
	"github.com/lordbasex/go-link-console/backend-device/pkg/signalclient"
	"github.com/lordbasex/go-link-console/backend-device/pkg/testpattern"
)

// browserPeer plays the web's role with Pion: it answers the device's
// offer and relays candidates back.
type browserPeer struct {
	id           string
	beforeAnswer func() // e.g. attach a microphone
	t            *testing.T
	pc           *webrtc.PeerConnection
	stream       *services.StreamService
	mu           sync.Mutex
	queue        []webrtc.ICECandidateInit
	remote       bool
	frames       chan struct{}
	inputs       chan *webrtc.DataChannel
}

// relay is the Sender given to the stream: it delivers straight to the
// browser, standing in for signalhub.
type relay struct{ b *browserPeer }

func (r relay) Send(env signalclient.Envelope) error {
	go r.b.onSignal(env.Payload)
	return nil
}

func (b *browserPeer) onSignal(payload json.RawMessage) {
	var sig services.RTCSignal
	if err := json.Unmarshal(payload, &sig); err != nil {
		b.t.Error(err)
		return
	}
	switch sig.Kind {
	case "offer":
		if err := b.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: sig.SDP}); err != nil {
			b.t.Error(err)
			return
		}
		if b.beforeAnswer != nil {
			b.beforeAnswer()
		}
		answer, err := b.pc.CreateAnswer(nil)
		if err != nil {
			b.t.Error(err)
			return
		}
		_ = b.pc.SetLocalDescription(answer)
		out, _ := json.Marshal(services.RTCSignal{Kind: "answer", SDP: answer.SDP})
		if err := b.stream.HandleSignal(b.id, out); err != nil {
			b.t.Error(err)
		}
		b.mu.Lock()
		b.remote = true
		queued := b.queue
		b.queue = nil
		b.mu.Unlock()
		for _, c := range queued {
			_ = b.pc.AddICECandidate(c)
		}
	case "candidate":
		b.mu.Lock()
		if !b.remote {
			b.queue = append(b.queue, *sig.Candidate)
			b.mu.Unlock()
			return
		}
		b.mu.Unlock()
		_ = b.pc.AddICECandidate(*sig.Candidate)
	}
}

func TestStreamToBrowser(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	stream, err := services.NewStreamService(services.StreamConfig{Width: 160, Height: 120, FPS: 30, BitrateKbps: 300, IncludeLoopback: true, Logger: logger}, services.NewICEStore())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = stream.Run(ctx) }()

	se := webrtc.SettingEngine{}
	se.SetIncludeLoopbackCandidate(true)
	pc, err := webrtc.NewAPI(webrtc.WithSettingEngine(se)).NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	b := &browserPeer{id: "browser", t: t, pc: pc, stream: stream, frames: make(chan struct{}, 100), inputs: make(chan *webrtc.DataChannel, 1)}

	audio := make(chan struct{}, 100)
	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		if track.Kind() == webrtc.RTPCodecTypeAudio {
			if track.Codec().MimeType != webrtc.MimeTypeOpus {
				t.Errorf("audio codec %s", track.Codec().MimeType)
			}
			for {
				if _, _, err := track.ReadRTP(); err != nil {
					return
				}
				select {
				case audio <- struct{}{}:
				default:
				}
			}
		}
		if track.Codec().MimeType != webrtc.MimeTypeVP8 {
			t.Errorf("codec %s", track.Codec().MimeType)
		}
		for {
			if _, _, err := track.ReadRTP(); err != nil {
				return
			}
			select {
			case b.frames <- struct{}{}:
			default:
			}
		}
	})
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		if dc.Label() == "input" {
			if dc.Ordered() || dc.MaxRetransmits() == nil || *dc.MaxRetransmits() != 0 {
				t.Errorf("input channel must be unordered without retransmits")
			}
			dc.OnOpen(func() { b.inputs <- dc })
		}
	})
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		init := c.ToJSON()
		out, _ := json.Marshal(services.RTCSignal{Kind: "candidate", Candidate: &init})
		_ = stream.HandleSignal("browser", out)
	})

	stream.SetSender(relay{b})
	if err := stream.AddViewer("browser"); err != nil {
		t.Fatal(err)
	}

	// Video: RTP packets must keep arriving.
	deadline := time.After(10 * time.Second)
	for got := 0; got < 20; got++ {
		select {
		case <-b.frames:
		case <-deadline:
			t.Fatalf("only %d RTP packets received", got)
		}
	}

	// Audio: the 1 kHz tone arrives as Opus.
	for got := 0; got < 10; got++ {
		select {
		case <-audio:
		case <-deadline:
			t.Fatalf("only %d audio packets received", got)
		}
	}

	// Input: the device must see the buttons.
	var dc *webrtc.DataChannel
	select {
	case dc = <-b.inputs:
	case <-time.After(5 * time.Second):
		t.Fatal("input channel never opened")
	}
	if err := dc.Send(input.Encode(input.Packet{Seq: 1, Player: 1, Pad: input.Pad{Buttons: input.State(input.Up | input.Button1)}})); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return stream.Pressed("browser") == input.State(input.Up|input.Button1) })

	stream.RemoveViewer("browser")
	if stream.ViewerCount() != 0 {
		t.Fatal("viewer not removed")
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// newBrowser builds a Pion peer that answers offers for id.
func newBrowser(t *testing.T, id string, stream *services.StreamService) *browserPeer {
	se := webrtc.SettingEngine{}
	se.SetIncludeLoopbackCandidate(true)
	pc, err := webrtc.NewAPI(webrtc.WithSettingEngine(se)).NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	b := &browserPeer{id: id, t: t, pc: pc, stream: stream, frames: make(chan struct{}, 100), inputs: make(chan *webrtc.DataChannel, 1)}
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		init := c.ToJSON()
		out, _ := json.Marshal(services.RTCSignal{Kind: "candidate", Candidate: &init})
		_ = stream.HandleSignal(id, out)
	})
	return b
}

// ctrlMsg is a control channel message as the browser sees it.
type ctrlMsg struct {
	Type string `json:"type"`
	ID   uint64 `json:"id"`
}

// multiRelay routes the stream's signals to the right fake browser.
type multiRelay map[string]*browserPeer

func (m multiRelay) Send(env signalclient.Envelope) error {
	if b := m[env.To]; b != nil {
		go b.onSignal(env.Payload)
	}
	return nil
}

func TestLinkGetsStatusAndLatency(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	stream, err := services.NewStreamService(services.StreamConfig{Width: 160, Height: 120, IncludeLoopback: true, Logger: logger}, services.NewICEStore())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = stream.Run(ctx) }()

	latency := make(chan int, 10)
	stream.OnLatency(func(peerID string, ms int) {
		if peerID == "link" {
			latency <- ms
		}
	})

	link := newBrowser(t, "link", stream)
	viewer := newBrowser(t, "viewer", stream)
	stream.SetSender(multiRelay{"link": link, "viewer": viewer})

	linkMsgs := make(chan ctrlMsg, 20)
	viewerMsgs := make(chan ctrlMsg, 20)
	answerPings := func(pc *webrtc.PeerConnection, out chan ctrlMsg, sawVideo *atomic.Bool) {
		pc.OnTrack(func(*webrtc.TrackRemote, *webrtc.RTPReceiver) { sawVideo.Store(true) })
		pc.OnDataChannel(func(dc *webrtc.DataChannel) {
			if dc.Label() != "control" {
				return
			}
			dc.OnMessage(func(m webrtc.DataChannelMessage) {
				var got ctrlMsg
				_ = json.Unmarshal(m.Data, &got)
				if got.Type == "ping" {
					b, _ := json.Marshal(ctrlMsg{Type: "pong", ID: got.ID})
					_ = dc.SendText(string(b))
				}
				out <- got
			})
		})
	}
	var linkVideo, viewerVideo atomic.Bool
	answerPings(link.pc, linkMsgs, &linkVideo)
	answerPings(viewer.pc, viewerMsgs, &viewerVideo)

	if err := stream.AddLink("link"); err != nil {
		t.Fatal(err)
	}
	if err := stream.AddViewer("viewer"); err != nil {
		t.Fatal(err)
	}

	select {
	case ms := <-latency:
		if ms < 0 || ms > 2000 {
			t.Fatalf("latency %d ms", ms)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no latency measured")
	}

	stream.SendToLinks(map[string]string{"type": "device_status"})
	waitForMsg(t, linkMsgs, "device_status")
	// Give the viewer the same chance, then check it got none.
	time.Sleep(300 * time.Millisecond)
	for len(viewerMsgs) > 0 {
		if m := <-viewerMsgs; m.Type == "device_status" {
			t.Fatal("a room viewer received the device status")
		}
	}
	if linkVideo.Load() {
		t.Fatal("a linked browser received video")
	}
}

func waitForMsg(t *testing.T, ch chan ctrlMsg, typ string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case m := <-ch:
			if m.Type == typ {
				return
			}
		case <-deadline:
			t.Fatalf("no %s message", typ)
		}
	}
}

func TestVoiceGoesOnlyToOtherPlayers(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	stream, err := services.NewStreamService(services.StreamConfig{Width: 160, Height: 120, IncludeLoopback: true, Logger: logger}, services.NewICEStore())
	if err != nil {
		t.Fatal(err)
	}
	seats := map[string]int{"alice": 1, "bob": 2} // carol only watches
	stream.SetRoomHooks(services.RoomHooks{PortOf: func(peer string, local uint8) (int, bool) {
		port, ok := seats[peer]
		return port, ok && local == 0
	}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = stream.Run(ctx) }()

	alice := newBrowser(t, "alice", stream)
	bob := newBrowser(t, "bob", stream)
	carol := newBrowser(t, "carol", stream)
	stream.SetSender(multiRelay{"alice": alice, "bob": bob, "carol": carol})

	// Count voice packets per receiver and stream id.
	var mu sync.Mutex
	got := map[string]int{}
	count := func(b *browserPeer) {
		b.pc.OnTrack(func(tr *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
			for {
				if _, _, err := tr.ReadRTP(); err != nil {
					return
				}
				if strings.HasPrefix(tr.StreamID(), "voice-") {
					mu.Lock()
					got[b.id+"/"+tr.StreamID()]++
					mu.Unlock()
				}
			}
		})
	}
	count(alice)
	count(bob)
	count(carol)

	// Alice attaches a microphone before answering.
	mic, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2}, "mic", "mic")
	if err != nil {
		t.Fatal(err)
	}
	alice.beforeAnswer = func() {
		if _, err := alice.pc.AddTrack(mic); err != nil {
			t.Error(err)
		}
	}

	for _, id := range []string{"alice", "bob", "carol"} {
		if err := stream.AddViewer(id); err != nil {
			t.Fatal(err)
		}
	}

	enc, err := encoder.NewOpus(1, 32000)
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	tone := testpattern.NewTone(440)
	deadline := time.Now().Add(10 * time.Second)
	for {
		pkt, _ := enc.Encode(tone.Frame())
		_ = mic.WriteSample(media.Sample{Data: pkt, Duration: 20 * time.Millisecond})
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		heard := got["bob/voice-p1"]
		mu.Unlock()
		if heard >= 20 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("bob heard %d voice packets; all: %v", heard, got)
		}
	}
	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	for key, n := range got {
		if key != "bob/voice-p1" && n > 0 {
			t.Errorf("%s received %d voice packets", key, n)
		}
	}
}
