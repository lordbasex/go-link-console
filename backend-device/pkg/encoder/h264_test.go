// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package encoder

import (
	"bufio"
	"bytes"
	"sync"
	"testing"
)

func TestSplitAccessUnitsCutsAtEachDelimiter(t *testing.T) {
	aud := []byte{0, 0, 0, 1, 0x09, 0xf0}
	sps := []byte{0, 0, 0, 1, 0x67, 1, 2}
	idr := []byte{0, 0, 1, 0x65, 0, 0, 3, 9, 9} // a slice whose data holds 00 00 03 and 0x09 bytes
	p := []byte{0, 0, 0, 1, 0x41, 7, 7, 7}
	stream := bytes.Join([][]byte{aud, sps, idr, aud, p, aud, p}, nil)
	var got [][]byte
	splitAccessUnits(bufio.NewReader(bytes.NewReader(stream)), func(au []byte) { got = append(got, au) })
	want := [][]byte{bytes.Join([][]byte{aud, sps, idr}, nil), bytes.Join([][]byte{aud, p}, nil), bytes.Join([][]byte{aud, p}, nil)}
	if len(got) != len(want) {
		t.Fatalf("got %d access units, want %d", len(got), len(want))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Errorf("access unit %d: got % x, want % x", i, got[i], want[i])
		}
	}
}

func TestH264EncodesFramesThroughFFmpeg(t *testing.T) {
	_, noFFmpeg := FFmpegPath()
	for _, kind := range H264Encoders {
		t.Run(kind, func(t *testing.T) {
			switch {
			case kind == "mediafoundation" && !hasMF, kind == "videotoolbox" && !hasVT && noFFmpeg != nil:
				t.Skip("not on this system")
			case kind == "x264" && noFFmpeg != nil:
				t.Skip(noFFmpeg)
			case kind == "nvenc" || kind == "vaapi":
				// a graphics card through ffmpeg: only where one works
				if err := TryH264(kind); err != nil {
					t.Skipf("%s: %v", kind, err)
				}
			}
			var mu sync.Mutex
			var frames [][]byte
			e, err := NewH264(Config{Width: 640, Height: 360, FPS: 60, BitrateKbps: 1000}, kind, func(au []byte) {
				mu.Lock()
				frames = append(frames, au)
				mu.Unlock()
			})
			if err != nil {
				t.Fatal(err)
			}
			frame := make([]byte, 640*360*3/2) // the Intel Mac's hardware encoder takes nothing smaller than about this
			for i := 0; i < 30; i++ {
				for k := range frame[:640*360] {
					frame[k] = byte(k + i*3)
				}
				if err := e.Write(frame); err != nil {
					t.Skipf("%s: %v (this ffmpeg may lack it)", kind, err)
				}
			}
			e.Close()
			mu.Lock()
			defer mu.Unlock()
			if len(frames) < 25 {
				t.Skipf("%s gave %d frames for 30 (this ffmpeg may lack it)", kind, len(frames))
			}
			// the first frame is a keyframe with its parameter sets: SPS (7), PPS (8), IDR (5)
			types := map[byte]bool{}
			for i := 0; i+3 < len(frames[0]); i++ {
				if frames[0][i] == 0 && frames[0][i+1] == 0 && frames[0][i+2] == 1 {
					types[frames[0][i+3]&0x1f] = true
				}
			}
			if !types[7] || !types[8] || !types[5] {
				t.Errorf("first frame NAL types %v, want SPS, PPS and IDR", types)
			}
		})
	}
}
