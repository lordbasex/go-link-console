// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/internal/telemetry"
)

// telemetryRequest is what the host's linked browser asks about a room's
// telemetry: telemetry_runs, telemetry_series, telemetry_events,
// telemetry_incidents or telemetry_find (a game id from the history).
type telemetryRequest struct {
	Type    string   `json:"type"`
	Req     int      `json:"req"`  // echoed, to match the answer
	ID      string   `json:"id"`   // the room ("test" for the test pattern room)
	Run     string   `json:"run"`  // telemetry_find: a game id
	FromMs  int64    `json:"from"` // unix ms; 0: the room's first run
	ToMs    int64    `json:"to"`   // unix ms; 0: now
	StepMs  int64    `json:"step"` // series bucket; 0: fit maxPoints
	Metrics []string `json:"metrics"`
	Limit   int      `json:"limit"`
	Level   string   `json:"level"` // events: "", "warn" or "error"
}

const (
	// maxPoints bounds a series so an answer fits one control message.
	maxPoints = 400
	// maxEvents bounds one page of the log.
	maxEvents = 300
)

// maxAnswer is the largest answer sent on the control channel (a
// DataChannel message has a size limit; 64 KiB is safe everywhere).
const maxAnswer = 60_000

// answerTelemetry builds the answer to one request, made to fit one
// control message: a series too long gets longer buckets, a page of the
// log fewer events (with "more"). Run it off the message loop: a long
// room's queries take a moment.
func answerTelemetry(store *telemetry.Store, data []byte) []byte {
	var q telemetryRequest
	if json.Unmarshal(data, &q) != nil {
		return nil
	}
	for try := 0; try < 10; try++ {
		res := buildTelemetry(store, q)
		if res == nil {
			return nil
		}
		b, err := json.Marshal(res)
		if err != nil {
			return nil
		}
		if len(b) <= maxAnswer {
			return b
		}
		switch q.Type {
		case "telemetry_series":
			step, _ := res["step"].(int64)
			q.StepMs = max(step, 1000) * 2
		case "telemetry_events":
			n, _ := res["events"].([]telemetry.Event)
			q.Limit = max(len(n)/2, 1)
		default:
			b, _ = json.Marshal(map[string]any{"type": q.Type, "req": q.Req, "id": q.ID, "error": "too much to send"})
			return b
		}
	}
	return nil
}

// buildTelemetry answers one request.
func buildTelemetry(store *telemetry.Store, q telemetryRequest) map[string]any {
	res := map[string]any{"type": q.Type, "req": q.Req, "id": q.ID}
	if store == nil {
		res["error"] = "telemetry is off"
		return res
	}
	if q.Type == "telemetry_find" {
		room, run, err := store.FindRun(q.Run)
		if err != nil {
			res["error"] = err.Error()
		} else {
			res["id"], res["run"] = room, run
		}
		return res
	}
	if q.ID == "" || len(q.ID) > 64 {
		res["error"] = "no room"
		return res
	}
	runs, err := store.Runs(q.ID)
	if err != nil {
		res["error"] = err.Error()
		return res
	}
	to := time.Now()
	if q.ToMs > 0 {
		to = time.UnixMilli(q.ToMs)
	}
	from := to.Add(-time.Hour)
	if len(runs) > 0 {
		from = runs[0].Started
	}
	if q.FromMs > 0 {
		from = time.UnixMilli(q.FromMs)
	}
	res["from"], res["to"] = from.UnixMilli(), to.UnixMilli()
	switch q.Type {
	case "telemetry_runs":
		peers, err := store.Peers(q.ID)
		if err != nil {
			res["error"] = err.Error()
		}
		res["runs"], res["peers"] = runs, peers
	case "telemetry_series":
		span := to.Sub(from)
		step := time.Duration(q.StepMs) * time.Millisecond
		if min := span / maxPoints; step < min {
			step = min
		}
		step = max(step.Round(time.Second), time.Second)
		series, err := store.Series(q.ID, from, to, step, q.Metrics)
		if err != nil {
			res["error"] = err.Error()
		}
		res["step"], res["series"] = step.Milliseconds(), series
	case "telemetry_events":
		limit := q.Limit
		if limit <= 0 || limit > maxEvents {
			limit = maxEvents
		}
		events, err := store.Events(q.ID, from, to, limit+1, telemetry.Level(q.Level))
		if err != nil {
			res["error"] = err.Error()
		}
		if len(events) > limit {
			events, res["more"] = events[:limit], true
		}
		res["events"] = events
	case "telemetry_incidents":
		inc, err := store.Incidents(q.ID, from, to)
		if err != nil {
			res["error"] = err.Error()
		}
		res["incidents"] = inc
	default:
		return nil
	}
	return res
}

// openTelemetry opens ~/go-link/telemetry.db for the CLI (the device may
// be running: SQLite lets both read it).
func openTelemetry() (*telemetry.Store, error) {
	base, err := dataDir()
	if err != nil {
		return nil, err
	}
	return telemetry.Open(filepath.Join(base, "telemetry.db"))
}

