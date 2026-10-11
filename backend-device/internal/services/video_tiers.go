// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/lordbasex/go-link/backend-device/pkg/encoder"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

// Video tiers (go-link HD, docs/experiments/hd-streaming.md): with
// StreamConfig.Tiers the picture is sent at up to three sizes, the source's
// and halves of it (4K, 1080p, 540p), each its own encoder and track, and
// every viewer gets the smallest one that fills its video at its size on
// screen (video_want on the control channel). Every viewer has its own peer
// connection (the star topology) and, here, its own video track: a tier's
// frames are written to the tracks of the viewers watching it, so moving a
// viewer to another tier only changes which frames reach its track, whose
// RTP sequence and timestamps run on unbroken (swapping tracks with
// ReplaceTrack restarted them at random and a lower sequence made the
// browser drop every packet). A tier is encoded only while a viewer
// watches it; a moved viewer starts at the tier's next keyframe.

// Below its smallest size, every stream has qualitySteps more tiers of that
// same size at half and a quarter of its bitrate: a player whose connection
// struggles drops one at a time (quality_ladder.go) without changing what
// anyone else gets. An arcade game at 2x is too small to halve, so for game
// rooms these are the only tiers besides the first.

// maxTiers is how many sizes a tiered stream offers.
const maxTiers = 3

// qualitySteps are the lower-bitrate tiers under the smallest size.
const qualitySteps = 2

// minTierHeight is the smallest tier: halving stops before going under it.
const minTierHeight = 360

type videoTier struct {
	vp8      *encoder.VP8
	h264     *encoder.H264
	recVP8   *encoder.VP8 // with H.264, the recording's own VP8 (WebM) while a room records
	w, h     int
	kbps     int
	keyframe atomic.Bool
	idle     int          // frames without a viewer
	buf      []byte       // the halved picture
	bytes    atomic.Int64 // sent since the last count
}

func (t *videoTier) close() {
	if t.vp8 != nil {
		t.vp8.Close()
		t.vp8 = nil
	}
	if t.h264 != nil {
		t.h264.Close()
		t.h264 = nil
	}
	t.closeRec()
	t.w, t.h = 0, 0
}

func (t *videoTier) closeRec() {
	if t.recVP8 != nil {
		t.recVP8.Close()
		t.recVP8 = nil
	}
}

// recordH264 gives a recording the full picture in VP8 when the room sends
// H.264: recordings are WebM, which carries VP8 (the browser makes the MP4).
func (s *StreamService) recordH264(t *videoTier, cfg encoder.Config, i420 []byte) {
	rec := s.rec.Load()
	if rec == nil {
		t.closeRec()
		return
	}
	if t.recVP8 == nil {
		enc, err := encoder.NewVP8(cfg)
		if err != nil {
			s.log.Error("recording encoder", "err", err)
			return
		}
		t.recVP8 = enc
	}
	if data, _, err := t.recVP8.Encode(i420, false); err == nil && len(data) > 0 {
		rec.Video(data, cfg.Width, cfg.Height)
	}
}

// tierLevels is how many tiers a w x h source has: the source and each half
// that keeps 4:2:0 whole and is at least minTierHeight tall.
func tierLevels(w, h int) int {
	n := 1
	for n < maxTiers && w%4 == 0 && h%4 == 0 && h/2 >= minTierHeight {
		w, h = w/2, h/2
		n++
	}
	return n
}

// tierFor picks a viewer's tier: the smallest picture that is at least 90 %
// of the size the viewer shows it at.
func tierFor(w, h, wantW, wantH, levels int) int {
	l := 0
	for l+1 < levels && (w>>(l+1))*10 >= wantW*9 && (h>>(l+1))*10 >= wantH*9 {
		l++
	}
	return l
}

// tierKbps gives each tier about a third of the one above (4K 25, 1080p 8, 540p 2.7 Mbps).
func tierKbps(kbps, level int) int {
	for ; level > 0; level-- {
		kbps /= 3
	}
	return max(kbps, 300)
}

// stepKbps is the bitrate of tier l of a stream with sizes size tiers: a
// size tier's own (tierKbps), halved for each quality step under the
// smallest size, never under 200 kbps.
func stepKbps(kbps, l, sizes int) int {
	last := max(sizes-1, 0)
	if l <= last {
		return tierKbps(kbps, l)
	}
	return max(tierKbps(kbps, last)>>(l-last), 200)
}

