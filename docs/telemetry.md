# Room telemetry

The device keeps what happens in each room while it runs, so a freeze, a lag or a stutter can be explained afterwards: was it the game on the host's computer, the host's internet, or one guest's connection? The host sees it in **My device › History › Network** (charts, freezes with their likely cause, the raw log), and the CLI writes it out for a closer look.

## Storage

- One SQLite database next to the rooms: `~/go-link-console/telemetry.db` (WAL), opened by `internal/telemetry` with the pure-Go driver `modernc.org/sqlite` (no cgo, so every platform builds it the same way).
- **Nothing is dropped by age.** A room's data stays from its first start through every time it was on, also while it is off; starting it again adds to it. Deleting the room for good (`purge`) deletes everything kept about it; a factory reset deletes the whole database.
- Writes never block the media path: they go through a buffered queue (8192) to one writer goroutine that commits them in batches every 500 ms. A full queue drops the write and counts it.
- Tables:

| Table | Columns | What |
|---|---|---|
| `runs` | `room, id, started, ended, game` | Every time a room was on. `id` is the **game id**, the same as the history entry's (`HistoryEntry.ID`, shown as `#a1b2c3d4` in My device › History). `ended` is null while it runs; a device that stopped without closing a run ends it at its last sample when it starts again (`EndOpenRuns`, never from the CLI). |
| `samples` | `room, ts, peer, kind, data` | One sample per second of the room (`peer` empty, `kind: "room"`) and of each participant, measured by the device (`"peer"`) or by the participant's browser (`"client"`). `data` is a JSON object of numbers. |
| `events` | `room, ts, level, kind, peer, msg, data` | The log: `level` is `info`, `warn` or `error`. |
| `peers` | `room, peer, name, first, last` | The last name each participant used. |

The test pattern room is recorded too (room `test`, one run per device run).

## Samples

**`room`** (the device, every second):

| Metric | Meaning |
|---|---|
| `fps_in` | Frames the game (its process) delivered |
| `gap_max_ms` | Longest pause between two of them: about 17 ms at 60 fps; a freeze on the host shows here |
| `enc_avg_ms`, `enc_max_ms`, `enc_late` | Encoding time per frame, and frames that took longer than a frame |
| `fps_sent`, `kbps` | What the encoder sent |
| `viewers` | Participants connected |
| `cpu_pct`, `proc_cpu_pct`, `proc_mem_mb` | The computer's CPU, go-link's (with its games) and its memory |
| `enc_load_pct` | The share of the last second spent encoding, every tier together (`encode_load.go`); 75 % or more for five seconds is an overload |
| `net_up_kbps`, `net_down_kbps` | Everything the host's computer sends and receives: every guest's picture shares the upload |
| `hud` | 1 while the latency test draws the controllers |

**`peer`** (the device, about each participant, every second):