// telemetryTarget turns ROOM or #GAME (a game id from the history, or its
// first characters) into a room and a time range: a game covers its own
// run, a room every run since its first. since keeps only the last part
// of either.
func telemetryTarget(store *telemetry.Store, arg string, since time.Duration) (room string, from, to time.Time, err error) {
	to = time.Now()
	if r, run, ferr := store.FindRun(arg); ferr == nil {
		from = run.Started
		if !run.Ended.IsZero() {
			to = run.Ended
		}
		if since > 0 && to.Add(-since).After(from) {
			from = to.Add(-since)
		}
		return r, from, to, nil
	}
	room = strings.TrimPrefix(arg, "#")
	runs, err := store.Runs(room)
	if err != nil {
		return "", from, to, err
	}
	if len(runs) == 0 {
		return "", from, to, fmt.Errorf("no telemetry for %q: name a room id or a game id (device telemetry rooms)", arg)
	}
	from = runs[0].Started
	if since > 0 {
		from = to.Add(-since)
	}
	return room, from, to, nil
}

func cmdTelemetryRooms(args []string) error {
	fs, _ := newFlags("telemetry rooms")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	store, err := openTelemetry()
	if err != nil {
		return err
	}
	defer store.Close()
	rooms, err := store.Rooms()
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ROOM\tGAME\tFIRST\tLAST\tRUNS\tSAMPLES\tEVENTS")
	for _, r := range rooms {
		game := ""
		if runs, _ := store.Runs(r.Room); len(runs) > 0 {
			game = runs[len(runs)-1].Game
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%d\t%d\n", r.Room, game, r.First.Local().Format("2006-01-02 15:04"), r.Last.Local().Format("2006-01-02 15:04"), r.Runs, r.Samples, r.Events)
	}
	return w.Flush()
}

func cmdTelemetryRuns(args []string) error {
	fs, _ := newFlags("telemetry runs")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: device telemetry runs ROOM")
	}
	store, err := openTelemetry()
	if err != nil {
		return err
	}
	defer store.Close()
	room, _, _, err := telemetryTarget(store, fs.Arg(0), 0)
	if err != nil {
		return err
	}
	runs, err := store.Runs(room)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "GAME ID\tSTARTED\tENDED\tLENGTH\tGAME")
	for _, r := range runs {
		end, length := "running", time.Since(r.Started)
		if !r.Ended.IsZero() {
			end, length = r.Ended.Local().Format("2006-01-02 15:04:05"), r.Ended.Sub(r.Started)
		}
		fmt.Fprintf(w, "#%s\t%s\t%s\t%s\t%s\n", r.ID, r.Started.Local().Format("2006-01-02 15:04:05"), end, length.Round(time.Second), r.Game)
	}
	return w.Flush()
}

func cmdTelemetryExport(args []string) error {
	fs, _ := newFlags("telemetry export")
	since := fs.Duration("since", 0, "only the last part of a room's telemetry (e.g. 2h)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: device telemetry export ROOM|#GAME [--since 2h]")
	}
	store, err := openTelemetry()
	if err != nil {
		return err
	}
	defer store.Close()
	room, from, to, err := telemetryTarget(store, fs.Arg(0), *since)
	if err != nil {
		return err
	}
	return store.Export(os.Stdout, room, from, to)
}

func cmdTelemetryEvents(args []string) error {
	fs, _ := newFlags("telemetry events")
	since := fs.Duration("since", 0, "only the last part (e.g. 30m)")
	warn := fs.Bool("warn", false, "only warnings and errors")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: device telemetry events ROOM|#GAME [--warn] [--since 30m]")
	}
	store, err := openTelemetry()
	if err != nil {
		return err
	}
	defer store.Close()
	room, from, to, err := telemetryTarget(store, fs.Arg(0), *since)
	if err != nil {
		return err
	}
	level := telemetry.Info
	if *warn {
		level = telemetry.Warn
	}
	events, err := store.Events(room, from, to, 0, level)
	if err != nil {
		return err
	}
	names := map[string]string{}
	if peers, err := store.Peers(room); err == nil {
		for _, p := range peers {
			names[p.ID] = p.Name
		}
	}
	for _, e := range events {
		who := ""
		if e.Peer != "" {
			who = names[e.Peer]
			if who == "" {
				who = e.Peer[:min(8, len(e.Peer))]
			}
			who = " [" + who + "]"
		}
		data := ""
		if len(e.Data) > 0 {
			data = " " + string(e.Data)
		}
		fmt.Printf("%s %-5s %-14s%s %s%s\n", e.At.Local().Format("2006-01-02 15:04:05.000"), e.Level, e.Kind, who, e.Msg, data)
	}
	return nil
}

func cmdTelemetryIncidents(args []string) error {
	fs, _ := newFlags("telemetry incidents")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: device telemetry incidents ROOM|#GAME [--json]")
	}
	store, err := openTelemetry()
	if err != nil {
		return err
	}
	defer store.Close()
	room, from, to, err := telemetryTarget(store, fs.Arg(0), 0)
	if err != nil {
		return err
	}
	inc, err := store.Incidents(room, from, to)
	if err != nil {
		return err
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(inc)
	}
	if len(inc) == 0 {
		fmt.Println("No freezes in", room, "between", from.Local().Format("2006-01-02 15:04"), "and", to.Local().Format("2006-01-02 15:04"))
		return nil
	}
	for _, i := range inc {
		fmt.Printf("%s  %5.1f s  %-12s %s\n", i.Start.Local().Format("2006-01-02 15:04:05"), i.End.Sub(i.Start).Seconds(), i.Verdict, i.Why)
	}
	return nil
}
