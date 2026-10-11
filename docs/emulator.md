# Emulator, ROMs, thumbnails and saves

## libretro frontend

The device is a libretro **frontend**, like RetroArch: it loads a **core** (the emulator, a `.dylib`, `.dll` or `.so` library) and runs it frame by frame. Instead of drawing the picture in a window, it sends it over WebRTC. The glue is `backend-device/pkg/libretro` (cgo, one active core per process, callbacks routed through an atomic pointer).

- **Core:** **mame2003-plus** (MAME 0.78), kept in `~/go-link-console/cores/`. It is downloaded from the official libretro buildbot, for the host's system and CPU: macOS (Intel and Apple Silicon), Windows x64, Linux x64 and 64/32-bit Linux ARM (Raspberry Pi). Download it from the website (My device) or with `device core download`. It is never bundled with go-link.
- **One process per game:** a core allows one instance per process, so each game room runs `device emulate …` as a child process (`WorkerSource`, protocol in `pkg/emuproc`). The worker sends video and sound to the device through a pipe; the device encodes and streams. A crashing game does not take the others down.
- **Picture:** the core delivers XRGB8888 or RGB565 at the game's native resolution (for example 320×224). The worker converts it to I420 in one pass, as the host's [video quality](device.md#video-quality) says: enlarged 2x with nearest neighbour (`libretro.ToI420Double`, High and Normal: each game pixel keeps its own color sample) or at its own size with each 2x2 block's color averaged (`libretro.ToI420Box`, Saver). The mode travels with each frame (`emuproc` video messages carry the scale; the parent switches it with a `VideoMode` message, for a live change or the automatic fallback). The device encodes VP8 at that size and tells viewers the scale and the game's size (`stream_stats.video`, see [protocol.md](protocol.md#video-scale)); the website averages a 2x picture back to the game's pixels before its styles and scales it with the display aspect (`aspect`). The [video quality lab](quality.md) measured the choice.
- **Sound:** stereo 48 kHz (a core option), straight to Opus.
- **Timing:** each game runs at its own rate (60 Hz, 57 Hz…) on a goroutine locked to an OS thread.
- **Core options:** the device answers every `GET_VARIABLE` with the default the core declared in `SET_VARIABLES`. Otherwise mame2003-plus reads 0, and with gamma 0 every color channel is either off or full, which gives flat colors.
- **Controls:** seat P1 to P4 is MAME player 1 to 4. RetroPad B, A, Y, X, L, R are buttons 1 to 6; Select is Coin; Start is Start.

## Where ROMs come from

go-link Console is based on MAME: the engine is mame2003-plus, which plays **MAME 0.78** sets.

- The project **does not** include, host, proxy or download ROMs or thumbnails, in the repository, the binaries or the website, and does not point to any site that offers them.
- The host gets compatible sets on their own and puts them in the device's ROM folder (`~/go-link-console/roms` by default), or drops them on the device window or on the linked website (they go straight to the device over WebRTC).
- ROMs never pass through the signaling server. The browser never stores them (no OPFS or IndexedDB): the emulator runs on the device.
- Every host is responsible for using only ROMs they have the right to use: sets you may use, or backups of arcade boards you own.

## ROM validation without running the game

Each MAME version expects different files inside each `.zip`. A modern MAME romset has other names, other revisions and new files, and many games fail on mame2003-plus ("Required files are missing"). The device finds out **without running the game** (`pkg/romcheck`):

1. **The core's game list.** The mame2003-plus project publishes an XML with the 5,000+ games it supports: name, title, year, manufacturer, parent and each ROM's name, size and CRC32, plus the control panel of each game. The device downloads it with the core, converts it to a compact index (`~/go-link-console/cores/mame2003_plus.romcheck`) and does not keep the XML.
2. **The index of each `.zip`.** A ZIP keeps the list of its files, with name, size and CRC32, at its end. The device reads only that list, without decompressing, so checking a whole folder takes milliseconds.
3. **The same rules as the MAME 0.78 loader:**
   - Each ROM is looked up in the game's zip and then along its parent chain (`romof`: the original game of a clone, or a BIOS such as `neogeo`).
   - It is found **by name** or **by CRC32** (a renamed file works).
   - Found by name with another size or CRC, MAME warns but runs: it counts as present.
   - Missing is fatal, except ROMs marked `nodump` (no known dump exists). Only the default BIOS is required.
   - Games with a hard disk need `<folder>/<game>/<disk>.chd`.

Result per game, in `check.status`:

| `status` | Meaning |
|---|---|
| `ok` | Every file is there: it runs |
| `missing` | Files are missing. `missing` lists up to 10, and `needs` the zips of the parent chain that are absent (for example `neogeo`) |
| `unsupported` | The game is not in mame2003-plus (added to MAME later) |
| `bios` | A BIOS, not a game (for example `neogeo.zip`) |
| `bad_zip` | Not a valid ZIP |

`check.driver` warns when the MAME driver is marked `preliminary` or has unemulated `protection`: it may start and fail later. Title, year and manufacturer come from the XML. In a calibration against 26 games tested for real, the prediction matched all 26. The website disables games that are not `ok`, with the reason, and the device refuses to load them anyway.

The game list also tells the website each game's controls (`<input buttons control players>`), so the touch gamepad shows the right buttons. An outdated list is downloaded again when the device starts.

## Thumbnails

Thumbnails are the host's own image files, never downloaded by go-link. They follow the libretro naming, one folder per kind, under `~/go-link-console/thumbnails/MAME` (configurable):

```
Named_Boxarts/<name>.png    the box or flyer
Named_Titles/<name>.png     the title screen
Named_Snaps/<name>.png      a screenshot while playing
```

`<name>` is the MAME set name (`galaga`) or the game title with `&*/:`<>?\|"` replaced by `_`. When there is no exact match, an image of the same game under another version is used (the title without what is in parentheses). The host chooses which kind is shown (Settings, or `device thumbnails kind`). The device sends browsers small JPEG copies (`get_thumb`), and pictures larger than 4096 px per side are refused.

Note: in MAME, "artwork" means bezels and overlays, so these images are always called thumbnails.

## Save states

- **Save:** `room_action save` stores the game state (a libretro save state) in `~/go-link-console/saves/<room>/slot-N.state`, with an optional name.
- **Automatic save:** archiving a room or stopping the device writes `auto.state`. Turning the room on again offers to continue from it, start fresh or load a slot.
- **Games that cannot be saved whole:** MAME 0.78 does not save every chip. For example, the Konami CPU (The Simpsons, Aliens) saves no registers, and some sound chips (QSound) save nothing. Loading such a save breaks the game: the board resets, loops in its RAM/ROM check, or plays without sound.

The device detects these games instead of guessing:

1. **When a game starts**, the worker saves once in memory and reads the list of what the core saved. Every CPU must save its program counter as `<cpu>.<n>.PC`, numbered from 0, and the core must not refuse to save.
2. **A save probe** (`services.ProbeSaves`) runs the game in two worker processes (`emulate --probe save|load`), pressing Coin and Start every 4 seconds, saves in one and loads in the other. The game fails when a chip module its hardware needs (from the game list, for example `konami` or `QSound`) is missing from the save, when the sound goes silent after loading, or when the picture freezes.
3. The result is cached per ROM and per core (`<cores>/saves.json`, keyed by SHA-256), so each game is probed once.

A game that fails is marked `no_saves`: it never loads a save (it always starts from power on), makes no automatic save, `save` answers `code: "no_saves"`, and the website offers pausing (the game stays in memory) instead of archiving. `device roms saves` probes the whole library from the CLI.

## Core patches

`cores/mame2003-plus/` holds optional patches that add the missing save state code to the core (Konami CPU, QSound, the QSound Z80 bank, CPU suspend, YM2151 timers, sound latches, the Simpsons and Aliens drivers, the Namco wave chip, the YM3812, the interrupt queue, each CPU's local time, the sound mixer, the YM2151 connections on load, the K054539 channels, and Galaga's stars, custom I/O and 54XX). They are applied at build time on a clean upstream checkout and are **not used** by releases yet: the device downloads the official core. See [its README](../cores/mame2003-plus/README.md).

## Licenses

MAME and the libretro cores have their own licenses. **mame2003-plus is for non-commercial use only.** Review this before any monetization.
