// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package golinkhd hosts go-link HD's engine: it loads its library
// (libgolinkhd.dylib, .so or golinkhd.dll, built from the golink-hd
// repository) and runs it through the engine's own API (golink_hd.h, a copy
// of the repository's include/golink_hd.h): load a game package, run one
// frame with the players' controllers, take its picture and sound, save and
// load its state. MAME runs through package libretro instead.
//
// This version of the engine runs one engine per process (the device runs
// each game in its own worker process).
package golinkhd

/*
#cgo linux LDFLAGS: -ldl
#include <stdlib.h>
#include "glue.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"
)

// APIVersion is the engine API this host was written for.
const APIVersion = C.GOLINKHD_API_VERSION

// KeepMax is the most a game keeps between sessions (API 3).
const KeepMax = C.GOLINKHD_KEEP_MAX

// MaxPlayers is the most players a game can take.
const MaxPlayers = C.GOLINKHD_MAX_PLAYERS

// Buttons of a pad (golink_hd.h).
const (
	Up     = uint32(C.GOLINKHD_UP)
	Down   = uint32(C.GOLINKHD_DOWN)
	Left   = uint32(C.GOLINKHD_LEFT)
	Right  = uint32(C.GOLINKHD_RIGHT)
	Jump   = uint32(C.GOLINKHD_JUMP)
	Run    = uint32(C.GOLINKHD_RUN)
	Start  = uint32(C.GOLINKHD_START)
	Select = uint32(C.GOLINKHD_SELECT)
	L      = uint32(C.GOLINKHD_L)
	R      = uint32(C.GOLINKHD_R)
	L2     = uint32(C.GOLINKHD_L2)
	R2     = uint32(C.GOLINKHD_R2)
	L3     = uint32(C.GOLINKHD_L3)
	R3     = uint32(C.GOLINKHD_R3)
	A      = uint32(C.GOLINKHD_A)
	B      = uint32(C.GOLINKHD_B)
	X      = uint32(C.GOLINKHD_X)
	Y      = uint32(C.GOLINKHD_Y)
)

// Pad is one player's controller for one frame; sticks are -32768..32767.
type Pad struct {
	Buttons        uint32
	LX, LY, RX, RY int32
}

// Info describes the loaded game.
type Info struct {
	Title         string
	Width, Height int
	FPS           int
	SampleRate    int
	Players       int
	SHA256        [32]byte // zeros for a built-in demo
	EngineVersion string
}

// Frame is one frame's output. Pixels (0x00RRGGBB, rows of Pitch pixels)
// and Audio (interleaved stereo) point into the engine and are only valid
// until the next call on it.
type Frame struct {
	Pixels               []uint32
	Width, Height, Pitch int
	Audio                []int16
}

// Engine is a loaded go-link HD engine.
type Engine struct {
	g    *C.ghd_t
	pads [MaxPlayers]C.golinkhd_pad
}

// Open loads the library and starts an engine (playing its built-in demo).
func Open(path string) (*Engine, error) {
	g := (*C.ghd_t)(C.calloc(1, C.size_t(unsafe.Sizeof(C.ghd_t{}))))
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	var errbuf [256]C.char
	if rc := C.ghd_open(g, cpath, &errbuf[0], C.size_t(len(errbuf))); rc != 0 {
		C.free(unsafe.Pointer(g))
		return nil, fmt.Errorf("go-link HD: cannot start %s: %s", path, C.GoString(&errbuf[0]))
	}
	return &Engine{g: g}, nil
}

// Load plays a game package (.glhd).
func (e *Engine) Load(pkg []byte) error {
	if len(pkg) == 0 {
		return errors.New("go-link HD: the game package is empty")
	}
	var why *C.char
	data := C.CBytes(pkg)
	defer C.free(data)
	if C.ghd_load(e.g, (*C.uint8_t)(data), C.size_t(len(pkg)), &why) == 0 {
		return fmt.Errorf("go-link HD: this game cannot be played: %s", C.GoString(why))
	}
	return nil
}

// LoadDemo plays a built-in demo: 0 the platformer, 1 the showcase.
func (e *Engine) LoadDemo(demo int) { C.ghd_load_demo(e.g, C.int32_t(demo)) }

// Info describes the loaded game.
func (e *Engine) Info() Info {
	var in C.golinkhd_info
	C.ghd_info(e.g, &in)
	info := Info{
		Title: C.GoString(in.title), Width: int(in.width), Height: int(in.height), FPS: int(in.fps),
		SampleRate: int(in.sample_rate), Players: int(in.players), EngineVersion: C.GoString(C.ghd_version(e.g)),
	}
	for i := range info.SHA256 {
		info.SHA256[i] = byte(in.sha256[i])
	}
	return info
}

// SetLanguage sets the language of the game's texts: "en", "es" or "pt".
func (e *Engine) SetLanguage(lang string) {
	c := C.CString(lang)
	defer C.free(unsafe.Pointer(c))
	C.ghd_set_language(e.g, c)
}

// SetMusic turns the music on or off.
func (e *Engine) SetMusic(on bool) {
	v := C.int(0)
	if on {
		v = 1
	}
	C.ghd_set_music(e.g, v)
}

// SetResolution asks for the picture's size of the next Load, in lines (360,
// 720 or 1080; 0 is the package's own): a package painted bigger is drawn
// at that size, never bigger than its pictures. False on an engine older
// than API 2, which keeps the package's own size.
func (e *Engine) SetResolution(lines int) bool {
	return C.ghd_set_resolution(e.g, C.int32_t(lines)) != 0
}

// Frame runs one frame with the players' pads (ports 1 to len(pads)).
func (e *Engine) Frame(pads []Pad) Frame {
	n := min(len(pads), MaxPlayers)
	for i := range e.pads {
		e.pads[i] = C.golinkhd_pad{}
		if i < n {
			p := pads[i]
			e.pads[i] = C.golinkhd_pad{buttons: C.uint32_t(p.Buttons), lx: C.int32_t(p.LX), ly: C.int32_t(p.LY), rx: C.int32_t(p.RX), ry: C.int32_t(p.RY)}
		}
	}
	var out C.golinkhd_frame_out
	C.ghd_frame(e.g, &e.pads[0], C.int32_t(MaxPlayers), &out)
	f := Frame{Width: int(out.width), Height: int(out.height), Pitch: int(out.pitch)}
	if out.pixels != nil {
		f.Pixels = unsafe.Slice((*uint32)(unsafe.Pointer(out.pixels)), f.Pitch*f.Height)
	}
	if out.audio != nil && out.audio_frames > 0 {
		f.Audio = unsafe.Slice((*int16)(unsafe.Pointer(out.audio)), int(out.audio_frames)*2)
	}
	return f
}

// Restart plays the game from its start.
func (e *Engine) Restart() { C.ghd_restart(e.g) }

// SaveState snapshots the game.
func (e *Engine) SaveState() ([]byte, error) {
	n := int(C.ghd_state_size(e.g))
	if n <= 0 {
		return nil, errors.New("go-link HD: no save state")
	}
	buf := make([]byte, n)
	if C.ghd_state_save(e.g, (*C.uint8_t)(unsafe.Pointer(&buf[0])), C.size_t(n)) == 0 {
		return nil, errors.New("go-link HD: the save state failed")
	}
	runtime.KeepAlive(buf)
	return buf, nil
}

// LoadState restores a snapshot made by SaveState.
func (e *Engine) LoadState(b []byte) error {
	if len(b) == 0 {
		return errors.New("go-link HD: the save state is empty")
	}
	var why *C.char
	if C.ghd_state_load(e.g, (*C.uint8_t)(unsafe.Pointer(&b[0])), C.size_t(len(b)), &why) == 0 {
		return fmt.Errorf("go-link HD: %s", C.GoString(why))
	}
	runtime.KeepAlive(b)
	return nil
}

// Keep returns what the game keeps between sessions (its record, the
// player's choices; at most KeepMax bytes), nil when it keeps nothing or the
// engine is older than API 3. The host stores it for the game and gives it
// back with SetKept after every Load.
func (e *Engine) Keep() []byte {
	buf := make([]byte, KeepMax)
	n := int(C.ghd_keep_save(e.g, (*C.uint8_t)(unsafe.Pointer(&buf[0])), C.size_t(len(buf))))
	if n <= 0 || n > len(buf) {
		return nil
	}
	return buf[:n]
}

// SetKept gives the game back what it kept (Keep) in an earlier session;
// false when it is not this game's or the engine is older than API 3.
func (e *Engine) SetKept(b []byte) bool {
	if len(b) == 0 || len(b) > KeepMax {
		return false
	}
	ok := C.ghd_keep_load(e.g, (*C.uint8_t)(unsafe.Pointer(&b[0])), C.size_t(len(b))) != 0
	runtime.KeepAlive(b)
	return ok
}

// Close stops the engine and unloads the library.
func (e *Engine) Close() {
	if e.g != nil {
		C.ghd_close(e.g)
		C.free(unsafe.Pointer(e.g))
		e.g = nil
	}
}
