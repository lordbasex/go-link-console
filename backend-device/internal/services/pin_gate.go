// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/lordbasex/go-link/backend-device/internal/models"
)

// A private room asks every guest for a PIN before the device streams to
// them. The device enforces it (not the web, not signalhub), and limits the
// guesses so a 6 digit PIN cannot be found by trying.
//
// Every invitation carries its own PIN, good for one person: the first
// browser that uses it gets in and receives a token to come back (a reload,
// a dropped connection) without a PIN; anyone else trying the same PIN is
// told it was used. A PIN nobody used expires after passTTL, and every PIN
// and token dies with the room's session (the gate is per session).
const (
	PinLength       = 6
	pinTriesPerPeer = 5                // wrong PINs before a guest is turned away
	pinRoomFails    = 20               // wrong PINs in pinWindow before the room locks
	pinWindow       = 10 * time.Minute // how far back wrong PINs count
	pinLock         = 10 * time.Minute // how long a locked room refuses every PIN
	passTTL         = 6 * time.Hour    // an invitation nobody used expires
	maxPasses       = 64               // invitations alive at once in a room

	// A group invitation lets several people in with one link: its key
	// (32 random bytes) is never typed, so it cannot be guessed, but a link
	// can be forwarded, hence the limits and, by default, the host's OK.
	GroupMaxUses   = 50
	GroupMinTTL    = time.Hour
	GroupMaxTTL    = 24 * time.Hour
	maxKnocks      = 20  // people waiting for the host at once
	maxGroupTokens = 200 // return tokens of people let in by group links
	KnockNameMax   = 24  // runes of the name shown to the host
)

// NewPIN returns a random 6 digit PIN.
func NewPIN() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		panic(err) // crypto/rand never fails on supported systems
	}
	return fmt.Sprintf("%06d", n.Int64())
}

// ValidPIN reports whether s is 6 digits.
func ValidPIN(s string) bool {
	if len(s) != PinLength {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// newToken returns a random 32 byte token, base64url.
func newToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// PinResult is the device's answer to a PIN, sent back to the guest.
type PinResult struct {
	OK bool `json:"ok"`
	// Token lets this browser come back into the room without a PIN.
	Token string `json:"token,omitempty"`
	// Reason when not OK: "wrong", "used" (someone already came in with
	// this invitation), "blocked" (this guest used its tries) or "locked"
	// (too many wrong PINs in the room: wait RetryAfter seconds).
	Reason     string `json:"reason,omitempty"`
	Left       int    `json:"left,omitempty"`
	RetryAfter int    `json:"retry_after,omitempty"`
	// Owner: the guest came in with the host's own key (never sent).
	Owner bool `json:"-"`
	// Watch: the guest came with the spectators' invitation: in to watch.
	Watch bool `json:"-"`
}

// Pass is one invitation's PIN, for the host to share.
type Pass struct {
	Pin       string    `json:"pin"`
	ExpiresAt time.Time `json:"expires_at"`
}

type pass struct {
	pin     string
	expires time.Time
	token   string // set once someone came in with it
	revoked bool   // its person was taken out of the room: the token is dead
}

// PinGate checks the PINs of one room session.
type PinGate struct {
	now func() time.Time

	mu          sync.Mutex
	private     bool
	owner       string // the host's own key (its browsers never need a PIN)
	passes      []*pass
	tries       map[string]int
	fails       []time.Time
	lockedUntil time.Time

	group       *groupPass       // the room's group invitation, if any
	watch       *groupPass       // the spectators' invitation, if any
	watchTokens map[string]bool  // group tokens that come back to watch
	groupTokens []string         // return tokens of people it let in
	knocks      map[string]knock // peer -> waiting for the host
	declined    map[string]bool  // peers the host said no to, this session
	removed     []string         // tokens of people the host took out of the room
}

type groupPass struct {
	key      string
	uses     int
	used     int
	expires  time.Time
	approval bool
	watch    bool // the spectators' invitation
}

type knock struct {
	name  string
	since time.Time
	watch bool // came with the spectators' invitation
}

// NewPinGate returns an open gate (no PIN asked). now may be nil (time.Now).
func NewPinGate(now func() time.Time) *PinGate {
	if now == nil {
		now = time.Now
	}
	return &PinGate{now: now, tries: map[string]int{}, owner: newToken(), knocks: map[string]knock{}, declined: map[string]bool{}, watchTokens: map[string]bool{}}
}

// SetPrivate makes guests need an invitation's PIN (or a token).
func (g *PinGate) SetPrivate(on bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.private = on
}

// Required reports whether the room asks for a PIN.
func (g *PinGate) Required() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.private
}

// OwnerKey is the host's key: the device gives it only to its linked
// browsers, which send it instead of a PIN.
func (g *PinGate) OwnerKey() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.owner
}

