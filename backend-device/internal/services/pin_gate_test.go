// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/pkg/signalclient"
)

func TestNewPINIsSixDigits(t *testing.T) {
	for range 50 {
		if pin := NewPIN(); !ValidPIN(pin) {
			t.Fatalf("bad PIN %q", pin)
		}
	}
	for _, bad := range []string{"", "12345", "1234567", "12a456", " 12345"} {
		if ValidPIN(bad) {
			t.Fatalf("%q passed", bad)
		}
	}
}

func TestPinGateLimitsGuesses(t *testing.T) {
	now := time.Date(2026, 9, 27, 3, 0, 0, 0, time.UTC)
	g := NewPinGate(func() time.Time { return now })
	g.SetPrivate(true)
	pin := g.Issue().Pin
	if r := g.Check("a", wrongPin(pin), ""); r.OK || r.Reason != "wrong" || r.Left != 4 {
		t.Fatalf("first wrong: %+v", r)
	}
	for range 3 {
		g.Check("a", wrongPin(pin), "")
	}
	if r := g.Check("a", wrongPin(pin), ""); r.Reason != "blocked" {
		t.Fatalf("fifth wrong: %+v", r)
	}
	// Blocked even with the right PIN.
	if r := g.Check("a", pin, ""); r.OK {
		t.Fatal("a blocked guest got in")
	}
	// Many guests guessing lock the whole room for a while.
	for i := range 16 {
		g.Check(string(rune('c'+i)), wrongPin(pin), "")
	}
	if r := g.Check("z", pin, ""); r.OK || r.Reason != "locked" || r.RetryAfter <= 0 {
		t.Fatalf("room lock: %+v", r)
	}
	now = now.Add(pinLock + time.Second)
	if r := g.Check("z", pin, ""); !r.OK {
		t.Fatalf("after the lock: %+v", r)
	}
}

func TestEveryInvitationLetsInOnePerson(t *testing.T) {
	now := time.Date(2026, 9, 27, 3, 0, 0, 0, time.UTC)
	g := NewPinGate(func() time.Time { return now })
	g.SetPrivate(true)
	first, second := g.Issue(), g.Issue()
	if first.Pin == second.Pin || !first.ExpiresAt.Equal(now.Add(passTTL)) {
		t.Fatalf("passes %+v %+v", first, second)
	}
	in := g.Check("ana", first.Pin, "")
	if !in.OK || in.Token == "" {
		t.Fatalf("first use: %+v", in)
	}
	// Someone else with the same invitation is told it was used.
	if r := g.Check("bob", first.Pin, ""); r.OK || r.Reason != "used" {
		t.Fatalf("second use: %+v", r)
	}
	// Ana reloads the page: her token brings her back without a PIN.
	if r := g.Check("ana-again", "", in.Token); !r.OK || r.Token != in.Token {
		t.Fatalf("token: %+v", r)
	}
	if r := g.Check("eve", "", "made-up"); r.OK {
		t.Fatal("a made-up token got in")
	}
	// The host's own key always works.
	if r := g.Check("host", "", g.OwnerKey()); !r.OK || !r.Owner {
		t.Fatalf("owner key: %+v", r)
	}
	// An invitation nobody used expires; a used one keeps working.
	now = now.Add(passTTL)
	if r := g.Check("bob", second.Pin, ""); r.OK {
		t.Fatal("an expired invitation got in")
	}
	if r := g.Check("ana-3", "", in.Token); !r.OK {
		t.Fatalf("token after the invitation time: %+v", r)
	}
}

func TestAnOpenGateAsksNothing(t *testing.T) {
	g := NewPinGate(nil)
	if g.Required() || !g.Check("a", "", "").OK {
		t.Fatal("an open room asked for a PIN")
	}
}

// wrongPin returns a PIN that is not pin.
func wrongPin(pin string) string {
	if pin == "000000" {
		return "111111"
	}
	return "000000"
}

