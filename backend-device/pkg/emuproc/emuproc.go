// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package emuproc is the protocol between the device and an emulator
// worker: a child process that runs one game and talks over its stdin
// and stdout. Running each game in its own process lets one device run
// several games at once, since a libretro core can only be loaded once
// per process.
//
// Every message is framed the same way:
//
//	byte  0     message type
//	bytes 1-4   payload length (uint32, big endian)
//	bytes 5-    payload
//
// Worker to parent: Ready, Video, Audio, StateSaved, StateLoaded, Error
// and Log. Parent to worker: Pads, Pause, SaveState, LoadState, Quit and
// VideoMode.
// The payload of each type is described next to its constant.
package emuproc

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/pkg/input"
)

// Type is the first byte of a message.
type Type byte

// Worker to parent messages.
const (
	// TypeReady: JSON Ready, sent once the game is loaded.
	TypeReady Type = 0x01
	// TypeVideo: width uint16, height uint16, duration in nanoseconds
	// uint64 (all big endian), the scale byte (1, or 2 when the frame is
	// the game's picture enlarged 2x, see VideoMode), then the packed I420
	// frame. Width and height are the frame's (the enlarged size). An
	// empty frame means "show the previous frame again".
	TypeVideo Type = 0x02
	// TypeAudio: interleaved stereo int16 samples, little endian, 48 kHz.
	TypeAudio Type = 0x03
	// TypeStateSaved: JSON StateResult, the answer to SaveState.
	TypeStateSaved Type = 0x04
	// TypeStateLoaded: JSON StateResult, the answer to LoadState.
	TypeStateLoaded Type = 0x05
	// TypeError: UTF-8 text of a fatal error, sent before the worker exits.
	TypeError Type = 0x06
	// TypeLog: UTF-8 text worth showing in the parent's log.
	TypeLog Type = 0x07
)

// Parent to worker messages.
const (
	// TypePads: 4 pads (ports 1-4), each buttons uint32 big endian and
	// 4 axes int8 (LX, LY, RX, RY): 32 bytes.
	TypePads Type = 0x81
	// TypePause: 1 byte, 1 holds the game and 0 resumes it.
	TypePause Type = 0x82
	// TypeSaveState: UTF-8 path of the file to write the state to.
	TypeSaveState Type = 0x83
	// TypeLoadState: UTF-8 path of the file to read the state from.
	TypeLoadState Type = 0x84
	// TypeQuit: no payload; the worker closes the game and exits.
	TypeQuit Type = 0x85
	// TypeVideoMode: 1 byte, a VideoMode for the next frames.
	TypeVideoMode Type = 0x86
)

// VideoMode is how the worker turns the core's frames into I420.
type VideoMode byte

const (
	// VideoNative is the game's size, each chroma sample from the
	// top-left pixel of its 2x2 block.
	VideoNative VideoMode = 0
	// VideoBox is the game's size, each chroma sample the average of its
	// 2x2 block (the "saver" video quality).
	VideoBox VideoMode = 1
	// VideoDouble enlarges the picture 2x with nearest neighbour, so every
	// game pixel gets its own color sample (the "high" and "normal"
	// qualities).
	VideoDouble VideoMode = 2
)

// Scale is how many times the mode enlarges the game's picture.
func (m VideoMode) Scale() int {
	if m == VideoDouble {
		return 2
	}
	return 1
}

// String names the mode, as the emulate worker's --video flag takes it.
func (m VideoMode) String() string {
	switch m {
	case VideoBox:
		return "box"
	case VideoDouble:
		return "double"
	}
	return "native"
}

// ParseVideoMode reads a mode named by String.
func ParseVideoMode(s string) (VideoMode, error) {
	for _, m := range []VideoMode{VideoNative, VideoBox, VideoDouble} {
		if m.String() == s {
			return m, nil
		}
	}
	return VideoNative, fmt.Errorf("emuproc: unknown video mode %q", s)
}

