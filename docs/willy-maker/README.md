# Willy Maker

**Willy Maker** is go-link's arcade game maker, on its own site, [maker.go-link.org](https://maker.go-link.org) (before 2026-10-05 it was **Tools › Willy Maker** on go-link.org; those addresses redirect there) for making arcade games for go-link visually: draw a level, drop in characters, test-play it in the browser at once, and turn it into a ROM that runs in MAME and in a go-link room.

It takes the idea of a game maker, where you build and play at the same time with instant tries, and applies it to real arcade hardware. The design, names, art, icons and sounds are all go-link's own. Nothing is taken from any other game maker.

This page is the source of truth before any code is written. More detail lives in:

- [architecture.md](architecture.md): the module, its layers and what it shares with `rom/`.
- [file-format.md](file-format.md): the project file, the `.zip` layout, the AI pack and its prompt.
- [validation.md](validation.md): the four validation levels, from live rules to a power-on test on the device.
- [vision.md](vision.md): where Willy Maker is going: every genre, a ladder of boards from the CPS-1 to go-link's own engine, and the principles that keep the code ready for it.
- [editor.md](editor.md): the editor (Modernist look, zones over a picture, layers and groups, My games), the default since step 8 (the classic one behind `?editor=classic`), and its steps.
- [code.md](code.md): the second evolution, games with their own code (blocks and scripts run by go-link HD), planned.
- [genres.md](genres.md): the game genres as a roadmap (today the platform shooter, and the platformer on the same engine), with the mechanics, parts, board limits, controls and size of each.

Willy Maker builds on the ROM project:

- [docs/rom/README.md](../rom/README.md): why CPS-1 and how the core runs our sets.
- [art-spec.md](../rom/art-spec.md): sizes, colors, grid, collision tags and objects.
- [hardware.md](../rom/hardware.md): the board's real limits.
- [journal.md](../rom/journal.md): the prototype in `rom/`, which already proved every rule below in the core and in a go-link room.

## Who it is for

- People who want to make their own arcade game and play it with friends in a go-link room. They need no assembly, no 68000 and no knowledge of how a board stores graphics.
- Artists and level designers who deliver to the ROM project: Willy Maker checks their work against [art-spec.md](../rom/art-spec.md) while they work, not after.
- go-link itself: the missions of *Willy Gorklingo: The Lag Protocol* ([story.md](../rom/story.md)) are built with it.

## Platforms

| Board | Screen | Layout it uses | Players × buttons | Status |
|---|---|---|---|---|
| **Capcom CPS-1** | 384 × 224, 60 Hz | `slammast` | 4 × 3 | Phase 1 |
| Capcom CPS-1 | 384 × 224 | `captcomm` | 4 × 2 (special = both buttons) | Phase 1, as an option |
| Other boards | — | — | — | Phase 3 |

A project picks its board when it is created. Every limit, meter, check and converter comes from that board's **profile** (see [architecture.md](architecture.md#board-profiles)). Adding a board means adding a profile, not changing the editor.

## Phases

### Phase 1: build, play and export

1. Build a game in the browser: characters, backgrounds, levels, game settings.
2. Play-test it in the browser, with the same rules as the ROM engine, while building.
3. Export:
   - **Project `.zip`**: everything, to continue in another browser or keep a backup.
   - **AI pack `.zip`**: the levels as Tiled maps, the graphics as PNGs, and a generated prompt that includes our ROM docs. An AI (or a person) uses it to produce the ROM with the tools in `rom/`.

### Phase 2: "Create ROM" in the browser

