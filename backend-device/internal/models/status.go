// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package models

import (
	"time"

	"github.com/lordbasex/go-link/backend-device/pkg/romcheck"
	"github.com/lordbasex/go-link/backend-device/pkg/sysinfo"
)

// Signal connection states.
const (
	SignalConnecting   = "connecting"
	SignalConnected    = "connected"
	SignalDisconnected = "disconnected"
)

// Status is the device state shown by the window and, reduced, sent to
// linked browsers in device_status.
type Status struct {
	DeviceID string        `json:"device_id"`
	Version  string        `json:"version"`
	RomsDir  string        `json:"roms_dir"`
	Signal   SignalStatus  `json:"signal"`
	Pairing  PairingStatus `json:"pairing"`
	Peers    []LinkedPeer  `json:"peers"`
	// SavedLinks counts the browsers remembered in device.json. With none,
	// the window only shows the pairing code.
	SavedLinks int `json:"saved_links"`
	// Stream is only for the window: it is not sent to browsers.
	Stream StreamStatus `json:"-"`
	Room   *RoomStatus  `json:"room,omitempty"`
	// Rooms are the game rooms, besides the test pattern room.
	Rooms []ManagedRoom `json:"rooms"`
	// SavesBytes is the space the rooms' saved games take.
	SavesBytes int64         `json:"saves_bytes"`
	System     *SystemStatus `json:"system,omitempty"`
	Library    *Library      `json:"library,omitempty"`
	// Update is a newer go-link release, when there is one.
	Update *UpdateInfo `json:"update,omitempty"`
	// VideoQuality is the host's choice for game rooms (high, normal or
	// saver); each room's Video says what it really sends.
	VideoQuality string `json:"video_quality"`
	// HD is go-link HD's screen settings and its starter games.
	HD *HDStatus `json:"hd,omitempty"`
}

// HDStatus is go-link HD's side of the device: its rooms' screen and the
// free starter games it can download.
type HDStatus struct {
	RoomSize string        `json:"room_size"` // 720p or 1080p
	Codec    string        `json:"codec"`     // auto or vp8
	Starter  []StarterGame `json:"starter"`
}

// StarterGame is a free game go-link HD can download into its games
// folder.
type StarterGame struct {
	Name      string `json:"name"`
	Title     string `json:"title"`
	Installed bool   `json:"installed"`
	// Size is the screen size of the installed package (720p or 1080p).
	Size        string `json:"size,omitempty"`
	Downloading string `json:"downloading,omitempty"` // the size being downloaded
	Done        int64  `json:"done,omitempty"`
	Total       int64  `json:"total,omitempty"`
	Error       string `json:"error,omitempty"`
}

// UpdateInfo is a newer release of go-link than this build.
type UpdateInfo struct {
	Latest string `json:"latest"` // its version, e.g. v0.2.0
	URL    string `json:"url"`    // its release page, with the downloads
}

// Library is the host's ROM folder.
type Library struct {
	Dir string `json:"dir"`
	// Roms is every set in the folder, sorted by name. It is never sent
	// whole: a large folder does not fit one WebRTC message, so browsers
	// get Summary and ask for pages (roms_query) or sets by name
	// (roms_get). Scan replaces the slice, never edits it, so it is shared.
	Roms    []RomInfo      `json:"-"`
	Summary LibrarySummary `json:"summary"`
	Core    CoreStatus     `json:"core"`
	Disk    *sysinfo.Disk  `json:"disk,omitempty"` // the volume of Dir
	// HDDir is go-link HD's games folder; HDGames and HDBytes count the
	// packages (.glhd) in it.
	HDDir   string `json:"hd_dir"`
	HDGames int    `json:"hd_games"`
	HDBytes int64  `json:"hd_bytes"`
	// ThumbnailsDir holds the host's thumbnails, one folder per kind.
	ThumbnailsDir string `json:"thumbnails_dir"`
	// ThumbKind is the kind of thumbnail the host chose to show (boxart,
	// title or snap); browsers show the same one.
	ThumbKind string `json:"thumb_kind"`
	// ThumbnailsBytes is the space the thumbnail images take.
	ThumbnailsBytes int64 `json:"thumbnails_bytes"`
}

// The states of a set, as the website groups them, in their status order.
const (
	RomRuns        = "runs"
	RomMissing     = "missing"
	RomUnsupported = "unsupported"
	RomBroken      = "broken"
	RomBIOS        = "bios"
	RomUnchecked   = "unchecked"
)

// RomKinds lists the states in the order the status sort uses.
var RomKinds = []string{RomRuns, RomMissing, RomUnsupported, RomBroken, RomBIOS, RomUnchecked}

// LibrarySummary counts the ROM folder for device_status.
type LibrarySummary struct {
	// Revision changes with every scan: a browser asks for its pages again.
	Revision int64          `json:"revision"`
	Total    int            `json:"total"`
	Bytes    int64          `json:"bytes"`
	Playable int            `json:"playable"`
	Kinds    map[string]int `json:"kinds"` // sets per state (RomKinds)
	Biggest  []RomBrief     `json:"biggest"`
	// Thumbs counts the sets with each kind of the host's thumbnails.
	Thumbs ThumbCounts `json:"thumbs"`
}

