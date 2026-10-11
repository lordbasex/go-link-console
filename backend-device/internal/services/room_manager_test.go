// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
)

type outbox struct {
	mu   sync.Mutex
	msgs map[string][]map[string]any
}

func newOutbox() *outbox { return &outbox{msgs: make(map[string][]map[string]any)} }

func (o *outbox) SendControl(peer string, b []byte) bool {
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	o.mu.Lock()
	o.msgs[peer] = append(o.msgs[peer], m)
	o.mu.Unlock()
	return true
}

// lastState returns the latest room_state sent to peer.
func (o *outbox) lastState(t *testing.T, peer string) map[string]any {
	t.Helper()
	o.mu.Lock()
	defer o.mu.Unlock()
	for i := len(o.msgs[peer]) - 1; i >= 0; i-- {
		if o.msgs[peer][i]["type"] == "room_state" {
			return o.msgs[peer][i]
		}
	}
	t.Fatalf("no room_state for %s", peer)
	return nil
}

func (o *outbox) chats(peer string) []map[string]any {
	o.mu.Lock()
	defer o.mu.Unlock()
	var out []map[string]any
	for _, m := range o.msgs[peer] {
		if m["type"] == "chat" {
			out = append(out, m)
		}
	}
	return out
}

func newManager(t *testing.T) (*RoomManager, *outbox, *[]RoomSummary) {
	out := newOutbox()
	var sums []RoomSummary
	m := NewRoomManager(RoomManagerConfig{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), OnSummary: func(s RoomSummary) { sums = append(sums, s) }}, out)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go m.Run(ctx)
	return m, out, &sums
}

func you(st map[string]any) map[string]any { return st["you"].(map[string]any) }

func ports(st map[string]any) []any { return you(st)["ports"].([]any) }

func TestFourPlayAndOneQueues(t *testing.T) {
	m, out, sums := newManager(t)
	for _, p := range []string{"a", "b", "c", "d", "e"} {
		m.Join(p)
	}
	m.Sync()
	for i, p := range []string{"a", "b", "c", "d"} {
		if got := ports(out.lastState(t, p)); len(got) != 1 || got[0] != float64(i+1) {
			t.Fatalf("%s ports %v", p, got)
		}
		if port, ok := m.PortOf(p, 0); !ok || port != i+1 {
			t.Fatalf("PortOf(%s) = %d %v", p, port, ok)
		}
	}
	st := out.lastState(t, "e")
	if q := you(st)["queue_positions"].([]any); len(q) != 1 || q[0] != float64(1) {
		t.Fatalf("e queue %v", q)
	}
	if _, ok := m.PortOf("e", 0); ok {
		t.Fatal("queued guest has a port")
	}
	last := (*sums)[len(*sums)-1]
	members := last.Members
	last.Members = nil
	if !reflect.DeepEqual(last, RoomSummary{Players: 4, Queue: 1, MaxPlayers: 4}) {
		t.Fatalf("summary %+v", last)
	}
	// Everyone, in order of arrival, with the port each one plays at.
	for i, mem := range members {
		want := []int{i + 1}
		if i == 4 {
			want = nil
		}
		if mem.Peer != string(rune('a'+i)) || !reflect.DeepEqual(mem.Ports, want) {
			t.Fatalf("members %+v", members)
		}
	}

	// P2 leaves: the head of the queue takes that same port.
	m.Leave("b")
	m.Sync()
	if got := ports(out.lastState(t, "e")); len(got) != 1 || got[0] != float64(2) {
		t.Fatalf("e should take P2, got %v", got)
	}
	found := false
	for _, c := range out.chats("a") {
		if c["system"] == "Guest E took seat P2" {
			// The web translates it from the event and its values.
			args, _ := c["args"].(map[string]any)
			found = c["event"] == EventTookSeat && args["name"] == "Guest E" && args["port"] == float64(2)
		}
	}
	if !found {
		t.Fatalf("no system message: %v", out.chats("a"))
	}
}

func TestLocalPlayersTakeSeveralSeats(t *testing.T) {
	m, out, _ := newManager(t)
	m.Join("home")
	m.MarkOwner("home") // only the host seats several controllers
	m.HandleControl("home", []byte(`{"type":"hello","name":"Fede","local_players":[0,1,1,7]}`))
	m.Sync()
	st := out.lastState(t, "home")
	if got := ports(st); len(got) != 2 {
		t.Fatalf("ports %v", got)
	}
	seats := st["seats"].([]any)
	if seats[0].(map[string]any)["name"] != "Fede (1)" || seats[1].(map[string]any)["name"] != "Fede (2)" {
		t.Fatalf("seats %v", seats)
	}
	if port, ok := m.PortOf("home", 1); !ok || port != 2 {
		t.Fatalf("local player 1 port %d %v", port, ok)
	}
	// Going back to one local player frees P2.
	m.HandleControl("home", []byte(`{"type":"hello","local_players":[0]}`))
	m.Sync()
	if _, ok := m.PortOf("home", 1); ok {
		t.Fatal("P2 not freed")
	}
}

