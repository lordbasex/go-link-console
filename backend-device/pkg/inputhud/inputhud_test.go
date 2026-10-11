// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package inputhud

import (
	"testing"

	"github.com/lordbasex/go-link-console/backend-device/pkg/input"
)

func blank(w, h int) []byte {
	buf := make([]byte, w*h+2*((w+1)/2)*((h+1)/2))
	for i := range buf[:w*h] {
		buf[i] = 120
	}
	return buf
}

// luma is the average brightness inside a beacon's rect.
func luma(buf []byte, w, h int, r Rect) float64 {
	x0, y0 := int(r.X*float64(w)), int(r.Y*float64(h))
	x1, y1 := int((r.X+r.W)*float64(w)), int((r.Y+r.H)*float64(h))
	sum, n := 0, 0
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			sum += int(buf[y*w+x])
			n++
		}
	}
	return float64(sum) / float64(n)
}

func TestTheBeaconLightsWhileItsSeatPresses(t *testing.T) {
	for _, size := range [][2]int{{384, 224}, {768, 448}, {320, 240}, {248, 256}, {1280, 720}} {
		w, h := size[0], size[1]
		pads := []input.Pad{{Buttons: input.State(input.Button1)}, {}, {Axes: input.Axes{0, 100, 0, 0}}, {}}
		buf := blank(w, h)
		Draw(buf, w, h, pads)
		beacons := Beacons(w, h, len(pads))
		if len(beacons) != 4 {
			t.Fatalf("%dx%d: %d beacons", w, h, len(beacons))
		}
		for i, b := range beacons {
			if b.X < 0 || b.Y < 0 || b.X+b.W > 1 || b.Y+b.H > 1 || b.W <= 0 {
				t.Fatalf("%dx%d: beacon %d out of the frame: %+v", w, h, i, b)
			}
			lit := luma(buf, w, h, b) > 200
			if want := Active(pads[i]); lit != want {
				t.Fatalf("%dx%d: beacon P%d lit %v, want %v", w, h, i+1, lit, want)
			}
		}
	}
}

func TestTwoSeatsUseBiggerCells(t *testing.T) {
	two, four := Beacons(384, 224, 2), Beacons(384, 224, 4)
	if two[0].W < four[0].W {
		t.Fatalf("two seats %+v, four %+v", two[0], four[0])
	}
	if Beacons(384, 224, 0) == nil || len(Beacons(384, 224, 0)) != 0 {
		t.Fatal("no seats, no beacons")
	}
}

func TestASmallFrameIsLeftAlone(t *testing.T) {
	buf := blank(8, 8)
	Draw(buf, 8, 8, []input.Pad{{Buttons: 1}})
	for _, v := range buf[:64] {
		if v != 120 {
			t.Fatal("drew on an 8x8 frame")
		}
	}
	Draw(buf[:10], 8, 8, []input.Pad{{}}) // short buffer: no panic
}