// halveI420 averages each 2x2 block of a w x h I420 picture into dst (w/2 x h/2).
func halveI420(dst, src []byte, w, h int) []byte {
	w2, h2 := w/2, h/2
	need := w2*h2 + 2*(w2/2)*(h2/2)
	if cap(dst) < need {
		dst = make([]byte, need)
	}
	dst = dst[:need]
	plane := func(out, in []byte, pw, ph int) {
		ow := pw / 2
		for y := 0; y < ph/2; y++ {
			r0, r1 := in[2*y*pw:], in[(2*y+1)*pw:]
			o := out[y*ow:]
			for x := 0; x < ow; x++ {
				o[x] = byte((int(r0[2*x]) + int(r0[2*x+1]) + int(r1[2*x]) + int(r1[2*x+1]) + 2) >> 2)
			}
		}
	}
	plane(dst, src, w, h)
	cw, ch := w/2, h/2
	plane(dst[w2*h2:], src[w*h:], cw, ch)
	plane(dst[w2*h2+(w2/2)*(h2/2):], src[w*h+cw*ch:], cw, ch)
	return dst
}

func (s *StreamService) newTiers() {
	s.tiers = make([]*videoTier, maxTiers+qualitySteps)
	for i := range s.tiers {
		s.tiers[i] = &videoTier{}
	}
}

// tierTracks are the video tracks of the viewers watching tier l.
func (s *StreamService) tierTracks(l int) []*webrtc.TrackLocalStaticSample {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*webrtc.TrackLocalStaticSample
	for _, v := range s.viewers {
		if v.kind == KindViewer && v.video != nil && s.tierOfLocked(v) == l {
			out = append(out, v.video)
		}
	}
	return out
}

// writeTier sends one encoded frame of tier l to its viewers.
func (s *StreamService) writeTier(l int, data []byte, dur time.Duration) {
	for _, track := range s.tierTracks(l) {
		if err := track.WriteSample(media.Sample{Data: data, Duration: dur}); err != nil && !errors.Is(err, errClosedPipe) {
			s.log.Debug("write sample", "err", err)
		}
	}
}

// tierOf is a viewer's tier now: the size it shows, then as many quality
// steps down as its connection asked for (callers hold s.mu).
func (s *StreamService) tierOfLocked(v *viewer) int {
	sizes := max(s.tierLevels, 1)
	drop := v.drop
	if s.room.Saver != nil && s.room.Saver(v.id) {
		drop = max(drop, 1) // a spectator's lighter video
	}
	return min(min(v.tier, sizes-1)+drop, sizes-1+qualitySteps)
}

// sizeOfLocked is the size tier a tier shows (callers hold s.mu).
func (s *StreamService) sizeOfLocked(l int) int {
	return min(l, max(s.tierLevels-1, 0))
}

// setVideoWant moves a viewer to the tier that fits the size it shows the
// video at (device pixels); the new tier starts with a keyframe.
func (s *StreamService) setVideoWant(v *viewer, w, h int) {
	if s.tiers == nil || w <= 0 || h <= 0 {
		return
	}
	s.mu.Lock()
	v.wantW, v.wantH = w, h
	l := tierFor(s.vp8W, s.vp8H, w, h, max(s.tierLevels, 1))
	if s.vp8W == 0 {
		l = 0 // no picture yet: chosen again on the first frame
	}
	old := s.tierOfLocked(v)
	v.tier = l
	now := s.tierOfLocked(v)
	s.mu.Unlock()
	if now == old {
		return
	}
	s.tiers[now].keyframe.Store(true)
	s.log.Info("video tier", "viewer", v.id, "want", fmt.Sprintf("%dx%d", w, h), "tier", l)
	go s.sendStreamStats()
}

