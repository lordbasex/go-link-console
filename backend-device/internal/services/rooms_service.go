// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/lordbasex/go-link/backend-device/internal/models"
	"github.com/lordbasex/go-link/backend-device/internal/telemetry"
	"github.com/lordbasex/go-link/backend-device/pkg/libretro"
	"github.com/lordbasex/go-link/backend-device/pkg/romcheck"
	"github.com/lordbasex/go-link/backend-device/pkg/signalclient"
	"github.com/lordbasex/go-link/backend-device/pkg/thumbnails"
)

// Errors returned when a game room cannot be created or changed.
var (
	ErrNoCore       = errors.New("the emulator core is not installed")
	ErrUnknownRom   = errors.New("that ROM is not in the library")
	ErrUnknownRoom  = errors.New("that room does not exist")
	ErrTooManyRooms = errors.New("the device runs as many games as it can at once")
	ErrRoomState    = errors.New("the room cannot do that in its current state")
	ErrTooManySaves = fmt.Errorf("a room keeps up to %d saved games: delete one first", MaxSaveSlots)
	// ErrNoSaves: the emulator does not save this game whole (SavedRoom.NoSaves).
	ErrNoSaves = errors.New("this game cannot be saved in this emulator: it always starts from the beginning")
	// ErrBadPicture: a picture style or sides the website does not know.
	ErrBadPicture = errors.New("unknown picture style or sides")
)

// MaxSaveSlots caps the saved games of one room, so saves cannot fill the
// disk.
const MaxSaveSlots = 20

// GameRequest asks to open a room playing a ROM.
type GameRequest struct {
	Rom    string `json:"rom"`
	Title  string `json:"title"`
	Public bool   `json:"public"`
	Voice  bool   `json:"voice"`
	Art    string `json:"art,omitempty"` // lobby picture: boxart, title or snap ("" = Settings)
	// Pin for a private room (6 digits); empty picks a random one.
	// Chat false turns the room's chat off; absent (older clients) is on.
	Chat *bool `json:"chat,omitempty"`
	// Picture is the room's default picture style for its guests; absent
	// or with unknown values, the website's own default.
	Picture *models.RoomPicture `json:"picture,omitempty"`
	// Maker names the Willy Maker game and its buttons (rom MakerRom only).
	Maker *MakerInfo `json:"maker,omitempty"`
}

// TooManyRoomsError says the device already runs its limit of games.
// Paused games count: they keep their emulator in memory. It matches
// ErrTooManyRooms with errors.Is.
type TooManyRoomsError struct{ Limit int }

func (e *TooManyRoomsError) Error() string {
	return fmt.Sprintf("this device runs at most %d games at once (live or paused) and all %d are on: archive a room to turn it off, or raise max_rooms in device.json", e.Limit, e.Limit)
}

// Is makes errors.Is(err, ErrTooManyRooms) true.
func (e *TooManyRoomsError) Is(target error) bool { return target == ErrTooManyRooms }

// ErrorCode is a stable code for errors the web explains in its own
// words, plus the limit for "too_many_rooms".
func ErrorCode(err error) (code string, limit int) {
	var tooMany *TooManyRoomsError
	if errors.As(err, &tooMany) {
		return "too_many_rooms", tooMany.Limit
	}
	if errors.Is(err, ErrNoSaves) {
		return "no_saves", 0
	}
	switch {
	case errors.Is(err, ErrRecordPaused):
		return "record_paused", 0
	case errors.Is(err, ErrRecording):
		return "already_recording", 0
	case errors.Is(err, ErrNotRecording):
		return "not_recording", 0
	case errors.Is(err, ErrUnknownRecording):
		return "unknown_recording", 0
	}
	return "", 0
}

