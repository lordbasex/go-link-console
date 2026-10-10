// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/lordbasex/go-link/backend-device/internal/models"
	"github.com/lordbasex/go-link/backend-device/pkg/signalclient"
)

// TestRoomID is how linked browsers name the test pattern room in
// room_action (it is not one of the saved game rooms).
const TestRoomID = "test"

// Viewers is what TestRoomService needs from the stream.
type Viewers interface {
	AddViewer(peerID string) error
	HandleSignal(from string, payload json.RawMessage) error
	RemoveViewer(peerID string)
	CloseAll()
}

// TestRoomService keeps one room open in signalhub and turns its guests
// into viewers of a stream. The device uses one for the permanent test
// pattern room and one per game room (with status nil: game rooms report
// through RoomsService).
type TestRoomService struct {
	log      *slog.Logger
	status   *StatusService
	viewers  Viewers
	hostName string
	title    string
	game     string
	public   bool
	opened   func(roomID string)
	manager  *RoomManager

	mu      sync.Mutex
	sender  Sender
	roomID  string
	guests  map[string]bool
	tokens  map[string]string // the return token each guest came in with
	summary RoomSummary
	extra   map[string]any // more lobby fields (paused, art)
	closed  bool           // closed for good: never reopened
	onRoom  func(roomID string)
	opener  *RoomOpener
	// A private room's PIN: guests wait in pending until they send it.
	pin     *PinGate
	pending map[string]bool
	trusted func(peerID string) bool // the device's owner, who needs no PIN
	// Invitations (game rooms): each opened room asks signalhub for one;
	// a private room then admits guests only through it.
	invites    bool
	onInvite   func(invite, code string)
	invite     string
	inviteCode string
	picture    *models.RoomPicture // the host's default picture style

	onAdmission func() // the group invitation or the people waiting changed
}

// ErrNotWaiting is the answer to a host who lets in someone not waiting.
var ErrNotWaiting = errors.New("that person is not waiting to come in")

// OnAdmission calls fn when the room's group invitation or the people
// waiting for the host change (game rooms publish their status then).
func (t *TestRoomService) OnAdmission(fn func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onAdmission = fn
}

func (t *TestRoomService) admissionChanged() {
	t.mu.Lock()
	fn := t.onAdmission
	t.mu.Unlock()
	t.setStatus()
	if fn != nil {
		fn()
	}
}

// Admission is the room's group invitation and the people waiting for the
// host, for the owner's status.
func (t *TestRoomService) Admission() (*models.GroupInvite, []models.Knock) {
	return t.pin.Group()
}

// IssueGroup makes the room's group invitation (one link for several
// people), replacing the one before. People still waiting with the old link
// are told it expired.
func (t *TestRoomService) IssueGroup(uses int, ttl time.Duration, approval bool) models.GroupInvite {
	gi, waiting := t.pin.IssueGroup(uses, ttl, approval)
	t.tellWaiting(waiting, "expired")
	t.admissionChanged()
	return gi
}

// RevokeGroup ends the room's group invitation: its link stops working.
func (t *TestRoomService) RevokeGroup() {
	t.tellWaiting(t.pin.RevokeGroup(), "expired")
	t.admissionChanged()
}

func (t *TestRoomService) tellWaiting(peers []string, reason string) {
	for _, p := range peers {
		t.signal(p, struct {
			Kind string `json:"kind"`
			PinResult
		}{"pin_result", PinResult{Reason: reason}})
	}
}

// Answer is the host letting someone waiting in, or not. Someone who left
// or already got in another way is not waiting.
func (t *TestRoomService) Answer(peer string, accept bool) error {
	t.mu.Lock()
	pending := t.pending[peer]
	t.mu.Unlock()
	if !pending {
		t.pin.Forget(peer)
		t.admissionChanged()
		return ErrNotWaiting
	}
	res, ok := t.pin.Answer(peer, accept)
	if !ok {
		return ErrNotWaiting
	}
	t.signal(peer, struct {
		Kind string `json:"kind"`
		PinResult
	}{"pin_result", res})
	if res.OK {
		t.admitWith(peer, res.Token)
	}
	t.log.Info("host answered a guest", "peer_id", peer, "accept", accept, "reason", res.Reason)
	t.admissionChanged()
	return nil
}

