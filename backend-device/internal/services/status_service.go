// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package services holds the device's business logic.
package services

import (
	"slices"
	"sync"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
)

// StatusService keeps the live device state and notifies listeners (the
// tray) when it changes. Snapshots are deep copies.
type StatusService struct {
	mu        sync.Mutex
	st        models.Status
	listeners []func(models.Status)
}

// NewStatusService starts in the "connecting" state.
func NewStatusService(deviceID, version, signalURL, romsDir string) *StatusService {
	return &StatusService{st: models.Status{
		DeviceID: deviceID,
		Version:  version,
		RomsDir:  romsDir,
		Signal:   models.SignalStatus{URL: signalURL, State: models.SignalConnecting},
		Peers:    []models.LinkedPeer{},

		VideoQuality: models.DefaultVideoQuality,
	}}
}

// Snapshot returns a copy of the current state.
func (s *StatusService) Snapshot() models.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clone(s.st)
}

// OnChange registers a listener. It is called after every change, outside
// the lock, with a snapshot.
func (s *StatusService) OnChange(fn func(models.Status)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners, fn)
}

func (s *StatusService) update(mutate func(st *models.Status)) {
	s.mu.Lock()
	mutate(&s.st)
	snap := clone(s.st)
	listeners := slices.Clone(s.listeners)
	s.mu.Unlock()
	for _, fn := range listeners {
		fn(snap)
	}
}

// SetVideoQuality records the host's video quality for game rooms.
func (s *StatusService) SetVideoQuality(q string) {
	s.update(func(st *models.Status) { st.VideoQuality = q })
}

// SetHD publishes go-link HD's screen settings and starter games.
func (s *StatusService) SetHD(hd models.HDStatus) {
	s.update(func(st *models.Status) { st.HD = &hd })
}

// SetConnected records a live signalhub link and the ICE URLs it offered.
func (s *StatusService) SetConnected(peerID string, iceURLs []string) {
	s.update(func(st *models.Status) {
		st.Signal.State = models.SignalConnected
		st.Signal.PeerID = peerID
		st.Signal.Error = ""
		st.Signal.ICEURLs = slices.Clone(iceURLs)
	})
}

// SetDisconnected clears everything tied to the old connection: the code
// and the linked peers die with it on the server side.
func (s *StatusService) SetDisconnected(reason string) {
	s.update(func(st *models.Status) {
		st.Signal.State = models.SignalDisconnected
		st.Signal.PeerID = ""
		st.Signal.SessionID = ""
		st.Signal.Error = reason
		st.Signal.ICEURLs = nil
		st.Pairing = models.PairingStatus{}
		st.Peers = []models.LinkedPeer{}
		st.Room = nil
	})
}

// SetError records a protocol error without changing the link state.
func (s *StatusService) SetError(msg string) {
	s.update(func(st *models.Status) { st.Signal.Error = msg })
}

// SetCode shows a new pairing code.
func (s *StatusService) SetCode(code string, issued, refreshes time.Time) {
	s.update(func(st *models.Status) {
		st.Pairing = models.PairingStatus{Code: code, IssuedAt: &issued, RefreshesAt: &refreshes}
		st.Signal.Error = ""
	})
}

// SetRoomDetails records the open room as the lobby sees it.
func (s *StatusService) SetRoomDetails(room models.RoomStatus) {
	s.update(func(st *models.Status) {
		if room.RoomID == "" {
			st.Room = nil
			return
		}
		r := room
		st.Room = &r
	})
}

// SetRooms records the game rooms (the whole list).
func (s *StatusService) SetRooms(rooms []models.ManagedRoom) {
	s.update(func(st *models.Status) { st.Rooms = slices.Clone(rooms) })
}

// SetSavesBytes records the space the saved games take.
func (s *StatusService) SetSavesBytes(n int64) {
	s.update(func(st *models.Status) { st.SavesBytes = n })
}