func TestSpectateAndRequeue(t *testing.T) {
	m, out, sums := newManager(t)
	for _, p := range []string{"a", "b", "c", "d", "e"} {
		m.Join(p)
	}
	m.HandleControl("a", []byte(`{"type":"spectate"}`))
	m.Sync()
	if got := ports(out.lastState(t, "e")); len(got) != 1 || got[0] != float64(1) {
		t.Fatalf("e should take P1, got %v", got)
	}
	if st := out.lastState(t, "a"); you(st)["spectator"] != true || len(st["spectators"].([]any)) != 1 {
		t.Fatalf("a spectator state %v", st)
	}
	if last := (*sums)[len(*sums)-1]; last.Spectators != 1 || last.Queue != 0 {
		t.Fatalf("summary %+v", last)
	}
	m.HandleControl("a", []byte(`{"type":"queue"}`))
	m.Sync()
	if q := you(out.lastState(t, "a"))["queue_positions"].([]any); len(q) != 1 {
		t.Fatalf("a not queued again: %v", q)
	}
}

func TestChat(t *testing.T) {
	m, out, _ := newManager(t)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	m.cfg.Now = func() time.Time { return now }
	m.Join("a")
	m.Join("b")
	m.HandleControl("a", []byte(`{"type":"hello","name":"  Ana\u0007  "}`))
	m.HandleControl("a", []byte(`{"type":"chat","text":"hi there"}`))
	m.HandleControl("a", []byte(`{"type":"chat","text":"`+strings.Repeat("x", 400)+`"}`))
	m.Sync()
	var got []map[string]any
	for _, c := range out.chats("b") {
		if c["name"] != nil {
			got = append(got, c)
		}
	}
	if len(got) != 2 || got[0]["name"] != "Ana" || got[0]["role"] != "P1" || got[0]["port"] != float64(1) || got[0]["text"] != "hi there" {
		t.Fatalf("chat %v", got)
	}
	if n := len([]rune(got[1]["text"].(string))); n != maxChatLen {
		t.Fatalf("long message kept %d runes", n)
	}

	// Rate limit: 5 messages per 5 seconds.
	for i := 0; i < 4; i++ {
		m.HandleControl("a", []byte(`{"type":"chat","text":"spam"}`))
	}
	m.Sync()
	warned := false
	for _, c := range out.chats("a") {
		if c["system"] == "You are sending messages too fast." {
			warned = true
		}
	}
	if !warned {
		t.Fatal("no rate limit warning")
	}

	// A late joiner receives the history.
	m.Join("c")
	m.Sync()
	if len(out.chats("c")) < 5 {
		t.Fatalf("history not sent: %d", len(out.chats("c")))
	}
}

func TestIgnoresGarbage(t *testing.T) {
	m, out, _ := newManager(t)
	m.Join("a")
	m.HandleControl("a", []byte(`not json`))
	m.HandleControl("a", []byte(`{"type":"chat","text":"   "}`))
	m.HandleControl("stranger", []byte(`{"type":"chat","text":"hi"}`))
	m.Leave("nobody")
	m.Sync()
	if n := len(out.chats("a")); n != 1 { // only "took seat P1"
		t.Fatalf("unexpected chat: %v", out.chats("a"))
	}
}

func TestOrdinal(t *testing.T) {
	for n, want := range map[int]string{1: "1st", 2: "2nd", 3: "3rd", 4: "4th", 11: "11th", 12: "12th", 21: "21st"} {
		if got := ordinal(n); got != want {
			t.Errorf("ordinal(%d) = %s", n, got)
		}
	}
}

