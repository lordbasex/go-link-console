// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"encoding/json"
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
	"github.com/lordbasex/go-link-console/backend-device/pkg/signalclient"
)

type fakeViewers struct {
	mu      sync.Mutex
	added   []string
	signals []string
	removed []string
	closed  int
}

func (f *fakeViewers) AddViewer(id string) error {
	f.mu.Lock()
	f.added = append(f.added, id)
	f.mu.Unlock()
	return nil
}
func (f *fakeViewers) HandleSignal(from string, _ json.RawMessage) error {
	f.mu.Lock()
	f.signals = append(f.signals, from)
	f.mu.Unlock()
	return nil
}
func (f *fakeViewers) RemoveViewer(id string) {
	f.mu.Lock()
	f.removed = append(f.removed, id)
	f.mu.Unlock()
}
func (f *fakeViewers) CloseAll() { f.mu.Lock(); f.closed++; f.mu.Unlock() }

func TestTestRoomRouting(t *testing.T) {
	st := NewStatusService(deviceID, "test", "ws://x", "")
	views := &fakeViewers{}
	room := NewTestRoomService(st, views, "mac", slog.New(slog.NewTextHandler(io.Discard, nil)))
	sender := newFakeSender()
	room.SetSender(sender)

	room.OnConnect(signalclient.Envelope{PeerID: "A"})
	open := <-sender.ch
	var meta map[string]any
	_ = json.Unmarshal(open.Meta, &meta)
	if open.Type != signalclient.TypeRoomOpen || !open.Public || meta["title"] != "Test pattern" || meta["host"] != "mac" {
		t.Fatalf("room_open %+v %v", open, meta)
	}

	room.OnMessage(signalclient.Envelope{Type: signalclient.TypeRoomOpened, RoomID: "R"})
	if s := st.Snapshot(); s.Room == nil || s.Room.RoomID != "R" {
		t.Fatalf("status room %+v", s.Room)
	}

	// A paired browser (peer_joined without our room) is not a viewer.
	room.OnMessage(signalclient.Envelope{Type: signalclient.TypePeerJoined, Remote: "other", RoomID: "X"})
	room.OnMessage(signalclient.Envelope{Type: signalclient.TypePeerJoined, Remote: "G", RoomID: "R"})
	upd := <-sender.ch
	_ = json.Unmarshal(upd.Meta, &meta)
	if upd.Type != signalclient.TypeRoomUpdate || meta["players"] != float64(1) {
		t.Fatalf("room_update %+v %v", upd, meta)
	}

	room.OnMessage(signalclient.Envelope{Type: signalclient.TypeSignal, From: "G", Payload: json.RawMessage(`{}`)})
	room.OnMessage(signalclient.Envelope{Type: signalclient.TypeSignal, From: "stranger", Payload: json.RawMessage(`{}`)})
	room.OnMessage(signalclient.Envelope{Type: signalclient.TypePeerLeft, From: "G"})

	views.mu.Lock()
	if !slices.Equal(views.added, []string{"G"}) || !slices.Equal(views.signals, []string{"G"}) || !slices.Equal(views.removed, []string{"G"}) {
		t.Fatalf("routing: %+v", views)
	}
	views.mu.Unlock()
	if s := st.Snapshot(); s.Room.Viewers != 0 {
		t.Fatalf("viewers %d", s.Room.Viewers)
	}

	room.OnDisconnect(nil)
	views.mu.Lock()
	defer views.mu.Unlock()
	if views.closed != 1 || st.Snapshot().Room != nil {
		t.Fatal("disconnect did not reset the room")
	}
}

func TestReopenChangesNameAndVisibility(t *testing.T) {
	st := NewStatusService(deviceID, "test", "ws://x", "")
	room := NewTestRoomService(st, &fakeViewers{}, "mac", slog.New(slog.NewTextHandler(io.Discard, nil)))
	sender := newFakeSender()
	room.SetSender(sender)
	room.OnConnect(signalclient.Envelope{})
	<-sender.ch // first room_open
	room.OnMessage(signalclient.Envelope{Type: signalclient.TypeRoomOpened, RoomID: "R1"})

	var got string
	room.Reopen("Friday night", "Robby Roto", false, func(id string) { got = id })
	closeMsg, openMsg := <-sender.ch, <-sender.ch
	var meta map[string]any
	_ = json.Unmarshal(openMsg.Meta, &meta)
	if closeMsg.Type != signalclient.TypeRoomClose || closeMsg.RoomID != "R1" {
		t.Fatalf("close %+v", closeMsg)
	}
	if openMsg.Type != signalclient.TypeRoomOpen || openMsg.Public || meta["title"] != "Friday night" || meta["game"] != "Robby Roto" {
		t.Fatalf("open %+v %v", openMsg, meta)
	}
	room.OnMessage(signalclient.Envelope{Type: signalclient.TypeRoomOpened, RoomID: "R2"})
	if got != "R2" || st.Snapshot().Room.RoomID != "R2" {
		t.Fatalf("callback %q, status %+v", got, st.Snapshot().Room)
	}
	// A later reconnection reopens with the same name and visibility.
	room.OnConnect(signalclient.Envelope{})
	again := <-sender.ch
	if again.Public {
		t.Fatal("visibility lost on reconnect")
	}
}

func TestTheRoomIsFoundByItsIDInvitationOrCode(t *testing.T) {
	room := NewTestRoomService(nil, &fakeViewers{}, "mac", slog.New(slog.NewTextHandler(io.Discard, nil)))
	room.SetSender(newFakeSender())
	if got := room.Matches("R", "", ""); got != "" {
		t.Fatalf("a room not open yet matched: %q", got)
	}
	room.OnConnect(signalclient.Envelope{PeerID: "A"})
	room.OnMessage(signalclient.Envelope{Type: signalclient.TypeRoomOpened, RoomID: "R"})
	room.OnMessage(signalclient.Envelope{Type: signalclient.TypeInviteCreated, RoomID: "R", Invite: "INV", Code: "123456789"})
	for _, c := range []struct{ id, invite, code, want string }{
		{"R", "", "", "R"},
		{"", "INV", "", "R"},
		{"", "", "123456789", "R"},
		{"X", "", "", ""},
		{"", "other", "", ""},
		{"", "", "", ""},
	} {
		if got := room.Matches(c.id, c.invite, c.code); got != c.want {
			t.Errorf("Matches(%q, %q, %q) = %q, want %q", c.id, c.invite, c.code, got, c.want)
		}
	}
}

func TestTestRoomShowsItsDefaultPictureToTheOwner(t *testing.T) {
	st := NewStatusService(deviceID, "test", "ws://x", "")
	room := NewTestRoomService(st, &fakeViewers{}, "mac", slog.New(slog.NewTextHandler(io.Discard, nil)))
	sender := newFakeSender()
	room.SetSender(sender)
	room.OnConnect(signalclient.Envelope{})
	<-sender.ch
	room.OnMessage(signalclient.Envelope{Type: signalclient.TypeRoomOpened, RoomID: "R1"})
	room.SetPicture(&models.RoomPicture{Style: "crt", Bands: "frame"})
	if p := st.Snapshot().Room.Picture; p == nil || *p != (models.RoomPicture{Style: "crt", Bands: "frame"}) {
		t.Fatalf("status picture %+v", p)
	}
	room.SetPicture(&models.RoomPicture{Style: "crt", Bands: "tartan"})
	if p := st.Snapshot().Room.Picture; p != nil {
		t.Fatalf("unknown sides kept: %+v", p)
	}
}
