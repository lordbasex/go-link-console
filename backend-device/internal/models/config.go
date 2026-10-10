// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package models holds the device's domain types.
package models

import "time"

// App is the signalhub namespace of this project.
const App = "go-link"

// DefaultSignalURL is the public signalhub operated by the project. Users
// who run their own server override it with --server-signaling or with
// signal_url in device.json. STUN and TURN are never configured here: the
// signaling server sends them in its first message (hello).
const DefaultSignalURL = "wss://signal.go-link.org/ws"

// DefaultWebURL is the website where hosts type the pairing code: the rooms'
// site (the landing, go-link.org, sends its /device there too).
const DefaultWebURL = "https://play.go-link.org"

// Config is persisted in device.json with 0600 permissions.
type Config struct {
	DeviceID string `json:"device_id"`
	// DeviceSecret binds the device_id to this device on signalhub (sent
	// with register): 32 random bytes, base64url. Made on first run.
	DeviceSecret string `json:"device_secret,omitempty"`
	SignalURL    string `json:"signal_url,omitempty"`
	RomsDir      string `json:"roms_dir,omitempty"`
	WebURL       string `json:"web_url,omitempty"`
	// UDPPort carries every WebRTC connection on one UDP port, so a single
	// port forward on a router opens a direct path (0: random ports).
	UDPPort int `json:"udp_port,omitempty"`
	// AnnounceIPs are addresses where browsers reach UDPPort through a
	// router that forwards it: the second router's WAN side at home, or
	// the public IP for players outside.
	AnnounceIPs []string `json:"announce_ips,omitempty"`
	// Links are the browsers linked with a pairing code. They come back
	// without a code; only a hash of their secret is kept.
	Links []Link `json:"links,omitempty"`
	// Rooms are the host's game rooms (live, paused, archived or in the
	// trash); live and paused ones start again with the device.
	Rooms []SavedRoom `json:"rooms,omitempty"`
	// Thumbnails are how the host's game images are shown (Settings).
	Thumbnails ThumbnailSettings `json:"thumbnails,omitempty"`
	// MaxRooms caps the rooms with a running game (live or paused).
	// Zero means DefaultMaxRooms.
	MaxRooms int `json:"max_rooms,omitempty"`
	// PanelToken is the UUID v4 the local web panel asks for (headless
	// devices: Raspberry Pi, Docker). The device makes it the first time
	// the panel runs; `device panel token --new` replaces it.
	PanelToken string `json:"panel_token,omitempty"`
	// Language of the device window: en, es or pt; empty follows the
	// computer's language.
	Language string `json:"language,omitempty"`
	// TestRoomPicture is the host's default picture style for the test
	// pattern room (nil: the website's own default).
	TestRoomPicture *RoomPicture `json:"test_room_picture,omitempty"`
	// VideoQuality of game rooms: high, normal or saver (see VideoHigh);
	// empty means DefaultVideoQuality.
	VideoQuality string `json:"video_quality,omitempty"`
	// HD is go-link HD's settings: its games folder and its rooms' screen.
	HD HDSettings `json:"hd,omitempty"`
	// StarterGames is set once the free starter games were offered (on the
	// first link): a game the host deletes is never downloaded again.
	StarterGames bool `json:"starter_games,omitempty"`
}

// HDSettings are go-link HD's settings. Empty fields mean the defaults.
type HDSettings struct {
	GamesDir string `json:"games_dir,omitempty"` // default ~/go-link/hd-games
	RoomSize string `json:"room_size,omitempty"` // 720p (default) or 1080p
	Codec    string `json:"codec,omitempty"`     // auto (default) or vp8
}

// go-link HD rooms' screen sizes and codecs (HDSettings).
const (
	HD720p  = "720p"
	HD1080p = "1080p"
	HDAuto  = "auto"
	HDVP8   = "vp8"
)

// HDRoomSizes and HDCodecs are the choices of HDSettings, the default first.
var (
	HDRoomSizes = []string{HD720p, HD1080p}
	HDCodecs    = []string{HDAuto, HDVP8}
)

// Link is one browser linked to this device.
type Link struct {
	ID        string    `json:"id"`
	TokenHash string    `json:"token_hash"` // hex SHA-256 of the token the browser keeps
	CreatedAt time.Time `json:"created_at"`
	LastSeen  time.Time `json:"last_seen"`
	// Terms is the version of the terms of use the browser's user accepted
	// when linking (or later), and TermsAt when the device learned it: the
	// host's acceptance, kept on the host's own computer.
	Terms   string    `json:"terms,omitempty"`
	TermsAt time.Time `json:"terms_at,omitempty"`
}

// FactoryDefaults is the config after a factory reset: a fresh install's,
// keeping only what makes this device itself and reachable, so it can be
// linked again right away: its identity (device_id, device_secret), its
// signaling server, its network settings (udp_port, announce_ips) and the
// local panel's token. Links, rooms, the ROM and thumbnail folders, the
// room limit, the website, the language, the video quality and go-link HD's
// settings go back to their defaults (the starter games are offered again).
func (c Config) FactoryDefaults() Config {
	return Config{
		DeviceID: c.DeviceID, DeviceSecret: c.DeviceSecret, SignalURL: c.SignalURL,
		UDPPort: c.UDPPort, AnnounceIPs: c.AnnounceIPs, PanelToken: c.PanelToken,
	}
}

// EffectiveWebURL returns the configured website or the default.
func (c Config) EffectiveWebURL() string {
	if c.WebURL != "" {
		return c.WebURL
	}
	return DefaultWebURL
}

// EffectiveSignalURL returns the configured URL or the default.
func (c Config) EffectiveSignalURL() string {
	if c.SignalURL != "" {
		return c.SignalURL
	}
	return DefaultSignalURL
}

// ThumbnailSettings say where the host's thumbnails are and which kind to
// show. Empty fields mean the defaults.
type ThumbnailSettings struct {
	Dir  string `json:"dir,omitempty"`  // default ~/go-link/thumbnails/MAME
	Kind string `json:"kind,omitempty"` // boxart (default), title or snap
	Size string `json:"size,omitempty"` // small (default), medium or large, in the device window
}

// ThumbnailSizes are the choices of ThumbnailSettings.Size.
var ThumbnailSizes = []string{"small", "medium", "large"}
