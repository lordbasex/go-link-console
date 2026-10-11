// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package telemetry keeps what happens in each room while it runs, from
// its first start, in a SQLite database next to the rooms
// (~/go-link-console/telemetry.db): every run (when it was on), a sample per
// second of the room and of each participant (frames, gaps, latency,
// packet loss, input, voice), and a log of raw events. The charts in My
// device and the incident report are built from it, and `device telemetry
// export` writes it out for a closer look. Nothing is ever dropped by age;
// a room's data goes when the room is deleted for good.
//
// Writes never block the media path: they go through a buffered queue to
// one writer goroutine that commits them in batches; when the queue is
// full a write is dropped and counted.
package telemetry

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite" // pure Go SQLite: no cgo, builds everywhere
)

// Level of an event.
type Level string

const (
	Info  Level = "info"
	Warn  Level = "warn"
	Error Level = "error"
)

// Metrics is one sample: name to value.
type Metrics map[string]float64

const schema = `
CREATE TABLE IF NOT EXISTS runs (
	room    TEXT NOT NULL,
	id      TEXT NOT NULL DEFAULT '',
	started INTEGER NOT NULL,
	ended   INTEGER,
	game    TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (room, started)
);
CREATE TABLE IF NOT EXISTS samples (
	room TEXT NOT NULL,
	ts   INTEGER NOT NULL,
	peer TEXT NOT NULL,
	kind TEXT NOT NULL,
	data TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS runs_id ON runs (id);
CREATE INDEX IF NOT EXISTS samples_room_ts ON samples (room, ts);
CREATE TABLE IF NOT EXISTS events (
	room  TEXT NOT NULL,
	ts    INTEGER NOT NULL,
	level TEXT NOT NULL,
	kind  TEXT NOT NULL,
	peer  TEXT NOT NULL,
	msg   TEXT NOT NULL,
	data  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS events_room_ts ON events (room, ts);
CREATE TABLE IF NOT EXISTS peers (
	room  TEXT NOT NULL,
	peer  TEXT NOT NULL,
	name  TEXT NOT NULL,
	first INTEGER NOT NULL,
	last  INTEGER NOT NULL,
	PRIMARY KEY (room, peer)
);
`

// queueSize bounds the writes waiting for the writer (a few seconds of a
// busy room).
const queueSize = 8192

type opKind int

const (
	opSample opKind = iota
	opEvent
	opRunStart
	opRunEnd
	opPeer
	opSync
)

type op struct {
	kind              opKind
	room, peer, k, sl string // k: sample/event kind; sl: level or name
	msg               string
	ts                int64
	data              []byte
	done              chan struct{}
}

// Store is the telemetry database.
type Store struct {
	db      *sql.DB
	ops     chan op
	stop    chan struct{}
	wg      sync.WaitGroup
	dropped atomic.Int64
	now     func() time.Time
	closed  atomic.Bool
}

// Open opens (or creates) the database at path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, fmt.Errorf("telemetry: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("telemetry: %w", err)
	}
	// Private to the user, like history.json: it keeps names and addresses.
	for _, f := range []string{path, path + "-wal", path + "-shm"} {
		_ = os.Chmod(f, 0o600)
	}
	s := &Store{db: db, ops: make(chan op, queueSize), stop: make(chan struct{}), now: time.Now}
	s.wg.Add(1)
	go s.writer()
	return s, nil
}

// EndOpenRuns ends the runs a device left open when it stopped without
// closing them, at their last sample or event. Only the device calls it,
// when it starts (never the CLI, which may run next to it).
func (s *Store) EndOpenRuns() error {
	if s == nil {
		return nil
	}
	_, err := s.db.Exec(`UPDATE runs SET ended = max(started,
		coalesce((SELECT max(ts) FROM samples WHERE samples.room = runs.room AND samples.ts >= runs.started), started),
		coalesce((SELECT max(ts) FROM events WHERE events.room = runs.room AND events.ts >= runs.started), started))
		WHERE ended IS NULL`)
	return err
}

// Close writes what is queued and closes the database.
func (s *Store) Close() error {
	if s == nil || s.closed.Swap(true) {
		return nil
	}
	close(s.stop)
	s.wg.Wait()
	return s.db.Close()
}

// Dropped is how many writes were lost because the queue was full.
func (s *Store) Dropped() int64 { return s.dropped.Load() }

func (s *Store) push(o op) {
	if s == nil || s.closed.Load() {
		return
	}
	select {
	case s.ops <- o:
	default:
		s.dropped.Add(1)
	}
}

// Sync waits until every write queued before it is in the database.
func (s *Store) Sync() {
	if s == nil || s.closed.Load() {
		return
	}
	done := make(chan struct{})
	select {
	case s.ops <- op{kind: opSync, done: done}:
		<-done
	case <-time.After(5 * time.Second):
	}
}

func (s *Store) writer() {
	defer s.wg.Done()
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	var batch []op
	flush := func() {
		if len(batch) == 0 {
			return
		}
		s.commit(batch)
		for _, o := range batch {
			if o.done != nil {
				close(o.done)
			}
		}
		batch = batch[:0]
	}
	for {
		select {
		case o := <-s.ops:
			batch = append(batch, o)
			if o.kind == opSync || len(batch) >= 1000 {
				flush()
			}
		case <-tick.C:
			flush()
		case <-s.stop:
			for {
				select {
				case o := <-s.ops:
					batch = append(batch, o)
				default:
					flush()
					return
				}
			}
		}
	}
}

