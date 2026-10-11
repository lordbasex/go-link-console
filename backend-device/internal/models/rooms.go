// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package models

import "time"

// DefaultMaxRooms is how many games can run at once unless device.json
// says otherwise.
const DefaultMaxRooms = 4

// TrashDays is how long a deleted room can be restored.
const TrashDays = 30

// Room states. Live and paused rooms have a running game (a process);
// archived and trashed ones only keep their saved games.
const (
	RoomLive     = "live"
	RoomPaused   = "paused"
	RoomArchived = "archived"
	RoomTrash    = "trash"
)

// SaveSlot is a game the players saved on purpose.
type SaveSlot struct {
	Slot int       `json:"slot"`
	Name string    `json:"name,omitempty"`
	At   time.Time `json:"at"`
}

// SavedRoom is one game room as device.json keeps it.
type SavedRoom struct {
	ID     string `json:"id"` // stable, unlike the signalhub room_id
	Name   string `json:"name"`
	Rom    string `json:"rom"`
	Public bool   `json:"public"`
	Voice  bool   `json:"voice"`
	// ChatOff: the host turned the room's chat off (on by default).
	ChatOff bool `json:"chat_off,omitempty"`
	// SpectatorsVoiceOff: the host does not let spectators hear the
	// voices (they hear them by default).
	SpectatorsVoiceOff bool   `json:"spectators_voice_off,omitempty"`
	Art                string `json:"art,omitempty"` // lobby picture: boxart, title or snap ("" = Settings)
	// Picture is the host's default picture style for the room's guests
	// (nil: the website's own default).
	Picture   *RoomPicture `json:"picture,omitempty"`
	State     string       `json:"state"`
	Favorite  bool         `json:"favorite,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
	Since     time.Time    `json:"since"` // when it entered its current state
	DeletedAt *time.Time   `json:"deleted_at,omitempty"`
	Saves     []SaveSlot   `json:"saves,omitempty"`
	Autosave  bool         `json:"autosave,omitempty"` // auto.state exists
	LastError string       `json:"last_error,omitempty"`
	// NoSaves: the emulator does not save this game whole (MAME 0.78 keeps
	// no registers for some CPUs, like Konami's), so it cannot resume
	// from a save: it always starts from power on.
	NoSaves bool `json:"no_saves,omitempty"`
}

// Running reports whether the room has a game process.
func (r SavedRoom) Running() bool { return r.State == RoomLive || r.State == RoomPaused }

// ManagedRoom is a game room as linked browsers see it (device_status).
type ManagedRoom struct {
	SavedRoom
	Game       string `json:"game"`    // the game's title
	RoomID     string `json:"room_id"` // signalhub room, while it runs
	Players    int    `json:"players"`
	MaxPlayers int    `json:"max_players"`
	Spectators int    `json:"spectators"`
	Queue      int    `json:"queue"`
	// The room's current invitation from signalhub, while it runs: the
	// invite goes in links and QR codes, the 9 digit code is typed.
	Invite     string `json:"invite,omitempty"`
	InviteCode string `json:"invite_code,omitempty"`
	// OwnerKey is what the host's own browsers send instead of a PIN
	// (guests get a PIN per invitation, see RoomsService.Invite).
	OwnerKey string `json:"owner_key,omitempty"`
	// GroupInvite is the room's group invitation, while one is alive, and
	// Knocks the people that used it and wait for the host to let them in.
	GroupInvite *GroupInvite `json:"group_invite,omitempty"`
	Knocks      []Knock      `json:"knocks,omitempty"`
	// Recording is set while the host records the game, since
	// RecordingSince.
	Recording      bool       `json:"recording,omitempty"`
	RecordingSince *time.Time `json:"recording_since,omitempty"`
	// PauseAsks are the players asking the host for a pause, waiting for
	// an answer (pause_answer).
	PauseAsks []PauseAsk `json:"pause_asks,omitempty"`
	// Video is what the running game sends: the quality in use and why it
	// is lower than the host's choice (nil while it does not run).
	Video *RoomVideo `json:"video,omitempty"`
}

// PauseAsk is a seated player asking the host for a pause: from is the
// player's peer id, what pause_answer names.
type PauseAsk struct {
	From      string    `json:"from"`
	Name      string    `json:"name"`
	Port      int       `json:"port"`
	ExpiresAt time.Time `json:"expires_at"`
}

// GroupInvite is an invitation for several people: one link and QR code
// (the key goes after # in the link, never to a server), good for Uses
// people until ExpiresAt. With Approval the host lets each one in.
type GroupInvite struct {
	Key       string    `json:"key"`
	Uses      int       `json:"uses"` // how many people it lets in
	Used      int       `json:"used"`
	ExpiresAt time.Time `json:"expires_at"`
	Approval  bool      `json:"approval"`
}

// Knock is someone who came with a group invitation and waits for the
// host's answer (GroupInvite.Approval).
type Knock struct {
	Peer  string    `json:"peer"`
	Name  string    `json:"name"`
	Since time.Time `json:"since"`
}
