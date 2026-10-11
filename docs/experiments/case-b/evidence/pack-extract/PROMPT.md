# Willy Gorklingo: build the ROM

You are building an arcade game ROM for go-link with the tools in the go-link repository (rom/). Target board: Capcom CPS-1 (384 × 224 at 60 Hz, 68000 main CPU), laid out as the files of the `slammast` set: 4 players × 3 buttons, run by the mame2003-plus core (MAME 0.78). Every byte must be original: no code, graphics, music or text from any existing game.

Follow docs/rom-README.md, docs/art-spec.md and docs/hardware.md (in this pack). docs/journal.md is the lab journal of the prototype that already runs in the core and in a go-link room: its section "How to reproduce from zero" is the build you start from. docs/story.md is the game bible: the world, the heroes and the tone.

The rules the game must keep (jump about 64 px, push-climb 32 px, one-way ledges, ladders, double-tap run, automatic knife) are the ones Willy Maker's play mode used; they are listed below with their numbers, the same as rom/src/main.c.

## The game

- Title: "Willy Gorklingo" by go-link.
- Board: CPS-1, layout `slammast` (4 players × 3 buttons). Players: 4.
- Levels, in play order: 1.
  1. "Puerto Madero docks" (`level-1`, 1536 × 448 px, camera forward only with a 48 px margin back): levels/level-1.tmj
- Characters: none of its own. Use the prototype's Willy (rom/tools/art.mjs) for every player, with a different shirt per player.
- Buttons (Run is a double tap of the stick toward a side, within 15 frames):
  - Button 1: jump (down + jump drops through a one-way ledge)
  - Button 2: fire: the machine gun, aimed with the stick; the knife automatically when an enemy is right in front
  - Button 3: special: the picked-up weapon, with limited ammo (a grenade when there is none)
  - Start and Coin: join and credit, like any arcade board. go-link maps the RetroPad B, A, Y to buttons 1, 2, 3 on every seat.
- DIP switches (the slammast board keeps them in EEPROM): difficulty normal, 3 lives, free play off, demo sound on.
- Menus: title, attract, select, hud, continue, gameOver, highScores. Their blocks are in project.json (settings.menus); an empty screen gets a plain default with the 8 × 8 font.

## Rules the engine keeps

The numbers of Willy Maker's play mode (its engine/rules.ts), the same as the prototype's rom/src/main.c. Vertical speeds are in 1/16 px per frame, like the 68000 code.

| Rule | Value |
|---|---|
| Screen, frame rate | 384 × 224, 60 frames per second |
| Collision grid | 16 px |
| Player body | 40 px tall, 5 px half width at the feet |
| Gravity | 6/16 px per frame, per frame |
| Jump | start speed -112/16 px per frame (about 64 px high) |
| Fastest fall | 128/16 px per frame |
| Ladders | 24/16 px per frame (up and down on the stick) |
| Push-climb | edges up to 32 px (one 32 px crate), in 10 frames |
| Run | a second tap toward the same side within 15 frames |
| Drop through a one-way ledge | down + jump, for 12 frames |
| Camera | moves forward; goes back at most 48 px from the farthest point reached |
| Machine gun | 6 shots per player on screen, speed 6 px per frame, one every 7 frames |
| Knife | 16 frames, reach 18 px |
| Bazooka | 3 rockets, 24 frames each |
| Enemies | 4 hits, see 170 px, fire every 90 frames at 3 px per frame |
| Crates and breakable walls | 3 and 2 hits (a rocket counts 9, a knife 2) |
| Lives | 3 by default (the DIP switch above wins); 120 frames without harm after a respawn |
| Score | crate 100, enemy 500, rescued civilian 1000 |

## Board limits

- 32 sprite palettes, 32 for the play layer (scroll2, 16 px tiles) and 32 for the far layer (scroll3, 32 px tiles); the HUD and text use scroll1 (8 px tiles).
- 15 colors plus transparency per palette, per 16 × 16 tile or sprite zone. Colors are 12-bit: every channel a multiple of 17.
- Graphics ROM 6 MB, program ROM 2 MB, 256 sprite table entries.
- What this game uses now:
  - spritePalettes: 0 of 32
  - playPalettes: 1 of 32
  - farPalettes: 1 of 32
  - colors: 15 of 15
  - graphics: 0.01 of 6 MB
  - sprites: 42 of 256