// DecodeVideoMode parses a VideoMode payload.
func DecodeVideoMode(p []byte) (VideoMode, error) {
	if len(p) != 1 || p[0] > byte(VideoDouble) {
		return VideoNative, errors.New("emuproc: video mode message must be 1 known byte")
	}
	return VideoMode(p[0]), nil
}

// HeaderSize is the length of the type and length prefix.
const HeaderSize = 5

// MaxPayload bounds one message; a larger length means a broken stream.
const MaxPayload = 64 << 20

// Ports is the number of pads in a Pads message.
const Ports = 4

// padSize is the wire size of one pad.
const padSize = 8

// videoHeader is the wire size of the fields before the I420 bytes.
const videoHeader = 13

// ErrTooLarge is returned for a message longer than MaxPayload.
var ErrTooLarge = errors.New("emuproc: message too large")

// Ready describes the loaded game (libretro's AV info).
type Ready struct {
	Core        string  `json:"core,omitempty"`
	CoreVersion string  `json:"core_version,omitempty"`
	BaseWidth   int     `json:"base_width"`
	BaseHeight  int     `json:"base_height"`
	MaxWidth    int     `json:"max_width"`
	MaxHeight   int     `json:"max_height"`
	AspectRatio float64 `json:"aspect_ratio"`
	FPS         float64 `json:"fps"`
	SampleRate  float64 `json:"sample_rate"` // the core's; audio is sent at 48 kHz
	// SavesIncomplete: the emulator does not save this game whole (see
	// GameCore.SavesComplete), so the worker never loads a save for it.
	SavesIncomplete bool `json:"saves_incomplete,omitempty"`
}

// StateResult answers SaveState and LoadState.
type StateResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Path  string `json:"path,omitempty"` // the path of the request
}

// Video is one decoded Video message.
type Video struct {
	Width, Height int // the frame's size (already enlarged when Scale is 2)
	Scale         int // 1, or 2 for a picture enlarged 2x
	Duration      time.Duration
	I420          []byte // empty: repeat the previous frame
}

// Writer frames messages onto a stream. It buffers: call Flush after each
// frame or command. It is not safe for concurrent use.
type Writer struct {
	w   *bufio.Writer
	hdr [videoHeader]byte
	buf []byte
}

// NewWriter writes messages to w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: bufio.NewWriterSize(w, 256<<10)}
}

// Write sends one message made of the concatenated parts.
func (w *Writer) Write(t Type, parts ...[]byte) error {
	n := 0
	for _, p := range parts {
		n += len(p)
	}
	if n > MaxPayload {
		return ErrTooLarge
	}
	var h [HeaderSize]byte
	h[0] = byte(t)
	binary.BigEndian.PutUint32(h[1:], uint32(n))
	if _, err := w.w.Write(h[:]); err != nil {
		return err
	}
	for _, p := range parts {
		if _, err := w.w.Write(p); err != nil {
			return err
		}
	}
	return nil
}

// Flush sends what is buffered.
func (w *Writer) Flush() error { return w.w.Flush() }

// WriteJSON sends a message with a JSON payload.
func (w *Writer) WriteJSON(t Type, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return w.Write(t, b)
}

// WriteVideo sends one frame of width x height, the game's picture
// enlarged scale times (1 or 2); a nil i420 repeats the previous one.
func (w *Writer) WriteVideo(width, height, scale int, dur time.Duration, i420 []byte) error {
	binary.BigEndian.PutUint16(w.hdr[0:2], uint16(width))
	binary.BigEndian.PutUint16(w.hdr[2:4], uint16(height))
	binary.BigEndian.PutUint64(w.hdr[4:12], uint64(dur))
	w.hdr[12] = byte(max(scale, 1))
	return w.Write(TypeVideo, w.hdr[:], i420)
}

// WriteAudio sends interleaved stereo samples.
func (w *Writer) WriteAudio(pcm []int16) error {
	if cap(w.buf) < len(pcm)*2 {
		w.buf = make([]byte, len(pcm)*2)
	}
	b := w.buf[:len(pcm)*2]
	for i, s := range pcm {
		binary.LittleEndian.PutUint16(b[i*2:], uint16(s))
	}
	return w.Write(TypeAudio, b)
}

