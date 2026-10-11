// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"errors"
	"sync"

	"github.com/lordbasex/go-link-console/backend-device/pkg/signalclient"
)

// ErrNoSender means there is no way to reach a peer right now.
var ErrNoSender = errors.New("no signaling connection")

// RoutingSender sends each message through the connection its peer came
// by: browsers of the local web panel have their own, everyone else is on
// signalhub. The WebRTC negotiation does not care which one carries it.
type RoutingSender struct {
	mu     sync.Mutex
	base   Sender
	routes map[string]Sender
}

// NewRoutingSender returns a sender with no connection yet.
func NewRoutingSender() *RoutingSender {
	return &RoutingSender{routes: map[string]Sender{}}
}

// SetBase sets the connection for peers without a route (signalhub).
func (r *RoutingSender) SetBase(s Sender) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.base = s
}

// Route sends everything for peer through s.
func (r *RoutingSender) Route(peer string, s Sender) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes[peer] = s
}

// Unroute forgets a peer's own connection.
func (r *RoutingSender) Unroute(peer string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.routes, peer)
}

// Send implements Sender.
func (r *RoutingSender) Send(env signalclient.Envelope) error {
	r.mu.Lock()
	s, ok := r.routes[env.To]
	if !ok {
		s = r.base
	}
	r.mu.Unlock()
	if s == nil {
		return ErrNoSender
	}
	return s.Send(env)
}