// Issue makes a new invitation: a PIN no other live invitation has.
func (g *PinGate) Issue() Pass {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	g.dropExpired(now)
	if len(g.passes) >= maxPasses {
		// Forget the oldest invitation nobody used; used ones stay.
		if i := slices.IndexFunc(g.passes, func(p *pass) bool { return p.token == "" }); i >= 0 {
			g.passes = slices.Delete(g.passes, i, i+1)
		}
	}
	pin := NewPIN()
	for slices.ContainsFunc(g.passes, func(p *pass) bool { return p.pin == pin }) {
		pin = NewPIN()
	}
	p := &pass{pin: pin, expires: now.Add(passTTL)}
	g.passes = append(g.passes, p)
	return Pass{Pin: pin, ExpiresAt: p.expires}
}

// dropExpired forgets invitations nobody used in time.
func (g *PinGate) dropExpired(now time.Time) {
	g.passes = slices.DeleteFunc(g.passes, func(p *pass) bool {
		return p.token == "" && !now.Before(p.expires)
	})
}

// Check answers one try of a guest: an invitation's PIN, or a token (the
// owner's key, or the one a guest got when it first came in).
func (g *PinGate) Check(peer, pin, token string) PinResult {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	if !g.private {
		return PinResult{OK: true}
	}
	g.dropExpired(now)
	// The host's key and the tokens of guests already in come first: someone
	// guessing PINs can lock the PIN path, but never lock them out.
	if token != "" {
		for _, tk := range g.removed {
			if equal(token, tk) {
				return PinResult{Reason: "removed"}
			}
		}
		if equal(token, g.owner) {
			delete(g.tries, peer)
			delete(g.knocks, peer)
			return PinResult{OK: true, Owner: true}
		}
		for _, p := range g.passes {
			if p.token != "" && !p.revoked && equal(token, p.token) {
				delete(g.tries, peer)
				delete(g.knocks, peer)
				return PinResult{OK: true, Token: p.token}
			}
		}
		for _, tk := range g.groupTokens {
			if equal(token, tk) {
				delete(g.tries, peer)
				delete(g.knocks, peer)
				return PinResult{OK: true, Token: tk, Watch: g.watchTokens[tk]}
			}
		}
	}
	if now.Before(g.lockedUntil) {
		return PinResult{Reason: "locked", RetryAfter: int(g.lockedUntil.Sub(now).Seconds()) + 1}
	}
	if g.tries[peer] >= pinTriesPerPeer {
		return PinResult{Reason: "blocked"}
	}
	if token != "" {
		return g.fail(peer, now, "wrong") // a made-up token
	}
	for _, p := range g.passes {
		if !equal(pin, p.pin) {
			continue
		}
		if p.token != "" {
			return g.fail(peer, now, "used")
		}
		p.token = newToken()
		delete(g.tries, peer)
		delete(g.knocks, peer)
		return PinResult{OK: true, Token: p.token}
	}
	return g.fail(peer, now, "wrong")
}

// fail counts a wrong try and says what the guest may do next.
func (g *PinGate) fail(peer string, now time.Time, reason string) PinResult {
	g.tries[peer]++
	recent := g.fails[:0]
	for _, t := range g.fails {
		if now.Sub(t) < pinWindow {
			recent = append(recent, t)
		}
	}
	g.fails = append(recent, now)
	if len(g.fails) >= pinRoomFails {
		g.lockedUntil = now.Add(pinLock)
		g.fails = nil
		return PinResult{Reason: "locked", RetryAfter: int(pinLock.Seconds())}
	}
	if left := pinTriesPerPeer - g.tries[peer]; left > 0 {
		return PinResult{Reason: reason, Left: left}
	}
	return PinResult{Reason: "blocked"}
}

