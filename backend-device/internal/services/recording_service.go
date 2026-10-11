// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

// RecordingInfo describes one finished recording. It is kept next to the
// file (<file>.json) and copied into the history of games.
type RecordingInfo struct {
	// ID is "<room id>/<file name>": what the owner's browser asks for.
	ID         string    `json:"id"`
	RoomID     string    `json:"room_id"` // the saved room's stable id
	Room       string    `json:"room"`    // the room's name when it was recorded
	File       string    `json:"file"`    // file name, e.g. my-room-2026-09-28-2130.webm
	StartedAt  time.Time `json:"started_at"`
	DurationMS int64     `json:"duration_ms"`
	Size       int64     `json:"size"`
	SHA256     string    `json:"sha256"` // hex, to check a download
	// Tracks that got data: video, game, voice-p1..voice-p4.
	Tracks []string `json:"tracks"`
	// Reason it stopped: stopped, paused, limit_time, limit_size,
	// room_stopped, device_stopped or interrupted.
	Reason string `json:"reason"`
}

// Recording errors.
var (
	ErrUnknownRecording = errors.New("unknown recording")
	ErrRecording        = errors.New("the room is already being recorded")
	ErrNotRecording     = errors.New("the room is not being recorded")
	ErrRecordPaused     = errors.New("the game is paused: resume it to record")
)

// RecordingService keeps the recordings of game rooms under one folder
// (~/go-link-console/rec), one subfolder per room. Only the host sees them: they
// are listed and downloaded by the host's linked browsers and the CLI.
type RecordingService struct {
	dir string
	now func() time.Time

	mu     sync.Mutex
	active map[string]bool // files being written, by path
}

// NewRecordingService opens the recordings folder. A recording the device
// never closed (it crashed or lost power) is kept as "interrupted": its
// file plays, only without the seek index.
func NewRecordingService(dir string, now func() time.Time) *RecordingService {
	if now == nil {
		now = time.Now
	}
	s := &RecordingService{dir: dir, now: now, active: map[string]bool{}}
	s.recover()
	return s
}

// Dir is the recordings folder.
func (s *RecordingService) Dir() string { return s.dir }

