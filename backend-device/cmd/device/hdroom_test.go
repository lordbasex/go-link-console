// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"errors"
	"testing"
)

func TestHDRoomVideo(t *testing.T) {
	found := func() (string, error) { return "/opt/homebrew/bin/ffmpeg", nil }
	missing := func() (string, error) { return "", errors.New("ffmpeg not found") }
	noCard := func() string { return "" }
	hw := func(kind string) func() string { return func() string { return kind } }
	for _, c := range []struct {
		codec, h264, goos string
		kbps, scale       int
		ffmpeg            func() (string, error)
		mf                func() string
		want              string
		wantKbps          int
		fails             bool
	}{
		{"auto", "x264", "darwin", 0, 3, found, hw("videotoolbox"), "videotoolbox", 8000, false},
		{"auto", "x264", "darwin", 0, 2, missing, hw("videotoolbox"), "videotoolbox", 4000, false}, // no ffmpeg needed
		{"h264", "videotoolbox", "linux", 0, 3, found, noCard, "", 0, true},
		{"auto", "x264", "linux", 0, 2, found, noCard, "", 4000, false},
		{"vp8", "x264", "darwin", 6000, 3, found, noCard, "", 6000, false},
		{"h264", "x264", "linux", 0, 3, found, noCard, "x264", 8000, false},
		{"h264", "videotoolbox", "darwin", 0, 3, missing, noCard, "videotoolbox", 8000, false},
		{"h264", "x264", "darwin", 0, 3, missing, noCard, "", 0, true},
		{"av1", "x264", "darwin", 0, 3, found, noCard, "", 0, true},
		{"auto", "x264", "windows", 0, 3, missing, hw("mediafoundation"), "mediafoundation", 8000, false}, // the graphics card's, no ffmpeg needed
		{"auto", "x264", "windows", 0, 2, found, noCard, "", 4000, false},                                 // no hardware encoder: VP8
		{"h264", "mediafoundation", "windows", 0, 3, missing, noCard, "mediafoundation", 8000, false},
		{"h264", "mediafoundation", "linux", 0, 3, found, noCard, "", 0, true},
		{"auto", "x264", "linux", 0, 3, found, hw("vaapi"), "vaapi", 8000, false}, // an Intel or AMD card that works
		{"h264", "nvenc", "linux", 0, 3, found, noCard, "nvenc", 8000, false},
		{"h264", "nvenc", "linux", 0, 3, missing, noCard, "", 0, true}, // through ffmpeg
		{"h264", "vaapi", "windows", 0, 3, found, noCard, "", 0, true},
	} {
		got, kbps, err := hdRoomVideo(c.codec, c.h264, c.kbps, c.scale, c.goos, c.ffmpeg, c.mf)
		if (err != nil) != c.fails || got != c.want || kbps != c.wantKbps {
			t.Errorf("%+v: got %q %d %v", c, got, kbps, err)
		}
	}
}
