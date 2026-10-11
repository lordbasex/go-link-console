# Willy Maker: architecture

Willy Maker is **its own site, [maker.go-link.org](https://maker.go-link.org)**: its own app in the frontend workspace (`frontend/willy-maker`, `@go-link/willy-maker`), built and deployed apart from go-link.org (`make maker-build`, `make maker-deploy`). It was a module of the website until 2026-10-05, built self-contained so it could move out. Overview and features: [README.md](README.md).

## Where it lives

```
frontend/willy-maker/
  index.html, vite.config.ts   the site (its CSP, port 5181 in development)
  src/main.tsx, App.tsx        the shell: header (language, theme), routes / and /:gameId, the old games banner
  src/site.ts                  the main site's address (VITE_SITE_URL), the language and the theme
  src/bridge.ts                the way to the owner's go-link: the play.go-link.org bridge tab (below)
  src/importGames.ts           brings the games made while Willy Maker was on go-link.org
frontend/willy-maker/src/maker/
  index.ts          the one public entry: <WillyMakerApp lang projectId onProjectId device/> and its types
  model/            the project types (file-format.md), cell run-length encoding, factories, migration
  board/            board profiles: limits, meters, converters (cps1.ts first)
  templates/        "Buenos Aires" (Mission 1's five sections) and "Empty"; the starter tile numbers
  editor/           the store (commands, undo/redo), operations (strokes, stamps, moves), the parts
                    palette, auto art, reachability
  engine/           the game rules, pure TypeScript, no DOM
  play/             play mode (entry play/index.ts), edit while playing
  io/               storage (localStorage + IndexedDB), project .zip, Tiled import, starter pictures
  power/            the power-on test of a ROM .zip (validation level 3): its Worker and client
  rom/              Create ROM (stage 2, engine.md): pack.ts (the game as wm_data, graphics, the set's
                    files), createRom.ts (engine loader, tile pictures, zip), specFixture.ts (Game Spec v1)
  sprites/          the Characters screen (entry sprites/index.ts): frame detection,
                    animations, palette zones, preview
  game/             the Game and Menus tabs (entry game/index.ts): actions, players, your controller,
                    DIP switches, the menu screens with their previews and fit checks
  i18n/             the module's own texts: <part>.<lang>.ts (core, studio, sprites, play, game, menus, export, prompt)
  ui/               atomic design: atoms.tsx, molecules.tsx, organisms/, WillyMakerApp.tsx, render.ts
  ui/studio/        the editor, the default ([editor.md](editor.md)): Studio.tsx, its UI state (state.ts), header, panels,
                    options bar, canvas, footer, floating menus, My games (GamesHome.tsx), the wizard and demo;
                    ui/modernist.css holds its design tokens, workspaces.css dresses the shared screens in them
frontend/willy-maker/public/willy-maker/
  tiles/            city16.png and sky32.png (+ .json): the starter tilesets
  engine/           engine.bin and engine.json: the prebuilt ROM engine (rom/tools/engine.mjs)
frontend/willy-maker/scripts/willy-maker-tiles.mjs   builds them from the ROM prototype's art
frontend/willy-maker/vite.config.ts  also serves (dev) and copies (build) play mode's character atlases (player, robot,
                    npcs) from apps/web/public/destroy at /destroy, so the maker's site has them from that one copy
```

- The shell mounts the entry lazily, so the header shows while the editor loads. console.go-link.org keeps a card on `/tools` that links here, and its old `/tools/willy-maker[/:gameId]` addresses redirect here (with `?import=1` when that browser still has games made there).
- The module imports **only** `@go-link/shared`, `@go-link/ui` (the picture and the controller drawings and model recognition, shared with go-link Console's website), `@go-link/cps1` (the board conversion code, shared with `rom/tools`) and `@go-link/cps1-sim` (the board model of the power-on test). Nothing of go-link.org's website.

### The device bridge

The link to the owner's go-link Console lives on go-link Console's rooms site (play.go-link.org, `PLAY_URL`): its token is in that origin's storage, which another origin cannot read. So **Test on my go-link Console** (validation level 4) and **Play on my go-link Console** go through a tab of that site: **Connect my go-link Console** opens `play.go-link.org/maker-bridge` (`MakerBridgePage.tsx`), and the two tabs talk with `postMessage` (`packages/shared/src/maker-bridge.ts`):

- The maker sees the bridge as the device link it always used (`RomTestLink`: `sendControl`, `sendFile`, plus the device's answers), so `testRomOnDevice` and `playMakerGame` are unchanged.
- The bridge accepts messages only from Willy Maker's origin and from the tab that opened it, and posts only to that origin. It passes on only `rom_test`, `create_room` for the `@maker` game (private), zips of at most 16 MB with purpose `rom_test` or `maker`, and back only `upload_result`, `rom_test_result`, `room_created` and `room_error`. Nothing else of the device (unlink, settings, status, other rooms) is reachable from the maker.
- A game's room opens in the bridge tab, which becomes the room; the next **Connect** opens a new bridge tab.
- The games made while Willy Maker lived on go-link.org are in the landing's storage: **Bring my games** (`?import=1`) opens the landing's own `/maker-bridge` (`SITE_URL`, console.go-link.org) for that alone.
- `import` sends the games kept on go-link.org (`go-link.wm.*` in localStorage and the pictures in IndexedDB) once, for the **Bring my games** banner.
- **Its texts are its own**: each part keeps `i18n/<part>.<lang>.ts` (`core` for the shell, `sprites`, `play`, `game`, `menus`), English being the reference shape that Spanish and Portuguese must match (a test checks it); these es/pt files are the only non-English text in the module. The shell passes the site's language to the entry (`lang`, chosen in the header and kept as `go-link.lang`), `i18n/index.ts` provides it, and a part reads its texts with `useMessages({ en, es, pt })`; the shell's own texts are `core.site`. go-link.org's `en.ts`/`es.ts`/`pt.ts` only hold its Tools card and the bridge page. Its styles are its own CSS (`ui/willy-maker.css`), using the site's tokens; the root carries `.stage-tokens`, so the IDE stays dark in both themes.

## Layers

### `engine/`: the rules, shared by play mode and the ROM

Pure TypeScript with no DOM, tested on its own. It holds:

- the world: the collision grid and tags, objects, breakable state;
- the player physics, with the prototype's numbers (`rom/src/main.c`):
  - gravity 6/16 px per frame², jump −7 px per frame (it peaks at 61.9 px, so ledges up to 48 px);
  - push-climb up to 32 px;
  - ladders at 1.5 px per frame;
  - drop through `oneway` with down + jump;
  - the double-tap run;
- the controls: jump, fire with the automatic knife, special, run;
- enemies, civilians, pickups, camera locks, checkpoints and the camera rule (forward only, a small backtrack margin);
- a fixed 60 Hz step, so the browser and the ROM step the same way.

The ROM's data-driven engine (Phase 2, [engine.md](engine.md), `rom/engine/engine.c`) implements the same rules on the 68000, and both read the game's `GameRules` (`engine/rules.ts`). The engine's tests double as the specification both must pass. A shared table of test levels and expected positions keeps them in step.

### `editor/`: the project and its changes

- The project model lives in `model/`, matching [file-format.md](file-format.md): cell layers are run-length encoded text (`rle:0*120,1*4,…`), `migrateProject` checks and upgrades a parsed file (unknown fields are kept).
- `editor/store.ts` holds the open project. Every change is a **command** with apply and revert: `cellsCommand` (the cells of one or more layers, so a pencil stroke and its auto art are one step) and `editLevel`/`editProject` (a before/after snapshot of a level or the project). Undo/redo (200 steps), autosave and edits made in play mode all go through it.
- `editor/ops.ts`: pencil, eraser and fill strokes (`Stroke`: cells change at once, `end()` records the stroke), object stamps (`placePart`; a crate is 2 × 2 `crate` cells plus a `crate` object with `hp` and `contents`), moves and property changes, deleting, and `applyPlayEdit` for the pieces dropped while playing.
- `editor/autoArt.ts`: painting collision tags also paints the matching starter tiles on the play layer (street top or body, platform with or without a support, ladder, the right quarter of a crate…), replacing only tiles it could have painted itself, so hand-placed art stays.
- `editor/validate/`: validation level 1 (see [validation.md](validation.md)). `index.ts` (`reviewProject`, `applyFix`, `mergeReview`, `checkText`), `art.ts` (tile grids, pivots, heights, sprite wrap, program size), `pictures.ts` (the rules that read pixels, run a moment after each change in short steps by `useReview`, which says `checking` until they finish), `extra.ts`/`game.ts` (the Game and Menus tabs' rules).
- `editor/reach.ts`: a flood from the player starts over the places a 44 px hero can stand, with the prototype's moves (walk, fall, jump up to 64 px within a few cells, ladders, dropping through one-way ledges and breakable floors; crates and breakable walls can be shot away). Ledges it never reaches (with how far above the floor they are) and the exit, civilians, pickups and checkpoints it cannot get to become warnings on the canvas and in the Warnings panel. `routes()` reuses the same moves (`moves()`) for the places nobody can leave toward the exit (`level.trap`), the forward-only camera of `engine/game.ts` (`level.camera`) and the walk to the exit in frames (`level.timer`). The Buenos Aires template passes it with no warnings (a test keeps it so).

### `board/`: platform profiles

Each board is a profile object. Adding a board means adding a profile:

```ts
interface BoardProfile {
  id: "cps1";                         // more later
  screen: { w: 384; h: 224; fps: 60 };
  layouts: SetLayout[];               // slammast (4×3), captcomm (4×2)
  layers: LayerSpec[];                // far 32 px, play 16 px, text 8 px
  colors: { bits: 12; snap(rgb): rgb; deltaE(a, b): number };
  palettes: { sprite: 32; play: 32; far: 32; colorsPer: 15 };
  rom: { graphicsBytes: 6 * 1024 * 1024; programBytes: 2 * 1024 * 1024 };
  sprites: { perScreen: 256; tile: 16 };
  meters(project): Meter[];           // what the top bar shows
  convert: { tiles(), sprites(), palettes() }; // to the board's format (Phase 2)
}
```

`cps1.ts` takes its numbers from [hardware.md](../rom/hardware.md) and [art-spec.md](../rom/art-spec.md).

### `io/`

- **Storage** (`io/storage.ts`, `io/assets.ts`): each project's JSON in `localStorage` under `go-link.wm.p.<id>`, plus an index `go-link.wm.index` for the list; pictures in IndexedDB (`go-link-willy-maker`, store `assets`) by SHA-256. Every access is wrapped: when storage is blocked or full the top bar says "not saved" and the work stays in memory (pictures too). Autosave runs 0.7 s after the last change and when the page is hidden. Deleting a game also deletes the pictures no other saved game uses.
- **Project `.zip`** (`io/zip.ts`, `io/projectZip.ts`): a small zip writer and reader of our own, using the browser's `CompressionStream("deflate-raw")` (no library, no WebAssembly needed, no network); the reader checks every CRC and refuses archives over the size limits. Import checks the format, each picture against its hash and lists missing pictures before anything is saved; a game with the same id asks: replace, or keep both.
- **Bad files** (`model/inputError.ts`): every reader (zip, project, Tiled, sprite sheets in `sprites/sheetInput.ts`) throws an `InputError` with a code; the UI shows the module's sentence for it (`inputErrors` in `i18n/core.*.ts`), never a raw exception. Sprite sheets are checked by size from their header before the browser decodes them. When `localStorage` or IndexedDB is full, the editor shows an alert and keeps the last good save.
- **AI pack build check** (`io/packCheck.ts`): validation level 2 for the pack, run by `buildAiPack` on its own zip before the download.
- **Tiled import** (`io/tiled.ts`): `.tmj`/`.json`, or `.tmx` with CSV data, on a 16 px grid: the `collision` layer's tile types (or their order) become tags, the `objects` layer's typed objects become objects. Tiled export comes with the AI pack.
- **Starter pictures** (`io/starter.ts`): a game made from a template fetches the starter tilesets once and stores them by hash like any other picture.
- **AI pack**: the Tiled maps, the PNG assets, the project JSON, our ROM docs and the generated prompt (next step).

### `ui/`: atomic design

- **Atoms** (`atoms.tsx`): capsule button, icon button with its tooltip, meter, tag and object swatch, field, segmented choice, card.
- **Molecules** (`molecules.tsx`): part button, tag chip, layer row (eye, lock, name renamed with a double click, its board layer, order), property row.
- **Organisms** (`organisms/`): `Home` (the games list and `Wizard`), `Ide` (top bar, tabs and the Build screen), `LevelCanvas`, `Panels` (project tree, parts, inspector, layers, warnings, board limits, minimap), `ExportView`; the Characters tab and play mode come from `sprites/` and `play/` through their entries, lazily.
- **Template**: `WillyMakerApp.tsx` (the home screen or one game's IDE, inside an error boundary).

The canvas is drawn with Canvas 2D (`ui/render.ts`): only the visible cells of each layer, tags as tinted cells with a top line, objects as labelled boxes, the grid with a stronger line every screen width, sections, reachability warnings and the 384 × 224 screen frame; pixels are never smoothed and colors come from the tokens. The play view draws at 384 × 224 and scales by whole numbers.

## Shared code with `rom/`: `@go-link/cps1`

The CPS-1 conversion code is one TypeScript package, [`frontend/packages/cps1`](../../frontend/packages/cps1) (`@go-link/cps1`), with no DOM and no Node APIs (pictures and ROM files are plain `Uint8Array`s) and its own tests. `rom/tools` imports it by path (Node runs TypeScript since 22.18) and Willy Maker imports it as a workspace package:

| Module | What it is | Use in Willy Maker |
|---|---|---|
| `color.ts` | OKLab, every CPS-1 color with its palette word, the 4096 full-brightness colors (`toBoardColor`), k-means | color snapping, palette zones, the color meters |
| `sprites.ts` | dominant-color downscale, per-tile palettes, `renderFrame` | the sprite importer (downscale); Phase 2: converting characters |
| `gfx.ts` | the board's graphics format (`GfxRegion`, ROM file split) | Create ROM: the font, the engine's sprites and the level's tiles into the graphics files |
| `kabuki.ts` | the sound CPU's encryption | Create ROM: the Z80 program for `slammast` |
| `font.ts` | go-link Console's 8 × 8 font for the text layer (`rom/tools/font.mjs` re-exports it), glyph checks | the Menus tab's previews and fit checks, play mode's Game over screen |
| `sets.ts` | the `slammast` set's files and sizes, the loader in reverse (`assembleProgram`, `joinGfx`) and forward (`splitProgram`: the 68000's space into the program files) | the power-on test; Create ROM |
| `screen.ts` | draws the board's screen from graphics RAM, the CPS-A/CPS-B registers and the graphics region (`renderScreen`), the palette formula (`paletteRgb`) | the power-on test's picture |

`level.mjs` (the prototype level) stays in `rom/tools` until the "Prototype street" template needs it, and `png.mjs` is not needed in the browser (the browser decodes images). The move changed no byte of the prototype's set: the 28 files of `slammast.zip` were compared one by one before and after.

### `sprites/`: the Characters screen

- `detect.ts`: background keying (flood fill from the border, magenta everywhere, strokes removed), figure and grid detection, reading order, feet pivots. Pure, tested on synthetic sheets.
- `convert.ts`: the character's one scale, the downscale (`@go-link/cps1`), the 16 px palette zones with their meters and reductions, the delta E figures, the 1:1 atlas packing.
- `character.ts`: the importer's draft, and saving it into the project (character, zone palettes, `source`) or removing it.
- `image.ts`: the only browser part (decode a picture, encode a PNG through a canvas), replaced in the tests.
- `ui/`: `SheetView` (the sheet and its editable boxes), `AnimationPanel`, `BoardPanel` (preview and zones); `CharactersScreen.tsx` puts them together and is the entry the shell mounts: `<CharactersScreen project onChange characterId? />`.

## The power-on test: `@go-link/cps1-sim` and `power/`

Validation level 3 ([validation.md](validation.md#level-3-power-on-in-the-browser)) runs a ROM set's 68000 program in the browser. It has two parts:

- [`frontend/packages/cps1-sim`](../../frontend/packages/cps1-sim) (`@go-link/cps1-sim`), with no DOM and no Node APIs:
  - `musashi/`: the 68000 emulator [Musashi](https://github.com/kstenerud/Musashi) 4.60 by Karl Stenerud, **MIT license**, vendored from commit `313ebf1` (`VENDORED.txt` lists the files kept and the one change: a hook in `m68ki_jump_vector` so the board sees every exception). Its notice is in `THIRD_PARTY_NOTICES.md` (`scripts/third-party-notices.sh`).
  - `c/board.c`: the board model. The `slammast` memory map as the core's driver wires it (program ROM, work RAM, graphics RAM, the CPS-A/CPS-B registers with the CPS-B-21 ID at `0x80016e`, the inputs, the sound latch, the QSound shared RAM and EEPROM port), the level 2 vblank interrupt held until taken, and 10 MHz / 60 cycles per frame. Any other address, a write to ROM, an odd word access, an exception or a stack outside work RAM stops the CPU and is reported. The Z80 and QSound are not run. `c/conf.h` is Musashi's configuration (a plain 68000), `c/nofpu.c` stands in for the floating point routines a 68000 never calls.
  - `wasm/cps1sim.wasm`: the build, **committed**, so the website and CI never need Emscripten. `tools/build.mjs` (`npm run wasm -w @go-link/cps1-sim`) builds it with `emcc` as a standalone module: no JavaScript runtime, no imports, a fixed 4 MB memory, only the `board_*` functions exported. `npm run wasm:check -w @go-link/cps1-sim` rebuilds it in a temporary folder and fails when the committed file differs; `wasm/cps1sim.json` records the `emcc` version, Musashi's commit and the SHA-256.
  - `src/sim.ts` (`BoardSim`, a thin wrapper over the exports), `src/powerOn.ts` (`runPowerOn(files, { wasm })`: the steps), `src/text.ts` (each result's English sentence) and `src/wasmUrl.ts` (the asset URL, imported only by the Worker).
- `src/willy-maker/power/`: `powerOn.ts` reads the zip with `io/zip.ts` and calls `runPowerOn`; `powerOn.worker.ts` fetches the `.wasm` (same origin, `application/wasm`) the first time a test runs and posts each step as it finishes; `client.ts` starts one Worker per test. `ui/organisms/PowerOnCard.tsx` is the Export tab's card. Nothing loads until the user drops a file: the Worker and the `.wasm` are separate files of the build.

The CSP already allows it: `script-src 'self' 'wasm-unsafe-eval'` (WebAssembly compilation, added for the MP4 conversion), the Worker and the `.wasm` are same-origin files, and nothing is fetched from another site.

## Why this shape

- **Movable**: one entry, its own folders, assets and CSS, and few dependencies (`@go-link/shared`, `@go-link/cps1`, `@go-link/cps1-sim`, `src/controllers`). It can become its own app or package.
- **One set of rules**: play mode and the ROM run the same physics and object rules, tested once.
- **Boards as data**: profiles keep the editor board-agnostic for Phase 3.
- **Nothing on a server**: like the rest of Tools, all work stays in the browser until the user exports it or, in Phase 2, sends it to their own go-link Console device.
