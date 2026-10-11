// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package emuproc

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/pkg/input"
)

func TestRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	frame := make([]byte, FrameSizeI420(3, 3))
	for i := range frame {
		frame[i] = byte(i)
	}
	pads := [Ports]input.Pad{
		{Buttons: input.State(input.Up | input.Button1), Axes: input.Axes{-127, 127, 0, 5}},
		{},
		{Buttons: input.State(input.Coin)},
		{Buttons: input.State(1<<31 | 1), Axes: input.Axes{1, -1, 2, -2}},
	}
	pcm := []int16{0, -1, 32767, -32768}
	ready := Ready{Core: "MAME 2003-Plus", BaseWidth: 256, BaseHeight: 224, AspectRatio: 4.0 / 3, FPS: 59.94, SampleRate: 48000}
	steps := []func() error{
		func() error { return w.WriteJSON(TypeReady, ready) },
		func() error { return w.WriteVideo(3, 3, 1, 16683350*time.Nanosecond, frame) },
		func() error { return w.WriteVideo(3, 3, 2, time.Second/60, nil) },
		func() error { return w.WriteVideoMode(VideoBox) },
		func() error { return w.WriteAudio(pcm) },
		func() error { return w.WritePads(pads) },
		func() error { return w.WritePause(true) },
		func() error { return w.WritePause(false) },
		func() error { return w.Write(TypeSaveState, []byte("/tmp/Zürich 日本.state")) },
		func() error {
			return w.WriteJSON(TypeStateSaved, StateResult{OK: false, Error: "disk full", Path: "/x"})
		},
		func() error { return w.Write(TypeQuit) },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}

	r := NewReader(&buf)
	next := func(want Type) []byte {
		t.Helper()
		typ, p, err := r.Next()
		if err != nil {
			t.Fatal(err)
		}
		if typ != want {
			t.Fatalf("got %v, want %v", typ, want)
		}
		return p
	}
	if got, err := DecodeReady(next(TypeReady)); err != nil || got != ready {
		t.Fatalf("ready %+v %v", got, err)
	}
	v, err := DecodeVideo(next(TypeVideo))
	if err != nil || v.Width != 3 || v.Height != 3 || v.Scale != 1 || v.Duration != 16683350*time.Nanosecond || !bytes.Equal(v.I420, frame) {
		t.Fatalf("video %+v %v", v, err)
	}
	if v, err := DecodeVideo(next(TypeVideo)); err != nil || len(v.I420) != 0 || v.Scale != 2 || v.Duration != time.Second/60 {
		t.Fatalf("repeat %+v %v", v, err)
	}
	if m, err := DecodeVideoMode(next(TypeVideoMode)); err != nil || m != VideoBox {
		t.Fatalf("video mode %v %v", m, err)
	}
	if got, err := DecodeAudio(nil, next(TypeAudio)); err != nil || len(got) != len(pcm) || got[2] != 32767 || got[3] != -32768 || got[1] != -1 {
		t.Fatalf("audio %v %v", got, err)
	}
	if got, err := DecodePads(next(TypePads)); err != nil || got != pads {
		t.Fatalf("pads %+v %v", got, err)
	}
	if p, err := DecodePause(next(TypePause)); err != nil || !p {
		t.Fatalf("pause %v %v", p, err)
	}
	if p, err := DecodePause(next(TypePause)); err != nil || p {
		t.Fatalf("resume %v %v", p, err)
	}
	if p := string(next(TypeSaveState)); p != "/tmp/Zürich 日本.state" {
		t.Fatalf("path %q", p)
	}
	if res, err := DecodeStateResult(next(TypeStateSaved)); err != nil || res.OK || res.Error != "disk full" || res.Path != "/x" {
		t.Fatalf("state %+v %v", res, err)
	}
	if p := next(TypeQuit); len(p) != 0 {
		t.Fatalf("quit payload %v", p)
	}
	if _, _, err := r.Next(); err != io.EOF {
		t.Fatalf("end: %v", err)
	}
}

func TestBrokenStreams(t *testing.T) {
	// Cut inside a message.
	var buf bytes.Buffer
	w := NewWriter(&buf)
	_ = w.Write(TypeLog, []byte("hello"))
	_ = w.Flush()
	cut := buf.Bytes()[:buf.Len()-2]
	if _, _, err := NewReader(bytes.NewReader(cut)).Next(); err != io.ErrUnexpectedEOF {
		t.Fatalf("cut: %v", err)
	}
	// Absurd length.
	huge := []byte{byte(TypeVideo), 0xff, 0xff, 0xff, 0xff}
	if _, _, err := NewReader(bytes.NewReader(huge)).Next(); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("huge: %v", err)
	}
	// Payloads with the wrong size.
	if _, err := DecodeVideo([]byte{0, 2, 0, 2, 0, 0, 0, 0, 0, 0, 0, 1, 1, 9}); err == nil {
		t.Fatal("a frame with the wrong size was accepted")
	}
	if _, err := DecodeVideo([]byte{0, 2, 0, 2, 0, 0, 0, 0, 0, 0, 0, 1, 3}); err == nil {
		t.Fatal("scale 3 accepted")
	}
	if _, err := DecodeVideoMode([]byte{7}); err == nil {
		t.Fatal("unknown video mode accepted")
	}
	if _, err := DecodePads(make([]byte, 31)); err == nil {
		t.Fatal("short pads accepted")
	}
	if _, err := DecodeAudio(nil, make([]byte, 6)); err == nil {
		t.Fatal("half a stereo sample accepted")
	}
	if _, err := DecodePause(nil); err == nil {
		t.Fatal("empty pause accepted")
	}
}

func TestVideoModes(t *testing.T) {
	for _, m := range []VideoMode{VideoNative, VideoBox, VideoDouble} {
		got, err := ParseVideoMode(m.String())
		if err != nil || got != m {
			t.Fatalf("%v: %v %v", m, got, err)
		}
	}
	if VideoDouble.Scale() != 2 || VideoBox.Scale() != 1 || VideoNative.Scale() != 1 {
		t.Fatal("scales")
	}
	if _, err := ParseVideoMode("triple"); err == nil {
		t.Fatal("unknown mode parsed")
	}
}
