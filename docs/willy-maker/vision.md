# Vision: a game-making IDE for every genre and every board

Willy Maker is named after go-link Console's mascot, Willy Gorklingo, but it is not a maker of Willy games. Its goal is a broad game-making IDE: any of the [genres](genres.md), first squeezing the CPS-1 to its limits, then on bigger boards that MAME runs, and finally on go-link Console's own engine with modern 2D quality. Every game it makes must play in a go-link Console room.

## Principles

1. **A game is described once.** Levels, characters, animations, rules, menus and settings live in the project (see [file-format.md](file-format.md)), never in a board's code.
2. **A genre is an engine module.** Each genre (platform shooter today, the others in [genres.md](genres.md)) is a set of rules and parts: the editor shows its parts, the play mode runs its rules, and each board target has the same rules in its own CPU's code.
3. **A board is a target.** A board profile (`board/`) gives its limits, its converters and its prebuilt engine. Exporting to a board checks the project against that board (the [validator](validation.md) is already per profile) and converts it; a smaller board may need cuts (fewer colors, fewer sprites), which the validator explains before exporting.
4. **Nothing in the core is tied to Willy or to one genre.** Willy is the default character of a template, not a rule.
5. **Every target is validated the same way:** live rules, the export's self-check, a power-on test in the browser and on the linked device ([validation.md](validation.md)).
6. **The play mode is the same game.** What the user plays while building must behave like the board's ROM; a part the engine does not run yet is marked "coming soon", never silently ignored.

## The board ladder

| Step | Target | What it adds | Main limit |
|---|---|---|---|
| 1, today | **Capcom CPS-1** (384 × 224, the `slammast` layout of 4 players × 3 buttons) | Three scroll layers, 256 sprites, 6 MB of graphics; the board of the classic fighting and beat 'em up games of 1991 | 15 colors per tile, no rotation, no transparency |
| 2 | **Bigger boards that the mame2003-plus core runs**, to be evaluated: Taito F3 (transparency, per-line effects, much more graphics memory), CPS-2 (more memory; its encryption must be handled), Sega boards with sprite scaling | Effects and sizes the CPS-1 lacks | Each board is a new profile: its rules, its graphics converters and a genre engine in its CPU |
| 3 | **go-link Console's own engine**, go-link HD: a "virtual board", a library with its own API | HD output, colors without palette limits, skeletal animation, particles, shaders, updates: modern console-class 2D | It has to be written; it is not MAME, so it does not carry MAME's hardware or license limits |

Step 3 fits go-link Console as it is: the device runs go-link HD next to MAME's libretro core and streams it to the players the same way, and rooms, voice, controllers, recordings, invitations and phones keep working unchanged.

## What this asks of the code today

- The project format keeps `genre` and the board as separate fields (format 3 has `genre`), and converters work from board-agnostic data.
- The data-driven engine of stage 2 ("Create ROM", see [README.md](README.md#phase-2-create-rom-in-the-browser)) is the first genre engine on the first board: the pattern every later genre and board follows.
- New parts get an entry in `editor/support.ts` per genre engine, so the editor never offers what a target cannot run without saying so.

## The AI playtester

Its full specification, item by item: [ai-playtester.md](ai-playtester.md).

Every game made in Willy Maker should be playable by an AI before anyone else plays it: "Test with AI" trains a small player on that game and reports, with videos, how often a level is cleared, where players die, which jumps are almost impossible and where they get stuck.

- **Why it is cheap:** the pieces exist. The board simulator runs in WebAssembly (validation level 3), each ROM keeps an exact game state in RAM (experiment 1's `lab_state`), and a route bot already plays from it ([harness](../experiments/harness.md)).
- **The player:** a small policy network (thousands to a million parameters, under a millisecond per decision) trained on the game itself, by imitation of the route bot first and then by reinforcement learning (rewards for progress, rescues and clears; penalties for deaths and getting stuck). A general decision model such as Laya, tried in experiment 1, is not a game player: asked "next action?" with no training it answered "fire" in every state.
- **Where it runs:** in the browser, with the emulator in Web Workers on the CPU (one game per worker, many at once) and the network on WebGPU (many games' decisions in one step, faster training, and later players that read the screen instead of the game state), falling back to WebAssembly on the CPU where WebGPU is missing; or on the linked go-link Console device.
- **First step:** a short experiment on one ROM comparing the route bot, Laya asked small yes/no questions, and a trained player; the winner becomes the playtester.
- **Laya on Apple silicon:** run the same Laya player on the M1 with PyTorch's MPS backend (`--device mps`) to measure the real decision time against the 4.6 s per decision seen on an Intel CPU under load.
- **Real games as training data:** a room records only each frame's input packets (the 12-byte `input` of every seat), which are tiny. The core is deterministic from power on, so the board simulator replays a recording and rebuilds the exact `lab_state` of every frame offline: any game the host and friends play in a room (with their consent) becomes imitation data, without reading RAM live or slowing the room.
- **QA bots in real rooms:** bots that join a room as ordinary guests (invitation, PIN, WebRTC, the `input` channel) and play for hours, to stress-test the device, WebRTC and the core together: memory over time, frame pacing, input latency, reconnections, seat changes and four players at once.

## Order of work

1. Stage 2 on the CPS-1 (Create ROM), informed by [experiment 1](../experiments/README.md).
2. The AI playtester.
3. More genres on the CPS-1, in the order of [genres.md](genres.md), each one as an experiment with a frozen spec and a jury.
4. The first bigger board, chosen when a genre needs what the CPS-1 cannot do.
5. go-link Console's own engine (its base: [go-link-hd.md](../go-link-hd.md)).
6. **The second evolution: code.** Scripts in a language close to TypeScript and Scratch-like blocks that generate them, run by go-link HD as a sandboxed WebAssembly module, so a game goes from visual to code in the same tool ([code.md](code.md)).