// Remove takes a guest out of the room for good (the host's "remove from
// the room"): the stream to it stops and its return token no longer opens
// the room; the guest is told with pin_result reason "removed". It reports
// whether the peer was in the room.
func (t *TestRoomService) Remove(peer string) bool {
	t.mu.Lock()
	guest := t.guests[peer]
	token := t.tokens[peer]
	delete(t.guests, peer)
	delete(t.tokens, peer)
	t.mu.Unlock()
	if !guest {
		return false
	}
	t.pin.Revoke(token)
	t.signal(peer, struct {
		Kind string `json:"kind"`
		PinResult
	}{"pin_result", PinResult{Reason: "removed"}})
	t.viewers.RemoveViewer(peer)
	t.log.Info("host removed a guest", "peer_id", peer)
	t.publish()
	return true
}

// SetPrivate makes guests need an invitation's PIN. Guests already in
// stay in.
func (t *TestRoomService) SetPrivate() {
	t.pin.SetPrivate(true)
	t.setStatus()
}

// SetPicture sets the room's default picture style for its guests (nil:
// the website's own default): the Room Manager sends it in room_state and
// the device status shows it to the owner.
func (t *TestRoomService) SetPicture(p *models.RoomPicture) {
	p = models.CleanPicture(p)
	t.mu.Lock()
	t.picture = p
	m := t.manager
	t.mu.Unlock()
	if m != nil {
		m.SetPicture(p)
	}
	t.setStatus()
}

// IssuePass makes a new invitation PIN, good for one person.
func (t *TestRoomService) IssuePass() Pass { return t.pin.Issue() }

// OwnerKey is what the host's own browsers send instead of a PIN.
func (t *TestRoomService) OwnerKey() string { return t.pin.OwnerKey() }

// SetTrusted tells who may skip the PIN: the device's own linked browsers.
func (t *TestRoomService) SetTrusted(fn func(peerID string) bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.trusted = fn
}

// signal sends a signal payload to one peer.
func (t *TestRoomService) signal(to string, payload any) {
	b, err := json.Marshal(payload)
	if err == nil {
		t.send(signalclient.Envelope{Type: signalclient.TypeSignal, To: to, Payload: b})
	}
}

// admit turns a guest into a viewer.
func (t *TestRoomService) admit(peer string) { t.admitWith(peer, "") }

// admitWith is admit for a guest that came in with a return token (kept so
// the host can take the guest out for good).
func (t *TestRoomService) admitWith(peer, token string) {
	t.mu.Lock()
	t.guests[peer] = true
	if token != "" {
		t.tokens[peer] = token
	}
	delete(t.pending, peer)
	t.mu.Unlock()
	t.log.Info("viewer joined", "peer_id", peer)
	if err := t.viewers.AddViewer(peer); err != nil {
		t.log.Error("cannot start stream", "peer_id", peer, "err", err)
	}
	t.publish()
}

// markOwner tells the Room Manager that a guest is the host's own browser.
func (t *TestRoomService) markOwner(peer string) {
	t.mu.Lock()
	m := t.manager
	t.mu.Unlock()
	if m != nil {
		m.MarkOwner(peer)
	}
}

// checkPin answers a guest's PIN and lets it in when right.
func (t *TestRoomService) checkPin(peer string, payload json.RawMessage) {
	var msg struct {
		Kind  string `json:"kind"`
		Pin   string `json:"pin"`
		Token string `json:"token"`
		Key   string `json:"key"`  // a group invitation's key
		Name  string `json:"name"` // shown to the host while waiting
	}
	if len(payload) > 512 || json.Unmarshal(payload, &msg) != nil || msg.Kind != "pin" {
		return
	}
	var res PinResult
	wasWaiting := t.pin.Knocking(peer)
	if msg.Key != "" && msg.Token == "" {
		res = t.pin.CheckKey(peer, msg.Key, msg.Name)
	} else {
		res = t.pin.Check(peer, msg.Pin, msg.Token)
	}
	t.signal(peer, struct {
		Kind string `json:"kind"`
		PinResult
	}{"pin_result", res})
	if res.OK {
		if res.Owner {
			t.markOwner(peer)
		}
		t.admitWith(peer, res.Token)
		if msg.Key != "" || wasWaiting {
			t.admissionChanged() // one more place used, or one less waiting
		}
		return
	}
	if res.Reason == "waiting" {
		if !wasWaiting { // asking again changes nothing for the host
			t.log.Info("guest waits for the host", "peer_id", peer)
			t.admissionChanged()
		}
		return
	}
	t.log.Warn("wrong room PIN", "peer_id", peer, "reason", res.Reason)
}