func TestPauseOnlyByTheHostDuringAGame(t *testing.T) {
	m, out, _ := newManager(t)
	var calls []bool
	m.OnPause(func(p bool) { calls = append(calls, p) })
	for _, p := range []string{"host", "a", "b"} {
		m.Join(p)
	}
	m.MarkOwner("host")
	m.HandleControl("host", []byte(`{"type":"hello","name":"Fede"}`))
	// The test card never pauses, not even for the host.
	m.HandleControl("host", []byte(`{"type":"pause","paused":true}`))
	m.Sync()
	if st := out.lastState(t, "b"); st["paused"] == true || st["pausable"] == true {
		t.Fatalf("test card state %v", st)
	}

	m.SetPausable(true)
	// A seated guest who is not the host is refused, with a code.
	m.HandleControl("a", []byte(`{"type":"pause","paused":true}`))
	m.Sync()
	if st := out.lastState(t, "b"); st["paused"] == true {
		t.Fatalf("a guest paused: %v", st)
	}
	if e := lastOfType(out, "a", "error"); e == nil || e["code"] != "pause_owner_only" {
		t.Fatalf("no pause_owner_only error: %v", e)
	}
	// The host pauses for everyone.
	m.HandleControl("host", []byte(`{"type":"pause","paused":true}`))
	m.Sync()
	if st := out.lastState(t, "a"); st["paused"] != true || st["paused_by"] != "Fede" {
		t.Fatalf("after pause: %v", st)
	}
	chats := out.chats("a")
	if last := chats[len(chats)-1]; last["system"] != "Fede paused the game" {
		t.Fatalf("chat %v", last)
	}
	// Guests cannot resume either; the host can.
	m.HandleControl("b", []byte(`{"type":"pause","paused":false}`))
	m.Sync()
	if st := out.lastState(t, "a"); st["paused"] != true {
		t.Fatalf("a guest resumed: %v", st)
	}
	m.HandleControl("host", []byte(`{"type":"pause","paused":false}`))
	m.Sync()
	if st := out.lastState(t, "a"); st["paused"] == true {
		t.Fatalf("after resume: %v", st)
	}
	// The linked browser pauses through Pause; leaving the game ends it.
	m.Pause(true, "The host")
	m.SetPausable(false)
	m.Sync()
	if st := out.lastState(t, "a"); st["paused"] == true || st["pausable"] == true {
		t.Fatalf("after the game: %v", st)
	}
	if want := []bool{true, false, true, false}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("emulator calls %v, want %v", calls, want)
	}
}

// lastOfType returns the latest message of a type sent to peer, or nil.
func lastOfType(o *outbox, peer, typ string) map[string]any {
	o.mu.Lock()
	defer o.mu.Unlock()
	for i := len(o.msgs[peer]) - 1; i >= 0; i-- {
		if o.msgs[peer][i]["type"] == typ {
			return o.msgs[peer][i]
		}
	}
	return nil
}

// pauseRoom is a running game with the host (owner, not seated: it
// watches) and two seated players, a (Ana, P1) and b (P2).
func pauseRoom(t *testing.T) (*RoomManager, *outbox, *[]PauseAskEvent, *time.Time) {
	t.Helper()
	m, out, _ := newManager(t)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	m.cfg.Now = func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	var events []PauseAskEvent
	m.OnPauseAsk(func(ev PauseAskEvent) { events = append(events, ev) })
	m.Join("a")
	m.Join("b")
	m.Join("host")
	m.MarkOwner("host")
	m.HandleControl("host", []byte(`{"type":"spectate"}`))
	m.HandleControl("a", []byte(`{"type":"hello","name":"Ana"}`))
	m.SetPausable(true)
	m.Sync()
	return m, out, &events, &now
}

func TestAskForAPauseAndTheHostAccepts(t *testing.T) {
	m, out, events, _ := pauseRoom(t)
	if st := out.lastState(t, "a"); st["host_online"] != true {
		t.Fatalf("host in the room but not online: %v", st)
	}
	m.HandleControl("a", []byte(`{"type":"pause_request"}`))
	m.Sync()
	asked, _ := you(out.lastState(t, "a"))["pause_asked"].(map[string]any)
	if asked == nil || asked["expires_at"] != "2026-01-01T12:00:30Z" {
		t.Fatalf("requester state %v", you(out.lastState(t, "a")))
	}
	// Only the host sees the requests.
	if _, ok := you(out.lastState(t, "b"))["pause_asks"]; ok {
		t.Fatal("a guest sees the requests")
	}
	asks, _ := you(out.lastState(t, "host"))["pause_asks"].([]any)
	if len(asks) != 1 {
		t.Fatalf("host asks %v", asks)
	}
	ask := asks[0].(map[string]any)
	if ask["from"] != "a" || ask["name"] != "Ana" || ask["port"] != float64(1) {
		t.Fatalf("ask %v", ask)
	}
	if len(*events) != 1 || (*events)[0].Type != "pause_asked" || (*events)[0].From != "a" || (*events)[0].Port != 1 {
		t.Fatalf("link events %+v", *events)
	}
	// A guest cannot answer; the host accepts.
	m.HandleControl("b", []byte(`{"type":"pause_answer","from":"a","accept":true}`))
	m.Sync()
	if out.lastState(t, "b")["paused"] == true {
		t.Fatal("a guest answered")
	}
	m.HandleControl("host", []byte(`{"type":"pause_answer","from":"a","accept":true}`))
	m.Sync()
	st := out.lastState(t, "b")
	if st["paused"] != true || st["paused_by"] != "Ana" {
		t.Fatalf("after accept: %v", st)
	}
	chats := out.chats("b")
	if last := chats[len(chats)-1]; last["event"] != EventGamePaused || last["args"].(map[string]any)["name"] != "Ana" || last["args"].(map[string]any)["name2"] != "Guest HOST" {
		t.Fatalf("chat %v", last)
	}
	if you(out.lastState(t, "a"))["pause_asked"] != nil || len(you(out.lastState(t, "host"))["pause_asks"].([]any)) != 0 {
		t.Fatal("the request is still there")
	}
	if last := (*events)[len(*events)-1]; last.Type != "pause_ask_gone" || last.From != "a" {
		t.Fatalf("link events %+v", *events)
	}
	// Paused: no new requests.
	m.HandleControl("b", []byte(`{"type":"pause_request"}`))
	m.Sync()
	if you(out.lastState(t, "b"))["pause_asked"] != nil {
		t.Fatal("asked while paused")
	}
}

