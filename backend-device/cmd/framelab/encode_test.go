// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/lordbasex/go-link-console/backend-device/pkg/framelab"
)

func TestBitrates(t *testing.T) {
	// 2 seconds at 10 fps: 1000 bytes per frame, one 11000 byte frame.
	sizes := make([]int, 20)
	for i := range sizes {
		sizes[i] = 1000
	}
	sizes[15] = 11000
	avg, peak := bitrates(sizes, 10)
	if math.Abs(avg-(30000*8/1000/2.0)) > 1e-9 {
		t.Fatalf("avg %.2f", avg)
	}
	if math.Abs(peak-(20000*8/1000.0)) > 1e-9 {
		t.Fatalf("peak %.2f", peak)
	}
	if a, p := bitrates(nil, 10); a != 0 || p != 0 {
		t.Fatal("empty input")
	}
}

func TestReadIVF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.ivf")
	w, err := framelab.CreateIVF(path, "VP80", 16, 16, 60)
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte{0x10, 1, 2}, 0) // bit 0 clear: keyframe
	w.Write([]byte{0x11, 1}, 1)    // delta frame
	w.Write([]byte{0x11}, 3)       // frame 2 was skipped
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	pts, sizes, keys, err := readIVF(path)
	if err != nil || keys != 1 || len(pts) != 3 || pts[2] != 3 || sizes[0] != 3 {
		t.Fatalf("pts %v sizes %v keys %d err %v", pts, sizes, keys, err)
	}
}

func TestParseClip(t *testing.T) {
	if f, c, err := parseClip("3420:120"); err != nil || f != 3420 || c != 120 {
		t.Fatalf("%d %d %v", f, c, err)
	}
	for _, bad := range []string{"3420", "a:1", "1:0", "-1:5"} {
		if _, _, err := parseClip(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
	if l, err := parseInts("1, 2,,30"); err != nil || len(l) != 3 || l[2] != 30 {
		t.Fatalf("%v %v", l, err)
	}
}
