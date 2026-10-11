// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package panel is the device's local web panel, for devices without a
// window (Raspberry Pi, servers, Docker). It serves the website's files
// and one WebSocket, and nothing else: the WebSocket only carries the
// WebRTC negotiation (the same hello, paired and signal messages as
// signalhub), and everything the panel does travels afterwards over the
// WebRTC data channel, exactly like a browser linked through signalhub.
// There is no REST API. A browser gets in with the panel token, a UUID v4
// kept in device.json.
package panel

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/lordbasex/go-link-console/backend-device/internal/services"
	"github.com/lordbasex/go-link-console/backend-device/pkg/ids"
	"github.com/lordbasex/go-link-console/backend-device/pkg/signalclient"
)

// DefaultAddr is where the panel listens: every interface, so other
// computers and phones on the LAN reach it.
const DefaultAddr = ":7373"

// DevicePeerID is the device's peer id as the panel's browsers see it.
const DevicePeerID = "device"

// TypePanel starts the panel's login: {"type":"panel"} gets a panel_nonce,
// then {"type":"panel","proof":"..."} (HMAC of the nonce with the token)
// gets paired. The token itself never crosses the network.
//
// A login with "mode":"room" opens no data link: that socket is for
// playing, and its next message is a join (room_id, invite or code), as on
// signalhub. signalhub cannot carry the panel's rooms: it refuses the
// panel's LAN origin.
const (
	TypePanel      = "panel"
	TypePanelNonce = "panel_nonce"
	ModeRoom       = "room"
)

const (
	authTimeout    = 15 * time.Second
	maxMessage     = 64 << 10
	sendBuffer     = 64
	writeWait      = 10 * time.Second
	failsPerIP     = 5               // wrong tokens from one address...
	failWindow     = time.Minute     // ...in this long lock it out...
	failLockout    = 5 * time.Minute // ...for this long
	maxWaiting     = 32              // sockets waiting for their proof at once
	errBadToken    = "invalid panel token"
	errTooMany     = "too many attempts, try again later"
	errFirstPanel  = "send the panel token first"
	errNoNonce     = "ask for a nonce first"
	errNoRoom      = "room not found"
	errUnsupported = "not available on the device panel"
	metaTag        = `<meta name="go-link-panel" content="1" />`
)

// Links trusts a panel browser (services.LinkService).
type Links interface {
	PairedByPanel(peerID string)
	Forget(peerID string)
}

// Streams opens and negotiates the data link (services.StreamService).
type Streams interface {
	AddLink(peerID string) error
	HandleSignal(from string, payload json.RawMessage) error
	RemoveViewer(peerID string)
}

// Router sends a peer's signaling through its own connection
// (services.RoutingSender).
type Router interface {
	Route(peer string, s services.Sender)
	Unroute(peer string)
}

// Rooms lets the panel's browsers into the device's own rooms.
type Rooms interface {
	// Find returns the room_id of a running room by its id, invitation
	// or code, or "".
	Find(roomID, invite, code string) string
	// OnMessage takes a room event as if signalhub had sent it.
	OnMessage(env signalclient.Envelope)
}

// Config wires the panel to the device.
type Config struct {
	Addr    string
	Token   func() string // the current panel token
	Files   fs.FS         // the website (index.html and assets)
	Links   Links
	Streams Streams
	Router  Router
	Rooms   Rooms                   // optional: without it, rooms are not found
	Status  *services.StatusService // optional: counts the panel's browsers
	Logger  *slog.Logger
	Now     func() time.Time
}

// Server is the panel's HTTP server.
type Server struct {
	cfg      Config
	upgrader websocket.Upgrader

	mu    sync.Mutex
	fails map[string][]time.Time
	locks map[string]time.Time

	waiting atomic.Int32 // sockets that have not proven the token yet
}

// New builds the server. Call Run to listen.
func New(cfg Config) *Server {
	if cfg.Addr == "" {
		cfg.Addr = DefaultAddr
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	s := &Server{cfg: cfg, fails: map[string][]time.Time{}, locks: map[string]time.Time{}}
	s.upgrader = websocket.Upgrader{CheckOrigin: sameOrigin}
	return s
}

// Handler returns the panel's routes (tests use it with httptest).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.serveWS)
	mux.HandleFunc("/", s.serveFiles)
	return securityHeaders(knownHost(mux))
}

