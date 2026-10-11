// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
)

// Messages on the control channel of a linked browser.
const (
	msgAuth       = "auth"
	msgAuthOK     = "auth_ok"
	msgAuthFailed = "auth_failed"
	msgUnlink     = "unlink"
	// A remembered browser first asks the device to prove itself, so its
	// token never goes to whoever answers for the device_id on signalhub.
	msgChallenge = "auth_challenge"
	msgProof     = "auth_proof"
	msgUnlinked  = "unlinked"
)

// errNotLinked is shown by the website when a remembered link is gone.
const errNotLinked = "This browser is no longer linked to the device. Link it again with a new code."

// LinkConfig configures LinkService. Zero values get defaults.
type LinkConfig struct {
	DeviceID    string
	Links       []models.Link             // loaded from device.json
	Save        func([]models.Link) error // persists the links
	AuthTimeout time.Duration             // default 15 s, for peers that came with reach
	HangUpDelay time.Duration             // default 500 ms, after the last message to a dropped peer
	Now         func() time.Time
	Logger      *slog.Logger
}

// LinkService remembers the browsers linked with a pairing code, so they
// come back without one. On the first link the device gives the browser a
// random token over the encrypted WebRTC channel and keeps only its hash.
// A browser that comes back with reach proves itself with that token
// before it gets anything from the device.
type LinkService struct {
	cfg    LinkConfig
	status *StatusService

	mu      sync.Mutex
	links   []models.Link
	trusted map[string]string // peer_id -> link id ("" until it got a token)
	timers  map[string]*time.Timer
	send    func(peerID string, msg []byte) bool
	drop    func(peerID string)
	// onChange runs (outside the lock) when a browser becomes trusted or
	// is forgotten.
	onChange func()
}

// OnChange registers who is told that the trusted browsers changed.
func (l *LinkService) OnChange(fn func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onChange = fn
}

// changed runs the OnChange callback. Never call it holding l.mu.
func (l *LinkService) changed() {
	l.mu.Lock()
	fn := l.onChange
	l.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// NewLinkService builds the service.
func NewLinkService(cfg LinkConfig, status *StatusService) *LinkService {
	if cfg.AuthTimeout <= 0 {
		cfg.AuthTimeout = 15 * time.Second
	}
	if cfg.HangUpDelay <= 0 {
		cfg.HangUpDelay = 500 * time.Millisecond
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Save == nil {
		cfg.Save = func([]models.Link) error { return nil }
	}
	l := &LinkService{
		cfg:     cfg,
		status:  status,
		links:   slices.Clone(cfg.Links),
		trusted: map[string]string{},
		timers:  map[string]*time.Timer{},
	}
	status.SetSavedLinks(len(l.links))
	return l
}

// SetTransport wires the control channel: send a message to a linked
// browser, and drop its connection.
func (l *LinkService) SetTransport(send func(peerID string, msg []byte) bool, drop func(peerID string)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.send, l.drop = send, drop
}

// Links returns the remembered browsers.
func (l *LinkService) Links() []models.Link {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.links)
}

// Linked reports whether at least one browser is remembered.
func (l *LinkService) Linked() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.links) > 0
}

// Trusted reports whether a peer may use the device: it redeemed a code
// now, or proved its token.
func (l *LinkService) Trusted(peerID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, ok := l.trusted[peerID]
	return ok
}

// PairedByCode trusts a browser that just redeemed the pairing code. It
// gets its token when it sends auth.
func (l *LinkService) PairedByCode(peerID string) {
	l.mu.Lock()
	l.trusted[peerID] = ""
	l.mu.Unlock()
	l.changed()
}

// PanelLinkID marks a browser that came through the local web panel with
// the panel token: trusted while it stays, never remembered as a link.
const PanelLinkID = "panel"

// PairedByPanel trusts a browser of the local web panel, which proved the
// panel token over its own connection.
func (l *LinkService) PairedByPanel(peerID string) {
	l.mu.Lock()
	l.trusted[peerID] = PanelLinkID
	l.mu.Unlock()
	l.changed()
}