func TestPauseRequestDeclinedCancelledOrRefused(t *testing.T) {
	m, out, events, _ := pauseRoom(t)
	// The host declines: only the requester is told.
	m.HandleControl("a", []byte(`{"type":"pause_request"}`))
	m.HandleControl("host", []byte(`{"type":"pause_answer","from":"a","accept":false}`))
	m.Sync()
	if out.lastState(t, "b")["paused"] == true {
		t.Fatal("declined but paused")
	}
	chats := out.chats("a")
	if last := chats[len(chats)-1]; last["event"] != EventPauseDeclined {
		t.Fatalf("requester chat %v", last)
	}
	for _, c := range out.chats("b") {
		if c["event"] == EventPauseDeclined {
			t.Fatal("another guest saw the decline")
		}
	}
	if you(out.lastState(t, "a"))["pause_asked"] != nil {
		t.Fatal("declined request still pending")
	}
	// Asking and withdrawing.
	m.HandleControl("b", []byte(`{"type":"pause_request"}`))
	m.HandleControl("b", []byte(`{"type":"pause_request","cancel":true}`))
	m.Sync()
	if len(you(out.lastState(t, "host"))["pause_asks"].([]any)) != 0 {
		t.Fatal("withdrawn request still pending")
	}
	if last := (*events)[len(*events)-1]; last.Type != "pause_ask_gone" || last.From != "b" {
		t.Fatalf("events %+v", *events)
	}
	// Spectators (and the queue) cannot ask.
	m.Join("c")
	m.HandleControl("c", []byte(`{"type":"spectate"}`))
	m.HandleControl("c", []byte(`{"type":"pause_request"}`))
	m.Sync()
	if you(out.lastState(t, "c"))["pause_asked"] != nil {
		t.Fatal("a spectator asked for a pause")
	}
	// Leaving the seat drops the request.
	m.HandleControl("a", []byte(`{"type":"pause_request"}`))
	m.HandleControl("a", []byte(`{"type":"spectate"}`))
	m.Sync()
	if len(you(out.lastState(t, "host"))["pause_asks"].([]any)) != 0 {
		t.Fatal("request kept without a seat")
	}
}

func TestPauseRequestExpiresAndAskingAgainRefreshesIt(t *testing.T) {
	m, out, _, now := pauseRoom(t)
	m.HandleControl("a", []byte(`{"type":"pause_request"}`))
	m.Sync()
	first := you(out.lastState(t, "a"))["pause_asked"].(map[string]any)["expires_at"]
	m.do(func() { *now = now.Add(10 * time.Second) })
	m.HandleControl("a", []byte(`{"type":"pause_request"}`))
	m.Sync()
	if asks := you(out.lastState(t, "host"))["pause_asks"].([]any); len(asks) != 1 {
		t.Fatalf("one guest, one request: %v", asks)
	}
	if again := you(out.lastState(t, "a"))["pause_asked"].(map[string]any)["expires_at"]; again == first || again != "2026-01-01T12:00:40Z" {
		t.Fatalf("expiry %v then %v", first, again)
	}
	// Expiry is a timer inside the actor; run it by hand.
	m.do(func() {
		i := slices.IndexFunc(m.pauseAsks, func(a pauseAsk) bool { return a.peer == "a" })
		m.dropPauseAsk(i)
		m.broadcastState()
	})
	m.Sync()
	if you(out.lastState(t, "a"))["pause_asked"] != nil {
		t.Fatal("still asked")
	}
}

