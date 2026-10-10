# The device (host app)

`backend-device/` is the program the host runs on their own computer. It runs MAME through libretro, streams over WebRTC, manages the rooms, and shows the pairing code in a **native window** (Fyne) with a tray icon. With a window it opens no port at all. Without one (a server, a Raspberry Pi, Docker) it serves a local web panel on the LAN.

## Running it

```bash
cd backend-device
go build -o bin/device ./cmd/device                  # with the window (Fyne)
go build -tags headless -o bin/device ./cmd/device   # no GUI: servers, Raspberry Pi
./bin/device                                         # window + tray icon
./bin/device --headless                              # no window: log, CLI and the LAN panel
```

On macOS the release is `go-link.app`: open it like any app. From a terminal, use the binary inside it: `go-link.app/Contents/MacOS/go-link-device`.

| Flag | Description |
|---|---|
| `--config <path>` | Another `device.json` (useful to run several test devices) |
| `--server-signaling <url>` | Signaling server for this run (`wss://…/ws`) |
| `--headless` | No window or tray icon. Automatic on Linux without a display and in `-tags headless` builds. Opens the local web panel |
| `--panel <addr>` | Address of the local web panel when headless (default `:7373`, every LAN interface) |
| `--no-panel` | Do not open the web panel when headless |
| `--web-url <url>` | Website the window shows for linking, for this run (default `web_url` in `device.json`, else `https://play.go-link.org`) |
| `--game <set>` | Start the test room with this game, for example `--game robby` |
| `--core <path>` | libretro core to use (default `~/go-link/cores/mame2003_plus_libretro.<ext>`) |
| `--udp-port <port>` | Carry every WebRTC connection on this UDP port (see [networking.md](networking.md#direct-connections-one-udp-port)) |
| `--announce <ips>` | Comma-separated addresses where browsers reach `--udp-port` through a forwarding router |
| `--test-room=false` | Do not open the test pattern room |
| `--test-room-pause` | Let the host pause the test pattern room (the card holds and the tone stops), to try the pause and its requests without a game |
| `--debug` | Verbose logs |

Flags take one or two dashes (`-headless` or `--headless`).

### Which signaling server it uses

In this order:

1. `--server-signaling`, for this run only.
2. `signal_url` in `device.json`, permanently. The right choice for a host without a terminal who runs their own server.
3. The project's public server, built in: `wss://signal.go-link.org/ws`.

The URL must be `ws://` or `wss://`. With `ws://` (unencrypted) to anything but the local machine, the device warns in the log. **STUN and TURN are never configured on the device**: signalhub sends them in its first message (`hello`), and they live only in memory.

### How the pairing code stays valid

- On connecting, the device asks for a code (`register`).
- When a browser redeems it (`paired`), the code is spent, so it asks for a new one at once.
- Every 9 minutes it renews the code before it expires (after 10).
- If the connection drops, it hides the code and retries with exponential backoff and jitter (1 s, 2 s, 4 s… up to 30 s), so a thousand devices do not all reconnect at the same instant after a server restart.

## Command line

Everything the website does can also be done from a terminal, for a device without a screen (`device help` lists the commands):

```bash
device                                   # run the device
device core download                     # download the emulator core and its game list
device roms dir [PATH]                   # show or change the ROM folder
device roms check [--dir D] [--json]     # check which ROM sets the core can run
device roms saves [--json]               # test which games can resume from a save
device romtest [--frames N] [--json] [--shot FILE] ZIP   # power a set on with the exact core
device romtest --input FILE [--checkpoints LIST] [--frames-dir DIR] [--mp4 FILE] [--wav FILE] ZIP   # replay an input script on the exact core
device hdbench --far PICTURE [--play PICTURE] [--res 720p,1080p,2160p] [--encoder vp8|videotoolbox|x264]   # go-link HD: time an HD scene's encoder
device hdprobe --far PICTURE [--play PICTURE] [--json]   # go-link HD: what this computer streams at 60 fps
device thumbnails check [--json]         # count the thumbnails of the ROM sets
device thumbnails dir [PATH|default]     # show or change the thumbnails folder
device thumbnails kind [boxart|title|snap]   # which thumbnail is shown
device video quality [high|normal|saver]     # video quality of game rooms
device panel token [--new]               # show (or replace) the web panel token
device rec list [--json]                 # list the recordings of game rooms
device rec rm ID...|--all                # delete recordings
device telemetry rooms                   # rooms with telemetry (~/go-link/telemetry.db)
device telemetry runs ROOM               # the times a room was on, with their game ids
device telemetry events ROOM|#GAME [--warn] [--since 30m]   # a room's log
device telemetry incidents ROOM|#GAME [--json]             # freezes and their likely cause
device telemetry export ROOM|#GAME [--since 2h]            # everything, as JSON lines
device reset --yes                       # factory reset (with the device stopped)
```

Every subcommand accepts `--config PATH`. Settings changed from the CLI are saved in `device.json`; a running device picks them up on its next start.

### The ROM library

`LibraryService.Scan` reads the ROM folder (each zip's file list, checked against the core's game list by `pkg/romcheck`, plus its thumbnails); a full MAME 0.78 collection of about 5,000 sets takes a couple of seconds. Each scan builds an index in memory (`library_index.go`): the sets sorted four ways (title, size, year, state) and a lower-case search text per set. Browsers never get the whole list, which does not fit one WebRTC message: `device_status` carries `library.summary` (counts, size, the five biggest, a `revision` that changes with every scan), and the website asks for pages with `roms_query` (search, filter, sort, at most 100 sets) and for single sets with `roms_get` ([protocol](protocol.md)), each answered with one pass over the index. Sets imported from the website or the window are copied at once and scanned together, 750 ms after the last one (`scanSoon`), so dropping a hundred zips scans once. A control message that cannot be sent (for example one larger than the browser takes) is logged, at most once a minute (`control message not sent`).

### go-link's own sets

go-link makes its own games (the [CPS-1 ROM](rom/README.md), and later Willy Maker's). The stock core only runs sets of its driver list, so they are laid out as one of them (`slammast`) and, by name, look like that original game. The device ships a list of them, `backend-device/pkg/ownsets/sets.json` (embedded), with the **SHA-256 and size of every file inside the zip**: never the zip's own hash, which changes with its entry timestamps on every build. `rom/tools/ownsets.mjs` writes it, and `rom/tools/build.mjs` runs it after every build, so the list always matches the last build.

- A zip is a go-link set only when it has exactly the listed files, each with its size and SHA-256. A set that only matches by name (a real `slammast.zip`, or ours with one byte changed) stays the original game, as `romcheck` sees it. Hashes are kept per file path, size and modification time, so a rescan hashes nothing new.
- The library (the sets of `roms_page`) then shows go-link's `title`, `year`, `maker` and `description`, `own: true` and `controls` (`players`, `buttons`, `labels`: Jump, Fire, Special), and the picture go-link ships with the set for every thumbnail kind (or none): never the host's thumbnails for that name, which are the original game's. Rooms of it carry `room_state.info.own` and `room_state.controls.labels`.
- `device roms check` marks it `[go-link set, verified]` (`"own": true` with `--json`).
- Nothing is weakened: the list only renames, every set still goes through `romcheck`, and a go-link set runs exactly like any other.

### ROM test

`device romtest ZIP` is validation level 4 from a terminal: the same test as the website's **Test on my go-link** ([protocol](protocol.md#rom-test)). It copies the zip to `~/go-link/tmp/romtest/run-<random>/`, runs the two worker processes (`device romtest --child`, with and without scripted input) with the core in `~/go-link/cores` (`--core` for another one), prints the checklist and deletes the copy. It never reads or changes `device.json` or the ROM folder. `--json` prints the result, `--shot FILE` saves the last frame, and the exit status is 1 when a check fails. With `--input FILE` it replays an input script instead (experiment 1's harness, [docs/experiments/harness.md](experiments/harness.md#the-real-core)): the JSON script of `rom/tools/lab/run.mjs`, frame by frame on the exact core from power on, saving the `--checkpoints` frames (default: the script's) and every `--png-every` frame as PNG in `--frames-dir`, with `--mp4 FILE` every frame as a video (needs ffmpeg), and with `--wav FILE` the core's stereo sound (48 kHz). `device hdbench` and the device flag `--test-room-hd 720p|1080p|2160p` (with `--hd-far`, `--hd-play`, `--hd-kbps`, `--hd-threads`, and `--hd-codec h264 --hd-h264 x264|videotoolbox` for an H.264 track made by ffmpeg, which must be installed) belong to go-link HD's experiment ([docs/experiments/hd-streaming.md](experiments/hd-streaming.md)): the first times an HD test scene's encoder, the second streams the scene in the test room instead of the test card, and with `auto` picks the size and codec this computer keeps at 60 fps after trying them (`device hdprobe` prints that choice). **go-link HD** ([go-link-hd.md](go-link-hd.md)): game packages (`name.glhd`, `pkg/glhd` reads their manifest) in the ROM folder are listed next to the zips (`RomInfo.kind: "glhd"`, the manifest's title and players, buttons Jump and Run; a `.zip` of the same name wins) and their game rooms run go-link HD's engine instead of MAME (`LibraryService.CoreFor`), in the same `device emulate` worker with `--golinkhd` (the engine's own API through `pkg/golinkhd`, `GameCoreConfig.Native`; MAME keeps `pkg/libretro`) and `--upscale` (`--hd-room-size 720p` by default, ×2, or `1080p`, ×3; a game drawn at 720p or 1080p, format 3's `resolution`, is enlarged only up to the room's size, `hdUpscale`; before loading a package the worker asks the engine for the room's size, `Engine.SetResolution` (API 2, skipped by an older engine), so a package painted bigger is drawn at the room's size). HD rooms stream with their own codec and bitrate (`RoomsConfig.HDH264`, `HDKbps`): `--hd-room-codec auto` (the default) sends H.264 from the Mac's hardware on macOS (VideoToolbox called directly through cgo, `pkg/encoder/vt_darwin.go`: nothing to install, a keyframe as soon as a viewer joins) and VP8 elsewhere, `vp8` or `h264` (with `--hd-h264`'s encoder: `videotoolbox`, or `x264` through ffmpeg) force one; `--hd-room-kbps` is the bitrate at high quality (4000 at 720p, 8000 at 1080p by default; two thirds at normal, a half at saver). While an H.264 room records, its full picture is also encoded in VP8 for the WebM recording. Measured on an M1 with ANTÍDOTO at 1080p: VP8 at 3,500 kbps took 28–90 % of a core to encode, H.264 from VideoToolbox at 8,000 kbps 12–14 % plus 6 % for ffmpeg. The engine is its library, `libgolinkhd.dylib`, `.so` or `golinkhd.dll` (`glhd.LibraryFile`), **shipped with the device** since 0.2.8 (`go-link.app/Contents/Frameworks`, beside the program in the Linux and Windows archives and in the Docker image; `glhd.BundledLibrary`); `--hd-core PATH` names another one, and without a shipped one it is looked for in `~/go-link/cores`. `library.core.hd_installed` tells the website, and New game asks for the engine the chosen game needs. A library in the cores folder is checked like every core against the SHA-256 kept next to it on first use (a rebuilt library needs that `.sha256` file removed); the shipped one is signed with the app instead. Whenever the engine is installed, the library also lists its **built-in games**, `glhd_platformer` (go-link HD: Platformer) and `glhd_showcase` (go-link HD: Showcase, a scene for each effect: Mode 7, the road, lights, the sea, colors), which need no package (`glhd.Demos`; the worker gets `--rom golinkhd-demo:N` and calls `golinkhd_load_demo`); a file of the folder with the same name wins. Its save states are exact by design, so the room's save probe is skipped. It gets every button (L2, R2, L3 and R3 too) and both analog sticks, which MAME's core never gets. Its texts and dialogs are in the device's language (device.json `language`, else the computer's, else English; `emulate --language`). With `--hd-core PATH` and `--test-room-hd` with a size (not `auto`) the test room plays the engine's built-in demo: its 640 × 360 screen enlarged ×2, ×3 or ×6 with nearest neighbour (`libretro.ToI420Scaled`), and the seated players are its ports 1 to 4 (B jumps, Y runs, Start joins or pauses). `GOLINK_HD_LIB=PATH go test -run TestGoLinkHD ./internal/services/` checks a built library through it. It works in `~/go-link/tmp/romreplay-<pid>/` and deletes it.

```
ROM test of slammast.zip (slammast)
  ok    zip             28 files
  ok    set             the core's slammast (Saturday Night Slam Masters (World 930713) layout)
  ok    identity        go-link set "Willy Gorklingo: The Lag Protocol (prototype)", verified by the SHA-256 of its 28 files
  ok    core.loaded     MAME 2003-Plus 3141930, 384x224 at 60.00 Hz
  ok    core.files      every file found with its size; 28 differ from the original set, as expected for a go-link set
  ok    video.picture   first picture at frame 9
  ok    video.alive     220 different pictures in the last 300 frames (at least 10)
  ok    audio           sound runs, silent in these 900 frames
  ok    input.reacts    the picture changed at frame 301, after Coin at frame 300 (the run without input stayed the same until then)
  ok    time.realtime   900 frames in 2.8 s: 5.4x real time
PASSED: the set powers on in this core. 900 frames in 6.5 s.
```

A copy whose 68000 program was erased crashes the core: the worker dies (`core.run`: "the set crashed the emulator (SIGSEGV: segmentation violation)") or the screen stays black (`video.picture`), and the device itself is never touched. One test runs at a time per device.

## Configuration: `device.json`

Created on the first run with mode `0600`:

| System | Location |
|---|---|
| macOS | `~/Library/Application Support/go-link/device.json` |
| Windows | `%AppData%\go-link\device.json` |
| Linux | `~/.config/go-link/device.json` |

| Key | Meaning |
|---|---|
| `device_id` | Permanent UUID of this computer |
| `device_secret` | Random secret that binds `device_id` on signalhub. Never share it |
| `signal_url` | Signaling server (empty = the public one) |
| `web_url` | Website the window shows for linking (empty = `https://play.go-link.org`) |
| `roms_dir` | ROM folder (empty = `~/go-link/roms`) |
| `thumbnails` | `dir`, `kind` (`boxart`, `title`, `snap`) and `size` (list size in the window) |
| `udp_port`, `announce_ips` | Fixed WebRTC UDP port and the addresses to announce for it |
| `max_rooms` | Game rooms running at once (default 4, counting paused ones) |
| `rooms` | The saved game rooms and their state (with each room's `picture`, the host's default picture style for guests, see [protocol.md](protocol.md#room-picture-default)) |
| `test_room_picture` | The test pattern room's default picture style (`style`, `bands`; absent = the site's default) |
| `links` | Linked browsers: `id`, `token_hash` (SHA-256 of the token, never the token), `created_at`, `last_seen`, and the terms of use version accepted (`terms`, `terms_at`) |
| `panel_token` | UUID token of the local web panel (headless only) |
| `language` | Language of the window: `en`, `es` or `pt` (empty follows the computer) |
| `video_quality` | Video quality of game rooms: `high` (default: 2x at 3,500 kbps), `normal` (2x at 2,500 kbps) or `saver` (the game's own size with averaged color, 2,500 kbps). See [Video quality](#video-quality) |

- It is saved atomically (temporary file + rename), so a power cut never leaves it half written.
- STUN and TURN are **not** stored here.
- Other files live in `~/go-link/`: `cores/` (emulator core and game list), `roms/` (default ROM folder), `thumbnails/MAME/`, `saves/<room>/` (save states), `rec/<room>/` (recordings, 0600, see [Recordings](protocol.md#recordings)), `history.json` (0600), `tmp/romtest/` (sets waiting for or under a [ROM test](#rom-test), deleted after each test and at startup), `maker/` (the game Willy Maker sent last, `slammast.zip` and `game.json` with its name and buttons, played by the room of `@maker`; apart from the ROM folder, so a real `slammast.zip` there is never touched) and `logs/device.log`.

### Video quality

Game rooms send the game's picture **enlarged 2x** with nearest neighbour, so every game pixel keeps its own color through VP8's 4:2:0 color (one color sample per 2x2 block); the website averages each block back before drawing. The host picks how, in the window (Settings › Rooms), on the website (My device › Overview › Video quality), with `device video quality` or in `device.json`:

| Quality | Picture | VP8 target | Average measured |
|---|---|---|---|
| **High** (default) | 2x | 3,500 kbps | about 2.6 Mbps per room |
| **Normal** | 2x | 2,500 kbps | about 2.0 Mbps |
| **Saver** | the game's size, each 2x2 block's color averaged | 2,500 kbps | about 1.6 Mbps |

- A change from the window or the website reaches running rooms at once (the encoder restarts with a keyframe). The CLI only saves it: a running device uses it after its next start.
- The 2x picture is made in the worker in **one pass** with the RGB to I420 conversion (`libretro.ToI420Double`: each game pixel writes a 2x2 block of luma and its own chroma sample), bit for bit the same as upscaling first; Saver uses `libretro.ToI420Box`. On an Intel i9-10900 a 384x224 frame takes 0.55 ms at 2x, 0.60 ms for Saver and 0.49 ms for the old conversion (a separate upscale then conversion took 3.3 ms).
- **Automatic fallback:** when a 2x room starts streaming, the device measures its encoder for two seconds; if the 95th percentile of the encode time is more than 60 % of a frame's time, the room goes to Saver for the rest of its run, logs `the 2x picture does not fit this computer: the room goes to saver` and shows **Saver (CPU)** on the website. Choosing a quality again makes running rooms try 2x again.
- The test pattern room keeps its own 640x480 test card (it is drawn at that size).
- The contract for clients is in [protocol.md](protocol.md#video-scale).

### Factory reset

From My device on the website (`factory_reset`) or, with the device stopped, `device reset --yes`. It deletes what the host made on this device:

- every room (running ones stop without saving) and every saved game (`saves/`),
- the history of games and every recording (`history.json`, `rec/`),
- the linked browsers (they must link again with a new pairing code),
- the settings: ROM folder (back to `~/go-link/roms`), thumbnails, `max_rooms`, `web_url`, `language`, `test_room_picture` and `video_quality`.

It keeps what makes the device itself and reachable, so it can be linked again right away: `device_id`, `device_secret`, `signal_url`, `udp_port`, `announce_ips` and `panel_token`. It never deletes the host's own files: ROMs, thumbnails, the emulator core and the logs stay.

### Single instance

The device locks `device.lock` next to `device.json` (`flock` on macOS and Linux, `LockFileEx` on Windows). If another device holds it, the second one says so and exits. The system releases the lock when the process ends, even after a crash.

## Native window (Fyne)

`internal/gui` builds the window and the menu bar panel. **There is no web server behind it**: the window calls the same services (`StatusService`, `LibraryService`, `RoomsService`…) that the website reaches over WebRTC and the CLI calls directly.

- **Before any browser is linked**, the window shows only the pairing code and the website where to type it.
- **After that**, a sidebar with:

| Section | Content |
|---|---|
| **Overview** | The app's logo and what the device is doing: rooms (live, paused), people playing, linked browsers, CPU, memory, network, streaming, players, latency; a notice with a download button when a newer go-link is released; **Open go-link** (the rooms on the website), **Link another browser**, **Unlink all**; and **This computer**: system, processor, memory, device ID, version and website |
| **Emulators › MAME › ROMs** | Folder management only: emulator status and download, the ROM folder (choose, open), a drop zone (and a file picker) that copies `.zip` sets into it, and counters: sets, runs, will not run, added from the window, folder size and free space. Every game, its check and Play are on the website |
| **Emulators › MAME › Thumbnails** | How many sets have Boxart, Title and Snap, the list per game, how to name images. Dropping images saves them; dropping a folder with `Named_Boxarts`, `Named_Titles` and `Named_Snaps` saves each in its kind |
| **Settings** | **General**: the window's language (Automatic, English, Español, Português; kept in `device.json` as `language`). **Thumbnails**: which picture is shown and the folder. **Rooms**: the video quality of game rooms (High, Normal, Saver), `max_rooms` and where saves go. **Network**: the signaling server and the STUN/TURN received |

- **Menu bar panel:** a click on the tray icon opens a small panel with status, CPU, memory, network, streaming, players and browsers (and the code while nothing is linked). Right click opens the menu. On macOS the panel appears under the icon and closes when clicking outside (`panel_darwin.m`).
- Closing the window hides it; the device keeps running in the tray. **Quit** in the tray stops it.
- The UI repaints on every status change (`StatusService.OnChange`), always on the UI thread (`fyne.Do`). Fyne runs on the main goroutine, as macOS requires.
- The app and tray icon is the website's logo, drawn in code by `pkg/trayicon`.
- **Optional at build time:** `cmd/device/ui_gui.go` and `cmd/device/ui_headless.go` pick the UI. `-tags headless` links neither Fyne nor OpenGL.

## Local web panel (headless devices)

A device **without a window** (Raspberry Pi, server, Docker, or `--headless`) opens a **web panel** on the LAN: the same "My device" website, served by the device at `http://<device IP>:7373`. A device with a window opens no port.

- **Business logic stays 100% WebRTC, no REST API.** The port serves only the website files (the rooms' build, `play`, with no landing, guide or tools: embedded from `backend-device/web/panel/dist`, built with `make panel`) and a WebSocket at `/ws` that carries **only the WebRTC negotiation**, with signalhub-like messages (`hello`, `paired`, `signal`). Everything else travels afterwards over the WebRTC data channel, exactly as for a browser linked with a code. Any other HTTP method gets 405.
- **Panel token:** a UUID v4 created on the first run and kept in `device.json` (`panel_token`). `device panel token` shows it (never the log); `--new` replaces it (with the device stopped). The token **never travels over the network**: the browser asks for a challenge (`{"type":"panel"}` → `{"type":"panel_nonce","nonce":"…"}`) and answers `{"type":"panel","proof":"…"}`, an HMAC-SHA256 of the nonce with the token. Each nonce allows one try. The browser remembers the token (`go-link.panel-token`, never in the URL).
- The WebSocket only accepts pages of the same panel (`Origin` header), and 5 wrong proofs in a minute block that IP for 5 minutes.
- **The website knows it is the panel** because the device adds `<meta name="go-link-panel">` when serving `index.html`. The link with the device then goes over the local WebSocket, so it works on a LAN without internet.
- **Playing from the panel:** the public signaling server does not accept the panel's origin, so panel rooms also go through the device's WebSocket. The website opens a second socket in room mode (`{"type":"panel","mode":"room"}`, no data channel) and sends `join` with a `room_id`, `invite` or `code`, as on signalhub. Guests from the internet still join through the official website with their invitation.
- On the panel, `/` opens My device until a browser is linked, then the rooms. The landing page is not shown.

## Updates

A few seconds after starting, and every 6 hours, the device reads go-link's public releases list on GitHub. When a newer version than its own is out, the window's Overview and the linked website show it with a link to the release page. Nothing is downloaded or installed by itself, and development builds (without a version tag) never check.

## Players, names and pausing

Each room's **Room Manager** (`internal/services/room_manager.go`) is an actor: one goroutine owns the seats, the queue, the chat and the requests for a pause, and timers post back to it (a request for a pause expires after 30 s inside that goroutine). It enforces the rules itself, never trusting a browser:

- **Names:** `CleanName` keeps letters, digits and single spaces (NFC first), cuts to 20 characters and falls back to the generated `Guest XXXX` below 2; the guest learns the name in use from `room_state.you.name` ([protocol](protocol.md#player-names)).
- **Pausing:** only the host pauses. The PIN gate reports which guests came in with the owner key (`PinResult.Owner`), and the linked browser itself is trusted too; `TestRoomService` marks both with `MarkOwner`. Others ask with `pause_request`; the host answers in the room or from a linked browser (`RoomsService.AnswerPause`, `pause_answer`). `host_online` comes from the owners in the room plus `SetHostLinked`, which `cmd/device` updates whenever a proven linked browser's control channel opens or closes (`StreamService.HasLink`, `LinkService.OnChange`). Linked browsers hear about requests through `RoomsConfig.OnPauseAsk` (`pause_asked`, `pause_ask_gone`) and see them in `device_status` ([protocol](protocol.md#pausing-is-the-hosts)).
- **Raised hands:** the Room Manager keeps each spectator's hand (`member.hand`, one raise per `handEvery`, a minute) and whether the host made them a spectator (`held`, set by `release_seat` `watch`); `promote` (queue, or play at a port: the seat's player goes to the head of the queue), `hand_later` and `remove_guest` come only from owner peers, and only owners get peer ids in `room_state`. `remove_guest` deletes the member and calls `OnRemove`, which `TestRoomService.Remove` answers: it ends the guest's return token (`PinGate.Revoke`; later tries get `removed`), tells the guest (`pin_result` `removed`) and stops its stream ([protocol](protocol.md#raised-hands)).

## Metrics

Every 2 seconds the device measures CPU and RAM of the machine and of go-link itself, including its emulator processes (`pkg/sysinfo`, gopsutil), plus machine-wide network traffic. The data lives in `StatusService`, which the window reads and linked browsers receive over WebRTC. It also measures peer-to-peer latency to each linked browser with a ping over the data channel.

## Docker

The `go-link-device` image (`backend-device/build/docker.Dockerfile`, `make device-docker`) is the headless binary with the panel on port 7373. Each release publishes it for amd64 and arm64 on the GitHub Container Registry as `ghcr.io/lordbasex/go-link-device:vX.Y.Z` and `:latest` (`.github/workflows/docker.yml`, on every published release or by hand with a tag), so `docker run … ghcr.io/lordbasex/go-link-device:latest` is enough; the release also carries it as an OCI archive for machines without internet (`docker load`, it loads as `go-link-device:vX.Y.Z`). Everything it keeps lives in the `/data` volume (configuration, core, saves, history). The core downloads on first use and is never in the image.

ROMs and thumbnails are **volumes you mount from outside**:

| Inside the container | What goes there | Example |
|---|---|---|
| `/data/go-link/roms` | MAME 2003-Plus (0.78) sets | `-v ~/roms:/data/go-link/roms` (add `:ro` to block uploads from the website) |
| `/data/go-link/thumbnails/MAME` | Thumbnails in the libretro layout (`Named_Boxarts`, `Named_Titles`, `Named_Snaps`) | `-v ~/thumbnails/MAME:/data/go-link/thumbnails/MAME:ro` |

The image creates those folders owned by the device user (`golink`, uid 1000), so a new volume is not owned by root. Host folders mounted without `:ro` must be writable by uid 1000.

```bash
cd backend-device && docker compose up -d --build     # the example docker-compose.yml
docker compose exec device go-link-device panel token --config /data/device.json
```

- **Network:** WebRTC needs direct UDP to the players. On Linux the simple way is `network_mode: host`.
- **Without host networking** (Docker Desktop): publish the panel and a fixed UDP port, and announce your LAN address: `docker run -p 7373:7373 -p 50000:50000/udp -v go-link:/data go-link-device --udp-port 50000 --announce 192.168.1.20`.
- **One-off commands:** the `ENTRYPOINT` already runs the device (`--headless --config /data/device.json`), so override it: `docker run --rm --entrypoint go-link-device -v go-link:/data go-link-device core download --config /data/device.json`. With the container running, `docker compose exec device go-link-device …` is enough.

## Logs

Started without a terminal (the macOS app from the Finder, the Windows `.exe`, which opens no console), the log goes to `~/go-link/logs/device.log` (0600; at 5 MB it starts over and keeps the previous one as `.1`). From a terminal it prints there.

## Tests

```bash
go test -race ./...
go test -race -run TestName ./internal/services/
```

On macOS the linker may print `ld: warning: ... malformed LC_DYSYMTAB`. It comes from cgo and is harmless.
