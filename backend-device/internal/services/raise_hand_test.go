// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/lordbasex/go-link/backend-device/pkg/signalclient"
)

// handRoom is a two seat room: the host and b play, c waits in the queue
// and s watches. The clock moves only when the test says so.
func handRoom(t *testing.T) (*RoomManager, *outbox, func(time.Duration)) {
	t.Helper()
	out := newOutbox()
	var mu sync.Mutex
	now := time.Date(2026, 10, 10, 21, 0, 0, 0, time.UTC)
	m := NewRoomManager(RoomManagerConfig{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), MaxPlayers: 2,
		Now: func() time.Time { mu.Lock(); defer mu.Unlock(); return now }}, out)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go m.Run(ctx)
	for _, p := range []string{"host", "b", "c", "s"} {
		m.Join(p)
	}
	m.MarkOwner("host")
	m.HandleControl("s", []byte(`{"type":"hello","name":"Sol"}`))
	m.HandleControl("s", []byte(`{"type":"spectate"}`))
	m.Sync()
	return m, out, func(d time.Duration) { mu.Lock(); now = now.Add(d); mu.Unlock() }
}

func spectatorsOf(st map[string]any) []map[string]any {
	var out []map[string]any
	for _, s := range st["spectators"].([]any) {
		out = append(out, s.(map[string]any))
	}
	return out
}

func portOf(m *RoomManager, peer string) int {
	n, _ := m.PortOf(peer, 0)
	return n
}

func refusals(o *outbox, peer string) []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	var codes []string
	for _, m := range o.msgs[peer] {
		if m["type"] == "error" {
			codes = append(codes, m["code"].(string))
		}
	}
	return codes
}

func TestASpectatorRaisesAHandAndTheHostSeesIt(t *testing.T) {
	m, out, tick := handRoom(t)
	m.HandleControl("s", []byte(`{"type":"hand","on":true}`))
	m.Sync()
	if you(out.lastState(t, "s"))["hand"] != true {
		t.Fatal("the spectator's own hand is not up")
	}
	host := spectatorsOf(out.lastState(t, "host"))
	if len(host) != 1 || host[0]["hand"] != true || host[0]["peer"] != "s" || host[0]["hand_at"] == nil {
		t.Fatalf("the host sees %v", host)
	}
	// Everyone sees the hand, nobody but the host sees peer ids.
	guest := spectatorsOf(out.lastState(t, "b"))
	if guest[0]["hand"] != true || guest[0]["peer"] != nil || guest[0]["hand_at"] != nil {
		t.Fatalf("a guest sees %v", guest)
	}
	for _, q := range out.lastState(t, "b")["queue"].([]any) {
		if q.(map[string]any)["peer"] != nil {
			t.Fatal("a guest sees a peer id in the queue")
		}
	}
	// Down and up again within a minute: refused, with when it can.
	m.HandleControl("s", []byte(`{"type":"hand","on":false}`))
	m.HandleControl("s", []byte(`{"type":"hand","on":true}`))
	m.Sync()
	if you(out.lastState(t, "s"))["hand"] != false || !slices.Contains(refusals(out, "s"), "hand_too_soon") {
		t.Fatalf("raised twice in a minute: %v", refusals(out, "s"))
	}
	if you(out.lastState(t, "s"))["hand_again"] == nil {
		t.Fatal("no hand_again while waiting")
	}
	tick(handEvery)
	m.HandleControl("s", []byte(`{"type":"hand","on":true}`))
	m.Sync()
	if you(out.lastState(t, "s"))["hand"] != true {
		t.Fatal("the hand did not go up after a minute")
	}
	// Players and people in the queue have no hand to raise.
	m.HandleControl("c", []byte(`{"type":"hand","on":true}`))
	m.Sync()
	if you(out.lastState(t, "c"))["hand"] != false {
		t.Fatal("someone in the queue raised a hand")
	}
}

func TestNoHandWithoutTheHost(t *testing.T) {
	m, out, _ := handRoom(t)
	m.Leave("host")
	m.HandleControl("s", []byte(`{"type":"hand","on":true}`))
	m.Sync()
	if you(out.lastState(t, "s"))["hand"] != false || !slices.Contains(refusals(out, "s"), "hand_no_host") {
		t.Fatalf("a hand nobody can see: %v", refusals(out, "s"))
	}
}