func TestPauseRequestTimesOut(t *testing.T) {
	if testing.Short() {
		t.Skip("waits 30 s")
	}
	m, out, _, _ := pauseRoom(t)
	m.HandleControl("a", []byte(`{"type":"pause_request"}`))
	deadline := time.Now().Add(pauseAskFor + 5*time.Second)
	for time.Now().Before(deadline) {
		m.Sync()
		if you(out.lastState(t, "a"))["pause_asked"] == nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("the request never expired")
}

func TestHostOnline(t *testing.T) {
	m, out, _ := newManager(t)
	m.Join("a")
	m.Sync()
	if out.lastState(t, "a")["host_online"] != false {
		t.Fatal("online with no host")
	}
	m.SetHostLinked(true)
	m.Sync()
	if out.lastState(t, "a")["host_online"] != true {
		t.Fatal("a linked browser is the host online")
	}
	m.SetHostLinked(false)
	m.MarkOwner("host") // marked before its control channel opens
	m.Sync()
	if out.lastState(t, "a")["host_online"] != false {
		t.Fatal("an owner not in the room yet")
	}
	m.Join("host")
	m.Sync()
	if out.lastState(t, "a")["host_online"] != true || you(out.lastState(t, "host"))["owner"] != true {
		t.Fatal("the host in the room")
	}
	m.Leave("host")
	m.Sync()
	if out.lastState(t, "a")["host_online"] != false {
		t.Fatal("the host left")
	}
}

func TestCleanName(t *testing.T) {
	for in, want := range map[string]string{
		"Ana":                        "Ana",
		"  Ana   María  ":            "Ana María",
		"José Ñandú":                 "José Ñandú",
		"Jose\u0301":                 "José", // decomposed accent, joined (NFC)
		"<script>alert(1)</script>":  "scriptalert1script",
		"Fede 😀🎮":                    "Fede",
		"😀 Ana 🎮 Bo":                 "Ana Bo",
		"a":                          "", // too short
		"😀😀":                         "",
		"!!":                         "",
		"   ":                        "",
		"A1":                         "A1",
		"Player\tOne\nTwo":           "Player One Two",
		"李小龍":                        "李小龍",
		"Ölçer 2":                    "Ölçer 2",
		"ABCDEFGHIJKLMNOPQRSTUVWXYZ": "ABCDEFGHIJKLMNOPQRST",
		"abcdefghijklmnopqrs tuvw":   "abcdefghijklmnopqrs", // cut, no trailing space
		"a.b":                        "ab",
		"x_y-z":                      "xyz",
	} {
		if got := CleanName(in); got != want {
			t.Errorf("CleanName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHelloNameIsSanitizedEverywhere(t *testing.T) {
	m, out, _ := newManager(t)
	m.Join("a")
	m.Join("b")
	m.HandleControl("a", []byte(`{"type":"hello","name":"<b>Ana</b> 😀"}`))
	m.HandleControl("a", []byte(`{"type":"chat","text":"hi"}`))
	m.HandleControl("b", []byte(`{"type":"hello","name":"@"}`))
	m.Sync()
	if got := you(out.lastState(t, "a"))["name"]; got != "bAnab" {
		t.Fatalf("you.name = %v", got)
	}
	if got := you(out.lastState(t, "b"))["name"]; got != "Guest B" {
		t.Fatalf("a name too short keeps the generated one: %v", got)
	}
	for _, c := range out.chats("b") {
		if c["text"] == "hi" && c["name"] != "bAnab" {
			t.Fatalf("chat name %v", c["name"])
		}
	}
}

func TestStateCarriesTheGameControls(t *testing.T) {
	m, out, _ := newManager(t)
	m.Join("a")
	m.Sync()
	c, _ := out.lastState(t, "a")["controls"].(map[string]any)
	if c["buttons"] != float64(6) || c["control"] != "joy8way" {
		t.Fatalf("test card controls %v", c)
	}
	m.SetControls(GameControls{Players: 2, Buttons: 2, Control: "joy4way"})
	m.Sync()
	c, _ = out.lastState(t, "a")["controls"].(map[string]any)
	if c["buttons"] != float64(2) || c["control"] != "joy4way" || c["players"] != float64(2) {
		t.Fatalf("game controls %v", c)
	}
}

func swapsOf(st map[string]any, key string) []any { return you(st)[key].([]any) }

func TestSwapControllersNeedsTheOthersYes(t *testing.T) {
	m, out, _ := newManager(t)
	m.Join("a")
	m.Join("b")
	m.Sync()
	// b (P2) asks a (P1) for the joystick.
	m.HandleControl("b", []byte(`{"type":"swap_seat","from":2,"to":1}`))
	m.Sync()
	if offers := swapsOf(out.lastState(t, "a"), "swap_offers"); len(offers) != 1 {
		t.Fatalf("a offers %v", offers)
	}
	if asked := swapsOf(out.lastState(t, "b"), "swap_asked"); len(asked) != 1 {
		t.Fatalf("b asked %v", asked)
	}
	if p, _ := m.PortOf("b", 0); p != 2 {
		t.Fatal("seats changed before the answer")
	}
	// Only the asked player answers.
	m.HandleControl("b", []byte(`{"type":"swap_answer","from":2,"to":1,"accept":true}`))
	m.Sync()
	if p, _ := m.PortOf("b", 0); p != 2 {
		t.Fatal("the asker answered its own request")
	}
	m.HandleControl("a", []byte(`{"type":"swap_answer","from":2,"to":1,"accept":true}`))
	m.Sync()
	if pa, _ := m.PortOf("a", 0); pa != 2 {
		t.Fatalf("a is P%d, want P2", pa)
	}
	if pb, _ := m.PortOf("b", 0); pb != 1 {
		t.Fatalf("b is P%d, want P1", pb)
	}
	if len(swapsOf(out.lastState(t, "a"), "swap_offers")) != 0 {
		t.Fatal("the request stayed after the answer")
	}
}

func TestSwapDeclinedOrToAFreeSeat(t *testing.T) {
	m, _, _ := newManager(t)
	m.Join("a")
	m.Join("b")
	m.Sync()
	m.HandleControl("b", []byte(`{"type":"swap_seat","from":2,"to":1}`))
	m.HandleControl("a", []byte(`{"type":"swap_answer","from":2,"to":1,"accept":false}`))
	m.Sync()
	if p, _ := m.PortOf("a", 0); p != 1 {
		t.Fatal("a lost P1 after saying no")
	}
	// A free seat is taken at once.
	m.HandleControl("b", []byte(`{"type":"swap_seat","from":2,"to":4}`))
	m.Sync()
	if p, _ := m.PortOf("b", 0); p != 4 {
		t.Fatalf("b is P%d, want P4", p)
	}
	// Nobody moves someone else.
	m.HandleControl("a", []byte(`{"type":"swap_seat","from":4,"to":3}`))
	m.Sync()
	if p, _ := m.PortOf("b", 0); p != 4 {
		t.Fatal("a moved b")
	}
}

func TestAGuestTakesOneSeatWithSeveralControllers(t *testing.T) {
	m, out, _ := newManager(t)
	m.Join("guest")
	m.HandleControl("guest", []byte(`{"type":"hello","name":"Ana","local_players":[0,1,2]}`))
	m.Sync()
	if got := ports(out.lastState(t, "guest")); len(got) != 1 {
		t.Fatalf("a guest with three controllers has ports %v", got)
	}
	if _, ok := m.PortOf("guest", 1); ok {
		t.Fatal("a guest's second controller took a seat")
	}
	// The same browser as the host's seats them all.
	m.MarkOwner("guest")
	m.Sync()
	if got := ports(out.lastState(t, "guest")); len(got) != 3 {
		t.Fatalf("the host's ports %v", got)
	}
}

func TestSwapBetweenPlayersOfOneBrowserIsImmediate(t *testing.T) {
	m, _, _ := newManager(t)
	m.Join("a")
	m.MarkOwner("a")
	m.HandleControl("a", []byte(`{"type":"hello","local_players":[0,1]}`))
	m.Sync()
	m.HandleControl("a", []byte(`{"type":"swap_seat","from":1,"to":2}`))
	m.Sync()
	if p, _ := m.PortOf("a", 0); p != 2 {
		t.Fatalf("local 0 is P%d, want P2", p)
	}
	if p, _ := m.PortOf("a", 1); p != 1 {
		t.Fatalf("local 1 is P%d, want P1", p)
	}
}

func TestSwapRequestDropsWhenAPlayerLeaves(t *testing.T) {
	m, out, _ := newManager(t)
	m.Join("a")
	m.Join("b")
	m.Sync()
	m.HandleControl("b", []byte(`{"type":"swap_seat","from":2,"to":1}`))
	m.Leave("b")
	m.Sync()
	if len(swapsOf(out.lastState(t, "a"), "swap_offers")) != 0 {
		t.Fatal("a still sees the request of a player who left")
	}
}

func TestCrossedSwapRequestsSwapAtOnce(t *testing.T) {
	m, out, _ := newManager(t)
	m.Join("a")
	m.Join("b")
	m.Sync()
	m.HandleControl("a", []byte(`{"type":"swap_seat","from":1,"to":2}`))
	m.HandleControl("b", []byte(`{"type":"swap_seat","from":2,"to":1}`))
	m.Sync()
	if p, _ := m.PortOf("a", 0); p != 2 {
		t.Fatalf("a is P%d, want P2", p)
	}
	if len(swapsOf(out.lastState(t, "b"), "swap_offers")) != 0 {
		t.Fatal("a crossed request stayed")
	}
}

// typingOf returns the names in the latest "typing" message sent to peer.
func (o *outbox) typingOf(t *testing.T, peer string) []string {
	t.Helper()
	o.mu.Lock()
	defer o.mu.Unlock()
	for i := len(o.msgs[peer]) - 1; i >= 0; i-- {
		if o.msgs[peer][i]["type"] == "typing" {
			var names []string
			for _, n := range o.msgs[peer][i]["names"].([]any) {
				names = append(names, n.(map[string]any)["name"].(string))
			}
			return names
		}
	}
	return nil
}

func TestTypingGoesToTheOthersAndStopsWithTheMessage(t *testing.T) {
	m, out, _ := newManager(t)
	m.Join("a")
	m.Join("b")
	m.HandleControl("a", []byte(`{"type":"hello","name":"Ana"}`))
	m.HandleControl("a", []byte(`{"type":"typing","on":true}`))
	m.Sync()
	if got := out.typingOf(t, "b"); len(got) != 1 || got[0] != "Ana" {
		t.Fatalf("b sees %v typing", got)
	}
	if got := out.typingOf(t, "a"); len(got) != 0 {
		t.Fatalf("a sees itself typing: %v", got)
	}
	m.HandleControl("a", []byte(`{"type":"chat","text":"hi"}`))
	m.Sync()
	if got := out.typingOf(t, "b"); len(got) != 0 {
		t.Fatalf("still typing after sending: %v", got)
	}
}

func TestChatOffDropsMessagesAndTyping(t *testing.T) {
	m, out, _ := newManager(t)
	m.Join("a")
	m.Join("b")
	m.SetChat(false)
	m.Sync()
	if out.lastState(t, "b")["chat"] != false {
		t.Fatal("room_state does not say the chat is off")
	}
	before := len(out.chats("b"))
	m.HandleControl("a", []byte(`{"type":"chat","text":"hi"}`))
	m.HandleControl("a", []byte(`{"type":"typing","on":true}`))
	m.Sync()
	for _, c := range out.chats("b")[before:] {
		if c["text"] == "hi" {
			t.Fatal("a chat message got through with the chat off")
		}
	}
	if got := out.typingOf(t, "b"); len(got) != 0 {
		t.Fatalf("typing with the chat off: %v", got)
	}
	m.SetChat(true)
	m.HandleControl("a", []byte(`{"type":"chat","text":"back"}`))
	m.Sync()
	if last := out.chats("b"); last[len(last)-1]["text"] != "back" {
		t.Fatal("the chat did not come back")
	}
}

func TestAGuestCannotMakeTheRoomFloodTheOthers(t *testing.T) {
	m, out, _ := newManager(t)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	m.cfg.Now = func() time.Time { return now }
	m.Join("a")
	m.Join("b")
	m.Sync()
	out.mu.Lock()
	before := len(out.msgs["b"])
	out.mu.Unlock()
	// 200 renames in the same second: each would send everyone a new state.
	for i := range 200 {
		m.HandleControl("a", []byte(fmt.Sprintf(`{"type":"hello","name":"n%d"}`, i)))
	}
	m.Sync()
	out.mu.Lock()
	got := len(out.msgs["b"]) - before
	out.mu.Unlock()
	if got > controlBurst+2 {
		t.Fatalf("b received %d messages from one flooding guest", got)
	}
	// The next second the guest is heard again.
	now = now.Add(time.Second)
	m.HandleControl("a", []byte(`{"type":"hello","name":"Ana"}`))
	m.Sync()
	if st := out.lastState(t, "b"); !strings.Contains(fmt.Sprint(st), "Ana") {
		t.Fatalf("the guest was not heard after a second: %v", st)
	}
}

func TestDefaultNamesUseThePeersOwnID(t *testing.T) {
	for peer, want := range map[string]string{
		"9f3ab2c4d5":         "Guest 9F3A",
		"panel-41c482756881": "Guest 41C4",
		"ab":                 "Guest AB",
	} {
		if got := defaultName(peer); got != want {
			t.Errorf("defaultName(%q) = %q, want %q", peer, got, want)
		}
	}
}

func TestRoomStateCarriesTheHostsDefaultPicture(t *testing.T) {
	m, out, _ := newManager(t)
	m.Join("a")
	m.Sync()
	if _, ok := out.lastState(t, "a")["picture"]; ok {
		t.Fatal("no default picture: room_state leaves it out (the site's default)")
	}
	m.SetPicture(&models.RoomPicture{Style: "crt", Bands: "ambient"})
	m.Sync()
	pic, _ := out.lastState(t, "a")["picture"].(map[string]any)
	if pic["style"] != "crt" || pic["bands"] != "ambient" {
		t.Fatalf("picture %v", out.lastState(t, "a")["picture"])
	}
	// A guest who comes later gets it too.
	m.Join("b")
	m.Sync()
	if pic, _ := out.lastState(t, "b")["picture"].(map[string]any); pic["style"] != "crt" {
		t.Fatalf("late guest picture %v", pic)
	}
	// Unknown values are ignored: back to the site's default.
	m.SetPicture(&models.RoomPicture{Style: "vaporwave", Bands: "ambient"})
	m.Sync()
	if _, ok := out.lastState(t, "a")["picture"]; ok {
		t.Fatal("an unknown style reached the guests")
	}
	m.SetPicture(&models.RoomPicture{Style: "sharp", Bands: "frame"})
	m.SetPicture(nil)
	m.Sync()
	if _, ok := out.lastState(t, "a")["picture"]; ok {
		t.Fatal("cleared, the picture must be left out")
	}
}

func TestSeatsForTheGamesPlayers(t *testing.T) {
	for _, c := range []struct{ players, seats int }{{0, 4}, {1, 1}, {2, 2}, {4, 4}, {6, 4}} {
		if got := SeatsFor(GameControls{Players: c.players}); got != c.seats {
			t.Fatalf("SeatsFor(%d players) = %d, want %d", c.players, got, c.seats)
		}
	}
}

func TestTwoPlayerGameQueuesTheThird(t *testing.T) {
	out := newOutbox()
	var sums []RoomSummary
	m := NewRoomManager(RoomManagerConfig{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), MaxPlayers: 2, OnSummary: func(s RoomSummary) { sums = append(sums, s) }}, out)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go m.Run(ctx)
	for _, p := range []string{"a", "b", "c"} {
		m.Join(p)
	}
	m.Sync()
	st := out.lastState(t, "c")
	if st["max_players"] != float64(2) || len(st["seats"].([]any)) != 2 {
		t.Fatalf("max_players %v, seats %v", st["max_players"], st["seats"])
	}
	if _, ok := m.PortOf("c", 0); ok {
		t.Fatal("the third guest of a two player game has a port")
	}
	if last := sums[len(sums)-1]; last.Players != 2 || last.Queue != 1 || last.MaxPlayers != 2 {
		t.Fatalf("summary %+v", last)
	}
}

func TestTheHostsLatencyTestReachesTheRoomState(t *testing.T) {
	m, out, _ := newManager(t)
	m.Join("guest")
	m.Sync()
	if st := out.lastState(t, "guest"); st["input_hud"] != nil {
		t.Fatalf("input_hud %v before the host turns it on", st["input_hud"])
	}
	m.SetInputHUD(true)
	m.Sync()
	if st := out.lastState(t, "guest"); st["input_hud"] != true {
		t.Fatalf("input_hud %v", st["input_hud"])
	}
}

// The host frees a seat: its player goes to the end of the queue (the head
// of the queue takes the seat) or watches; a guest cannot do it.
func TestTheHostFreesASeat(t *testing.T) {
	out := newOutbox()
	m := NewRoomManager(RoomManagerConfig{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), MaxPlayers: 2}, out)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go m.Run(ctx)
	for _, p := range []string{"host", "b", "c"} {
		m.Join(p)
	}
	m.MarkOwner("host")
	m.Sync()
	port := func(p string) int {
		n, _ := m.PortOf(p, 0)
		return n
	}
	if port("host") != 1 || port("b") != 2 || port("c") != 0 {
		t.Fatalf("seats host %d b %d c %d", port("host"), port("b"), port("c"))
	}
	// A guest is refused.
	m.HandleControl("c", []byte(`{"type":"release_seat","port":2,"mode":"queue"}`))
	m.Sync()
	if port("b") != 2 {
		t.Fatal("a guest freed a seat")
	}
	// The host sends b to the queue: c, who waited, takes P2.
	m.HandleControl("host", []byte(`{"type":"release_seat","port":2,"mode":"queue"}`))
	m.Sync()
	if port("c") != 2 || port("b") != 0 {
		t.Fatalf("after the queue: b %d c %d", port("b"), port("c"))
	}
	if q := you(out.lastState(t, "b"))["queue_positions"].([]any); len(q) != 1 {
		t.Fatalf("b is not in the queue: %v", q)
	}
	// The host gives up its own seat: it waits at the end of the queue and
	// b, first in it, plays.
	m.HandleControl("host", []byte(`{"type":"release_seat","port":1,"mode":"queue"}`))
	m.Sync()
	if port("host") != 0 || port("b") != 1 {
		t.Fatalf("after the host's own seat: host %d b %d", port("host"), port("b"))
	}
	// c goes to watching: the host, who waited, takes P2.
	m.HandleControl("host", []byte(`{"type":"release_seat","port":2,"mode":"watch"}`))
	m.Sync()
	if port("c") != 0 || you(out.lastState(t, "c"))["spectator"] != true || port("host") != 2 {
		t.Fatalf("after watch: c %d host %d", port("c"), port("host"))
	}
	// Nobody else waits: "queue" works as "watch" and the seat stays free.
	m.HandleControl("host", []byte(`{"type":"release_seat","port":1,"mode":"queue"}`))
	m.Sync()
	if port("b") != 0 || you(out.lastState(t, "b"))["spectator"] != true {
		t.Fatalf("b: port %d, state %v", port("b"), you(out.lastState(t, "b")))
	}
	events := map[string]bool{}
	for _, c := range out.chats("b") {
		if e, ok := c["event"].(string); ok {
			events[e] = true
		}
	}
	if !events[EventSentToQueue] || !events[EventSentToWatch] {
		t.Fatalf("chat events %v", events)
	}
	// One line per release: no "left P2" after "moved c from P2 to watching".
	if events[EventLeftSeat] {
		t.Fatalf("a released seat also said left: %v", events)
	}
}
