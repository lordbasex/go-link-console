// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package test runs the device services against a fake signalhub and
// checks the result through the real panel HTTP API.
package test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
	"github.com/lordbasex/go-link-console/backend-device/internal/services"
	"github.com/lordbasex/go-link-console/backend-device/pkg/signalclient"
)

const (
	deviceID     = "7f3c2a10-1b2c-4d3e-8f90-a1b2c3d4e5f6"
	deviceSecret = "CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC"
)

// fakeHub mimics the parts of signalhub the device uses.
type fakeHub struct {
	*httptest.Server
	mu        sync.Mutex
	conn      *websocket.Conn
	registers chan signalclient.Envelope
}

func newFakeHub(t *testing.T) *fakeHub {
	h := &fakeHub{registers: make(chan signalclient.Envelope, 10)}
	up := websocket.Upgrader{}
	codes := []string{"113 134 323", "555 666 777", "888 999 000"}
	h.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		h.mu.Lock()
		h.conn = conn
		h.mu.Unlock()
		h.send(signalclient.Envelope{Type: signalclient.TypeHello, PeerID: "devicepeer", ICEServers: []signalclient.ICEServer{
			{URLs: []string{"stun:stun.example:3478"}},
			{URLs: []string{"turn:turn.example:3478"}, Username: "1:devicepeer", Credential: "c"},
		}})
		n := 0
		for {
			var env signalclient.Envelope
			if err := conn.ReadJSON(&env); err != nil {
				return
			}
			if env.Type == signalclient.TypeRegister {
				h.registers <- env
				h.send(signalclient.Envelope{Type: signalclient.TypeCode, Code: codes[n%len(codes)]})
				n++
			}
		}
	}))
	t.Cleanup(h.Close)
	return h
}

// drop closes the device's WebSocket. httptest's CloseClientConnections
// does not see hijacked WebSocket connections.
func (h *fakeHub) drop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	_ = h.conn.Close()
}

func (h *fakeHub) send(env signalclient.Envelope) {
	h.mu.Lock()
	defer h.mu.Unlock()
	_ = h.conn.WriteJSON(env)
}

func TestDeviceShowsCodeAndLinkedBrowsers(t *testing.T) {
	hub := newFakeHub(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	status := services.NewStatusService(deviceID, "test", "fake", "")
	pairing := services.NewPairingService(services.PairingConfig{DeviceID: deviceID, DeviceSecret: deviceSecret, Logger: logger}, status, services.NewICEStore())
	client := signalclient.New(signalclient.Config{URL: "ws" + strings.TrimPrefix(hub.URL, "http") + "/ws", Logger: logger, MinBackoff: 10 * time.Second}, pairing)
	pairing.SetSender(client)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = client.Run(ctx) }()

	// The window reads the same StatusService directly.
	getStatus := status.Snapshot
	eventually := func(what string, ok func(models.Status) bool) models.Status {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for {
			st := getStatus()
			if ok(st) {
				return st
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: last status %+v", what, st)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}

	reg := <-hub.registers
	// The secret goes with every register: signalhub binds the device_id to it.
	if reg.App != models.App || reg.DeviceID != deviceID || reg.DeviceSecret != deviceSecret {
		t.Fatalf("register %+v", reg)
	}
	st := eventually("first code", func(s models.Status) bool { return s.Pairing.Code == "113 134 323" })
	if st.Signal.State != models.SignalConnected || len(st.Signal.ICEURLs) != 2 {
		t.Fatalf("signal %+v", st.Signal)
	}

	// A browser redeems the code: it appears and a new code replaces the old one.
	hub.send(signalclient.Envelope{Type: signalclient.TypePaired, SessionID: "S", Remote: "browserpeer"})
	<-hub.registers
	eventually("linked browser and new code", func(s models.Status) bool {
		return len(s.Peers) == 1 && s.Peers[0].PeerID == "browserpeer" && s.Pairing.Code == "555 666 777"
	})

	hub.send(signalclient.Envelope{Type: signalclient.TypePeerLeft, From: "browserpeer", SessionID: "S"})
	eventually("browser gone", func(s models.Status) bool { return len(s.Peers) == 0 })

	// signalhub goes away: the window shows it and hides the stale code.
	hub.drop()
	eventually("disconnected", func(s models.Status) bool {
		return s.Signal.State != models.SignalConnected && s.Pairing.Code == ""
	})
}
