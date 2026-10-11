// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"testing"
	"time"

	"github.com/pion/rtp"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time      { return c.t }
func (c *clock) add(d time.Duration) { c.t = c.t.Add(d) }

func voicePkt(seq uint16, ts uint32) *rtp.Packet {
	p := &rtp.Packet{Header: rtp.Header{SequenceNumber: seq, Timestamp: ts, SSRC: 1234}, Payload: make([]byte, 80)}
	_ = p.SetExtension(1, []byte{10})      // the audio level: speech
	_ = p.SetExtension(3, []byte{1, 2, 3}) // another of the speaker's
	return p
}

func TestVoiceSlotsGoToTheFourPeopleSpeaking(t *testing.T) {
	c := &clock{t: time.Date(2026, 10, 10, 21, 0, 0, 0, time.UTC)}
	vs := newVoiceSelector(c.now)
	var changes [][voiceSlots]string
	vs.onChange = func(p [voiceSlots]string) { changes = append(changes, p) }

	// Silence never takes a slot.
	if s := vs.Pick("a", false, voicePkt(1, 0), 0); s != -1 {
		t.Fatalf("silence took slot %d", s)
	}
	for i, p := range []string{"a", "b", "c", "d"} {
		if s := vs.Pick(p, true, voicePkt(1, 0), 0); s != i {
			t.Fatalf("%s got slot %d", p, s)
		}
	}
	// A fifth voice waits while the four keep talking.
	if s := vs.Pick("e", true, voicePkt(1, 0), 0); s != -1 {
		t.Fatalf("a fifth voice got slot %d", s)
	}
	// Each keeps its slot, loud or quiet.
	if s := vs.Pick("c", false, voicePkt(2, 960), 0); s != 2 {
		t.Fatalf("c moved to %d", s)
	}
	// b goes quiet: after slotQuiet its slot can go to e.
	c.add(slotQuiet)
	for _, p := range []string{"a", "c", "d"} {
		vs.Pick(p, true, voicePkt(3, 1920), 0)
	}
	if s := vs.Pick("e", true, voicePkt(1, 0), 0); s != 1 {
		t.Fatalf("e got slot %d, want b's", s)
	}
	if got := vs.Peers(); got != [voiceSlots]string{"a", "e", "c", "d"} {
		t.Fatalf("slots %v", got)
	}
	// Someone who left frees the slot at once.
	vs.Drop("a")
	if got := vs.Peers(); got[0] != "" {
		t.Fatalf("a still has a slot: %v", got)
	}
	// A slot nobody is heard in for slotIdle is free again.
	c.add(slotIdle)
	vs.Pick("f", true, voicePkt(1, 0), 0)
	if got := vs.Peers(); got != [voiceSlots]string{"f", "", "", ""} {
		t.Fatalf("after idle: %v", got)
	}
	if len(changes) == 0 || changes[len(changes)-1] != vs.Peers() {
		t.Fatalf("changes %v", changes)
	}
}

func TestAVoiceSlotIsOneStreamAcrossSpeakers(t *testing.T) {
	c := &clock{t: time.Date(2026, 10, 10, 21, 0, 0, 0, time.UTC)}
	vs := newVoiceSelector(c.now)
	p := voicePkt(500, 100000)
	vs.Pick("a", true, p, 1)
	// Only the audio level stays of the speaker's header extensions.
	if p.SequenceNumber != 500 || p.Timestamp != 100000 || p.SSRC != 0x766f6900 || len(p.GetExtensionIDs()) != 1 || p.GetExtension(1)[0] != 10 {
		t.Fatalf("first packet %+v", p.Header)
	}
	p = voicePkt(501, 100960)
	c.add(20 * time.Millisecond)
	vs.Pick("a", true, p, 1)
	if p.SequenceNumber != 501 || p.Timestamp != 100960 {
		t.Fatalf("second packet %+v", p.Header)
	}
	// a stops; a second later b takes slot 0 with numbers of its own:
	// they go on from a's, with the second in between.
	vs.Drop("a")
	c.add(time.Second)
	p = voicePkt(9, 7)
	vs.Pick("b", true, p, 1)
	if p.SequenceNumber != 502 || p.Timestamp != 100960+48000+960 || !p.Marker {
		t.Fatalf("b's first packet %+v", p.Header)
	}
	p = voicePkt(10, 967)
	c.add(20 * time.Millisecond)
	vs.Pick("b", true, p, 1)
	if p.SequenceNumber != 503 || p.Timestamp != 100960+48000+960+960 {
		t.Fatalf("b's second packet %+v", p.Header)
	}
}

func TestVoiceIsLoud(t *testing.T) {
	p := voicePkt(1, 0)
	if !voiceIsLoud(p, 1) {
		t.Fatal("level 10 is speech")
	}
	_ = p.SetExtension(1, []byte{0x80 | 90})
	if voiceIsLoud(p, 1) {
		t.Fatal("level 90 is silence")
	}
	// Without the extension: the packet's size.
	if !voiceIsLoud(&rtp.Packet{Payload: make([]byte, 80)}, 0) || voiceIsLoud(&rtp.Packet{Payload: make([]byte, 8)}, 0) {
		t.Fatal("size fallback")
	}
}

func TestTheQueueTalksAndSpectatorsHearWhenTheHostLetsThem(t *testing.T) {
	m, out, _ := handRoom(t) // host P1, b P2, c in the queue, s watching
	role := func(p string) [2]bool { a, b := m.VoiceRole(p); return [2]bool{a, b} }
	if role("host") != [2]bool{true, true} || role("c") != [2]bool{true, true} || role("s") != [2]bool{false, true} {
		t.Fatalf("roles host %v c %v s %v", role("host"), role("c"), role("s"))
	}
	if out.lastState(t, "s")["spectators_hear"] != true {
		t.Fatal("spectators hear by default")
	}
	m.SetSpectatorsHear(false)
	m.Sync()
	if role("s") != [2]bool{false, false} || out.lastState(t, "s")["spectators_hear"] != false {
		t.Fatal("the host's switch did not stop the spectators hearing")
	}
	// Who each voice track carries, with their seat (0 in the queue).
	m.SetVoiceSlots([4]string{"c", "b", "", ""})
	m.Sync()
	slots := out.lastState(t, "b")["voice_slots"].([]any)
	first, second := slots[0].(map[string]any), slots[1].(map[string]any)
	if first["name"] == nil || first["port"] != nil || second["port"] != 2.0 || second["you"] != true || slots[2] != nil {
		t.Fatalf("slots %v", slots)
	}
	// Leaving takes the right away at once.
	m.Leave("c")
	m.Sync()
	if role("c") != [2]bool{false, false} {
		t.Fatal("someone who left may still talk")
	}
}