// Reached starts the clock for a browser that came back with reach: it
// must prove its token in time or it is dropped.
func (l *LinkService) Reached(peerID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if t := l.timers[peerID]; t != nil {
		t.Stop()
	}
	l.timers[peerID] = time.AfterFunc(l.cfg.AuthTimeout, func() {
		if !l.Trusted(peerID) {
			l.cfg.Logger.Warn("browser did not authenticate", "peer_id", peerID)
			l.dropPeer(peerID)
		}
	})
}

// Forget clears a browser that left.
func (l *LinkService) Forget(peerID string) {
	l.mu.Lock()
	delete(l.trusted, peerID)
	if t := l.timers[peerID]; t != nil {
		t.Stop()
		delete(l.timers, peerID)
	}
	l.mu.Unlock()
	l.changed()
}

type linkMessage struct {
	Type   string `json:"type"`
	LinkID string `json:"link_id,omitempty"`
	Token  string `json:"token,omitempty"`
	Nonce  string `json:"nonce,omitempty"`
	// Terms: the version of the terms of use accepted in this browser.
	Terms string `json:"terms,omitempty"`
}

// validTerms accepts a terms version as the website sends it (a date).
func validTerms(v string) bool {
	if len(v) == 0 || len(v) > 20 {
		return false
	}
	for _, r := range v {
		if (r < '0' || r > '9') && r != '-' && r != '.' {
			return false
		}
	}
	return true
}

// HandleMessage processes auth and unlink. It reports false for other
// messages, which the caller may process only for trusted peers.
func (l *LinkService) HandleMessage(peerID string, data []byte) bool {
	var m linkMessage
	if json.Unmarshal(data, &m) != nil {
		return false
	}
	switch m.Type {
	case msgChallenge:
		l.prove(peerID, m)
	case msgAuth:
		l.auth(peerID, m)
	case msgUnlink:
		l.unlink(peerID)
	default:
		return false
	}
	return true
}

