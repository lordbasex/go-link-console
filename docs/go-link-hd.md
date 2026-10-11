# go-link HD: the engine's base (specification)

go-link HD is step 3 of Willy Maker's [board ladder](willy-maker/vision.md#the-board-ladder): go-link Console's own 2D engine, a "virtual board" with modern console-class quality. Its games play in a go-link Console room like any MAME set does. This page lists, before anything is built, **what the engine supports and what could be done with it**. It is the base for the roadmap ([status.md](status.md)); nothing here exists yet except the streaming path, which the [HD experiment](experiments/hd-streaming.md) proved.

Every item is marked:

- **Base:** the first version must have it.
- **Later:** planned, after the base.
- **Idea:** possible and worth keeping in mind; not committed.

## Decisions taken (2026-10-06)

| Question | Decision | Why |
|---|---|---|
| What it is | **An engine of our own with its own API** (`libgolinkhd`, `include/golink_hd.h` in the `golink-hd` repository) that runs a game package; the device runs it in a worker process per room, next to MAME (which it runs through libretro) | Rooms, voice, controllers, recordings, invitations, saves and phones work for both engines. go-link HD is meant to become go-link Console's main engine and MAME the second one |
| Look | **Pixel art on a 640 × 360 screen**, scaled ×3 to 1080p (×6 to 4K); real HD art (1920 × 1080) supported too | Pixel art is quicker to draw and to make with an image AI, and fits the arcade look; HD art stays possible for who has it |
| Language | **C99**, with no dependencies (changed on 2026-10-07; it was Go) | Portable to every platform and to WebAssembly for Willy Maker's play mode (clang's wasm32 target); go-link Console's device calls it from Go (`pkg/golinkhd`, cgo) |
| Its own API, not libretro's (2026-10-08) | A small versioned API: create, load a package, one frame (controllers in, picture and sound out), save states | It first followed libretro's API, which inspired it; its own lets it grow with what go-link Console needs (events for the room, views per player, several engines in one program) without libretro's limits. MAME keeps running through libretro |
| Repository | Its own public repository, `golink-hd` (next to this one, like signalhub) | libretro takes cores from their own repositories; the core knows nothing about go-link Console |
| Compatibility | Versioned save states (`GLHD` header, layout version, checksum, little endian words) and versioned packages | A save state or package of another version is refused cleanly, never misread; a new core keeps reading older packages |
| First genre | **Platformer** (then the beat 'em up) | Both have engines and level-checked rules on the CPS-1, so the HD version can be compared with them |
| License | MIT, like go-link Console | Our own code: no MAME license (non-commercial) limits |

## How it fits go-link Console

```
Willy Maker project ──export──► game package (.glhd)
                                     │
             ┌───────────────────────┴───────────────────────┐
             ▼                                               ▼
  browser: engine in WebAssembly                 device: libgolinkhd (its own API)
  (play mode, power-on test)                     (game room worker, like MAME)
                                                             │
                                          HD path: H.264 or VP8, size per host
                                          and per guest (video tiers)
                                                             ▼
                                                 guests (web, iOS, Android)
```

- **One simulation, two renderers of the same pixels:** the engine renders in software into a frame buffer, in both builds, so the browser and the device show the same pixels. A 640 × 360 frame is small enough for software rendering, and the HD experiment drew a 1080p scene in under 1 ms. The scaling to the output size is done after that.
- **The host does all the work:** guests only receive the stream, as with MAME. A phone never runs the engine, except in Willy Maker's play mode.

## 1. Simulation

| Feature | Level | Notes |
|---|---|---|
| Fixed step of 60 Hz, independent of the output's frame rate | Base | One `Run()` per libretro frame |
| **Deterministic**: the same inputs give the same frames on every computer and in the browser | Base | Integer and fixed-point math for positions, speeds and collisions; no `float` in game logic (compilers may fuse or reorder float operations differently on each CPU); one seeded random generator in the state |
| Whole state in one serializable struct | Base | Save states are exact by design: no pointers to code, no hidden timers. That avoids the problems [the MAME patches](../cores/mame2003-plus/README.md) fight |
| Input log replays (start state + inputs) | Base | For bug reports, regression tests, the AI playtester ([row 12](status.md)) and attract-mode demos |
| Rewind (a ring of recent states) | Idea | Cheap with a small state; useful in Willy Maker's play mode |
| Entities with components (position, sprite, body, behaviour, health…) | Base | Data-driven, as Willy Maker's engines are today |
| State machines per entity (idle, walk, jump, hurt…) | Base | The genres' rules are written this way already |
| Timers and events (on hit, on enter zone, on clear) | Base | |

## 2. Picture

### Screen

| Feature | Level | Notes |
|---|---|---|
| Logical screen 640 × 360, 16:9 | Base | The default |
| Other logical sizes: 320 × 180, 480 × 270, 960 × 540, 1920 × 1080 | Base | Per game; all 16:9 so 1080p and 4K scale exactly |
| Integer scaling to the output (×3 → 1080p, ×6 → 4K), nearest neighbour | Base | Done by the device before encoding; guests get the size from the [video tiers](experiments/hd-streaming.md#chosen-per-viewer-2026-10-03) |
| 4:3 and vertical (9:16) games | Later | Vertical needs the rooms to show a portrait picture well |
| 30 fps games | Idea | Half the encoder's work for slow genres (quiz, puzzle) |

### Color

| Feature | Level | Notes |
|---|---|---|
| 32-bit color (RGBA 8888), no palette limits | Base | The opposite of the CPS-1's 15 colors per tile |
| Alpha (half transparency) on sprites and layers | Base | |
| Palette swap for players 2 to 4 (shirt colors) | Base | A color table per sprite, as Willy Maker recolors players today |
| Blend modes: normal, add, multiply, screen | Base | Add for fire and lights, multiply for shadows |
| Tint and flash (an entity turns white when hit) | Base | |
| Fades and color grading with a lookup table (night, sepia, underwater) | Later | |

### Layers

| Feature | Level | Notes |
|---|---|---|
| Up to 8 tile layers with their own tile size (8, 16, 32 px) | Base | Background, play layer, front layer, HUD |
| Parallax: each layer scrolls at its own speed | Base | |
| Per-line scroll (raster effects: waves, heat, water) | Base | Like the CPS-1's row scroll, without its limits |
| Animated tiles (water, lava, lights) | Base | |
| Big picture layers (a full painted background instead of tiles) | Base | For HD art |
| Layer rotation and scaling ("Mode 7": floors seen in perspective) | Later | Racing seen from behind and flying stages |
| Per-line scaling (pseudo 3D roads) | Later | The classic arcade road |

### Sprites

| Feature | Level | Notes |
|---|---|---|
| At least 1024 sprites on screen, any size | Base | No sprites-per-line limit |
| Flip, rotation, scaling | Base | |
| Sprite sheets and animations with their own speed per animation | Base | Willy Maker's characters already work this way |
| Sorting by depth (beat 'em up) and by layer | Base | |
| Outline and shadow generated by the engine | Later | |
| **Skeletal animation**: bones, attached parts, interpolated keys | Later | Smooth big characters with few drawings; the editor needs a bone tool |
| Mesh deformation (cloth, flags) | Idea | |

### Effects

| Feature | Level | Notes |
|---|---|---|
| Camera shake, hit stop (the freeze on a strong hit) | Base | |
| **Particles**: emitters with rate, life, speed, gravity, color over time, blend mode | Base | Sparks, dust, explosions, rain, snow |
| 2D lights: point lights and a darkness level | Later | Caves, night stages |
| Normal-map lighting on sprites | Idea | Needs a second picture per frame |
| Post effects: bloom, blur, distortion waves, pixelate transition | Later | In software: each costs CPU per frame, so each gets a measured budget |
| Shaders written by the user | Idea | Hard to keep identical between WebAssembly and the device; would need a small own shader language |
| CRT and scanline looks | Not in the engine | The rooms already offer them in the guest's picture settings |

### Text

| Feature | Level | Notes |
|---|---|---|
| Bitmap fonts (pixel fonts) and scaled fonts | Base | |
| UTF-8, the game's texts in English, Spanish and Portuguese | Base | The player picks the language in the game's menu |
| Dialog boxes with typing effect, portraits | Later | |

## 3. Camera

| Feature | Level | Notes |
|---|---|---|
| Follow one or up to 4 players, keeping all of them on screen | Base | Today's rule in Willy Maker |
| Locks (the camera stops until a wave is cleared) and zones | Base | |
| Forward only, or free in both axes | Base | Per level, as today |
| Zoom in and out (players far apart, bosses) | Later | |
| Split screen for 2 to 4 players | Idea | Racing; costs drawing each view |

## 4. Sound

| Feature | Level | Notes |
|---|---|---|
| Mixer at 48 kHz stereo (the stream's Opus rate) | Base | |
| 32 channels of effects with volume, pitch and pan | Base | |
| Music as a stream (OGG or Opus) with a loop point | Base | |
| Music crossfade between stages and when a boss appears | Base | |
| Effects: low pass (underwater), echo | Later | |
| Tracker music (MOD/XM) | Idea | Small files, chiptune style |
| Positional sound (left or right by where the source is on screen) | Later | |

Voice chat between players stays go-link Console's; the engine only makes the game's sound.

## 5. Controls

| Feature | Level | Notes |
|---|---|---|
| 4 players | Base | Ports P1-P4 of the room |
| D-pad, 6 buttons, Start, Coin per player | Base | What go-link Console's `input` packet carries ([protocol](protocol.md#input-channel)) |
| Analog sticks | Base | The packet already has left and right stick X and Y |
| L2, R2, L3, R3 | Later | The packet has the bits; MAME games never get them (`retroButton` in `game_core.go`), our engine can |
| Up to 8 players | Idea | Needs more ports in the room |
| Rumble | Idea | Needs a channel from the device to the guest |
| On-screen controls per game | Base | The room's touch pad takes its buttons from the game package (as `room_state.controls` does for MAME): buttons 1 to 4 (B, A, Y, X) jump, jump, run, run, unless the manifest's `weapon` fires ("Fire") or throws its `super` ("Special") with Y or X (`glhd.Manifest.Labels`) |

## 6. Game rules

| Feature | Level | Notes |
|---|---|---|
| Tile collision with tags (solid, one-way, ladder, hazard, water…) | Base | Willy Maker's tags |
| Slopes | Base | The CPS-1 engines have none |
| Moving and falling platforms | Base | |
| Simple physics: gravity, friction, bounce, push | Base | |
| Hit and hurt boxes per animation frame | Base | Fighting and beat 'em up |
| Grid pathfinding (A*) for enemies | Later | Maze and top-down genres |
| Rigid bodies (crates that tumble, ragdolls) | Idea | |
| **The 13 genres of Willy Maker** as modules of the same engine | Base: platformer, then beat 'em up. Later: the others | [genres.md](willy-maker/genres.md) |
| Behaviours built from blocks, Scratch style (when / if / do) | Later | So a game can do something no genre module has |
| A scripting language | Idea | Only if blocks are not enough; must stay deterministic and safe |

## 7. Saves and progress

| Feature | Level | Notes |
|---|---|---|
| Save states (libretro serialize) | Base | Exact by design (section 1); the room's save slots and automatic save work at once |
| Persistent data per game: high scores, unlocked stages, options | Base | Like MAME's NVRAM, kept by the device per room |
| Checkpoints and continue | Base | |
| Achievements | Idea | |

## 8. Game package

| Feature | Level | Notes |
|---|---|---|
| One `.glhd` file (a zip): manifest, levels, characters, rules, pictures (PNG), sound, fonts | Base | Built by Willy Maker's export; never compiled |
| Manifest: title, version, engine version, logical size, players, controls, languages, box art | Base | The device shows title, art and controls from it, as it does for go-link Console's own sets |
| Identified by its SHA-256 | Base | As go-link Console's own CPS-1 sets are today |
| Format and engine versions, with old games running on newer engines | Base | |
| Size limit (proposed 256 MB) | Base | |
| Import from Aseprite and from sprite sheets | Later | The sheet detection exists in Willy Maker |
| Signed packages | Idea | Only matters if packages are shared publicly |

## 9. In go-link Console

| Feature | Level | Notes |
|---|---|---|
| The device runs the engine in a worker per room like MAME: `device emulate --golinkhd --core libgolinkhd` (done 2026-10-08) | Base | |
| The engine's library ships with the device (done in 0.2.8) | Base | MAME's core keeps coming from the libretro buildbot |
| The HD room path for game rooms: H.264 or VP8, size per host (`hdprobe`), size per guest (tiers) | Base | Today only the HD test room uses it |
| Recordings in H.264 | Base | Today they are VP8 in WebM; H.264 needs MP4 or Matroska |
| ROM library: `.glhd` files next to the zips, validated by the engine | Base | `romcheck` only knows MAME sets |
| Game picture as thumbnail and box art from the package | Base | |
| Raspberry Pi hosts | Later | 640 × 360 surely; 1080p depends on the Pi's encoder, to be measured |

## 10. In Willy Maker

| Feature | Level | Notes |
|---|---|---|
| **go-link HD** as a board next to the CPS-1 | Base | A board profile: its limits, its checks, its export |
| Play mode runs the engine in WebAssembly | Base | The same game as the room |
| Art without the 15-color and 16 px zone rules | Base | The pixel editor keeps its tools; the board's limits come from the profile, as they already do |
| Moving a CPS-1 project to go-link HD | Base | The other way can be lossy and is checked by the validator |
| Power-on test in the browser and on the linked device | Base | Validation levels 3 and 4 ([validation.md](willy-maker/validation.md)) |
| Debug view: hit boxes, collision tags, entity inspector, frame step | Base | |
| Performance meter: milliseconds per frame, sprites, particles | Base | |
| Bone and particle editors | Later | |

## 11. Budgets (proposed, to be measured)

| Item | Budget |
|---|---|
| Engine time per frame on the host | ≤ 4 ms at 640 × 360 (the encoder takes the most) |
| Sprites on screen | 1024 |
| Tile layers | 8 |
| Particles alive | 4096 |
| Sound channels | 32 effects + 2 music streams |
| Graphics in memory | 512 MB uncompressed |
| Save state | ≤ 2 MB |
| Package | ≤ 256 MB |

## 12. Not in scope

- **3D.** go-link HD is a 2D engine; 3D-looking effects come from layers (Mode 7, roads).
- **Netplay or online code in the game.** go-link Console streams from the host; the engine never talks to the network.
- **A store or paid games.**
- **Running MAME sets.** MAME stays the core for arcade games.

## Phases

| Phase | What | Rough size |
|---|---|---|
| 1 | The core with the base picture, sound, controls and save states; a platformer demo playable in a room (proof of concept done on 2026-10-07: the engine and its demo in the `golink-hd` repository, game packages (`.glhd` format 1: manifest, level and optional PNG pictures, documented in that repository's README), and the device plays it in the test room with `--test-room-hd 1080p --hd-core PATH`; `e2e/tests/hd-core.spec.ts` checks it through WebRTC) | 3-4 sessions |
| 2 | Game rooms on the HD path; H.264 recordings (proof of concept done on 2026-10-07: `.glhd` packages in the ROM folder are listed and start game rooms with go-link HD's core at 720p or 1080p; the tiers, H.264 and recordings for those rooms are next) | 2 sessions |
| 3 | Willy Maker's go-link HD board: export, play mode in WebAssembly, validation (proof of concept done on 2026-10-07: a platformer's Export tab downloads a `.glhd` with its first level and the city tiles, [file-format.md](willy-maker/file-format.md#the-go-link-hd-package-glhd-beta); the end to end test plays it in a game room) | 3-4 sessions |
| 4 | Later items: skeletal animation, lights, post effects, Mode 7, more genres. **Part A, the engine's effects, done on 2026-10-08** (repository `golink-hd`, its README lists them with their measured cost): 4:3 and vertical screens, up to 8 players, both sticks and L2/R2/L3/R3, blend modes, outlines and shadows, sprites turned and scaled, Mode 7, the pseudo 3D road, skeletal animation, fades and color grading (presets and LUTs), bloom, blur, waves, pixelate, 2D lights, the camera's zoom, dialog boxes with accents, low pass and echo, A*; package format 2 carries a level's effects; the showcase demo has a scene for each. Next: part B (the other genres as engine modules) and part C (Willy Maker's bone, particle and block editors, Aseprite import) | by parts |
| 5 | Distribution: **done on 2026-10-08 in go-link Console 0.2.8, shipped inside the device** instead of downloaded: the engine's repository is public (github.com/lordbasex/golink-hd, its own CI on Linux, macOS and Windows), every device build compiles its committed HEAD (`make golinkhd-src`, `GOLINK_HD_DIR`) into go-link-console.app's Frameworks (universal), the Linux and Windows archives and the Docker image, and the device lists the engine's built-in games (the platformer and the showcase) | done |
| 6 | **Code (Willy Maker's second evolution):** games with their own scripts, compiled to a WebAssembly module that the engine runs sandboxed, with its memory in the save state and a per-frame budget; blocks in Willy Maker generate the same code ([code.md](willy-maker/code.md), its phases C1 to C5) | 10-15 sessions |
| 7 | **API v2, system services:** like a console's system libraries, a game asks the host for what the platform shows the same way in every game, and go-link Console (the room) decides how: achievements, the player's progress saved apart from save states, game events for the room ("level 3 cleared", "P2 is out", shown in the room and the lobby, recorded in the room's telemetry), a view per player, rumble. Functions are only added (`GOLINKHD_API_VERSION` 2); a v1 host keeps working | 3-4 sessions |
| 8 | **A draw list:** the engine records each frame as a list of drawing commands (layers, sprites, effects) and a backend runs it: today's software renderer, later WebGL or Metal where it is faster. The game's logic never changes, and the hash gates decide whether a backend draws exactly the same (answers open question 1) | 3-4 sessions |
| 9 | **The AI kit:** what an AI needs to make a whole game with the SDK alone, without Willy Maker: (1) the API reference as one document, every function of the host API, the scripting API (phase 6) and the system services (phase 7) with its rules, limits and budgets; (2) the scripting API's type file, so the AI's code is checked before it runs; (3) small complete examples and templates (a platformer, a top-down game, a menu, saved progress); (4) a command-line loop the AI drives by itself: write the code, `glhd pack` and `check`, `hdrun` with a button script and screenshots to see the result, `bench` for the frame budget and the hash gates for regressions, with readable errors that name the file and line. Its test: an AI given only the kit builds a 2D mining and building game (side view: blocks to dig and place, caves lit by torches, creatures that chase with A*, day and night, crafting, the world kept as saved progress, up to 4 players in a room), with its own names and art | 3-4 sessions, after phases 6 and 7 |

## The go-link HD SDK

Together, these parts are go-link HD's SDK, open (MIT) and free, the way a console maker gives its developers a kit: the API (`include/golink_hd.h`, versioned, only functions added), the game format (`.glhd` packages), the tools (`tools/glhd` to pack and check, `tools/hdrun` to play and record without a screen, `tools/bench`), the conformance tests (the hash gates every engine build and backend must match), the test bench (go-link Console's rooms and Willy Maker's play mode), the editor (Willy Maker, visual and later with code, [code.md](willy-maker/code.md)) and the scripting API. Phase 7 adds the system services and phase 9 the AI kit (the reference, the type file, examples and the command-line loop); the kit is published as one download, with its documentation and examples, once the scripting API exists.

## Open questions

1. Whether Willy Maker's play mode should render with WebGL instead of software when the game is HD art at 1080p (faster in the browser, but pixels may differ from the device).
2. How the bone editor fits Willy Maker's pixel editor.
3. Whether a game can mix pixel art and HD art layers (a pixel art hero on a painted background).
4. The Raspberry Pi as a host for HD games.