// WritePads sends the controllers of ports 1-4.
func (w *Writer) WritePads(pads [Ports]input.Pad) error {
	return w.Write(TypePads, EncodePads(pads))
}

// WriteVideoMode switches how the next frames are converted.
func (w *Writer) WriteVideoMode(m VideoMode) error {
	return w.Write(TypeVideoMode, []byte{byte(m)})
}

// WritePause holds (true) or resumes (false) the game.
func (w *Writer) WritePause(paused bool) error {
	var b byte
	if paused {
		b = 1
	}
	return w.Write(TypePause, []byte{b})
}

// Reader reads framed messages from a stream. It is not safe for
// concurrent use.
type Reader struct {
	r   *bufio.Reader
	buf []byte
}

// NewReader reads messages from r.
func NewReader(r io.Reader) *Reader {
	return &Reader{r: bufio.NewReaderSize(r, 256<<10)}
}

// Next returns the next message. The payload is only valid until the next
// call. At the end of the stream it returns io.EOF, and
// io.ErrUnexpectedEOF when the stream ends inside a message.
func (r *Reader) Next() (Type, []byte, error) {
	var h [HeaderSize]byte
	if _, err := io.ReadFull(r.r, h[:1]); err != nil {
		return 0, nil, err
	}
	if _, err := io.ReadFull(r.r, h[1:]); err != nil {
		return 0, nil, unexpected(err)
	}
	n := binary.BigEndian.Uint32(h[1:])
	if n > MaxPayload {
		return 0, nil, ErrTooLarge
	}
	if cap(r.buf) < int(n) {
		r.buf = make([]byte, n)
	}
	p := r.buf[:n]
	if _, err := io.ReadFull(r.r, p); err != nil {
		return 0, nil, unexpected(err)
	}
	return Type(h[0]), p, nil
}

func unexpected(err error) error {
	if err == io.EOF {
		return io.ErrUnexpectedEOF
	}
	return err
}

// DecodeVideo parses a Video payload. I420 aliases p.
func DecodeVideo(p []byte) (Video, error) {
	if len(p) < videoHeader {
		return Video{}, errors.New("emuproc: short video message")
	}
	v := Video{
		Width:    int(binary.BigEndian.Uint16(p[0:2])),
		Height:   int(binary.BigEndian.Uint16(p[2:4])),
		Duration: time.Duration(binary.BigEndian.Uint64(p[4:12])),
		Scale:    int(p[12]),
		I420:     p[videoHeader:],
	}
	if v.Scale != 1 && v.Scale != 2 {
		return Video{}, fmt.Errorf("emuproc: video scale %d", v.Scale)
	}
	if len(v.I420) > 0 && len(v.I420) != FrameSizeI420(v.Width, v.Height) {
		return Video{}, fmt.Errorf("emuproc: %dx%d frame with %d bytes", v.Width, v.Height, len(v.I420))
	}
	return v, nil
}

// FrameSizeI420 returns the byte length of a packed I420 frame.
func FrameSizeI420(w, h int) int {
	cw, ch := (w+1)/2, (h+1)/2
	return w*h + 2*cw*ch
}

// DecodeAudio parses an Audio payload, appending the samples to dst.
func DecodeAudio(dst []int16, p []byte) ([]int16, error) {
	if len(p)%4 != 0 {
		return dst, errors.New("emuproc: audio is not whole stereo samples")
	}
	for i := 0; i+1 < len(p); i += 2 {
		dst = append(dst, int16(binary.LittleEndian.Uint16(p[i:])))
	}
	return dst, nil
}

// EncodePads builds a Pads payload.
func EncodePads(pads [Ports]input.Pad) []byte {
	b := make([]byte, Ports*padSize)
	for i, pad := range pads {
		o := i * padSize
		binary.BigEndian.PutUint32(b[o:], uint32(pad.Buttons))
		for j, a := range pad.Axes {
			b[o+4+j] = byte(a)
		}
	}
	return b
}

