// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"testing"

	"github.com/lordbasex/go-link-console/backend-device/pkg/signalclient"
)

type countingSender struct{ to []string }

func (c *countingSender) Send(env signalclient.Envelope) error {
	c.to = append(c.to, env.To)
	return nil
}

func TestPanelPeersGetTheirOwnConnection(t *testing.T) {
	r := NewRoutingSender()
	if err := r.Send(signalclient.Envelope{To: "a"}); err != ErrNoSender {
		t.Fatalf("no connection: %v", err)
	}
	hub, panel := &countingSender{}, &countingSender{}
	r.SetBase(hub)
	r.Route("p", panel)
	_ = r.Send(signalclient.Envelope{To: "a"})
	_ = r.Send(signalclient.Envelope{To: "p"})
	r.Unroute("p")
	_ = r.Send(signalclient.Envelope{To: "p"})
	if len(panel.to) != 1 || len(hub.to) != 2 {
		t.Fatalf("hub %v panel %v", hub.to, panel.to)
	}
}
