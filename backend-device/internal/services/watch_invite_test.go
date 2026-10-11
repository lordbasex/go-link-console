// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/lordbasex/go-link/backend-device/pkg/signalclient"
)

func TestTheSpectatorsInvitationHasItsOwnPlacesAndLetsPeopleWatch(t *testing.T) {
	now := time.Date(2026, 10, 11, 20, 0, 0, 0, time.UTC)
	g := NewPinGate(func() time.Time { return now })
	g.SetPrivate(true)
	play, _ := g.IssueGroup(1, time.Hour, false)
	watch, _ := g.IssueWatchGroup(2, time.Hour, true)
	if play.Watch || !watch.Watch || play.Key == watch.Key {
		t.Fatalf("play %+v watch %+v", play, watch)
	}
	// The players' link is full after one person; the spectators' is not.
	if r := g.CheckKey("P", play.Key, "Pia"); !r.OK || r.Watch {
		t.Fatalf("player %+v", r)
	}
	if r := g.CheckKey("P2", play.Key, "Pol"); r.Reason != "full" {
		t.Fatalf("second player %+v", r)
	}
	// Spectators wait for the host (approval on), and say so.
	if r := g.CheckKey("A", watch.Key, "Ana"); r.Reason != "waiting" {
		t.Fatalf("spectator %+v", r)
	}
	_, knocks := g.Group()
	if len(knocks) != 1 || !knocks[0].Watch || knocks[0].Name != "Ana" {
		t.Fatalf("knocks %+v", knocks)
	}
	res, ok := g.Answer("A", true)
	if !ok || !res.OK || !res.Watch || res.Token == "" {
		t.Fatalf("let in %+v", res)
	}
	// Coming back with that token is still to watch.
	if r := g.Check("A2", "", res.Token); !r.OK || !r.Watch {
		t.Fatalf("back %+v", r)
	}
	if w := g.WatchGroup(); w == nil || w.Used != 1 || w.Uses != 2 {
		t.Fatalf("watch status %+v", w)
	}
	// Ending one invitation leaves the other, and its waiting people.
	g.CheckKey("B", watch.Key, "Bea")
	if waiting := g.RevokeGroup(); len(waiting) != 0 {
		t.Fatalf("ending the players' link told %v", waiting)
	}
	if g.WatchGroup() == nil || !g.Knocking("B") {
		t.Fatal("the spectators' invitation went with the players' one")
	}
	if waiting := g.RevokeWatchGroup(); len(waiting) != 1 || waiting[0] != "B" {
		t.Fatalf("ending the spectators' link told %v", waiting)
	}
	if r := g.CheckKey("C", watch.Key, "Cid"); r.Reason != "wrong" {
		t.Fatalf("after the end %+v", r)
	}
}

func TestSomeoneWhoCameToWatchComesInWatchingAndHeld(t *testing.T) {
	m, out, _ := handRoom(t)
	m.MarkWatcher("w")
	m.Join("w")
	m.Sync()
	st := you(out.lastState(t, "w"))
	if st["spectator"] != true || st["held"] != true {
		t.Fatalf("watcher %v", st)
	}
	// Watching, w gets the lighter video; players do not.
	if !m.Saver("w") || m.Saver("host") || !m.Saver("s") {
		t.Fatal("saver roles")
	}
	m.SetSpectatorSaver(false)
	m.Sync()
	if m.Saver("w") || out.lastState(t, "w")["spectator_saver"] != false {
		t.Fatal("the host's switch did not give spectators the full video")
	}
	// Marked after joining: moved to watching.
	m.MarkWatcher("c")
	m.Sync()
	if you(out.lastState(t, "c"))["spectator"] != true {
		t.Fatal("c still waits to play")
	}
}

func TestASpectatorsKeyLetsTheGuestInToWatch(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	room := NewTestRoomService(nil, &fakeViewers{}, "mac", logger)
	sender := newFakeSender()
	room.SetSender(sender)
	room.SetPrivate()
	out := newOutbox()
	m := NewRoomManager(RoomManagerConfig{Logger: logger}, out)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go m.Run(ctx)
	room.SetManager(m)
	room.OnConnect(signalclient.Envelope{PeerID: "A"})
	<-sender.ch
	room.OnMessage(signalclient.Envelope{Type: signalclient.TypeRoomOpened, RoomID: "R"})
	gi := room.IssueWatchGroup(5, time.Hour, false)
	if w := room.WatchInvite(); w == nil || w.Key != gi.Key {
		t.Fatalf("watch invite %+v", w)
	}
	room.OnMessage(signalclient.Envelope{Type: signalclient.TypePeerJoined, Remote: "G", RoomID: "R"})
	room.OnMessage(signalclient.Envelope{Type: signalclient.TypeSignal, From: "G", Payload: json.RawMessage(`{"kind":"pin","key":"` + gi.Key + `","name":"Gus"}`)})
	m.Join("G") // its control channel opens
	m.Sync()
	if st := you(out.lastState(t, "G")); st["spectator"] != true || st["held"] != true {
		t.Fatalf("G %v", st)
	}
}

func TestSpectatorsGetOneQualityStepDown(t *testing.T) {
	s := &StreamService{viewers: map[string]*viewer{}}
	s.newTiers()
	s.tierLevels = 1 // a game room: one size, then the quality steps
	player, watcher := &viewer{id: "p"}, &viewer{id: "w"}
	if s.tierOfLocked(player) != 0 || s.tierOfLocked(watcher) != 0 {
		t.Fatal("without a saver rule everyone gets the first tier")
	}
	s.room.Saver = func(id string) bool { return id == "w" }
	if s.tierOfLocked(player) != 0 || s.tierOfLocked(watcher) != 1 {
		t.Fatalf("player %d watcher %d", s.tierOfLocked(player), s.tierOfLocked(watcher))
	}
	// A struggling connection still goes further down.
	watcher.drop = 2
	if s.tierOfLocked(watcher) != 2 {
		t.Fatalf("struggling watcher %d", s.tierOfLocked(watcher))
	}
}
