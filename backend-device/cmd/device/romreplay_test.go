// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/lordbasex/go-link-console/backend-device/pkg/input"
)

func TestParseCheckpoints(t *testing.T) {
	got, err := parseCheckpoints("300, 600,,900")
	if err != nil || !slices.Equal(got, []int{300, 600, 900}) {
		t.Fatalf("got %v %v", got, err)
	}
	for _, bad := range []string{"x", "0", "-5"} {
		if _, err := parseCheckpoints(bad); err == nil {
			t.Errorf("%q: want an error", bad)
		}
	}
}

func TestLoadReplayScript(t *testing.T) {
	file := filepath.Join(t.TempDir(), "s.json")
	js := `{"frames": 900, "checkpoints": [300], "steps": [
		{"from": 120, "to": 125, "buttons": ["coin"]},
		{"from": 200, "to": 210, "port": 2, "buttons": ["right", "b1"]},
		{"from": 300, "to": 301, "buttons": []}]}`
	if err := os.WriteFile(file, []byte(js), 0o600); err != nil {
		t.Fatal(err)
	}
	meta, sc, err := loadReplayScript(file)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Frames != 900 || !slices.Equal(meta.Checkpoints, []int{300}) {
		t.Fatalf("meta %+v", meta)
	}
	if p := sc.Pads(122); !p[0].Buttons.Pressed(input.Coin) {
		t.Error("coin on port 1 at frame 122")
	}
	if p := sc.Pads(205); !p[1].Buttons.Pressed(input.Right) || !p[1].Buttons.Pressed(input.Button1) || p[0].Buttons != 0 {
		t.Errorf("port 2 at frame 205: %v", p)
	}
	if p := sc.Pads(126); p[0].Buttons != 0 {
		t.Error("the range is inclusive and ends at 125")
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	_ = os.WriteFile(bad, []byte(`{"steps":[{"from":1,"to":2,"buttons":["fly"]}]}`), 0o600)
	if _, _, err := loadReplayScript(bad); err == nil {
		t.Error("an unknown button must fail")
	}
}
