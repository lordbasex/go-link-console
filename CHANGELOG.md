# Changelog

All notable changes to go-link. Newest first. Willy Maker, which has its own site (maker.go-link.org), keeps its own: [frontend/willy-maker/CHANGELOG.md](frontend/willy-maker/CHANGELOG.md).

## [Unreleased]

### Changed (core patches)

- **X-Men resumes exactly, and 13 more games resume better:** the missing state was the input ports' values latched at VBLANK (`0019`, generic: Snow Bros., Superman, Puzzle Bobble, Marvel Super Heroes, Slam Masters and others now resume with the same picture), the register each YM2151 writes to (`0020`) and X-Men's EEPROM count (`0021`); with `0015`, X-Men gives 600/600 identical pictures and sound frames. Sent to libretro as #2053 and #2054.

## [0.3.1] - 2026-10-10

### Changed (go-link HD)

- **The engine shipped with the device is go-link HD's newest:** lives and a continue countdown per player (game over only when nobody is left), a moment of invulnerability after a hit, acts that share a level's art, longer levels (up to 1,792 cells and 224 enemies), four more enemy kinds (roller, hopper, puffer, splitter), bosses that evolve into a second form after they are beaten once, and credits rolling after the ending. Game packages that use them need this version.

### Added (device)

- **go-link HD rooms in H.264 from the Mac's hardware:** `--hd-room-codec auto` (the default) sends HD game rooms on macOS in H.264 from VideoToolbox, called directly (nothing to install: no ffmpeg; `pkg/encoder` vt_darwin.go, a keyframe as soon as a viewer joins), and VP8 elsewhere, at their own bitrate (`--hd-room-kbps`, 4,000 kbps at 720p and 8,000 at 1080p by default); recordings keep working (VP8 for the WebM). On an M1 at 1080p the encoding went from 28–90 % of a core to 12–14 %, with more than twice the bitrate.
- **go-link HD games drawn at the room's size:** the device asks the engine for the room's size (`golinkhd_set_resolution`, go-link HD's API 2; `Engine.SetResolution`), so one package painted for 1080p plays at 1080p in a 1080p room and at 720p in a 720p room, its pictures made smaller by the engine as it loads; an older engine keeps the package's size.

### Fixed

- **Release: the macOS app carries its own notarization ticket.** Only the `.dmg` was stapled, so go-link.app copied to /Applications had to ask Apple's servers the first time it opened (and failed offline). `scripts/release.sh` now notarizes and staples the app first, then makes the `.dmg` with it and notarizes that (`notarize.sh` takes a `.app` or a `.dmg`).

- **CI:** gitleaks runs from GitHub's registry (`ghcr.io/gitleaks/gitleaks`): Docker Hub refused the image past its unauthenticated pull limit and failed the secrets check.
- **go-link HD games drawn in HD:** a game made for 720p or 1080p (format 3's `resolution`) streams at its own size instead of being enlarged again (a 720p game in a 1080p room was 2560 x 1440); a 360p game is still enlarged 2 or 3 times.

### Added (iOS)

- **go-link Player on TestFlight:** the iPhone and iPad app is signed with the developer account and uploaded to App Store Connect: `make testflight` (archive, sign, upload, a new build number each time) and `make install-device` (signed and installed on the connected iPhone), both with an App Store Connect API key from the environment, and `scripts/asc.py` for the TestFlight groups and beta review. The app now has its privacy manifest (no tracking, no data collected) and Universal Links (`go-link.org/g/...`) are on.

## [0.3.0] - 2026-10-09

### Changed (device)

- **Signed and notarized by Apple:** the macOS app is signed with a Developer ID (with the hardened runtime) and the disk image is notarized and stapled, so macOS opens go-link like any other app, without the "Open Anyway" step. The release is no longer a pre-release.

### Fixed (device)

- **The macOS app shows its own version:** Finder's Get Info showed the version before (the release's tag did not exist yet while it built); a release build now takes its version number.

## [0.2.9] - 2026-10-09

### Fixed (device and website)

- **A large ROM folder loads:** with thousands of sets (a full MAME 0.78 collection is about 5,000) My device › ROMs stayed on its loading cards with "0 of 0", because the device sent the whole library in every `device_status` and a WebRTC message carries at most 256 KiB in Chrome. Now `device_status` carries only the library in numbers (`library.summary`), and the website asks the device for pages of 60 sets (`roms_query`: the device searches, filters and sorts an index built once per scan) and for single sets by name (`roms_get`), loading more as the list scrolls. New game searches the device's whole library the same way, and lists the sets that will not run a page at a time. Sets dropped together are scanned once, and a control message the device cannot send is now logged. A device before 0.2.9 keeps working with the new website, which pages its whole list itself.

### Added (device)

- **go-link HD packages of format 3:** the device lists and plays games with their own physics, sprites, painted layers, floor textures, screens (title, a level intro skipped by holding jump, ending), music and sounds (go-link HD 0.2.0's package format 3).
- **go-link HD games of several levels:** packages with format 3's `levels` (one level file each) are listed and played.
- **go-link HD games name their buttons:** a package with a weapon shows Fire and Special (its super) on the buttons that do them, in the game list and the room's controls, instead of Run.
- **go-link HD games speak the device's language:** their texts and dialogs show in the device's language (Settings › General, else the computer's), in English, Spanish or Portuguese; until now they were always in English.

### Added

- **Roadmap** (`docs/status.md`): save states across the whole catalog (a batch `savecheck` and a source scan, fixes by impact) and FinalBurn Neo as a second core.

### Changed (core patches)

- **Save states across the catalog, first tools:** `cores/mame2003-plus/tools/statescan` reads the core's source and lists, per game and per part of its board, the state variables that nothing saves, ranked by how many of the 5372 games use each part; `tools/savecheck/batch.sh` runs `savecheck` over a whole ROM folder. Their first run (33 sets that save) found that no timer is in the core's save states, which touches every game, and which games still miss a part of their board.
- **`cores/mame2003-plus/tools/savecheck`:** a small C program (any libretro core, no go-link needed) that plays a game twice with the same buttons, once without stopping and once loading a state into a fresh board, and counts the identical frames of picture and sound. It is the evidence for the core pull requests, and it was offered to libretro as a tool of its own (libretro/mame2003-plus-libretro#2047).
- **More fixes are upstream** (libretro/mame2003-plus-libretro): the Namco wave sound chip (`0009`, Pac-Man's sound after loading, #2044) and the YM3812 (`0010`, Snow Bros.' sound after loading, #2045) were merged on 2026-10-08; the Konami CPU (`0001`, The Simpsons and Aliens resume, #2046), the interrupt event queue (`0011`, #2048, the first with `savecheck` as evidence) and the sound latches, each CPU's local time and the mixer (`0008`, `0012`, `0013`, #2049) on 2026-10-09, and the same day the boards sent with `savecheck` evidence: The Simpsons (`0004`, #2050), Aliens and TMNT (`0007`, #2051) and Galaga (`0016`-`0018`, #2052). `cores/mame2003-plus` builds on that commit (`e05caba`) and keeps one patch, the K054539 (`0015`), which waits until X-Men's other missing state is found.

### Docs

- **go-link HD's phase 9, the AI kit** ([docs/go-link-hd.md](docs/go-link-hd.md#phases)): the API reference in one document, the scripting API's type file, examples and templates, and a command-line loop (pack, check, play with screenshots, bench, hash gates) so an AI can make a whole game with the SDK alone; its test is an AI building a 2D mining and building game from the kit.

## [0.2.8] - 2026-10-08

### Added (device)

- **go-link HD comes with the device:** its engine (the public repository [golink-hd](https://github.com/lordbasex/golink-hd), MIT) ships inside go-link.app (universal), the Linux and Windows archives and the Docker image, so `.glhd` games play with nothing to install (`--hd-core` still names another library, and one in the cores folder is used when none is shipped). With the engine, My games lists its two **built-in games**: **go-link HD: Platformer** and **go-link HD: Showcase** (a scene for each effect: a kart on Mode 7, the road, a cave with lights, the sea, colors), which open game rooms like any game. The build needs the engine's clone next to this repository (`GOLINK_HD_DIR`), and `THIRD_PARTY_NOTICES.md` carries its license.

### Docs

- **go-link HD's phases 7 and 8 and its SDK** ([docs/go-link-hd.md](docs/go-link-hd.md#phases)): API v2 with system services (achievements, saved progress, game events for the room, a view per player, rumble), a draw list run by a software or GPU backend, and the parts that make go-link HD's open SDK.
- **go-link HD's phase 6, code** ([docs/go-link-hd.md](docs/go-link-hd.md#phases)): games with their own scripts run by the engine as a sandboxed WebAssembly module, planned in Willy Maker's [code.md](docs/willy-maker/code.md).

### Added

- **A demo video** (`video/`, `npm run demo`): about three minutes in seven chapters (your arcade, linking the computer, opening a room, inviting friends, the phone as a console, Willy Maker and the network report) from takes of the real app, with the trailer's narrator, burned-in subtitles and a SubRip file. `docs/building.md` explains how the trailer and the demo are made.

### Changed (core patches)

- **Two more fixes are upstream** (libretro/mame2003-plus-libretro, both merged 2026-10-08): the YM2151 timers (#2042) and the QSound games' sound after loading (`0002`, `0003`, `0005`, #2043). `cores/mame2003-plus` builds on that commit and keeps thirteen patches. The Namco wave sound chip (`0009`, Pac-Man's sound after loading) went upstream as #2044.

### Fixed (device)

- **One chat line when the host makes a player a spectator:** "Fede moved Bob from P2 to watching" no longer comes with a second "Bob left P2".

## [0.2.7] - 2026-10-08

### Changed (website)

- **The network report shows each player's video quality** (a chart of the bitrate each one's video is sent at, stepping down and back up) and the share of each second the host spent encoding (in the host's CPU chart).
- **A shorter dock and the keys in sight:** the dock beside the video keeps what you use while playing: full screen (now first), the microphone with its level and a small chevron for the volumes and devices, pause, controls, sound and screenshot (six buttons instead of eleven, so they all fit a small window). **Record** moved to the room's header, next to Invite (host only), and **Picture** to the header's "…" menu while a game streams. The start buttons (1P, 2P…) left the dock: a player's menu in the players capsule has **Press Start**, and the keyboard (1 to 4), the gamepad's Start and the touch pad still press them. The side panel opens on **Controls** while a game streams, so someone new sees the keys at once; the tab chosen last stays for the browser tab. Host and guests alike; How to play explains the changes.

### Added

- **The host frees a seat:** in the players capsule the host sends a player (itself included) to the end of the queue, so the next in line plays, or to watching; with nobody waiting only watching is offered. The device accepts it only from the host's browsers (`release_seat`), tells everyone in the chat ("Fede sent Bob from P2 to the queue") and keeps it in the room telemetry.

### Changed (device)

- **A busy computer drops the room to saver mid-game:** the device measures, every second, how much of it goes into encoding (every tier together). When that stays at 75 % or more for five seconds in a row, a 2x room goes to its saver quality (the game's own size), as the encoder check already did when a room starts; a room already at 1x only logs that the computer cannot keep up. At most once a minute; each time is a `host_overloaded` telemetry event, and room samples carry `enc_load_pct`.
- **Each player's video quality adapts to their own connection:** every room now gives each guest their own video track (the tiers go-link HD used), with two more steps under the full picture at half and a quarter of its bitrate. Lost packets (3 % or more) or a freeze in a player's reports step that player down at once; 20 clean seconds step them back up, one step at a time. The others keep their full picture, and a step costs an encoder only while someone is on it. Recordings and the icon drawn while recording come from the full picture. Each step is a `video_quality` telemetry event with its reason, and peer samples carry `video_kbps_target`. With a simulated 8 % loss on one guest, that guest went from 2500 to 1250 and 625 kbps in four seconds while the host stayed at full quality with no loss.
- **The game's sound holds up better on lossy connections:** once a second the Opus encoder hears the worst packet loss among the players and, when there is any, turns on in-band FEC and leans less on the previous frame, so one lost packet hurts fewer of the ones after it (the browser hides the gap). With no loss nothing changes.

### Added (device)

- **A loss simulator for tests:** `GOLINK_SIM_LOSS=<percent>` makes the device drop that share of the packets it sends to every connection but the first one that gets video (the host, who comes in first), after the retransmission buffer, as a real network would. Never set it on a real device.

### Added (website)

- **Zoom, enlarge and pictures in the network report:** drag across any chart to zoom every chart into that span (down to 2-second steps; the zoom stays in the address to share it), double-click or **Show everything** to go back. **Enlarge** opens a chart in a dialog with **Save PNG** and **Copy image**, and the picture says what it shows: the chart, the room, the game id, the span and the legend. A new chart shows each player's **video wait** (the playout delay climbing on trouble and coming back down).

### Changed (website)

- **The network report reads better:** the three longest freezes first with **Show all**, freezes marked under each chart instead of bands across it, axes in k and M (100k instead of 100000) with more time marks, and two participants with the same name told apart by the time they joined ("Fede" and "Fede (12:58)").
- **Guests get the same room as the host:** `/g/<invite>` now has the compact header and the CHAT | CONTROLS panel of `/r/` (the header only made room for them on `/r/`).

### Fixed (device)

- **Loss is read from the video's own report:** browsers bundle their RTCP receiver reports on every stream (video, game sound, voices) and the device kept whichever came last, so a player's video loss could read 90 % when it was 8 %. It now keeps the report on that player's video stream (its SSRC), for the latency card, the network report's verdicts and the quality steps.

### Fixed (website)

- **The host's room opens cleanly:** it no longer shows "Room not found" and then the PIN form for a moment before the game. The page waited for nothing: it joined by the room id before its device said which invitation opens the room, and showed the PIN form while it answered it with the host's key. It now waits for the device's status (8 s at most) and shows the PIN form only after what the browser already has was tried; a guest without a return token still gets the PIN form at once.

## [0.2.6] - 2026-10-07

### Changed (device)

- **The video no longer gets slower the longer you play:** the device now tells each browser how long it may hold the game's video before showing it (the `playout-delay` RTP extension, as cloud gaming does). Chrome used to size that wait itself and almost never lower it: 8 ms when a room started, 79 ms two hours later, with no freeze. Each participant starts at 0 ms (Chrome shows each frame about 0.5 ms after decoding it); a freeze, lost packets or uneven arrival in their reports raises their own maximum one step (40, 80, 160 ms), and 30 clean seconds bring it back down. Each change is in the room telemetry with its reason. Measured on the host's Mac: the video buffer stayed at 0.6 ms for 30 minutes (it used to reach 56 ms by then) and the end-to-end latency was 37 ms instead of 98-147 ms; a guest over WiFi whose connection stalled for 10 s climbed to 160 ms and was back at 0 ms 90 s later, while the other player never moved.

### Fixed (device)

- **A hidden tab's freezes are not counted:** the browser stops drawing a tab that is not shown, so those freezes were not real; the network report and its verdicts leave them out.

### Changed (go-link HD)

- **go-link HD has its own API, not libretro's:** the engine is a library (`libgolinkhd`) with a small versioned API; the device runs it from Go through `pkg/golinkhd` (`device emulate --golinkhd`, `GameCoreConfig.Native`), next to MAME, which keeps running through libretro. go-link HD is meant to become go-link's main engine. libretro's API is credited as its inspiration. The `--all-buttons` worker flag is gone: the engine's own API carries every button and both sticks.

- **The engine's effects (phase 4, part A):** go-link HD's core has 4:3 and vertical screens, up to 8 players, blend modes, outlines and shadows, sprites turned and scaled, Mode 7, a pseudo 3D road, skeletal animation, color grading, bloom, blur, waves, 2D lights, the camera's zoom, dialog boxes with accents, low pass and echo, A*, package format 2 with a level's effects and a showcase demo (in its own repository, `golink-hd`). The device gives its core L2, R2, L3, R3 and both analog sticks (`--all-buttons`), never to MAME's.

- **The engine is C99, in its own repository:** go-link HD's core is written in C99 instead of Go, in `golink-hd`, so it can be published to libretro; the device calls it from Go through `pkg/libretro`. Phase 1's core and its demo are done, and it reads game packages (`.glhd` format 1); status and phases updated.
- **go-link HD games in the library and in game rooms:** `.glhd` packages in the ROM folder are listed next to the zips with their manifest's title, players and buttons (`RomInfo.kind`, `pkg/glhd`), and their rooms run go-link HD's core in the `device emulate` worker (`--upscale`; `--hd-room-size 720p|1080p`; `--hd-core PATH` or `golink_hd_libretro` in the cores folder; `library.core.hd_installed`). The website shows each game's file (`romFile`), the controls of a package and asks for the core the chosen game needs. `e2e/tests/hd-core.spec.ts` (on demand) plays the demo in the test room and an exported Willy Maker game in its own room.
- **The device plays go-link HD:** `--test-room-hd 720p|1080p|2160p --hd-core PATH` makes the test room play the core's built-in demo, its 640 × 360 screen enlarged ×2, ×3 or ×6 (`libretro.ToI420Scaled`, bit for bit the same as enlarging first), seated players at its ports; `pkg/libretro` starts a core with no content when given an empty path; a test runs the real core when `GOLINK_HD_CORE` is set.

## [0.2.5] - 2026-10-07

### Changed (emulator)

- **The YM2151 fix is upstream:** libretro merged our pull request [#2041](https://github.com/libretro/mame2003-plus-libretro/pull/2041) (2026-10-07), so the official core plays FM instruments right after loading a save. `cores/mame2003-plus` now builds on that commit without `0014`, and `0006` was refreshed against it.

### Fixed (device)

- **No more freeze every 9 minutes:** the picture stopped for up to 3 s every 546 s, each time ~130 ms shorter, with no lost packets. Every 32768 frames the VP8 PictureID wrapped to 0 and Pion's payloader left it out of that frame, so the browser dropped the picture until the next keyframe (a keyframe every 180 frames, and 32768 = 182 × 180 + 8, hence the shorter freeze each time). The device now uses its own VP8 payloader with a 15-bit PictureID on every frame. Found in the room telemetry, predicted to the second in a new room (13:32:21, 13:41:27, 13:50:33) and gone with the fix: 0 freezes in a new room on the host's Mac for two hours (443,928 frames, 13 wraps).
- **The CLI takes flags after its arguments:** `device telemetry export '#f7d0' --since 3h` stopped with the usage message, because Go's flag package stops reading at the first argument; every command now reads flags anywhere (`--` still ends them). `--since` also narrows a game id, not only a room (it used to export the whole run).
- **A room that comes back paused shows its game**, not a black screen: it is paused as soon as its game is ready, before the first frame, so the device had no picture to repeat. The emulator now runs that one frame first (found in the room telemetry: a paused Street Fighter II room made no frame in an hour and a half).
- **telemetry.db is private** (0600, with its -wal and -shm), like history.json: it keeps the participants' names and addresses.

## [0.2.4] - 2026-10-07

### Added (website)

- **Network report** in My device › History (`/device/history/<room>/network`): the freezes with their likely cause, nine charts (latency, lost packets, frozen picture, frames, controls, jitter, the host's CPU and upload, voice) from the room's first start or one game, live while it runs, and the raw event log with Copy log. The history has a **Game ID** column (a click copies it, and the search finds it). Each browser in a room reports what it measured every 2 s. The privacy policy lists the network report among what the host's computer keeps; the terms are now version 2026-10-07 (website, Android and iOS apps), so everyone accepts them again once.
- **Latency in the controls panel, for everyone:** Network (round trip to the device), Video (jitter buffer plus decoding) and End to end, green, yellow or red. The host's **Controls on the video** switch starts the latency test: each player's browser watches its seat's light on the picture and measures every press and release from the hand to the screen (median of the last 20, the last one and the worst).

### Added (device)

- **Room telemetry** (`internal/telemetry`, `~/go-link/telemetry.db`, SQLite in pure Go): every run of every room with its game id, a sample per second of the room (frames from the game and the longest pause between them, encoding, CPU, the computer's upload) and of each participant (input packets and silences, RTCP loss and jitter, keyframe and retransmission requests, voice packets, round trips), the browsers' `client_report` every 2 s, and a raw event log (connections, seats, names, frame gaps, losses, freezes, signaling). Kept from the room's first start with no age limit, deleted with the room or by a factory reset. Incidents group the freezes and give each a verdict (the host's computer, its internet, or one player). Linked browsers ask with `telemetry_runs`, `telemetry_series`, `telemetry_events`, `telemetry_incidents` and `telemetry_find`; the CLI has `device telemetry rooms|runs|events|incidents|export`. A history entry's id is now its game id.
- **Latency test:** `room_action` `input_hud_on` / `input_hud_off` (game rooms and the test pattern room) draws one cell per seat under the game, before any encoder: a small pad lit with what the device received and a beacon that is white while that seat presses anything (`pkg/inputhud`); `stream_stats.hud` says where each beacon is and `room_state.input_hud` that it is on. Not saved: a diagnostic.

## [0.2.3] - 2026-10-07

### Changed (website)

- **One side panel in a room, two tabs on top: CHAT and CONTROLS** (the latter while a game streams), with a close button and the same width on both. Chat holds everything the side panel had: your place, your name, and the Chat, Queue and Spectators tabs with the message field; Controls holds the keyboard and gamepads. The header's chat button opens it on Chat and the dock's controller button on Controls (pressing again closes it); the separate controls panel is gone. Unread messages are counted on the chat button while another tab shows.
- **The keys a game uses have an orange border** on the keyboard map: the stick, Coin and Start, the buttons up to the game's count and the players' starts up to its players (from the game's control panel), with a note under the keyboard.
- **One player per browser by default:** the keyboard and every gamepad drive the same player, so Coin (5) and Start work from any of them and a gamepad plugged in by mistake never takes a new seat. The host gets a **Several controllers** switch at the top of Controls (for the browser tab's session) that gives each gamepad its own seat and brings back "Plays as" and "Keyboard plays as".
- **The header's "…" is a menu** (in a room and on phones): the linked device, the language (its three choices open in place), the signaling server, the theme and GitHub, each a line of the list. In a room the device and the language live only there.

### Changed (device)

- **A guest takes one seat however many controllers that browser has:** only the host's own browsers seat several local players (`RoomManager.seatable`); the rest of a guest's local players never reach the game.

## [0.2.2] - 2026-10-06

### Fixed (device)

- **A room has the seats its game has.** Every game room had 4 seats, so a two-player game such as Street Fighter II said "1 of 4 playing" and offered P3 and P4, which the game never reads. The room now has as many seats as the game has players (from mame2003-plus's list of games, the same number that already chose the 1P-2P start buttons): two in Street Fighter II, four in Teenage Mutant Ninja Turtles, four when the game does not say. A third person waits in the queue. The rooms list and cards show the game's seats too, even before anyone joins and for rooms that are off.
- **A paused room came back stuck.** A room paused when go-link stopped was paused again before its game had loaded, which was ignored: the room said Paused while the game ran, and Resume did nothing. It now pauses once the game is ready, and Resume always brings a paused room back.

### Fixed (website)

- **A narrow window looked broken in a room:** with the video and the chat stacked (under 1100 px wide), the chat's panel sat over the video. Now it looks like the phone app: the video on top at the full width and its own shape, and the chat under it filling the rest of the window with its own scroll.
- **Tooltips at the header's right edge were cut** (Leave this room, Close game): the header's tooltips now grow to the left from their button.
- **The chat always shows the newest message.** It followed new lines by counting them, and the chat keeps the last 200 lines (seat changes included): once a long session reached 200, new messages stopped scrolling into view. It also stayed put when the chat changed height. Now it follows every new line and every change of height while you are at the bottom, and leaves you where you are if you scrolled up to read older lines (in the room's side panel and in the phone console's drawer).

### Changed (website)

- **The controls open beside the video:** on a computer, the dock's controller button opens the keyboard and gamepads in the chat's place, as a side panel (with a close button), and the video keeps its whole height; closing it brings the chat back, with any new messages counted on the chat button meanwhile. The keyboard or the gamepads sit centered in the panel and fit its width (smaller keys, the arrows under it); in a short window they start at the top and scroll. Before, the panel opened under the video and made it smaller. The gamepad's Plays as list showed only a huge arrow; fixed.
- **A cleaner room header:** in a room, the top bar is one row: the logo, the room (its picture, its title and how many are playing), the room's own buttons (chat, How to play, Invite, Close game, Leave), your device, the language and a "…" with the signaling server, the theme and GitHub. The section tabs are gone there, and the room's title and buttons no longer take rows of their own, so the video gets more height: the video and the chat now fill the window side by side, equally tall, with the picture (and its frame, if you chose one) centered in the video.
- **Language is one button:** the three EN/ES/PT bubbles became one button with the current language and a menu to pick another, on every page.

### Docs

- **go-link HD's base** ([docs/go-link-hd.md](docs/go-link-hd.md)): what go-link's own 2D engine will support, each item marked Base, Later or Idea (simulation, picture, camera, sound, controls, rules, saves, game package, go-link and Willy Maker), with budgets, phases and the decisions taken: a libretro core of our own in Go, also built to WebAssembly, pixel art on a 640 × 360 screen first, the platformer first. Nothing is built yet.

## [0.2.1] - 2026-10-05

### Fixed (apps)

- **iOS: the room's drawer fits the screen with a skin.** The skin's shell covers the whole screen, and it made the room taller than the screen, so the drawer (chat, players, you), Game settings and the name step sat about 60 points below the bottom: on an iPhone 17 Pro Max the chat's text field was off screen. Found in a live four-player test (iPhone, Android and two browsers on the public signaling server).
- **Android: chatting with the phone held sideways.** With the keyboard open in landscape, the chat sheet kept its handle, its tabs and 85 % of the screen, and the text field was squeezed out of sight; now it keeps only the messages and the field, at the full height left.

### Core patches

- **The first patch went upstream**: `0014` (the YM2151's load code gave every FM channel the wrong algorithm) is proposed to mame2003-plus as [libretro/mame2003-plus-libretro#2041](https://github.com/libretro/mame2003-plus-libretro/pull/2041). The others follow one by one if it is accepted.

## [0.2.0] - 2026-10-05

### Changed (apps)

- **Skins look rounded on every phone**: outside the rim's rounded corners the shell is black, so on a screen with square corners (iPhone SE, many Android phones) the plastic no longer shows in the corners.
- **Skins are checked on every phone**: one list of screens, `docs/skins/screens.json`, for the skin editor and both apps' paste check: every iPhone from the SE and the 11 to the 17 Pro Max and the Air (one entry per screen size), an iPad, Android phones from 360 × 640 to 461 × 998 dp, a foldable and a tablet. The editor's phone menu shows them all and its checks run on each; the built-in skins pass on all of them.
- **Test controller wears your gamepad skin** (iOS and Android): the same shell, D-pad, buttons, Coin and Start as in a room (Smoke until you pick another), with the test card in the screen, the latency and screen readouts over it, and the buttons only a controller has (L2, R2, L3, R3, Home) as lamps in the skin's menu capsule.

### Changed (website)

- **The website is two sites.** go-link.org keeps the landing, the user guide, the tools and the legal pages; the rooms, invitations and My device move to **play.go-link.org**, which is also what a headless device's local panel serves (lighter: no landing, guide or tools inside the binary). The link to your go-link now lives only on play.go-link.org, and go-link.org never connects to the signaling server. Old addresses keep working: go-link.org/rooms, /device, /r/<id> and /g/<invite> take you to the same page on play, and invitations are still shared as go-link.org/g/<invite>, which the Player apps open. A browser linked before the split is offered, once, to bring its link, room passes and settings to play.go-link.org with one click (`/handoff`); go-link.org then forgets the link. Both are built from `apps/web` (`VITE_ROLE=site|play`, `make web-build` builds both). The device's window now points to play.go-link.org/device by default (`web_url`), and Willy Maker reaches your go-link through play.go-link.org.
- **Willy Maker moved to its own site, maker.go-link.org** ([its changelog](frontend/willy-maker/CHANGELOG.md)). `/tools/willy-maker` (and a game's link) now takes you there, offering to bring the games made here. The new **`/maker-bridge`** page is how Willy Maker reaches your linked go-link: a tab Willy Maker opens, which accepts messages only from maker.go-link.org and passes on only a ROM test and a Willy Maker game's room (`packages/shared/src/maker-bridge.ts`). The picture (canvas, renderer, settings) and the controller models moved to a new package, `@go-link/ui`, used by both sites.

### Added (core patches)

- **Saves that resume exactly** (`cores/mame2003-plus`, optional, not used by releases): eight more patches (`0011`-`0018`). The main one: a save made at the end of a frame left the VBLANK interrupt in MAME's interrupt queue, which was not saved, so the loaded game missed it and games that work every other frame ran one frame late. Also the sound mixer's state, a bug in the YM2151's load code (each channel got the wrong algorithm), the K054539's channels, each CPU's local time, and Galaga's stars, custom I/O and explosion generator. Aliens, X-Men, Street Fighter II, The Simpsons, TMNT and Galaga now resume with identical pictures and the same sound.
- **Two more save state patches for mame2003-plus** (`cores/mame2003-plus`, optional, not used by releases): `0009` saves the Namco wave sound chip (Pac-Man resumed silent: the game turns the sound on only at boot) and `0010` saves the YM3812 by keeping its registers and writing them back on load (Snow Bros. lost its sound). With them, 3 of the 4 games tried that did not resume now do; Hammerin' Harry's driver does not declare save support.

### Fixed (website)

- **End-to-end tests:** the test pattern room's test also clicks the video's controls with `clickControl` (it moved the mouse and clicked, so on a slow runner the controls could hide in between and the click waited out the test), and has two minutes for its four accessibility checks.

## [0.1.9] - 2026-10-04

## [0.1.8] - 2026-10-04

### Added (lab)

- **go-link HD sends each guest the size it needs**: the HD test room keeps up to three sizes (the source and its halves, each encoded only while someone watches it) and gives every guest the smallest that fills its picture, from the web's new `video_want` (a phone 540p, a laptop window 1080p, a television 4K), moving it when the window, the phone's turn or full screen change it. Each guest has its own video track, so a switch never breaks its stream.
- **go-link HD picks its size and codec per computer**: `--test-room-hd auto` (and `device hdprobe`) asks ffmpeg for its encoders and tries, two seconds each, 4K with x264 on Apple Silicon, 1080p with the hardware encoder, 1080p with x264 or VP8, then 720p VP8, keeping the first that streams at 60 fps (the Mac Pro picks 1080p with VideoToolbox).
- **go-link HD: an H.264 track.** The HD test room can send H.264 instead of VP8 (`--hd-codec h264 --hd-h264 x264|videotoolbox`), encoded by ffmpeg running as its own process, so go-link stays MIT whatever ffmpeg was built with. On the Intel Mac Pro, Chrome shows 1080p at 60 fps with the Mac's hardware encoder using under half a core (VP8 used about two); 4K reached 39 fps with x264 and 33 with the hardware encoder there, and **4K at 60 fps from the M1** to Chrome on the Mac Pro over the home network with x264 (about a quarter of its cores). Measured picture quality at 1080p and 8 Mbps is 40-54 dB PSNR on the HD scene and on real photos. `hd.spec.ts` now counts ffmpeg's CPU and can drive a device on another computer ([hd-streaming.md](docs/experiments/hd-streaming.md#an-h264-track-2026-10-03)).
- **go-link HD on Apple Silicon:** the HD experiment measured on the M1 ([hd-streaming.md](docs/experiments/hd-streaming.md#the-same-on-apple-silicon-m1)): its media engine encodes 1080p60 H.264 with half a core but stays at 54-63 fps at 4K, while x264 does 4K at 110-149 fps with about half the cores; the way to 4K is an H.264 track encoded with x264.

- **go-link HD, the streaming experiment** (task T-31 of [experiment 1's verdict](docs/experiments/verdict.md), [docs/experiments/hd-streaming.md](docs/experiments/hd-streaming.md)): an HD test scene (two image AI pictures scrolling as far and play layers, a bouncing ball; `pkg/hdscene`, composed in I420 in under a millisecond at 1080p), `device hdbench` to time the stream's VP8, the Mac's hardware H.264 and x264 at 720p, 1080p and 4K, `device --test-room-hd SIZE` to stream the scene in the test room, and `e2e/tests/hd.spec.ts` to measure it in Chrome. On the user's Mac Pro 2019: 1080p at 60 fps works end to end with today's VP8 (59.7 fps shown, about 2 cores, 7-8 Mbps) and 720p takes 1.25 cores; 4K reached 26 fps with VP8 and 39 fps with this Mac's hardware encoder, so 4K needs H.264 on a faster hardware encoder (the M1 next). Decision: go-link HD's first version streams 1080p60, with 720p as the fallback. `pkg/encoder` and the stream gain a libvpx thread count (the default stays 2).

- **Recorded sessions** (task T-17 of [experiment 1's verdict](docs/experiments/verdict.md), lesson L-15): `rom/tools/lab/session.mjs` records a user session with Playwright for every future case: fixed step numbers from the script, a caption in a fixed place that moves out of a control's way, the control outlined and a "Why" card in the corner farthest from it, then an MP4 with one chapter per step and the captions as a subtitle track, WebVTT chapters and captions, a timeline and `session.md`, whose links open the MP4 at each step ([docs/experiments/harness.md](docs/experiments/harness.md#recorded-sessions)).

### Added (website)

- **The room's Connection details name the video codec** the browser decodes (VP8, or H.264 from go-link HD's test room).

- **A project or AI pack is not a ROM** (task T-21 of [experiment 1's verdict](docs/experiments/verdict.md), lesson E-04): the Export tab says so under **Save the project** and **Pack for an AI**; My device › ROMs stops a `.willy.zip` or `.ai-pack.zip` before sending it, with a message that points to Create ROM, in English, Spanish and Portuguese. The device refuses them too, by their name or, renamed, by what they hold (`project.json` or `PROMPT.md`), answering `upload_result` with `code: "not_rom"`; its window says the same in its three languages.

### Fixed (website)

- **End-to-end tests:** the room tests clicked a video control right after waking the controls, and on a slow runner the controls hid again after 3 s without the mouse moving, so the click waited forever (the picture default test failed now and then in CI). The controls are now woken and seen, then the button gets a single click event (`clickControl` in `e2e/tests/go-link.spec.ts`), so a slow runner can neither miss the click nor land it twice.
- **The go-link mark:** go-link's own games carry a small "go-link" badge with a check next to their title in My device › ROMs (cards and list) and in New game, with their description and their controls ("4 players · Jump · Fire · Special", translated) on the card. `@go-link/shared` reads the new fields, and `rom-test.ts` adds the ROM test's types and `testRomOnDevice` (upload with `purpose: "rom_test"`, then `rom_test`, then the result), which Willy Maker's Export uses for **Test on my go-link**; `HostStream.sendFile` takes the upload's `purpose`.
- **README:** a "Why go-link" section near the top with the story behind the project (getting friends together to play like in the arcades of the 80s and 90s: tournaments and jokes while playing, "Come on, the next token's on me") and what makes it different, Willy Maker in the features with a picture, and the star request moved to Help the project.

### Added (device)

- **go-link's own sets, recognized by hash:** the device ships the list of ROM sets go-link made itself (`backend-device/pkg/ownsets/sets.json`, embedded) with the SHA-256 and size of every file inside each zip, never the zip's own hash (its timestamps change on every build). A zip with exactly those files shows go-link's title, year, maker, description, picture and controls (4 players, Jump, Fire, Special) in the library, the New game page and its rooms, with `own: true` in `device_status.library.roms`, `room_state.info.own` and `room_state.controls.labels`; the host's thumbnails for that set name (the original game's) are never shown for it. A set that only matches by name stays the original game. `device roms check` marks it `[go-link set, verified]`. `rom/tools/ownsets.mjs` writes the list, and `rom/tools/build.mjs` runs it after every build ([releasing the set](docs/rom/README.md#releasing-the-set)).
- **ROM test (Willy Maker validation level 4):** the linked owner's browser sends a set on the `files` channel with `purpose: "rom_test"` (kept in `~/go-link/tmp/romtest/`, never the ROM folder) and asks `rom_test`; the device powers it on with the exact core in two worker processes (`device romtest --child`, with and without scripted Coin, Start and buttons; 60 s at most, one test at a time) and answers `rom_test_result` with a checklist (zip, set, identity, core loaded, files, picture, alive, audio, input reacts, real time; a crash of the core is its own step), the last frame as a PNG, and deletes the set. The same test from the command line: `device romtest ZIP` ([protocol](docs/protocol.md#rom-test), [device](docs/device.md#rom-test)). The core's log now reaches callers (`GameCoreConfig.LogLine`), so the test reads the ROM loader's verdict on each file.
- **`device romtest --input FILE`:** replays an input script (the harness's JSON) on the exact core from power on and saves the `--checkpoints` frames and every `--png-every` frame as PNG in `--frames-dir`, and every frame as an MP4 with `--mp4` ([device](docs/device.md#rom-test), [harness](docs/experiments/harness.md#the-real-core)).

### Added (ROM prototype)

- **Experiment 1, the verdict** ([docs/experiments/verdict.md](docs/experiments/verdict.md)): closed with the user on 2026-10-02. The best ROM is Y, made by case A (by hand); the way forward is case C, Willy Maker's Create ROM, merged into `main`; case B's ROM built a different game because the AI pack carried Willy Maker's own numbers. The page holds the sealed mapping, the jury's medians and Claude's scores, the HOWTO reproductions (all three in the first round, case C to the same zip byte for byte), the external evidence, the user's playtest notes and the tasks T-01 to T-24, each linked to the bug or lesson that asked for it. The records of cases A and B join case C's in `docs/experiments/`.
- **Experiment 1, case C** ([docs/experiments/case-c/](docs/experiments/case-c/)): Game Spec v1 built in Willy Maker in a recorded browser session (82 steps, a note before each action, a chaptered MP4 and an HTML guide), its ROM, the scripts (clear, damage, odd inputs) and the records.
- **go-link's own arcade ROM** ([rom/](rom/README.md), [lab journal](docs/rom/journal.md)): a prototype of *Willy Gorklingo: The Lag Protocol* for the CPS-1 board, laid out as the `slammast` set so the stock mame2003-plus core runs it unchanged, in MAME and in go-link rooms: 4 players with 3 buttons each (Jump, Fire, Special), a Metal Slug-style two-tier street level at 384 x 224 with Willy, crates to push and climb, ladders and one-way ledges, and a camera that only moves forward. One command builds it (`node rom/tools/build.mjs`, Node 22.18 or newer, with our own 68000 code built by gcc and the Z80 sound program encrypted for the board); `rom/tools/room-test.mjs` and `framelab -log` check it in a room. The [hardware notes](docs/rom/hardware.md) and the [art spec](docs/rom/art-spec.md) (sizes, colors, grid, collision tags, objects, the 8192 x 672 Mission 1 canvas) go with it.
- **Experiment 1's harness** ([docs/experiments/harness.md](docs/experiments/harness.md)): the prototype keeps a **lab state** in work RAM every frame (`rom/src/lab_state.h`: a fixed big-endian struct at the symbol `lab_state`, 0xff0000, with the frame, mode, credits, camera, 4 players, 8 enemies, 4 civilians, the exit and the collision map), and `rom/tools/build.mjs` writes the symbol map (`build/symbols.json`, from `m68k-elf-nm`). `rom/tools/lab/` adds a simulator runner (`run.mjs`: a JSON input script or an external player over a JSON-lines protocol with a closed list of actions; the state of every frame, PNG checkpoints, expectations, an MP4 and the inputs as a replayable script), validation level 3 from the command line (`validate.mjs`), a frame compare tool (`compare.mjs`), a route bot (`bot.mjs`), a player that asks the local Laya decision model for the next action (`laya_player.py`) and one command for every automatic test (`acceptance.mjs`). The runner reads inputs one frame late, as the core does, and draws the previous frame's sprite table, so the prototype's scripted runs give the same pixels as the real core.

### Changed (ROM prototype)

- The CPS-1 conversion code (`color`, `sprites`, `cps1gfx` and `kabuki` from `rom/tools`) moved into the TypeScript package `frontend/packages/cps1` (`@go-link/cps1`, browser-safe, with tests), shared by `rom/tools` and Willy Maker. The ROM tools now need Node 22.18 or newer (they import TypeScript). The built set is byte for byte the same (all 28 files compared). The package also snaps colors to the 4096 full-brightness board colors (`toBoardColor`).

### Added (docs)

- **Arcade ROM project** ([docs/rom](docs/rom/README.md)): go-link's own arcade game, *Willy Gorklingo: The Lag Protocol*, built as a ROM set for the CPS-1 board that the mame2003-plus core runs (why that board, how the core and the device would find it, the toolchain, milestones and a prompt to brief an AI), and the game bible ([story.md](docs/rom/story.md)): Major Wilson "Willy" Gorklingo, his brainwashed teammates, the three layers of the Lag and ten detailed levels.

### Fixed (website)

- **Destroy's sprites:** Willy's black T-shirt and parts of his jeans were see-through in most frames (the background keying took dark clothes for the dark sheet background), and two of Glitch-9's walking frames and one punch frame held two poses each. The atlas script (`scripts/destroy-atlas.mjs`) now finds the frames as before but decides each frame's pixels with a tight background match plus everything connected to the character, so clothes stay solid while the gaps between legs and inside effects stay transparent; Glitch-9's walk and punch rows are cut from measured boxes (5 walking frames, 5 punches).

## [0.1.7] - 2026-09-30

### Added (Player apps)

- **Gamepad skins** on iOS and Android (Game settings › Skin): a console shell around the picture, with the D-pad, the action buttons in an arc, Coin and the start capsules drawn after Kenney's Mobile Controls (CC0), and the room's menu as a capsule. Six built-in skins (Violet, Red, Green, Blue, Smoke, Orange); Smoke is the default, and the plain pad without a shell is gone from the choices. Every skin is only data: a JSON file (format 1, [docs/skins](docs/skins/README.md), [schema](docs/skins/skin.schema.json)) that places each part in portrait and in landscape as boxes on a canvas, sets the plastic, decor, controls and menu colors, and may bring its own background pictures; more skins go in the app's Skins folder (the Files app on iOS, `Android/data/org.golink.player/files/Skins` on Android). The built-in skins live once in `docs/skins/builtin`, shared by both apps, and both run the same parser and placement math with the same tests. In the built-in skins the menu sits under the picture in portrait and folds into a handle over the picture in landscape; it shrinks to fit narrow phones, and Coin and the start capsules move apart instead of meeting. The game is as large as the phone allows and always whole (edge to edge in portrait, the full height in landscape, square corners); on small screens the picture gives way to the controls rather than the other way round. A held button or capsule sinks (smaller, darker, a shorter shadow) and the D-pad rocks toward the held direction; a skin may add a tint with `style.controls.lit`.
- **Install a skin by pasting its JSON** on iOS and Android (iOS: Settings › Gamepad skins; Android: Settings › Skins; on both also under the skins in Game settings): the app checks the text (a JSON mistake shows its line and column, a built-in id is refused, the layout is tried on phones and a tablet in both orientations, and missing background pictures are flagged), shows a preview drawn by the room's own painter in portrait and landscape with the skin's name, id and author, and on **Install** saves it to the Skins folder as a **Custom** skin (asking before replacing one with the same id) and offers to use it now. Installed skins can be deleted, after asking (on Android from Settings or with a long press on the skin in Game settings).
- **Cinema mode** in landscape without the on-screen pad (a controller in hand): the picture as large as the screen allows with its sides style around it, and the leave button and the room's buttons in a floating capsule that folds away after 3 s, instead of the old side columns.
- **Controller button:** the "*name* · display only" chip is now a round see-through button at the top right of the picture, like the stats one; a tap shows the controller's name.
- **Skin editor** on the website ([/tools/skin-editor](https://go-link.org/tools/skin-editor)), in English, Spanish and Portuguese: it draws a skin exactly as the apps do (their placement math and checks in TypeScript, with tests) and lets a designer drag and resize every part with rulers, guides and snapping, add plates and speaker grills, set every color and a background picture, design the buttons and the D-pad, preview 1 to 6 buttons and 1 to 4 players on five screens, see and edit the JSON side by side, run the apps' checks live, and export the skin JSON. The built-in skins are read from `docs/skins/builtin`, next to the guide and the schema; phones get a note that it needs a bigger screen. The zoom floats on the canvas and the built-in badge folds to a lock on narrower screens, so the top bar stays on one row. **My skins** keeps several skins in the browser (localStorage only), one record each, saved as they change: New and Open start a record, editing a built-in skin saves a copy, and the list opens, duplicates and deletes them (the delete asks first). A five-step welcome explains the editor on the first visit (the **?** button brings it back), and the user guide has a full **Skin editor** manual (quick start, interface, precision tools, style, previews, what each check means, exporting and installing on iOS and Android, My skins, shortcuts and tips).
- **Coin** is called Coin in every language (the apps and the website's guide), like on an arcade panel.

### Added (website)

- **Destroy this page** (an easter egg on the landing): the header's little devil, which wiggles now and then, starts a rescue mission on the page itself. With the alarm on, the go-link hero (drawn from the project's own sprite sheets) runs, jumps, double jumps, flies a moment with a jetpack, crawls, drops through floors and breaks every word, picture, button and card with a machine gun (aimed up, ahead or straight down), a knife, a bazooka and flying kicks; each piece has health by kind with a small health bar while it is hit, bullet holes, shattering pieces and debris, and wide layout bands are background he falls through, so he digs from the header down to the footer with the camera following; people to rescue (about one per screen of page, the four kinds mixed) are hidden in different places every run and a beam of light lifts each one away once saved; the Lag gang (Glitch-9, Vera Buffer and Jitter, in several outfits) guards the page, shooting on sight and chasing Willy once angry; Willy has 4 lives and health, a 10-minute clock (MISSION ABORTED when it runs out), a mission bar, radar arrows to what is out of sight, Restart and Unstuck, and the mission is complete at 100 % destroyed with everyone safe and the gang defeated. Controllers, keyboard or a touch pad, 8-bit sound made in the browser, nothing stored; leaving puts the page back at once. The briefing tells the story (Willy Gorklingo, whose surname is an anagram of go-link.org, against the Lag gang), shows who to rescue and who is wanted, keeps its buttons always in view, and its controls help adapts to what you use: with a controller it draws that model (recognized by its USB ids, like the controller test) with each action on that controller's own button names, and whatever you press lights up in the drawing and in the list; the keyboard and touch tabs light their rows too. The hero's machine gun aims in 8 directions with poses made from the sheet by the atlas script (`scripts/destroy-atlas.mjs`, which also cuts the villains' and the civilians' sheets), and the bullets leave each pose's real muzzle. It has a direct link, [go-link.org/WillyGorklingo](https://go-link.org/WillyGorklingo), which opens the landing with the briefing without waiting for the startup logo. The game and its characters (lossless WebP, a third lighter) start downloading when the pointer reaches the devil, and the briefing opens at once with skeletons where the characters go until they are in. Controllers use this browser's button map for their model, like the controller test, so a generic USB pad plays and lights the help the same way. It is a self-contained module (`src/destroy/`, atomic design) with one entry, loaded only when the devil is clicked; the controller drawings and model recognition it shares with the controller test live in `src/controllers/`. The header's startup sound switch is gone: the intro's coin always plays (when the browser allows).
- **Tools** (`/tools`), last in the menu: a card for each browser tool (no device, nothing stored). The skin editor lives there too.
- **Controller check** on Test your controller: each connected controller gets a card that recognizes its brand and model by the USB ids (Switch Pro, Joy-Con, DualSense, DualShock 4, Xbox 360, Xbox One/Series, 8BitDo), draws it as it is (our own drawings, no logos) and lights what is pressed, and measures stick drift, dead zone and circularity, trigger range, return and what stays pressed, raw buttons with double presses (worn contacts), the updates per second, and vibration. **Run the check** guides through it and sums up each result. **Event log**: a terminal next to the controller cards with every button, stick, trigger and connection change as it happens (bounces as warnings), filters, pause and clear; **Copy report for AI** and **Download report** make a Markdown diagnosis of every controller (checks, sticks, triggers, each button's presses and bounces, the log and the raw data) ready to paste into an AI assistant. The update rate shows the highest rate seen instead of 0 while a controller rests. Beside the screen's refresh rate the page shows one frame's length and explains that the delay is about one or two frames, so a 120 Hz screen halves it.
- **Controller mini-games** (`/tools/games`): Link (a cable puzzle), Snake, Memory, Paddle, Racer and Special moves, each one measuring something: buttons and shoulders, directions, double presses and answer time, stick drift, triggers, and combo timing. Controllers or the keyboard, pixel art in the site's colors, a How to play panel beside each game (goal, controls, points and what the result says about the controller), retro chiptune music for each game and a sound for every action (made in the browser, no sound files; music and sound switches); the How to play panel takes the width left beside the board, in two columns on wide screens; an on-screen touch pad on phones and tablets with only each game's controls (D-pad, face buttons, L/R, Start, a steering slider, gas and brake; several fingers at once), and effects on the board (particles, points floating up, flashes, a short shake, a pulse along Link's cables; calmer with reduced motion), nothing stored.

### Changed (website)

- Tools, Test your controller and Picture styles sit in the same centered frame as My device, with its margins, instead of against the left edge and the header.
- The header's startup-sound switch became the Destroy devil (drawn in the icons' outline style), shown only on the landing (`/`): the game is that page's easter egg. The phone tab bar has the Tools tab.

### Fixed (website)

- A non-standard controller's button held at the moment the browser first lists it (often the press that wakes it) counts again once released, in rooms, the controller test and the games (`readGamepad`).
- Fonts are never inlined as `data:` URLs in the build, which the CSP (`font-src 'self'`) blocked.

## [0.1.6] - 2026-09-29

### Fixed (Android app)

- The room chat no longer drops the last characters of a fast-typed message: its field keeps its own text state while the room screen redraws.

### Added (device, developer tools)

- **Video quality lab** (`backend-device/cmd/framelab`, `pkg/framelab`, [docs/quality.md](docs/quality.md)): records the exact picture a core draws (through the same `GameCore` as the `emulate` worker, with scripted controller input in the device's input bits), pushes it through the device's real VP8 path with variants (2x nearest-neighbour upscale, averaged chroma, bitrate, quantizers, cpu-used, VP9 for reference), decodes it with libvpx the way a browser does (checked against Chrome's WebGL and canvas output) and reports PSNR (RGB, Y, Cb, Cr), SSIM (Y and RGB), VMAF and MS-SSIM, bitrate and encoder time. `GameCoreConfig.RawVideo`, `libretro.ToRGB` and optional `MinQuantizer`, `MaxQuantizer` and `CPUUsed` in `encoder.Config` (zero keeps the streaming defaults) support it.

### Added (device)

- **Sharper game colors: 2x video.** Game rooms send the game's picture enlarged 2x with nearest neighbour, so every game pixel keeps its own color through VP8's 4:2:0 color (about +7 dB RGB PSNR in the [video quality lab](docs/quality.md), B3500). The worker makes it in one pass with the RGB to I420 conversion (`libretro.ToI420Double`, tested bit for bit against upscaling first; 0.55 ms per 384 × 224 frame on an i9-10900 instead of 3.3 ms for a separate upscale).
- **Video quality setting** (`device.json` `video_quality`): **High** (default: 2x at 3,500 kbps), **Normal** (2x at 2,500 kbps) or **Saver** (the game's own size with each 2x2 block's color averaged, `libretro.ToI420Box`, 2,500 kbps). In the window (Settings › Rooms), the CLI (`device video quality [high|normal|saver]`) and linked browsers (`set_video_quality` / `video_quality_result`; `device_status.video_quality`). A change from the window or the website reaches running rooms at once; a factory reset goes back to High.
- **Automatic fallback**: a 2x room measures its encoder for its first two seconds of streaming; when the 95th percentile takes more than 60 % of a frame's time it moves to Saver for the rest of its run, logs it and reports it in `device_status.rooms[].video` (`quality`, `fallback: "cpu"`, `scale`).
- **Protocol**: `stream_stats.video` (`scale`, the game's `width` and `height`, `quality`, `fallback`), also sent when the control channel opens and when the frame size changes; the `emulate` worker takes `--video native|box|double`, its video messages carry the scale and the parent switches it with a `VideoMode` message. See [docs/protocol.md](docs/protocol.md#video-scale).
- `framelab encode` converts with the device's own one-pass conversions (`Variant.PrepareFrame`) and reports `encode_p50_ms`.

### Added (website)

- **2x streams**: when `stream_stats.video.scale` is 2 the picture renderer averages each 2x2 block back to the game's pixels before every style and the ambient light, so CRT scanlines follow the game's real lines and smooth edges see real pixels (identical to a native stream in every style, checked in Chromium). The plain `<video>` is unchanged. The stream figures show **Video** ("768×448 (2× of 384×224)") and **Quality**; My device › Overview has a **Video quality** card (High, Normal, Saver) that lists the quality each running room uses ("Saver (CPU)" after a fallback). `/picture-lab` takes `?up=2`.
- **Picture styles in the room**: a **Picture** button in the dock (on phones, in the console drawer's You tab) chooses how this browser draws the game, live and only for this viewer, remembered in `go-link.picture-style` / `go-link.picture-bands`. The site's default is **Smooth** with **Ambient** sides (constants in `picture/settings.ts`). Styles: **Smooth** (bilinear; with Black sides it is the browser's own look and keeps the plain `<video>`), **Sharp** (square pixels with a one-screen-pixel blend at each pixel edge), **CRT arcade** (our own shader: gaussian scanlines sized to the game's lines, an aperture grille at the screen's pixel density, gentle barrel curvature that keeps the whole picture visible, rounded tube corners, a soft glow, light added in linear space) and **Smooth edges** (experimental, our own edge-directed enlargement). Sides: **Black**, **Ambient** (the frame shrunk to a 32 × 24 texture, blurred, enlarged behind the picture and darkened) or **Frame** (an arcade cabinet surround drawn in the shader: bezel, lit trim, speaker grilles). The GPU renderer (`src/picture/`: WebGL 2, else WebGL 1, else the plain video) draws the room's `<video>` into a canvas on each new video frame (`requestVideoFrameCallback`), never crops, pauses while the page is hidden and survives a lost GPU context; the `<video>` keeps playing underneath, so sound, screenshots, recordings, full screen and the console layout are unchanged. **Compare** puts a draggable line over the video (left: the browser's look). The stream figures show the renderer and the screen's measured refresh rate ("Screen: 120 Hz"), as does Test your controller.
- **Room picture default**: the host gives a room a default picture for guests who never chose one: **Set as the room's default** in the room's Picture popover, **Picture default** in a room's ⋯ menu in Rooms, and a picture button next to the test pattern room in My device (`RoomPictureDialog`). The viewer's own choice still wins, then the room's default, then the site's; the popover shows "Room default: *style* · *sides*" and **Use the room's default** when the viewer's choice differs. Protocol: `room_action` `picture` (`style`, `bands`; both empty clear it; `id: "test"` for the test pattern room), optional `create_room.picture`, `room_state.picture`, `device_status.rooms[].picture` and `device_status.room.picture` (types and parsers in `@go-link/shared` `picture.ts`).
- **`/picture-demo`**: the styles side by side on a synthetic 384 × 224 test frame with moving sprites, with the same choices, a draggable divider and full screen; no device needed.

### Changed (website)

- **The site's own dropdowns**: the Microphone and Output lists in the room's Volume and voice settings, "Plays as" and "Keyboard plays as" in the controls panel and the ROMs tab's Sort no longer open the browser's system-drawn list. A new `Select` (a capsule button with a listbox in the site's colors, in both themes) shows a check on the current choice, stays inside the window (opening upwards when needed, also inside the voice popover, the console drawer and dialogs) and works with the keyboard like a native select: arrows, Home and End, typing to jump, Enter or Space to pick, Escape to close without closing the popover around it. Touch screens get 44 px options.

### Added (device)

- **Room picture default**: the device validates and keeps each room's default picture style in `device.json` (`rooms[].picture`, and `test_room_picture` for the test pattern room; unknown values are refused or ignored, a factory reset forgets them) and sends it to every guest in `room_state.picture`. It only stores and forwards it: the picture is drawn by each browser.

### Fixed (website)

- **Room side panel**: the chat, queue and spectators panel can take keyboard focus, so a long list scrolls from the keyboard (axe `scrollable-region-focusable`).

### Added (iOS and Android apps)

- **High refresh rate in the room and Test controller**: 120 Hz on ProMotion iPhones and iPads (`CADisableMinimumFrameDurationOnPhone`, a `CADisplayLink` asking for 60 to 120 Hz, and the game picture's Metal view checking for new frames 120 times a second, so a frame is shown within about 8 ms); on Android the display mode with the same resolution and the highest refresh rate (90, 120 or 144 Hz), restored on leaving. Menus stay at the system's rate to save battery.
- **Game settings in the room**: a gear (**Settings**) in the dock, right under the gamepad button, opens **Game settings** without leaving the room (a side sheet in landscape, a bottom sheet in portrait, so the picture stays in sight): your name (same rules as the name step; Save sends a new `hello`), the game and voices volumes (now 0 to 100 %, remembered), the microphone and output with Test sound (moved from the Sound / Volume and voice sheet), the picture and a Show stats switch (the same setting as the stats button). The drawer's You tab opens the same panel instead of its own name field and sound sheet.
- **Picture styles in the apps**: the website's styles (**Smooth**, **Sharp**, **CRT arcade**, **Smooth edges**) and sides (**Black**, **Ambient**, **Frame**) drawn by native GPU renderers with the website's shader math (iOS: a Metal Shading Language port compiled at run time; Android: the GLSL ES shaders), from each decoded frame converted to RGB (BT.601), never cropped, at the stream's display aspect. **Compare** draws a draggable line over the live picture: the plain decoded picture on the left, the chosen style on the right. Remembered per phone in `go-link.picture-style` / `go-link.picture-bands`, applied live; defaults Smooth with Ambient sides (constants in GoLinkCore and `:core`, with layout and settings tests mirroring the website's). The picture is drawn only on a new frame, setting or size, ticking at the screen's highest rate; if the GPU path cannot start, the plain WebRTC view stays and the panel says so. The host's room default (`room_state.picture`) applies when the viewer never chose one (the viewer's own choice, then the room's, then the app's, as on the website), with "Room default: …" and **Use the room's default** in Game settings. Android draws the game into a `SurfaceView` (it was a `TextureView`) and uses libwebrtc's plain drawer for Smooth on Black. Debug builds have a picture lab that streams the website's test card at 60 fps into the renderer.
- **2x streams in the apps**: both apps read `stream_stats.video` (`scale`, the game's size, `quality`, `fallback`; GoLinkCore `StreamVideo.swift`, `:core` `StreamVideo.kt`, with tests) and, when the decoded frame is exactly twice the game's size, their renderers average each 2x2 block back to the game's pixels before every style, the smooth edges pass and the ambient light, like the website (iOS: a Metal pass reading the four texels; Android: one bilinear sample at the block's center, exact even on GPUs whose coordinates land slightly off a texel center). Scale 1 and the plain fallback views are unchanged. Debug offscreen checks (iOS `-picture2xCheck`, UI test `testPicture2xMatchesNative`; Android `--es lab check2x`) confirm a 2x nearest-neighbour test card draws byte for byte like the native one in every style and side; the picture labs stream 2x frames (`-labUp2`, `--ez up2 true`). The stats box shows "video 768×448 (2× of 384×224)" and "quality High" ("Saver (CPU)" after a fallback), in English, Spanish and Portuguese.
- **Screen refresh rate on screen**: the stats box gains a "screen 120 Hz" line (measured from the screen's frames; fps is still the game video's), and Test controller shows "screen 120 Hz · 1 frame 8.3 ms" next to the latency, whose hint now says it is about one screen frame. Shared `ScreenRate` and `RefreshRateMeter` with the same tests in `:core` and GoLinkCore.

### Fixed (iOS and Android apps)

- **Test controller**: the readouts (controller, latency, screen rate) no longer cover the lower part of the test card's circle, in portrait and landscape.

### Fixed (Android app)

- **Controller connection in Test controller**: a controller was called USB whenever Android could not confirm Bluetooth (any external device). Now USB is shown only when a USB device with the controller's vendor and product id is attached, Bluetooth when a paired device has its name, and otherwise just the controller's name.

## [0.1.5] - 2026-09-29

### Added (device and website)

- **Only the host pauses; players ask for a pause.** The device accepts `pause` only from the host (a browser in the room that came in with the owner key, or a linked browser with `room_action`); anyone else gets `error` `pause_owner_only`. Seated players send `pause_request` (withdraw with `cancel`), one per player, for 30 seconds. The host sees "NAME wants to pause" with **Pause** and **Keep playing** (over the video in the room, and as a notice on any page of the linked website, naming the room), answered with `pause_answer`; accepting pauses for everyone in the requester's name, declining tells only the requester. `room_state` gains `host_online`, `you.owner`, `you.pause_asks` (host only) and `you.pause_asked`; linked browsers get `pause_asked` / `pause_ask_gone`, and `device_status` rooms list `pause_asks`. The request lives in the Room Manager actor, with its expiry timer. On the website the dock's pause button becomes **Ask the host for a pause** (Cancel while it waits), off with a tooltip when the host is not connected or the room cannot pause. `--test-room-pause` lets the test pattern room pause, to try the flow without a game (used by the e2e).
- **Player names (alias)**: 2 to 20 characters of letters (any language, accents, ñ), digits and single spaces. The device cleans every `hello` name itself (removes symbols and emoji, joins spaces, cuts, falls back to the generated name) and tells the guest the name it uses in `room_state.you.name`. On the website, after the code and PIN let you in, **What's your name?** asks for it, prefilled with the last name used (`go-link.player-name`), with live checking (a red field and why, a counter); the name field in the room checks the same rule.
- **Test your controller** (`/test-controller`): the test pattern's controller drawn in the browser, lit by the keyboard, the on-screen gamepad and Bluetooth or USB gamepads, with each gamepad's name and the input-to-screen time. Linked from the guide's Controls page and the room's How to play.
- The phone console's **You** tab gives the host **Invite** and **Close game**.

### Fixed (website)

- **Light theme contrast**: the accent as text (links, eyebrows, codes), the voice color, the faint text and the player colors failed WCAG AA on light backgrounds. New `--color-accent-text` tokens and darker light-theme player colors fix every page; the accessibility tests now really switch the site to its light theme (they checked the dark one twice), and the room is checked in both themes.

### Added (website, iOS and Android apps)

- **Startup intro "INSERT COIN"**: on a dark arcade background the go-link icon pops in with a soft orange glow, "go-link" rises and **INSERT COIN** (pixel letters drawn in code, the same in every language) rises and blinks, with a **coin sound**: three quick square-wave notes, an original sound made in code (Web Audio on the website; the apps play the same synthesis rendered to a small WAV by `scripts/coin-sound.mjs`, MIT like the project). A tap skips it; reduced motion only fades it in. On the website it is the loading screen that already waited for the first real data (no longer than before, and dark in both themes); browsers allow sound only after a gesture, so the coin plays on your first tap or key while the intro shows or within the first seconds, never later. A **Startup sound** switch (on by default) is in the header's tools (a coin button; on phones under "…") and in the apps' Settings.
- **Room codes grouped while typing**: the code field shows `915 355 636` as you type, on the website's join form and in the apps. The spaces are only visual, the caret stays after the same digit, backspace after a space deletes the digit before it, and pasting with or without spaces works; a pasted invitation link is left as it is. Shared tests for the formatting on the three platforms.

### Added (iOS and Android apps)

- **Your name before entering**: after the code or QR and the PIN, the apps ask "What's your name?", prefilled with the saved name. Only letters (with accents and ñ, any script), numbers and spaces, 2 to 20 characters: a symbol or an emoji turns the field red and says why, with a counter. The name you change in the room follows the same rules. Shared `PlayerName` rules with the same tests in `:core` and GoLinkCore.
- **Ask for a pause**: only the host pauses now, so the apps' pause button asks the host for a pause (and can cancel it while it waits). It is dimmed and explains why when the host is not in the room or the game cannot pause; a declined request shows a notice.
- **Stats**: a small round button at the top left of the game shows or hides a see-through box with the frames per second, resolution and codec, the ping to the device, whether the connection is direct or relayed, packet loss and the game audio. Remembered.
- **See-through gamepad with a real controller**: with a controller connected (Xbox, DualSense/DualShock, Switch Pro, MFi, Bluetooth or USB), the gamepad button shows the on-screen pad see-through and display only, lighting what you press on the controller, with a "*controller* · display only" chip.
- **Test controller** on the home screen: an offline test card with a controller diagram that lights up for the on-screen pad and real controllers, the controller's name and connection, and the input latency (last, minimum, average).
- **Version** on the home screen and at the bottom of Settings ("go-link Player v0.1.5 (build 105)", " · debug" in debug builds). iOS takes it from `MARKETING_VERSION` / `CURRENT_PROJECT_VERSION` (`make ... VERSION=x.y.z`), Android from `-PversionName` / `-PversionCode`, with the same build number rule.

### Added (iOS app)

- **go-link Player for iPhone and iPad** (`mobile/ios/`, SwiftUI, iOS 17+), the iOS counterpart of the Android app, not distributed yet: scan the invitation's QR code, type the code or paste the link, then the PIN and the terms; the room in portrait (Game Boy) and landscape (Switch) with the on-screen gamepad from the game's controls, Coin and 1P…NP, MFi, Xbox, PlayStation and Switch Pro controllers, and a drawer with **Chat** (typing, unread badge), **Players** and **You** (name, Volume and voice, How to play, Leave). **Volume and voice** has game and voices volumes, the microphone choice and the system's output picker (speaker, headphones, AirPods), plus a test chime. The microphone is asked for only when it is first turned on (the app uses its own audio device so listening never opens it), and sound never goes to the earpiece. Settings has your name and your own signaling server (`wss://` only, tested before saving, back to the official one). Universal Links for `https://go-link.org/g/…` once the association file carries the signing team.
- `GoLinkCore`, a Swift package ported from the Android `:core` module with the same tests (invitation parsing, the 12-byte input packet, room state, the signaling URL rules, the PIN gate and return tokens, the room client), and an XCUITest target that takes screenshots and joins a real test pattern room.
- The website publishes `/.well-known/apple-app-site-association` (with a placeholder team id until the app is signed for distribution); CI builds the iOS app for the Simulator and runs the GoLinkCore tests on macOS.

### Fixed (iOS and Android apps)

- **On-screen gamepad after turning the phone**: after rotating between portrait (Game Boy) and landscape (Switch) the pad's buttons could stop answering or press the wrong button, and a button held while turning could stay pressed on the device. On iOS the old layout's buttons, going away after the new ones had registered, removed the new ones' hit areas; on both apps a finger held across the layout swap was never released. The buttons are now measured where they are at each touch (iOS) or registered per control (Android), every rotation lets go of all held buttons (the device gets 0), and the D-pad and a button still work together. A shared `TouchPadTracker` in `GoLinkCore` and the Android `:core` module has tests for it, and an XCUITest (`testPadAcrossRotations`, on the debug-only `-padLab` screen) rotates the Simulator and presses every pad control in each orientation.

### Added (website)

- **Choose your microphone and speakers** in the room: **Volume and voice** has a **Devices** block. Pick the microphone (it switches instantly, without reconnecting) and test it with a "Speak to test" meter; pick where the game and the voices play (speakers, headphones, AirPods…) and play a test tone there. Chrome and Edge list the outputs, Firefox adds "Choose another output". On iPhone and iPad, where the page cannot choose the output, a note explains how to do it from Control Center. The choices are remembered in the browser; if the chosen device disconnects, the sound goes back to the default and a notice says so. On phones the settings open as a sheet from the bottom.

### Added (Android app)

- A **Sound** sheet in the room (Players › Sound) picks the microphone (Automatic, the phone's, or a connected Bluetooth, wired or USB headset) and the output (Automatic, the loudspeaker, or a headset), with a Test sound chime. Automatic keeps the old route: headphones when connected, otherwise the loudspeaker, never the earpiece. The choice is remembered; a saved headset that is not connected yet is used as soon as it connects, and if the chosen device disconnects the app goes back to Automatic and says so. On Android 12+ each device is chosen exactly (`setCommunicationDevice`); on Android 8–11 by kind (Bluetooth SCO or speakerphone). The chosen microphone is also libwebrtc's preferred input.

### Changed (website)

- **Phone console drawer with tabs**: on phones and tablets playing with the on-screen gamepad, the side drawer is split into **Chat**, **Players** and **You**. The chat now fills the whole drawer (on an iPhone held sideways it showed only one or two lines), the Chat tab counts unread messages, Players holds the seats (swap, move, silence), the queue and the spectators, and You holds your name and big buttons for Volume and voice, Full screen, How to play and Leave room. Sideways the drawer takes about 40 % of the screen; the last tab is remembered while the page is open. The computer's room layout does not change.
- The rooms search hint no longer names real games ("Search your games…").
- The landing page shows **real screenshots** instead of drawings, in the page's language: the website (My device, its ROMs tab, a live test pattern room, the invitation with its QR code and PIN), the go-link window on the computer and the go-link Player app. Only the test pattern room and an invented ROM library appear: no real game, and nothing personal.
- New order: hero, then the guest's quick path ("Someone sent you a QR? Three taps": scan, type the PIN, play and talk, with the app's downloads and the browser as the other way in), the how it works walkthrough (now with screenshots of each step), the features (three columns), then the **Player app** section with Android's download (iPhone and iPad as coming soon) and a **gallery** whose pictures open large in an accessible dialog (Esc, arrow keys).

### Added (development)

- `cd e2e && npm run shots` makes the landing's screenshots again in English, Spanish and Portuguese: the device's window from Fyne's test driver (`TestShots`, `GOLINK_SHOTS`), the website on the e2e stack with an invented ROM library (`backend-device/cmd/shotseed`, `internal/shots`) and the Player app on an Android emulator. The computer's name, user name, local paths and addresses are masked, and a picture that still shows them fails.
- The Android end-to-end test covers more of the room: the 1P…NP start buttons (the device's input log and the test card's player lamps), the queue when the four seats are taken by browser guests (the app waits as #1, takes the freed seat, watches and comes back) and swapping seats with a browser guest asked from either side.
- `cd e2e && npm run test:android:camera` (opt-in): the app scans a QR code of a real invitation with the emulator's virtual camera (the poster on the virtual room's wall, the device moved in front of it over the emulator's gRPC), refuses a foreign one and joins with the PIN. It restarts the emulator with `-camera-back virtualscene` when needed and starts it again as it was.
- The Android harness closes system dialogs that a slow emulator shows over the app ("isn't responding": Wait; another app's crash: Close) and fails with the text when the app itself crashes; a release build on the device is replaced by the debug build. The weekly workflow waits for the launcher to settle before the test.

## [0.1.4] - 2026-09-29

### Added (Android app)

- **go-link Player**, a native Android app (Kotlin, Jetpack Compose, libwebrtc; `mobile/android/`) to join a game as a player: scan the invitation's QR code, type the 9-digit code or paste the link, then type the PIN and accept the terms. Every new join asks for the PIN; the device's return token only gets the app back in after a dropped connection.
- In the room: the game's video and sound, chat (notices in the reader's language), voice with the other players (the microphone line answered sendonly, per-player silence), the on-screen gamepad from the game's controls (portrait Game Boy, landscape Switch), Bluetooth and USB controllers as local players, start buttons 1P…NP, seats, queue, spectating, swapping controllers and pause.
- Headphones: sound and voice go to a Bluetooth, wired or USB headset when one is connected, otherwise to the loudspeaker. Camera, microphone and Bluetooth permissions are each asked when needed, after a short explanation.
- Invitation links (`https://go-link.org/g/…`) open the app on the PIN screen (Android App Links). QR codes are read strictly: only a go-link invitation link or a 9-digit code, never a server or a PIN.
- Settings: player name and your own signaling server (`wss://` only, tested before saving, with a way back to the official one).
- An end-to-end test of the app on an Android emulator (`cd e2e && npm run test:android`, also weekly in `.github/workflows/android-e2e.yml`): join by link and by code, wrong and used PINs, picture, gamepad, game sound, chat, voice both ways and coming back after the network drops. Debug builds log room events and WebRTC counters (`GoLinkE2E`) and accept `ws://` to the development machine; release builds do neither.
- Settings' "Test and save" saved no server at all (a server that answered was reported as silent); fixed.
- Releases publish the app signed with the release key as `go-link-vX.Y.Z-android.apk` (`make release` builds it; `ANDROID=0` skips it; the key stays outside the repository, `ANDROID_SIGNING`). The invitation page on Android downloads it directly, `/.well-known/assetlinks.json` carries the key's fingerprint so invitation links open the app, and the landing lists the app among the features.
- The portable protocol and room logic live in a plain Kotlin module (`:core`) with unit tests, as the base for an iOS version. `make android-debug` and `VERSION=x.y.z make android-apk` build it; CI runs its tests and a debug build.

### Added (website)

- On Android, an invitation page (`/g/…`) shows a card to get the go-link Player app from the latest release, next to playing in the browser.
- `/.well-known/assetlinks.json` for the app's App Links (the release key's fingerprint is still a placeholder, see `mobile/android/README.md`).
- The user guide has an "Android app" page (install, join, play, permissions).

## [0.1.3] - 2026-09-28

### Added (website)

- "How it works" is an animated walkthrough on the landing page and on My device: four steps (download, link, add ROMs and play, share) that play like a video, with a progress bar, pause and previous/next. Step 1 offers the download for your system (Windows, macOS, Linux, Docker, Raspberry Pi).
- My device shows the downloads of the latest release instead of "Downloads will be published soon": your system first, the others with their sizes, the release page and the checksums.
- My device, before linking, puts the code form first and shows the go-link window as it really looks (its texts in each language), with the downloads below.

- The Docker page of the user guide starts with a quick start (run, pairing code from the log, emulator, ROMs, panel key), explains host and bridge networking with one full command each, and lists the useful `docker exec` commands.

### Fixed (website and device)

- Room chat notices (took a seat, left, paused, swapped controllers…) and generated guest names ("Guest 9F3A") show in the reader's language: the device sends each notice as an event with its values (`event`, `args`).

### Fixed (device window)

- The countdown to a new pairing code is translated, and it no longer runs under the "Copy code" button.

### Changed (release)

- The device image is published on the GitHub Container Registry, `ghcr.io/lordbasex/go-link-device` (`:vX.Y.Z` and `:latest`, amd64 and arm64), by a workflow on every release; the website and the guide use it (`docker run … ghcr.io/lordbasex/go-link-device:latest`).
- The image is also on Docker Hub as `lordbasex/go-link-device`; the workflow copies each release there when the repository has Docker Hub secrets.
- The repository has a contributing guide, issue templates and a README that invites people to star, install and help.
- Releases include the Docker image by default (`DOCKER=0` skips it); v0.1.2 got its image afterwards. The Docker guide runs the image by its version tag (`go-link-device:vX.Y.Z`), which is how `docker load` names it.

## [0.1.2] - 2026-09-28

### Added (website)

- Recording in the room: the host's record button (red dot, then a square to stop) in the dock, with the time; everyone sees REC over the video and a notice in the chat; pausing a recorded game asks first.
- When a recording ends, the host is offered to download it; My device › History shows each game's recordings (download, delete), deletes a game from the history, and the space they take.
- Screenshots: a camera button saves a PNG of the game (crisp pixels, the game's shape), in the browser only.
- Factory reset on My device, with a confirmation.

### Changed (website)

- The landing page and the user guide describe recordings, MP4 export, screenshots and the phone console; a new guide page, "Recordings, screenshots and MP4". The guide no longer says voice is never recorded.
- Phones and tablets play in a full-screen console: upright like a Game Boy (picture on top, gamepad below), sideways like a Switch (picture in the middle, gamepad halves on the sides). The touch gamepad is on by default; chat, players and room actions open from a side tab.
- The controls over the video are see-through and hide after 3 seconds without moving the mouse or tapping, as in video players.
- On phones, a one-row header with a "…" tools menu, and the sections as a tab bar at the bottom of the screen.
- Recordings download as MP4 (H.264 + AAC, the picture enlarged with crisp pixels) ready for chat apps and phones: the browser converts them with WebCodecs and a small Go WebAssembly helper, the device takes no part. When someone spoke, a preview plays the recording first ("Preview and export"), with a Game and a Voices volume and a clipping meter, and "Export MP4" applies them; with voices the MP4 has three sound tracks (game and voices, the default; game alone; voices alone). Browsers that cannot convert save the original WebM.

### Added (device)

- Recording of game rooms for the host (`room_action` `record_start` / `record_stop`): a WebM file with the game's picture and sound and one track per player's voice, copied from the packets the room already sends (no encoding twice). It stops at 2 hours or 2 GB, when the game is paused and when the room stops; everyone in the room sees `room_state.recording` and a chat notice. Files in `~/go-link/rec/`, listed in the history of games (`get_recordings`, `delete_recording`, `delete_history`; clearing the history deletes them).
- Recordings download to the owner's browser over the `files` channel, pulled in 60 KB pieces with progress, cancel and a SHA-256 check at the end.
- Factory reset (`factory_reset`, `device reset --yes`): rooms, saved games, history, recordings, linked browsers and settings go; the device's identity and network settings, ROMs, thumbnails and the core stay.
- CLI: `device rec list`, `device rec rm`.

### Fixed

- The update notice reaches My device on the website (it only showed in the window).

## [0.1.1] - 2026-09-28

### Changed (device window)

- The window speaks English, Spanish or Portuguese (Settings › General, or automatic from the computer), live, without restarting.
- System is merged into Overview (the computer's details at the bottom, no usage shown twice); the Overview uses the app's logo, counts every live and paused room, and opens the rooms on the website.
- MAME › ROMs only manages the folder: drop zone and file picker, counters (sets, runs, will not run, added, folder size, free space); the game list lives on the website.

### Added

- The device checks go-link's releases on GitHub every 6 hours; the window and My device show a download notice when a newer version is out. My device also shows the device's version.
- My device › ROMs can choose which picture every browser shows and the device's pictures folder (`set_thumbnails`).
- Install page of the user guide with one tab per system (it opens on the visitor's own).

### Fixed

- The troubleshooting guide quotes the exact message the website shows for a used invitation.

## [0.1.0] - 2026-09-28

First public release (pre-release): universal macOS dmg, Windows, Linux, Raspberry Pi and Docker, with licenses in every file.

### Added

- User guide at `/docs` (English, Spanish and Portuguese): install, linking, ROMs, first game, invitations, controls, voice and chat, rooms, the app, pictures, headless panel, command line, Docker, network and troubleshooting. "Docs" in the menu and the footer.
- An arcade 404 page, and the website's version in the footer and in a `go-link-version` meta tag.

### Legal

- Terms of use and privacy policy (`docs/legal.md`, and `/terms` and `/privacy` on the website in English, Spanish and Portuguese), with the MAME trademark notice and the non-commercial license of mame2003-plus.
- Linking a device and joining a game require accepting the terms (a checkbox); the device keeps the accepted version with each linked browser.
- `THIRD_PARTY_NOTICES.md` (generated by `scripts/third-party-notices.sh`) ships with every release: inside the macOS app, the zip and tar.gz archives, the Docker image and as a release file.
- The website's fonts are bundled instead of loaded from Google Fonts, so no third party receives visitors' IP addresses.
- Site footer with licenses and legal links, and a GitHub link in the header.

### Documentation

- English documentation split into `docs/` (architecture, flows, device protocol, device, emulator, website, networking, security, building, deploying, status); the README is now a summary with quick start and deployment.
- This changelog, with the history of the private repository.

### Fixed

- A room of a game that cannot be saved failed to come back after a device restart: it was launched from its automatic save while the save check removed that file.

### Changed

- macOS: one universal app and `.dmg` (Intel + Apple silicon) for macOS 12 or later, built from any Mac; libvpx and Opus are built from source per architecture (`build/macos/static-libs.sh`) instead of Homebrew's copies, which required the build Mac's macOS version.
- Website hosting moved to a gitignored `deploy/local/hosting.mk`; `deploy/hosting.example.mk` shows a generic upload over SSH.
- Test fixtures use neutral values; fake test tokens are marked with `gitleaks:allow`.

## History before the public repository (2026-09-26 to 2026-09-28)

The project was built in a private repository first. These are its commits, grouped by day, newest first.

### 2026-09-28

- Deploy: signalhub is deployed from its own repository; go-link keeps only its CDN security headers, and the Makefile has no signalhub image targets
- Cores: document what is left for the mame2003-plus save state patches

### 2026-09-27

- Cores: mame2003-plus save state patches (Konami CPU, QSound, QSound Z80 bank) applied at build time by build.sh; the save probe also fails a game that freezes after loading
- Device: a game's save must hold the modules of its chips some cores leave out (Konami CPU, QSound), so a core that saves them resumes those games
- Web: a game that cannot be saved offers pausing (it stays in memory) before archiving or closing it
- Device: save probe per game and core (a save that leaves out a CPU or the sound, or that the core refuses, means the game always starts over), catalog marks Konami CPU and QSound games, 'device roms saves' tests the library
- Device: a thumbnail of the same game under another version is found (title without its parentheses); a game that cannot be saved drops its stale automatic save
- Device: games the emulator cannot save whole (a CPU without saved registers, like Konami's) never resume from a save; the web only offers a fresh start and no save
- Panel: no landing page; the local panel opens on the device until linked, then on its rooms
- Web: Spanish uses 'encender' instead of 'prender'
- Web: closing a game from its room goes back to the rooms instead of 'room not found'
- Gui: no Fyne call after the app loop ends (quitting from the menu panicked in GLFW)
- Device: L2, R2, L3 and R3 never reach the core (mame2003-plus: L3 turned the game sound off for everyone, R2 opens MAME's menu)
- Web: Escape closes dialogs and menus by key name too (not only the key code); the CPU headline has the peak's precision
- Web: history shows when each game ended; the go-link CPU share hides below 1% machine load
- Docker: ROM and thumbnail folders are volumes (image owns them, a new volume is not root); panel opens one link per page; device CPU counts its game processes; CPU peak keeps its decimal, share capped at 100; stats popover above the dock
- Signalhub moves to its own repository (next to this one): compose, e2e, CI and make docker take it from SIGNALING_DIR / SIGNAL_DIR; docs updated
- Web: the guest join page loads with the lobby (no ineffective dynamic import warning)
- Panel: rooms play through the device's own socket (signalhub refuses the panel's LAN origin): room-mode login, join by room/invite/code, every request answered; client handshake before open; tested with a real game in Docker
- A11y: axe WCAG 2.1 AA checks in e2e (every page, both themes, linked device and rooms); one main landmark, room tab list holds only tabs; landing music no longer fails when closed while resuming
- Test: device CLI commands (roms, thumbnails, panel token, help, log file)
- Web: parsers refuse non-object messages, ICE servers are validated, room lists and early ICE candidates are bounded, a broken thumbnail never stalls the queue
- Test: saved game files are 0600
- Go 1.26.8 everywhere (govulncheck clean); device: picture size cap, disk space guard on uploads, 20 saves per room, private saves, core hash checked before loading, guest control rate limit, ICE candidate cap, panel host check and bounded waiting sockets
- Docs: CDN security headers are applied
- E2e: Playwright tests of the whole story (landing, pairing, remembered link, test room video, single-use invitation, local panel); the join page's PIN error is shown
- Gitleaks: fingerprints of the test tokens after the history rewrite
- Gitleaks: fingerprints of the test tokens after the history rewrite
- Ci: GitHub Actions for signalhub, the device, the website and secrets
- Gitleaks: fingerprints of the test tokens after the history cleanup
- License: MIT; the kickoff brief stays local
- Device: x/image v0.43.0 and x/net v0.56.0 (govulncheck: no reachable vulnerabilities)
- Signalhub: device_secret is required on register
- Security: bind device_id to a device secret, the device proves itself before a browser's token, panel login by nonce proof, connection and message limits, hardened coturn, PIN lockout spares the host and guests, CDN security headers ready
- Docs: neutral Spanish in the signing example
- Release: MacDub-style release script (every platform, SHA256SUMS, Homebrew cask, GitHub pre-release), Developer ID signing and notarization ready
- Web: the landing is the home page and the rooms move to /rooms; Spanish copy without voseo
- MacOS: drag-to-Applications .dmg in the go-link style (make device-dmg)
- Device: keep Go 1.25 (x/term v0.40.0), as the build containers use
- MacOS: the device ships as go-link.app with its icon, so the Finder opens no Terminal; logs go to a file without a terminal
- Device: local web panel for headless devices (UUID token, WebRTC only), Docker image, and libvpx/Opus linked statically
- Landing: retro chiptune music with the fight's hits, and a mute button in the arcade screen
- Windows build: link winpthreads for libvpx and build Opus without run time CPU detection on ARM
- Landing: How it works page with a quick join, a retro fight animation, steps, controllers and the legal note; Linux build gets Wayland
- Tests: run the room passes test
- Invitations: one PIN per person, return tokens and an owner key; room: compact header, chat button, sound on by default
- My device: test pattern button and an invite to test with someone else
- History: who played at each port, their names and the address they came from
- Theme: light mode next to the default dark one, switched from the header
- Loading: wait for the device's status and room art before showing the page; skeletons instead of empty lists
- History: ask the device again once the data link is up
- My device: game history tab, skeletons while loading, infinite scroll for ROMs and a branded loading screen
- Icons: the brand mark as favicon, web app icons and the device's app and tray icon
- Rooms: no join-by-code for owners; ⋯ menus are never cut by the list or the cards
- Rooms: New game is a + circle at the right of the filters, no longer in the header
- Guest join page in the pairing page's layout: left-aligned fields with icons, a sample invitation and the steps
- Rooms and ROMs: more space between the filter chips and before the cards or list
- Header: signaling server and linked device as 36 px circles with tooltips
- Security: every room is private with an invitation and a PIN renewed on each start; guests get a code and PIN form, a Guest badge and the room details from the device
- Room: players on the left and the dock on the right at the same adaptive size; phone layouts that do not cover the game; gamepad buttons with depth
- Web: capsule buttons across the site (36 px, icon circles with tooltips), and a room with one dock over the video for start, voice and tools
- Room: the stream figures sit behind a fixed-size (i) button that opens the details
- Rooms: guest view without owner actions, room chat the host can turn off (enforced by the device), and players in a floating capsule over the video
- Input: keyboard maps saved before the start buttons move to 1-4 = 1P-4P, keeping custom keys
- Invitations: signalhub makes an invite link and a 9 digit code per room, private rooms admit only them; device shows them and renews them; web joins by /g link or code, with an Invite dialog and QR
- Web: pages other than the lobby load on demand, so no chunk is over 500 kB
- Chat: typing indicator relayed by the device, a chime for new messages, and a panel that can be hidden with an unread count
- Private rooms: a 6 digit PIN the device checks before streaming, with limited tries, a New PIN action and a lock in the lobby
- All pages share the My device look: PageHero, chips and cards; New game with a searchable game picker and a preview
- Room page in the style of My device: hero with the game's picture, chips and actions, and cards
- Controls panel: outline drawings of the detected gamepad (PlayStation, Switch Pro, Xbox or generic) whose parts light up
- Rooms: start buttons of players 1 to 4, swap controllers between seats, and a clear message for the games limit
- Rooms page: the list view is a table with columns, like the ROMs list
- My device: Space used card with ROMs, thumbnails and saved games instead of Current room
- Rooms page: search, count and cards or list view
- Home page is Rooms: public rooms of everyone plus the host's own with all states and actions; My device loses its Rooms tab
- Lobby: drop the side cards; How it works in the header opens the help dialog
- Lobby: the approved design, wide grid of cards with the room's thumbnail on the left
- ROMs tab: search box, and Play asks for name, public or private, voice and the lobby picture
- Device window: bigger by default, resizable with a minimum, remembers its size (and place on macOS)
- Device window: Settings section (thumbnail kind, size and folder, rooms, network) and thumbnails dir/kind CLI
- Docs: thumbnail kind from Settings, room action icons
- Web: show the thumbnail kind the host chose; room actions as icons with tooltips and a ⋯ menu
- Settings service for thumbnails (folder, kind, size); never start a new room twice
- Fix room actions being dropped (text id), goroutine dump on SIGUSR1, quieter input logs, clearer room errors
- Rotate vertical games in the core, and keep game workers out of the device's Ctrl-C so they are saved first
- Deploy: let a device keep several rooms open (one per game)
- Web: My device › Rooms to run several games, with favorites, pause, saves, archive and trash; Boxart in the lobby
- Game server: several rooms at once, each game in its own process, with pause, save states, archive, trash and favorites
- Thumbnails: the host's Boxart, Title and Snap in the device window (Emulators › MAME) and on the web

### 2026-09-26

- Remove the free ROM download: the app only says it is based on MAME and ROMs are found online
- My device: ROMs tab with cards and list views, Play opens create room with the game chosen
- My device: live dashboard with CPU, memory, network and ROM storage, and a new pairing view
- Switch Node in the same shell, not a subshell
- Build the website with the Node pinned in frontend/.nvmrc
- One persistent header for every page, rendered once outside the routes
- On-screen gamepad for phones and tablets, full screen, how to play dialog, one header everywhere
- Room state carries the game's controls from the mame2003-plus list
- Web in English, Spanish and Portuguese with a three-bubble language switch
- Remap keys and gamepad buttons, pick each pad's player, close the game
- One UDP port for WebRTC and announced addresses, for a direct path
- Show whether media flows directly or through the TURN relay
- Pause the game for everyone, and anti-spoofing DNS notes
- Website live on go-link.org: logo, CDN invalidation from a local file
- Makefile for web, device and signalhub builds and the website upload
- Deploy: Caddy always on in production, with automatic certificates
- Deploy: the production server builds signalhub from source
- Deploy: production compose for a small server, running on a small server
- Rename the project to go-link, on go-link.org
- Window: capsule Open in browser button on the pairing screen
- Window: keep the code countdown centered when Copy code changes
- Window: copy the pairing code from the pairing screen
- Window: roomier pairing screen with numbered steps
- Window: move by the top band, no dark frame, responsive cards
- Remember linked browsers, and a native window in MacDub's style
- Signalhub: reach, to come back to a linked device without a code
- Device: native window with Fyne, and no web server on the device
- Check ROM sets without running them, and manage the device from the CLI
- Emulator: answer every core option with its declared default
- Set the ROM folder and drop ROMs from the web over WebRTC
- Create game rooms from the web, and download the emulator core
- Docs: native device GUI planned with Fyne, replacing the local web panel
- Emulator: libretro frontend running mame2003-plus, playable in the browser
- Voice between players: separate tracks, SFU forwarding, volumes
- Free ROMs from mamedev.org, downloaded by the device on request
- Room Manager: seats P1-P4, arcade queue, spectators and chat
- Web: controls panel, keyboard player choice, and early offer fix
- Input v2 with gamepads and local players; test card draws a gamepad at 60 fps
- Test room: PM5544-style test card and 1 kHz reference tone
- Device status and P2P latency over WebRTC for linked browsers
- Phase 5 step 1: test pattern video over WebRTC and controller input
- Device: avoid a double period in the panel's next-code line
- Frontend: phase 4, shared package and web with the four design screens
- Docs: help message when a room is not found on the current server
- Docs: custom signaling server selectable from the web (localStorage)
- Device: --server-signaling flag and public default server
- Device: phase 3 skeleton, and deploy stack with coturn
- Docs: signalhub stays in-memory, drop metrics and Redis from roadmap
- Signaling: signalhub v0.2 rooms, directory, rate limit and TURN
- Signaling: signalhub v0.1 pairing and signal relay
- Docs: architecture, signaling protocol and UI prototype
