# Device protocol (WebRTC)

This page describes what the device and the website say to each other over WebRTC. The signaling protocol (pairing, rooms, invitations, the `signal` envelope) belongs to signalhub and is documented in [its README](https://github.com/lordbasex/signalhub). A protocol change goes to signalhub's README first, then to the Go models (`backend-device/internal/models`, `pkg/signalclient`) and the TypeScript types (`frontend/packages/shared`).

## Two kinds of connection

| Who | How they arrived | Gets | For |
|---|---|---|---|
| **Room guest** | signalhub `join` (invitation) | Video, audio, voice, `control`, `input` | Playing or watching |
| **Linked browser** | `claim` with the code, or `reach` | `control` + `files` | Managing the device: hardware, CPU, RAM, latency, ROM library, core, rooms, history |

Hardware and usage data go **only** to linked browsers, never to room guests.

## Channels

| Channel | Kind | Settings | Content |
|---|---|---|---|
| Video | Track | VP8 | Emulator frames: the game's picture enlarged 2x (video quality high or normal) or at its own size (saver, the test pattern room). See [Video scale](#video-scale) |
| Audio | Track | Opus 48 kHz stereo | Game sound |
| Voice | Track (one per seat) | Opus 48 kHz mono | Seated players' microphones, forwarded by the device |
| `control` | DataChannel | `ordered: true`, reliable | JSON: chat, room state, queue, device status |
| `input` | DataChannel | `ordered: false`, `maxRetransmits: 0` | 12-byte binary packets: sequence, local player, buttons, sticks |
| `files` | DataChannel | `ordered: true`, reliable | Linked browsers only: ROM zips and images dropped on the website, and recordings downloaded from the device |

### Audio and voice tracks

Each guest connection carries these m-lines, in this order:

| # | Track | Direction | `stream id` | Content |
|---|---|---|---|---|
| 0 | Video | device → browser | `go-link` | VP8 game video |
| 1 | Audio | device → browser | `go-link` | Opus game sound |
| 2-5 | Voice P1 to P4 | device → browser | `voice-p1` … `voice-p4` | Forwarded voice of each port |
| 6 | Microphone | browser → device | | The guest's microphone |

- Voice is **never mixed** with the game or with other voices: the device forwards each player's RTP packets to the other players' `voice-pN` tracks. So the browser has separate volumes for game and voice and can silence one player.
- Every track exists from the first offer: winning or losing a seat never renegotiates or interrupts the video.
- The browser answers the microphone m-line as `sendonly` and attaches the microphone with `replaceTrack` when the player turns it on (with echo cancellation, noise suppression and auto gain).
- **Rules, enforced on the device:** only seated players' audio is forwarded; nobody gets their own voice; spectators and the queue get none, unless the room has `spectators_hear_voice`.
- Cost: each voice is ~30 kbps Opus. With 4 players talking it is under 5 % of the video, and forwarding uses no codec CPU.

## Negotiation

The **device always offers** and the browser answers. Messages travel inside signalhub's `signal` payload, which signalhub does not read:

```json
{ "kind": "offer", "sdp": "..." }
{ "kind": "answer", "sdp": "..." }
{ "kind": "candidate", "candidate": { "candidate": "...", "sdpMid": "0", "sdpMLineIndex": 0 } }
```

ICE is trickle: candidates are sent as they appear. STUN/TURN servers for `RTCPeerConnection` come from signalhub's `hello`.

### The room PIN gate

Also inside `signal`, before any offer:

```json
{ "kind": "pin_required" }                                         // device → guest, on joining
{ "kind": "pin", "pin": "123456" }                                 // guest → device (the PIN of their invitation)
{ "kind": "pin", "token": "<43 chars base64url>" }                 // guest → device (return token or owner key)
{ "kind": "pin_result", "ok": true, "token": "<return token>" }    // device → guest
{ "kind": "pin_result", "ok": false, "reason": "used", "left": 4 } // device → guest
```

`reason` is `wrong` (`left` tries remain), `used` (another person used that invitation), `blocked` (this guest used their 5 tries) or `locked` (20 failures in 10 minutes in this room: nothing is accepted for 10 minutes, `retry_after` in seconds). A made-up token counts as a failed try. The website tries, in order, the owner key, the saved return token and the PIN typed on the guest page, and only shows the form when none works.

## `input` channel

12 bytes, big endian:

| Bytes | Field | Type |
|---|---|---|
| 0-1 | Sequence (wraps at 65535) | `uint16` |
| 2 | Local player (0 to 3) | `uint8` |
| 3 | Reserved (0) | `uint8` |
| 4-7 | Buttons | `uint32` (bits below) |
| 8-11 | Left stick X, Y and right stick X, Y | `int8` (-127 to 127, 0 = center) |

The **local player** lets two or more people play from the same browser (for example one on the keyboard and one on a gamepad) over the same channel. Only the host's own browsers get a seat per local player; a guest's browser gets one seat (its first local player) and the device drops the other local players' input, however many it lists. The website sends one local player unless the host turns on **Several controllers**. The device keeps one state per browser and local player and drops packets older than the last one. The browser repeats the state every 100 ms while anything is pressed, because the channel does not retransmit.

| Bit | Button | Keyboard | Standard gamepad |
|---|---|---|---|
| 0-3 | Up, down, left, right | Arrows | D-pad and left stick |
| 4 | Button 1 (bottom) | `Z` | 0 |
| 5 | Button 2 (right) | `X` | 1 |
| 6 | Button 3 (left) | `C` | 2 |
| 7 | Button 4 (top) | `A` | 3 |
| 8 | Button 5 (L1) | `S` | 4 |
| 9 | Button 6 (R1) | `D` | 5 |
| 10 | Start (your own seat's) | `Enter` | 9 |
| 11 | Coin | `5` | 8 |
| 12, 13 | L2, R2 | `Q`, `W` | 6, 7 |
| 14, 15 | L3, R3 | | 10, 11 |
| 16 | Home | | 16 |
| 17 | Capture | | 17 |
| 18-21 | Start of player 1 to 4 (1P to 4P) | `1`, `2`, `3`, `4` | |

- Bits 18 to 21 are the **Start row of an arcade panel**: any seated player can press another port's Start. The device merges them into that port's Start (`PortPad`) and ignores them from people without a seat.
- L2, R2, L3 and R3 never reach the core: in mame2003-plus they toggle things for everyone (L3 turns the game sound off, R2 opens MAME's menu).
- In the emulator: RetroPad B, A, Y, X, L, R are MAME buttons 1 to 6; Select is Coin.
- Gamepads are read with the browser's **Gamepad API** (USB and Bluetooth, no permissions or drivers). WebUSB is not used: Chrome blocks it for HID devices. By default the keyboard and every gamepad are one local player (the keyboard's); with the host's **Several controllers** switch on, each gamepad is a local player in connection order. Gamepads without the standard mapping take their first reading as rest, so an idle gamepad never sends buttons, and the website has a remap screen.

## `control` channel (JSON)

### Room guests

| `type` | Direction | Content |
|---|---|---|
| `welcome` | device → guest | Greeting when the channel opens |
| `hello` | guest → device | `name` (the player's name, see [Player names](#player-names)) and `local_players` (list of local players, 0 to 3). Sent again when they change |
| `room_state` | device → guest | Seats (`max_players`: as many as the game has players, 1 to 4, from its control panel; 4 when the game does not say, and in the test room), queue, spectators, `chat` (on/off), `info` (title, game, host, artwork), `you` (`name`: the name the device actually uses for you, after cleaning it; your `ports`, `queue_positions`, `spectator`, `swap_offers` and `swap_asked`; `owner`, `pause_asks` and `pause_asked`, see [Pausing](#pausing-is-the-hosts)), `pausable`, `paused`, `paused_by`, `host_online`, `controls` (`{players, buttons, control}` from the game's control panel, to draw the touch gamepad), `recording` (the host is recording the game with the players' voices: everyone sees a REC badge) and `picture` (`{style, bands}`, the host's default picture style for the room; absent = the site's default, see [Room picture default](#room-picture-default)). Personal to each guest, sent on every change |
| `stream_stats` | device → guest | `fps` sent, `width` and `height` of the frames sent, display `aspect`, `video` (`{scale, width, height, quality, fallback}`, see [Video scale](#video-scale)) and, during the host's latency test, `hud` (see [Latency test](#latency-test)). Every 2 s, when the channel opens and at once when the frame size changes |
| `video_want` | guest → device | `{width, height}`: the size the guest shows the video at, in device pixels (at most 2 per CSS pixel). Sent when the control channel opens and when the picture's box changes by more than 10 % (window, phone turned, full screen). go-link HD's test room sends each guest the smallest of its sizes (the source and its halves, never under 360 rows) that is at least 90 % of it; other rooms ignore it. The guest's `stream_stats` then gives its own size |
| `chat` | guest → device | `text` (up to 300 characters, 5 messages every 5 s) |
| `chat` | device → guest | `name`, `port`, `role`, `text`, `ts`, or `system` for notices. Every notice carries an `event` and its values in `args` (`name`, `port`, `name2`, `port2`), and the website shows it in the reader's language (the English `system` text is the fallback): `recording_started`, `recording_stopped`, `game_paused`, `game_resumed` (name), `now_watching` (name), `moved` (name, port), `swap_asked` (name, port, name2, port2), `kept_seat` (name, port), `swapped` (name, port, name2, port2), `left_seat` (name, port), `seat_free` (port), `took_seat` (name, port), `pause_declined` (name, port: only to the player whose request the host declined). `game_paused` carries `name2` too when the pause answers a request: `name` asked and `name2` (the host) paused. Generated guest names ("Guest 9F3A") are also shown translated. The last 50 on joining |
| `typing` | guest → device | `on` (`true` while typing, repeated every ~2.5 s; `false` when cleared). Sending a `chat` also clears it; the device clears it after 6 s without a repeat |
| `typing` | device → guest | `names` (`[{name, port}]`): who else is typing, never yourself |
| `spectate` | guest → device | Leave the seat and the queue, to just watch |
| `queue` | guest → device | Back to the queue from spectating |
| `swap_seat` | guest → device | `from`, `to` (ports). Swap controllers. Only from the person seated in `from` |
| `release_seat` | host → device | `port`, `mode` (`queue` or `watch`). Frees a seat, the host's own included: its player goes to the end of the queue (the head of the queue takes the seat) or watches. With nobody else waiting, `queue` works as `watch`, or the same player would take the seat back. Only from the host's browsers (`refused` `release_owner_only` otherwise); chat events `sent_to_queue` / `sent_to_watch` (`name`: the host, `name2`, `port`) and a `seat_released` telemetry event |
| `swap_answer` | guest → device | `from`, `to`, `accept`. The answer of the player in `to` |
| `pause` | guest → device | `paused` (`true` or `false`). **Only from the host** (an owner peer, see [Pausing](#pausing-is-the-hosts)), only while a game runs (the test card cannot pause). Anyone else gets `error` with `code: "pause_owner_only"`. While paused, the device repeats the last frame and sends no sound |
| `pause_request` | guest → device | Ask the host for a pause: `{"type":"pause_request"}`; withdraw it with `{"type":"pause_request","cancel":true}` |
| `pause_answer` | guest → device | `from` (the requester's peer id, from `you.pause_asks`), `accept`. Only from an owner peer |
| `error` | device → guest | A refused request: `code` (`pause_owner_only`) and a readable English `error` |
| `ping` / `pong` | device → browser → device | `id`. The device measures peer-to-peer latency every 2 s |

The browser measures its own latency from WebRTC ICE stats (`currentRoundTripTime`).

### Linked browsers

| `type` | Direction | Content |
|---|---|---|
| `auth` | linked → device | First message. Empty after redeeming a code; `link_id` and `token` when coming back with `reach`. Optional `terms`: the version of the terms of use accepted in that browser, kept with the link (`terms`, `terms_at` in `device.json`) |
| `auth_challenge` / `auth_proof` | linked ⇄ device | `link_id` + `nonce`, answered with `proof` (see [flows](flows.md#2-linking-a-browser-with-the-device)) |
| `auth_ok` | device → linked | `device_id`, `link_id` and, only after a code, the `token` to keep |
| `auth_failed` | device → linked | Readable `error`. The link no longer exists: the website forgets it and the device hangs up |
| `unlink` / `unlinked` | both | Remove this browser's link / the link was removed (from the website or the device window) |
| `device_status` | device → linked | Every 2 s: `rooms` (below), hardware (with `hostname`), CPU, RAM, machine-wide network traffic (`net_sent_bps`, `net_recv_bps`), STUN/TURN in use, the ROM library in numbers (`library.summary`: `revision`, `total`, `bytes`, `playable`, sets per state in `kinds`, the five `biggest` and the sets with each kind of picture in `thumbs`; never the sets themselves, see `roms_query`), `library.thumb_kind`, `library.disk`, `library.thumbnails_bytes`, `saves_bytes` the core state (`installed`, `catalog`), the device's `version` and, when a newer go-link was released, `update` (`latest`, `url` of its GitHub release page; the website only accepts go-link's own releases), `room`, the test pattern room (`room_id`, its invitation, `owner_key` and `picture`), `video_quality` (`high`, `normal` or `saver`: the host's choice for game rooms), go-link HD's games folder in numbers (`library.hd_dir`, `hd_games`, `hd_bytes`) and `hd`: its rooms' `room_size` (`720p` or `1080p`) and `codec` (`auto` or `vp8`) and its free `starter` games (`name`, `title`, `installed`, the installed `size` and `version`, `update` with the newer version that can be downloaded, and while downloading `downloading`, `done`, `total`; `error` after a failed download). Never includes the pairing code |
| `create_room` | linked → device | `rom`, `title`, `voice`, `chat` (absent = on), optional `art` (`boxart`, `title` or `snap`) and optional `picture` (`{style, bands}`, the room's default picture; unknown values are ignored). Opens a **new** room. `rom` `@maker` is the game Willy Maker sent (a `files` upload with `purpose: "maker"`), with optional `maker` (`{title, players, labels}`: the room's name and the buttons its genre names); it has one room, which a new `create_room` with `@maker` replaces (stopped and removed with its saves) |
| `room_created` / `room_error` | device → linked | `id` (device room), `room_id` (signalhub) or a readable `error`. Some errors carry a `code` the website translates: `too_many_rooms` with `limit`, `no_saves` |
| `room_action` | linked → device | `id` and `action`: `pause`, `resume` (the host's own pause; it also answers every pending request for one), `save` (optional `name`), `archive`, `delete` (to the trash), `purge` (forever, from the trash), `favorite`, `unfavorite`, `new_link` (new invitation), `chat_on` / `chat_off`, `input_hud_on` / `input_hud_off` (the latency test, also `id: "test"`; see [Latency test](#latency-test)), `record_start` / `record_stop` (see [Recordings](#recordings)), `picture` (with `style` and `bands`: the room's default picture; both empty clear it; `id: "test"` for the test pattern room; see [Room picture default](#room-picture-default)) |
| `pause_asked` | device → linked | A seated player asks the host for a pause: `id` (device room id, `test` for the test pattern room), `from` (the player's peer id), `name`, `port`, `expires_at`. Sent again (with a new `expires_at`) when the same player asks again |
| `pause_ask_gone` | device → linked | `id`, `from`: that request was answered, withdrawn, expired, or the player left the seat |
| `pause_answer` | linked → device | `id`, `from`, `accept`: the host's answer from the website. The device replies `room_result` with `action: "pause_answer"` and `ok` (`false` when there is no such request any more) |
| `room_start` | linked → device | `id` of an archived or trashed room and `from`: `continue`, `fresh` or `slot` (with `slot`) |
| `room_result` | device → linked | `id`, `action`, `ok`, readable `error` (and `code`: `record_paused`, `already_recording`, `not_recording`…) and, for `save`, `slot` |
| `close_room` | linked → device | `id`: archives that room (kept for older websites) |
| `invite` | linked → device | `id` of a room (or `test`): make a new invitation PIN |
| `invite_pass` | device → linked | `id`, `pin`, `expires_at` (or `error`) |
| `get_thumb` / `thumb` | both | `set`, `kind` (empty = the host's choice) and `size` (`card` 360x480, `mini` 96x128) / the same plus `data` (JPEG, base64) or `missing: true` |
| `download_core` | linked → device | Download the core and its game list (whatever is missing) |
| `set_thumbnails` / `thumbnails_result` | both | `kind` (`boxart`, `title` or `snap`) and/or `dir` (a folder on the device, or `default`): which picture everyone sees and where the device reads them, like the window's Settings / `ok`, `error` |
| `set_video_quality` / `video_quality_result` | both | `quality` (`high`, `normal` or `saver`): the video quality of game rooms, like the window's Settings; running rooms switch at once / `quality`, `ok`, `error` (an unknown value changes nothing). See [Video scale](#video-scale) |
| `set_roms_dir` / `roms_dir_result` | both | `dir`: absolute path of an existing folder on the device / `dir`, `ok`, `error` |
| `upload_result` | device → linked | Result of one file of the `files` channel: `id`, `name`, `ok`, `error`, and `code` for an error the web translates (`not_rom`) |
| `rom_test` | linked → device | `id` (the one of a `files` upload with `purpose: "rom_test"`), `set` (the set name), optional `frames` (600 to 3600, default 900). Powers the set on with the exact core. See [ROM test](#rom-test) |
| `rom_test_result` | device → linked | `id`, `set`, `ok`, `steps` (`name`, `ok`, `detail`), `frames`, `seconds`, optional `shot` (PNG of the last frame, base64), `own` (`id`, `title`: a go-link set) or `error` with `code` (`busy`, `not_found`) |
| `roms_query` | linked → device | A page of the ROM library: `req` (echoed), `q` (every word must be in the set's name, title or maker, any case), `filter` (`all`, `playable`, `unplayable` or a state: `runs`, `missing`, `unsupported`, `broken`, `bios`, `unchecked`), `sort` (`name` by title, `size` and `year` largest and newest first, `status`), `offset`, `limit` (at most 100). The device answers `roms_page` |
| `roms_get` | linked → device | Some sets by name (`names`, at most 100), for a room's or a past game's picture and title. The device answers `roms_page` with the ones in the folder |
| `roms_page` | device → linked | `req`, `revision` (the library's, from `device_status`), `total` (sets that match), `offset` and `roms`: each set with its `name`, `size`, `title`, `year`, `maker`, `check`, `thumbs` and, for go-link's own sets and go-link HD packages, `own`/`kind`, `description` and `controls`. A full MAME collection is about 5,000 sets, more than one WebRTC message carries (Chrome: 256 KiB), so the library is only sent by pages; the device searches, filters and sorts an index built once per scan of the folder. When `device_status` shows a new `revision` (the folder changed), the website asks for its pages again. A device before 0.2.9 still sends every set in `library.roms` and knows no `roms_query`: the website then makes the summary and the pages itself (`legacyRoms`, `queryRomsLocally`) |
| `get_history` / `clear_history` / `delete_history` | linked → device | Ask for the game history, clear it (**with every recording**), or delete one game (`id`, with its recordings). The device answers `history` (with `error` for an unknown `id`) |
| `history` | device → linked | `items`, newest first (up to 500): `id` (absent on games saved before ids existed), `room_id`, `name`, `rom`, `game`, `started_at`, `ended_at`, `peak_players`, `peak_spectators`, `reason` (`archived`, `deleted`, `failed` or `device_stopped`) and `people` (each browser that joined: `name`, `ports`, `ip` as seen on the chosen ICE pair, empty through TURN, and `path`, `direct` or `relay`) and `recordings` (the recordings made during that game, as below). Only the owner sees it |
| `get_recordings` / `delete_recording` | linked → device | List every finished recording / delete one (`id`). The device answers `recordings` |
| `recordings` | device → linked | `items` (newest first), `bytes` (space they take) and, after a failed delete, `error` and `code` |
| `recording_saved` / `recording_error` | device → linked | A recording ended: `room` (device room id), `reason` and `recording`, or a readable `error` when nothing could be kept (it stopped before the first picture). Sent to every linked browser, so the website can offer "Download now" |
| `factory_reset` | linked → device | `confirm: "factory_reset"` (anything else is ignored). See [Factory reset](device.md#factory-reset) |
| `factory_reset_result` | device → linked | `ok`, `error`. Then the device unlinks every browser |

Each room in `device_status.rooms`:

```json
{ "id": "…", "name": "Turtles co-op", "rom": "tmnt", "game": "Teenage Mutant Ninja Turtles",
  "voice": true, "state": "live", "favorite": true, "room_id": "…",
  "invite": "…", "invite_code": "…", "owner_key": "…",
  "players": 3, "max_players": 4, "spectators": 5, "queue": 0,
  "since": "2026-09-27T01:10:00Z", "deleted_at": null, "no_saves": false,
  "saves": [{ "slot": 1, "name": "Stage 3", "at": "2026-09-26T22:17:00Z" }], "autosave": true,
  "picture": { "style": "crt", "bands": "ambient" },
  "video": { "quality": "saver", "fallback": "cpu", "scale": 1 },
  "pause_asks": [{ "from": "<peer id>", "name": "Ana", "port": 2, "expires_at": "2026-09-27T01:12:30Z" }] }
```

`pause_asks` (only while someone asks) lists the pending requests for a pause, so a browser that opens later sees them too. `video` (only while the game runs) is what the room really sends: `quality` in use, `scale` (2 or 1) and `fallback: "cpu"` when it is lower than the host's choice because this computer could not keep up with 2x (see [Video scale](#video-scale)).

## VP8 packets

The device cuts each VP8 frame into RTP packets with its own payloader (`internal/services/vp8_payloader.go`, RFC 7741): every frame carries a 15-bit PictureID, 0 included, wrapping from 32767 to 0, as browsers send it. Pion's payloader leaves the PictureID out of frame 0 and writes IDs under 128 in 7 bits; every 32768 frames (9 min 6 s at 60 fps) the browser met a frame with no PictureID, lost the frame references and dropped the picture until the next keyframe.

## Playout delay

The device stamps every video packet with the `playout-delay` RTP header extension (`http://www.webrtc.org/experiments/rtp-hdrext/playout-delay`, registered for video in `NewWebRTCAPI`): the shortest and longest time the browser may hold a frame before showing it. Left alone, Chrome sizes that wait from how unevenly frames arrive and almost never lowers it: in a two-hour room with no freeze it went from 8 ms to 79 ms. Each connection has its own interceptor (`internal/services/playout_delay.go`), so each participant has their own wait:

- It starts at 0 ms: each frame is shown as soon as it is decoded (Chrome holds it about 0.5 ms).
- Each `client_report` (every 2 s) with a freeze, 2 % or more lost video packets, or 15 ms or more of jitter raises the longest wait one step: 40, 80, then 160 ms.
- 15 clean reports in a row (30 s) lower it one step. A hidden tab's reports never move it.

The minimum is always 0. Each change is a `playout_delay` telemetry event with its reason, and peer samples carry `playout_max_ms`. A browser that does not accept the extension keeps its own buffer.

Every room is tiered (`StreamConfig.Tiers`, `internal/services/video_tiers.go`): each guest gets its own video track, written with the frames of the tier it watches, so moving a guest never touches its RTP sequence. The tiers are the sizes go-link HD halves to, then two quality steps of the smallest size at half and a quarter of its bitrate (`stepKbps`, never under 200 kbps); an arcade game at 2x cannot be halved, so its tiers are the full picture and those two steps. A tier is encoded only while someone watches it (a recording always takes the first). Each guest's quality ladder (`quality_ladder.go`) moves it from its size's tier: a `client_report` with 3 % or more lost video or a freeze steps it down at once (with a keyframe on the new tier), 10 clean reports (20 s) step it back up; a hidden tab's reports never move it. Each step is a `video_quality` telemetry event with `from_kbps`, `to_kbps` and the reason, and peer samples carry `video_kbps_target`. The device reads each guest's RTCP receiver reports only for its video stream (its SSRC): browsers bundle the reports on every stream.

The game's sound uses one Opus encoder for everyone; once a second it is told the worst video loss any player's receiver report shows (`OPUS_SET_PACKET_LOSS_PERC`, up to 30 %, with in-band FEC on above 0). At 96 kbps stereo Opus codes music in CELT frames, which have no FEC of their own but lean less on the previous frame when loss is expected, so a lost packet spoils fewer of the next ones.

For tests, `GOLINK_SIM_LOSS=<percent>` (`internal/services/sim_loss.go`) drops that share of the RTP packets (video and sound) sent to every connection but the first one that got video, closest to the network (the first interceptor, after the NACK buffer), so a clean host and a lossy guest can be compared in one room. It is off unless set.

## Video scale

Arcade pixel art has one color per pixel, but VP8 (4:2:0) keeps one color sample per 2x2 block, so at the game's own size small colored details bleed. Game rooms therefore send the picture **enlarged 2x with nearest neighbour** (each game pixel becomes a 2x2 block, so it gets its own color sample), and the website averages it back before drawing. The [video quality lab](quality.md) measured about +7 dB RGB PSNR over the old stream.

**The host's choice** (`device.json` `video_quality`, the window's Settings, `device video quality`, `set_video_quality`):

| `quality` | Frames | VP8 target | Measured average |
|---|---|---|---|
| `high` (default) | 2x, nearest neighbour | 3,500 kbps | about 2.6 Mbps per room |
| `normal` | 2x, nearest neighbour | 2,500 kbps | about 2.0 Mbps |
| `saver` | the game's size, each 2x2 block's color averaged | 2,500 kbps | about 1.6 Mbps |

A change reaches running rooms at once (the encoder starts again with a keyframe). The test pattern room always streams its test card at its own size (640x480, `scale: 1`, no `quality`).

**Automatic fallback.** When a 2x room starts streaming, the device measures its encoder over the first two seconds (after 10 warm-up frames). If the 95th percentile of the encode time is more than 60 % of a frame's time (10 ms at 60 fps), 2x does not fit this computer next to what else it runs, and that room goes to `saver` for the rest of its run: the device logs it, `device_status.rooms[].video` says `{"quality": "saver", "fallback": "cpu"}` and guests see it in `stream_stats`. Choosing a quality again makes running rooms try 2x again.

**`stream_stats.video`**, the contract for every client that draws the picture:

```json
{ "type": "stream_stats", "fps": 59.6, "width": 768, "height": 448, "aspect": 1.3333,
  "video": { "scale": 2, "width": 384, "height": 224, "quality": "high" } }
```

- `scale` is `2` when every game pixel arrives as a 2x2 block, else `1`. `video.width` x `video.height` is the **game's own size**; the frames are `scale` times larger (the top-level `width` and `height`).
- `quality` (`high`, `normal`, `saver`) is present in game rooms; `fallback` (`"cpu"`) only when the room fell back. Absent `video` (older devices, before the first frame) means `scale: 1`.
- The device sends `stream_stats` when the control channel opens, every 2 s, and at once when the frame size changes (a quality change or a fallback), just after the keyframe of the new size.
- A client that draws the frames itself (a GPU renderer) must, when `scale` is 2 **and** the decoded frame is exactly 2 x `video.width` by 2 x `video.height`, first average each 2x2 block back to one pixel (a texture of the game's size: four samples at texel centers, or one bilinear sample at the block's center), then apply every style (smooth, sharp, CRT, edges, ambient light) to that texture. Otherwise it draws the frames as they are (during a size change the decoder may still show frames of the old size). Styles fed the raw 2x picture look wrong: CRT draws twice as many scanlines as the game has and edge smoothing sees 2-pixel steps.
- A client that shows the plain `<video>` (or the platform's video view) changes nothing: the 2x picture scales like any other.
- Stream statistics should show the frames and the game's size, e.g. "768×448 (2× of 384×224)", and the quality, "Saver (CPU)" after a fallback.

## Room picture default

How a browser draws the game (the website's [picture styles](web.md#picture-styles)) is each viewer's own choice, kept in their browser. The host may give a room a **default**, for guests who never chose:

1. A linked browser sends `room_action` `{"id": "<room id>", "action": "picture", "style": "crt", "bands": "ambient"}` (or `create_room` with `picture`). `id` is `test` for the test pattern room. `style` is `smooth`, `sharp`, `crt` or `edges`; `bands` is `black`, `ambient` or `frame`. Both empty clear the default. Any other value is refused (`room_result` with `ok: false`) and changes nothing.
2. The device keeps it in `device.json`: `rooms[].picture` for game rooms, `test_room_picture` for the test pattern room (a factory reset forgets both). A hand-edited unknown value is ignored when the device loads it.
3. Every guest gets it in `room_state.picture` (`{"style", "bands"}`; absent = none), and linked browsers see it in `device_status` (`rooms[].picture`, `room.picture`).
4. The website picks, per value: the viewer's own choice, else the room's default, else the site's default (**Smooth** with **Ambient** sides). The device never draws, checks or enforces the picture: it is a suggestion.

## Player names

`hello.name` is what the others see in the room and in the chat. The rule:

- After trimming and joining runs of spaces into one, **2 to 20 characters**.
- Only **Unicode letters** (`\p{L}`, any case and language, accents and `ñ` included), **digits** (`\p{N}`) and **spaces**. Any whitespace counts as a space. Text is NFC-normalized first, so a letter with a separate accent mark counts as one letter.

The device never trusts the browser: it removes every other character (symbols, emoji, punctuation, control characters), trims, joins spaces, cuts to 20 characters and, if fewer than 2 remain, keeps the generated name (`Guest 9F3A`). `room_state.you.name` is the name it actually uses, and every `name` in `room_state`, `chat` and `typing` is a cleaned name. The website and the apps check the same rule as the person types, to explain what is wrong.

```text
"  Ana   María "   -> "Ana María"
"<b>Zoë</b> 😀!!"   -> "bZoëb"
"Fede 😀"           -> "Fede"
"!!"                -> (kept: "Guest 9F3A")
```

## Pausing is the host's

The game belongs to the host, so only the host pauses and resumes it. The host is:

- an **owner peer** in the room: a guest connection that came in with the room's `owner_key` (from `device_status`) instead of a PIN, or the linked browser itself. The device marks it and sends it `room_state.you.owner: true`;
- a **linked browser**, with `room_action` `pause` / `resume` and `pause_answer`.

The other players ask:

1. A **seated** player sends `pause_request`, only while a pausable game runs and is not paused (anything else is ignored). One pending request per guest: asking again refreshes it. A request expires after **30 seconds**.
2. The requester sees `room_state.you.pause_asked: {"expires_at": "…"}` (`null` when it has none) and may withdraw it with `pause_request` + `cancel: true`.
3. Owner peers see the requests in `room_state.you.pause_asks: [{from, name, port, expires_at}]` (only owners get this field); linked browsers get `pause_asked` and `pause_ask_gone`.
4. The host answers with `pause_answer {from, accept}` (in the room) or `pause_answer {id, from, accept}` (linked). **Accept** pauses the game for everyone (`paused_by` is the requester's name; chat `game_paused` with `name` = requester and `name2` = the host) and clears every request. **Decline** removes that request and sends the requester alone a chat notice `pause_declined`.
5. `room_state.host_online` tells guests whether the host can answer (an owner peer is in the room, or a linked browser is connected to the device), so the website can explain why **Ask for a pause** is off.

A request also goes away when the game is paused (by anyone), when the game ends, and when the requester leaves the seat. A `pause` from a guest who is not the host is refused with `error` `pause_owner_only`.

## Recordings

The host (a linked browser) records a running game room with `room_action` `record_start`, and stops it with `record_stop`. The device writes a WebM file with these tracks: the game's VP8 picture and Opus sound, exactly as the room already encodes them for its guests, plus one Opus track per player's voice (`voice-p1`…`voice-p4`), copied from the packets it forwards. Nothing is decoded or encoded again, so recording costs a copy per packet and a disk write.

- Files live in `~/go-link/rec/<room id>/<room name>-<date>.webm` (mode `0600`), each with a `.json` description (a `recording` object, below). The device only ever serves a file under that folder by its `id`.
- A recording stops by itself at **2 hours or 2 GB**, when the game is **paused**, and when the room is archived, deleted, fails or the device shuts down. The website warns the host before pausing a recorded game. A paused game cannot start one (`code: record_paused`).
- Everyone in the room is told: `room_state.recording` and a chat notice (`event`), because their voices are recorded too.
- A recording the device never closed (power loss) is kept as `interrupted` at the next start: it plays, only without the seek index.

```json
{ "id": "a1b2/turtles-co-op-2026-09-28-2130.webm", "room_id": "a1b2", "room": "Turtles co-op",
  "file": "turtles-co-op-2026-09-28-2130.webm", "started_at": "2026-09-28T21:30:00Z",
  "duration_ms": 1122000, "size": 225000000, "sha256": "…",
  "tracks": ["video", "game", "voice-p1", "voice-p2"], "reason": "stopped" }
```

`reason` is `stopped`, `paused`, `limit_time`, `limit_size`, `room_stopped`, `device_stopped` or `interrupted`. Screenshots never involve the device: the browser draws the video on a canvas and saves a PNG.

## `files` channel

Only on linked browsers' connections, and only after the browser proved its link.

### Downloads (recordings)

The browser pulls a recording piece by piece, so it sets the pace, shows the progress, can stop at any time and checks the whole file at the end:

```json
{ "type": "download", "id": "t1", "file": "a1b2/turtles-co-op-2026-09-28-2130.webm" }
{ "type": "download_ready", "id": "t1", "file": "…", "name": "turtles-co-op-2026-09-28-2130.webm", "size": 225000000, "sha256": "…" }
{ "type": "read", "id": "t1", "offset": 0, "length": 61440 }
```

Each `read` is answered with one **binary** message: the offset (8 bytes, big endian) and then up to `length` bytes (at most 60 KB). The browser keeps a few reads in flight; the channel is ordered, so pieces arrive in the order asked and the browser hashes them as they come. It ends with `{ "type": "done", "id": "t1" }`, or stops with `{ "type": "cancel", "id": "t1" }` (what arrived is dropped). Any failure is `{ "type": "download_error", "id": "t1", "error": "…", "code": "…" }`. One download per browser at a time; the device closes one nobody reads for 2 minutes.

### Uploads (ROMs)

The host drops ROM zips on the website, and they land in the device's ROM folder, with no web server on the device. One file at a time:

```json
{ "type": "begin", "id": "f1", "name": "robby.zip", "size": 307200 }
```

Then the content in 16 KB **binary** messages, and finally:

```json
{ "type": "end", "id": "f1" }
```

The device answers on `control` with `upload_result`. Rules, enforced on the device:

- The name is a MAME set name: `.zip`, lowercase letters, digits and `_`, up to 16 characters. Only the base name is used, never a path.
- The file must start like a real ZIP, be at most 512 MB and match the announced `size`.
- It is written to a temporary file in the same folder and renamed at the end. It **never replaces** an existing set. Files get mode `0644`.
- If the channel closes halfway, the partial file is removed.
- A Willy Maker project (`.willy.zip`) or AI pack (`.ai-pack.zip`) is refused by its name, and a renamed one by what it holds (`project.json` or `PROMPT.md` at its top), with `code: "not_rom"`: they are a game's sources, and Create ROM makes its ROM (task T-21 of experiment 1's verdict). The web also stops them before sending, and the device window and the CLI say the same.
- The browser watches `bufferedAmount` so large files do not fill its memory.

With `"purpose": "rom_test"` in `begin`, the file is a set for a [ROM test](#rom-test) instead: the device writes it to `~/go-link/tmp/romtest/<id>/<set>.zip` (`0600`, at most 16 MB, `id` of letters, digits, `_` and `-`), never to the ROM folder or the library, and keeps only the last one waiting. With `"purpose": "maker"` the file is the game Willy Maker made (`slammast.zip`, at most 16 MB, a zip): the device keeps it in `~/go-link/maker/slammast.zip`, replacing the one before, never in the ROM folder, and `create_room` with `rom: "@maker"` opens its room (the website's **Play on my go-link**, `playMakerGame` in `packages/shared/src/maker-play.ts`). Any other `purpose` is refused (a device before 0.1.9 refuses `maker` too: the website then asks to update it).

## ROM test

Validation level 4 of [Willy Maker](willy-maker/validation.md#level-4-power-on-on-the-device): does a set really power on in the exact core this device runs? Only the linked owner can ask (the `files` channel and the linked `control` messages), never a room guest; one test runs at a time (`code: "busy"`).

1. Upload the zip on `files` with `purpose: "rom_test"` and wait for `upload_result`.
2. Send `{ "type": "rom_test", "id": "<the upload id>", "set": "slammast" }`.
3. The device runs `device romtest --child` twice, each a worker process like a game room's `emulate` (a fresh system folder, the test folder as the only ROM path, 60 s for the whole test): once with scripted input on player 1 (Coin at frame 300, Start at 360, right held from 420, then buttons 1-3) and once without. It answers `rom_test_result` and deletes the test folder (leftovers are removed at startup).

Steps, in order (a failing `zip`, `set` or `core.loaded` ends the test):

| `name` | Passes when |
|---|---|
| `zip` | the file is a readable zip with files |
| `set` | the core's game list knows the set and every file it needs is there (the library's check; skipped without the game list) |
| `identity` | always passes; `detail` says whether it is a go-link set (verified by the SHA-256 of every file inside, see [device.md](device.md#go-links-own-sets)) |
| `core.loaded` | the core loads the set |
| `core.files` | the core's loader found every file with its size (`NOT FOUND` or a wrong length fail; a wrong checksum only warns, as it does for a go-link set) |
| `video.picture` | a frame that is not black before frame 300 |
| `video.alive` | at least 10 different pictures in the last 300 frames (no freeze) |
| `audio` | the core keeps sending sound (at least half the samples of the time run); silence is allowed and said in `detail` |
| `input.reacts` | both runs show the same pictures until the first press, and a different one after it |
| `time.realtime` | the core ran faster than real time on this device |
| `core.run` | only when a worker stopped: a crash (`detail` names the signal) or the 60 s limit |

```json
{ "type": "rom_test_result", "id": "rt-1", "set": "slammast", "ok": true, "frames": 900, "seconds": 6.5,
  "own": { "id": "willy-proto", "title": "Willy Gorklingo: The Lag Protocol (prototype)" },
  "steps": [{ "name": "zip", "ok": true, "detail": "28 files" }, { "name": "video.picture", "ok": true, "detail": "first picture at frame 9" }],
  "shot": "iVBORw0KGgo…" }
```

The website's helper is `testRomOnDevice` in `frontend/packages/shared/src/rom-test.ts`. The same test runs from the command line: `device romtest ZIP` (see [device.md](device.md#rom-test)).

## Room telemetry

Guests send `client_report` on the control channel every 2 s (what their browser measured), and the host's linked browsers ask for a room's telemetry with `telemetry_runs`, `telemetry_series`, `telemetry_events`, `telemetry_incidents` and `telemetry_find`. Both are described in [telemetry.md](telemetry.md#protocol).

## Latency test

The host measures how long a press takes to show on the picture, for every player at once.

- **Switch:** `room_action` `input_hud_on` / `input_hud_off` from the host's linked browser (game rooms and `id: "test"`). It is a diagnostic: not saved, off again when the room or the device restarts. `room_state.input_hud: true` tells every guest it is on.
- **Drawing (`pkg/inputhud`):** while it is on, the device draws one cell per seat along the bottom of each frame (about a seventh of its height), before any encoder or tier: the seat's label, a small D-pad, six buttons, Coin and Start lit with that port's pad (`StreamService.PortPad`), and a **beacon**, a square that is white while the seat presses any button or a stick past half way and black otherwise. Everyone sees it, and a recording keeps it.
- **Where the beacons are:** `stream_stats.hud` is a list (P1 first) of `{x, y, w, h}` in fractions of the frame, so it holds at any size the video is sent or shown.
- **Measuring (browser):** the page notes the time of each press and release of the local player that holds its seat (`pressEdges` in `useHostStream`), watches the `<video>` with `requestVideoFrameCallback`, samples the middle of its beacon on a 4x4 canvas, and when the beacon shows that change takes `expectedDisplayTime` minus the press time (`LatencyMeter`, the last 20 measurements; more than 2 s is a missed frame, not latency). It covers the whole way there and back: input channel, device, encoder, network, jitter buffer, decoder and screen, but not the game's own reaction (the beacon shows what the device received, not what the game did with it).
- **Shown:** the controls panel's **Latency** card, for everyone: Network (the ICE pair's round trip), Video (jitter buffer plus decoding over the last second, from `getStats`), and End to end (the median, with the last one, the count and the worst). Green up to 80 ms end to end, yellow up to 150 ms, red above.