| Metric | Meaning |
|---|---|
| `input_pps`, `input_lost` | Input packets received, and the ones missing from their sequence |
| `input_gap_max_ms` | Longest silence while a control was held (the browser repeats a held control every 100 ms) |
| `rr_loss_pct`, `rr_jitter_ms`, `rr_lost` | The browser's RTCP receiver reports on the video: fraction lost, jitter, total lost |
| `video_kbps_target` | The bitrate this participant's video is sent at: its quality step ([quality ladder](protocol.md#playout-delay)) |
| `playout_max_ms` | The longest wait the device lets this browser add before showing a frame ([playout delay](protocol.md#playout-delay)) |
| `pli`, `nack` | Keyframe requests and retransmission requests |
| `voice_in_pps` | Packets of that player's microphone reaching the device |
| `ctl_rtt_ms` | Round trip of the control channel's ping |
| `ice_rtt_ms` | Round trip of the ICE pair in use (every 2 s) |

**`client`** (the participant's browser, every 2 s, `client_report` on the control channel; the device keeps only these numbers, each within a range):

| Metric | Meaning |
|---|---|
| `rtt_ms` | Round trip of the pair in use |
| `video_loss_pct`, `audio_loss_pct` | Packets lost in the last 2 s |
| `freeze_ms`, `freezes` | How long the picture stood still (the browser's own freeze count) |
| `dropped`, `fps` | Frames dropped and shown |
| `jitter_ms`, `buffer_ms`, `decode_ms` | Jitter, jitter buffer and decoding |
| `kbps` | Received |
| `e2e_ms`, `e2e_last_ms` | The latency test's median and last measurement, while it is on |
| `hidden` | 1 when the tab is hidden (the browser may slow it down) |

## Events

| Kind | Level | When |
|---|---|---|
| `run_start`, `run_end` | info | The room started (with the game id) or stopped (with the reason) |
| `game_ready`, `game_failed` | info, error | The game is running; it stopped with an error |
| `peer_state`, `peer_path` | info, warn | A participant's connection changed (failed and disconnected warn); how it connected (direct or relay, candidate types, address) |
| `join`, `leave`, `name` | info | Arrivals, departures and names (from the Room Manager) |
| `took_seat`, `left_seat`, `seat_free`, ... | info | The Room Manager's events, as in the chat |
| `frame_gap` | warn | The game sent no frame for 250 ms or more (`gap_ms`) |
| `video_loss` | warn | A receiver report with 5 % or more lost (at most every 5 s per participant) |
| `input_gap` | warn | A held control went quiet for 400 ms or more |
| `client_freeze` | warn | A browser reported a frozen picture of 250 ms or more (never while its tab is hidden: the browser stops drawing it on purpose, so a hidden tab's `freeze_ms` and `freezes` are not kept) |
| `video_quality` | info | A participant's video quality stepped down or up, with the reason (`from_kbps`, `to_kbps`, `step`) |
| `host_overloaded` | warn | Encoding kept the host busy 75 % of each second for five seconds: a 2x room went to saver (`action: "saver"`), or even 1x did not keep up (`action: "none"`); `encode_pct` |
| `seat_released` | info | The host freed a seat: to the queue or to watching (`port`, `mode`) |
| `playout_delay` | info | A participant's playout delay changed, with the reason (`from_ms`, `to_ms`) |
| `client_hidden` | info | A participant's tab was hidden or shown again |
| `signal_down`, `signal_up` | warn, info | The device lost (or got back) the signaling server: games already playing go on, nobody new gets in |

## Incidents

`Store.Incidents` turns the freezes of a span into incidents: a `frame_gap` (the device) or a `client` sample with `freeze_ms` of 250 ms or more (a browser). Signals less than 1.5 s apart are one incident. For each one it looks at who was there (samples within 5 s), the packets they lost and whether voice kept flowing, and gives a verdict:

| Verdict | When | Meaning |
|---|---|---|
| `device` | The game paused its frames | The host's computer: the game or the encoder. If voice kept flowing through the device, the network was up |
| `host_network` | Everyone froze and at least half lost packets | The host's internet (its upload) |
| `host` | Everyone froze without lost packets while the game kept sending | The host's computer or network |
| `guest` | Only some froze | Those players' internet or computer |
| `network` | One participant, with lost packets | The host's or that player's internet: nobody to compare with |
| `player` | One participant, no lost packets, frames kept coming | That player's browser or computer (a hidden tab, a busy machine) |

Voice is the key witness: guests' microphones travel through the host's device (star topology), so voice that keeps flowing during a freeze rules out the network between them and the host.

## Protocol

Linked browsers of the host ask on the control channel (only trusted links; the device reads these messages on their own, before the generic message, because `from` and `to` are numbers):

| Message | Fields | Answer |
|---|---|---|
| `telemetry_runs` | `req`, `id` (room; `test` for the test pattern room) | `runs` and `peers` |
| `telemetry_series` | `req`, `id`, `from`, `to` (unix ms; 0: the first run, now), `step` (ms), `metrics` (`kind.metric`) | `series`: `{peer, kind, metric, avg[], max[]}` per participant and metric, `null` where there was no sample, and the `step` used |
| `telemetry_events` | `req`, `id`, `from`, `to`, `limit` (≤ 300), `level` (`""`, `warn`, `error`) | `events`, oldest first, and `more` |
| `telemetry_incidents` | `req`, `id`, `from`, `to` | `incidents`: `{start, end, verdict, why, peers, device_gap_ms, lost, voice}` |
| `telemetry_find` | `req`, `run` (a game id or its first characters, `#` allowed) | `id` (the room) and `run` |

Every answer echoes `req` and `type`, and carries `error` when it fails. An answer must fit one DataChannel message (60 KB): a series too long gets longer buckets (at most 400 points), a page of the log fewer events (with `more`). Guests send `client_report` (see [Samples](#samples)); the device drops it from anyone who is not a room viewer.

## CLI

The CLI reads the database while the device runs (SQLite allows it):

```bash
device telemetry rooms                        # every room with telemetry
device telemetry runs ROOM                    # its runs, with their game ids
device telemetry events ROOM|#GAME [--warn] [--since 30m]
device telemetry incidents ROOM|#GAME [--json]
device telemetry export ROOM|#GAME [--since 2h]   # everything, as JSON lines
```

`#GAME` is a game id from the history (its first characters are enough when they name one game); it covers that run, a room covers every run since its first; `--since` keeps only the last part of either. Flags may go before or after the room or game. `export` writes the runs and participants first, then every sample and event in time order: `{"type":"sample","at":...,"kind":"client","peer":"...","m":{...}}`, `{"type":"event",...}`.

## Website

My device › History has a **Game ID** column (a click copies `#id`) and a **Network report** action per game (`/device/history/<room>/network?run=<id>`; without `run`, every time the room was on). The page (`components/device/NetworkReport.tsx`, charts in `TimeChart.tsx`) shows the freezes with their verdict (the three longest, and **Show all**), eleven charts (latency with the latency test dashed, lost packets, frozen picture with the game's pauses, frames per second, controls, jitter and buffer, each player's video wait from `playout_max_ms`, the host's CPU and upload, voice) with the freezes marked under the plot and a cursor that lists every value, and the raw log (all or warnings, more pages, **Copy log**). A running room refreshes every 5 seconds. Dragging across any chart zooms every chart into that span (`from` and `to` in the address, in unix ms, so a zoomed report can be shared; the device answers with buckets down to 2 s); a double click or **Show everything** goes back. **Enlarge** opens one chart in a dialog with **Save PNG** and **Copy image**: the picture carries the title, the room, the game id, the span and the legend (`chartImage.ts`, the SVG copied with its computed colors as presentation attributes, since the CSP forbids inline styles). Two participants with the same name (someone who came back) are told apart by the time they joined.
