// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package panel

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gorilla/websocket"

	"github.com/lordbasex/go-link-console/backend-device/internal/services"
	"github.com/lordbasex/go-link-console/backend-device/pkg/signalclient"
)

const token = "0b5c7c1e-9d7a-4b8e-8a31-2f6f3c9d1e24" // gitleaks:allow (test value)

type fakes struct {
	mu      sync.Mutex
	trusted []string
	links   []string
	signals []string
	removed []string
	routes  map[string]services.Sender
}

func (f *fakes) PairedByPanel(p string) { f.mu.Lock(); f.trusted = append(f.trusted, p); f.mu.Unlock() }
func (f *fakes) Forget(string)          {}
func (f *fakes) AddLink(p string) error {
	f.mu.Lock()
	f.links = append(f.links, p)
	f.mu.Unlock()
	return nil
}
func (f *fakes) HandleSignal(from string, payload json.RawMessage) error {
	f.mu.Lock()
	f.signals = append(f.signals, from+" "+string(payload))
	f.mu.Unlock()
	return nil
}
func (f *fakes) RemoveViewer(p string) { f.mu.Lock(); f.removed = append(f.removed, p); f.mu.Unlock() }
func (f *fakes) Route(p string, s services.Sender) {
	f.mu.Lock()
	f.routes[p] = s
	f.mu.Unlock()
}
func (f *fakes) Unroute(p string) { f.mu.Lock(); delete(f.routes, p); f.mu.Unlock() }