func TestPrivateRoomStreamsOnlyAfterThePIN(t *testing.T) {
	views := &fakeViewers{}
	room := NewTestRoomService(nil, views, "mac", slog.New(slog.NewTextHandler(io.Discard, nil)))
	sender := newFakeSender()
	room.SetSender(sender)
	room.SetPrivate()
	pin := room.IssuePass().Pin
	room.SetTrusted(func(peer string) bool { return peer == "owner" })
	room.OnConnect(signalclient.Envelope{PeerID: "A"})
	<-sender.ch // room_open
	room.OnMessage(signalclient.Envelope{Type: signalclient.TypeRoomOpened, RoomID: "R"})

	room.OnMessage(signalclient.Envelope{Type: signalclient.TypePeerJoined, Remote: "G", RoomID: "R"})
	ask := <-sender.ch
	if ask.Type != signalclient.TypeSignal || ask.To != "G" || string(ask.Payload) != `{"kind":"pin_required"}` {
		t.Fatalf("ask: %+v %s", ask, ask.Payload)
	}
	// Offers and answers from a guest without the PIN are ignored.
	room.OnMessage(signalclient.Envelope{Type: signalclient.TypeSignal, From: "G", Payload: json.RawMessage(`{"kind":"answer"}`)})
	room.OnMessage(signalclient.Envelope{Type: signalclient.TypeSignal, From: "G", Payload: json.RawMessage(`{"kind":"pin","pin":"` + wrongPin(pin) + `"}`)})
	var res map[string]any
	_ = json.Unmarshal((<-sender.ch).Payload, &res)
	if res["kind"] != "pin_result" || res["ok"] == true || res["reason"] != "wrong" {
		t.Fatalf("wrong PIN answer %v", res)
	}
	views.mu.Lock()
	if len(views.added) != 0 || len(views.signals) != 0 {
		t.Fatalf("streamed before the PIN: %+v", views)
	}
	views.mu.Unlock()
	room.OnMessage(signalclient.Envelope{Type: signalclient.TypeSignal, From: "G", Payload: json.RawMessage(`{"kind":"pin","pin":"` + pin + `"}`)})
	_ = json.Unmarshal((<-sender.ch).Payload, &res)
	if res["ok"] != true || res["token"] == "" {
		t.Fatalf("right PIN answer %v", res)
	}
	// The owner's own browser needs no PIN.
	room.OnMessage(signalclient.Envelope{Type: signalclient.TypePeerJoined, Remote: "owner", RoomID: "R"})
	views.mu.Lock()
	defer views.mu.Unlock()
	if !slices.Equal(views.added, []string{"G", "owner"}) {
		t.Fatalf("viewers %v", views.added)
	}
}

func TestAPinGuesserCannotLockOutTheHostOrGuestsAlreadyIn(t *testing.T) {
	now := time.Date(2026, 9, 27, 3, 0, 0, 0, time.UTC)
	g := NewPinGate(func() time.Time { return now })
	g.SetPrivate(true)
	pass := g.Issue()
	in := g.Check("ana", pass.Pin, "")
	// Many wrong PINs lock the room...
	for i := range pinRoomFails {
		g.Check(string(rune('a'+i))+"-guesser", wrongPin(pass.Pin), "")
	}
	if r := g.Check("new", g.Issue().Pin, ""); r.Reason != "locked" {
		t.Fatalf("the PIN path should be locked: %+v", r)
	}
	// ...but not for the host or a guest coming back with its token.
	if r := g.Check("host", "", g.OwnerKey()); !r.OK {
		t.Fatalf("the host was locked out: %+v", r)
	}
	if r := g.Check("ana-reload", "", in.Token); !r.OK {
		t.Fatalf("a guest already in was locked out: %+v", r)
	}
}

func TestTheOwnerKeyMakesTheGuestTheRoomsHost(t *testing.T) {
	views := &fakeViewers{}
	room := NewTestRoomService(nil, views, "mac", slog.New(slog.NewTextHandler(io.Discard, nil)))
	sender := newFakeSender()
	room.SetSender(sender)
	room.SetPrivate()
	out := newOutbox()
	manager := NewRoomManager(RoomManagerConfig{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}, out)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go manager.Run(ctx)
	room.SetManager(manager)
	room.SetTrusted(func(peer string) bool { return peer == "link" })
	room.OnConnect(signalclient.Envelope{PeerID: "A"})
	<-sender.ch // room_open
	room.OnMessage(signalclient.Envelope{Type: signalclient.TypeRoomOpened, RoomID: "R"})

	// The host's room page comes in with the owner key; its linked browser
	// needs no PIN at all; a guest with an invitation is not the host.
	pin := room.IssuePass().Pin
	for peer, payload := range map[string]string{
		"host":  `{"kind":"pin","token":"` + room.OwnerKey() + `"}`,
		"guest": `{"kind":"pin","pin":"` + pin + `"}`,
	} {
		room.OnMessage(signalclient.Envelope{Type: signalclient.TypePeerJoined, Remote: peer, RoomID: "R"})
		room.OnMessage(signalclient.Envelope{Type: signalclient.TypeSignal, From: peer, Payload: json.RawMessage(payload)})
	}
	room.OnMessage(signalclient.Envelope{Type: signalclient.TypePeerJoined, Remote: "link", RoomID: "R"})
	for _, p := range []string{"host", "guest", "link"} {
		manager.Join(p)
	}
	manager.Sync()
	for peer, want := range map[string]bool{"host": true, "guest": false, "link": true} {
		if got := you(out.lastState(t, peer))["owner"]; got != want {
			t.Errorf("%s owner = %v, want %v", peer, got, want)
		}
	}
}
