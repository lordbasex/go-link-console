// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"sync"
	"time"

	"github.com/pion/rtp"
)

// Every guest connection carries four voice tracks (voice-p1..voice-p4)
// from its first offer, so nothing renegotiates when people move. More
// people than that may talk (the players and the queue), so the tracks are
// slots: each one carries one of the people speaking now. A slot belongs to
// whoever started speaking in it while that person keeps talking; once
// quiet for slotQuiet it may go to someone else who starts, and after
// slotIdle without a sound it is free. A fifth voice waits for a slot.
const (
	voiceSlots = 4
	slotQuiet  = 800 * time.Millisecond
	slotIdle   = 5 * time.Second
	// voiceLoud is the loudest level (in -dBov, RFC 6464: 0 is the
	// loudest, 127 silence) that counts as speech.
	voiceLoud = 50
)

type voiceSlot struct {
	peer     string
	loudAt   time.Time // last packet with speech
	heardAt  time.Time // last packet at all
	out      slotSeq
	assigned bool
}

// voiceSelector picks which speaker each slot carries. Safe for the
// goroutines that read the microphones (one per guest).
type voiceSelector struct {
	now      func() time.Time
	onChange func(peers [voiceSlots]string) // called without the lock held

	mu    sync.Mutex
	slots [voiceSlots]voiceSlot
}

func newVoiceSelector(now func() time.Time) *voiceSelector {
	if now == nil {
		now = time.Now
	}
	return &voiceSelector{now: now}
}

// Pick returns the slot (0-3) that carries this packet of peer's voice,
// rewritten to continue that slot's stream, or -1 to drop it. levelID is
// the audio level extension's id (kept on the packet; 0: none).
func (vs *voiceSelector) Pick(peer string, loud bool, pkt *rtp.Packet, levelID int) int {
	now := vs.now()
	vs.mu.Lock()
	slot := -1
	changed := false
	for i := range vs.slots {
		s := &vs.slots[i]
		if s.assigned && s.peer != peer && now.Sub(s.heardAt) >= slotIdle {
			s.assigned, s.peer = false, "" // gone quiet for good: free
			changed = true
		}
		if s.assigned && s.peer == peer {
			slot = i
		}
	}
	if slot < 0 && loud {
		slot = vs.claim(now)
		if slot >= 0 {
			s := &vs.slots[slot]
			s.assigned, s.peer, s.loudAt = true, peer, now
			changed = true
		}
	}
	if slot >= 0 {
		s := &vs.slots[slot]
		s.heardAt = now
		if loud {
			s.loudAt = now
		}
		s.out.rewrite(peer, slot, pkt, now, levelID)
	}
	var peers [voiceSlots]string
	if changed {
		peers = vs.peersLocked()
	}
	fn := vs.onChange
	vs.mu.Unlock()
	if changed && fn != nil {
		fn(peers)
	}
	return slot
}

// claim finds a slot for a new speaker: a free one first, else the one
// quiet the longest, if quiet for slotQuiet at least.
func (vs *voiceSelector) claim(now time.Time) int {
	best := -1
	for i := range vs.slots {
		s := &vs.slots[i]
		if !s.assigned {
			return i
		}
		if now.Sub(s.loudAt) >= slotQuiet && (best < 0 || s.loudAt.Before(vs.slots[best].loudAt)) {
			best = i
		}
	}
	return best
}

// Drop frees peer's slot (it left, or may no longer talk).
func (vs *voiceSelector) Drop(peer string) {
	vs.mu.Lock()
	changed := false
	for i := range vs.slots {
		if vs.slots[i].assigned && vs.slots[i].peer == peer {
			vs.slots[i].assigned, vs.slots[i].peer = false, ""
			changed = true
		}
	}
	peers := vs.peersLocked()
	fn := vs.onChange
	vs.mu.Unlock()
	if changed && fn != nil {
		fn(peers)
	}
}

// Peers is who each slot carries now ("" for a free slot).
func (vs *voiceSelector) Peers() [voiceSlots]string {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	return vs.peersLocked()
}

func (vs *voiceSelector) peersLocked() [voiceSlots]string {
	var out [voiceSlots]string
	for i, s := range vs.slots {
		if s.assigned {
			out[i] = s.peer
		}
	}
	return out
}

// slotSeq makes one slot a single RTP stream for the browsers that hear
// it: when the slot passes to another speaker, sequence numbers and
// timestamps go on from where they were (the time in between included),
// so the receiver's jitter buffer sees one voice that paused, not a jump.
type slotSeq struct {
	peer    string
	started bool
	seqOff  uint16
	tsOff   uint32
	lastSeq uint16
	lastTS  uint32
	lastAt  time.Time
}

// opusRate is the RTP clock of Opus.
const opusRate = 48000

func (o *slotSeq) rewrite(peer string, slot int, pkt *rtp.Packet, now time.Time, levelID int) {
	if !o.started || o.peer != peer {
		if o.started {
			gap := uint32(now.Sub(o.lastAt).Seconds()*opusRate) + 960 // at least one 20 ms frame
			o.seqOff = o.lastSeq + 1 - pkt.SequenceNumber
			o.tsOff = o.lastTS + gap - pkt.Timestamp
			pkt.Marker = true // a talk spurt starts
		} else {
			o.seqOff, o.tsOff = 0, 0
		}
		o.peer, o.started = peer, true
	}
	pkt.SequenceNumber += o.seqOff
	pkt.Timestamp += o.tsOff
	// One SSRC per slot (the device sets each receiver's own when sending).
	// Of the header extensions only the audio level stays: browsers show
	// who speaks from it, and the device negotiates the same id for it on
	// every connection (one media engine), so it means the same to the
	// listener. The others were the speaker's.
	pkt.SSRC = 0x766f6900 + uint32(slot)
	var level []byte
	if levelID > 0 {
		level = append([]byte(nil), pkt.GetExtension(uint8(levelID))...)
	}
	pkt.Extension, pkt.Extensions, pkt.ExtensionProfile = false, nil, 0
	if len(level) > 0 {
		_ = pkt.SetExtension(uint8(levelID), level)
	}
	o.lastSeq, o.lastTS, o.lastAt = pkt.SequenceNumber, pkt.Timestamp, now
}