// DecodePads parses a Pads payload.
func DecodePads(p []byte) ([Ports]input.Pad, error) {
	var pads [Ports]input.Pad
	if len(p) != Ports*padSize {
		return pads, errors.New("emuproc: pads message must be 32 bytes")
	}
	for i := range pads {
		o := i * padSize
		pads[i].Buttons = input.State(binary.BigEndian.Uint32(p[o:]))
		for j := range pads[i].Axes {
			pads[i].Axes[j] = int8(p[o+4+j])
		}
	}
	return pads, nil
}

// DecodePause parses a Pause payload.
func DecodePause(p []byte) (bool, error) {
	if len(p) != 1 {
		return false, errors.New("emuproc: pause message must be 1 byte")
	}
	return p[0] != 0, nil
}

// ProbeResult is what "emulate --probe" prints on stdout, as one JSON line:
// how many sound samples the game made and how often its picture changed
// in the frames after the save point (or after loading the save), and
// whether every CPU is in the save.
type ProbeResult struct {
	Sound    int      `json:"sound"`
	Changes  int      `json:"changes"` // frames whose picture differed from the one before
	Complete bool     `json:"complete"`
	Modules  []string `json:"modules,omitempty"` // what the save holds
}

// DecodeReady parses a Ready payload.
func DecodeReady(p []byte) (Ready, error) {
	var r Ready
	err := json.Unmarshal(p, &r)
	return r, err
}

// DecodeStateResult parses a StateSaved or StateLoaded payload.
func DecodeStateResult(p []byte) (StateResult, error) {
	var r StateResult
	err := json.Unmarshal(p, &r)
	return r, err
}

// String names a message type for logs.
func (t Type) String() string {
	switch t {
	case TypeReady:
		return "ready"
	case TypeVideo:
		return "video"
	case TypeAudio:
		return "audio"
	case TypeStateSaved:
		return "state_saved"
	case TypeStateLoaded:
		return "state_loaded"
	case TypeError:
		return "error"
	case TypeLog:
		return "log"
	case TypePads:
		return "pads"
	case TypePause:
		return "pause"
	case TypeSaveState:
		return "save_state"
	case TypeLoadState:
		return "load_state"
	case TypeQuit:
		return "quit"
	case TypeVideoMode:
		return "video_mode"
	}
	return fmt.Sprintf("type(0x%02x)", byte(t))
}

// RomTestReport is what "romtest --child" prints on stdout, as one JSON
// line: what one power-on run of a set did (see services.RunRomTest).
type RomTestReport struct {
	Loaded      bool    `json:"loaded"`
	Error       string  `json:"error,omitempty"` // why the core did not load it
	Core        string  `json:"core,omitempty"`
	CoreVersion string  `json:"core_version,omitempty"`
	Width       int     `json:"width,omitempty"`
	Height      int     `json:"height,omitempty"`
	FPS         float64 `json:"fps,omitempty"`
	Frames      int     `json:"frames"`  // frames run
	Seconds     float64 `json:"seconds"` // wall time of those frames, unpaced
	// FirstPicture is the first frame that is not black, or -1.
	FirstPicture int `json:"first_picture"`
	// Hashes has one hash of the picture per frame, to compare runs.
	Hashes []uint32 `json:"hashes,omitempty"`
	// Audio: how many times the core sent sound, the samples it sent, the
	// samples that were not silence and the first frame with sound (-1).
	AudioCalls   int `json:"audio_calls"`
	AudioSamples int `json:"audio_samples"`
	Sound        int `json:"sound"`
	FirstSound   int `json:"first_sound"`
	// What the core's ROM loader said about the set's files.
	NotFound      []string `json:"not_found,omitempty"`
	BadLength     []string `json:"bad_length,omitempty"`
	WrongChecksum []string `json:"wrong_checksum,omitempty"`
	// Shot is a small PNG (base64) of the last frame.
	Shot string `json:"shot,omitempty"`
}