var (
	recFileRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,80}\.webm$`)
	slugRe    = regexp.MustCompile(`[^a-z0-9]+`)
)

// validRecordingID accepts only "<room id>/<file>.webm": never a path
// that leaves the folder.
func validRecordingID(id string) bool {
	room, file, ok := strings.Cut(id, "/")
	return ok && validRoomID(room) && recFileRe.MatchString(file)
}

// slug turns a room name into a file name part.
func slug(name string) string {
	s := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(s) > 40 {
		s = strings.Trim(s[:40], "-")
	}
	if s == "" {
		s = "game"
	}
	return s
}

// Start begins a recording of a room. onLimit runs when the recording
// stops by itself (2 hours, 2 GB or a disk error).
func (s *RecordingService) Start(roomID, roomName string, onLimit func(reason string)) (*Recorder, error) {
	if !validRoomID(roomID) {
		return nil, ErrUnknownRoom
	}
	dir := filepath.Join(s.dir, roomID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	base := slug(roomName) + "-" + s.now().Format("2006-01-02-1504")
	path := filepath.Join(dir, base+".webm")
	for n := 2; ; n++ {
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			break
		}
		path = filepath.Join(dir, fmt.Sprintf("%s-%d.webm", base, n))
	}
	rec, err := NewRecorder(RecorderConfig{Path: path, OnLimit: onLimit, Now: s.now})
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.active[path] = true
	s.mu.Unlock()
	return rec, nil
}

// Finish stops a recording, checksums the file and saves its description.
// A recording without a single picture leaves nothing and returns
// ErrEmptyRecording.
func (s *RecordingService) Finish(rec *Recorder, roomID, roomName, reason string) (RecordingInfo, error) {
	rec.Stop(reason)
	defer func() {
		s.mu.Lock()
		delete(s.active, rec.Path())
		s.mu.Unlock()
	}()
	dur, size, tracks, why, err := rec.Result()
	if errors.Is(err, ErrEmptyRecording) {
		return RecordingInfo{}, err
	}
	info := RecordingInfo{
		ID: roomID + "/" + filepath.Base(rec.Path()), RoomID: roomID, Room: roomName, File: filepath.Base(rec.Path()),
		StartedAt: rec.StartedAt(), DurationMS: dur.Milliseconds(), Size: size, Tracks: tracks, Reason: why,
	}
	if info.Tracks == nil {
		info.Tracks = []string{}
	}
	sum, err2 := fileSHA256(rec.Path())
	if err2 != nil {
		return info, errors.Join(err, err2)
	}
	info.SHA256 = sum
	if err2 := writeRecInfo(rec.Path(), info); err2 != nil {
		return info, errors.Join(err, err2)
	}
	return info, err
}

func writeRecInfo(path string, info RecordingInfo) error {
	b, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return writePrivate(path+".json", b)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// recover describes the recordings left without a description.
func (s *RecordingService) recover() {
	for _, path := range s.files() {
		if _, err := os.Stat(path + ".json"); err == nil {
			continue
		}
		fi, err := os.Stat(path)
		if err != nil {
			continue
		}
		if fi.Size() == 0 {
			_ = os.Remove(path)
			continue
		}
		roomID := filepath.Base(filepath.Dir(path))
		info := RecordingInfo{
			ID: roomID + "/" + filepath.Base(path), RoomID: roomID, File: filepath.Base(path),
			StartedAt: fi.ModTime(), Size: fi.Size(), Tracks: []string{}, Reason: RecInterrupted,
		}
		if sum, err := fileSHA256(path); err == nil {
			info.SHA256 = sum
		}
		_ = writeRecInfo(path, info)
	}
}

// files lists the .webm files of the folder.
func (s *RecordingService) files() []string {
	matches, _ := filepath.Glob(filepath.Join(s.dir, "*", "*.webm"))
	return matches
}

// List returns every finished recording, newest first.
func (s *RecordingService) List() []RecordingInfo {
	out := []RecordingInfo{}
	for _, path := range s.files() {
		if info, err := readRecInfo(path); err == nil {
			out = append(out, info)
		}
	}
	slices.SortFunc(out, func(a, b RecordingInfo) int { return b.StartedAt.Compare(a.StartedAt) })
	return out
}

// Bytes is the space the recordings take.
func (s *RecordingService) Bytes() int64 { return dirSize(s.dir) }

func readRecInfo(path string) (RecordingInfo, error) {
	b, err := os.ReadFile(path + ".json")
	if err != nil {
		return RecordingInfo{}, err
	}
	var info RecordingInfo
	if err := json.Unmarshal(b, &info); err != nil {
		return RecordingInfo{}, err
	}
	return info, nil
}

// path returns the file of a finished recording.
func (s *RecordingService) path(id string) (string, error) {
	if !validRecordingID(id) {
		return "", ErrUnknownRecording
	}
	path := filepath.Join(s.dir, filepath.FromSlash(id))
	s.mu.Lock()
	busy := s.active[path]
	s.mu.Unlock()
	if busy {
		return "", ErrUnknownRecording // still being written
	}
	if _, err := os.Stat(path + ".json"); err != nil {
		return "", ErrUnknownRecording
	}
	return path, nil
}

// Get describes one finished recording.
func (s *RecordingService) Get(id string) (RecordingInfo, error) {
	path, err := s.path(id)
	if err != nil {
		return RecordingInfo{}, err
	}
	return readRecInfo(path)
}

// Open opens a finished recording to read it (downloads).
func (s *RecordingService) Open(id string) (*os.File, RecordingInfo, error) {
	path, err := s.path(id)
	if err != nil {
		return nil, RecordingInfo{}, err
	}
	info, err := readRecInfo(path)
	if err != nil {
		return nil, RecordingInfo{}, err
	}
	f, err := os.Open(path)
	return f, info, err
}

// Delete removes a finished recording for good.
func (s *RecordingService) Delete(id string) error {
	path, err := s.path(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	_ = os.Remove(path + ".json")
	_ = os.Remove(filepath.Dir(path)) // the room's folder, when it is empty
	return nil
}

// DeleteAll removes every finished recording (the ones being written go
// on). It returns how many it removed.
func (s *RecordingService) DeleteAll() int {
	n := 0
	for _, info := range s.List() {
		if s.Delete(info.ID) == nil {
			n++
		}
	}
	return n
}