// GameReply is sent back to the owner who asked.
type GameReply struct {
	Type   string `json:"type"` // "room_created", "room_error" or "room_closed"
	ID     string `json:"id,omitempty"`
	RoomID string `json:"room_id,omitempty"`
	Rom    string `json:"rom,omitempty"`
	Title  string `json:"title,omitempty"`
	Error  string `json:"error,omitempty"`
	// Code and Limit come from ErrorCode.
	Code  string `json:"code,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

// RoomSource is a game that runs in a room: the emulator (in its own
// process), which can hold its picture and save its state.
type RoomSource interface {
	MediaSource
	SetPaused(paused bool)
	SaveState(ctx context.Context, path string) error
}

// savesReporter is a source that knows whether the emulator saves its game
// whole (WorkerSource).
type savesReporter interface {
	SavesIncomplete() bool
}

// RoomSourceFactory makes the game of a room. statePath, when not empty,
// is a save state to load right after the game.
type RoomSourceFactory func(rom, statePath string, onReady func(libretro.AVInfo)) RoomSource

// RoomsConfig wires RoomsService.
type RoomsConfig struct {
	Library   *LibraryService
	Status    *StatusService
	ICE       *ICEStore
	Stream    StreamConfig // for each room's stream: set API to share the UDP port
	Opener    *RoomOpener
	HostName  string
	SavesDir  string // saved games, one folder per room
	MaxRooms  int    // rooms with a running game; 0 means models.DefaultMaxRooms
	Rooms     []models.SavedRoom
	Save      func([]models.SavedRoom) error // keeps the list in device.json
	NewSource RoomSourceFactory
	// ProbeSaves tells whether the emulator saves a game whole (see the
	// package function ProbeSaves; the device caches it by ROM). Optional.
	ProbeSaves func(ctx context.Context, rom string) (bool, error)
	// History records every finished game (nil: no history).
	History *HistoryService
	// Telemetry keeps what happens in each room while it runs (nil:
	// nothing); Host gives the computer's CPU, memory and network for it.
	Telemetry *telemetry.Store
	Host      func() telemetry.Metrics
	// Recordings keeps the games the host records (nil: no recording).
	Recordings *RecordingService
	// OnRecording tells the host's browsers that a recording ended
	// (recording_saved or recording_error), so they can offer it.
	OnRecording func(RecordingEvent)
	// Trusted tells the device's own linked browsers, who skip the PIN.
	Trusted func(peerID string) bool
	// OnPauseAsk tells the host's linked browsers that a player asks for a
	// pause (pause_asked) or that the request is gone (pause_ask_gone).
	OnPauseAsk func(RoomPauseAskEvent)
	// VideoQuality is the host's video quality for game rooms (high,
	// normal or saver; empty is the default). SetVideoQuality changes it.
	VideoQuality string
	// HDH264 is go-link HD rooms' video encoder: "" sends VP8 like every
	// room, else H.264 made by ffmpeg with this encoder ("videotoolbox",
	// the Mac's hardware, or "x264").
	HDH264 string
	// HDKbps is go-link HD rooms' bitrate at high quality (their picture is
	// 720p or 1080p, not a 2x arcade game); 0 keeps the qualities' own.
	HDKbps int
	// HDVideo, when set, gives HDH264 and HDKbps for each room as it
	// starts (the host's go-link HD screen settings can change).
	HDVideo func() (h264 string, kbps int)
	Logger  *slog.Logger
	Now     func() time.Time
}

// RoomsService turns the device into a game server: each room is one game,
// several can run at once, and each can be paused, saved, archived,
// deleted (to a trash) and started again. A room's game runs in its own
// process; its stream, Room Manager and signalhub room live here, and all
// rooms share one UDP port and one signaling connection.
type RoomsService struct {
	cfg RoomsConfig
	log *slog.Logger

	mu        sync.Mutex
	rooms     []*gameRoom // in creation order
	sender    Sender
	connected bool
	// games is the context of every room's game. It outlives Run's ctx so
	// Shutdown can still save the games; Shutdown ends it.
	games    context.Context
	endGames context.CancelFunc
	streams  sync.WaitGroup // each room's stream (and so its game process)
	stopping bool           // Shutdown began: games ending is expected
	// resume are the rooms device.json had running at startup; Run starts
	// only those (a room created meanwhile must not start twice).
	resume []*gameRoom
	// hostLinked: a linked browser of the host is connected, so the host
	// can answer requests for a pause.
	hostLinked bool
	// quality is the host's video quality for game rooms.
	quality string
}

// RoomPauseAskEvent is a PauseAskEvent of one room, for linked browsers:
// id is the device's room id.
type RoomPauseAskEvent struct {
	PauseAskEvent
	ID string `json:"id"`
}

// gameRoom is one room and, while its game runs, the parts that serve it.
type gameRoom struct {
	saved models.SavedRoom
	game  string // the game's title
	// hdKbps is a go-link HD room's bitrate at high quality, from its start.
	hdKbps int

	cancel  context.CancelFunc
	stream  *StreamService
	signal  *TestRoomService
	manager *RoomManager
	source  RoomSource
	ready   bool
	roomID  string
	summary RoomSummary
	seats   int             // as many as the game has players, known when the room opens
	art     string          // Boxart for the lobby, base64 JPEG
	reply   func(GameReply) // pending answer to the owner who started it
	invite  string          // current invitation (link, QR) while it runs
	code    string          // its 9 digit code
	rec     *Recorder       // the recording in progress, if any
	recs    []RecordingInfo // recordings of this session, for the history
	// The running session, for the history of games; runID names it there
	// and in the telemetry, which tele records.
	runID          string
	tele           *telemetry.Recorder
	startedAt      time.Time
	peakPlayers    int
	peakSpectators int
	people         []string                  // peers, in order of arrival
	person         map[string]*HistoryPerson // by peer
	// pauseAsks are the pending requests for a pause, for device_status.
	pauseAsks []models.PauseAsk
	// video is what the running game sends (device_status).
	video models.RoomVideo
}

// RecordingEvent is sent to the host's linked browsers when a recording
// ends: recording_saved with the recording, or recording_error.
type RecordingEvent struct {
	Type      string         `json:"type"`
	Room      string         `json:"room"`   // the saved room's id
	Reason    string         `json:"reason"` // why it stopped (RecStopped...)
	Recording *RecordingInfo `json:"recording,omitempty"`
	Error     string         `json:"error,omitempty"`
}

// GameService is the older name of RoomsService, used by the window.
type GameService = RoomsService

// NewRoomsService builds the service from the rooms saved in device.json.
// Call Run to start the ones that were running.
func NewRoomsService(cfg RoomsConfig) *RoomsService {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.MaxRooms <= 0 {
		cfg.MaxRooms = models.DefaultMaxRooms
	}
	r := &RoomsService{cfg: cfg, log: cfg.Logger, quality: models.CleanVideoQuality(cfg.VideoQuality)}
	r.games, r.endGames = context.WithCancel(context.Background())
	for _, s := range cfg.Rooms {
		if s.ID == "" || s.Rom == "" {
			continue
		}
		s.Picture = models.CleanPicture(s.Picture) // a hand-edited device.json
		gr := &gameRoom{saved: s, game: r.title(s.Rom)}
		r.rooms = append(r.rooms, gr)
		if s.Running() {
			r.resume = append(r.resume, gr)
		}
	}
	return r
}

// Run starts the rooms that were running when the device stopped (from
// where they left off), empties the trash of old rooms every hour, and
// saves every running game when ctx ends.
func (r *RoomsService) Run(ctx context.Context) {
	r.publish()
	r.mu.Lock()
	restart := r.resume
	r.resume = nil
	r.mu.Unlock()
	for _, gr := range restart {
		state := ""
		if gr.saved.Autosave && !gr.saved.NoSaves {
			state = r.statePath(gr.saved.ID, "auto")
		}
		if state != "" && r.cfg.ProbeSaves != nil {
			// As in Start: make sure the save brings the whole game back
			// before loading it, off this loop (the first time takes a
			// few seconds).
			go func(gr *gameRoom, state string) {
				if !r.savesWork(gr.saved.Rom) {
					r.log.Warn("this game cannot resume from a save in this emulator: starting it over", "room", gr.saved.Name, "rom", gr.saved.Rom)
					r.markNoSaves(gr)
					state = ""
				}
				r.restart(gr, state)
			}(gr, state)
			continue
		}
		r.restart(gr, state)
	}
	r.purgeTrash()
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.purgeTrash()
		}
	}
}

// restart launches a room that was running when the device stopped, from
// state (empty = from power on), paused again if it was.
func (r *RoomsService) restart(gr *gameRoom, state string) {
	if err := r.launch(gr, state); err != nil {
		r.log.Warn("cannot restart room", "room", gr.saved.Name, "err", err)
		r.update(gr, func(s *models.SavedRoom) { s.State, s.LastError = models.RoomArchived, err.Error() })
		return
	}
}

// Shutdown saves every running game (auto.state) so the rooms come back
// on the next start, then stops the games. Their state stays live or
// paused in device.json.
func (r *RoomsService) Shutdown(ctx context.Context) {
	r.mu.Lock()
	r.stopping = true
	r.mu.Unlock()
	for _, gr := range r.running() {
		_ = r.stopRecording(gr, nil, RecDeviceStopped)
		r.autosave(ctx, gr)
		r.record(gr, "device_stopped")
	}
	r.persist()
	r.endGames()
	// Wait for the game processes to quit: MAME writes its settings on
	// the way out.
	done := make(chan struct{})
	go func() {
		r.streams.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		r.log.Warn("some games did not stop in time")
	}
}

// SetSender wires the signaling client.
func (r *RoomsService) SetSender(s Sender) {
	r.mu.Lock()
	r.sender = s
	parts := r.runningLocked()
	r.mu.Unlock()
	for _, gr := range parts {
		gr.stream.SetSender(s)
		gr.signal.SetSender(s)
	}
}

var _ signalclient.Handler = (*RoomsService)(nil)

// OnConnect opens every running room again: rooms die with the signaling
// connection.
func (r *RoomsService) OnConnect(env signalclient.Envelope) {
	r.mu.Lock()
	r.connected = true
	var ready []*gameRoom
	for _, gr := range r.runningLocked() {
		if gr.ready {
			ready = append(ready, gr)
		}
	}
	r.mu.Unlock()
	for _, gr := range ready {
		gr.signal.OnConnect(env)
	}
	r.teleAll(telemetry.Info, "signal_up", "connected to the signaling server", nil)
}

// OnMessage hands guests and their WebRTC signaling to their room.
func (r *RoomsService) OnMessage(env signalclient.Envelope) {
	for _, gr := range r.running() {
		gr.signal.OnMessage(env) // each room keeps only its own guests
	}
}

// OnDisconnect drops every guest; the rooms reopen on the next connection.
func (r *RoomsService) OnDisconnect(err error) {
	r.mu.Lock()
	r.connected = false
	for _, gr := range r.rooms {
		gr.roomID = ""
	}
	parts := r.runningLocked()
	r.mu.Unlock()
	for _, gr := range parts {
		gr.signal.OnDisconnect(err)
	}
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	// Games already playing go on (WebRTC does not need it), but nobody
	// new gets in until it is back.
	r.teleAll(telemetry.Warn, "signal_down", "lost the signaling server", map[string]any{"err": reason})
	r.publish()
}

// teleAll writes an event to every running room's telemetry.
func (r *RoomsService) teleAll(level telemetry.Level, kind, msg string, data map[string]any) {
	r.mu.Lock()
	var recs []*telemetry.Recorder
	for _, gr := range r.runningLocked() {
		if gr.tele != nil {
			recs = append(recs, gr.tele)
		}
	}
	r.mu.Unlock()
	for _, rec := range recs {
		rec.Event(level, kind, "", msg, data)
	}
}

// Create starts a new room playing a ROM. reply receives room_created
// (once the game runs and the room is open) or room_error; it may be
// called from another goroutine.
func (r *RoomsService) Create(req GameRequest, reply func(GameReply)) error {
	lib := r.cfg.Library
	if !lib.HasRom(req.Rom) {
		return ErrUnknownRom
	}
	if !lib.HasCoreFor(req.Rom) {
		return ErrNoCore
	}
	if req.Rom == MakerRom {
		// Willy Maker's game has one room: a new game sent replaces the
		// room of the one before (and its saves, which were of that build).
		if req.Maker != nil {
			if err := lib.SetMakerInfo(*req.Maker); err != nil {
				return err
			}
		}
		r.replaceMakerRooms()
	}
	// Refuse a set the core cannot run, before loading anything.
	if res, ok := lib.CheckRom(req.Rom); ok && res.Status != romcheck.StatusOK {
		return errors.New(res.Reason())
	}
	game := r.title(req.Rom)
	name := cleanText(req.Title, 60)
	if name == "" {
		name = game
	}
	now := r.cfg.Now()
	gr := &gameRoom{
		saved: models.SavedRoom{
			ID: newRoomID(), Name: name, Rom: req.Rom, Public: false, Voice: req.Voice, Art: artKind(req.Art),
			ChatOff: req.Chat != nil && !*req.Chat,
			Picture: models.CleanPicture(req.Picture),
			State:   models.RoomLive, CreatedAt: now, Since: now,
		},
		game:  game,
		reply: reply,
	}
	r.mu.Lock()
	if r.countRunningLocked() >= r.cfg.MaxRooms {
		r.mu.Unlock()
		return &TooManyRoomsError{Limit: r.cfg.MaxRooms}
	}
	r.rooms = append(r.rooms, gr)
	r.mu.Unlock()
	if err := r.launch(gr, ""); err != nil {
		r.remove(gr)
		return err
	}
	r.persist()
	return nil
}

// HasRoomFor reports whether a room (in any state but the trash) plays a
// game.
func (r *RoomsService) HasRoomFor(rom string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, gr := range r.rooms {
		if gr.saved.Rom == rom && gr.saved.State != models.RoomTrash {
			return true
		}
	}
	return false
}

// AddArchived adds a room for a game without starting it: it is listed
// with the archived ones, ready to Start.
func (r *RoomsService) AddArchived(rom string, voice bool) error {
	lib := r.cfg.Library
	if !lib.HasRom(rom) {
		return ErrUnknownRom
	}
	now := r.cfg.Now()
	game := r.title(rom)
	gr := &gameRoom{
		saved: models.SavedRoom{
			ID: newRoomID(), Name: game, Rom: rom, Voice: voice,
			State: models.RoomArchived, CreatedAt: now, Since: now,
		},
		game: game,
	}
	r.mu.Lock()
	r.rooms = append(r.rooms, gr)
	r.mu.Unlock()
	r.persist()
	r.publish()
	return nil
}

// Start turns on an archived or deleted room. from is "continue" (the
// automatic save), "fresh" or "slot" (a saved game, slot).
func (r *RoomsService) Start(id, from string, slot int, reply func(GameReply)) error {
	gr := r.find(id)
	if gr == nil {
		return ErrUnknownRoom
	}
	r.mu.Lock()
	if gr.saved.Running() {
		r.mu.Unlock()
		return ErrRoomState
	}
	if r.countRunningLocked() >= r.cfg.MaxRooms {
		r.mu.Unlock()
		return &TooManyRoomsError{Limit: r.cfg.MaxRooms}
	}
	saved := gr.saved
	r.mu.Unlock()
	if !r.cfg.Library.HasRom(saved.Rom) {
		return ErrUnknownRom
	}
	state := ""
	switch from {
	case "continue":
		if saved.Autosave {
			state = r.statePath(id, "auto")
		}
	case "slot":
		if !slices.ContainsFunc(saved.Saves, func(s models.SaveSlot) bool { return s.Slot == slot }) {
			return fmt.Errorf("saved game %d does not exist", slot)
		}
		state = r.statePath(id, fmt.Sprintf("slot-%d", slot))
	}
	r.mu.Lock()
	gr.reply = reply
	gr.saved.State, gr.saved.Since, gr.saved.DeletedAt, gr.saved.LastError = models.RoomLive, r.cfg.Now(), nil, ""
	r.mu.Unlock()
	if state != "" && r.cfg.ProbeSaves != nil {
		// Resuming: first make sure the save brings the whole game back
		// (a few seconds the first time), off the owner's message loop.
		r.persist()
		go func() {
			if !r.savesWork(saved.Rom) {
				r.log.Warn("this game cannot resume from a save in this emulator: starting it over", "room", saved.Name, "rom", saved.Rom)
				r.markNoSaves(gr)
				state = ""
			}
			if err := r.launch(gr, state); err != nil {
				r.update(gr, func(s *models.SavedRoom) { s.State = saved.State; s.DeletedAt = saved.DeletedAt })
				if reply != nil {
					reply(GameReply{Type: "room_error", ID: id, Rom: saved.Rom, Error: err.Error()})
				}
			}
		}()
		return nil
	}
	if err := r.launch(gr, state); err != nil {
		r.update(gr, func(s *models.SavedRoom) { s.State = saved.State; s.DeletedAt = saved.DeletedAt })
		return err
	}
	r.persist()
	return nil
}

// savesWork asks ProbeSaves about rom; when it cannot tell, the answer is
// yes (the worker still refuses a save that leaves out a CPU).
func (r *RoomsService) savesWork(rom string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	ok, err := r.cfg.ProbeSaves(ctx, rom)
	if err != nil {
		r.log.Warn("cannot test the game's saves", "rom", rom, "err", err)
		return true
	}
	return ok
}

// markNoSaves remembers that a room's game cannot be saved whole: no
// automatic save, no saved games, and its stale automatic save goes.
func (r *RoomsService) markNoSaves(gr *gameRoom) {
	r.mu.Lock()
	id, done := gr.saved.ID, gr.saved.NoSaves && !gr.saved.Autosave
	r.mu.Unlock()
	if done {
		return
	}
	_ = os.Remove(r.statePath(id, "auto"))
	r.update(gr, func(s *models.SavedRoom) {
		s.NoSaves = true
		s.Autosave = false
	})
}

// Action changes a room: pause, resume, save (name), archive, delete,
// purge, favorite or unfavorite. For save it returns the new slot.
func (r *RoomsService) Action(ctx context.Context, id, action, name string) (int, error) {
	gr := r.find(id)
	if gr == nil {
		return 0, ErrUnknownRoom
	}
	r.mu.Lock()
	state := gr.saved.State
	r.mu.Unlock()
	switch action {
	case "favorite", "unfavorite":
		r.update(gr, func(s *models.SavedRoom) { s.Favorite = action == "favorite" })
	case "chat_on", "chat_off":
		on := action == "chat_on"
		r.update(gr, func(s *models.SavedRoom) { s.ChatOff = !on })
		if m := r.managerOf(gr); m != nil {
			m.SetChat(on)
		}
	case "input_hud_on", "input_hud_off":
		// The host's latency test: the controllers drawn on the picture
		// while the game runs (not saved, a diagnostic).
		r.mu.Lock()
		stream, m, seats := gr.stream, gr.manager, gr.seats
		r.mu.Unlock()
		if stream == nil || m == nil {
			return 0, ErrRoomState
		}
		on := action == "input_hud_on"
		if on {
			stream.SetInputHUD(seatsOrDefault(seats))
		} else {
			stream.SetInputHUD(0)
		}
		m.SetInputHUD(on)
	case "new_link":
		// A new invitation: the old link, QR and code stop working.
		r.mu.Lock()
		sig := gr.signal
		r.mu.Unlock()
		if sig == nil {
			return 0, ErrRoomState
		}
		sig.NewInvite()
	case "pause", "resume":
		m := r.managerOf(gr)
		if m == nil {
			return 0, ErrRoomState
		}
		m.Pause(action == "pause", "The host")
		if action == "resume" {
			// A room marked paused whose game already runs (the manager has
			// nothing to resume, so its hook never fires): mark it live.
			r.update(gr, func(s *models.SavedRoom) {
				if s.State == models.RoomPaused {
					s.State, s.Since = models.RoomLive, r.cfg.Now()
				}
			})
			r.mu.Lock()
			sig := gr.signal
			r.mu.Unlock()
			if sig != nil {
				sig.SetMetaExtra(r.lobbyExtra(gr))
			}
		}
	case "record_start":
		return 0, r.startRecording(gr)
	case "record_stop":
		return 0, r.stopRecording(gr, nil, RecStopped)
	case "save":
		if r.managerOf(gr) == nil {
			return 0, ErrRoomState
		}
		r.mu.Lock()
		noSaves := gr.saved.NoSaves
		r.mu.Unlock()
		if noSaves {
			return 0, ErrNoSaves
		}
		return r.saveSlot(ctx, gr, name)
	case "archive":
		if state != models.RoomLive && state != models.RoomPaused {
			return 0, ErrRoomState
		}
		r.stop(ctx, gr, true, "archived")
		r.update(gr, func(s *models.SavedRoom) { s.State, s.Since = models.RoomArchived, r.cfg.Now() })
	case "delete":
		if state == models.RoomTrash {
			return 0, ErrRoomState
		}
		if r.managerOf(gr) != nil {
			r.stop(ctx, gr, true, "deleted")
		}
		now := r.cfg.Now()
		r.update(gr, func(s *models.SavedRoom) { s.State, s.Since, s.DeletedAt = models.RoomTrash, now, &now })
	case "purge":
		if state != models.RoomTrash {
			return 0, ErrRoomState
		}
		r.purge(gr)
	default:
		return 0, fmt.Errorf("unknown room action %q", action)
	}
	return 0, nil
}

// SetPicture sets a room's default picture style for its guests (nil
// clears it: the website's own default). Unknown values are refused and
// change nothing.
func (r *RoomsService) SetPicture(id string, p *models.RoomPicture) error {
	gr := r.find(id)
	if gr == nil {
		return ErrUnknownRoom
	}
	if p != nil && !p.Valid() {
		return ErrBadPicture
	}
	p = models.CleanPicture(p)
	r.update(gr, func(s *models.SavedRoom) { s.Picture = p })
	if m := r.managerOf(gr); m != nil {
		m.SetPicture(p)
	}
	return nil
}

// Current returns the ROM of the newest running room, or "" (the window
// shows it as the game being played).
func (r *RoomsService) Current() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.rooms) - 1; i >= 0; i-- {
		if r.rooms[i].saved.Running() {
			return r.rooms[i].saved.Rom
		}
	}
	return ""
}

// SetHostLinked tells every room whether a linked browser of the host is
// connected (guests see host_online).
func (r *RoomsService) SetHostLinked(on bool) {
	r.mu.Lock()
	r.hostLinked = on
	parts := r.runningLocked()
	r.mu.Unlock()
	for _, gr := range parts {
		if m := r.managerOf(gr); m != nil {
			m.SetHostLinked(on)
		}
	}
}

// AnswerPause is the host's answer, from a linked browser, to a player's
// request for a pause in room id.
func (r *RoomsService) AnswerPause(id, from string, accept bool) error {
	gr := r.find(id)
	if gr == nil {
		return ErrUnknownRoom
	}
	m := r.managerOf(gr)
	if m == nil || !m.AnswerPause(from, accept) {
		return ErrRoomState
	}
	return nil
}

// pauseAsked keeps a room's requests for a pause and tells the host.
func (r *RoomsService) pauseAsked(gr *gameRoom, ev PauseAskEvent) {
	r.mu.Lock()
	gr.pauseAsks = slices.DeleteFunc(gr.pauseAsks, func(a models.PauseAsk) bool { return a.From == ev.From })
	if ev.Type == "pause_asked" && ev.ExpiresAt != nil {
		gr.pauseAsks = append(gr.pauseAsks, models.PauseAsk{From: ev.From, Name: ev.Name, Port: ev.Port, ExpiresAt: *ev.ExpiresAt})
	}
	id := gr.saved.ID
	fn := r.cfg.OnPauseAsk
	r.mu.Unlock()
	if fn != nil {
		fn(RoomPauseAskEvent{PauseAskEvent: ev, ID: id})
	}
	r.publish()
}

// Close archives the newest running room (the window's Stop button, and
// close_room from older web versions).
func (r *RoomsService) Close(reply func(GameReply)) {
	r.mu.Lock()
	var gr *gameRoom
	for i := len(r.rooms) - 1; i >= 0 && gr == nil; i-- {
		if r.rooms[i].saved.Running() {
			gr = r.rooms[i]
		}
	}
	r.mu.Unlock()
	if gr == nil {
		return
	}
	id := gr.saved.ID
	if _, err := r.Action(context.Background(), id, "archive", ""); err == nil && reply != nil {
		reply(GameReply{Type: "room_closed", ID: id})
	}
}

// launch starts a room's game, stream and Room Manager. The room opens
// in signalhub once the game runs, so a ROM that fails never shows up in
// the lobby.
func (r *RoomsService) launch(gr *gameRoom, statePath string) error {
	streamCfg := r.cfg.Stream
	if r.isHD(gr) {
		h264, kbps := r.cfg.HDH264, r.cfg.HDKbps
		if r.cfg.HDVideo != nil {
			h264, kbps = r.cfg.HDVideo()
		}
		streamCfg.H264Encoder = h264
		gr.hdKbps = kbps
	}
	stream, err := NewStreamService(streamCfg, r.cfg.ICE)
	if err != nil {
		return err
	}
	r.mu.Lock()
	saved, game, sender := gr.saved, gr.game, r.sender
	parent := r.games
	r.mu.Unlock()

	signal := NewTestRoomService(nil, stream, r.cfg.HostName, r.log)
	signal.SetOpener(r.cfg.Opener)
	signal.Configure(saved.Name, game, saved.Public)
	signal.SetTrusted(r.cfg.Trusted)
	signal.EnableInvites(func(invite, code string) {
		r.mu.Lock()
		gr.invite, gr.code = invite, code
		r.mu.Unlock()
		r.publish()
	})
	if saved.Public {
		// Rooms saved before every room was private become private now.
		r.update(gr, func(s *models.SavedRoom) { s.Public = false })
	}
	signal.Configure(saved.Name, game, false)
	signal.SetPrivate()
	// The room has as many seats as the game has players (Street Fighter II
	// two, Teenage Mutant Ninja Turtles four).
	controls := r.controlsOf(saved.Rom)
	r.mu.Lock()
	gr.seats = SeatsFor(controls)
	r.mu.Unlock()
	tele := r.cfg.Telemetry.Room(saved.ID)
	stream.SetTelemetry(tele, r.cfg.Host)
	manager := NewRoomManager(RoomManagerConfig{Logger: r.log, MaxPlayers: SeatsFor(controls), OnLog: RoomTeleLog(tele), OnSummary: func(s RoomSummary) {
		signal.OnSummary(s)
		r.mu.Lock()
		gr.summary = s
		gr.peakPlayers = max(gr.peakPlayers, s.Players)
		gr.peakSpectators = max(gr.peakSpectators, s.Spectators)
		gr.remember(s.Members)
		r.mu.Unlock()
		r.publish()
	}}, stream)
	signal.SetManager(manager)
	stream.SetRoomHooks(RoomHooks{Opened: manager.Join, Message: manager.HandleControl, Closed: manager.Leave, PortOf: manager.PortOf})
	stream.SetVoiceEnabled(saved.Voice)
	signal.OnRoomID(func(roomID string) {
		r.mu.Lock()
		gr.roomID = roomID
		reply := gr.reply
		gr.reply = nil
		id, rom, name := gr.saved.ID, gr.saved.Rom, gr.saved.Name
		r.mu.Unlock()
		if reply != nil {
			reply(GameReply{Type: "room_created", ID: id, RoomID: roomID, Rom: rom, Title: name})
		}
		r.publish()
	})

	var once sync.Once
	source := r.cfg.NewSource(saved.Rom, statePath, func(libretro.AVInfo) {
		once.Do(func() {
			manager.SetVoice(saved.Voice)
			manager.SetChat(!saved.ChatOff)
			manager.SetPicture(saved.Picture)
			manager.SetPausable(true)
			// A room saved paused comes back paused, now that the game can
			// be paused (a pause before this was ignored, leaving the room
			// marked paused while its game ran, and Resume did nothing).
			r.mu.Lock()
			wasPaused := gr.saved.State == models.RoomPaused
			r.mu.Unlock()
			if wasPaused {
				manager.Pause(true, "The host")
			}
			manager.SetControls(controls)
			tele.Event(telemetry.Info, "game_ready", "", "the game is running", map[string]any{"rom": saved.Rom, "paused": wasPaused})
			r.mu.Lock()
			gr.ready = true
			connected := r.connected
			src := gr.source
			r.mu.Unlock()
			// The worker knows by now whether this game can resume.
			if ss, ok := src.(savesReporter); ok && ss.SavesIncomplete() {
				r.markNoSaves(gr)
			}
			signal.SetMetaExtra(r.lobbyExtra(gr))
			if connected {
				signal.OnConnect(signalclient.Envelope{})
			}
		})
	})
	manager.OnPauseAsk(func(ev PauseAskEvent) { r.pauseAsked(gr, ev) })
	manager.OnPause(func(paused bool) {
		source.SetPaused(paused)
		if paused {
			// A paused game ends its recording (the host is warned first).
			go func() { _ = r.stopRecording(gr, nil, RecPaused) }()
		}
		state := models.RoomLive
		if paused {
			state = models.RoomPaused
		}
		r.update(gr, func(s *models.SavedRoom) {
			if s.Running() && s.State != state {
				s.State, s.Since = state, r.cfg.Now()
			}
		})
		signal.SetMetaExtra(r.lobbyExtra(gr))
	})
	stream.OnSourceError(func(err error) { go r.failed(gr, err) })
	r.mu.Lock()
	quality := r.quality
	r.mu.Unlock()
	r.applyVideo(gr, stream, source, PlanFor(quality))
	stream.SetSource(source)
	stream.SetSender(sender)
	signal.SetSender(sender)

	ctx, cancel := context.WithCancel(parent)
	r.mu.Lock()
	gr.cancel, gr.stream, gr.signal, gr.manager, gr.source = cancel, stream, signal, manager, source
	gr.ready, gr.roomID, gr.summary = false, "", RoomSummary{}
	gr.startedAt, gr.peakPlayers, gr.peakSpectators = r.cfg.Now(), 0, 0
	gr.runID, gr.tele = NewRunID(), tele
	tele.Start(gr.runID, game)
	gr.people, gr.person = nil, map[string]*HistoryPerson{}
	gr.rec, gr.recs = nil, nil
	gr.pauseAsks = nil
	hostLinked := r.hostLinked
	gr.art = r.art(saved.Rom, saved.Art)
	info := RoomInfo{Title: saved.Name, Game: game, Host: r.cfg.HostName, Art: gr.art, Own: r.cfg.Library != nil && r.cfg.Library.Own(saved.Rom) != nil}
	r.mu.Unlock()
	manager.SetInfo(info)
	manager.SetHostLinked(hostLinked)
	go manager.Run(ctx)
	r.streams.Add(1)
	go func() {
		defer r.streams.Done()
		if err := stream.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			r.log.Warn("room stream stopped", "room", saved.Name, "err", err)
		}
	}()
	r.log.Info("room starting", "room", saved.Name, "rom", saved.Rom, "state", statePath != "")
	if r.cfg.ProbeSaves != nil && !saved.NoSaves && statePath == "" {
		// Learn early whether this game can be saved, so the owner is
		// never offered a save that would not work. A room loading a
		// save was already checked (Start, Run): testing it again here
		// could remove auto.state while the worker is still reading it.
		go func() {
			if !r.savesWork(saved.Rom) {
				r.markNoSaves(gr)
			}
		}()
	}
	r.publish()
	return nil
}

// applyVideo sets up a room's picture for a video plan: the worker's
// conversion, the bitrate and, for a 2x picture, the encoder check that
// falls back to saver when 2x does not fit this computer.
func (r *RoomsService) applyVideo(gr *gameRoom, stream *StreamService, source RoomSource, plan VideoPlan) {
	if gr.hdKbps > 0 && r.isHD(gr) {
		// a go-link HD room's picture is 720p or 1080p: its own bitrate, less for the lower qualities
		plan.Kbps = hdPlanKbps(gr.hdKbps, plan.Quality)
	}
	scale := 1
	if vs, ok := source.(videoModeSetter); ok {
		vs.SetVideoMode(plan.Mode)
		scale = plan.Mode.Scale()
	} else {
		plan = PlanFor(models.VideoSaver) // a source that only sends the game's size
		plan.Kbps = stream.cfg.BitrateKbps
	}
	stream.SetBitrate(plan.Kbps)
	stream.SetVideoInfo(plan.Quality, "")
	if scale == 2 {
		stream.ArmEncodeProbe(func(p95, frame time.Duration) { r.encoderTooSlow(gr, stream, p95, frame) })
	} else {
		stream.ArmEncodeProbe(nil)
	}
	stream.SetOverloadHandler(func(share float64) { r.hostOverloaded(gr, stream, share) })
	r.mu.Lock()
	gr.video = models.RoomVideo{Quality: plan.Quality, Scale: scale}
	r.mu.Unlock()
}

// isHD is whether a room plays a go-link HD game (a .glhd package or a built-in one).
func (r *RoomsService) isHD(gr *gameRoom) bool {
	return r.cfg.Library != nil && r.cfg.Library.IsHD(gr.saved.Rom)
}

// hdPlanKbps is a go-link HD room's bitrate for a quality: kbps at high,
// two thirds at normal, a half at saver.
func hdPlanKbps(kbps int, quality string) int {
	switch quality {
	case models.VideoNormal:
		return kbps * 2 / 3
	case models.VideoSaver:
		return kbps / 2
	}
	return kbps
}

// encoderTooSlow moves a room whose 2x picture does not fit this computer
// to saver (the game's size, averaged color) for the rest of its run.
func (r *RoomsService) encoderTooSlow(gr *gameRoom, stream *StreamService, p95, frame time.Duration) {
	r.mu.Lock()
	source, name := gr.source, gr.saved.Name
	current := gr.stream == stream && source != nil
	r.mu.Unlock()
	if !current {
		return // the room stopped or started again meanwhile
	}
	r.log.Warn("the 2x picture does not fit this computer: the room goes to saver",
		"room", name, "encode_p95_ms", float64(p95.Microseconds())/1000,
		"frame_ms", float64(frame.Microseconds())/1000, "limit_share", slowShare)
	plan := PlanFor(models.VideoSaver)
	if vs, ok := source.(videoModeSetter); ok {
		vs.SetVideoMode(plan.Mode)
	}
	stream.SetBitrate(plan.Kbps)
	stream.SetVideoInfo(plan.Quality, models.VideoFallbackCPU)
	r.mu.Lock()
	gr.video = models.RoomVideo{Quality: plan.Quality, Fallback: models.VideoFallbackCPU, Scale: 1}
	r.mu.Unlock()
	r.publish()
}

// hostOverloaded runs when encoding kept a running room busy for seconds
// (encode_load.go): a 2x room drops to saver, as when the encoder probe
// finds 2x too slow; a 1x room has nothing lighter, so it is only logged.
func (r *RoomsService) hostOverloaded(gr *gameRoom, stream *StreamService, share float64) {
	r.mu.Lock()
	current := gr.stream == stream && gr.source != nil
	scale := gr.video.Scale
	r.mu.Unlock()
	if !current {
		return
	}
	pct := math.Round(share * 100)
	if scale == 2 {
		stream.teleEvent(telemetry.Warn, "host_overloaded", "", "the host's computer could not keep up: the room went to saver (the game's own size)", map[string]any{"encode_pct": pct, "action": "saver"})
		const frame = time.Second / 60
		r.encoderTooSlow(gr, stream, time.Duration(share*float64(frame)), frame)
		return
	}
	stream.teleEvent(telemetry.Warn, "host_overloaded", "", "the host's computer cannot keep up, even at the game's own size", map[string]any{"encode_pct": pct, "action": "none"})
	r.log.Warn("the host's computer cannot keep up", "room", gr.saved.Name, "encode_pct", pct)
}

// SetVideoQuality changes the video quality of game rooms. Running rooms
// switch at once (the encoder starts again with a keyframe), and a room
// that fell back to saver checks the encoder again.
func (r *RoomsService) SetVideoQuality(q string) {
	q = models.CleanVideoQuality(q)
	r.mu.Lock()
	r.quality = q
	type live struct {
		gr     *gameRoom
		stream *StreamService
		source RoomSource
	}
	var rooms []live
	for _, gr := range r.runningLocked() {
		if gr.stream != nil && gr.source != nil {
			rooms = append(rooms, live{gr, gr.stream, gr.source})
		}
	}
	r.mu.Unlock()
	for _, l := range rooms {
		r.applyVideo(l.gr, l.stream, l.source, PlanFor(q))
	}
	r.publish()
}

// VideoQuality is the host's video quality for game rooms.
func (r *RoomsService) VideoQuality() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.quality
}

// failed handles a game that could not run (or crashed): the room is
// archived with the reason, and the owner who started it is told.
func (r *RoomsService) failed(gr *gameRoom, err error) {
	r.mu.Lock()
	stopping := r.stopping
	r.mu.Unlock()
	if stopping {
		return // the device is stopping: the room stays as it was, to come back
	}
	r.log.Warn("room game failed", "room", gr.saved.Name, "err", err)
	r.mu.Lock()
	gr.tele.Event(telemetry.Error, "game_failed", "", "the game stopped with an error", map[string]any{"err": err.Error()})
	reply := gr.reply
	gr.reply = nil
	id, rom := gr.saved.ID, gr.saved.Rom
	r.mu.Unlock()
	r.stop(context.Background(), gr, false, "failed")
	r.update(gr, func(s *models.SavedRoom) {
		s.State, s.Since, s.LastError = models.RoomArchived, r.cfg.Now(), err.Error()
	})
	if reply != nil {
		reply(GameReply{Type: "room_error", ID: id, Rom: rom, Error: err.Error()})
	}
}

// stop closes the room in signalhub and ends its game, saving it first
// when save is true.
func (r *RoomsService) stop(ctx context.Context, gr *gameRoom, save bool, reason string) {
	_ = r.stopRecording(gr, nil, RecRoomStopped)
	if save {
		r.autosave(ctx, gr)
	}
	r.record(gr, reason)
	r.mu.Lock()
	cancel, signal := gr.cancel, gr.signal
	gr.cancel, gr.stream, gr.signal, gr.manager, gr.source = nil, nil, nil, nil, nil
	gr.pauseAsks = nil
	gr.ready, gr.roomID, gr.summary = false, "", RoomSummary{}
	r.mu.Unlock()
	if signal != nil {
		signal.Close()
	}
	if cancel != nil {
		cancel()
	}
	r.publish()
}

// remember adds the room's current members to the people of this game:
// the last name each one used and every port it played at. Callers hold
// the RoomsService lock.
func (gr *gameRoom) remember(members []MemberSummary) {
	if gr.person == nil {
		gr.person = map[string]*HistoryPerson{}
	}
	for _, m := range members {
		p := gr.person[m.Peer]
		if p == nil {
			if len(gr.people) >= maxHistoryPeople {
				continue
			}
			p = &HistoryPerson{}
			gr.person[m.Peer] = p
			gr.people = append(gr.people, m.Peer)
		}
		p.Name = m.Name
		for _, port := range m.Ports {
			if !slices.Contains(p.Ports, port) {
				p.Ports = append(p.Ports, port)
			}
		}
		slices.Sort(p.Ports)
	}
}

// record adds a running room's session to the history of games.
func (r *RoomsService) record(gr *gameRoom, reason string) {
	if r.cfg.History == nil {
		return
	}
	r.mu.Lock()
	e := HistoryEntry{
		ID: gr.runID, RoomID: gr.saved.ID, Name: gr.saved.Name, Rom: gr.saved.Rom, Game: gr.game,
		StartedAt: gr.startedAt, EndedAt: r.cfg.Now(),
		PeakPlayers: gr.peakPlayers, PeakSpectators: gr.peakSpectators, Reason: reason,
		Recordings: gr.recs,
	}
	gr.recs = nil
	running := gr.signal != nil && !gr.startedAt.IsZero()
	stream := gr.stream
	for _, peer := range gr.people {
		p := *gr.person[peer]
		if stream != nil {
			if a, ok := stream.PeerAddr(peer); ok {
				p.IP, p.Path = a.IP, a.Path
			}
		}
		e.People = append(e.People, p)
	}
	tele := gr.tele
	gr.tele = nil
	r.mu.Unlock()
	if !running {
		return
	}
	tele.End(reason)
	if err := r.cfg.History.Add(e); err != nil {
		r.log.Warn("cannot save the history of games", "err", err)
	}
}

// startRecording begins recording a running room's game: its picture, its
// sound and the players' voices. Everyone in the room is told.
func (r *RoomsService) startRecording(gr *gameRoom) error {
	if r.cfg.Recordings == nil {
		return ErrRoomState
	}
	r.mu.Lock()
	stream, manager, ready, busy := gr.stream, gr.manager, gr.ready, gr.rec != nil
	id, name, paused := gr.saved.ID, gr.saved.Name, gr.saved.State == models.RoomPaused
	r.mu.Unlock()
	switch {
	case stream == nil || manager == nil || !ready:
		return ErrRoomState
	case busy:
		return ErrRecording
	case paused:
		return ErrRecordPaused
	}
	var rec *Recorder
	rec, err := r.cfg.Recordings.Start(id, name, func(reason string) { _ = r.stopRecording(gr, rec, reason) })
	if err != nil {
		return err
	}
	r.mu.Lock()
	if gr.rec != nil || gr.stream != stream {
		r.mu.Unlock()
		_, _ = r.cfg.Recordings.Finish(rec, id, name, RecStopped) // empty: leaves no file
		return ErrRecording
	}
	gr.rec = rec
	r.mu.Unlock()
	stream.SetRecorder(rec)
	manager.SetRecording(true)
	r.log.Info("recording started", "room", name, "file", rec.Path())
	r.publish()
	return nil
}

// stopRecording ends a room's recording (only want, when not nil), saves
// it and tells the host's browsers.
func (r *RoomsService) stopRecording(gr *gameRoom, want *Recorder, reason string) error {
	r.mu.Lock()
	rec, stream, manager := gr.rec, gr.stream, gr.manager
	if rec == nil || want != nil && rec != want {
		r.mu.Unlock()
		return ErrNotRecording
	}
	gr.rec = nil
	id, name := gr.saved.ID, gr.saved.Name
	r.mu.Unlock()
	if stream != nil {
		stream.SetRecorder(nil)
	}
	if manager != nil {
		manager.SetRecording(false)
	}
	r.publish()
	info, err := r.cfg.Recordings.Finish(rec, id, name, reason)
	ev := RecordingEvent{Type: "recording_saved", Room: id, Reason: reason}
	if info.ID == "" {
		ev.Type = "recording_error"
		if err != nil {
			ev.Error = err.Error()
		}
		r.log.Warn("recording lost", "room", name, "err", err)
	} else {
		if err != nil {
			r.log.Warn("recording saved with an error", "room", name, "err", err)
		}
		r.log.Info("recording saved", "room", name, "file", info.File, "reason", reason, "bytes", info.Size, "dropped_frames", rec.Dropped())
		r.mu.Lock()
		gr.recs = append(gr.recs, info)
		r.mu.Unlock()
		ev.Recording = &info
	}
	if r.cfg.OnRecording != nil {
		r.cfg.OnRecording(ev)
	}
	r.publish()
	return nil
}

// Reset stops every room without saving and forgets them all with their
// saved games (factory reset).
func (r *RoomsService) Reset(ctx context.Context) {
	for _, gr := range r.running() {
		r.stop(ctx, gr, false, "reset")
	}
	r.mu.Lock()
	r.rooms, r.resume = nil, nil
	r.mu.Unlock()
	if r.cfg.SavesDir != "" {
		_ = os.RemoveAll(r.cfg.SavesDir)
	}
	r.persist()
	r.publish()
}

// autosave writes auto.state for a running room.
func (r *RoomsService) autosave(ctx context.Context, gr *gameRoom) {
	r.mu.Lock()
	src, ready, id, noSaves := gr.source, gr.ready, gr.saved.ID, gr.saved.NoSaves
	r.mu.Unlock()
	if src == nil || !ready || noSaves {
		return // a game the emulator cannot save whole is never resumed
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := r.writeState(ctx, src, id, "auto"); err != nil {
		r.log.Warn("cannot save the game", "room", gr.saved.Name, "err", err)
		return
	}
	r.update(gr, func(s *models.SavedRoom) { s.Autosave = true })
}

// saveSlot saves the game as a new numbered slot.
func (r *RoomsService) saveSlot(ctx context.Context, gr *gameRoom, name string) (int, error) {
	r.mu.Lock()
	src, id := gr.source, gr.saved.ID
	slot := 1
	for _, s := range gr.saved.Saves {
		slot = max(slot, s.Slot+1)
	}
	full := len(gr.saved.Saves) >= MaxSaveSlots
	r.mu.Unlock()
	if full {
		return 0, ErrTooManySaves
	}
	if src == nil {
		return 0, ErrRoomState
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := r.writeState(ctx, src, id, fmt.Sprintf("slot-%d", slot)); err != nil {
		return 0, err
	}
	r.update(gr, func(s *models.SavedRoom) {
		s.Saves = append(s.Saves, models.SaveSlot{Slot: slot, Name: cleanText(name, 40), At: r.cfg.Now()})
	})
	return slot, nil
}

func (r *RoomsService) writeState(ctx context.Context, src RoomSource, id, name string) error {
	path := r.statePath(id, name)
	// Saved games are the host's: readable only by the user.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return src.SaveState(ctx, path)
}

// statePath is where a room keeps a saved game (auto or slot-N).
func (r *RoomsService) statePath(id, name string) string {
	return filepath.Join(r.cfg.SavesDir, id, name+".state")
}

// purge deletes a room and its saved games for good.
func (r *RoomsService) purge(gr *gameRoom) {
	// The id comes from device.json: only a plain id may name a folder to
	// delete, never a path.
	if r.cfg.SavesDir != "" && validRoomID(gr.saved.ID) {
		_ = os.RemoveAll(filepath.Join(r.cfg.SavesDir, gr.saved.ID))
	}
	// Its telemetry goes with it: every run's samples and events.
	if err := r.cfg.Telemetry.DeleteRoom(gr.saved.ID); err != nil {
		r.log.Warn("cannot delete the room's telemetry", "room", gr.saved.ID, "err", err)
	}
	r.remove(gr)
	r.persist()
	r.publish()
}

// purgeTrash deletes the rooms that stayed in the trash too long.
func (r *RoomsService) purgeTrash() {
	limit := r.cfg.Now().Add(-models.TrashDays * 24 * time.Hour)
	r.mu.Lock()
	var old []*gameRoom
	for _, gr := range r.rooms {
		if gr.saved.State == models.RoomTrash && gr.saved.DeletedAt != nil && gr.saved.DeletedAt.Before(limit) {
			old = append(old, gr)
		}
	}
	r.mu.Unlock()
	for _, gr := range old {
		r.log.Info("room removed from the trash", "room", gr.saved.Name)
		r.purge(gr)
	}
}

// update changes a room's saved fields, then saves and publishes the list.
func (r *RoomsService) update(gr *gameRoom, fn func(s *models.SavedRoom)) {
	r.mu.Lock()
	fn(&gr.saved)
	r.mu.Unlock()
	r.persist()
	r.publish()
}

func (r *RoomsService) persist() {
	if r.cfg.Save == nil {
		return
	}
	r.mu.Lock()
	list := make([]models.SavedRoom, 0, len(r.rooms))
	for _, gr := range r.rooms {
		list = append(list, gr.saved)
	}
	r.mu.Unlock()
	if err := r.cfg.Save(list); err != nil {
		r.log.Warn("cannot save the rooms", "err", err)
	}
}

// publish puts the room list in the device status (linked browsers get it
// in device_status).
func (r *RoomsService) publish() {
	if r.cfg.Status == nil {
		return
	}
	r.cfg.Status.SetRooms(r.List())
	if r.cfg.SavesDir != "" {
		r.cfg.Status.SetSavesBytes(dirSize(r.cfg.SavesDir))
	}
}

// dirSize adds up the bytes of the files under dir.
func dirSize(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

// List returns the rooms as linked browsers see them.
func (r *RoomsService) List() []models.ManagedRoom {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]models.ManagedRoom, 0, len(r.rooms))
	for _, gr := range r.rooms {
		// A room that is not running yet still has its game's seats.
		seats := gr.seats
		if seats < 1 {
			seats = SeatsFor(r.controlsOf(gr.saved.Rom))
		}
		out = append(out, models.ManagedRoom{
			SavedRoom: gr.saved, Game: gr.game, RoomID: gr.roomID,
			Players: gr.summary.Players, MaxPlayers: seats, Spectators: gr.summary.Spectators, Queue: gr.summary.Queue,
			Invite: gr.invite, InviteCode: gr.code, OwnerKey: ownerKey(gr.signal),
			PauseAsks: slices.Clone(gr.pauseAsks),
		})
		if gr.signal != nil && gr.video.Quality != "" {
			v := gr.video
			out[len(out)-1].Video = &v
		}
		if gr.rec != nil {
			since := gr.rec.StartedAt()
			out[len(out)-1].Recording, out[len(out)-1].RecordingSince = true, &since
		}
	}
	return out
}

// Find returns the room_id of the running game room that roomID, invite or
// code names, or "".
func (r *RoomsService) Find(roomID, invite, code string) string {
	for _, gr := range r.running() {
		r.mu.Lock()
		sig := gr.signal
		r.mu.Unlock()
		if sig == nil {
			continue
		}
		if id := sig.Matches(roomID, invite, code); id != "" {
			return id
		}
	}
	return ""
}

// ownerKey is the running room's key for the host's own browsers.
func ownerKey(sig *TestRoomService) string {
	if sig == nil {
		return ""
	}
	return sig.OwnerKey()
}

// Invite makes a new invitation to a running room: a PIN good for one
// person, to share with the room's link or code.
func (r *RoomsService) Invite(id string) (Pass, error) {
	gr := r.find(id)
	if gr == nil {
		return Pass{}, ErrUnknownRoom
	}
	r.mu.Lock()
	sig := gr.signal
	r.mu.Unlock()
	if sig == nil {
		return Pass{}, ErrRoomState
	}
	return sig.IssuePass(), nil
}

// lobbyExtra adds the pause and the Boxart to the room's lobby entry.
func (r *RoomsService) lobbyExtra(gr *gameRoom) map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	extra := map[string]any{"paused": gr.saved.State == models.RoomPaused}
	if gr.art != "" {
		extra["art"] = gr.art
	}
	return extra
}

// maxArtBytes keeps the Boxart inside signalhub's 4 KB meta (base64 adds
// a third).
const maxArtBytes = 2200

// artKind keeps a valid thumbnail kind, or "" (the one from Settings).
func artKind(kind string) string {
	if thumbnails.Kind(kind).Valid() {
		return kind
	}
	return ""
}

// art is a tiny thumbnail for the lobby: the kind picked for the room, or
// the one from Settings; "" when the host has none.
func (r *RoomsService) art(rom, kind string) string {
	if r.cfg.Library == nil {
		return ""
	}
	k := thumbnails.Kind(kind)
	if !k.Valid() {
		k = r.cfg.Library.ThumbnailKind()
	}
	b, err := r.cfg.Library.Thumbnail(rom, k, 64, 86, maxArtBytes)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(b)
}

// replaceMakerRooms stops and removes the rooms of the Willy Maker game.
func (r *RoomsService) replaceMakerRooms() {
	r.mu.Lock()
	var old []*gameRoom
	var running []bool
	for _, gr := range r.rooms {
		if gr.saved.Rom == MakerRom {
			old = append(old, gr)
			running = append(running, gr.cancel != nil)
		}
	}
	r.mu.Unlock()
	for i, gr := range old {
		if running[i] {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			r.stop(ctx, gr, false, "replaced")
			cancel()
		}
		r.purge(gr)
	}
}

// controlsOf reads a game's control panel from the core's game list. An
// unknown game offers all six buttons rather than too few.
func (r *RoomsService) controlsOf(rom string) GameControls {
	var game *romcheck.Game
	if r.cfg.Library != nil && rom == MakerRom {
		// a Willy Maker game: the buttons its genre uses, named
		m := r.cfg.Library.Maker()
		buttons := len(m.Labels)
		if buttons == 0 {
			buttons = 3
		}
		return GameControls{Players: m.Players, Buttons: buttons, Control: "joy8way", Labels: m.Labels}
	}
	if r.cfg.Library != nil {
		// go-link HD's packages: their players and buttons (a weapon fires with some)
		if m := r.cfg.Library.HD(rom); m != nil {
			return GameControls{Players: m.Players, Buttons: len(m.Labels()), Control: "joy8way", Labels: m.Labels()}
		}
		// go-link's own games name their buttons.
		if s := r.cfg.Library.Own(rom); s != nil {
			return GameControls{Players: s.Players, Buttons: s.Buttons, Control: s.Control, Labels: s.Labels}
		}
		game = r.cfg.Library.Catalog().Game(rom)
	}
	if game == nil || game.Input == (romcheck.Input{}) {
		return GameControls{Buttons: 6, Control: "joy8way"}
	}
	return GameControls{Players: game.Input.Players, Buttons: game.Input.Buttons, Control: game.Input.Control}
}

func (r *RoomsService) title(rom string) string {
	if r.cfg.Library == nil {
		return rom
	}
	return r.cfg.Library.Title(rom)
}

func (r *RoomsService) find(id string) *gameRoom {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, gr := range r.rooms {
		if gr.saved.ID == id {
			return gr
		}
	}
	return nil
}

func (r *RoomsService) remove(gr *gameRoom) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rooms = slices.DeleteFunc(r.rooms, func(x *gameRoom) bool { return x == gr })
}

// running returns the rooms whose game is up.
func (r *RoomsService) running() []*gameRoom {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.runningLocked()
}

func (r *RoomsService) runningLocked() []*gameRoom {
	var out []*gameRoom
	for _, gr := range r.rooms {
		if gr.signal != nil {
			out = append(out, gr)
		}
	}
	return out
}

func (r *RoomsService) countRunningLocked() int {
	n := 0
	for _, gr := range r.rooms {
		if gr.signal != nil {
			n++
		}
	}
	return n
}

// managerOf returns the room's Room Manager while its game runs.
func (r *RoomsService) managerOf(gr *gameRoom) *RoomManager {
	r.mu.Lock()
	defer r.mu.Unlock()
	return gr.manager
}

// RoomTeleLog writes the Room Manager's events to the room's telemetry
// and remembers the names people use.
func RoomTeleLog(tele *telemetry.Recorder) func(kind, peer, text string, data map[string]any) {
	if tele == nil {
		return nil
	}
	return func(kind, peer, text string, data map[string]any) {
		if name, _ := data["name"].(string); peer != "" && name != "" {
			tele.Peer(peer, name)
		}
		tele.Event(telemetry.Info, kind, peer, text, data)
	}
}

// newRoomID makes a stable local id for a room.
// validRoomID reports whether id is what newRoomID makes: hex, no path.
func validRoomID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func newRoomID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
