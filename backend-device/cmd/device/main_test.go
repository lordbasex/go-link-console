// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"testing"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
)

func TestResolveSignalURL(t *testing.T) {
	cases := []struct {
		name   string
		flag   string
		stored string
		want   string
		ok     bool
	}{
		{"hardcoded default", "", "", models.DefaultSignalURL, true},
		{"device.json", "", "wss://mine.example/ws", "wss://mine.example/ws", true},
		{"flag wins", "wss://flag.example/ws", "wss://mine.example/ws", "wss://flag.example/ws", true},
		{"local dev", "ws://127.0.0.1:8090/ws", "", "ws://127.0.0.1:8090/ws", true},
		{"bad flag", "https://x.example/ws", "", "", false},
		{"bad stored", "", "signal.example", "", false},
	}
	for _, c := range cases {
		got, err := resolveSignalURL(c.flag, models.Config{SignalURL: c.stored})
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("%s: got %q, %v", c.name, got, err)
		}
	}
}

func TestIsLoopback(t *testing.T) {
	cases := map[string]bool{
		"ws://127.0.0.1:8090/ws":    true,
		"ws://localhost:8090/ws":    true,
		"ws://[::1]:8090/ws":        true,
		"ws://192.168.1.10:8090/ws": false,
		"wss://signal.example/ws":   false,
	}
	for in, want := range cases {
		if got := isLoopback(in); got != want {
			t.Errorf("isLoopback(%q) = %v", in, got)
		}
	}
}

func TestRoomPictureAction(t *testing.T) {
	if p, ok := roomPicture("", ""); !ok || p != nil {
		t.Fatal("both empty clear the room's default")
	}
	if p, ok := roomPicture("crt", "ambient"); !ok || p == nil || p.Style != "crt" || p.Bands != "ambient" {
		t.Fatalf("crt/ambient: %+v %v", p, ok)
	}
	for _, c := range [][2]string{{"crt", ""}, {"", "black"}, {"blur", "black"}, {"sharp", "neon"}} {
		if _, ok := roomPicture(c[0], c[1]); ok {
			t.Errorf("%v accepted", c)
		}
	}
}
