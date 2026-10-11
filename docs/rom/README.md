# Project: go-link Console's own arcade ROM

**Willy Gorklingo: The Lag Protocol** is a side-scrolling run-and-gun arcade game for up to four players, made by go-link Console with its own characters and released as a ROM set that the mame2003-plus core (MAME 0.78) runs, so it can be played in any go-link Console room and shipped freely, because every byte is ours.

- The story, characters, factions and the ten levels: [story.md](story.md).
- Willy Maker, the visual tool planned in Tools to build games like this one, play-test them and export them: [docs/willy-maker](../willy-maker/README.md).
- How to deliver sprites, backgrounds and levels so they go into the ROM untouched (sizes, colors, grid, the map format, Mission 1's canvas): [art-spec.md](art-spec.md).
- The characters come from the go-link Console sprite sheets used by the website's *Destroy this page* easter egg ([web.md](../web.md#destroy-this-page)): Willy, Vera Buffer, Glitch-9, Jitter and the four civilians. The atlas script (`frontend/apps/web/scripts/destroy-atlas.mjs`) already cuts them into frames.
- Status: design, plus a feasibility prototype in [`rom/`](../../rom/README.md): a mini level built as a `slammast`-layout set (4 players, 3 buttons) runs on the stock core and in a real go-link Console room.
- Prototype docs: the step-by-step [lab journal](journal.md) (how to reproduce it from zero, dead ends included) and the [CPS-1 hardware facts](hardware.md) it relies on, each with its source in the driver.

## Goals

1. An original game: code, graphics, music, sound effects and text all made for go-link Console, so the ROM can be distributed with go-link Console and played in rooms without any third-party ROM.
2. It runs on the stock mame2003-plus core the device already downloads (`device core download`), with no core changes.
3. Four players at once, with Coin and Start like any arcade board, matching go-link Console's four seats, the arcade queue and the controls the device maps (RetroPad B/A/Y/X/L/R = buttons 1 to 6, Select = Coin).
4. The same characters and tone as *Destroy this page*: one universe across the website and the arcade.

## The hard constraint: real arcade hardware

MAME does not load PNGs or scripts; it emulates real arcade boards. A game for mame2003-plus is machine code and graphics data for one of the boards it emulates, laid out in ROM chips exactly as that board expects. So the game is written like a 1990s arcade game, for real (emulated) hardware.

### Choosing the board

| Board (MAME 0.78 driver) | For us | Against |
|---|---|---|
| **Capcom CPS-1** | No BIOS. 384×224, the resolution the go-link Console test card already uses. 68000 CPU, Z80 sound with YM2151 FM and OKI ADPCM samples. Hundreds of 16×16 sprites with 4-bit color, many palettes. The classic hardware of side-scrolling beat-'em-ups and run-and-guns | Each 16×16 tile has 15 colors plus transparency, so the sprite sheets need palette reduction |
| SNK Neo Geo (MVS) | Built for big sprites and 4 players' worth of action; open-source homebrew toolchains exist | Every Neo Geo game needs the system BIOS (`neogeo.zip`), which is copyrighted: go-link Console could not ship a complete game |
| Sega System 16/18 | Good 2D hardware, no BIOS | Fewer free tools and documents than CPS-1 |
| Capcom CPS-2 | More sprites | Encrypted program ROMs: every set needs a key |

**Recommendation: CPS-1.** No BIOS means the whole game can be ours, its resolution matches go-link Console's, and it is the most documented 2D arcade board for homebrew.

### How the core finds the game

mame2003-plus only runs the games in its own driver list (identified by the set's short name and the files and CRCs it expects). Two ways forward, to decide before writing code:

- **A. A replacement set:** build our game to the exact ROM layout (file names and sizes) of an existing CPS-1 set in the driver list, so the stock core runs it unchanged. MAME 0.78 still loads a ROM whose name matches with a different CRC (it warns). go-link Console's device checks every set against the core's list (`pkg/romcheck`); the prototype showed it accepts such a set as it is, because it matches files by name like the core does ([journal, step 3](journal.md#step-3--a-go-link-room)), but it then shows the original game's title and art, so the device still needs an allow-list of go-link Console's own sets, identified by our own hashes, to show our title and to tell our set from the original one. The game would show under the original set's name in the core; go-link Console's library can show our title and art instead.
- **B. A new driver entry:** add `willy` to a fork of mame2003-plus. Cleanest for players, but it means shipping and maintaining our own core build, and checking the core's license terms for redistribution first.

Option A keeps goal 2 (stock core). Its one change in go-link Console is built (2026-10-01): the device ships a list of go-link Console's own sets by the SHA-256 of every file inside the zip (`backend-device/pkg/ownsets`), and the library, the New game page and the rooms show our title, description, picture and controls (Jump, Fire, Special) with a "go-link" mark when every file matches. A set that only matches by name stays the original game. See [device.md, go-link Console's own sets](../device.md#go-links-own-sets).

### Releasing the set

1. Build it: `node rom/tools/build.mjs`. The build ends by running `rom/tools/ownsets.mjs`, which writes the SHA-256 and size of each file inside `rom/build/slammast.zip` to `backend-device/pkg/ownsets/sets.json` (and copies the set's picture to `backend-device/pkg/ownsets/art/`). Run `node rom/tools/ownsets.mjs` alone to refresh the list from an existing build. Builds are reproducible file by file, so an unchanged program prints "unchanged"; the zip's own hash changes on every build, which is why the list never uses it.
2. Check it with the exact core: `device romtest rom/build/slammast.zip` must print PASSED and `identity: go-link Console set`.
3. Commit `backend-device/pkg/ownsets/` together with the source change, and ship **that** build's zip: a zip from another build with different files would show as the original game.
4. Release the device as usual: the list is embedded in the binary, so devices of older releases keep showing the set as the original game.

## Technical plan

### Toolchain

- **Code:** 68000 assembly or C cross-compiled for the 68000 (GCC for m68k, or an assembler such as vasm). Sound driver in Z80 assembly.
- **Graphics converter:** a script, in the same spirit as `destroy-atlas.mjs`, that takes the go-link Console sprite sheets and outputs CPS-1 graphics: 16×16 tiles, a palette per character (15 colors plus transparency), deduplicated tiles, and the sprite and animation tables as data. The palette step matters most: the sheets are painted with many shades and must be quantized per character without losing the look (outline, skin, shirt and logo ramps).
- **Music and sound:** YM2151 FM tracks (a sequencer or tracker export converted to the sound driver's format) and OKI ADPCM samples for voices and effects ("Nobody waits!", explosions, the rescue jingle). The chiptune themes of the website's mini-games and *Destroy* are a starting point.
- **Testing:** MAME and the mame2003-plus core itself, plus a go-link Console room to check four seats, Coin, Start and the voice and video path.
- **Build:** one command that assembles, converts the graphics, splits the output into the CPS-1 ROM files and zips the set, with the hashes the device's allow-list needs.

### Game engine (on the 68000)

- Fixed 60 Hz loop; scrolling layers for the city backgrounds; sprites for characters, enemies, bullets, debris and civilians.
- Destructible scenery as tile changes plus debris sprites (the *Destroy* homage).
- The move set of the sprite sheets: run, jump, double jump, short jetpack, crouch, crawl, flying kick, machine gun in 8 directions, knife, bazooka, rescue. The aimed poses for the diagonals exist in the atlas (`aim_up45`, `aim_down45`, `aim_up`, `aim_down`).
- Up to 4 players: three buttons (jump, fire with an automatic knife up close, special: the picked-up weapon) plus Start and Coin each; inputs map 1:1 to go-link Console's seats. See [story.md, Controls and weapons](story.md#controls-and-weapons).
- DIP switches: difficulty, lives, free play, demo sound.
- Attract mode with the title, the story intro, a demo of level 1 and the high score table, saved in the core's NVRAM.

### Assets to produce

- Character sprites from the existing sheets: Willy (all 13 animations plus the 4 aimed poses), Vera, Glitch-9, Jitter (each with both their brainwashed and freed palettes), the four civilians.
- New: Lag troopers and their variants, Spinners, Pingers, delay mines, vehicles and the bosses (Commander Rook, Kasimir Spool, the Choke, Director Kessler, Dr. Laggard, Desmond Hollowell, the Delay Engine).
- Ten level backgrounds (see [story.md](story.md)); the neon city sheet is the style reference.
- Title screen, story cutscenes as still panels with text, the ending and the true ending, and a font.

## Milestones

1. **Board prototype:** a CPS-1 build that shows a background, Willy walking and jumping with sound, running in mame2003-plus and in a go-link Console room.
2. **Graphics pipeline:** the converter turns the go-link Console sheets into CPS-1 data with acceptable palettes.
3. **Level 1 complete:** "Dead Air", with enemies, civilians, the gunship boss, four players, score and continue.
4. **The team:** Vera, Glitch-9 and Jitter playable, the brainwashing fights and freeing them.
5. **Levels 2 to 10**, the endings and the attract mode.
6. **go-link Console integration:** the device's allow-list for go-link Console's own sets and the game in the ROM library with its title and art (built 2026-10-01), and the set shipped with go-link Console releases.

## Rules

- 100 % original content: no code, graphics, music or text from any existing game. The *Alias*-inspired structure stays a structure; every name is ours.
- go-link Console still never downloads, hosts or names third-party ROMs or ROM websites. This set is the one exception because it is go-link Console's own work, and it is shipped like any other go-link Console file.
- Source in English with the go-link Console copyright header, like the rest of the repository.

## Prompt for an AI collaborator

Paste this, together with [story.md](story.md), to brief an AI (or a person) on any part of the game:

```text
You are helping build "Willy Gorklingo: The Lag Protocol", an original side-scrolling
run-and-gun arcade game for up to 4 players, made by go-link. It must run as a ROM set on
the Capcom CPS-1 arcade board as emulated by MAME 0.78 (the mame2003-plus libretro core):
68000 main CPU, Z80 sound CPU with a YM2151 and an OKI MSM6295, 384x224 at about 60 Hz,
16x16 sprite tiles with 15 colors plus transparency per palette, scrolling tile layers.
No BIOS, no third-party code or data: every byte is original.

Story and levels: see the attached game bible (story.md). The hero is Major Wilson
"Willy" Gorklingo of Task Force Uplink. His teammates Vera "Buffer" Valen, the android
Glitch-9 and the alien engineer Jitter were brainwashed by the Lag with Throttle Crowns;
he frees them, never kills them. The Lag is a three-layer organization: Section Null (a
fake allied cell), the Round Trip council of seven Hops led by Desmond Hollowell, and the
Order of the Long Wait, which hunts Aurelio Lattenza's Delay Engine. Ten levels:
Buenos Aires, Cairo, Hong Kong, the Swiss Alps, Tokyo, the North Atlantic, Nevada,
Venice, Antarctica and the airship over Buenos Aires.

Player moves: run, jump, double jump, short jetpack, crouch, crawl, flying kick, machine
gun in 8 directions, knife, bazooka, rescue civilians. Inputs per player: stick, 3
buttons, Start and Coin. Scoring, lives as hearts, continues, DIP switch difficulty,
attract mode, high scores in NVRAM.

Tone: fast, readable, humorous dialogue, serious stakes; civilians are always rescued.
Pixel art in the style of the provided go-link Console sprite sheets; palette of night blues,
accent orange (#f2a33a), warning red and rescue green.

Task: <describe the part you need: a level design document, a boss pattern, the 68000
game loop, the graphics converter, a YM2151 music track, the cutscene script...>
Deliver it ready to use, explain the hardware limits you respected, and list anything
that needs a decision.
```