// tieredFrame encodes the tiers viewers watch: the source, then each half,
// then the quality steps of the smallest size.
func (s *StreamService) tieredFrame(i420 []byte, w, h, kbps int, dur time.Duration) {
	sizes := tierLevels(w, h)
	levels := sizes + qualitySteps
	var need [maxTiers + qualitySteps]bool
	s.mu.Lock()
	resized := s.vp8W != w || s.vp8H != h
	s.vp8W, s.vp8H, s.tierLevels = w, h, sizes
	for _, v := range s.viewers {
		if v.kind != KindViewer {
			continue
		}
		if resized && v.wantW > 0 {
			if l := tierFor(w, h, v.wantW, v.wantH, sizes); l != v.tier {
				v.tier = l
				s.tiers[s.tierOfLocked(v)].keyframe.Store(true)
			}
		}
		l := s.tierOfLocked(v)
		if l != v.lastTier {
			// Moved by something else than video_want (a spectator who now
			// plays, or the room's saver switch): the tier starts with a
			// keyframe for it.
			v.lastTier = l
			s.tiers[l].keyframe.Store(true)
		}
		need[l] = true
	}
	s.mu.Unlock()
	if s.rec.Load() != nil {
		need[0] = true // a recording takes the full picture
	}
	if resized {
		go s.sendStreamStats()
	}
	watched := false
	for l, t := range s.tiers {
		s.sentBytes += int(t.bytes.Swap(0))
		watched = watched || need[l]
	}
	if watched {
		s.sent++ // frames sent: one per source frame, at any size
	}
	cur, cw, ch := i420, w, h
	for l := 0; l < levels; l++ {
		t := s.tiers[l]
		if l > 0 {
			further := false
			for k := l; k < levels; k++ {
				further = further || need[k]
			}
			if !further {
				for k := l; k < len(s.tiers); k++ {
					s.tierIdle(s.tiers[k])
				}
				return
			}
			if l < sizes {
				t.buf = halveI420(t.buf, cur, cw, ch)
				cur, cw, ch = t.buf, cw/2, ch/2
			}
		}
		if !need[l] {
			s.tierIdle(t)
			continue
		}
		t.idle = 0
		s.tierEncode(t, l, cur, cw, ch, stepKbps(kbps, l, sizes), dur)
	}
}

// tierIdle closes a tier's encoder after two seconds without viewers.
func (s *StreamService) tierIdle(t *videoTier) {
	if t.vp8 == nil && t.h264 == nil {
		return
	}
	if t.idle++; t.idle > 120 {
		t.close()
	}
}

func (s *StreamService) tierEncode(t *videoTier, level int, i420 []byte, w, h, kbps int, dur time.Duration) {
	fps := max(int(time.Second/dur), 1)
	cfg := encoder.Config{Width: w, Height: h, FPS: fps, BitrateKbps: kbps, Threads: s.cfg.EncoderThreads, GOPFrames: fps}
	if t.w != w || t.h != h || t.kbps != kbps || (t.vp8 == nil && t.h264 == nil) {
		t.close()
		var err error
		if s.cfg.H264Encoder != "" {
			t.h264, err = encoder.NewH264(cfg, s.cfg.H264Encoder, func(au []byte) {
				s.writeTier(level, au, dur)
				t.bytes.Add(int64(len(au)))
			})
		} else {
			t.vp8, err = encoder.NewVP8(cfg)
		}
		if err != nil {
			s.log.Error("video encoder", "tier", level, "err", err)
			t.close()
			return
		}
		t.w, t.h, t.kbps = w, h, kbps
		t.keyframe.Store(true)
		s.log.Info("video tier encoder", "tier", level, "size", fmt.Sprintf("%dx%d", w, h), "kbps", kbps, "h264", s.cfg.H264Encoder)
	}
	if t.h264 != nil {
		if level == 0 {
			s.recordH264(t, cfg, i420)
		}
		if err := t.h264.WriteKey(i420, t.keyframe.Swap(false)); err != nil {
			s.log.Error("encode failed", "tier", level, "err", err)
			t.close()
		}
		return
	}
	start := time.Now()
	data, _, err := t.vp8.Encode(i420, t.keyframe.Swap(false))
	if level == 0 {
		s.noteEncode(time.Since(start), dur)
	}
	if err != nil || len(data) == 0 {
		return
	}
	if rec := s.rec.Load(); rec != nil && level == 0 {
		rec.Video(data, w, h)
	}
	s.writeTier(level, data, dur)
	t.bytes.Add(int64(len(data)))
}