// SetStream records what is being streamed.
func (s *StatusService) SetStream(stream models.StreamStatus) {
	s.update(func(st *models.Status) { st.Stream = stream })
}

// SetRoom records the open room; an empty id means no room.
func (s *StatusService) SetRoom(roomID string, viewers int) {
	s.update(func(st *models.Status) {
		if roomID == "" {
			st.Room = nil
			return
		}
		st.Room = &models.RoomStatus{RoomID: roomID, Viewers: viewers}
	})
}

// SetSystem records hardware and usage.
func (s *StatusService) SetSystem(sys models.SystemStatus) {
	s.update(func(st *models.Status) { st.System = &sys })
}

// SetLibrary records the ROM folder contents and download progress.
func (s *StatusService) SetLibrary(lib models.Library) {
	s.update(func(st *models.Status) {
		st.Library = &lib
		st.RomsDir = lib.Dir
	})
}

// SetPeerLatency records the round trip to a linked browser.
func (s *StatusService) SetPeerLatency(peerID string, ms int) {
	s.update(func(st *models.Status) {
		for i := range st.Peers {
			if st.Peers[i].PeerID == peerID {
				v := ms
				st.Peers[i].LatencyMs = &v
			}
		}
	})
}

// SetSavedLinks records how many browsers are remembered.
// SetUpdate records a newer go-link release (nil: none).
func (s *StatusService) SetUpdate(u *models.UpdateInfo) {
	s.update(func(st *models.Status) { st.Update = u })
}

func (s *StatusService) SetSavedLinks(n int) {
	s.update(func(st *models.Status) { st.SavedLinks = n })
}

// AddPeer records a linked browser.
func (s *StatusService) AddPeer(peerID, sessionID string, since time.Time) {
	s.update(func(st *models.Status) {
		st.Signal.SessionID = sessionID
		if !slices.ContainsFunc(st.Peers, func(p models.LinkedPeer) bool { return p.PeerID == peerID }) {
			st.Peers = append(st.Peers, models.LinkedPeer{PeerID: peerID, Since: since})
		}
	})
}

// RemovePeer forgets a browser that left.
func (s *StatusService) RemovePeer(peerID string) {
	s.update(func(st *models.Status) {
		st.Peers = slices.DeleteFunc(st.Peers, func(p models.LinkedPeer) bool { return p.PeerID == peerID })
	})
}

func clone(st models.Status) models.Status {
	st.Signal.ICEURLs = slices.Clone(st.Signal.ICEURLs)
	if st.Signal.ICEURLs == nil {
		st.Signal.ICEURLs = []string{}
	}
	st.Peers = slices.Clone(st.Peers)
	for i := range st.Peers {
		if st.Peers[i].LatencyMs != nil {
			v := *st.Peers[i].LatencyMs
			st.Peers[i].LatencyMs = &v
		}
	}
	if st.System != nil {
		sys := *st.System
		st.System = &sys
	}
	if st.Library != nil {
		// Roms is replaced whole by each scan, never edited, so the copy
		// shares it (a large folder holds thousands of sets).
		lib := *st.Library
		st.Library = &lib
	}
	if st.HD != nil {
		hd := *st.HD
		hd.Starter = slices.Clone(hd.Starter)
		st.HD = &hd
	}
	if st.Peers == nil {
		st.Peers = []models.LinkedPeer{}
	}
	// Rooms are replaced whole by SetRooms, never edited in place, so a
	// shallow copy of the slice is enough.
	st.Rooms = slices.Clone(st.Rooms)
	if st.Rooms == nil {
		st.Rooms = []models.ManagedRoom{}
	}
	if st.Room != nil {
		r := *st.Room
		st.Room = &r
	}
	if st.Pairing.IssuedAt != nil {
		t := *st.Pairing.IssuedAt
		st.Pairing.IssuedAt = &t
	}
	if st.Pairing.RefreshesAt != nil {
		t := *st.Pairing.RefreshesAt
		st.Pairing.RefreshesAt = &t
	}
	return st
}