// LinkProof is what the device answers to a remembered browser's nonce:
// HMAC-SHA256 keyed with SHA-256(token), which only the real device keeps.
func LinkProof(tokenHash []byte, nonce string) string {
	mac := hmac.New(sha256.New, tokenHash)
	mac.Write([]byte("go-link link proof\n" + nonce))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// prove answers auth_challenge: the proof that this is the device the
// browser linked to, before the browser shows its token.
func (l *LinkService) prove(peerID string, m linkMessage) {
	if len(m.Nonce) < 16 || len(m.Nonce) > 128 {
		return
	}
	l.mu.Lock()
	i := slices.IndexFunc(l.links, func(r models.Link) bool { return r.ID == m.LinkID })
	var hash []byte
	if i >= 0 {
		hash, _ = hex.DecodeString(l.links[i].TokenHash)
	}
	l.mu.Unlock()
	if len(hash) != sha256.Size {
		l.reply(peerID, map[string]string{"type": msgAuthFailed, "error": errNotLinked})
		l.dropLater(peerID)
		return
	}
	l.reply(peerID, map[string]string{"type": msgProof, "link_id": m.LinkID, "proof": LinkProof(hash, m.Nonce)})
}

func (l *LinkService) auth(peerID string, m linkMessage) {
	l.mu.Lock()
	linkID, trusted := l.trusted[peerID]
	switch {
	case trusted && linkID == "":
		// Just redeemed a code: remember this browser.
		token, rec, err := newLink(l.cfg.Now())
		if err != nil {
			l.mu.Unlock()
			l.cfg.Logger.Error("cannot create a link", "err", err)
			return
		}
		if validTerms(m.Terms) {
			rec.Terms, rec.TermsAt = m.Terms, l.cfg.Now()
		}
		l.links = append(l.links, rec)
		l.trusted[peerID] = rec.ID
		links := slices.Clone(l.links)
		l.mu.Unlock()
		l.persist(links)
		l.cfg.Logger.Info("browser remembered", "link_id", rec.ID, "terms", rec.Terms)
		l.reply(peerID, map[string]string{"type": msgAuthOK, "device_id": l.cfg.DeviceID, "link_id": rec.ID, "token": token})
		return
	case trusted:
		l.mu.Unlock()
		l.reply(peerID, map[string]string{"type": msgAuthOK, "device_id": l.cfg.DeviceID, "link_id": linkID})
		return
	}
	i := slices.IndexFunc(l.links, func(r models.Link) bool { return r.ID == m.LinkID })
	if i < 0 || !tokenMatches(l.links[i].TokenHash, m.Token) {
		l.mu.Unlock()
		l.cfg.Logger.Warn("browser failed to authenticate", "peer_id", peerID)
		l.reply(peerID, map[string]string{"type": msgAuthFailed, "error": errNotLinked})
		l.dropLater(peerID)
		return
	}
	l.links[i].LastSeen = l.cfg.Now()
	if validTerms(m.Terms) && m.Terms != l.links[i].Terms {
		l.links[i].Terms, l.links[i].TermsAt = m.Terms, l.cfg.Now()
	}
	l.trusted[peerID] = m.LinkID
	if t := l.timers[peerID]; t != nil {
		t.Stop()
		delete(l.timers, peerID)
	}
	links := slices.Clone(l.links)
	l.mu.Unlock()
	l.persist(links)
	l.changed()
	l.status.AddPeer(peerID, "", l.cfg.Now())
	l.cfg.Logger.Info("browser back", "peer_id", peerID, "link_id", m.LinkID)
	l.reply(peerID, map[string]string{"type": msgAuthOK, "device_id": l.cfg.DeviceID, "link_id": m.LinkID})
}

// unlink forgets the browser that asked.
func (l *LinkService) unlink(peerID string) {
	l.mu.Lock()
	linkID, ok := l.trusted[peerID]
	if !ok {
		l.mu.Unlock()
		return
	}
	l.links = slices.DeleteFunc(l.links, func(r models.Link) bool { return r.ID == linkID })
	links := slices.Clone(l.links)
	l.mu.Unlock()
	l.persist(links)
	l.cfg.Logger.Info("browser unlinked itself", "link_id", linkID)
	l.reply(peerID, map[string]string{"type": msgUnlinked})
	l.dropLater(peerID)
}

// UnlinkAll forgets every browser and disconnects the connected ones.
// The window shows the pairing code again.
func (l *LinkService) UnlinkAll() {
	l.mu.Lock()
	peers := make([]string, 0, len(l.trusted))
	for p := range l.trusted {
		peers = append(peers, p)
	}
	l.links = nil
	l.mu.Unlock()
	l.persist(nil)
	for _, p := range peers {
		l.reply(p, map[string]string{"type": msgUnlinked})
		l.dropLater(p)
	}
	l.cfg.Logger.Info("every browser unlinked")
}

func (l *LinkService) persist(links []models.Link) {
	if err := l.cfg.Save(links); err != nil {
		l.cfg.Logger.Warn("cannot save the linked browsers", "err", err)
	}
	l.status.SetSavedLinks(len(links))
}

func (l *LinkService) reply(peerID string, msg any) {
	l.mu.Lock()
	send := l.send
	l.mu.Unlock()
	if b, err := json.Marshal(msg); err == nil && send != nil {
		send(peerID, b)
	}
}

// dropLater forgets the peer now and hangs up a moment later, so the last
// message (auth_failed, unlinked) reaches the browser before the
// connection closes.
func (l *LinkService) dropLater(peerID string) {
	l.Forget(peerID)
	time.AfterFunc(l.cfg.HangUpDelay, func() { l.dropPeer(peerID) })
}

func (l *LinkService) dropPeer(peerID string) {
	l.Forget(peerID)
	l.mu.Lock()
	drop := l.drop
	l.mu.Unlock()
	if drop != nil {
		drop(peerID)
	}
}

// newLink creates a random token for a browser and the record that keeps
// only its hash.
func newLink(now time.Time) (string, models.Link, error) {
	var id [8]byte
	var secret [32]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", models.Link{}, err
	}
	if _, err := rand.Read(secret[:]); err != nil {
		return "", models.Link{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(secret[:])
	return token, models.Link{ID: hex.EncodeToString(id[:]), TokenHash: hashToken(token), CreatedAt: now, LastSeen: now}, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// tokenMatches compares in constant time, so timing reveals nothing.
func tokenMatches(hash, token string) bool {
	if token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(hash), []byte(hashToken(token))) == 1
}