func newPanel(t *testing.T) (*httptest.Server, *fakes) {
	t.Helper()
	f := &fakes{routes: map[string]services.Sender{}}
	files := fstest.MapFS{
		"index.html":    {Data: []byte("<!doctype html><html><head><title>go-link</title></head><body></body></html>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}
	s := New(Config{
		Token: func() string { return token }, Files: files,
		Links: f, Streams: f, Router: f,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv, f
}

func dial(t *testing.T, srv *httptest.Server, origin string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	h := http.Header{}
	if origin != "" {
		h.Set("Origin", origin)
	}
	ws, resp, err := websocket.DefaultDialer.Dial(url, h)
	if err != nil {
		code := 0
		if resp != nil {
			code = resp.StatusCode
		}
		t.Fatalf("dial: %v (%d)", err, code)
	}
	t.Cleanup(func() { _ = ws.Close() })
	return ws
}

func read(t *testing.T, ws *websocket.Conn) signalclient.Envelope {
	t.Helper()
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	var env signalclient.Envelope
	if err := ws.ReadJSON(&env); err != nil {
		t.Fatalf("read: %v", err)
	}
	return env
}

// login asks for a nonce and answers it with a proof made from token.
func login(t *testing.T, ws *websocket.Conn, token string) signalclient.Envelope {
	t.Helper()
	_ = ws.WriteJSON(map[string]string{"type": "panel"})
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	var n map[string]string
	if err := ws.ReadJSON(&n); err != nil {
		t.Fatalf("nonce: %v", err)
	}
	if n["type"] != TypePanelNonce {
		// An error (the lockout) instead of a nonce.
		return signalclient.Envelope{Type: n["type"], Error: n["error"]}
	}
	_ = ws.WriteJSON(map[string]string{"type": "panel", "proof": Proof(token, n["nonce"])})
	return read(t, ws)
}

func TestThePanelServesTheWebsiteMarkedAsPanel(t *testing.T) {
	srv, _ := newPanel(t)
	for _, route := range []string{"/", "/device/roms"} {
		resp, err := http.Get(srv.URL + route)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || !strings.Contains(string(body), `name="go-link-panel"`) {
			t.Fatalf("%s: %d %s", route, resp.StatusCode, body)
		}
		if !strings.Contains(resp.Header.Get("Content-Security-Policy"), "frame-ancestors 'none'") {
			t.Fatal("no CSP")
		}
	}
	resp, _ := http.Get(srv.URL + "/assets/app.js")
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Cache-Control"), "immutable") {
		t.Fatalf("asset: %d %s", resp.StatusCode, resp.Header.Get("Cache-Control"))
	}
	// Nothing to call: only files are served.
	resp, _ = http.Post(srv.URL+"/device", "application/json", strings.NewReader("{}"))
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST: %d", resp.StatusCode)
	}
}

func TestThePanelRefusesOtherWebsitesAndWrongTokens(t *testing.T) {
	srv, f := newPanel(t)
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	if _, resp, err := websocket.DefaultDialer.Dial(url, http.Header{"Origin": {"https://evil.example"}}); err == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatal("another website opened the panel's socket")
	}
	for i := range failsPerIP {
		ws := dial(t, srv, srv.URL)
		read(t, ws) // hello
		if env := login(t, ws, "nope"); env.Type != "error" || env.Error != errBadToken {
			t.Fatalf("try %d: %+v", i, env)
		}
	}
	// Too many wrong tokens: even the right one waits now.
	ws := dial(t, srv, srv.URL)
	read(t, ws)
	if env := login(t, ws, token); env.Error != errTooMany {
		t.Fatalf("after the lockout: %+v", env)
	}
	if len(f.trusted) != 0 || len(f.links) != 0 {
		t.Fatal("an untrusted browser got a link")
	}
}

func TestThePanelTokenOpensTheDataLink(t *testing.T) {
	srv, f := newPanel(t)
	ws := dial(t, srv, srv.URL)
	hello := read(t, ws)
	if hello.Type != "hello" || !strings.HasPrefix(hello.PeerID, "panel-") {
		t.Fatalf("hello %+v", hello)
	}
	if env := login(t, ws, strings.ToUpper(token)); env.Type != "paired" || env.Remote != DevicePeerID {
		t.Fatalf("paired %+v", env)
	}
	// The device's offer comes through this socket...
	f.mu.Lock()
	route := f.routes[hello.PeerID]
	links := append([]string(nil), f.links...)
	f.mu.Unlock()
	if route == nil || len(links) != 1 || links[0] != hello.PeerID {
		t.Fatalf("route %v links %v", route, links)
	}
	_ = route.Send(signalclient.Envelope{Type: "signal", To: hello.PeerID, Payload: json.RawMessage(`{"kind":"offer"}`)})
	if env := read(t, ws); env.Type != "signal" || env.From != DevicePeerID || string(env.Payload) != `{"kind":"offer"}` {
		t.Fatalf("offer %+v", env)
	}
	// ...and the answer goes back to the device.
	_ = ws.WriteJSON(map[string]any{"type": "signal", "to": DevicePeerID, "payload": map[string]string{"kind": "answer"}})
	deadline := time.Now().Add(2 * time.Second)
	for {
		f.mu.Lock()
		n := len(f.signals)
		f.mu.Unlock()
		if n == 1 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.signals) != 1 || f.signals[0] != hello.PeerID+` {"kind":"answer"}` {
		t.Fatalf("signals %v", f.signals)
	}
}

func TestThePanelTokenNeverCrossesTheSocketAndNoncesAreSingleUse(t *testing.T) {
	srv, _ := newPanel(t)
	ws := dial(t, srv, srv.URL)
	read(t, ws) // hello
	// Sending the token itself does not log in.
	_ = ws.WriteJSON(map[string]string{"type": "panel", "proof": token})
	if env := read(t, ws); env.Error != errNoNonce {
		t.Fatalf("token instead of a proof: %+v", env)
	}
	// A proof for someone else's nonce is refused too.
	ws2 := dial(t, srv, srv.URL)
	read(t, ws2)
	_ = ws2.WriteJSON(map[string]string{"type": "panel"})
	var n map[string]string
	_ = ws2.ReadJSON(&n)
	_ = ws2.WriteJSON(map[string]string{"type": "panel", "proof": Proof(token, "not-the-nonce")})
	if env := read(t, ws2); env.Error != errBadToken {
		t.Fatalf("wrong nonce: %+v", env)
	}
}

func TestThePanelAnswersOnlyForTheDevicesOwnNames(t *testing.T) {
	srv, _ := newPanel(t)
	for host, want := range map[string]int{
		"127.0.0.1:7391":         200,
		"localhost:7391":         200,
		"raspberrypi.local:7391": 200,
		"evil.example:7391":      http.StatusMisdirectedRequest, // DNS rebinding
	} {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/", nil)
		req.Host = host
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("Host %s: %d, want %d", host, resp.StatusCode, want)
		}
	}
}

// fakeRooms is one running room, "R1", with invitation "INV".
type fakeRooms struct {
	mu     sync.Mutex
	events []signalclient.Envelope
}

func (r *fakeRooms) Find(roomID, invite, code string) string {
	if roomID == "R1" || invite == "INV" {
		return "R1"
	}
	return ""
}

func (r *fakeRooms) OnMessage(env signalclient.Envelope) {
	r.mu.Lock()
	r.events = append(r.events, env)
	r.mu.Unlock()
}

func (r *fakeRooms) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, e := range r.events {
		out = append(out, e.Type+" "+e.RoomID+e.SessionID+" "+e.From+e.Remote+" "+string(e.Payload))
	}
	return out
}