**Built (2026-10-01), in [experiment 1, case C](../experiments/README.md):** the engine, the packer and the Create ROM card, described in [engine.md](engine.md). The level of Game Spec v1 made with it in a recorded browser session passes validation levels 3 and 4, the scripted clear and the same-picture test on the real core. What it leaves out for now is listed in [engine.md](engine.md#what-the-rom-leaves-out-for-now). [Experiment 1's verdict](../experiments/verdict.md) chose this way over a ROM written by hand or by an AI from the AI pack, and turned what the hand-made ROM did better into Willy Maker's next tasks.

The prototype's 68000 program is turned into a **data-driven engine**: levels, characters, menus and settings become data that the engine reads. Building a game is then **packing**, not compiling:

- Convert the graphics to the board's format, with the shared `@go-link/cps1` package (`frontend/packages/cps1`: `gfx.ts`, `sprites.ts`, `color.ts`), the same code `rom/tools` builds the prototype with.
- Pack the maps and data next to the prebuilt engine.
- Encrypt the sound program the way the board expects (`kabuki.ts` in the same package).
- Lay everything out as the set's files and zip it.

All of it can run in the browser (WebAssembly where it helps, for example a fast zip or quantizer). A **Create and play** button then sends the `.zip` to the user's go-link device over the existing `files` channel (as ROM uploads already do) and opens a room with it.

### Phase 3: more boards

Profiles for other boards the mame2003-plus core runs, chosen for their limits and how well they are documented. The editor, the play mode and the export stay the same.

## Features

### Projects

- A list of projects, each with its board, title, thumbnail and last change.
- **New project** from a template:
  - empty;
  - "Mission 1: Buenos Aires": the 8192 × 672 canvas of [art-spec.md](../rom/art-spec.md#5-level-size-and-mission-1-buenos-aires), with its five sections;
  - "Prototype street": the Step 4 level of the prototype.
- **Autosave** as you work:
  - the project's JSON goes to `localStorage`, one record per project, like the skin editor's **My skins**;
  - pictures (sprite sheets, background layers, PNG tiles) go to **IndexedDB**, because they outgrow `localStorage` (about 5 MB per site). The JSON refers to them by hash.
  - If the browser refuses to save, a visible warning says so, and leaving the page asks first.
- **Export** the project as a `.zip`, and **import** a `.zip` to continue in another browser or machine.
- Nothing is sent anywhere in Phase 1.

### The board profile and budget meters

Live meters in the top bar, from the board profile, always visible while building:

| Meter | CPS-1 limit |
|---|---|
| Sprite palettes | 32 (shared by every character) |
| Play layer palettes / far layer palettes | 32 / 32 |
| Colors per tile or per sprite zone | 15 + transparent |
| Colors | CPS-1 12-bit: every channel a multiple of 17 |
| Unique tiles and sprite cells vs graphics ROM | 6 MB |
| Sprites on screen | 256 entries |
| Program and data size | 2 MB (68000 program ROM: the engine in the first megabyte, the game's data in the second) |
| Sound samples | 4 MB (QSound: go-link's engine uses about 0.4 MB of synthesized effects and instruments) |

A meter turns amber near its limit and red over it. The meters count what go-link's engine already takes (its sprites, the font, 25 of the 32 sprite palettes and its program, from `engine.json`). **Board usage** (since experiment 1's verdict, T-27): the top bar's **CPS-1 · %** chip shows the most used limit; it opens a panel with every meter as a bar, the live estimate while building, and **Measure exactly**, which packs the game as Create ROM does and shows the real graphics, program and sprite palette numbers (`rom/usage.ts`, `ui/organisms/BoardUsage.tsx`).

### Characters

1. **Drop a sprite sheet** (PNG, WebP, GIF or JPEG), or pick it with the button.
2. **Automatic frame detection**, like `scripts/destroy-atlas.mjs` and the prototype's converter:
   - the background is keyed from the border of the sheet with a tolerance (18 by default, per channel): only background connected to the border becomes transparent, so a black shirt inside the outline survives;
   - a magenta `#FF00FF` background (the art spec's) is keyed everywhere, enclosed gaps included;
   - separator lines and underlines (long strokes at most 7 px thick) are removed, so figures standing on a line are not glued together;
   - each figure is found on its own (near shapes such as a muzzle flash join it; label text and small effects are left out), or the sheet is cut by a **fixed grid** (48 × 48 by default) where empty cells are skipped;
   - boxes several figures wide are split where poses only touch (a nearly empty column);
   - frames are numbered in reading order, and the pivot is placed on the middle of the feet, on the box's last row.
3. **Edit the boxes**: click to select (Shift adds), drag to move, drag the corner to resize, arrows nudge by 1 px (Shift: 8), Delete removes, **Add a box** draws one by hand, and the selected frame's box and pivot can be typed in. Zoom 1×, 2× and 4×.
4. **Assign frames to animations**: the list of [art-spec.md](../rom/art-spec.md#frame-list) for the character's role (`idle`, `walk`, `run`, `jump`, `climb`… for heroes; enemies, civilians and bosses have their own), each with how many frames it has against the art spec's count. Select boxes, pick an animation and **Add the selected frames**; set its frame rate and loop; other animations can be added by name. Boxes show the animation they belong to. (The special points, `muzzle` and `hand`, come later.)
5. **Size**: the character's height on screen, with the art-spec sizes as presets (44 px heroes, 48 px android, 42 px adults, 32 px child, 20 px baby…). One scale fits the median `idle` frame (or all frames) to that height, by the dominant color of each footprint (never blended). Drawn 1:1 art is the best; a shrunk sheet says from what height, an enlarged one warns about blocky pixels.
6. **Palette zones**:
   - the character is split into zones of 16 px rows counted from the feet (the board's sprite tiles, as the ROM cuts them): head, torso and legs for a 44 px hero, plus a row above the head when a pose is taller; each zone with at most 15 colors;
   - colors are snapped to the board's 4096 (each channel a multiple of 17) perceptually, in OKLab, with the mean error shown as delta E (below 1 is invisible);
   - a zone with more than 15 colors turns red and is reduced to the 15 nearest (k-means, the outline color kept); the two most alike colors are named as a merge hint, and painted (not pixel) art gets one note instead;
   - a click on a color marks it as the shirt, for the palette swaps of players 2 to 4.
7. **Preview at 1x and 4x** with the animation playing, exactly as the board shows it (zone palettes applied), and the height as a share of the screen.
8. **Save character**: the frames are packed 1:1 into an atlas picture (transparent background, exact board colors), which becomes the character's `sheet`; its zones become sprite palettes. The dropped sheet and the boxes are kept too, so the character opens again for editing (see [file-format.md](file-format.md)).

### Backgrounds and layers

Layers like an image editor's, each with a name, visibility, lock, opacity (editor only) and order:

| Layer | Grid | What it is |
|---|---|---|
| `far` | 32 px | sky and skyline (the board's slow scroll layer) |
| `mid` | 32 px | a second far picture, merged into `far` at export (the board has one far layer) |
| `play` | 16 px | what players walk on and in front of |
| `collision` | 16 px | invisible tags (below) |
| `objects` | free (snaps to 8 px) | everything that is not a tile |
| `text` | 8 px | HUD and menus (game settings) |

Background art is painted with tiles from a tileset (the prototype's city, crates and ladders, and its night sky), or comes from a Tiled map in the new game wizard. **Own backgrounds (T-28 of [experiment 1's verdict](../experiments/verdict.md)):** a game can also start **from a picture of your own** (the new game wizard's fourth start) or put one into the far or play layer later (**Picture…** in Layers). Any picture works: one screen, several side by side, a small one repeated, or one long strip that widens the level. `editor/picture.ts` scales it to board pixels by taking each block's dominant board color (pixel art drawn at 2x-12x comes back pixel for pixel), keeps at most 15 colors per tile, spreads the layer over up to 32 palettes of 15 chosen tile by tile, cuts and deduplicates the tiles, and `editor/pictureImport.ts` saves them as the layer's own tileset (`ts-pic-<level>-<layer>`, with `tilePalettes`) in one undo step, composed over what the layer already had when asked. Then the collision tags are traced over it with the pencil; Auto art only paints into a play layer that uses the starter city tiles, never over a picture. Play mode draws the level's far and play tiles as the board does (the far layer at half speed), and Create ROM loads every palette (wm_data version 3, [engine.md](engine.md)). A dense 1600 × 900 pixel-art city came back at 398 × 224 with 350 tiles, 32 palettes and 198 colors in about 0.3 s, and its ROM matched the real core at tolerance 0. Tags suggested from the picture are a later step.

**Prompt for image AI (T-29):** the pictures can come from an image AI. **Prompt for image AI** (in Layers, Parts › Objects and Tiles, Characters and Export) opens a dialog that asks what to create (a far, play, panorama, one-screen or boss arena background; a hero, enemy, civilian or boss; an object; an effect; a tile set), a description, checkboxes with a "?" that explains each one, place and period, time and weather, a style reference (used only as a style: the prompt forbids existing characters, logos, names and text), palette and quality. `prompts/imagePrompt.ts` adds what the board needs: every board pixel as an exact 4 × 4 block (a screen is 1536 × 896, a 4-screen stretch 6144 × 896, the far layer half the level plus a screen), 12-bit color, at most 15 colors per 16 × 16 tile, no gradients, anti-aliasing or semi-transparency, a #FF00FF magenta background where the far layer or nothing shows, one prompt per parallax layer and per play stretch naming the level's sections it shows, character sheets with a row per chosen animation and its preset frame count (hovering or focusing an animation shows Willy playing it, or the closest move he has, with a line on what it is), feet aligned and facing right, and a negative prompt, plus **All in one**: a single message for a chat image AI (which has no negative prompt field) with each picture's exact size, its prompt and "Avoid: …". Each kind keeps to its own fields: place, time and weather belong to backgrounds (and place to tiles); a character, an object or an effect gets only its description, its colors and the style, so its prompt says nothing about what is behind it. A character's animations can be chosen all, none or one by one, and the user's own ones added with a frame count (`customAnims`); with none chosen, the prompt asks for the character reference (three standing poses) that every animation is then drawn from. The prompts ask for what image AIs draw well and never for the board's limits (pixel counts, 15 colors per tile, 12-bit color): an image AI cannot count them and only draws smaller and plainer when asked, while Willy Maker scales the picture, removes its background and fits its colors when it is brought in. Each move is described in words, a shape is asked instead of a size, a character sheet is split into pictures of at most 6 animations or 32 frames (one chat message each) and a background into one screen per picture. Up to 3 **reference pictures** go with the first message: **Copy** puts each on the clipboard and then the text (tested: a chat AI attaches a pasted picture but drops it when text comes with it). Brought in, a picture that is not pixel art on an exact grid (an image AI's) is scaled by averaging each block (backgrounds) or by the block's own color closest to its average (sprites), as the most present color turns such art's fine detail into noise; Characters › **Assign by rows** gives a pasted sheet's rows the animations its prompt asked for, and **Add another picture** (or a paste with a sheet open) puts the next message's picture under the sheet, keeping the animations; Assign by rows then fills only the empty ones with the new frames (`sprites/append.ts`). Every prompt asks for art designed for its real size on the board (a screen of 384 × 224 game pixels, a character about 44 game pixels tall) with big readable shapes and no detail smaller than 2 game pixels: tested, it is what makes a picture survive the board. The prompts are in English (image AIs follow it best); the dialog is in English, Spanish and Portuguese. **Picture…** has a "Magenta is transparent" option (on for the play layer) so such a picture drops straight in. The choices are kept in `settings.imagePrompts` ([file-format.md](file-format.md)).

**Chrome's built-in AI in the dialog** (`ai/chromeAi.ts`, all on the user's computer: nothing typed or said leaves it). Every free text field is detected (LanguageDetector) and translated into English (Translator, Chrome 138+) before it goes into a prompt; while Chrome's translator is not downloaded, its language model (the Prompt API, `LanguageModel`) translates instead, and typing in the dialog counts as the gesture Chrome asks for to download the translator once (a button does it otherwise). The description has **Dictate** (the Web Speech API with `processLocally`: on-device recognition only, never the cloud one), **Check spelling** (the Proofreader API when Chrome has it, else the Prompt API) and **Improve with AI** (the Prompt API writes a richer English description); a suggestion is only applied with **Use it**. Each button shows only when this browser can do it: in another browser, or a Chrome too old, the dialog says what Chrome would add and the prompts carry the text as typed, with a warning. APIs still in an origin trial (the Proofreader) turn on with tokens in `VITE_ORIGIN_TRIAL_TOKENS` at build time ([apps/web/.env.example](../../frontend/apps/web/.env.example)). Tested in Chrome 154: a misspelled word can come out of the translator wrong ("bisicleta" lost the bicycle), so the dialog suggests checking the spelling first.

### The pencil: collision tags

A pencil, rectangle, fill and eraser that paint **tags** on the `collision` layer:

| Tag | Meaning |
|---|---|
| `solid` | blocks and can be stood on |
| `oneway` | stood on; jumped through from below; dropped through with down + jump |
| `ladder` | climbed with up and down |
| `crate` | climbed by pushing (32 px), breakable (`hp`) |
| `breakable` | solid until destroyed (`hp`, the tile it turns into) |
| `hazard` | hurts (`damage`) |
| `water` | slows down, no jetpack |

Each tag has its own color and pattern, so it reads without relying on color alone. A toggle shows the tags over the art, under it, or hides them.

### Objects

Stamps placed on the `objects` layer, each with a **reference name** (for example `crate_dock_3`) and its properties:

| Object | Properties |
|---|---|
| Player start | player 1–4 |
| Enemy | kind, facing, patrol distance |
| Civilian | kind (woman, child, baby, elder), trapped in (a crate or wall) |
| Crate | size (16 or 32), hp, contents (nothing, weapon, health, civilian) |
| Pickup | bazooka, flamethrower, spread, grenades, health, Lattenza page, coin, spring |
| Moving platform | width, across or up and down, distance, speed |
| Camera lock | a rectangle where the camera stops until its enemies are beaten |
| Checkpoint | where players come back after losing a life |
| Boss | kind, arena rectangle |
| Exit | the end of the level |

The names are references: the AI pack, the warnings and the play mode's debug overlay use them.

### Helpers

- Grids (8, 16 and 32 px), rulers, guides and snapping, like the skin editor.
- A **minimap** of the whole canvas, with the visible screen (384 × 224) and the sections.
- **Reachability check** with the engine's rules (a jump reaches ledges up to 48 px, push-climb 32 px, ladders, drop through one-way ledges): it shades what a player cannot reach and points at the gap.
- **Warnings** while working:
  - no exit;
  - a civilian behind an unbreakable wall;
  - a camera lock with no enemies;
  - a tile over its colors;
  - a budget over its limit.
- Undo and redo for every change; copy and paste of regions and objects across levels.

### Game settings: the game's "IDE"

Two tabs of the IDE (`src/maker/game/`, texts in `i18n/game.*.ts` and `i18n/menus.*.ts`). Every change is an undoable command on the editor store (`store.editSettings`; the letters typed in one field are one undo step) and is autosaved.

**Game tab**

- **Players**: 1 up to the board layout's count (4 on both CPS-1 layouts).
- **Actions**: the board inputs are fixed by the layout; each action's label is editable (empty = its default name):

  | Input | Action |
  |---|---|
  | B1 | Jump (hold: nothing for now); down + jump drops through a one-way platform |
  | B2 | Fire: the machine gun, the knife when an enemy is right in front (automatic) |
  | B3 (`slammast`) / B1+B2 (`captcomm`) | Special: the picked-up weapon, with ammo |
  | → → | Run: a double tap within the run window (250 ms by default, 100-400 ms) |
  | ↑ ↓ | Ladders and aim |
  | START / COIN | Join or pause / a credit |

- **Each player's character**: a project hero or the built-in Willy, plus a shirt for Willy (own colors or one of three recruit shirts; an own hero keeps its colors). New games: Willy for player 1, recruits for players 2-4. Play mode and the ROM draw an own hero with its own animations (idle, walk or run, jump, fire), at the size it was saved. How many play is also a selector in the top bar.
- **Your controller**: the connected controller (Gamepad API) drawn with its own button names (`src/controllers`) and lit while pressed; press-to-assign remapping of the keyboard and of each controller model for Up, Down, Left, Right, B1-B3, Start and Coin; the on-screen pad in play mode (automatic on touch screens, always or never); and "go-link defaults". The mapping is the site's button map in this browser (`go-link.input`), so play mode and go-link rooms use it; the touch choice is `go-link.wm.touchpad`.
- **DIP switches**: difficulty (Easy, Normal, Hard, Lag), lives (1-5), free play and demo sound, saved with the game. Play mode uses the lives and the difficulty; in the ROM the lives are the hits a player takes, and free play needs no credit. The difficulty (task T-15) sets how often an enemy fires and how fast its shot flies: Easy every 150 frames at 2 px per frame, Normal 90 at 3 (the prototype's), Hard 60 at 4, Lag 40 at 5; the ROM reads it from the header's flags (bits 5-6, `WM_F_DIFFICULTY`).
- **Rules**: hits an enemy takes, points per enemy, rescue and crate, touch damage, enemies that chase or shoot, an exit that needs every enemy down (with `ENEMY n` in the HUD and a message when the exit is reached early), what a hit does (back near the camera, or blink in place, with the blink's length), how a 32 px crate is climbed (by jumping, the default, or by walking into it) and what Start does on a port past the game's players ("coming soon" or nothing). Play mode and the ROM read the same values ([engine.md](engine.md#rules-the-game-tab)); **Default rules** clears them.
- **Checks**: the live rules of the game settings and menus (below), each with Go.

**Menus tab**

- Seven screens: **Title**, **Demo** (attract, with its demo level), **Player select**, **HUD**, **Continue**, **Game over** and **High scores**.
- Each one has a live 384 × 224 preview drawn with go-link's 8 × 8 board font (`@go-link/cps1` `font.ts`, the same glyphs the ROM prototype uses), its text fields (lowercase is shown as uppercase, as the board draws it), a background (a level's first screen darkened, or a solid board color), a music slot (a reference only until sound arrives in Phase 2) and the credits line (`(C) 2026 go-link` by default, shown per screen).
- **Fit checks**, live under each field and on the preview: the text must fit the 48 × 28 character text layer (a HUD join prompt fits its player's slot), should stay inside the safe area (2 characters in from each edge), and may use only the font's glyphs.
- Play mode uses the HUD texts (join prompt, ammo, level clear) and draws the Game over screen in the board font.

**Live rules** (`editor/validate/game.ts`, shown in the Game tab, the Build warnings and the Export review): players within the board's count (error), a character for every active player, a title on the title screen, the continue and game over screens not empty, lines that do not fit the screen or use letters the font lacks (warnings), lines outside the safe area (info).

**Levels**: their order and their music and sound references (sound is Phase 2 and later).

### Play mode: building and playing at once

**The moves** ([moves.md](moves.md), T-25): crouch and crawl on Down, firing low, a jump kick (Down + B2 in the air), land, turn, a thumbs up on a rescue, victory on the clear, a yawn after 5 s idle, and the Rules card's double jump and jet pack, the same in play mode and in the ROM; the reach check climbs as far as the rules allow.

- **Play** runs the current level in the browser at the board's resolution (384 × 224), shown at 2×. It uses the same physics, camera, collision and object rules as the ROM engine, written once in the module's `engine/` (see [architecture.md](architecture.md)).
- **Input**: controllers (Gamepad API, the site's button map), keyboard, and a touch pad on phones. Up to 4 local players.
- **Debug overlays**: collision tags, hitboxes, the camera window and its locks, object names, frame time.
- **Edit while playing**:
  - **Pause** freezes the game; you edit tiles, tags or objects;
  - **Resume** continues from the same moment with the change in place;
  - **Restart here** puts the players at the cursor.
- What the browser cannot promise is the board's exact timing and sprite-per-line limits; the meters and the core (Phase 2) settle those.

### Validation and export

Before any export, a checklist runs and links each problem to where it is:

- budgets;
- colors;
- grid alignment;
- missing animations;
- unreachable areas;
- objects without required properties.

Errors block the AI pack; warnings do not. Each entry has **Go** (to the level, object or tab) and, when the change is safe, **Fix** (undoable). The rules and what is built are in [validation.md](validation.md#level-1-live-rules); the pack's layout is in [file-format.md](file-format.md#the-ai-pack).

Export also has a **Power-on test**: drop a ROM `.zip` (for example the one an AI built from the pack) and a 68000 with a model of the CPS-1 board starts it in the browser, presses Coin and Start, and lists each step with a picture of what the board drew. Nothing is uploaded. See [validation.md](validation.md#level-3-power-on-in-the-browser).

## UI overview

The screens match the mocks being designed on the go-link design canvas:

| Screen | What is on it |
|---|---|
| **Home / New project** | the project list; New project with board (CPS-1 card with its limits, future boards greyed out), layout (`slammast` 4 × 3, `captcomm` 4 × 2) and template |
| **IDE** | top bar (project, board badge, budget meters, Play, Export); left: the project tree (characters, enemies, civilians, backgrounds, levels, menus, settings); center: the level canvas with grids and the minimap; right: the inspector of the selection; bottom: the layers panel; the tool bar (select, pencil, rectangle, fill, eraser, stamp, hand, zoom) |
| **Sprite import** | the dropped sheet with detected frames, animation slots, pivot editor, palette zones with their 15-color meters, CPS-1 snap preview, 1x and 2x preview |
| **Play test** | the 2× game view with HUD, debug overlay switches, the controller panel with live buttons, Pause / Edit / Resume / Restart here |
| **Game settings** | players, buttons and actions, controller mapping, menus, DIP switches, level order |
| **Export / Build** | validation checklist, Export project, Export AI pack (with a preview of the prompt), Create ROM (its steps, the power-on test at once with its picture, Download ROM, the symbol map, Play on my go-link, notes on what the ROM leaves out), the power-on test of a ROM `.zip` |

### Keyboard shortcuts

| Key | Action |
|---|---|
| V / H / Space (held) | select / hand / hand while held |
| B / R / G / E | pencil / rectangle / fill / eraser |
| 1–7 | the collision tags, in table order |
| O | object stamp |
| P | play / back to editing |
| Esc | pause while playing |
| ⌘Z / ⇧⌘Z | undo / redo |
| ⌘C / ⌘V / ⌘D / ⌫ | copy / paste / duplicate / delete |
| ⌘S | export the project `.zip` |
| ⌘+ / ⌘− / ⌘0 | zoom in / out / fit |
| [ / ] | previous / next layer |

### Accessibility and languages

- Real buttons and labels, keyboard reachable tools and panels, visible focus, tooltips on icon buttons, and 44 px targets on touch screens.
- Collision tags use pattern and color.
- Meters announce their state in text, not only in color.
- `prefers-reduced-motion` is respected.
- English, Spanish and Portuguese, in the site's i18n (a `willyMaker` section); the reference names users type are free text.
- Willy Maker needs a computer-size screen to build. On phones it opens projects in play mode only, with a note.

## Milestones

1. **Docs** (this folder) and the canvas mocks, approved.
2. **The module's skeleton**: route, Tools card, projects with autosave and `.zip` export and import.
3. **The level editor**: layers, tiles, the pencil and tags, objects, minimap, undo.
4. **Play mode** with the engine rules ported from `rom/src/main.c`, and edit while playing.
5. **Characters**: sheet import, frame detection, animations, palette zones.
6. **Game settings**: players, buttons, controller mapping, menus, DIP switches.
7. **Validation and the AI pack.** This completes Phase 1.
8. **The data-driven ROM engine and Create ROM** (Phase 2): built ([engine.md](engine.md)). Next: the project's own characters in the ROM, more levels, bosses and camera locks, and a room opened with the created set on the linked go-link.
9. **More boards** (Phase 3).

## Open questions

- **The engine's data format** for Phase 2: answered by `wm_data` ([engine.md](engine.md#the-data-block-wm_data)), a translation of the project format.
- **A room with a created set** (built 2026-10-04, device 0.1.9): **Play on my go-link** sends the game to the device's own Willy Maker folder (`~/go-link-console/maker/`, never the ROM folder) and opens its room (`@maker`, one per device: a new game replaces it), and the site opens that room ([protocol](../protocol.md)).
- **Sound** in the maker: QSound samples on `slammast` need a sound driver first (journal: pending). Phase 1 only stores references.
- **Our sets' identity**: the device's allow-list of go-link-owned sets by hash, so the library shows the user's title and art instead of the original set's ([docs/rom/README.md](../rom/README.md#how-the-core-finds-the-game)). It is needed before Create and play.
- **Sharing projects between users**: Phase 1 is files only. A gallery would need storage, which go-link does not have by choice.
- **Where the shared CPS-1 code lives** (see [architecture.md](architecture.md#shared-code-with-rom)).
