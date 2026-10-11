// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import "github.com/lordbasex/go-link-console/backend-device/pkg/signalclient"

// Dispatcher fans signalhub events out to several handlers, in order.
type Dispatcher []signalclient.Handler

var _ signalclient.Handler = Dispatcher(nil)

func (d Dispatcher) OnConnect(hello signalclient.Envelope) {
	for _, h := range d {
		h.OnConnect(hello)
	}
}

func (d Dispatcher) OnMessage(env signalclient.Envelope) {
	for _, h := range d {
		h.OnMessage(env)
	}
}

func (d Dispatcher) OnDisconnect(err error) {
	for _, h := range d {
		h.OnDisconnect(err)
	}
}