// EnableInvites makes the room ask signalhub for an invitation each time
// it opens; onInvite receives it (and "" "" when the room closes).
func (t *TestRoomService) EnableInvites(onInvite func(invite, code string)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.invites, t.onInvite = true, onInvite
}

// NewInvite asks for a new invitation, which retires the current one
// ("New link").
func (t *TestRoomService) NewInvite() {
	t.mu.Lock()
	id, on := t.roomID, t.invites
	t.mu.Unlock()
	if on && id != "" {
		t.send(signalclient.Envelope{Type: signalclient.TypeInviteCreate, RoomID: id})
	}
}

// Matches returns the room's room_id when roomID, invite or code names it
// (the device's local panel lets its browsers in without signalhub).
func (t *TestRoomService) Matches(roomID, invite, code string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch {
	case t.roomID == "":
		return ""
	case roomID != "" && roomID == t.roomID,
		invite != "" && invite == t.invite,
		code != "" && code == t.inviteCode:
		return t.roomID
	}
	return ""
}

// setInvite records the current invitation and reports it.
func (t *TestRoomService) setInvite(invite, code string) {
	t.mu.Lock()
	changed := t.invite != invite || t.inviteCode != code
	t.invite, t.inviteCode = invite, code
	fn := t.onInvite
	t.mu.Unlock()
	if changed && fn != nil {
		fn(invite, code)
	}
	if changed {
		t.setStatus()
	}
}

// SetOpener makes the room open through the device's shared opener, so
// several rooms on one signaling connection each get their own room_id.
func (t *TestRoomService) SetOpener(o *RoomOpener) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.opener = o
}

// open asks signalhub for the room.
func (t *TestRoomService) open(public bool, meta json.RawMessage) {
	t.mu.Lock()
	o := t.opener
	inviteOnly := t.invites && !public
	t.mu.Unlock()
	if o == nil {
		t.send(signalclient.Envelope{Type: signalclient.TypeRoomOpen, Public: public, InviteOnly: inviteOnly, Meta: meta})
		return
	}
	o.Open(public, inviteOnly, meta, func(roomID string, err error) {
		if err != nil {
			t.log.Warn("cannot open the room", "err", err)
			return
		}
		t.roomOpened(roomID)
	})
}

// Configure sets the room's name, game and visibility before it opens.
func (t *TestRoomService) Configure(title, game string, public bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.title, t.game, t.public = title, game, public
}

// OnRoomID is told the signalhub room_id each time the room opens.
func (t *TestRoomService) OnRoomID(fn func(roomID string)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onRoom = fn
}

// SetMetaExtra adds fields to the lobby entry (for example paused and
// art) and publishes it.
func (t *TestRoomService) SetMetaExtra(extra map[string]any) {
	t.mu.Lock()
	t.extra = extra
	t.mu.Unlock()
	t.publish()
}

// Close closes the room for good: it leaves the lobby, guests are
// dropped and it does not open again on reconnection.
func (t *TestRoomService) Close() {
	t.mu.Lock()
	t.closed = true
	id := t.roomID
	t.roomID = ""
	t.guests = make(map[string]bool)
	t.pending = make(map[string]bool)
	t.mu.Unlock()
	if id != "" {
		t.send(signalclient.Envelope{Type: signalclient.TypeRoomClose, RoomID: id})
	}
	t.setInvite("", "")
	t.viewers.CloseAll()
}

// SetGame names the room after the game it streams (default: the test
// pattern).
func (t *TestRoomService) SetGame(title, game string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.title, t.game = title, game
}

// Reopen replaces the room with a new name and visibility: it closes the
// current room and opens a new one (the room_id changes; guests already in
// the session stay connected). opened, if not nil, receives the new id.
func (t *TestRoomService) Reopen(title, game string, public bool, opened func(roomID string)) {
	t.mu.Lock()
	t.title, t.game, t.public = title, game, public
	t.opened = opened
	old := t.roomID
	t.roomID = ""
	sum := t.summary
	t.mu.Unlock()
	if old != "" {
		t.send(signalclient.Envelope{Type: signalclient.TypeRoomClose, RoomID: old})
	}
	t.setInvite("", "") // it died with the old room
	t.open(public, t.meta(sum))
}

