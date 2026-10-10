// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/lordbasex/go-link/backend-device/internal/models"
	"github.com/lordbasex/go-link/backend-device/pkg/input"
)

// ControlSender delivers JSON messages on a peer's "control" channel.
type ControlSender interface {
	SendControl(peerID string, msg []byte) bool
}

// RoomSummary is what the public directory shows about the room.
type RoomSummary struct {
	Players, Queue, Spectators, MaxPlayers int
	// Members is everyone in the room now, for the history of games.
	Members []MemberSummary
}

// MemberSummary is one browser in the room: its name and the ports
// (1-4) its players sit at, none for spectators and the queue.
type MemberSummary struct {
	Peer  string
	Name  string
	Ports []int
}

// RoomManagerConfig configures RoomManager. Zero values get defaults.
type RoomManagerConfig struct {
	MaxPlayers int              // ports, default 4
	Now        func() time.Time // default time.Now
	Logger     *slog.Logger
	// OnSummary runs (on the actor goroutine) after every change.
	OnSummary func(RoomSummary)
	// OnLog, when set, gets what happens in the room for its telemetry:
	// arrivals, departures, seats, names, pauses (on the actor goroutine;
	// it must not block).
	OnLog func(kind, peer, text string, data map[string]any)
}

const (
	// A player's name: 2 to 20 letters, digits and single spaces.
	minNameLen = 2
	maxNameLen = 20
	maxChatLen = 300
	chatBurst  = 5
	// Any control message from a guest (hello, typing, queue...) makes the
	// room answer everyone: a guest gets controlBurst of them per second,
	// the rest are dropped, so nobody can make the room flood the others.
	controlBurst = 20
	chatWindow   = 5 * time.Second
	chatHistory  = 50
	commandQueue = 256
	// swapTimeout is how long a request to swap controllers waits for an
	// answer.
	swapTimeout = 30 * time.Second
	// pauseAskFor is how long a guest's request for a pause waits for the
	// host's answer.
	pauseAskFor = 30 * time.Second
	// typingFor is how long "is typing…" lasts after the last keystroke
	// notice; browsers repeat it every few seconds while typing.
	typingFor = 6 * time.Second
	// handEvery is how often a spectator may raise a hand to ask the host
	// to play: often enough to be seen, not enough to pester.
	handEvery = time.Minute
)

// seatKey is one local player of one browser.
type seatKey struct {
	peer  string
	local uint8
}

type member struct {
	peer      string
	name      string
	locals    []uint8
	spectator bool
	order     int
	chatTimes []time.Time
	// controlAt/controlN count the control messages of the current second.
	controlAt time.Time
	controlN  int
	// typingUntil: the member is typing a chat message until then.
	typingUntil time.Time
	// hand: a spectator raised a hand to ask the host to play (zero: down);
	// handAt is the last time it went up (one raise per handEvery).
	hand, handAt time.Time
	// held: the host moved this guest to watching, so the guest asks (a
	// raised hand) instead of joining the queue alone.
	held bool
}

// pendingSwap is a seated player asking another one to swap controllers
// (seats), like handing over the joystick: it needs the other's yes.
type pendingSwap struct {
	id       int
	from, to int     // ports
	asker    seatKey // who sat at from when asking
	target   seatKey // who sat at to when asking
}

// pauseAsk is a seated player asking the host for a pause: only the host
// pauses the game, so the others ask.
type pauseAsk struct {
	peer    string
	name    string
	port    int
	expires time.Time
}

