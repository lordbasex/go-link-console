// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"encoding/json"
	"sync"

	"github.com/lordbasex/go-link-console/backend-device/pkg/signalclient"
)

// RoomOpener opens rooms for everyone that shares the device's signaling
// connection (the test pattern room and every game room). signalhub
// answers room_open with room_opened (or an error) without saying which
// request it answers, but it answers in order, so the opener keeps the
// requests in a queue and hands each answer to the oldest one.
type RoomOpener struct {
	mu      sync.Mutex
	sender  Sender
	pending []func(roomID string, err error)
}

// NewRoomOpener builds an opener; wire the signaling client with SetSender.
func NewRoomOpener() *RoomOpener { return &RoomOpener{} }

// SetSender wires the signaling client.
func (o *RoomOpener) SetSender(s Sender) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.sender = s
}

// Open asks signalhub for a room; done gets its room_id, or the error.
// The request is queued and sent under the same lock, so the queue order
// is the order signalhub sees.
// An inviteOnly room admits guests only through its invitation.
func (o *RoomOpener) Open(public, inviteOnly bool, meta json.RawMessage, done func(roomID string, err error)) {
	err := o.send(public, inviteOnly, meta, done)
	if err != nil {
		done("", err) // outside the lock: done may open again
	}
}

func (o *RoomOpener) send(public, inviteOnly bool, meta json.RawMessage, done func(roomID string, err error)) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.sender == nil {
		return errNotConnected
	}
	o.pending = append(o.pending, done)
	if err := o.sender.Send(signalclient.Envelope{Type: signalclient.TypeRoomOpen, Public: public, InviteOnly: inviteOnly, Meta: meta}); err != nil {
		o.pending = o.pending[:len(o.pending)-1]
		return err
	}
	return nil
}

// roomOpenErrors are the errors signalhub can send back for room_open.
var roomOpenErrors = map[string]bool{"room limit reached": true, "invalid meta": true}

var _ signalclient.Handler = (*RoomOpener)(nil)

// OnConnect does nothing: owners reopen their rooms themselves.
func (o *RoomOpener) OnConnect(signalclient.Envelope) {}

// OnMessage answers the oldest pending request.
func (o *RoomOpener) OnMessage(env signalclient.Envelope) {
	var err error
	switch {
	case env.Type == signalclient.TypeRoomOpened:
	case env.Type == signalclient.TypeError && roomOpenErrors[env.Error]:
		err = &RoomOpenError{Reason: env.Error}
	default:
		return
	}
	o.mu.Lock()
	if len(o.pending) == 0 {
		o.mu.Unlock()
		return
	}
	done := o.pending[0]
	o.pending = o.pending[1:]
	o.mu.Unlock()
	if err != nil {
		done("", err)
		return
	}
	done(env.RoomID, nil)
}

// OnDisconnect fails every pending request: rooms die with the connection
// and are opened again on the next one.
func (o *RoomOpener) OnDisconnect(error) {
	o.mu.Lock()
	pending := o.pending
	o.pending = nil
	o.mu.Unlock()
	for _, done := range pending {
		done("", errNotConnected)
	}
}

// RoomOpenError is signalhub refusing to open a room.
type RoomOpenError struct{ Reason string }

func (e *RoomOpenError) Error() string { return "signalhub: " + e.Reason }

type opError string

func (e opError) Error() string { return string(e) }

const errNotConnected = opError("not connected to the signaling server")