// knownHost answers only requests for the device itself (an IP address,
// localhost, a .local name or its hostname). A page on another site that
// points its own name at the device (DNS rebinding) is refused.
func knownHost(next http.Handler) http.Handler {
	name, _ := os.Hostname()
	name = strings.ToLower(strings.TrimSuffix(name, ".local"))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.ToLower(strings.Trim(host, "[]"))
		ok := net.ParseIP(host) != nil || host == "localhost" || strings.HasSuffix(host, ".local") ||
			(name != "" && host == name)
		if !ok {
			http.Error(w, "unknown host", http.StatusMisdirectedRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Run listens until ctx ends.
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{Addr: s.cfg.Addr, Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return err
	}
	for _, u := range URLs(ln.Addr()) {
		s.cfg.Logger.Info("web panel", "url", u)
	}
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// URLs lists where the panel can be opened from the LAN.
func URLs(addr net.Addr) []string {
	tcp, ok := addr.(*net.TCPAddr)
	if !ok {
		return []string{"http://" + addr.String()}
	}
	port := tcp.Port
	if tcp.IP != nil && !tcp.IP.IsUnspecified() {
		return []string{"http://" + net.JoinHostPort(tcp.IP.String(), strconv.Itoa(port))}
	}
	var out []string
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok || ipn.IP.To4() == nil || ipn.IP.IsLoopback() || ipn.IP.IsLinkLocalUnicast() {
			continue
		}
		out = append(out, "http://"+net.JoinHostPort(ipn.IP.String(), strconv.Itoa(port)))
	}
	return append(out, "http://"+net.JoinHostPort("localhost", strconv.Itoa(port)))
}

// securityHeaders applies the same rules as the website: only its own
// files and scripts, the fonts, and WebSocket connections (this one and
// signalhub's, for the rooms).
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", strings.Join([]string{
			"default-src 'self'",
			"script-src 'self' 'wasm-unsafe-eval'",
			"style-src 'self'",
			"font-src 'self'",
			"img-src 'self' data: blob:",
			"connect-src 'self' ws: wss:",
			"media-src 'self' blob:",
			"object-src 'none'",
			"base-uri 'none'",
			"frame-ancestors 'none'",
		}, "; "))
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

// serveFiles serves the website; any other path is a route of the app.
func (s *Server) serveFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name != "" && name != "index.html" {
		if st, err := fs.Stat(s.cfg.Files, name); err == nil && !st.IsDir() {
			if strings.HasPrefix(name, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			http.ServeFileFS(w, r, s.cfg.Files, name)
			return
		}
	}
	page, err := fs.ReadFile(s.cfg.Files, "index.html")
	if err != nil {
		http.Error(w, "the panel is not built", http.StatusNotFound)
		return
	}
	// The app knows it runs as the device's panel from this tag.
	html := strings.Replace(string(page), "<head>", "<head>\n    "+metaTag, 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(html))
}

// sameOrigin accepts WebSockets only from pages of this same panel, so
// another website open in the browser cannot use it.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	i := strings.Index(origin, "://")
	return i >= 0 && strings.EqualFold(origin[i+3:], r.Host)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// locked reports whether an address used up its wrong tokens.
func (s *Server) locked(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	until, ok := s.locks[ip]
	if ok && s.cfg.Now().Before(until) {
		return true
	}
	delete(s.locks, ip)
	return false
}

// fail counts a wrong token and locks the address after too many.
func (s *Server) fail(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.cfg.Now()
	// Forget old tries of every address, so the map does not grow forever.
	for addr, times := range s.fails {
		if len(times) == 0 || now.Sub(times[len(times)-1]) >= failWindow {
			delete(s.fails, addr)
		}
	}
	for addr, until := range s.locks {
		if !now.Before(until) {
			delete(s.locks, addr)
		}
	}
	recent := s.fails[ip][:0]
	for _, t := range s.fails[ip] {
		if now.Sub(t) < failWindow {
			recent = append(recent, t)
		}
	}
	recent = append(recent, now)
	if len(recent) >= failsPerIP {
		s.locks[ip] = now.Add(failLockout)
		delete(s.fails, ip)
		return
	}
	s.fails[ip] = recent
}

