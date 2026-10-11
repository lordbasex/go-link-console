// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"crypto/sha256"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
)

type fakeLinkTransport struct {
	mu      sync.Mutex
	sent    map[string][]map[string]string
	dropped []string
}

func (f *fakeLinkTransport) send(peerID string, msg []byte) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	var m map[string]string
	_ = json.Unmarshal(msg, &m)
	f.sent[peerID] = append(f.sent[peerID], m)
	return true
}

func (f *fakeLinkTransport) drop(peerID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dropped = append(f.dropped, peerID)
}

func (f *fakeLinkTransport) last(peerID string) map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	msgs := f.sent[peerID]
	if len(msgs) == 0 {
		return nil
	}
	return msgs[len(msgs)-1]
}

func (f *fakeLinkTransport) wasDropped(peerID string) bool {
	for i := 0; i < 200; i++ {
		if f.dropped1(peerID) {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return false
}

func (f *fakeLinkTransport) dropped1(peerID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.dropped {
		if p == peerID {
			return true
		}
	}
	return false
}

func newTestLinks(t *testing.T, saved *[]models.Link, timeout time.Duration) (*LinkService, *fakeLinkTransport, *StatusService) {
	t.Helper()
	status := NewStatusService(deviceID, "test", "ws://x", "")
	l := NewLinkService(LinkConfig{
		DeviceID:    deviceID,
		Links:       *saved,
		Save:        func(links []models.Link) error { *saved = links; return nil },
		AuthTimeout: timeout,
		HangUpDelay: time.Millisecond,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, status)
	tr := &fakeLinkTransport{sent: map[string][]map[string]string{}}
	l.SetTransport(tr.send, tr.drop)
	return l, tr, status
}

func TestLinkRememberedAndBack(t *testing.T) {
	var saved []models.Link
	l, tr, status := newTestLinks(t, &saved, time.Minute)
	if l.Linked() {
		t.Fatal("no browser yet")
	}

	// First time: the pairing code; auth hands out a token.
	l.PairedByCode("b1")
	if !l.Trusted("b1") || !l.HandleMessage("b1", []byte(`{"type":"auth"}`)) {
		t.Fatal("code-paired browser must be trusted and auth handled")
	}
	ok := tr.last("b1")
	if ok["type"] != "auth_ok" || ok["device_id"] != deviceID || ok["link_id"] == "" || len(ok["token"]) < 40 {
		t.Fatalf("auth_ok = %+v", ok)
	}
	if len(saved) != 1 || saved[0].ID != ok["link_id"] || strings.Contains(saved[0].TokenHash, ok["token"]) || len(saved[0].TokenHash) != 64 {
		t.Fatalf("saved = %+v", saved)
	}
	if !l.Linked() || status.Snapshot().SavedLinks != 1 {
		t.Fatal("one browser remembered")
	}

	// The device restarts: a new service loads the saved links.
	l2, tr2, status2 := newTestLinks(t, &saved, time.Minute)
	l2.Reached("b2")
	if l2.Trusted("b2") || l2.HandleMessage("b2", []byte(`{"type":"create_room"}`)) {
		t.Fatal("a reached browser is not trusted and other messages are not auth's")
	}
	l2.HandleMessage("b2", []byte(`{"type":"auth","link_id":"`+ok["link_id"]+`","token":"`+ok["token"]+`"}`))
	if back := tr2.last("b2"); back["type"] != "auth_ok" || back["token"] != "" || !l2.Trusted("b2") {
		t.Fatalf("back = %+v", back)
	}
	if len(status2.Snapshot().Peers) != 1 {
		t.Fatal("the browser shows as linked once proven")
	}

	// A wrong token is refused and dropped.
	l2.Reached("evil")
	l2.HandleMessage("evil", []byte(`{"type":"auth","link_id":"`+ok["link_id"]+`","token":"guess"}`))
	if bad := tr2.last("evil"); bad["type"] != "auth_failed" || !tr2.wasDropped("evil") || l2.Trusted("evil") {
		t.Fatalf("wrong token: %+v", bad)
	}

	// The browser unlinks itself.
	l2.HandleMessage("b2", []byte(`{"type":"unlink"}`))
	if tr2.last("b2")["type"] != "unlinked" || !tr2.wasDropped("b2") || len(saved) != 0 || l2.Linked() {
		t.Fatalf("after unlink: %+v", saved)
	}
	_ = tr
}

func TestTheLinkRemembersTheAcceptedTerms(t *testing.T) {
	var saved []models.Link
	l, tr, _ := newTestLinks(t, &saved, time.Minute)
	l.PairedByCode("b1")
	l.HandleMessage("b1", []byte(`{"type":"auth","terms":"2026-09-28"}`))
	ok := tr.last("b1")
	if len(saved) != 1 || saved[0].Terms != "2026-09-28" || saved[0].TermsAt.IsZero() {
		t.Fatalf("saved = %+v", saved)
	}
	// A newer version accepted later is recorded when the browser comes back;
	// anything that is not a version is ignored.
	l2, _, _ := newTestLinks(t, &saved, time.Minute)
	l2.Reached("b2")
	l2.HandleMessage("b2", []byte(`{"type":"auth","link_id":"`+ok["link_id"]+`","token":"`+ok["token"]+`","terms":"2027-01-01"}`))
	if saved[0].Terms != "2027-01-01" {
		t.Fatalf("terms = %q", saved[0].Terms)
	}
	l2.Reached("b3")
	l2.HandleMessage("b3", []byte(`{"type":"auth","link_id":"`+ok["link_id"]+`","token":"`+ok["token"]+`","terms":"<script>"}`))
	if saved[0].Terms != "2027-01-01" {
		t.Fatalf("terms = %q", saved[0].Terms)
	}
}

func TestReachedMustAuthInTime(t *testing.T) {
	var saved []models.Link
	l, tr, _ := newTestLinks(t, &saved, 30*time.Millisecond)
	l.Reached("silent")
	deadline := time.Now().Add(2 * time.Second)
	for !tr.wasDropped("silent") {
		if time.Now().After(deadline) {
			t.Fatal("a browser that never authenticates must be dropped")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestUnlinkAll(t *testing.T) {
	var saved []models.Link
	l, tr, _ := newTestLinks(t, &saved, time.Minute)
	l.PairedByCode("a")
	l.HandleMessage("a", []byte(`{"type":"auth"}`))
	l.PairedByCode("b")
	l.HandleMessage("b", []byte(`{"type":"auth"}`))
	if len(saved) != 2 {
		t.Fatalf("saved = %d", len(saved))
	}
	l.UnlinkAll()
	if len(saved) != 0 || l.Linked() || !tr.wasDropped("a") || !tr.wasDropped("b") || tr.last("a")["type"] != "unlinked" {
		t.Fatalf("after UnlinkAll: saved=%d", len(saved))
	}
}

func TestTheDeviceProvesItselfBeforeTheToken(t *testing.T) {
	var saved []models.Link
	l, tr, _ := newTestLinks(t, &saved, time.Minute)
	l.PairedByCode("b1")
	l.HandleMessage("b1", []byte(`{"type":"auth"}`))
	ok := tr.last("b1")
	token, linkID := ok["token"], ok["link_id"]

	// A browser coming back sends a nonce; the device answers with a proof
	// only it can make (it keeps the token's hash).
	l.Reached("b2")
	nonce := "nonce-0123456789abcdef"
	if !l.HandleMessage("b2", []byte(`{"type":"auth_challenge","link_id":"`+linkID+`","nonce":"`+nonce+`"}`)) {
		t.Fatal("auth_challenge not handled")
	}
	sum := sha256.Sum256([]byte(token))
	if got := tr.last("b2"); got["type"] != "auth_proof" || got["proof"] != LinkProof(sum[:], nonce) {
		t.Fatalf("proof %v", got)
	}
	// Answering a challenge does not trust anyone: the token still must come.
	if l.Trusted("b2") {
		t.Fatal("a challenge trusted the browser")
	}
	// An unknown link gets no proof.
	l.HandleMessage("b3", []byte(`{"type":"auth_challenge","link_id":"nope","nonce":"`+nonce+`"}`))
	if got := tr.last("b3"); got["type"] != "auth_failed" {
		t.Fatalf("unknown link %v", got)
	}
}