func TestAPanelBrowserPlaysInARoomThroughThePanel(t *testing.T) {
	f := &fakes{routes: map[string]services.Sender{}}
	rooms := &fakeRooms{}
	s := New(Config{
		Token: func() string { return token }, Files: fstest.MapFS{"index.html": {Data: []byte("<html><head></head></html>")}},
		Links: f, Streams: f, Router: f, Rooms: rooms,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)

	ws := dial(t, srv, srv.URL)
	hello := read(t, ws)
	_ = ws.WriteJSON(map[string]string{"type": "panel"})
	var n map[string]string
	_ = ws.ReadJSON(&n)
	_ = ws.WriteJSON(map[string]string{"type": "panel", "mode": ModeRoom, "proof": Proof(token, n["nonce"])})
	if env := read(t, ws); env.Type != signalclient.TypePaired {
		t.Fatalf("login: %+v", env)
	}
	// A socket for playing opens no data link and is not a linked browser.
	f.mu.Lock()
	links, trusted := len(f.links), len(f.trusted)
	_, routed := f.routes[hello.PeerID]
	f.mu.Unlock()
	if links != 0 || trusted != 0 || !routed {
		t.Fatalf("links %d, trusted %d, routed %v", links, trusted, routed)
	}

	// Every request gets a reply, so the web's replies stay in order.
	_ = ws.WriteJSON(map[string]string{"type": "rooms_list"})
	if env := read(t, ws); env.Type != signalclient.TypeRooms || len(env.Rooms) != 0 {
		t.Fatalf("rooms_list: %+v", env)
	}
	_ = ws.WriteJSON(map[string]string{"type": "claim", "code": "123456789"})
	if env := read(t, ws); env.Type != signalclient.TypeError || env.Error != errUnsupported {
		t.Fatalf("claim: %+v", env)
	}
	_ = ws.WriteJSON(map[string]string{"type": "join", "room_id": "nope"})
	if env := read(t, ws); env.Type != signalclient.TypeError || env.Error != errNoRoom {
		t.Fatalf("unknown room: %+v", env)
	}
	_ = ws.WriteJSON(map[string]string{"type": "join", "invite": "INV"})
	joined := read(t, ws)
	if joined.Type != signalclient.TypeJoined || joined.RoomID != "R1" || joined.Remote != DevicePeerID {
		t.Fatalf("joined: %+v", joined)
	}
	_ = ws.WriteJSON(map[string]any{"type": "signal", "to": DevicePeerID, "payload": map[string]string{"kind": "pin"}})
	// The device answers through the panel socket.
	f.mu.Lock()
	sender := f.routes[hello.PeerID]
	f.mu.Unlock()
	if err := sender.Send(signalclient.Envelope{Type: signalclient.TypeSignal, To: hello.PeerID, Payload: json.RawMessage(`{"kind":"offer"}`)}); err != nil {
		t.Fatal(err)
	}
	if env := read(t, ws); env.Type != signalclient.TypeSignal || env.From != DevicePeerID {
		t.Fatalf("offer: %+v", env)
	}
	_ = ws.Close()

	peer := hello.PeerID
	want := []string{
		"peer_joined R1R1 " + peer + " ",
		`signal R1 ` + peer + ` {"kind":"pin"}`,
		"peer_left R1 " + peer + " ",
	}
	deadline := time.Now().Add(3 * time.Second)
	for strings.Join(rooms.seen(), "|") != strings.Join(want, "|") {
		if time.Now().After(deadline) {
			t.Fatalf("room events:\n%s\nwant:\n%s", strings.Join(rooms.seen(), "\n"), strings.Join(want, "\n"))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestOnlyARoomSocketMayJoin(t *testing.T) {
	srv, _ := newPanel(t)
	ws := dial(t, srv, srv.URL)
	read(t, ws) // hello
	if env := login(t, ws, token); env.Type != signalclient.TypePaired {
		t.Fatalf("login: %+v", env)
	}
	// The data link's socket ignores joins.
	_ = ws.WriteJSON(map[string]string{"type": "join", "room_id": "R1"})
	_ = ws.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	var env signalclient.Envelope
	if err := ws.ReadJSON(&env); err == nil {
		t.Fatalf("a link socket answered a join: %+v", env)
	}
}