func (s *Store) commit(batch []op) {
	tx, err := s.db.Begin()
	if err != nil {
		s.dropped.Add(int64(len(batch)))
		return
	}
	defer tx.Rollback() //nolint:errcheck // a no-op after Commit
	for _, o := range batch {
		switch o.kind {
		case opSample:
			_, err = tx.Exec(`INSERT INTO samples (room, ts, peer, kind, data) VALUES (?, ?, ?, ?, ?)`, o.room, o.ts, o.peer, o.k, string(o.data))
		case opEvent:
			_, err = tx.Exec(`INSERT INTO events (room, ts, level, kind, peer, msg, data) VALUES (?, ?, ?, ?, ?, ?, ?)`, o.room, o.ts, o.sl, o.k, o.peer, o.msg, string(o.data))
		case opRunStart:
			_, err = tx.Exec(`INSERT OR IGNORE INTO runs (room, id, started, game) VALUES (?, ?, ?, ?)`, o.room, o.peer, o.ts, o.msg)
		case opRunEnd:
			_, err = tx.Exec(`UPDATE runs SET ended = ? WHERE room = ? AND ended IS NULL`, o.ts, o.room)
		case opPeer:
			_, err = tx.Exec(`INSERT INTO peers (room, peer, name, first, last) VALUES (?, ?, ?, ?, ?)
				ON CONFLICT (room, peer) DO UPDATE SET name = CASE WHEN excluded.name <> '' THEN excluded.name ELSE peers.name END, last = excluded.last`,
				o.room, o.peer, o.sl, o.ts, o.ts)
		}
		if err != nil {
			s.dropped.Add(1)
		}
	}
	if tx.Commit() != nil {
		s.dropped.Add(int64(len(batch)))
	}
}

// DeleteRoom removes everything kept about a room.
func (s *Store) DeleteRoom(room string) error {
	if s == nil {
		return nil
	}
	s.Sync()
	var errs []error
	for _, table := range []string{"samples", "events", "runs", "peers"} {
		if _, err := s.db.Exec(`DELETE FROM `+table+` WHERE room = ?`, room); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// DeleteAll removes the telemetry of every room (a factory reset).
func (s *Store) DeleteAll() error {
	if s == nil {
		return nil
	}
	s.Sync()
	var errs []error
	for _, table := range []string{"samples", "events", "runs", "peers"} {
		if _, err := s.db.Exec(`DELETE FROM ` + table); err != nil {
			errs = append(errs, err)
		}
	}
	_, _ = s.db.Exec(`VACUUM`)
	return errors.Join(errs...)
}

// Room returns the recorder of one room. A nil Store gives a recorder
// that records nothing.
func (s *Store) Room(room string) *Recorder {
	if s == nil {
		return nil
	}
	return &Recorder{s: s, room: room}
}

// Recorder writes the telemetry of one room. Every method is safe on a
// nil Recorder (it does nothing).
type Recorder struct {
	s    *Store
	room string
}

// ID is the room's id.
func (r *Recorder) ID() string {
	if r == nil {
		return ""
	}
	return r.room
}

func (r *Recorder) ms() int64 { return r.s.now().UnixMilli() }

// Start opens a run: the room is on, playing game. id names this run (the
// history's game id) so a game in the history finds its telemetry.
func (r *Recorder) Start(id, game string) {
	if r == nil {
		return
	}
	ts := r.ms()
	r.s.push(op{kind: opRunEnd, room: r.room, ts: ts}) // a run left open ends here
	r.s.push(op{kind: opRunStart, room: r.room, peer: id, ts: ts, msg: game})
	r.Event(Info, "run_start", "", "the room started", map[string]any{"game": game, "run": id})
}

// End closes the open run.
func (r *Recorder) End(reason string) {
	if r == nil {
		return
	}
	r.Event(Info, "run_end", "", "the room stopped", map[string]any{"reason": reason})
	r.s.push(op{kind: opRunEnd, room: r.room, ts: r.ms()})
}

// Sample records the metrics of the room (peer "") or of one participant
// now. kind says who measured it: "room" and "peer" the device, "client"
// the participant's browser.
func (r *Recorder) Sample(peer, kind string, m Metrics) {
	if r == nil || len(m) == 0 {
		return
	}
	data, err := json.Marshal(m)
	if err != nil {
		return
	}
	r.s.push(op{kind: opSample, room: r.room, ts: r.ms(), peer: peer, k: kind, data: data})
}

// Event records something that happened, with its details.
func (r *Recorder) Event(level Level, kind, peer, msg string, data map[string]any) {
	if r == nil {
		return
	}
	b := []byte("{}")
	if len(data) > 0 {
		if j, err := json.Marshal(data); err == nil {
			b = j
		}
	}
	r.s.push(op{kind: opEvent, room: r.room, ts: r.ms(), sl: string(level), peer: peer, k: kind, msg: msg, data: b})
}

// Peer remembers a participant's name (the last one it used).
func (r *Recorder) Peer(peer, name string) {
	if r == nil || peer == "" {
		return
	}
	r.s.push(op{kind: opPeer, room: r.room, peer: peer, sl: name, ts: r.ms()})
}

// db gives the queries their handle with a timeout.
func (s *Store) query(q string, args ...any) (*sql.Rows, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	return rows, cancel, nil
}
