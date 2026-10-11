// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
	"github.com/lordbasex/go-link-console/backend-device/pkg/signalclient"
)

const deviceID = "7f3c2a10-1b2c-4d3e-8f90-a1b2c3d4e5f6"

type fakeSender struct {
	mu   sync.Mutex
	sent []signalclient.Envelope
	ch   chan signalclient.Envelope
	err  error
}

func newFakeSender() *fakeSender { return &fakeSender{ch: make(chan signalclient.Envelope, 10)} }

func (f *fakeSender) Send(env signalclient.Envelope) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, env)
	f.ch <- env
	return nil
}

func expectRegister(t *testing.T, f *fakeSender) {
	t.Helper()
	select {
	case env := <-f.ch:
		if env.Type != signalclient.TypeRegister || env.App != models.App || env.DeviceID != deviceID {
			t.Fatalf("unexpected message %+v", env)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no register sent")
	}
}

func newPairing(refresh time.Duration) (*PairingService, *StatusService, *fakeSender) {
	st := NewStatusService(deviceID, "test", "ws://x", "")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	p := NewPairingService(PairingConfig{DeviceID: deviceID, RefreshEvery: refresh, Logger: logger}, st, NewICEStore())
	s := newFakeSender()
	p.SetSender(s)
	return p, st, s
}

func TestPairingLifecycle(t *testing.T) {
	p, st, s := newPairing(time.Hour)

	p.OnConnect(signalclient.Envelope{Type: signalclient.TypeHello, PeerID: "A"})
	expectRegister(t, s)
	if got := st.Snapshot(); got.Signal.State != models.SignalConnected || got.Signal.PeerID != "A" {
		t.Fatalf("after connect: %+v", got.Signal)
	}

	p.OnMessage(signalclient.Envelope{Type: signalclient.TypeCode, Code: "113 134 323"})
	if got := st.Snapshot(); got.Pairing.Code != "113 134 323" || got.Pairing.RefreshesAt == nil {
		t.Fatalf("code: %+v", got.Pairing)
	}

	// A browser redeems the code: it is recorded and a new code is requested.
	p.OnMessage(signalclient.Envelope{Type: signalclient.TypePaired, SessionID: "S", Remote: "B"})
	expectRegister(t, s)
	if got := st.Snapshot(); len(got.Peers) != 1 || got.Peers[0].PeerID != "B" || got.Signal.SessionID != "S" {
		t.Fatalf("paired: %+v", got)
	}

	p.OnMessage(signalclient.Envelope{Type: signalclient.TypeError, Error: "app not allowed"})
	if got := st.Snapshot(); got.Signal.Error != "app not allowed" {
		t.Fatalf("error not shown: %+v", got.Signal)
	}

	p.OnMessage(signalclient.Envelope{Type: signalclient.TypePeerLeft, From: "B"})
	if got := st.Snapshot(); len(got.Peers) != 0 {
		t.Fatalf("peer not removed: %+v", got.Peers)
	}

	p.OnDisconnect(errors.New("boom"))
	got := st.Snapshot()
	if got.Signal.State != models.SignalDisconnected || got.Pairing.Code != "" || got.Signal.Error != "boom" {
		t.Fatalf("after disconnect: %+v", got)
	}
}

func TestCodeIsRefreshedBeforeExpiry(t *testing.T) {
	p, _, s := newPairing(30 * time.Millisecond)
	p.OnConnect(signalclient.Envelope{PeerID: "A"})
	expectRegister(t, s)
	p.OnMessage(signalclient.Envelope{Type: signalclient.TypeCode, Code: "111 111 111"})
	expectRegister(t, s) // fired by the refresh timer
}

func TestDisconnectStopsRefresh(t *testing.T) {
	p, _, s := newPairing(30 * time.Millisecond)
	p.OnConnect(signalclient.Envelope{PeerID: "A"})
	expectRegister(t, s)
	p.OnMessage(signalclient.Envelope{Type: signalclient.TypeCode, Code: "111 111 111"})
	p.OnDisconnect(nil)
	select {
	case env := <-s.ch:
		t.Fatalf("register sent after disconnect: %+v", env)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestStatusListenersAndCopies(t *testing.T) {
	st := NewStatusService(deviceID, "test", "ws://x", "")
	var got []models.Status
	st.OnChange(func(s models.Status) { got = append(got, s) })
	st.SetConnected("A", nil)
	st.AddPeer("B", "S", time.Now())
	if len(got) != 2 || got[1].Peers[0].PeerID != "B" {
		t.Fatalf("listener calls: %+v", got)
	}
	snap := st.Snapshot()
	snap.Peers[0].PeerID = "mutated"
	if st.Snapshot().Peers[0].PeerID != "B" {
		t.Fatal("snapshot shares memory with the service")
	}
}

func TestICEServersKeptInMemoryOnly(t *testing.T) {
	ice := NewICEStore()
	st := NewStatusService(deviceID, "test", "ws://x", "")
	p := NewPairingService(PairingConfig{DeviceID: deviceID, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}, st, ice)
	p.SetSender(newFakeSender())
	p.OnConnect(signalclient.Envelope{PeerID: "A", ICEServers: []signalclient.ICEServer{
		{URLs: []string{"stun:stun.example:3478"}},
		{URLs: []string{"turn:turn.example:3478"}, Username: "1:A", Credential: "secret"},
	}})
	if got := ice.Get(); len(got) != 2 || got[1].Credential != "secret" {
		t.Fatalf("store: %+v", got)
	}
	urls := st.Snapshot().Signal.ICEURLs
	if len(urls) != 2 || urls[1] != "turn:turn.example:3478" {
		t.Fatalf("status urls: %v", urls)
	}
	got := ice.Get()
	got[0].URLs[0] = "mutated"
	if ice.Get()[0].URLs[0] != "stun:stun.example:3478" {
		t.Fatal("store shares memory with callers")
	}
	p.OnDisconnect(nil)
	if len(ice.Get()) != 0 || len(st.Snapshot().Signal.ICEURLs) != 0 {
		t.Fatal("ICE servers survived the disconnect")
	}
}