// SetManager attaches the Room Manager whose seats, queue and spectators
// are published in the lobby.
func (t *TestRoomService) SetManager(m *RoomManager) {
	t.mu.Lock()
	t.manager = m
	t.mu.Unlock()
	if m != nil {
		m.OnRemove(func(peer string) { t.Remove(peer) })
	}
}

// OnSummary receives the Room Manager's counts and updates the lobby.
func (t *TestRoomService) OnSummary(s RoomSummary) {
	s.Members = nil // the lobby only shows counts
	t.mu.Lock()
	changed := s.Players != t.summary.Players || s.Queue != t.summary.Queue ||
		s.Spectators != t.summary.Spectators || s.MaxPlayers != t.summary.MaxPlayers
	t.summary = s
	t.mu.Unlock()
	if changed {
		t.publish()
	}
}

var _ signalclient.Handler = (*TestRoomService)(nil)

// NewTestRoomService builds the service. hostName is shown in the lobby.
func NewTestRoomService(status *StatusService, viewers Viewers, hostName string, logger *slog.Logger) *TestRoomService {
	if logger == nil {
		logger = slog.Default()
	}
	return &TestRoomService{log: logger, status: status, viewers: viewers, hostName: hostName, title: "Test pattern", game: "Test pattern", public: true, guests: make(map[string]bool), tokens: make(map[string]string), pin: NewPinGate(nil), pending: make(map[string]bool)}
}

// SetSender wires the signaling client.
func (t *TestRoomService) SetSender(s Sender) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sender = s
}

func (t *TestRoomService) send(env signalclient.Envelope) {
	t.mu.Lock()
	s := t.sender
	t.mu.Unlock()
	if s == nil {
		return
	}
	if err := s.Send(env); err != nil {
		t.log.Warn("send failed", "type", env.Type, "err", err)
	}
}

// meta is the directory entry, in the shape the web expects.
func (t *TestRoomService) meta(s RoomSummary) json.RawMessage {
	t.mu.Lock()
	title, game := t.title, t.game
	meta := map[string]any{
		"title":       title,
		"game":        game,
		"host":        t.hostName,
		"players":     s.Players,
		"max_players": seatsOrDefault(s.MaxPlayers),
		"queue":       s.Queue,
		"spectators":  s.Spectators,
		"mode":        "coop",
	}
	for k, v := range t.extra {
		meta[k] = v
	}
	t.mu.Unlock()
	b, _ := json.Marshal(meta)
	return b
}

// OnConnect opens the room on every (re)connection: rooms die with the
// signaling connection.
func (t *TestRoomService) OnConnect(signalclient.Envelope) {
	t.mu.Lock()
	public, closed := t.public, t.closed
	t.mu.Unlock()
	if closed {
		return
	}
	t.open(public, t.meta(RoomSummary{}))
}

// OnMessage handles room and WebRTC signaling messages.
func (t *TestRoomService) OnMessage(env signalclient.Envelope) {
	switch env.Type {
	case signalclient.TypeRoomOpened:
		t.mu.Lock()
		shared := t.opener != nil
		t.mu.Unlock()
		if !shared { // with an opener, the opener delivers it
			t.roomOpened(env.RoomID)
		}
	case signalclient.TypeInviteCreated:
		t.mu.Lock()
		mine := env.RoomID != "" && env.RoomID == t.roomID
		t.mu.Unlock()
		if mine {
			t.setInvite(env.Invite, env.Code)
		}
	case signalclient.TypePeerJoined:
		t.mu.Lock()
		mine := env.RoomID != "" && env.RoomID == t.roomID
		trusted := t.trusted
		t.mu.Unlock()
		if !mine {
			return
		}
		if t.pin.Required() && (trusted == nil || !trusted(env.Remote)) {
			// A private room: nothing is streamed until the right PIN.
			t.mu.Lock()
			t.pending[env.Remote] = true
			t.mu.Unlock()
			t.log.Info("guest asked for the PIN", "peer_id", env.Remote)
			t.signal(env.Remote, map[string]string{"kind": "pin_required"})
			return
		}
		if trusted != nil && trusted(env.Remote) {
			t.markOwner(env.Remote) // the host's linked browser itself
		}
		t.admit(env.Remote)
	case signalclient.TypeSignal:
		t.mu.Lock()
		guest := t.guests[env.From]
		pending := t.pending[env.From]
		t.mu.Unlock()
		if pending {
			t.checkPin(env.From, env.Payload)
			return
		}
		if guest {
			if err := t.viewers.HandleSignal(env.From, env.Payload); err != nil {
				t.log.Warn("bad signal", "peer_id", env.From, "err", err)
			}
		}
	case signalclient.TypePeerLeft:
		t.mu.Lock()
		guest := t.guests[env.From]
		delete(t.guests, env.From)
		delete(t.tokens, env.From)
		_, waiting := t.pending[env.From]
		delete(t.pending, env.From)
		t.mu.Unlock()
		t.pin.Forget(env.From)
		if waiting {
			t.admissionChanged() // a guest waiting for the host gave up
		}
		if guest {
			t.viewers.RemoveViewer(env.From)
			t.log.Info("viewer left", "peer_id", env.From)
			t.publish()
		}
	}
}