// equal compares secrets in constant time.
func equal(a, b string) bool {
	return a != "" && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// Forget drops a guest that left. Its tries stay counted in the room, and
// it stops waiting for the host.
func (g *PinGate) Forget(peer string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.tries, peer)
	delete(g.knocks, peer)
}

// IssueGroup makes the room's group invitation, replacing the one before
// (whose link stops working; people it let in keep their tokens). uses is
// clamped to 1..GroupMaxUses and ttl to GroupMinTTL..GroupMaxTTL. People
// still waiting with the old link are returned, to be told.
func (g *PinGate) IssueGroup(uses int, ttl time.Duration, approval bool) (models.GroupInvite, []string) {
	return g.issue(false, uses, ttl, approval)
}

// IssueWatchGroup makes the room's spectators' invitation: like the group
// invitation, with its own link and places, but the people it lets in come
// to watch (PinResult.Watch) and play only if the host lets them.
func (g *PinGate) IssueWatchGroup(uses int, ttl time.Duration, approval bool) (models.GroupInvite, []string) {
	return g.issue(true, uses, ttl, approval)
}

func (g *PinGate) issue(watch bool, uses int, ttl time.Duration, approval bool) (models.GroupInvite, []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	uses = min(max(uses, 1), GroupMaxUses)
	ttl = min(max(ttl, GroupMinTTL), GroupMaxTTL)
	gp := &groupPass{key: newToken(), uses: uses, expires: g.now().Add(ttl), approval: approval, watch: watch}
	*g.passOf(watch) = gp
	waiting := g.dropKnocks(watch)
	return gp.info(), waiting
}

// passOf is where the group invitation of a kind lives (callers hold mu).
func (g *PinGate) passOf(watch bool) **groupPass {
	if watch {
		return &g.watch
	}
	return &g.group
}

// RevokeGroup ends the group invitation: its link stops working at once.
// People still waiting are returned, to be told.
func (g *PinGate) RevokeGroup() []string { return g.revoke(false) }

// RevokeWatchGroup ends the spectators' invitation.
func (g *PinGate) RevokeWatchGroup() []string { return g.revoke(true) }

func (g *PinGate) revoke(watch bool) []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	*g.passOf(watch) = nil
	return g.dropKnocks(watch)
}

// dropKnocks stops the waiting of the people who came with one kind of
// group invitation, and returns them.
func (g *PinGate) dropKnocks(watch bool) []string {
	var peers []string
	for p, k := range g.knocks {
		if k.watch == watch {
			peers = append(peers, p)
			delete(g.knocks, p)
		}
	}
	slices.Sort(peers)
	return peers
}

// Group returns the live group invitation and everyone waiting for the
// host with either invitation (Knock.Watch: to watch), oldest first, for
// the host's status. Nil once it expired.
func (g *PinGate) Group() (*models.GroupInvite, []models.Knock) {
	g.mu.Lock()
	defer g.mu.Unlock()
	gi := g.liveInfo(g.group)
	knocks := make([]models.Knock, 0, len(g.knocks))
	for p, k := range g.knocks {
		knocks = append(knocks, models.Knock{Peer: p, Name: k.name, Since: k.since, Watch: k.watch})
	}
	slices.SortFunc(knocks, func(a, b models.Knock) int { return a.Since.Compare(b.Since) })
	if len(knocks) == 0 {
		knocks = nil
	}
	return gi, knocks
}

// WatchGroup returns the live spectators' invitation (nil: none).
func (g *PinGate) WatchGroup() *models.GroupInvite {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.liveInfo(g.watch)
}

func (g *PinGate) liveInfo(gp *groupPass) *models.GroupInvite {
	if gp == nil || !g.now().Before(gp.expires) {
		return nil
	}
	v := gp.info()
	return &v
}

func (gp *groupPass) info() models.GroupInvite {
	return models.GroupInvite{Key: gp.key, Uses: gp.uses, Used: gp.used, ExpiresAt: gp.expires, Approval: gp.approval, Watch: gp.watch}
}