func TestTheHostAnswersAHand(t *testing.T) {
	m, out, tick := handRoom(t)
	m.HandleControl("s", []byte(`{"type":"hand","on":true}`))
	// Not now: the hand goes down and only Sol hears it.
	m.HandleControl("host", []byte(`{"type":"hand_later","from":"s"}`))
	m.Sync()
	if you(out.lastState(t, "s"))["hand"] != false || lastOfType(out, "s", "chat")["event"] != EventHandLater {
		t.Fatalf("not now: %v", lastOfType(out, "s", "chat"))
	}
	for _, c := range out.chats("b") {
		if c["event"] == EventHandLater {
			t.Fatal("the others heard the host's not now")
		}
	}
	// A guest cannot let someone in, nor say not now.
	tick(handEvery)
	m.HandleControl("s", []byte(`{"type":"hand","on":true}`))
	m.HandleControl("b", []byte(`{"type":"hand_later","from":"s"}`))
	m.HandleControl("b", []byte(`{"type":"promote","from":"s","mode":"queue"}`))
	m.Sync()
	if you(out.lastState(t, "s"))["hand"] != true || you(out.lastState(t, "s"))["spectator"] != true {
		t.Fatal("a guest answered the hand")
	}
	if !slices.Contains(refusals(out, "b"), "promote_owner_only") {
		t.Fatal("no refusal for the guest")
	}
	// To the queue: the hand goes down and Sol waits after c.
	m.HandleControl("host", []byte(`{"type":"promote","from":"s","mode":"queue"}`))
	m.Sync()
	st := you(out.lastState(t, "s"))
	if st["spectator"] != false || st["hand"] != false || len(st["queue_positions"].([]any)) != 1 || st["queue_positions"].([]any)[0] != 2.0 {
		t.Fatalf("to the queue: %v", st)
	}
	if lastOfType(out, "b", "chat")["event"] != EventLetQueue {
		t.Fatalf("chat %v", lastOfType(out, "b", "chat"))
	}
}

func TestPlayNowSeatsAFriendAndTheOtherGoesFirstInTheQueue(t *testing.T) {
	m, out, _ := handRoom(t)
	if portOf(m, "host") != 1 || portOf(m, "b") != 2 {
		t.Fatalf("seats host %d b %d", portOf(m, "host"), portOf(m, "b"))
	}
	m.HandleControl("b", []byte(`{"type":"pause_request"}`))
	m.HandleControl("host", []byte(`{"type":"promote","from":"s","mode":"play","port":2}`))
	m.Sync()
	if portOf(m, "s") != 2 || portOf(m, "b") != 0 {
		t.Fatalf("after play now: s %d b %d", portOf(m, "s"), portOf(m, "b"))
	}
	// b gave the seat up and goes first, before c who was waiting.
	queue := out.lastState(t, "host")["queue"].([]any)
	if len(queue) != 2 || queue[0].(map[string]any)["peer"] != "b" || queue[1].(map[string]any)["peer"] != "c" {
		t.Fatalf("queue %v", queue)
	}
	var events []string
	for _, c := range out.chats("c") {
		if e, ok := c["event"].(string); ok {
			events = append(events, e)
		}
	}
	if !slices.Contains(events, EventPlayNow) || !slices.Contains(events, EventFirstInQueue) {
		t.Fatalf("events %v", events)
	}
	// Someone in the queue can be seated at once too, and a free seat is
	// simply taken.
	m.HandleControl("host", []byte(`{"type":"release_seat","port":1,"mode":"watch"}`)) // b, first in line, takes P1
	m.Sync()
	if portOf(m, "b") != 1 {
		t.Fatalf("b %d", portOf(m, "b"))
	}
	m.HandleControl("host", []byte(`{"type":"promote","from":"c","mode":"play","port":2}`))
	m.Sync()
	if portOf(m, "c") != 2 || portOf(m, "s") != 0 {
		t.Fatalf("c %d s %d", portOf(m, "c"), portOf(m, "s"))
	}
	// Bad ports and unknown people change nothing.
	m.HandleControl("host", []byte(`{"type":"promote","from":"s","mode":"play","port":9}`))
	m.HandleControl("host", []byte(`{"type":"promote","from":"nobody","mode":"play","port":1}`))
	m.Sync()
	if portOf(m, "b") != 1 || portOf(m, "c") != 2 {
		t.Fatal("a bad promote moved someone")
	}
}