// Proof is what a browser sends instead of the panel token: HMAC-SHA256
// keyed with the token over the nonce the panel gave it (the website
// computes the same, packages/shared/src/hmac.ts).
func Proof(token, nonce string) string {
	mac := hmac.New(sha256.New, []byte(strings.ToLower(strings.TrimSpace(token))))
	mac.Write([]byte("go-link panel proof\n" + nonce))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func newNonce() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// conn is one panel browser. Only writeLoop writes to the socket.
type conn struct {
	ws   *websocket.Conn
	send chan []byte
	done chan struct{}
	once sync.Once
}

func (c *conn) close() { c.once.Do(func() { close(c.done) }) }

// Send implements services.Sender for the device's signals to this peer.
func (c *conn) Send(env signalclient.Envelope) error {
	env.From = DevicePeerID
	env.To = ""
	b, err := json.Marshal(env)
	if err != nil {
		return err
	}
	select {
	case c.send <- b:
		return nil
	case <-c.done:
		return errors.New("panel connection closed")
	default:
		return errors.New("panel connection is not keeping up")
	}
}

// sendRaw queues a message that is not a signaling envelope.
func (c *conn) sendRaw(b []byte) {
	select {
	case c.send <- b:
	case <-c.done:
	default:
	}
}

func (c *conn) writeLoop() {
	defer c.ws.Close()
	for {
		select {
		case b := <-c.send:
			_ = c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if c.ws.WriteMessage(websocket.TextMessage, b) != nil {
				c.close()
				return
			}
		case <-c.done:
			// Flush what is queued (an error reply), then hang up.
			for {
				select {
				case b := <-c.send:
					_ = c.ws.SetWriteDeadline(time.Now().Add(writeWait))
					_ = c.ws.WriteMessage(websocket.TextMessage, b)
				default:
					return
				}
			}
		}
	}
}

func (s *Server) serveWS(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	// A bounded number of sockets may wait for their proof at once.
	if s.waiting.Add(1) > maxWaiting {
		s.waiting.Add(-1)
		http.Error(w, "busy", http.StatusServiceUnavailable)
		return
	}
	waiting := true
	defer func() {
		if waiting {
			s.waiting.Add(-1)
		}
	}()
	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return // the upgrader already answered
	}
	ws.SetReadLimit(maxMessage)
	c := &conn{ws: ws, send: make(chan []byte, sendBuffer), done: make(chan struct{})}
	go c.writeLoop()
	defer c.close()

	peer := "panel-" + ids.New()
	_ = c.Send(signalclient.Envelope{Type: signalclient.TypeHello, PeerID: peer})

	trusted := false
	roomMode := false // a socket for playing, not the data link
	roomID := ""      // the room it joined
	nonce := ""       // the one this connection may answer
	defer func() {
		if !trusted {
			return
		}
		s.cfg.Router.Unroute(peer)
		if roomMode {
			if roomID != "" {
				s.cfg.Rooms.OnMessage(signalclient.Envelope{Type: signalclient.TypePeerLeft, From: peer, SessionID: roomID})
				s.cfg.Logger.Info("panel player left", "peer_id", peer, "room_id", roomID)
			}
			return
		}
		s.cfg.Streams.RemoveViewer(peer)
		s.cfg.Links.Forget(peer)
		if s.cfg.Status != nil {
			s.cfg.Status.RemovePeer(peer)
		}
		s.cfg.Logger.Info("panel browser left", "peer_id", peer)
	}()

	_ = ws.SetReadDeadline(time.Now().Add(authTimeout))
	for {
		_, data, err := ws.ReadMessage()
		if err != nil {
			return
		}
		var msg struct {
			Type    string          `json:"type"`
			Proof   string          `json:"proof"`
			Mode    string          `json:"mode"`
			To      string          `json:"to"`
			Payload json.RawMessage `json:"payload"`
			RoomID  string          `json:"room_id"`
			Invite  string          `json:"invite"`
			Code    string          `json:"code"`
		}
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		switch {
		case !trusted && msg.Type == TypePanel && msg.Proof == "":
			if s.locked(ip) {
				_ = c.Send(signalclient.Envelope{Type: signalclient.TypeError, Error: errTooMany})
				return
			}
			nonce = newNonce()
			b, _ := json.Marshal(map[string]string{"type": TypePanelNonce, "nonce": nonce})
			c.sendRaw(b)
		case !trusted && msg.Type == TypePanel:
			if s.locked(ip) {
				_ = c.Send(signalclient.Envelope{Type: signalclient.TypeError, Error: errTooMany})
				return
			}
			if nonce == "" {
				_ = c.Send(signalclient.Envelope{Type: signalclient.TypeError, Error: errNoNonce})
				return
			}
			want := s.cfg.Token()
			ok := want != "" && subtle.ConstantTimeCompare([]byte(msg.Proof), []byte(Proof(want, nonce))) == 1
			nonce = "" // one try per nonce
			if !ok {
				s.fail(ip)
				s.cfg.Logger.Warn("wrong panel token", "addr", ip)
				_ = c.Send(signalclient.Envelope{Type: signalclient.TypeError, Error: errBadToken})
				return
			}
			trusted = true
			waiting = false
			s.waiting.Add(-1)
			_ = ws.SetReadDeadline(time.Time{})
			if msg.Mode == ModeRoom {
				roomMode = true
				s.cfg.Router.Route(peer, c)
				_ = c.Send(signalclient.Envelope{Type: signalclient.TypePaired, SessionID: "panel", Remote: DevicePeerID})
				continue
			}
			s.cfg.Links.PairedByPanel(peer)
			s.cfg.Router.Route(peer, c)
			if s.cfg.Status != nil {
				s.cfg.Status.AddPeer(peer, "panel", s.cfg.Now())
			}
			s.cfg.Logger.Info("panel browser in", "peer_id", peer, "addr", ip)
			_ = c.Send(signalclient.Envelope{Type: signalclient.TypePaired, SessionID: "panel", Remote: DevicePeerID})
			if err := s.cfg.Streams.AddLink(peer); err != nil {
				s.cfg.Logger.Error("cannot open the panel's data link", "err", err)
				return
			}
		case !trusted:
			_ = c.Send(signalclient.Envelope{Type: signalclient.TypeError, Error: errFirstPanel})
			return
		case roomMode && msg.Type == signalclient.TypeJoin && roomID == "":
			found := ""
			if s.cfg.Rooms != nil {
				found = s.cfg.Rooms.Find(msg.RoomID, msg.Invite, msg.Code)
			}
			if found == "" {
				_ = c.Send(signalclient.Envelope{Type: signalclient.TypeError, Error: errNoRoom})
				continue
			}
			roomID = found
			s.cfg.Logger.Info("panel player in", "peer_id", peer, "room_id", roomID)
			_ = c.Send(signalclient.Envelope{Type: signalclient.TypeJoined, SessionID: roomID, RoomID: roomID, Remote: DevicePeerID})
			s.cfg.Rooms.OnMessage(signalclient.Envelope{Type: signalclient.TypePeerJoined, RoomID: roomID, SessionID: roomID, Remote: peer})
		case roomMode && msg.Type == signalclient.TypeSignal && msg.To == DevicePeerID && roomID != "":
			s.cfg.Rooms.OnMessage(signalclient.Envelope{Type: signalclient.TypeSignal, From: peer, SessionID: roomID, Payload: msg.Payload})
		case !roomMode && msg.Type == signalclient.TypeSignal && msg.To == DevicePeerID:
			if err := s.cfg.Streams.HandleSignal(peer, msg.Payload); err != nil {
				s.cfg.Logger.Warn("bad panel signal", "peer_id", peer, "err", err)
			}
		case roomMode && msg.Type != signalclient.TypeSignal:
			// Every request gets a reply: the web pairs replies with its
			// requests in order, so a silent one would hold the next ones.
			if msg.Type == signalclient.TypeRoomsList {
				// The panel has no public directory: the rooms come from the device status.
				c.sendRaw([]byte(`{"type":"rooms","rooms":[]}`))
			} else {
				_ = c.Send(signalclient.Envelope{Type: signalclient.TypeError, Error: errUnsupported})
			}
		}
	}
}
