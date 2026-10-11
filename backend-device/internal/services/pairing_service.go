// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
	"github.com/lordbasex/go-link-console/backend-device/pkg/signalclient"
)

// Sender is the part of signalclient.Client the pairing logic needs.
type Sender interface {
	Send(env signalclient.Envelope) error
}

// PairingConfig configures PairingService. Zero values get defaults.
type PairingConfig struct {
	DeviceID     string
	DeviceSecret string // sent with register: signalhub binds DeviceID to it
	// RefreshEvery replaces the code before signalhub expires it
	// (server TTL is 10 minutes). Default 9 minutes.
	RefreshEvery time.Duration
	Now          func() time.Time
	Logger       *slog.Logger
}

// PairingService keeps a valid pairing code on screen at all times and
// tracks the browsers linked to this device. It implements
// signalclient.Handler.
// Links opens data connections to linked browsers.
type Links interface {
	AddLink(peerID string) error
	HandleSignal(from string, payload json.RawMessage) error
	RemoveViewer(peerID string)
}

// LinkAuth decides which linked browsers the device trusts.
type LinkAuth interface {
	PairedByCode(peerID string)
	Reached(peerID string)
	Forget(peerID string)
}

type PairingService struct {
	cfg    PairingConfig
	status *StatusService
	ice    *ICEStore
	links  Links
	auth   LinkAuth
	linked map[string]bool

	mu     sync.Mutex
	sender Sender
	timer  *time.Timer
}

var _ signalclient.Handler = (*PairingService)(nil)

// NewPairingService builds the service. Call SetSender before the signal
// client starts.
func NewPairingService(cfg PairingConfig, status *StatusService, ice *ICEStore) *PairingService {
	if cfg.RefreshEvery <= 0 {
		cfg.RefreshEvery = 9 * time.Minute
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &PairingService{cfg: cfg, status: status, ice: ice, linked: make(map[string]bool)}
}

// SetSender wires the signal client. It breaks the construction cycle:
// the client needs the handler and the handler needs the client.
func (p *PairingService) SetSender(s Sender) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sender = s
}

// SetLinks enables WebRTC data connections to linked browsers.
func (p *PairingService) SetLinks(l Links) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.links = l
}

// SetAuth wires the remembered-links check. Without it every linked
// browser is trusted, as with a code.
func (p *PairingService) SetAuth(a LinkAuth) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.auth = a
}

func (p *PairingService) linksFor(peerID string) (Links, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.links, p.linked[peerID]
}

// OnConnect stores the ICE servers and asks for a code as soon as the
// link is up.
func (p *PairingService) OnConnect(hello signalclient.Envelope) {
	p.ice.Set(hello.ICEServers)
	p.status.SetConnected(hello.PeerID, p.ice.URLs())
	p.cfg.Logger.Info("connected to signalhub", "peer_id", hello.PeerID)
	p.register()
}

// OnMessage reacts to signalhub messages addressed to the device.
func (p *PairingService) OnMessage(env signalclient.Envelope) {
	switch env.Type {
	case signalclient.TypeCode:
		now := p.cfg.Now()
		p.status.SetCode(env.Code, now, now.Add(p.cfg.RefreshEvery))
		p.cfg.Logger.Info("pairing code", "code", env.Code)
		p.scheduleRefresh()
	case signalclient.TypePaired:
		p.status.AddPeer(env.Remote, env.SessionID, p.cfg.Now())
		p.cfg.Logger.Info("browser linked", "peer_id", env.Remote)
		if a := p.authFor(); a != nil {
			a.PairedByCode(env.Remote)
		}
		p.openLink(env.Remote)
		// The code was single-use: get a fresh one for the next browser.
		p.register()
	case signalclient.TypeReached:
		// A browser linked before came back without a code. It gets a
		// connection but nothing else until it proves its token.
		p.cfg.Logger.Info("browser back, waiting for its token", "peer_id", env.Remote)
		if a := p.authFor(); a != nil {
			a.Reached(env.Remote)
		}
		p.openLink(env.Remote)
	case signalclient.TypeSignal:
		if links, ok := p.linksFor(env.From); ok && links != nil {
			if err := links.HandleSignal(env.From, env.Payload); err != nil {
				p.cfg.Logger.Warn("bad link signal", "peer_id", env.From, "err", err)
			}
		}
	case signalclient.TypePeerLeft:
		links, ok := p.linksFor(env.From)
		if !ok {
			return
		}
		p.mu.Lock()
		delete(p.linked, env.From)
		auth := p.auth
		p.mu.Unlock()
		if auth != nil {
			auth.Forget(env.From)
		}
		if links != nil {
			links.RemoveViewer(env.From)
		}
		p.status.RemovePeer(env.From)
		p.cfg.Logger.Info("browser disconnected", "peer_id", env.From)
	case signalclient.TypeError:
		p.status.SetError(env.Error)
		p.cfg.Logger.Warn("signalhub error", "error", env.Error)
	default:
		p.cfg.Logger.Debug("message ignored", "type", env.Type)
	}
}

// OnDisconnect clears the state tied to the lost connection.
func (p *PairingService) OnDisconnect(err error) {
	p.stopTimer()
	p.mu.Lock()
	links, linked := p.links, p.linked
	p.linked = make(map[string]bool)
	p.mu.Unlock()
	if links != nil {
		for id := range linked {
			links.RemoveViewer(id)
		}
	}
	p.ice.Set(nil)
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	p.status.SetDisconnected(reason)
	p.cfg.Logger.Warn("signalhub link lost", "err", err)
}

func (p *PairingService) register() {
	p.mu.Lock()
	sender := p.sender
	p.mu.Unlock()
	if sender == nil {
		return
	}
	err := sender.Send(signalclient.Envelope{
		Type:         signalclient.TypeRegister,
		App:          models.App,
		DeviceID:     p.cfg.DeviceID,
		DeviceSecret: p.cfg.DeviceSecret,
	})
	if err != nil {
		p.cfg.Logger.Warn("register failed", "err", err)
	}
}

func (p *PairingService) scheduleRefresh() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.timer != nil {
		p.timer.Stop()
	}
	p.timer = time.AfterFunc(p.cfg.RefreshEvery, p.register)
}

func (p *PairingService) stopTimer() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
}

func (p *PairingService) authFor() LinkAuth {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.auth
}

// openLink starts the data-only WebRTC connection to a linked browser.
func (p *PairingService) openLink(peerID string) {
	p.mu.Lock()
	p.linked[peerID] = true
	links := p.links
	p.mu.Unlock()
	if links != nil {
		if err := links.AddLink(peerID); err != nil {
			p.cfg.Logger.Warn("cannot open data link", "peer_id", peerID, "err", err)
		}
	}
}