// roomOpened takes the room_id signalhub gave this room.
func (t *TestRoomService) roomOpened(roomID string) {
	t.mu.Lock()
	if t.closed {
		// Closed while the room was opening: close it right away.
		t.mu.Unlock()
		t.send(signalclient.Envelope{Type: signalclient.TypeRoomClose, RoomID: roomID})
		return
	}
	if t.roomID != "" {
		// Without a shared opener, every room_opened of the owner reaches
		// every room: keep the first.
		t.mu.Unlock()
		return
	}
	t.roomID = roomID
	opened := t.opened
	t.opened = nil
	title := t.title
	onRoom := t.onRoom
	t.mu.Unlock()
	if onRoom != nil {
		onRoom(roomID)
	}
	t.setStatus() // the window gets the full room state
	t.log.Info("room open", "room_id", roomID, "title", title)
	t.NewInvite()
	if opened != nil {
		opened(roomID)
	}
}

// OnDisconnect forgets the room and every viewer.
func (t *TestRoomService) OnDisconnect(error) {
	t.mu.Lock()
	t.roomID = ""
	t.guests = make(map[string]bool)
	t.pending = make(map[string]bool)
	m := t.manager
	t.mu.Unlock()
	t.setInvite("", "")
	t.viewers.CloseAll()
	if m != nil {
		m.Reset()
	}
	if t.status != nil {
		t.status.SetRoom("", 0)
	}
}

// publish updates the lobby entry and the local status.
func (t *TestRoomService) publish() {
	t.mu.Lock()
	id, n, sum := t.roomID, len(t.guests), t.summary
	t.mu.Unlock()
	if id == "" {
		return
	}
	if t.manager == nil {
		// No Room Manager: every guest counts as a player.
		sum = RoomSummary{Players: min(n, 4), Queue: max(n-4, 0)}
	}
	t.setStatusWith(id, n, sum)
	t.send(signalclient.Envelope{Type: signalclient.TypeRoomUpdate, RoomID: id, Meta: t.meta(sum)})
}

// setStatus records the open room in the device status.
func (t *TestRoomService) setStatus() {
	t.mu.Lock()
	id, n, sum := t.roomID, len(t.guests), t.summary
	t.mu.Unlock()
	if id != "" {
		t.setStatusWith(id, n, sum)
	}
}

func (t *TestRoomService) setStatusWith(id string, n int, sum RoomSummary) {
	if t.status == nil {
		return
	}
	t.mu.Lock()
	title, game, public := t.title, t.game, t.public
	invite, code, picture := t.invite, t.inviteCode, t.picture
	t.mu.Unlock()
	key := ""
	if t.pin.Required() {
		key = t.pin.OwnerKey()
	}
	group, knocks := t.pin.Group()
	t.status.SetRoomDetails(models.RoomStatus{
		RoomID: id, Viewers: n, Title: title, Game: game, Public: public,
		Players: sum.Players, MaxPlayers: seatsOrDefault(sum.MaxPlayers), Queue: sum.Queue, Spectators: sum.Spectators,
		Invite: invite, InviteCode: code, OwnerKey: key, Picture: picture,
		GroupInvite: group, Knocks: knocks,
	})
}
