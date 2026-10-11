# Experiment 1: the harness

The shared test bench of [experiment 1](README.md#the-harness). Every case and every juror uses it, the same way, from the command line. It reads the game's state from RAM, plays a ROM set with an input script or an automatic player, records the state of every frame, PNG frames and an MP4, replays the same script on the real core and compares the pictures.

| Piece | File | What it does |
|---|---|---|
| Lab state | [`rom/src/lab_state.h`](../../rom/src/lab_state.h) | The struct every ROM of the experiment fills each frame, at the symbol `lab_state` |
| Symbol map | `rom/tools/build.mjs` | Writes `build/<set>.symbols.json` and `build/symbols.json` next to the zip (from `m68k-elf-nm`) |
| Level 3 | [`rom/tools/lab/validate.mjs`](../../rom/tools/lab/validate.mjs) | Willy Maker's power-on test (`@go-link/cps1-sim`) from the command line |
| Runner | [`rom/tools/lab/run.mjs`](../../rom/tools/lab/run.mjs) | Plays a zip in the simulator with a script or a player; state, PNG, MP4 |
| Real core | `device romtest --input` ([`romreplay.go`](../../backend-device/cmd/device/romreplay.go)) | The same script on mame2003-plus; checkpoint PNGs, MP4 |
| Compare | [`rom/tools/lab/compare.mjs`](../../rom/tools/lab/compare.mjs) | Pixel difference between the simulator's and the core's frames |
| Route bot | [`rom/tools/lab/bot.mjs`](../../rom/tools/lab/bot.mjs) | Plays from the lab state and the collision map |
| Laya player | [`rom/tools/lab/laya_player.py`](../../rom/tools/lab/laya_player.py) | Asks the local Laya model "next action?" each decision |
| QA run | [`rom/tools/lab/qa.mjs`](../../rom/tools/lab/qa.mjs) | Adversarial players and a seeded 4-port fuzz, invariants on the lab state every frame |
| Minimizer | [`rom/tools/lab/ddmin.mjs`](../../rom/tools/lab/ddmin.mjs) | Reduces a finding's inputs to a short script that replays on the core |
| Acceptance | [`rom/tools/lab/acceptance.mjs`](../../rom/tools/lab/acceptance.mjs) | Acceptance 1 to 5 and the QA run on one zip, in one command |
| Prototype script | [`rom/tools/lab/runs/prototype-clear.json`](../../rom/tools/lab/runs/prototype-clear.json) | The scripted clear of the current prototype, with expectations |

## Setup

```sh
# the ROM toolchain (rom/README.md) and Node 22.18 or newer (the tools import TypeScript)
brew install m68k-elf-binutils m68k-elf-gcc z80asm
source ~/.nvm/nvm.sh && nvm use 22.23.2
(cd frontend && npm ci)            # links @go-link/cps1 and @go-link/cps1-sim
# ffmpeg for the MP4s (/usr/local/bin/ffmpeg, or set FFMPEG=...)
# the device CLI with "romtest --input": make device-darwin-universal
#   -> dist/device/darwin-universal/go-link-console.app/Contents/MacOS/go-link-console
# the emulator core: device core download (~/go-link-console/cores)
```

The Laya player needs Python, PyTorch, Transformers and the `laya` package, and the model on disk:

```sh
/usr/bin/python3 -m venv ~/go-link-lab/venv          # Python 3.9 on an Intel Mac, see below
~/go-link-lab/venv/bin/pip install "torch==2.2.2" "numpy<2"
~/go-link-lab/venv/bin/pip install transformers laya huggingface_hub
~/go-link-lab/venv/bin/python -c "from huggingface_hub import snapshot_download; \
  snapshot_download('convaiinnovations/laya-multilingual', local_dir='$HOME/go-link-lab/models/laya-multilingual')"
```

On an Intel Mac the newest PyTorch with macOS x86_64 wheels is 2.2.2, for Python 3.8 to 3.12: `pip` in a Python 3.14 venv finds no `torch` at all ("No matching distribution found"). The system Python 3.9 works: torch 2.2.2, numpy 1.26.4, transformers 4.57.6, laya 0.3.4 (2026-10-01). On an Apple Silicon Mac any current Python and PyTorch work. The model is 647 MB (`model.safetensors`, the mmBERT tokenizer and config).

## The lab state

Every ROM of the experiment keeps one struct in work RAM up to date, once per frame, after the frame's game logic: `struct lab_state` in [`rom/src/lab_state.h`](../../rom/src/lab_state.h), at the exported symbol `lab_state`. go-link's linker script puts it in its own section at **0xff0000**, the start of work RAM (`.lab_state`, `NOLOAD`, before `.data`). The harness finds it by the symbol in `symbols.json`, and, when a build has no symbol map, by scanning work RAM for the magic `LAB1` at an even address (`summary.json` says which: `labState.from` is `symbol` or `magic`; a run with neither records no state).

All fields are big-endian (the 68000's order). Offsets in hex:

| Offset | Size | Field | Meaning |
|---|---|---|---|
| 00 | 4 | `magic` | `LAB1` (0x4c414231) |
| 04 | 2 | `version` | 1 |
| 06 | 2 | `size` | 0xc8 (200) |
| 08 | 4 | `frame` | vblanks since power on |
| 0c | 1 | `mode` | 0 boot, 1 title, 2 playing, 3 clear, 4 game over |
| 0d | 1 | `credits` | |
| 0e | 1 | `section_clear` | 1 once the section is cleared |
| 0f | 1 | `flags` | 0x01 the level has an exit, 0x02 it clears when every civilian is rescued, 0x04 enemies hurt players, 0x08 the exit clears on touch (without it, an exit needs every enemy down; the QA run reads it) |
| 10 | 2 | `cam_x` | s16, world px of the screen's left edge |
| 12 | 2 | `cam_y` | s16, world px of the screen's top edge |
| 14 | 2 | `level_w` | px |
| 16 | 2 | `level_h` | px |
| 18 | 2 | `exit_x0` | s16, the exit zone's left world px, -1 for none |
| 1a | 2 | `exit_x1` | s16, its right px, -1 for none |
| 1c | 2 | `exit_y` | s16, the feet y of its floor, -1 for none |
| 1e | 1 | `n_enemies` | entries used in `enemy[]` (at most 8) |
| 1f | 1 | `n_civilians` | entries used in `civ[]` (at most 4) |
| 20 | 4 | `col_map` | address of the collision map, 0 for none: one byte per 16 × 16 cell, row-major: 0 empty, 1 solid, 2 one-way, 3 ladder, 4 crate |
| 24 | 2 | `col_cols` | |
| 26 | 2 | `col_rows` | |
| 28 | 4 × 16 | `player[4]` | at 0x28 + 16n: `+0` active (u8), `+1` state (u8: 0 off, 1 idle, 2 walk, 3 run, 4 air, 5 climb, 6 attack, 7 hurt, 8 dead, 9 crouch, 10 crawl: crouched on the ground, still or moving), `+2` facing (s8, +1 right, -1 left), `+3` energy (0-3), `+4` x (s16, world px, the body's middle), `+6` y (s16, world px of the feet), `+8` score (u32), `+c` hurt (frames of blinking left), `+d` flags (0x01 on the ground, 0x02 climbing, 0x04 running, 0x08 firing, 0x10 a jump kick's frames, 0x20 the jet pack lifts, 0x40 the double jump is used until landing; decoded as `ground`, `climbing`, `running`, `firing`, `kicking`, `jetting`, `airJump`), `+e` vy (s16, 1/16 px per frame) |
| 68 | 8 × 8 | `enemy[8]` | at 0x68 + 8n: `+0` alive, `+1` hp (hits left), `+2` x, `+4` y (feet), `+6` facing (s8), `+7` state (0 off, 1 walk, 2 hit, 3 down) |
| a8 | 4 × 8 | `civ[4]` | at 0xa8 + 8n: `+0` rescued, `+1` present, `+2` x, `+4` y (feet), `+6` reserved (0) |

The header checks every offset at compile time (`_Static_assert`). A case that writes its ROM another way must give the same bytes at a symbol named `lab_state` (or anywhere in work RAM with the magic); the map in `col_map` may be in RAM (a copy that changes, like broken crates) or in ROM.

**The current prototype** (`rom/src/main.c`, `lab_update()`): fills it at the end of every title and playing frame. It has no exit and no damage (`flags` = 0x02, `exit_*` = -1, energy always 3), its section clears when both civilians are rescued (`mode` 3, `section_clear` 1, the "MISSION COMPLETE!" screen), and it never reaches game over. Its robots take 4 hits and give 500 points; civilians give 1000.

**Symbol map:** `node rom/tools/build.mjs` also writes `rom/build/<set>.symbols.json` and `rom/build/symbols.json`: `{ "set", "lab_state": { address, size, type }, "symbols": { name: { address, size, type } } }`, every symbol of the program (`m68k-elf-nm -S -n`, static ones included, e.g. `col_map`, `pl`, `robot`). The build fails when the program has no `lab_state`.

## Input scripts

One JSON format for the simulator, the real core and every recorded run (`inputs.json`). Frames count from power on, so a script's steps after Start depend on when play begins: since 2026-10-03 the go-link engine starts play a fixed `SETUP_FRAMES` (10) frames after Start, whatever setting the level up took (it took about 4 frames and ended near a vblank on some levels, where the real core and the board model could start a frame apart). The engine's own scripts (`spec-*.json`, `platforms-*.json`) were shifted by those 6 frames then: play begins at frame 163 and the spec level clears at frame 1889 (1883 before). The prototype's and experiment 1's recorded runs keep their own ROMs and frames.

```json
{
 "name": "prototype-clear",
 "frames": 1300,
 "checkpoints": [150, 300, 600, 900, 1113, 1200, 1300],
 "expect": [
  {"frame": 1113, "path": "sectionClear", "equals": true},
  {"frame": 1300, "path": "players.0.score", "min": 3500}
 ],
 "steps": [
  {"from": 120, "to": 125, "port": 1, "buttons": ["coin"]},
  {"from": 150, "to": 155, "port": 1, "buttons": ["start"]},
  {"from": 156, "to": 221, "port": 1, "buttons": ["right"]}
 ]
}
```

- `steps`: buttons held on a port (1-4, default 1) from frame `from` to frame `to`, both included. Buttons are the device's names: `up`, `down`, `left`, `right`, `b1`-`b6`, `start`, `coin`. It is the device's framelab script (`FROM-TO:BUTTONS@PORT`) in JSON. On the board, as `slammast` wires it: P1/P2 directions and buttons 1-3 at 0x800000, their Coin and Start in the system byte, P3/P4 at 0xf1c000/2 with Coin at 0x40 and Start at 0x80, P3/P4 button 3 at 0x80/0x8000 of 0x800000; `b4`-`b6` do nothing. On the core, the RetroPad mapping (B/A/Y = buttons 1-3, Select = Coin).
- **Frame numbers:** frame N is the picture and the state after N frames have run since power on, the same in every tool. The steps' frames are the frame being run (0-based), so a step `from: 120` acts on the frame that ends as frame 121.
- `checkpoints`: frames saved as PNG (simulator and core) and with their state and the **screen text** in `summary.json` (the simulator's: 28 lines of 48 characters read from the text layer, scroll1, `Machine.text()`; a double-size letter shows with spaces after it, "C O N T I N U E ?").
- `expect`: checks on the lab state after a frame: `path` is a dot path into the decoded state (`mode`, `sectionClear`, `players.0.x`, `civilians.1.rescued`, `enemies.2.alive`...), with `equals`, `min` and/or `max`. Every failed expectation makes the run fail (`summary.json` `expect`, `expectOk`). The real core ignores them (it has no state).
- `expect` also checks the **screen text** (task T-11): `{"frame": 1320, "text": "INSERT COIN"}` passes when the screen shows it (`row` limits it to one line; spaces are left out of the comparison too, so `CONTINUE?` matches double-size text), `{"frame": 1440, "row": 17, "noText": "INSERT COIN"}` when it does not. The core gives no text, so these run on the simulator; its frames matching the core's (tolerance 0) covers the core. `rom/tools/lab/runs/spec-continue.json` and `spec-clear-tally.json` use them on Game Spec v1 made with Create ROM (`WM_ROM_OUT` of `rom.test.tsx`): the Continue screen with and without credits, the HUD kept under it, and the clear's tally (T-12).

Each case keeps its clear script as `harness/runs/clear.json` in its folder; it is written for its own level and must reach the exit with every enemy down.

## The simulator runner

```sh
node rom/tools/lab/run.mjs ZIP --out DIR --script FILE.json [--mp4] [--png-every K] [--checkpoints 300,600] [--frames N]
node rom/tools/lab/run.mjs ZIP --out DIR --player "COMMAND" [--every 6] [--frames 3600] [--mp4]
```

| Option | Default | Meaning |
|---|---|---|
| `--frames N` | the script's `frames`, else 3600 | frames to run |
| `--checkpoints LIST` | the script's | frames saved as PNG and kept in `summary.json` |
| `--png-every K` | off | also every K-th frame as PNG |
| `--mp4` | off | every frame into `run.mp4` (H.264, 768 × 448, nearest neighbour, 60 fps) |
| `--scale S` | 2 | the MP4's scale |
| `--every N` | 6 | player: frames per decision (at least 4) |
| `--player-port P` | 1 | player: the port it plays |
| `--timeout-ms MS` | 120000 | player: the longest wait for one answer |
| `--after-clear N` | 120 | player: frames run after the section clears |
| `--symbols FILE` | `<set>.symbols.json` next to the zip | the symbol map |

**Outputs in DIR:** `state.jsonl` (one line per frame: `{"f": N, "in": "right@1 b2@1", "lab": {...}}`, the decoded state with `mode`, `credits`, `sectionClear`, `flags`, `cam`, `level`, `exit`, `players[4]`, `enemies[n]`, `civilians[n]`), `inputs.json` (what was pressed, as a script: a player's game replays anywhere, the real core included), `frames/fNNNNNN.png`, `run.mp4`, `summary.json` (cleared and its frame, game over, start frame, checkpoints with their state, expectations, decisions, think time, actions used, energy lost, falls out of the map, the final scores, rescued civilians and enemies down, wall time, error), and for players `decisions.jsonl` (each answer with the frame, the player's x and y, the time it took and every field the player added) and `player.log` (its stderr). The exit status is 1 when the run has an error (a 68000 fault, a player failure, a failed expectation).

The MP4 runs at 60 fps from frame 1, so frame N is at N/60 s: the state of any moment of the video is line N of `state.jsonl`.

**Deterministic:** the board model has no clock or randomness; the same zip, inputs and player answers give the same frames, byte for byte (two runs of the same script give identical `state.jsonl` and PNGs).

**Input timing (found while building the harness):** the board model raises the vblank (where our programs read the controls) at the start of its frame, while mame2003-plus polls the controls at the start of `retro_run`, so without care the simulator reacts one frame before the core: from the first press on, every simulator frame N matched core frame N + 1. The runner therefore delays inputs by one frame (`Machine.open(zip, { inputDelay: 1 })`, the default). It also draws sprites from the previous frame's sprite table, as the board shows them. With both, the prototype's 900-frame test script gave the **same pixels as the core on all 900 frames** (tolerance 0), and the 1300-frame clear script matches at all 7 checkpoints.

**Opposite directions (experiment 1, J-17):** the core never delivers left with right, or up with down, held together; the board model passed both, so runs with those inputs diverged. Since the verdict, `@go-link/cps1-sim` (`Sim.inputs`) releases both directions of such a pair on every port, as the core does, and Willy Maker's engine and play mode cancel them too; the jury's `opposites` script on the spec level now matches the core at 0 %.

### The player protocol

The runner starts the player with `/bin/sh -c COMMAND` and talks JSON lines on its stdin and stdout (its stderr goes to `player.log`). The emulation **waits** for every answer.

1. Runner → player, once: `{"type": "hello", "protocol": 1, "set": "slammast", "actions": [...], "every": 6, "port": 1, "screen": {"w": 384, "h": 224}}`. No answer.
2. The runner presses Coin (frames 120-125) and Start (150-155) on the player's port itself; the player is asked from the first frame whose `mode` is `playing` (if that has not happened by frame 900, the run fails).
3. Runner → player, every `every` frames: `{"type": "state", "frame": F, "port": 1, "last_action": "right", "lab": {...}, "map": {...}}`. `lab` is the decoded state (as in `state.jsonl`). `map` comes with the first state and again whenever the collision map changes (a crate breaks): `{"cols", "rows", "cell": 16, "legend": {...}, "rows_text": ["....", "##=H"]}` with `.` empty, `#` solid, `=` one-way ledge, `H` ladder, `C` crate.
4. Player → runner, one line per state: `{"action": "right", ...}`. `action` must be one of the closed list; any other field is logged in `decisions.jsonl`. `{"error": "why"}` stops the run with that error.
5. Runner → player at the end: `{"type": "end", "summary": {"cleared", "frames", "error"}}`, then its stdin closes; a player still running after 5 s is killed.

**Actions** (the buttons held on each frame k = 0 … N-1 of the decision's window):

| Action | Buttons |
|---|---|
| `right` / `left` | the direction |
| `jump` | B1 + the facing direction on frames 0-2, then the facing direction (a forward jump; steer in the air with `right`/`left`) |
| `run_right` | a double tap: right, right, nothing, right… (frames 0-1, 3-); held for the whole window when the previous action was also `run_right` |
| `fire` | B2 on every frame but the last, so the next `fire` is a new press (a knife when an enemy is adjacent) |
| `climb_up` / `climb_down` | up / down |
| `drop` | down, with B1 on frames 1-2 |
| `wait` | nothing |

## The real core

```sh
go-link-console romtest --input FILE.json [--frames N] [--checkpoints 300,600] \
    [--frames-dir DIR] [--png-every K] [--mp4 FILE] [--json] ZIP
```

It runs the script frame by frame on the exact core (the one in `~/go-link-console/cores`, `--core` for another), unpaced, from power on, with a fresh system folder in `~/go-link-console/tmp/romreplay-<pid>/` (deleted at the end) and a copy of the zip named after the set. It saves the checkpoint frames (and every K-th) as `DIR/fNNNNNN.png` at the core's size, with `--mp4` every frame as an MP4 like the runner's, and with `--wav FILE` the core's stereo sound (48 kHz, 16-bit; task T-26). `--json` prints `{type: "rom_replay", set, core, fps, width, height, frames, seconds, checkpoints: [{frame, png, hash}], pngs, mp4}`. It never reads `device.json` or the ROM folder and does not touch a running device. The core gives no RAM access through libretro here, so the core's run has pictures but no lab state.

## Comparing frames

```sh
node rom/tools/lab/compare.mjs SIM_DIR CORE_DIR [--out DIR] [--tolerance 8] [--frames LIST] [--search K] [--json]
```

For every `fNNNNNN.png` in both folders: the share of pixels that differ by more than the tolerance in any channel, the share that are identical and the mean difference. `--out` writes `diff-NNNNNN.png` (white where they differ), `pair-NNNNNN.png` (simulator | core | difference) and `compare.json`. `--search K` also tries the core's frames N-K … N+K (when they exist, e.g. with `--png-every 1`) and reports the best offset: this is how the one-frame input delay above was found.

## The players

**Route bot** (`node rom/tools/lab/bot.mjs [--seed S] [--noise P] [--no-fight] [--verbose]`): it turns the collision map into standing spots (a cell top with room for a 40 px body) and moves between them: walk, push up a wall of at most 32 px (a crate), fall off an edge, drop through a one-way ledge, climb a ladder, and jump up to 64 px (4 cells) onto a ledge up to 3 columns away. It finds the cheapest path (Dijkstra) to the nearest goal: unrescued civilians, then, when the level has an exit, every live enemy and then the exit. It shoots an enemy on its floor within 170 px (turning first), steers toward the landing spot in the air, and when it has not moved for 16 decisions it jumps and forbids that move. It answers with the same closed list as Laya, so both play with the same controls. Without `--seed` it is deterministic; with a seed, a share `--noise` (default 0.05) of its decisions are random actions from a seeded generator, so seeded games differ from each other and repeat exactly.

**Laya** (`~/go-link-lab/venv/bin/python rom/tools/lab/laya_player.py [--model DIR] [--device cpu] [--seed S]`): loads [convaiinnovations/laya-multilingual](https://huggingface.co/convaiinnovations/laya-multilingual) (Apache 2.0, 322M, mmBERT-base and a decision head) with the `laya` package, offline, and for every state asks one typed `choice` question: an instruction ("you control `you` in a 2D side-scrolling platform game… what is the next action?") with the nine actions and a description of each as the options. The state given to the model is what a player sees, from its own point of view: its position, pose, energy and score; the nearest goal and enemy as distances (`dx`, `dy`, same floor, in front); and the cells around it read from the map (a ladder here, a wall left or right, a one-way ledge underfoot, a ledge within 64 px above, a gap ahead). It never gets the bot's route. It plays the most likely action (with `--seed`, it samples from the probabilities instead) and returns `prob`, every action's probability, the model's confidence, the time and the token count, which `decisions.jsonl` keeps. If the package or the model is missing, it answers the first state with an `error` and the run fails with that reason; it never chooses another way.

## The QA run

Task T-08 of [the verdict](verdict.md) (lesson L-03): the scripted runs, the bots and Laya never left the route, so none of them found the bugs a casual pair of players walks into (J-01, J-03, J-05, J-06, J-08). The QA run plays like those players, checks the game's own state every frame and turns every finding into a short script that replays on the real core.

```sh
node rom/tools/lab/qa.mjs ZIP --out DIR [--players newcomer,masher,run-past,skipper,shooter] [--frames 7200] \
    [--fuzz-minutes 2] [--seed 1] [--exit-rule auto|all|touch] [--patrol 160] \
    [--no-minimize] [--min-seconds 30] [--min-budget 120] [--symbols FILE] [--quiet]
```

It runs every player in the board model (`lab.mjs` `Machine`, input delay 1 like `run.mjs`), each from power on, for at most `--frames` frames (default 7200, two minutes); a game stops 120 frames after SECTION CLEAR, 60 after game over, and at its first `stuck` finding. Then the fuzz runs `--fuzz-minutes` minutes. Everything is deterministic: the same zip and options give the same findings and scripts.

### The players

| Player | What it does |
|---|---|
| `newcomer` | One player holds right, fires every 30 frames and jumps every 90 (the jury's naive player) |
| `masher` | Two players (P1 and P2 join) hold right, fire and jump every 20 frames |
| `run-past` | One player holds right and jumps every 20 frames, never fires |
| `skipper` | The [route bot](#the-players) with every ladder hidden from its map: it follows the route but never climbs; after 6 s without headway it gets the real map and tries to walk back and climb (the jury's `skip-upper`) |
| `shooter` | Fires from the start, 10 s standing, then walks right firing, turning back for 10 frames every 4 s |
| `fuzz` | Seeded 4-port random input (`--seed`): every port holds a random direction (P1 mostly right, never opposite directions) and random buttons for 3 to 40 frames; each port inserts a coin and presses Start every 40 s, staggered, so players keep joining and continuing |

The scripted players insert a coin at frame 120 and press Start at 150 (the masher's P2 15 frames later), and their pattern starts on the first playing frame.

### The invariants

Checked on the lab state after every frame. Kinds marked high make the run's verdict fail.

| Kind | Severity | When |
|---|---|---|
| `fault` | high | The 68000 faulted (the game stops) |
| `frozen` | high | The lab state's `frame` stayed the same for 120 frames |
| `stuck` | high | 30 s of trying (a direction held by a live player) while playing with no progress: no new 16 px cell visited by any player, no score, no enemy hit or down, no rescue, nobody joining. `detail.why` says "an enemy the exit needs is behind the camera" when that is the case (J-01) |
| `clear-enemies-alive` | high | SECTION CLEAR while an enemy is alive and the exit needs every enemy: `--exit-rule all`, or `auto` with flag 0x01 set and 0x08 clear (J-02) |
| `clear-civilians-left` | high | SECTION CLEAR while a civilian waits and flag 0x02 says the section needs every rescue |
| `lab-lost` | medium | The lab state disappeared after the game had one |
| `nobody-alive` | medium | Playing for 10 s with no live player |
| `body-in-solid` | medium | A live player's middle (feet y − 24) in a solid or crate cell, two frames in a row |
| `feet-in-floor` | medium | Feet inside a solid or crate cell (feet y − 1) with the body free, two frames in a row (J-05, J-06) |
| `standing-on-air` | medium | On the ground, not climbing, feet on a cell top and nothing under x − 6 … x + 6, three frames in a row |
| `player-outside-map` | medium | A live player outside the level |
| `camera-outside` | medium | The camera shows outside the level |
| `energy-over-start` | medium | Energy above what the player joined with |
| `score-down` | medium | A player's score went down during a game |
| `enemy-in-solid` | medium | An enemy's feet cell (y − 8) solid or crate, two frames in a row |
| `enemy-outside-map` | medium | An enemy outside the level |
| `enemy-off-patrol` | medium | An enemy more than `--patrol` px (default 160) sideways, or 8 px up or down, from where it appeared |
| `player-off-screen` | low | A live player outside the picture |
| `credits-over-9` | low | More than 9 credits |

**Deduplication:** a finding is its kind and the 16 px cell of its place (the feet's row); a `stuck` behind the camera is one finding per set of enemies left behind, wherever the players stand. A player run keeps at most 5 cells per kind and counts the rest; across players the same key is merged (`runs`, `count`) and the instance with the fewest inputs is kept. Every finding keeps its kind, severity, player, frame, x, y, details and **every input up to that frame** as a script.

### The minimizer

```sh
node rom/tools/lab/ddmin.mjs ZIP --script FILE.json --kind KIND --out MIN.json [--x X --y Y] [--max-seconds 120] [--exit-rule R] [--symbols FILE]
```

`qa.mjs` calls it for every high finding and for the first finding of each other kind, within `--min-seconds` per finding and `--min-budget` in all; on its own it takes any input script and finds the script's first finding of KIND (or the one near `--x`/`--y`). A replay "still triggers" when the checker reports the same kind within 192 px across and 112 px up or down of the original, at most 600 frames after it. It cuts the inputs after the finding, then tries coarse cuts (a whole port, a whole button, all of one button's presses merged into one hold), then delta debugging over the run-length steps (removing chunks, the latest first) and finally shortens each hold that can lose its second half. Each replay restarts from a snapshot of the board taken just before the first frame whose inputs changed (`Machine.snapshot()`/`restore()`; the checker's memory is copied with it), so cutting late steps costs only the frames after them. The result is a harness input script with the finding's frame as a checkpoint and expectations the simulator checks (`sectionClear` and the live enemies for a clear, `mode` playing for a soft-lock, the player's x and y otherwise).

### Outputs

In DIR: `qa.json` (the zip, `ok` — no high finding —, the verdict, the options, frames played, whether any run cleared, one entry per player run with its frames, clear, game over, the energy its players lost (`energyLost`: every hit, and the last energy of a player who went out), finding counts and time, the findings with their minimization, and the seconds spent playing and minimizing), `qa.md` (the same as tables, ending with what a naive player, the newcomer, lost; the acceptance run copies it as `tests.qa.naive`, task T-15), `findings/NN-KIND.inputs.json` (every input up to the finding) and `findings/NN-KIND.min.json` (the minimized script). The exit status is 0 whenever the run itself worked; the verdict is in the JSON.

To replay a finding, on the simulator and on the real core, and compare the pictures:

```sh
node rom/tools/lab/run.mjs ZIP --out SIM --script DIR/findings/03-stuck.min.json --checkpoints 600,1200,1800,2400,3000,3462
go-link-console romtest --input DIR/findings/03-stuck.min.json --checkpoints 600,1200,1800,2400,3000,3462 --frames-dir CORE ZIP
node rom/tools/lab/compare.mjs SIM/frames CORE --tolerance 0
```

### Results on experiment 1's ROMs (2026-10-02, Intel Mac)

The defaults (5 players, 2 minutes of fuzz, seed 1). "Fresh" is a Create ROM build of the spec level from `main` at `39314a1` (`cd frontend && WM_ROM_OUT=DIR CI=true npx vitest run apps/web/src/willy-maker/rom/rom.test.tsx`).

| ROM | Verdict | High findings (players, frame, place → minimized steps) | Time |
|---|---|---|---|
| X (Create ROM, the experiment's) | not ok | J-01 soft-lock: skipper f3298 (1079, 416) → 8 steps; masher and run-past f3466 (1524, 369), every enemy behind the camera → 3 steps (coin, Start, hold right). J-03 crate trap (no headway): newcomer and shooter f2128 (186, 413) → 4 steps (coin, Start, one shot, hold right) | 7 s playing, 64 s minimizing |
| Y (by hand) | not ok | J-01: newcomer, skipper and shooter f3301 (1109, 400) → 7 steps; masher and run-past f3453 (1524, 397) → 4 steps | 7 s + 60 s |
| Z (AI pack) | not ok | J-02 clear with enemies 0, 1, 2 alive: masher and run-past f1673 → 6 steps (coin, Start, hold right, three jumps); J-03 crate trap: newcomer f2250 (218, 369) and shooter f2811 (218, 416) → 4 steps each; skipper f3327 behind the camera → 8 steps | 6 s + 99 s |
| Fresh Create ROM | not ok, J-01 only | J-01: skipper f3298 (1079, 416) → 8 steps; masher and run-past f3466 (1524, 369) → 3 steps. No crate trap (T-03), no clear with enemies alive | 8 s + 46 s |

No medium or low finding came up in these runs. The medium invariants were checked on the jury's reductions: J-06's `join-sink.json` on Z gives `feet-in-floor` at frame 548, J-05's `drag-sink.json` on Y gives `body-in-solid` at frame 3804 (the player sank into the crates). On Z, the skipper's soft-lock is real for a player who plays by the spec's rule: Z's lab flags say its exit needs every enemy, and its exit does not (J-02).

**Same as the core:** three minimized scripts replayed on mame2003-plus (`romtest --input`) gave the simulator's pixels exactly (tolerance 0) at every checkpoint: X's J-01 (3 steps, 6 checkpoints up to 3462, 3.8 s on the core), Z's J-02 (6 steps, 4 checkpoints up to 1642, the clear frame) and the fresh build's J-01 (8 steps, 5 checkpoints up to 2673).

**Costs:** the board model with the per-frame checks runs about 3000 frames/s (the lab state is read straight from the board's memory, `Machine.labFast()`), so the six games (about 22,000 frames) take 6 to 8 s; a replay in the minimizer costs 0.3 to 0.8 s, and a finding takes 16 to 31 s (the skipper's, made of many short bot steps, stop at the 30 s cap with 7 or 8 steps). A whole QA run takes 54 to 106 s per ROM; `--no-minimize` takes under 10 s.

### Limits

- `stuck` is a heuristic: 30 s of trying without progress. A level that needs a long wait with a direction held, or a player that keeps walking into a wall on purpose, also counts; the minimized script and `detail.why` show which it is.
- The exit rule comes from the lab flags: Willy Maker's engine sets 0x08 when a project's exit clears on touch (`R->exit_needs_enemies` off, `rom/engine/engine.c` `lab_update()`); a ROM that keeps no such flag can pass `--exit-rule touch`.
- The patrol check knows only where an enemy appeared, not its patrol range; enemies that chase players (Z's troopers) leave it by design.
- Two minutes of fuzz found nothing on these ROMs; the jury's 2-player fuzz reached J-01 after 87 to 91 s and its 30-minute 4-port runs found J-05 and J-06. Longer fuzz (`--fuzz-minutes 30`, about 35 s of play) and other seeds find more, at the cost of minimization time.

## Recorded sessions

```js
import { recordSession } from "./rom/tools/lab/session.mjs";
const s = await recordSession({ out: "DIR", url: "http://localhost:5180/tools/willy-maker", title: "Case C: Game Spec v1" });
await s.step(1, "New game", "Every game starts from the wizard", "button:has-text('New game')", (l) => l.click());
await s.finish();
```

Task T-17 of [the verdict](verdict.md) (lesson L-15): the recorder every case's session should use. Each step has a **fixed number** given by the script (inserting or removing a step never renumbers the others; a number used twice is an error), a caption in a fixed place (top center, bottom center when the control is right under it), the control outlined, and a **Why** card in the screen corner farthest from the control, so it never covers what is clicked. `finish()` writes `session.mp4` (H.264) with **one chapter per step** and the captions as a subtitle track, `chapters.vtt`, `captions.vtt`, `timeline.json` (each step's number, title, why, time, box and screenshot) and `session.md`, whose links open the MP4 at each step (`session.mp4#t=SECONDS`). It needs Playwright (`e2e/node_modules`, or `PLAYWRIGHT=`) and ffmpeg; it loads nothing else from the harness, so plain `node` runs it.

## One command: the acceptance run

```sh
node rom/tools/lab/acceptance.mjs ZIP --script clear.json --out DIR \
    [--bot-games 3] [--laya-games 1] [--bot-frames 5400] [--laya-frames 3600] \
    [--device PATH] [--python PATH] [--no-mp4] [--skip level4,core,bot,laya] \
    [--no-qa] [--qa-minutes 2]
```

It runs, in order: **level 3** (`level3.json`, `level3.png`), **level 4** (`device romtest --json`, `level4.json`), the **scripted run** in the simulator (`scripted/sim/`, with its MP4), the **same script on the real core** (`scripted/core/frames`, `run.mp4`, `replay.json`) and the **comparison** at the checkpoints (`scripted/compare/`), the **[QA run](#the-qa-run)** (`qa/`, on by default: `--no-qa` skips it, `--qa-minutes` sets its fuzz length, default 2), then **N bot games** and **N Laya games** (`bot/game-NN/`, `laya/game-NN/`, each a runner folder with its MP4; game 1 is the plain game, games 2…N pass `--seed 1…N-1`). `acceptance.json` has every test's verdict: `level3`, `level4`, `scripted` (cleared, expectations), `sameAsCore` (frames compared, worst difference; it passes at 0 %), `qa` (`ok` when there is no high finding, the verdict, counts by severity, frames played, seconds, and one line per finding with its kind, players, frame, place, why and the path of its minimized script), `bot` and `laya` (games, clears, clear rate, best/median/worst clear frame, energy lost, falls, errors, one line per game).

### What the cases and the jury run

```sh
cd /Volumes/HD12TB/github/MAME-WEBRTC
source ~/.nvm/nvm.sh && nvm use 22.23.2
# a case, after building its set (its zip named after the set, e.g. slammast.zip):
node rom/tools/lab/acceptance.mjs <case>/build/slammast.zip --script <case>/harness/runs/clear.json \
    --out <shared folder>/<case>/acceptance --bot-games 5 --laya-games 3
# a juror, to play a case by hand-written script or watch one game:
node rom/tools/lab/run.mjs ZIP --out DIR --script my-test.json --mp4 --png-every 60
node rom/tools/lab/run.mjs ZIP --out DIR --player "node rom/tools/lab/bot.mjs --seed 7" --mp4
```

A case whose ROM is not built with `rom/tools/build.mjs` must still put `lab_state` in work RAM (with the magic) and may give its own symbol map with `--symbols`; without the state the runner still plays and records frames, but the scripted run cannot check its expectations and the players get `lab: null` (the run is recorded as "no state").

## Results on the prototype (2026-10-01, Intel Mac)

`rom/build/slammast.zip` (the prototype, now with the lab state), `rom/tools/lab/runs/prototype-clear.json`:

| Test | Result | Time |
|---|---|---|
| Level 3 | every step passes | 0.2 s |
| Level 4 | every step passes; identified as the go-link set by its 28 hashes | 2.9 s |
| Scripted run (simulator) | clears at frame 1113, all 7 expectations pass | 9.4 s with the MP4 (1300 frames) |
| Same script on the core | identical pixels at all 7 checkpoints | 1.8 s (1300 frames) |
| Route bot, 3 games | 3/3 clear: frames 1113 (plain), 1415 and 1546 (seeds 1, 2); 3500 points each | about 10 s per game with the MP4 |
| Laya, 1 game | 0/1: 3600 frames, 574 decisions, all `fire`; it never left x 64; 300 points | 214 s (324 ms per decision on average) |
| The whole acceptance run | `acceptance.mjs` with 3 bot games and 1 Laya game | 262 s |

Rough costs: the simulator runs about 1600 frames/s with the state log and no pictures (1300 frames in 0.8 s), and about 140 frames/s when every frame is drawn for the MP4 (about 7 ms per drawn frame); the core replays about 700 frames/s; the bot thinks under 1 ms per decision; Laya loads in about 10 s and answers in 70-500 ms per decision on the CPU (about 410 input tokens per state), so a 3600-frame game (about 575 decisions at `--every 6`) takes about 4 minutes.

**Laya, zero-shot:** it loads and runs (no failure to record), but it does not play the game: on the prototype it answered `fire` to every one of the 574 states of a full game, with probability 0.97-0.99, while the goal was up and to the right and no enemy was near (the first state already said the goal was 240 px right and 144 px up, and a wall was right in front). This matches its model card's warnings (near chance on typed decisions zero-shot, over-confident, "fine-tune for a specific workflow"). Its games are recorded as they are: they measure what an untrained decision model does with the same controls as the bot.

## Limits

- The board model is not the core: it has no sound, no row scroll, starfields or flip screen, and draws layers without the priority masks (`@go-link/cps1` `screen.ts`). The same-picture test is what shows whether a ROM uses something it does not model.
- The lab state is what the ROM says about itself. A wrong `lab_state` gives wrong data; the pictures and the core replay are the independent check.
- The core's run has no state (no RAM access through libretro in the device), only pictures.
- One automatic player per run, on one port. A second player can be scripted with `steps` on port 2 (Coin and 2P Start), not yet as a second process.
- The bot knows the closed list's moves only: it cannot plan runs or long jumps across gaps wider than 3 cells, and it follows the prototype's rules (64 px jumps, 32 px pushes). A level that needs other moves shows up as games that do not clear.
- The players' prelude (Coin at 120, Start at 150) assumes the title accepts a coin and 1P Start at those frames.