// ThumbCounts counts sets per kind of thumbnail.
type ThumbCounts struct {
	Boxart int `json:"boxart"`
	Title  int `json:"title"`
	Snap   int `json:"snap"`
}

// RomBrief names a set and its size.
type RomBrief struct {
	Name  string `json:"name"`
	Title string `json:"title,omitempty"`
	Size  int64  `json:"size"`
}

// CoreStatus is the libretro emulator core on this machine.
type CoreStatus struct {
	Name        string `json:"name"`
	Installed   bool   `json:"installed"`
	Downloading bool   `json:"downloading"`
	// Catalog is the core's game list, used to check ROMs without
	// running them. It is downloaded with the core.
	Catalog bool   `json:"catalog"`
	Error   string `json:"error,omitempty"`
	// HDInstalled tells whether go-link HD's core (for .glhd packages) is
	// on the device.
	HDInstalled bool `json:"hd_installed,omitempty"`
}

// KindHD marks a go-link HD game package (.glhd) in the library.
const KindHD = "glhd"

// RomInfo is one ROM set (a .zip) or go-link HD game package (.glhd,
// Kind "glhd") in the folder.
type RomInfo struct {
	Name string `json:"name"`
	// Kind is empty for a MAME set and KindHD for a go-link HD package,
	// whose Title, Description and Controls come from its manifest.
	Kind  string `json:"kind,omitempty"`
	Size  int64  `json:"size"`
	Title string `json:"title,omitempty"`
	Year  string `json:"year,omitempty"`
	Maker string `json:"maker,omitempty"`
	// Thumbs tells which of the host's thumbnails exist for the set.
	Thumbs Thumbs `json:"thumbs"`
	// Check tells whether the core can run the set; nil until the core's
	// game list is downloaded.
	Check *romcheck.Result `json:"check,omitempty"`
	// Own is set for a set go-link made itself, recognized by the SHA-256
	// of every file inside the zip (never by its name): Title, Year, Maker,
	// Description and Controls are then go-link's, not the original set's.
	Own         bool         `json:"own,omitempty"`
	Description string       `json:"description,omitempty"`
	Controls    *RomControls `json:"controls,omitempty"`
}

// RomControls is a game's control panel, as the library shows it.
type RomControls struct {
	Players int      `json:"players"`
	Buttons int      `json:"buttons"`
	Labels  []string `json:"labels,omitempty"` // what each button does, button 1 first
}

// SystemStatus is the machine's hardware and live usage.
type SystemStatus struct {
	Hardware  sysinfo.Hardware `json:"hardware"`
	Usage     sysinfo.Usage    `json:"usage"`
	SampledAt time.Time        `json:"sampled_at"`
}

// RoomStatus is the room this device has open on signalhub.
type RoomStatus struct {
	RoomID     string `json:"room_id"`
	Viewers    int    `json:"viewers"`
	Title      string `json:"title,omitempty"`
	Game       string `json:"game,omitempty"`
	Public     bool   `json:"public"`
	Players    int    `json:"players"`
	MaxPlayers int    `json:"max_players"`
	Queue      int    `json:"queue"`
	Spectators int    `json:"spectators"`
	// The room's invitation, and the key the host's own browsers send
	// instead of a PIN (the owner's browsers only).
	Invite     string `json:"invite,omitempty"`
	InviteCode string `json:"invite_code,omitempty"`
	OwnerKey   string `json:"owner_key,omitempty"`
	// Picture is the test pattern room's default picture style (nil: the
	// website's own default).
	Picture *RoomPicture `json:"picture,omitempty"`
}

// StreamStatus is what the device is sending right now.
type StreamStatus struct {
	FPS          float64 `json:"fps"`
	Width        int     `json:"width"`
	Height       int     `json:"height"`
	VideoKbps    float64 `json:"video_kbps"` // encoded video, before each viewer's copy
	VideoViewers int     `json:"video_viewers"`
	Scale        int     `json:"scale"` // 2 when the picture is sent enlarged 2x
}

// SignalStatus describes the link with signalhub.
type SignalStatus struct {
	URL       string `json:"url"`
	State     string `json:"state"`
	PeerID    string `json:"peer_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Error     string `json:"error,omitempty"`
	// ICEURLs lists the STUN/TURN URLs received in hello. Credentials are
	// never exposed through the panel.
	ICEURLs []string `json:"ice_urls"`
}

// PairingStatus is the code shown to the user. The device replaces it
// before it expires and right after someone redeems it.
type PairingStatus struct {
	Code        string     `json:"code,omitempty"` // "113 134 323"
	IssuedAt    *time.Time `json:"issued_at,omitempty"`
	RefreshesAt *time.Time `json:"refreshes_at,omitempty"`
}

// LinkedPeer is a browser linked to this device's session.
type LinkedPeer struct {
	PeerID string    `json:"peer_id"`
	Since  time.Time `json:"since"`
	// LatencyMs is the round trip over the WebRTC DataChannel, when known.
	LatencyMs *int `json:"latency_ms,omitempty"`
}

// Thumbs tells which thumbnails a set has (Boxart, Title, Snap).
type Thumbs struct {
	Boxart bool `json:"boxart"`
	Title  bool `json:"title"`
	Snap   bool `json:"snap"`
}