func TestSomeoneTheHostSentToWatchAsksToPlayAgain(t *testing.T) {
	m, out, _ := handRoom(t)
	m.HandleControl("host", []byte(`{"type":"release_seat","port":2,"mode":"watch"}`))
	m.Sync()
	if you(out.lastState(t, "b"))["held"] != true {
		t.Fatal("b is not held")
	}
	// b cannot just join the queue again: b raises a hand instead.
	m.HandleControl("b", []byte(`{"type":"queue"}`))
	m.Sync()
	if you(out.lastState(t, "b"))["spectator"] != true || !slices.Contains(refusals(out, "b"), "queue_ask_host") {
		t.Fatal("a held spectator joined the queue alone")
	}
	// Someone who chose to watch comes back alone, as before.
	m.HandleControl("s", []byte(`{"type":"queue"}`))
	m.Sync()
	if you(out.lastState(t, "s"))["spectator"] != false {
		t.Fatal("a spectator by choice could not queue")
	}
	m.HandleControl("host", []byte(`{"type":"promote","from":"b","mode":"queue"}`))
	m.Sync()
	if st := you(out.lastState(t, "b")); st["spectator"] != false || st["held"] != false {
		t.Fatalf("let into the queue: %v", st)
	}
}

func TestTheHostTakesSomeoneOutOfTheRoom(t *testing.T) {
	m, out, _ := handRoom(t)
	removed := make(chan string, 2)
	m.OnRemove(func(peer string) { removed <- peer })
	m.HandleControl("b", []byte(`{"type":"remove_guest","from":"c"}`))
	m.HandleControl("host", []byte(`{"type":"remove_guest","from":"host"}`))
	m.Sync()
	if !slices.Contains(refusals(out, "b"), "remove_owner_only") {
		t.Fatal("a guest removed someone")
	}
	m.HandleControl("host", []byte(`{"type":"remove_guest","from":"b"}`))
	m.Sync()
	if got := <-removed; got != "b" {
		t.Fatalf("removed %q", got)
	}
	// c, who waited, takes b's seat; the host stays.
	if portOf(m, "c") != 2 || portOf(m, "host") != 1 {
		t.Fatalf("c %d host %d", portOf(m, "c"), portOf(m, "host"))
	}
	if lastOfType(out, "s", "chat")["event"] != EventTookSeat {
		var events []any
		for _, c := range out.chats("s") {
			events = append(events, c["event"])
		}
		if !slices.Contains(events, any(EventRemoved)) {
			t.Fatalf("events %v", events)
		}
	}
	select {
	case p := <-removed:
		t.Fatalf("also removed %q", p)
	case <-time.After(50 * time.Millisecond):
	}
}

// The room service side of "remove": the stream stops and the guest's
// token no longer opens the room, even from a new connection.
func TestARemovedGuestCannotComeBackWithItsToken(t *testing.T) {
	views := &fakeViewers{}
	room := NewTestRoomService(nil, views, "mac", slog.New(slog.NewTextHandler(io.Discard, nil)))
	sender := newFakeSender()
	room.SetSender(sender)
	room.SetPrivate()
	room.OnConnect(signalclient.Envelope{PeerID: "A"})
	<-sender.ch
	room.OnMessage(signalclient.Envelope{Type: signalclient.TypeRoomOpened, RoomID: "R"})
	pass := room.IssuePass()

	// nextSignal skips the lobby updates (room_update) between signals.
	nextSignal := func() map[string]any {
		for {
			env := <-sender.ch
			if env.Type == signalclient.TypeSignal {
				var res map[string]any
				_ = json.Unmarshal(env.Payload, &res)
				return res
			}
		}
	}
	join := func(peer, body string) map[string]any {
		room.OnMessage(signalclient.Envelope{Type: signalclient.TypePeerJoined, Remote: peer, RoomID: "R"})
		nextSignal() // pin_required
		room.OnMessage(signalclient.Envelope{Type: signalclient.TypeSignal, From: peer, Payload: json.RawMessage(body)})
		return nextSignal()
	}
	res := join("G", `{"kind":"pin","pin":"`+pass.Pin+`"}`)
	token, _ := res["token"].(string)
	if res["ok"] != true || token == "" {
		t.Fatalf("in: %v", res)
	}
	if room.Remove("nobody") {
		t.Fatal("removed someone not in the room")
	}
	if !room.Remove("G") {
		t.Fatal("G was not removed")
	}
	if told := nextSignal(); told["kind"] != "pin_result" || told["reason"] != "removed" {
		t.Fatalf("told %v", told)
	}
	views.mu.Lock()
	gone := slices.Contains(views.removed, "G")
	views.mu.Unlock()
	if !gone {
		t.Fatal("the stream to G did not stop")
	}
	// A new connection with the old token is turned away, without counting
	// as a wrong try; the PIN stays used.
	if res := join("G2", `{"kind":"pin","token":"`+token+`"}`); res["ok"] == true || res["reason"] != "removed" || res["left"] != nil {
		t.Fatalf("back with the token: %v", res)
	}
	if res := join("G3", `{"kind":"pin","pin":"`+pass.Pin+`"}`); res["reason"] != "used" {
		t.Fatalf("the PIN again: %v", res)
	}
}
