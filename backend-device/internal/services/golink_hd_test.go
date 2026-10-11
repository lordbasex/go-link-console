// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/pkg/glhd"
	"github.com/lordbasex/go-link-console/backend-device/pkg/input"
)

// TestGoLinkHDCorePlaysItsDemo runs go-link HD's real engine library (built
// in its own repository, golink-hd) through its own API with no game, the
// way the test room does with --hd-core: GOLINK_HD_LIB=/path/libgolinkhd.dylib.
func TestGoLinkHDCorePlaysItsDemo(t *testing.T) {
	src := os.Getenv("GOLINK_HD_LIB")
	if src == "" {
		t.Skip("GOLINK_HD_LIB is not set")
	}
	// a copy, so the checksum file next to it stays in the test's folder
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	core := filepath.Join(dir, filepath.Base(src))
	if err := os.WriteFile(core, data, 0o755); err != nil {
		t.Fatal(err)
	}
	var w, h, frames, samples int
	var last [32]byte
	game, err := OpenGameCore(GameCoreConfig{
		CorePath:  core,
		SystemDir: filepath.Join(dir, "system"),
		Native:    true,
		Upscale:   3,
		Video: func(i420 []byte, fw, fh int, dur time.Duration) {
			w, h = fw, fh
			frames++
			last = sha256.Sum256(i420)
		},
		Audio: func(pcm []int16) { samples += len(pcm) / 2 },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer game.Close()
	if name := game.Info().Name; name != "go-link HD" {
		t.Fatalf("core %q", name)
	}
	if av := game.AV(); av.BaseWidth != 640 || av.BaseHeight != 360 || av.FPS != 60 || av.SampleRate != 48000 {
		t.Fatalf("av %+v", av)
	}
	press := func(f int) (pads [4]input.Pad) {
		switch {
		case f >= 10 && f < 14:
			pads[0].Buttons = input.State(input.Start)
		case f >= 14:
			pads[0].Buttons = input.State(input.Right | input.Button3)
			if f%50 < 18 {
				pads[0].Buttons |= input.State(input.Button1)
			}
		}
		return pads
	}
	for f := 0; f < 240; f++ {
		game.SetPads(press(f))
		game.Run()
	}
	if w != 1920 || h != 1080 || frames != 240 {
		t.Fatalf("got %d frames of %dx%d, want 240 of 1920x1080", frames, w, h)
	}
	if samples != 240*800 {
		t.Fatalf("got %d samples, want %d", samples, 240*800)
	}
	if !game.SavesComplete() {
		t.Fatal("go-link HD's save state is the whole game")
	}
	// a save state continues exactly where it was taken
	state, err := game.SaveState()
	if err != nil {
		t.Fatal(err)
	}
	for f := 240; f < 300; f++ {
		game.SetPads(press(f))
		game.Run()
	}
	want := last
	if err := game.LoadState(state); err != nil {
		t.Fatal(err)
	}
	for f := 240; f < 300; f++ {
		game.SetPads(press(f))
		game.Run()
	}
	if !bytes.Equal(last[:], want[:]) {
		t.Fatal("the frames after loading the save state differ")
	}
}

// The left stick reaches go-link HD's engine and moves the player like the D-pad.
func TestGoLinkHDCoreReadsTheStick(t *testing.T) {
	src := os.Getenv("GOLINK_HD_LIB")
	if src == "" {
		t.Skip("GOLINK_HD_LIB is not set")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	core := filepath.Join(dir, filepath.Base(src))
	if err := os.WriteFile(core, data, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(stick bool) [32]byte {
		var last [32]byte
		game, err := OpenGameCore(GameCoreConfig{
			CorePath: core, SystemDir: filepath.Join(dir, "system"), Native: true,
			Video: func(i420 []byte, w, h int, dur time.Duration) { last = sha256.Sum256(i420) },
		})
		if err != nil {
			t.Fatal(err)
		}
		defer game.Close()
		for f := 0; f < 200; f++ {
			var pads [4]input.Pad
			if f >= 10 && f < 14 {
				pads[0].Buttons = input.State(input.Start)
			} else if f >= 14 && stick {
				pads[0].Axes[0] = 127 // the left stick all the way right
			}
			game.SetPads(pads)
			game.Run()
		}
		return last
	}
	if run(true) == run(false) {
		t.Fatal("the stick made no difference: the engine does not get it")
	}
}

func TestGoLinkHDCorePlaysItsBuiltInGames(t *testing.T) {
	src := os.Getenv("GOLINK_HD_LIB")
	if src == "" {
		t.Skip("GOLINK_HD_LIB is not set")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	core := filepath.Join(dir, filepath.Base(src))
	if err := os.WriteFile(core, data, 0o755); err != nil {
		t.Fatal(err)
	}
	// each built-in game draws its own first picture
	seen := map[[32]byte]string{}
	for _, d := range glhd.Demos {
		var last [32]byte
		game, err := OpenGameCore(GameCoreConfig{
			CorePath:  core,
			RomPath:   glhd.DemoPath(d),
			SystemDir: filepath.Join(dir, "system"),
			Native:    true,
			Video:     func(i420 []byte, fw, fh int, dur time.Duration) { last = sha256.Sum256(i420) },
			Audio:     func(pcm []int16) {},
		})
		if err != nil {
			t.Fatalf("%s: %v", d.Name, err)
		}
		for range 30 {
			game.Run()
		}
		game.Close()
		if other, ok := seen[last]; ok {
			t.Fatalf("%s draws the same as %s", d.Name, other)
		}
		seen[last] = d.Name
	}
}

// A package made for 1080p in a 720p room is drawn by the engine at 720p
// (golinkhd_set_resolution), not drawn at 1080p and streamed bigger than the room.
func TestGoLinkHDDrawsAtTheRoomSize(t *testing.T) {
	src := os.Getenv("GOLINK_HD_LIB")
	if src == "" {
		t.Skip("GOLINK_HD_LIB is not set")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	core := filepath.Join(dir, filepath.Base(src))
	if err := os.WriteFile(core, data, 0o755); err != nil {
		t.Fatal(err)
	}
	rows := make([]string, 30)
	for y := range rows {
		c := "."
		if y >= 27 {
			c = "#"
		}
		rows[y] = strings.Repeat(c, 60)
	}
	level, _ := json.Marshal(map[string]any{"width": 60, "height": 30, "start": []int{2, 26}, "rows": rows})
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string][]byte{
		"manifest.json": []byte(`{"format": 3, "title": "Big", "level": "level.json", "resolution": "1080p"}`),
		"level.json":    level,
	} {
		f, _ := zw.Create(name)
		f.Write(body)
	}
	zw.Close()
	pkg := filepath.Join(dir, "big.glhd")
	if err := os.WriteFile(pkg, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	var w, h int
	game, err := OpenGameCore(GameCoreConfig{
		CorePath:  core,
		RomPath:   pkg,
		SystemDir: filepath.Join(dir, "system"),
		Native:    true,
		Upscale:   2,
		Video:     func(i420 []byte, fw, fh int, dur time.Duration) { w, h = fw, fh },
		Audio:     func(pcm []int16) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer game.Close()
	game.Run()
	if w != 1280 || h != 720 {
		t.Fatalf("frames of %dx%d, want 1280x720", w, h)
	}
}
