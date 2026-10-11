// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package testpattern

import (
	"bytes"
	"testing"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/pkg/encoder"
	"github.com/lordbasex/go-link-console/backend-device/pkg/input"
)

func TestFrameSizeAndClock(t *testing.T) {
	p := New(640, 480)
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	a := bytes.Clone(p.Frame(t0, Controls{}))
	if len(a) != encoder.FrameSize(640, 480) {
		t.Fatalf("frame size %d", len(a))
	}
	if bytes.Equal(a, p.Frame(t0.Add(time.Second), Controls{})) {
		t.Fatal("the clock does not change the picture")
	}
	if !bytes.Equal(a, p.Frame(t0, Controls{})) {
		t.Fatal("the same time must draw the same frame")
	}
}

func TestButtonsAreVisible(t *testing.T) {
	p := New(640, 480)
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	idle := bytes.Clone(p.Frame(t0, Controls{}))
	for i := 0; i < input.Count; i++ {
		if bytes.Equal(idle, p.Frame(t0, Controls{Buttons: input.State(1 << i)})) {
			t.Fatalf("button %d is not drawn", i)
		}
	}
	for _, c := range []Controls{
		{Axes: input.Axes{127, 0, 0, 0}},
		{Axes: input.Axes{0, 0, 0, -127}},
		{Players: [4]bool{false, true}},
		{FPS: 59.9},
	} {
		if bytes.Equal(idle, p.Frame(t0, c)) {
			t.Fatalf("%+v is not drawn", c)
		}
	}
}

func TestCardEncodes(t *testing.T) {
	p := New(640, 480)
	enc, err := encoder.NewVP8(encoder.Config{Width: 640, Height: 480, FPS: 30, BitrateKbps: 1500})
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	if data, key, err := enc.Encode(p.Frame(time.Now(), Controls{}), false); err != nil || !key || len(data) == 0 {
		t.Fatalf("encode: %v %v %d", err, key, len(data))
	}
}