// PauseAskEvent tells the host's linked browsers about a request for a
// pause: Type is "pause_asked" (a new or refreshed request) or
// "pause_ask_gone" (answered, withdrawn or expired).
type PauseAskEvent struct {
	Type      string     `json:"type"`
	From      string     `json:"from"`
	Name      string     `json:"name,omitempty"`
	Port      int        `json:"port,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// RoomManager owns the state of a room: seats P1-P4, the queue, the
// spectators and the chat. It is an actor: a single goroutine (Run) owns
// the state and every public method posts a command to it, so no state
// is shared and no locks are needed. The only exception is the seat map
// read by the input path, published as an immutable snapshot.
type RoomManager struct {
	cfg  RoomManagerConfig
	log  *slog.Logger
	out  ControlSender
	cmds chan func()

	// Owned by the actor goroutine.
	members   map[string]*member
	nextOrder int
	voiceOff  bool
	chatOff   bool // the host turned the room's chat off
	inputHUD  bool // the host shows the controllers on the video (latency test)
	picture   *models.RoomPicture
	info      RoomInfo
	pausable  bool // a game is running (the test card never pauses)
	controls  GameControls
	paused    bool
	pausedBy  string
	recording bool              // the host is recording the game (everyone is told)
	onPause   func(paused bool) // set by OnPause; called on the actor goroutine
	seats     []*seatKey
	queue     []seatKey
	history   [][]byte
	swaps     []pendingSwap
	nextSwap  int
	// owners are the host's own browsers in the room (they proved the
	// owner key); hostLinked tells that a linked browser of the host is
	// connected to the device. Either makes the host "online".
	owners     map[string]bool
	hostLinked bool
	pauseAsks  []pauseAsk
	onAsk      func(PauseAskEvent) // set by OnPauseAsk; called on the actor goroutine
	onRemove   func(peer string)   // set by OnRemove; called on its own goroutine

	ports atomic.Pointer[map[seatKey]int]
}

// NewRoomManager builds the manager. Call Run to start it.
func NewRoomManager(cfg RoomManagerConfig, out ControlSender) *RoomManager {
	if cfg.MaxPlayers <= 0 || cfg.MaxPlayers > 4 {
		cfg.MaxPlayers = 4
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	m := &RoomManager{
		cfg:      cfg,
		controls: TestCardControls,
		log:      cfg.Logger,
		out:      out,
		cmds:     make(chan func(), commandQueue),
		members:  make(map[string]*member),
		owners:   make(map[string]bool),
		seats:    make([]*seatKey, cfg.MaxPlayers),
	}
	empty := map[seatKey]int{}
	m.ports.Store(&empty)
	return m
}

// Run executes commands until ctx ends.
func (m *RoomManager) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case cmd := <-m.cmds:
			cmd()
		}
	}
}

func (m *RoomManager) do(cmd func()) { m.cmds <- cmd }

// Sync waits until every command posted before it has run (tests).
func (m *RoomManager) Sync() {
	done := make(chan struct{})
	m.do(func() { close(done) })
	<-done
}

// PortOf returns the port (1-4) of a local player, if seated. Safe to
// call from any goroutine; used by the input path at frame rate.
func (m *RoomManager) PortOf(peerID string, local uint8) (int, bool) {
	port, ok := (*m.ports.Load())[seatKey{peerID, local}]
	return port, ok
}

// Join adds a guest whose control channel just opened.
func (m *RoomManager) Join(peerID string) {
	m.do(func() {
		if _, ok := m.members[peerID]; ok {
			return
		}
		// Earlier chat first, then the new seat and state go out live.
		for _, msg := range m.history {
			m.out.SendControl(peerID, msg)
		}
		m.nextOrder++
		m.members[peerID] = &member{peer: peerID, name: defaultName(peerID), locals: []uint8{0}, order: m.nextOrder}
		m.teleLog("join", peerID, "joined the room", map[string]any{"name": defaultName(peerID)})
		m.reconcile()
	})
}

// Leave removes a guest. It is idempotent.
func (m *RoomManager) Leave(peerID string) {
	m.do(func() {
		mem, ok := m.members[peerID]
		if !ok {
			return
		}
		delete(m.members, peerID)
		delete(m.owners, peerID)
		m.teleLog("leave", peerID, "left the room", map[string]any{"name": mem.name})
		m.reconcile()
		if m.cfg.Now().Before(mem.typingUntil) {
			m.broadcastTyping() // who left stops "typing"
		}
	})
}

// SetVoice tells guests whether voice between players is on.
func (m *RoomManager) SetVoice(on bool) {
	m.do(func() {
		if m.voiceOff != !on {
			m.voiceOff = !on
			m.broadcastState()
		}
	})
}

// RoomInfo describes the room to its guests, over the control channel:
// rooms are private, so guests do not read it from the public directory.
type RoomInfo struct {
	Title string `json:"title"`
	Game  string `json:"game"`
	Host  string `json:"host"`
	// Art is a small JPEG of the game (base64), or "".
	Art string `json:"art,omitempty"`
	// Own is set when the game is a set go-link made itself (verified by
	// its files' hashes, see ownsets).
	Own bool `json:"own,omitempty"`
}

// SetInfo tells guests the room's name, game, host and picture.
func (m *RoomManager) SetInfo(info RoomInfo) {
	m.do(func() {
		if m.info != info {
			m.info = info
			m.broadcastState()
		}
	})
}

// SetChat turns the room's chat on or off. Off, the device drops chat and
// typing messages (the web also hides the chat, but this is the rule).
func (m *RoomManager) SetChat(on bool) {
	m.do(func() {
		if m.chatOff != !on {
			m.chatOff = !on
			if !on {
				for _, mem := range m.members {
					mem.typingUntil = time.Time{}
				}
				m.broadcastTyping()
			}
			m.broadcastState()
		}
	})
}

// SetInputHUD tells the room's guests whether the controllers are drawn
// on the video (room_state input_hud), so each one measures its latency.
func (m *RoomManager) SetInputHUD(on bool) {
	m.do(func() {
		if m.inputHUD != on {
			m.inputHUD = on
			m.broadcastState()
		}
	})
}

// SetPicture sets the host's default picture style for the room, sent to
// guests in room_state.picture (nil or invalid: the website's default).
func (m *RoomManager) SetPicture(p *models.RoomPicture) {
	p = models.CleanPicture(p)
	m.do(func() {
		if (m.picture == nil) == (p == nil) && (p == nil || *m.picture == *p) {
			return
		}
		m.picture = p
		m.broadcastState()
	})
}

// OnPause registers who stops and resumes the emulator.
func (m *RoomManager) OnPause(fn func(paused bool)) {
	m.do(func() { m.onPause = fn })
}

// OnPauseAsk registers who tells the host's linked browsers about
// requests for a pause.
func (m *RoomManager) OnPauseAsk(fn func(PauseAskEvent)) {
	m.do(func() { m.onAsk = fn })
}

// OnRemove registers who takes a guest out of the room when the host asks
// (remove_guest): it stops the stream and ends the guest's return token.
func (m *RoomManager) OnRemove(fn func(peer string)) {
	m.do(func() { m.onRemove = fn })
}

// MarkOwner records that a peer of the room is the host's own browser (it
// came in with the owner key, or is the linked browser itself). Owners
// pause and resume the game and answer requests for a pause. Call it
// before or after Join; Leave forgets it.
func (m *RoomManager) MarkOwner(peerID string) {
	m.do(func() {
		if m.owners[peerID] {
			return
		}
		m.owners[peerID] = true
		m.reconcile() // an owner may seat every controller it has
	})
}

// SetHostLinked tells whether a linked browser of the host is connected
// to the device: the host can answer requests for a pause from there.
func (m *RoomManager) SetHostLinked(on bool) {
	m.do(func() {
		if m.hostLinked != on {
			m.hostLinked = on
			m.broadcastState()
		}
	})
}

// hostOnline reports whether the host can answer a request for a pause.
func (m *RoomManager) hostOnline() bool {
	if m.hostLinked {
		return true
	}
	for peer := range m.owners {
		if m.members[peer] != nil {
			return true
		}
	}
	return false
}

// GameControls is the control panel of the running game, so browsers can
// draw a matching on-screen gamepad.
type GameControls struct {
	Players int    `json:"players,omitempty"`
	Buttons int    `json:"buttons"`           // action buttons per player
	Control string `json:"control,omitempty"` // joy4way, joy8way, dial... from MAME
	// Labels say what each button does, button 1 first (go-link's own
	// games; MAME's list has no such names).
	Labels []string `json:"labels,omitempty"`
}

// TestCardControls is what the test card offers: every button lights up.
var TestCardControls = GameControls{Players: 4, Buttons: 6, Control: "joy8way"}

// SeatsFor is how many seats a room running a game with these controls has:
// the game's players, 1 to 4, or 4 when the game does not say.
func SeatsFor(c GameControls) int {
	if c.Players < 1 || c.Players > 4 {
		return 4
	}
	return c.Players
}

// seatsOrDefault is a summary's seat count, 4 before the first summary.
func seatsOrDefault(n int) int {
	if n < 1 {
		return 4
	}
	return n
}

// SetControls tells guests which controls the running game has.
func (m *RoomManager) SetControls(c GameControls) {
	m.do(func() {
		m.controls = c
		m.broadcastState()
	})
}

// SetPausable tells whether a game is running. Starting or stopping a
// game always resumes.
func (m *RoomManager) SetPausable(on bool) {
	m.do(func() {
		m.pausable = on
		m.setPaused(false, "")
		m.broadcastState()
	})
}

// setPaused changes the pause and tells the emulator. Actor goroutine.
// A pause answers every request for one.
func (m *RoomManager) setPaused(paused bool, by string) {
	if paused || !m.pausable {
		m.clearPauseAsks()
	}
	if m.paused == paused {
		return
	}
	m.paused, m.pausedBy = paused, by
	if !paused {
		m.pausedBy = ""
	}
	if m.onPause != nil {
		m.onPause(paused)
	}
}

// Pause pauses or resumes the game for everyone on behalf of the host
// (from the linked browser), with a line in the chat.
func (m *RoomManager) Pause(paused bool, by string) {
	m.do(func() { m.pause(paused, by) })
}

// pause is Pause on the actor goroutine.
func (m *RoomManager) pause(paused bool, by string) {
	if !m.pausable || paused == m.paused {
		return
	}
	m.setPaused(paused, by)
	if paused {
		m.event(EventGamePaused, fmt.Sprintf("%s paused the game", by), chatArgs{Name: by})
	} else {
		m.event(EventGameResumed, fmt.Sprintf("%s resumed the game", by), chatArgs{Name: by})
	}
	m.broadcastState()
}

// AnswerPause is the host's answer (from a linked browser) to a request
// for a pause from the guest peer "from". It reports whether there was
// such a request.
func (m *RoomManager) AnswerPause(from string, accept bool) bool {
	done := make(chan bool, 1)
	m.do(func() { done <- m.answerPause(from, accept, "The host") })
	return <-done
}

// askPause records (or refreshes, or withdraws) a seated player's request
// for a pause. Only while a game runs and is not paused.
func (m *RoomManager) askPause(mem *member, cancel bool) {
	i := slices.IndexFunc(m.pauseAsks, func(a pauseAsk) bool { return a.peer == mem.peer })
	if cancel {
		if i >= 0 {
			m.dropPauseAsk(i)
			m.broadcastState()
		}
		return
	}
	role, port := m.roleOf(mem)
	if !m.pausable || m.paused || !strings.HasPrefix(role, "P") || m.owners[mem.peer] {
		return
	}
	ask := pauseAsk{peer: mem.peer, name: mem.name, port: port, expires: m.cfg.Now().Add(pauseAskFor)}
	if i >= 0 {
		m.pauseAsks[i] = ask // asking again refreshes the time
	} else {
		m.pauseAsks = append(m.pauseAsks, ask)
	}
	peer, until := ask.peer, ask.expires
	time.AfterFunc(pauseAskFor, func() {
		m.do(func() {
			j := slices.IndexFunc(m.pauseAsks, func(a pauseAsk) bool { return a.peer == peer && a.expires.Equal(until) })
			if j >= 0 {
				m.dropPauseAsk(j)
				m.broadcastState()
			}
		})
	})
	if m.onAsk != nil {
		m.onAsk(PauseAskEvent{Type: "pause_asked", From: ask.peer, Name: ask.name, Port: ask.port, ExpiresAt: &ask.expires})
	}
	m.broadcastState()
}

// answerPause accepts or declines a request for a pause. Accepting pauses
// the game in the requester's name (and so answers every request);
// declining tells only the requester.
func (m *RoomManager) answerPause(from string, accept bool, host string) bool {
	i := slices.IndexFunc(m.pauseAsks, func(a pauseAsk) bool { return a.peer == from })
	if i < 0 {
		return false
	}
	ask := m.pauseAsks[i]
	if accept {
		if !m.pausable || m.paused {
			return false
		}
		// paused_by is who asked; the chat says the host agreed.
		m.setPaused(true, ask.name)
		m.event(EventGamePaused, fmt.Sprintf("%s asked for a pause and %s paused the game", ask.name, host),
			chatArgs{Name: ask.name, Name2: host})
		m.broadcastState()
		return true
	}
	m.dropPauseAsk(i)
	b, err := json.Marshal(chatOut{Type: "chat", System: "The host would rather keep playing", Event: EventPauseDeclined, Args: &chatArgs{Name: ask.name, Port: ask.port}, TS: m.cfg.Now().UnixMilli()})
	if err == nil {
		m.out.SendControl(ask.peer, b)
	}
	m.broadcastState()
	return true
}

// dropPauseAsk removes one request and tells the host's linked browsers.
func (m *RoomManager) dropPauseAsk(i int) {
	ask := m.pauseAsks[i]
	m.pauseAsks = slices.Delete(m.pauseAsks, i, i+1)
	if m.onAsk != nil {
		m.onAsk(PauseAskEvent{Type: "pause_ask_gone", From: ask.peer})
	}
}

// clearPauseAsks removes every request (the game was paused or ended).
func (m *RoomManager) clearPauseAsks() {
	for len(m.pauseAsks) > 0 {
		m.dropPauseAsk(len(m.pauseAsks) - 1)
	}
}

// errorOut is a refused request, for the guest who sent it.
type errorOut struct {
	Type  string `json:"type"`
	Code  string `json:"code"`
	Error string `json:"error"`
}

// refuse tells a guest its request was refused.
func (m *RoomManager) refuse(peer, code, text string) {
	if b, err := json.Marshal(errorOut{Type: "error", Code: code, Error: text}); err == nil {
		m.out.SendControl(peer, b)
	}
}

// Chat events: a machine-readable kind for some system lines, so the web
// can show them in the reader's language.
const (
	EventRecordingStarted = "recording_started"
	EventRecordingStopped = "recording_stopped"
	EventGamePaused       = "game_paused"    // name (and name2: the host, when name asked for it)
	EventGameResumed      = "game_resumed"   // name
	EventNowWatching      = "now_watching"   // name
	EventMoved            = "moved"          // name, port
	EventSwapAsked        = "swap_asked"     // name, port, name2, port2
	EventKeptSeat         = "kept_seat"      // name, port
	EventSwapped          = "swapped"        // name, port, name2, port2
	EventLeftSeat         = "left_seat"      // name, port
	EventSeatFree         = "seat_free"      // port
	EventTookSeat         = "took_seat"      // name, port
	EventSentToQueue      = "sent_to_queue"  // name (the host), name2, port
	EventSentToWatch      = "sent_to_watch"  // name (the host), name2, port
	EventLetQueue         = "let_queue"      // name (the host), name2: from watching to the queue
	EventPlayNow          = "play_now"       // name (the host), name2, port: seated at once
	EventFirstInQueue     = "first_in_queue" // name, port: gave the seat up for play_now
	EventRemoved          = "removed"        // name (the host), name2: taken out of the room
	// EventPauseDeclined goes only to the guest whose request for a pause
	// the host declined (name, port: the requester's own).
	EventPauseDeclined = "pause_declined"
	// EventHandLater goes only to the spectator whose raised hand the host
	// answered with "not now".
	EventHandLater = "hand_later"
)

// chatArgs are the values of a chat event, for the web to put into the
// reader's language.
type chatArgs struct {
	Name  string `json:"name,omitempty"`
	Port  int    `json:"port,omitempty"`
	Name2 string `json:"name2,omitempty"`
	Port2 int    `json:"port2,omitempty"`
}

// SetRecording tells everyone in the room whether the host records the
// game: room_state.recording (a REC badge) and a line in the chat, since
// the players' voices are recorded too.
func (m *RoomManager) SetRecording(on bool) {
	m.do(func() {
		if m.recording == on {
			return
		}
		m.recording = on
		msg := chatOut{Type: "chat", TS: m.cfg.Now().UnixMilli()}
		if on {
			msg.System, msg.Event = "The host is recording this game, with the players' voices", EventRecordingStarted
		} else {
			msg.System, msg.Event = "The recording stopped", EventRecordingStopped
		}
		m.publish(msg)
		m.broadcastState()
	})
}

// Reset removes everyone (the signaling link was lost).
func (m *RoomManager) Reset() {
	m.do(func() {
		m.members = make(map[string]*member)
		m.history = nil
		m.reconcile()
	})
}

type controlIn struct {
	Type         string `json:"type"`
	Name         string `json:"name"`
	LocalPlayers []int  `json:"local_players"`
	Text         string `json:"text"`
	Paused       bool   `json:"paused"`
	// From is a port (swap_seat, swap_answer) or a peer id (pause_answer).
	From   json.RawMessage `json:"from"`
	To     int             `json:"to"`
	Accept bool            `json:"accept"`
	On     bool            `json:"on"`
	Cancel bool            `json:"cancel"`
	// release_seat: the seat and where its player goes ("queue" or "watch");
	// promote: "queue" or "play" (at Port).
	Port int    `json:"port"`
	Mode string `json:"mode"`
}

// fromPort reads "from" as a port (0 when it is not a number).
func (c controlIn) fromPort() int {
	var n int
	_ = json.Unmarshal(c.From, &n)
	return n
}

// fromPeer reads "from" as a peer id ("" when it is not text).
func (c controlIn) fromPeer() string {
	var s string
	_ = json.Unmarshal(c.From, &s)
	return s
}

// HandleControl processes a JSON message from a guest.
func (m *RoomManager) HandleControl(peerID string, data []byte) {
	var msg controlIn
	if len(data) > 4096 || json.Unmarshal(data, &msg) != nil {
		return
	}
	m.do(func() {
		mem := m.members[peerID]
		if mem == nil {
			return
		}
		if now := m.cfg.Now(); now.Sub(mem.controlAt) >= time.Second {
			mem.controlAt, mem.controlN = now, 0
		}
		if mem.controlN++; mem.controlN > controlBurst {
			return
		}
		switch msg.Type {
		case "hello":
			if name := CleanName(msg.Name); name != "" && name != mem.name {
				mem.name = name
				m.teleLog("name", peerID, "is called "+name, map[string]any{"name": name})
			}
			if locals := cleanLocals(msg.LocalPlayers); len(locals) > 0 {
				mem.locals = locals
			}
			m.reconcile()
		case "chat":
			if m.chatOff {
				return
			}
			m.chat(mem, msg.Text)
			m.setTyping(mem, false)
		case "typing":
			if !m.chatOff {
				m.setTyping(mem, msg.On)
			}
		case "spectate":
			if !mem.spectator {
				mem.spectator = true
				m.event(EventNowWatching, fmt.Sprintf("%s is now watching", mem.name), chatArgs{Name: mem.name})
				m.reconcile()
			}
		case "queue":
			if mem.spectator {
				if mem.held && !m.owners[peerID] {
					m.refuse(peerID, "queue_ask_host", "The host moved you to watching: raise your hand to play")
					return
				}
				mem.spectator = false
				m.reconcile()
			}
		case "hand":
			m.raiseHand(mem, msg.On)
		case "hand_later":
			if m.owners[peerID] {
				m.handLater(msg.fromPeer())
			}
		case "promote":
			// Only the host lets a spectator into the queue or seats someone
			// at once (a friend: "play now").
			if !m.owners[peerID] {
				m.refuse(peerID, "promote_owner_only", "Only the host lets people play")
				return
			}
			m.promote(mem.name, msg.fromPeer(), msg.Mode, msg.Port)
		case "remove_guest":
			if !m.owners[peerID] {
				m.refuse(peerID, "remove_owner_only", "Only the host takes someone out of the room")
				return
			}
			m.removeGuest(mem.name, msg.fromPeer())
		case "pause":
			// The game is the host's: only the host's own browsers pause
			// it; the others ask (pause_request).
			if !m.owners[peerID] {
				m.refuse(peerID, "pause_owner_only", "Only the host pauses the game: ask for a pause")
				return
			}
			m.pause(msg.Paused, mem.name)
		case "pause_request":
			m.askPause(mem, msg.Cancel)
		case "pause_answer":
			if m.owners[peerID] {
				m.answerPause(msg.fromPeer(), msg.Accept, mem.name)
			}
		case "release_seat":
			// Only the host frees someone's seat (its own included).
			if !m.owners[peerID] {
				m.refuse(peerID, "release_owner_only", "Only the host frees a seat")
				return
			}
			m.releaseSeat(mem.name, msg.Port, msg.Mode)
		case "swap_seat":
			m.askSwap(peerID, msg.fromPort(), msg.To)
		case "swap_answer":
			m.answerSwap(peerID, msg.fromPort(), msg.To, msg.Accept)
		}
	})
}

// releaseSeat frees seat port for the next person: its player goes to the
// end of the queue (mode "queue") or watches (mode "watch"). Sending to the
// queue needs someone else waiting, or the same player would take the seat
// back at once; then it is like watching.
func (m *RoomManager) releaseSeat(host string, port int, mode string) {
	if port < 1 || port > len(m.seats) || m.seats[port-1] == nil {
		return
	}
	k := *m.seats[port-1]
	mem := m.members[k.peer]
	if mem == nil {
		return
	}
	others := slices.ContainsFunc(m.queue, func(q seatKey) bool { return q.peer != k.peer })
	name := m.displayName(k)
	if mode == "queue" && others {
		m.seats[port-1] = nil
		m.queue = append(slices.DeleteFunc(m.queue, func(q seatKey) bool { return q == k }), k)
		m.event(EventSentToQueue, fmt.Sprintf("%s sent %s from P%d to the queue", host, name, port), chatArgs{Name: host, Name2: name, Port: port})
		m.teleLog("seat_released", k.peer, fmt.Sprintf("%s sent %s from P%d to the queue", host, name, port), map[string]any{"port": port, "mode": "queue"})
	} else {
		mem.spectator = true
		mem.held = true // back to playing through the host: a raised hand
		// The seat is freed here, as in the queue mode: reconcile would add a
		// "left P<port>" line to the chat that says the same thing again.
		m.seats[port-1] = nil
		if j := slices.IndexFunc(m.pauseAsks, func(a pauseAsk) bool { return a.peer == k.peer }); j >= 0 {
			m.dropPauseAsk(j)
		}
		m.event(EventSentToWatch, fmt.Sprintf("%s moved %s from P%d to watching", host, name, port), chatArgs{Name: host, Name2: name, Port: port})
		m.teleLog("seat_released", k.peer, fmt.Sprintf("%s moved %s from P%d to watching", host, name, port), map[string]any{"port": port, "mode": "watch"})
	}
	m.reconcile()
}

// raiseHand raises (or lowers) a spectator's hand: the host sees it and
// answers with promote (queue or play) or hand_later. One raise per
// handEvery, and only while the host can see it.
func (m *RoomManager) raiseHand(mem *member, up bool) {
	if !up {
		if !mem.hand.IsZero() {
			mem.hand = time.Time{}
			m.broadcastState()
		}
		return
	}
	if !mem.spectator || m.owners[mem.peer] || !mem.hand.IsZero() {
		return
	}
	if !m.hostOnline() {
		m.refuse(mem.peer, "hand_no_host", "The host is not here to see your hand")
		return
	}
	now := m.cfg.Now()
	if !mem.handAt.IsZero() && now.Sub(mem.handAt) < handEvery {
		m.refuse(mem.peer, "hand_too_soon", "You can raise your hand once a minute")
		return
	}
	mem.hand, mem.handAt = now, now
	m.teleLog("hand", mem.peer, "raised a hand", map[string]any{"name": mem.name})
	m.broadcastState()
}

// handLater is the host's "not now" to a raised hand: the hand goes down
// and only that guest is told, kindly.
func (m *RoomManager) handLater(peer string) {
	mem := m.members[peer]
	if mem == nil || mem.hand.IsZero() {
		return
	}
	mem.hand = time.Time{}
	b, err := json.Marshal(chatOut{Type: "chat", System: "The host will let you play later", Event: EventHandLater, Args: &chatArgs{Name: mem.name}, TS: m.cfg.Now().UnixMilli()})
	if err == nil {
		m.out.SendControl(peer, b)
	}
	m.broadcastState()
}

// promote is the host moving someone toward playing: a spectator to the
// end of the queue (mode "queue"), or a spectator or someone in the queue
// straight to seat port (mode "play", a friend who should not wait). A
// taken seat's player goes first in the queue, so nobody loses a turn.
func (m *RoomManager) promote(host, peer, mode string, port int) {
	mem := m.members[peer]
	if mem == nil {
		return
	}
	switch mode {
	case "queue":
		if !mem.spectator {
			return
		}
		mem.spectator, mem.held = false, false
		m.event(EventLetQueue, fmt.Sprintf("%s let %s into the queue", host, mem.name), chatArgs{Name: host, Name2: mem.name})
		m.teleLog("promote", peer, "to the queue", map[string]any{"name": mem.name, "mode": "queue"})
		m.reconcile()
	case "play":
		if port < 1 || port > len(m.seats) {
			return
		}
		if cur := m.seats[port-1]; cur != nil && cur.peer == peer {
			return // already there
		}
		i := slices.IndexFunc(m.seatable(mem), func(l uint8) bool { return m.portIndex(seatKey{peer, l}) < 0 })
		if i < 0 {
			return // every controller of that browser already plays
		}
		k := seatKey{peer, m.seatable(mem)[i]}
		mem.spectator, mem.held = false, false
		m.queue = slices.DeleteFunc(m.queue, func(q seatKey) bool { return q == k })
		old := m.seats[port-1]
		m.seats[port-1] = &k
		m.event(EventPlayNow, fmt.Sprintf("%s seated %s at P%d", host, m.displayName(k), port), chatArgs{Name: host, Name2: m.displayName(k), Port: port})
		m.teleLog("promote", peer, fmt.Sprintf("to P%d", port), map[string]any{"name": mem.name, "mode": "play", "port": port})
		if old != nil {
			m.queue = append([]seatKey{*old}, slices.DeleteFunc(m.queue, func(q seatKey) bool { return q == *old })...)
			if !slices.ContainsFunc(m.seats, func(s *seatKey) bool { return s != nil && s.peer == old.peer }) {
				if j := slices.IndexFunc(m.pauseAsks, func(a pauseAsk) bool { return a.peer == old.peer }); j >= 0 {
					m.dropPauseAsk(j)
				}
			}
			m.event(EventFirstInQueue, fmt.Sprintf("%s goes first in the queue", m.displayName(*old)), chatArgs{Name: m.displayName(*old), Port: port})
		}
		m.reconcile()
	}
}

// removeGuest takes a guest out of the room for good: the host's own
// browsers stay. The stream stops and the guest's way back in ends
// (OnRemove).
func (m *RoomManager) removeGuest(host, peer string) {
	mem := m.members[peer]
	if mem == nil || m.owners[peer] {
		return
	}
	delete(m.members, peer)
	m.event(EventRemoved, fmt.Sprintf("%s took %s out of the room", host, mem.name), chatArgs{Name: host, Name2: mem.name})
	m.teleLog("removed", peer, "taken out of the room", map[string]any{"name": mem.name})
	m.reconcile()
	if m.cfg.Now().Before(mem.typingUntil) {
		m.broadcastTyping()
	}
	if fn := m.onRemove; fn != nil {
		go fn(peer)
	}
}

// askSwap moves a seated player from port from to port to. A free port is
// taken at once, and so is a port of the same browser; a port of someone
// else waits for their answer (swap_answer), for swapTimeout at most.
func (m *RoomManager) askSwap(peer string, from, to int) {
	if from < 1 || to < 1 || from > len(m.seats) || to > len(m.seats) || from == to {
		return
	}
	asker := m.seats[from-1]
	if asker == nil || asker.peer != peer {
		return
	}
	target := m.seats[to-1]
	if target == nil {
		m.seats[to-1], m.seats[from-1] = asker, nil
		m.event(EventMoved, fmt.Sprintf("%s moved to P%d", m.displayName(*asker), to), chatArgs{Name: m.displayName(*asker), Port: to})
		m.reconcile()
		return
	}
	// The same browser, or the other player already asked for this very
	// swap: both want it, so no need to ask again.
	crossed := slices.ContainsFunc(m.swaps, func(p pendingSwap) bool {
		return p.from == to && p.to == from && m.swapValid(p)
	})
	if target.peer == peer || crossed {
		m.swapSeats(from, to)
		m.reconcile()
		return
	}
	// One request per port: a new one replaces the old.
	m.swaps = slices.DeleteFunc(m.swaps, func(p pendingSwap) bool { return p.from == from })
	m.nextSwap++
	req := pendingSwap{id: m.nextSwap, from: from, to: to, asker: *asker, target: *target}
	m.swaps = append(m.swaps, req)
	m.event(EventSwapAsked, fmt.Sprintf("%s (P%d) asks %s (P%d) to swap controllers", m.displayName(*asker), from, m.displayName(*target), to),
		chatArgs{Name: m.displayName(*asker), Port: from, Name2: m.displayName(*target), Port2: to})
	time.AfterFunc(swapTimeout, func() {
		m.do(func() {
			n := len(m.swaps)
			m.swaps = slices.DeleteFunc(m.swaps, func(p pendingSwap) bool { return p.id == req.id })
			if len(m.swaps) != n {
				m.broadcastState()
			}
		})
	})
	m.broadcastState()
}

// answerSwap is the reply of the player asked to swap.
func (m *RoomManager) answerSwap(peer string, from, to int, accept bool) {
	i := slices.IndexFunc(m.swaps, func(p pendingSwap) bool { return p.from == from && p.to == to && p.target.peer == peer })
	if i < 0 {
		return
	}
	req := m.swaps[i]
	m.swaps = slices.Delete(m.swaps, i, i+1)
	if !m.swapValid(req) {
		m.broadcastState()
		return
	}
	if !accept {
		m.event(EventKeptSeat, fmt.Sprintf("%s kept P%d", m.displayName(req.target), to), chatArgs{Name: m.displayName(req.target), Port: to})
		m.broadcastState()
		return
	}
	m.swapSeats(from, to)
	m.reconcile()
}

// swapSeats exchanges the players of two ports and says so in the chat.
func (m *RoomManager) swapSeats(a, b int) {
	m.seats[a-1], m.seats[b-1] = m.seats[b-1], m.seats[a-1]
	m.swaps = slices.DeleteFunc(m.swaps, func(p pendingSwap) bool { return !m.swapValid(p) })
	x, y := m.seats[a-1], m.seats[b-1]
	if x != nil && y != nil {
		m.event(EventSwapped, fmt.Sprintf("%s is now P%d and %s is P%d", m.displayName(*y), b, m.displayName(*x), a),
			chatArgs{Name: m.displayName(*y), Port: b, Name2: m.displayName(*x), Port2: a})
	}
}

// swapValid reports whether both players still sit where they sat when
// the swap was asked.
func (m *RoomManager) swapValid(p pendingSwap) bool {
	f, t := m.seats[p.from-1], m.seats[p.to-1]
	return f != nil && t != nil && *f == p.asker && *t == p.target
}

// reconcile brings seats and queue in line with the members, fills free
// ports from the queue, publishes the seat map and notifies everyone.
func (m *RoomManager) reconcile() {
	wants := func(k seatKey) bool {
		mem := m.members[k.peer]
		return mem != nil && !mem.spectator && slices.Contains(m.seatable(mem), k.local)
	}
	for i, s := range m.seats {
		if s != nil && !wants(*s) {
			// Without a seat, a request for a pause makes no sense.
			if j := slices.IndexFunc(m.pauseAsks, func(a pauseAsk) bool { return a.peer == s.peer }); j >= 0 {
				m.dropPauseAsk(j)
			}
			if mem := m.members[s.peer]; mem != nil {
				m.event(EventLeftSeat, fmt.Sprintf("%s left P%d", mem.name, i+1), chatArgs{Name: mem.name, Port: i + 1})
			} else {
				m.event(EventSeatFree, fmt.Sprintf("P%d is free", i+1), chatArgs{Port: i + 1})
			}
			m.seats[i] = nil
		}
	}
	m.queue = slices.DeleteFunc(m.queue, func(k seatKey) bool { return !wants(k) })
	for _, mem := range m.members {
		if !mem.spectator {
			mem.hand = time.Time{} // a hand asks to play: only spectators raise one
		}
	}

	// New wishes join the queue in arrival order.
	ordered := make([]*member, 0, len(m.members))
	for _, mem := range m.members {
		ordered = append(ordered, mem)
	}
	slices.SortFunc(ordered, func(a, b *member) int { return a.order - b.order })
	for _, mem := range ordered {
		if mem.spectator {
			continue
		}
		for _, l := range m.seatable(mem) {
			k := seatKey{mem.peer, l}
			if m.portIndex(k) < 0 && !slices.Contains(m.queue, k) {
				m.queue = append(m.queue, k)
			}
		}
	}

	// Free ports go to the head of the queue: like at the arcade, the
	// next in line takes the joystick that was just released.
	for i := range m.seats {
		if m.seats[i] == nil && len(m.queue) > 0 {
			k := m.queue[0]
			m.queue = m.queue[1:]
			m.seats[i] = &k
			m.event(EventTookSeat, fmt.Sprintf("%s took seat P%d", m.displayName(k), i+1), chatArgs{Name: m.displayName(k), Port: i + 1})
		}
	}

	ports := make(map[seatKey]int)
	for i, s := range m.seats {
		if s != nil {
			ports[*s] = i + 1
		}
	}
	m.ports.Store(&ports)
	m.swaps = slices.DeleteFunc(m.swaps, func(p pendingSwap) bool { return !m.swapValid(p) })
	m.broadcastState()
	if m.cfg.OnSummary != nil {
		m.cfg.OnSummary(m.summary())
	}
}

// seatable is the local players of a member that may take seats: all of
// them for the host's own browsers, only the first for a guest (one person,
// one seat, however many controllers that browser has).
func (m *RoomManager) seatable(mem *member) []uint8 {
	if m.owners[mem.peer] || len(mem.locals) <= 1 {
		return mem.locals
	}
	return mem.locals[:1]
}

func (m *RoomManager) portIndex(k seatKey) int {
	for i, s := range m.seats {
		if s != nil && *s == k {
			return i
		}
	}
	return -1
}

func (m *RoomManager) displayName(k seatKey) string {
	mem := m.members[k.peer]
	if mem == nil {
		return "?"
	}
	if len(mem.locals) > 1 {
		return fmt.Sprintf("%s (%d)", mem.name, k.local+1)
	}
	return mem.name
}

func (m *RoomManager) summary() RoomSummary {
	s := RoomSummary{MaxPlayers: m.cfg.MaxPlayers, Queue: len(m.queue)}
	for _, seat := range m.seats {
		if seat != nil {
			s.Players++
		}
	}
	for _, mem := range m.members {
		if mem.spectator {
			s.Spectators++
		}
		ms := MemberSummary{Peer: mem.peer, Name: mem.name}
		for i, seat := range m.seats {
			if seat != nil && seat.peer == mem.peer {
				ms.Ports = append(ms.Ports, i+1)
			}
		}
		s.Members = append(s.Members, ms)
	}
	slices.SortFunc(s.Members, func(a, b MemberSummary) int {
		return m.members[a.Peer].order - m.members[b.Peer].order
	})
	return s
}

// Peer (in seats, queue and spectators) is the person's peer id, sent only
// to the host's own browsers: what promote, hand_later and remove_guest name.
type seatOut struct {
	Port        int    `json:"port"`
	Name        string `json:"name"`
	LocalPlayer int    `json:"local_player"`
	You         bool   `json:"you"`
	Peer        string `json:"peer,omitempty"`
}

type queueOut struct {
	Position int    `json:"position"`
	Name     string `json:"name"`
	You      bool   `json:"you"`
	Peer     string `json:"peer,omitempty"`
}

type personOut struct {
	Name string `json:"name"`
	You  bool   `json:"you"`
	Peer string `json:"peer,omitempty"`
	// Hand: this spectator raised a hand to play (everyone sees it); HandAt
	// (owners only) orders the hands.
	Hand   bool       `json:"hand,omitempty"`
	HandAt *time.Time `json:"hand_at,omitempty"`
}

type youOut struct {
	Name           string `json:"name"`
	Ports          []int  `json:"ports"`
	QueuePositions []int  `json:"queue_positions"`
	Spectator      bool   `json:"spectator"`
	// SwapOffers are requests to swap controllers waiting for your answer;
	// SwapAsked are yours waiting for someone else's.
	SwapOffers []swapOut `json:"swap_offers"`
	SwapAsked  []swapOut `json:"swap_asked"`
	// Owner: this browser is the host's. PauseAsks (owners only) are the
	// requests for a pause waiting for its answer; PauseAsked is this
	// guest's own request, or null.
	Owner      bool           `json:"owner"`
	PauseAsks  *[]pauseAskOut `json:"pause_asks,omitempty"`
	PauseAsked *pauseAskedOut `json:"pause_asked"`
	// Hand: your hand is up. HandAgain: when you may raise it again (only
	// while that is still ahead). Held: the host moved you to watching, so
	// you raise a hand instead of joining the queue alone.
	Hand      bool       `json:"hand"`
	HandAgain *time.Time `json:"hand_again,omitempty"`
	Held      bool       `json:"held"`
}

type pauseAskOut struct {
	From      string    `json:"from"`
	Name      string    `json:"name"`
	Port      int       `json:"port"`
	ExpiresAt time.Time `json:"expires_at"`
}

type pauseAskedOut struct {
	ExpiresAt time.Time `json:"expires_at"`
}

type swapOut struct {
	From int    `json:"from"`
	To   int    `json:"to"`
	Name string `json:"name"` // the other player
}

type stateOut struct {
	Type       string              `json:"type"`
	MaxPlayers int                 `json:"max_players"`
	Voice      bool                `json:"voice"`
	Chat       bool                `json:"chat"`
	InputHUD   bool                `json:"input_hud,omitempty"`
	Picture    *models.RoomPicture `json:"picture,omitempty"`
	Info       RoomInfo            `json:"info"`
	Seats      []*seatOut          `json:"seats"`
	Queue      []queueOut          `json:"queue"`
	Spectators []personOut         `json:"spectators"`
	You        youOut              `json:"you"`
	Pausable   bool                `json:"pausable"`
	Paused     bool                `json:"paused"`
	PausedBy   string              `json:"paused_by,omitempty"`
	Controls   GameControls        `json:"controls"`
	Recording  bool                `json:"recording"`
	HostOnline bool                `json:"host_online"`
}

// broadcastState sends each member its own view of the room.
func (m *RoomManager) broadcastState() {
	online := m.hostOnline()
	now := m.cfg.Now()
	var asks []pauseAskOut
	for _, a := range m.pauseAsks {
		asks = append(asks, pauseAskOut{From: a.peer, Name: a.name, Port: a.port, ExpiresAt: a.expires})
	}
	for peer, mem := range m.members {
		st := stateOut{Type: "room_state", MaxPlayers: m.cfg.MaxPlayers, Voice: !m.voiceOff, Chat: !m.chatOff, InputHUD: m.inputHUD, Picture: m.picture, Info: m.info, Seats: make([]*seatOut, len(m.seats)), Queue: []queueOut{}, Spectators: []personOut{},
			Pausable: m.pausable, Paused: m.paused, PausedBy: m.pausedBy, Controls: m.controls, Recording: m.recording, HostOnline: online}
		owner := m.owners[peer]
		peerFor := func(p string) string {
			if owner {
				return p
			}
			return ""
		}
		st.You = youOut{Name: mem.name, Ports: []int{}, QueuePositions: []int{}, Spectator: mem.spectator, SwapOffers: []swapOut{}, SwapAsked: []swapOut{}, Owner: owner,
			Hand: !mem.hand.IsZero(), Held: mem.held && mem.spectator}
		if again := mem.handAt.Add(handEvery); !mem.handAt.IsZero() && now.Before(again) {
			st.You.HandAgain = &again
		}
		if owner {
			list := append([]pauseAskOut{}, asks...)
			st.You.PauseAsks = &list
		}
		for _, a := range m.pauseAsks {
			if a.peer == peer {
				st.You.PauseAsked = &pauseAskedOut{ExpiresAt: a.expires}
			}
		}
		for _, p := range m.swaps {
			if p.target.peer == peer {
				st.You.SwapOffers = append(st.You.SwapOffers, swapOut{From: p.from, To: p.to, Name: m.displayName(p.asker)})
			}
			if p.asker.peer == peer {
				st.You.SwapAsked = append(st.You.SwapAsked, swapOut{From: p.from, To: p.to, Name: m.displayName(p.target)})
			}
		}
		for i, s := range m.seats {
			if s == nil {
				continue
			}
			you := s.peer == peer
			st.Seats[i] = &seatOut{Port: i + 1, Name: m.displayName(*s), LocalPlayer: int(s.local), You: you, Peer: peerFor(s.peer)}
			if you {
				st.You.Ports = append(st.You.Ports, i+1)
			}
		}
		for i, k := range m.queue {
			you := k.peer == peer
			st.Queue = append(st.Queue, queueOut{Position: i + 1, Name: m.displayName(k), You: you, Peer: peerFor(k.peer)})
			if you {
				st.You.QueuePositions = append(st.You.QueuePositions, i+1)
			}
		}
		for _, other := range m.members {
			if other.spectator {
				p := personOut{Name: other.name, You: other.peer == peer, Peer: peerFor(other.peer), Hand: !other.hand.IsZero()}
				if owner && p.Hand {
					at := other.hand
					p.HandAt = &at
				}
				st.Spectators = append(st.Spectators, p)
			}
		}
		slices.SortFunc(st.Spectators, func(a, b personOut) int { return strings.Compare(a.Name, b.Name) })
		b, err := json.Marshal(st)
		if err == nil {
			m.out.SendControl(peer, b)
		}
	}
}

type chatOut struct {
	Type   string    `json:"type"`
	Name   string    `json:"name,omitempty"`
	Port   int       `json:"port,omitempty"`
	Role   string    `json:"role,omitempty"`
	Text   string    `json:"text,omitempty"`
	System string    `json:"system,omitempty"`
	Event  string    `json:"event,omitempty"` // EventRecordingStarted...
	Args   *chatArgs `json:"args,omitempty"`  // the event's values
	TS     int64     `json:"ts"`
}

func (m *RoomManager) chat(mem *member, raw string) {
	text := cleanText(raw, maxChatLen)
	if text == "" {
		return
	}
	now := m.cfg.Now()
	mem.chatTimes = slices.DeleteFunc(mem.chatTimes, func(t time.Time) bool { return now.Sub(t) > chatWindow })
	if len(mem.chatTimes) >= chatBurst {
		b, _ := json.Marshal(chatOut{Type: "chat", System: "You are sending messages too fast.", TS: now.UnixMilli()})
		m.out.SendControl(mem.peer, b)
		return
	}
	mem.chatTimes = append(mem.chatTimes, now)
	msg := chatOut{Type: "chat", Name: mem.name, Text: text, TS: now.UnixMilli()}
	msg.Role, msg.Port = m.roleOf(mem)
	m.publish(msg)
}

type typingOut struct {
	Name string `json:"name"`
	Port int    `json:"port,omitempty"`
}

// setTyping marks a member as typing a chat message (or not) and tells the
// others when that changes. The mark expires by itself after typingFor.
func (m *RoomManager) setTyping(mem *member, on bool) {
	now := m.cfg.Now()
	was := now.Before(mem.typingUntil)
	if on {
		mem.typingUntil = now.Add(typingFor)
		peer, until := mem.peer, mem.typingUntil
		time.AfterFunc(typingFor, func() {
			m.do(func() {
				if cur := m.members[peer]; cur != nil && cur.typingUntil.Equal(until) {
					cur.typingUntil = time.Time{}
					m.broadcastTyping()
				}
			})
		})
	} else {
		mem.typingUntil = time.Time{}
	}
	if was != on {
		m.broadcastTyping()
	}
}

// broadcastTyping sends each member who else is typing (never themselves).
func (m *RoomManager) broadcastTyping() {
	now := m.cfg.Now()
	var typing []*member
	for _, mem := range m.members {
		if now.Before(mem.typingUntil) {
			typing = append(typing, mem)
		}
	}
	slices.SortFunc(typing, func(a, b *member) int { return a.order - b.order })
	for peer := range m.members {
		out := struct {
			Type  string      `json:"type"`
			Names []typingOut `json:"names"`
		}{Type: "typing", Names: []typingOut{}}
		for _, mem := range typing {
			if mem.peer != peer {
				_, port := m.roleOf(mem)
				out.Names = append(out.Names, typingOut{Name: mem.name, Port: port})
			}
		}
		if b, err := json.Marshal(out); err == nil {
			m.out.SendControl(peer, b)
		}
	}
}

// roleOf describes a member for the chat: "P1", "queue 2nd" or "spectator".
func (m *RoomManager) roleOf(mem *member) (string, int) {
	for i, s := range m.seats {
		if s != nil && s.peer == mem.peer {
			return fmt.Sprintf("P%d", i+1), i + 1
		}
	}
	for i, k := range m.queue {
		if k.peer == mem.peer {
			return "queue " + ordinal(i+1), 0
		}
	}
	return "spectator", 0
}

// event posts a system line: the English text, plus its kind and values
// so the web shows it in the reader's language.
func (m *RoomManager) event(kind, text string, args chatArgs) {
	m.teleLog(kind, "", text, map[string]any{"name": args.Name, "port": args.Port})
	m.publish(chatOut{Type: "chat", System: text, Event: kind, Args: &args, TS: m.cfg.Now().UnixMilli()})
}

// teleLog hands an event to the room's telemetry.
func (m *RoomManager) teleLog(kind, peer, text string, data map[string]any) {
	if m.cfg.OnLog != nil {
		m.cfg.OnLog(kind, peer, text, data)
	}
}

// publish sends a chat line to everyone and keeps it in the history.
func (m *RoomManager) publish(msg chatOut) {
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}
	m.history = append(m.history, b)
	if len(m.history) > chatHistory {
		m.history = m.history[len(m.history)-chatHistory:]
	}
	for peer := range m.members {
		m.out.SendControl(peer, b)
	}
}

// cleanText trims, drops control characters and limits the length.
func cleanText(s string, limit int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > limit {
		s = string([]rune(s)[:limit])
	}
	return s
}

// CleanName makes a player's name safe to show: it keeps only letters and
// digits (any language, accents and ñ included) and spaces, joins runs of
// spaces into one, trims it and cuts it to maxNameLen characters. A name
// with fewer than minNameLen characters left returns "" (the guest keeps
// its generated name). The device never trusts the name a browser sends.
func CleanName(s string) string {
	s = norm.NFC.String(s)
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		case unicode.IsSpace(r):
			space = true
		}
	}
	out := b.String()
	if utf8.RuneCountInString(out) > maxNameLen {
		out = strings.TrimSpace(string([]rune(out)[:maxNameLen]))
	}
	if utf8.RuneCountInString(out) < minNameLen {
		return ""
	}
	return out
}

// cleanLocals keeps valid, distinct local player numbers.
func cleanLocals(in []int) []uint8 {
	var out []uint8
	for _, v := range in {
		if v >= 0 && v < input.MaxLocalPlayers && !slices.Contains(out, uint8(v)) {
			out = append(out, uint8(v))
		}
	}
	slices.Sort(out)
	return out
}

func defaultName(peerID string) string {
	// The local panel's peers are "panel-<hex>": name them by the hex.
	short := strings.TrimPrefix(peerID, "panel-")
	if len(short) > 4 {
		short = short[:4]
	}
	return "Guest " + strings.ToUpper(short)
}

func ordinal(n int) string {
	switch {
	case n%100 >= 11 && n%100 <= 13:
		return fmt.Sprintf("%dth", n)
	case n%10 == 1:
		return fmt.Sprintf("%dst", n)
	case n%10 == 2:
		return fmt.Sprintf("%dnd", n)
	case n%10 == 3:
		return fmt.Sprintf("%drd", n)
	}
	return fmt.Sprintf("%dth", n)
}