## What is in this pack

```
PROMPT.md                 this brief
project.json              the Willy Maker project (format in docs/willy-maker/file-format.md of the repository)
review.json               Willy Maker's checks at export (below)
levels/<id>.tmj           Tiled maps: far (image), play, collision and objects layers
levels/<id>/far.png       the far layer (and the middle one merged in) as one picture
levels/<id>/play.png      the play layer as one picture
levels/<id>/collision.png the collision tags in the colors of docs/art-spec.md, on the 16 px grid
tilesets/<id>.png         each tileset, in board colors; tilesets/collision.png the tag tiles
characters/<id>/<anim>.png one strip per animation on magenta #FF00FF, feet on one line
characters/<id>/sheet.json frames (boxes in the strip), pivots, palette zones, fps, loop
docs/                     rom-README.md, art-spec.md, hardware.md, story.md, journal.md
```

Collision tags: solid, oneway, ladder, crate, breakable, hazard, water (air is empty). Objects have unique names and the types and properties of docs/art-spec.md section 4. Every picture is already in board colors; transparency is alpha 0 (strips use magenta).

## Willy Maker's checks at export

Ready: no errors, 0 warnings.
- info: No hero of your own yet: the game uses the built-in Willy.
- Passed: There is a player 1 start and an exit; The whole path can be walked with jumps, crates and ladders; No place where a player gets stuck; The camera can follow the way to every exit; Every level fits the board's map (up to 16384 × 2048 px); Every object has its properties; Every reference name is unique and valid; The title “Willy Gorklingo” fits the board's font; 4 players · 3 buttons assigned; Sprite palettes: 0 of 32; Every tile and sprite zone has 15 colors or fewer; Every color exists on the board; Graphics: 0.01 of 6 MB; Sprites on one screen: 42 of 256; Every background is on its layer's grid; Program and levels: 0.09 of 2 MB; scroll2: 9 different 16 px tiles of 49152; scroll3: 6 different 32 px tiles of 12288; Every background tile and sprite part has 15 colors or fewer.

## Build it

1. Tools (macOS with Homebrew, as in docs/journal.md; Linux has the same packages): the m68k-elf cross compiler, binutils and z80asm, and **Node 22.18 or newer** (rom/tools imports TypeScript from frontend/packages/cps1).
   ```sh
   brew install m68k-elf-binutils m68k-elf-gcc z80asm
   ```
2. Build the prototype once, to prove the toolchain: `node rom/tools/build.mjs` writes rom/build/slammast.zip.
3. Extend rom/tools/build.mjs (art.mjs, level.mjs and rom/src/main.c) to read this pack instead of the prototype's level: the maps and collision from levels/*.tmj, the tiles cut from the layer pictures (deduplicated, 15 colors per tile), the characters from their strips and sheet.json, the objects with their properties, the buttons, players and DIP switches above.
4. Get the core the device uses (`device core download`), build the frame capture tool and run the set in it:
   ```sh
   (cd backend-device && go build -o /tmp/framelab ./cmd/framelab)
   /tmp/framelab capture -log -core ~/go-link-console/cores/mame2003_plus_libretro.dylib -rom rom/build/slammast.zip -out /tmp/rom-run -frames 860 -script "60-63:coin 110-113:start 140-400:right"
   ```
   (The core is a .so on Linux.) The core's log must have no "WRONG CHECKSUMS", "NOT FOUND" or "INCORRECT LENGTH" line.
5. Play it in a real go-link room: `node rom/tools/room-test.mjs` (it needs the signalhub repository next to go-link).

## Task

Produce the ROM set (slammast.zip) with rom/tools/build.mjs extended to read this pack. Keep within the budgets above and the rules of docs/hardware.md (watch its pitfalls: sprites wrap at 512 px, write board registers with plain stores). Test it with framelab and in a go-link room, and report what you changed and anything that needs a decision.