// CheckKey answers a guest that came with a group invitation's key (to
// play, or the spectators' one, to watch). With the host's approval on, the
// guest waits (Reason "waiting") until Answer. The right key is checked
// before the room's lock, like a guest's token: a PIN guesser can lock the
// PIN path, never the people the host invited. A wrong key counts as a
// wrong PIN. Someone the host said no to stays out.
func (g *PinGate) CheckKey(peer, key, name string) PinResult {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	if !g.private {
		return PinResult{OK: true}
	}
	var gp *groupPass
	for _, c := range []*groupPass{g.group, g.watch} {
		if c != nil && equal(key, c.key) {
			gp = c
		}
	}
	if gp == nil {
		if now.Before(g.lockedUntil) {
			return PinResult{Reason: "locked", RetryAfter: int(g.lockedUntil.Sub(now).Seconds()) + 1}
		}
		if g.tries[peer] >= pinTriesPerPeer {
			return PinResult{Reason: "blocked"}
		}
		return g.fail(peer, now, "wrong")
	}
	if g.declined[peer] {
		return PinResult{Reason: "declined"}
	}
	if !now.Before(gp.expires) {
		return PinResult{Reason: "expired"}
	}
	if gp.used >= gp.uses {
		return PinResult{Reason: "full"}
	}
	delete(g.tries, peer)
	if !gp.approval {
		delete(g.knocks, peer)
		return g.letInByGroup(gp)
	}
	if _, ok := g.knocks[peer]; ok {
		return PinResult{Reason: "waiting"} // asked again: still waiting, same place in line
	}
	if len(g.knocks) >= maxKnocks {
		return PinResult{Reason: "busy"}
	}
	g.knocks[peer] = knock{name: cleanName(name), since: now, watch: gp.watch}
	return PinResult{Reason: "waiting"}
}

// Knocking reports whether a guest is already waiting for the host.
func (g *PinGate) Knocking(peer string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	_, ok := g.knocks[peer]
	return ok
}

// Answer is the host's answer to someone waiting: let in (with a token) or
// not ("declined"). ok is false when the peer was not waiting.
func (g *PinGate) Answer(peer string, accept bool) (res PinResult, ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	k, ok := g.knocks[peer]
	if !ok {
		return PinResult{}, false
	}
	delete(g.knocks, peer)
	if !accept {
		g.declined[peer] = true // no knocking again with the same connection
		if len(g.declined) > 1000 {
			g.declined = map[string]bool{peer: true}
		}
		return PinResult{Reason: "declined"}, true
	}
	gp := *g.passOf(k.watch)
	switch {
	case gp == nil || !g.now().Before(gp.expires):
		return PinResult{Reason: "expired"}, true
	case gp.used >= gp.uses:
		return PinResult{Reason: "full"}, true
	}
	return g.letInByGroup(gp), true
}

func (g *PinGate) letInByGroup(gp *groupPass) PinResult {
	gp.used++
	tk := newToken()
	g.groupTokens = append(g.groupTokens, tk)
	if len(g.groupTokens) > maxGroupTokens {
		delete(g.watchTokens, g.groupTokens[0])
		g.groupTokens = g.groupTokens[len(g.groupTokens)-maxGroupTokens:]
	}
	if gp.watch {
		g.watchTokens[tk] = true
	}
	return PinResult{OK: true, Token: tk, Watch: gp.watch}
}

// Revoke ends a guest's return token: the host took that person out of
// the room, so the token gets "removed" instead of a way back in (it never
// counts as a wrong try). The owner's key is never revoked.
func (g *PinGate) Revoke(token string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if token == "" || equal(token, g.owner) {
		return
	}
	for _, p := range g.passes {
		if equal(token, p.token) {
			p.revoked = true // the PIN stays used, its token no longer opens
		}
	}
	g.groupTokens = slices.DeleteFunc(g.groupTokens, func(tk string) bool { return equal(token, tk) })
	delete(g.watchTokens, token)
	g.removed = append(g.removed, token)
	if len(g.removed) > maxGroupTokens {
		g.removed = g.removed[len(g.removed)-maxGroupTokens:]
	}
}

// cleanName keeps a guest's name short and printable, for the host's list.
func cleanName(s string) string {
	s = strings.Map(func(r rune) rune {
		// control and format characters (bidi overrides, zero width)
		// could make one name look like another in the host's list
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if r := []rune(s); len(r) > KnockNameMax {
		s = string(r[:KnockNameMax])
	}
	return s
}
