// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"slices"
	"sync"

	"github.com/lordbasex/go-link-console/backend-device/pkg/signalclient"
)

// ICEStore keeps the STUN/TURN servers received from signalhub in memory.
// They are never written to disk: TURN credentials are unique per
// connection and expire, and every reconnection delivers fresh ones in
// hello. The WebRTC layer (a later phase) reads them from here.
type ICEStore struct {
	mu      sync.RWMutex
	servers []signalclient.ICEServer
}

// NewICEStore returns an empty store.
func NewICEStore() *ICEStore { return &ICEStore{} }

// Set replaces the servers. nil clears them.
func (s *ICEStore) Set(servers []signalclient.ICEServer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.servers = cloneServers(servers)
}

// Get returns a deep copy of the current servers.
func (s *ICEStore) Get() []signalclient.ICEServer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneServers(s.servers)
}

// URLs returns every server URL, without credentials.
func (s *ICEStore) URLs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []string
	for _, srv := range s.servers {
		out = append(out, srv.URLs...)
	}
	return out
}

func cloneServers(in []signalclient.ICEServer) []signalclient.ICEServer {
	if in == nil {
		return nil
	}
	out := make([]signalclient.ICEServer, len(in))
	for i, srv := range in {
		srv.URLs = slices.Clone(srv.URLs)
		out[i] = srv
	}
	return out
}
