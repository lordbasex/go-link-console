# Code in Willy Maker (second evolution)

Willy Maker's first evolution is visual: draw levels, drop in characters, set rules in cards and play at once. Its **second evolution** lets people write the game's logic in code too, so the same tool takes someone from making a game visually to building their own experiences with their own code, the path that Scratch, Roblox Studio, Unity and Unreal Engine (Blueprints, then C++) offer. Willy Maker does not try to match those tools in size; it takes their creation path: start visual, then open the code when the visual parts are not enough.

This page is the plan before anything is built (2026-10-08). Every item is **Base** (the first version), **Later** or **Idea**.

## What changes for the person making a game

1. **Visual first, as today.** Levels, characters, rules cards and effects keep working with no code at all.
2. **Blocks.** Logic as blocks, in Scratch's style ("when the player touches a coin: add 1 to score, play a sound").
3. **The same blocks as code.** "Show as code" opens the text those blocks make; from there the person can keep writing code.
4. **Scripts on objects.** A script attached to an enemy, a door or a level changes how that object behaves (like a Unity component or a Roblox script); a game script runs the game's own rules (menus, modes, scores).
5. **Play while writing.** The play mode reloads the script on save, without leaving the level.

## Only on go-link HD

Code runs only in go-link HD games. The CPS-1 board's engine is a prebuilt 68000 program that reads the game as data; it cannot host a scripting runtime in its memory and speed budget. The visual tools keep working on every board. This is one more reason for go-link HD to become the default target ([vision.md](vision.md#the-board-ladder)).

## The runtime: WebAssembly inside the engine

The engine (the `golink-hd` repository, C99) runs a game's code as a **WebAssembly module**, through a small interpreter linked into the engine (candidates: wasm3 or WAMR's interpreter, both C with permissive licenses; chosen in phase C1). WebAssembly fits the engine's rules better than embedding a scripting language directly:

| Need | Why WebAssembly meets it |
|---|---|
| **Determinism** (the same controllers give the same frames on every computer) | Its instructions have one defined result everywhere; the engine gives no clock, no threads and its own random generator, so a script cannot tell two computers apart |
| **Save states** (rooms, save slots, recordings) | The module's whole memory is one byte array plus a few globals: a save state copies them next to `hd_state`. A Lua or JavaScript heap is full of pointers and needs a serializer of its own |
| **A sandbox** (a game from a friend runs on the host's computer) | A module can only call the functions the engine gives it: no files, no network, no system calls |
| **Limits** | Memory is capped per game; each frame has a budget counted in instructions (fuel), not in time, so the same game stops at the same point everywhere |
| **The same game in the browser and in the room** | The browser runs WebAssembly natively for Willy Maker's play mode; the device runs the engine's interpreter. The hashes of both must match, as they do today |

Floating point is allowed in scripts (basic operations are exact in WebAssembly); the engine canonicalizes NaNs and gives trigonometry and square roots in its own fixed point, never the system's math library.

### The language

| Item | Level | Notes |
|---|---|---|
| **A typed language close to TypeScript**, compiled to WebAssembly in the browser (AssemblyScript is the first candidate: its compiler runs in the browser, Apache 2.0) | Base | Close to the web's language and to Unity's C# in style; types let the editor complete and check code as you type |
| Blocks that generate that same code | Base | One language underneath; blocks are a view of it |
| Lua as a second language (an interpreter inside the module) | Idea | For people coming from Roblox or PICO-8; decided after the first language is used |

Which runtime and which language are confirmed by phase C1's experiment (frozen spec, measured cases), not by this page.

## The scripting API

The functions the engine gives a script, versioned like the host API (`include/golink_hd.h`): a version grows when functions are added, never changed, and a package says the version it needs.

| Group | Base | Later |
|---|---|---|
| Events | start, every frame, a player joins or leaves, touch and hit between objects, a level starts or ends | timers, custom events between scripts |
| Objects | create, remove, move, speed, sprite and animation, flip, collision box, tags, find by tag | parent and child objects, pools |
| Players | each player's buttons and sticks (8 players), their character, lives, score | per-player views |
| Level | read and change cells, the camera (follow, shake, zoom) | load another level, rooms inside a level |
| Picture | the effects of phase 4A: fades, color presets, bloom, lights, blend modes, outlines, Mode 7 and the road | particles from a script |
| Sound | effects and music, low pass and echo | per-object positional sound |
| Text | dialogs with accents, the texts of the game's languages, numbers on screen | |
| Utilities | the engine's random generator, fixed point math, A* paths, logging to the editor's console | save progress between sessions |

The engine's own rules (the platformer, and the genres of phase 4B) stay in C; a script changes or adds to them through events. A game can be all script (its own genre) or a genre with a few scripts.

## In the engine

| Item | Level | Notes |
|---|---|---|
| Load a module from the package, check its imports (only the engine's), its memory and its API version | Base | A refused module gives a readable error, as a bad package does today |
| The module's memory and globals in the save state; `HD_STATE_VERSION` grows | Base | Script memory capped at 1 MB at first, so a save state stays under its 2 MB budget |
| Fuel per frame; a script that runs out stops the game with a message on screen and in the log | Base | Never a hang of the room |
| A script error (trap) stops the game the same way, with the script's line when the package has a source map | Base | |
| Package format 3: `code/game.wasm`, its sources and the source map; the manifest says the scripting API version | Base | Sources travel with the package so a game can be opened and edited again |
| Hash gates with a scripted game in `tools/runs`, equal on macOS and Linux, x86_64 and arm64 | Base | Like `walk` and `showcase` |
| Measured cost per frame next to the 4 ms budget (`make bench`) | Base | |
| Hot reload keeping the game's state when the memory layout is compatible | Later | |
| An AOT or JIT runtime on the device | Idea | Only if the interpreter's cost is too high; the result must stay the same |

## In Willy Maker

| Item | Level | Notes |
|---|---|---|
| A code editor (CodeMirror 6: light, works on phones and tablets) with completion and errors from the API's type file | Base | |
| Compile in the browser on save and reload the play mode | Base | Needs the play mode in WebAssembly ([go-link-hd.md](../go-link-hd.md#10-in-willy-maker)) |
| A console with the script's log and errors, with a link to the line | Base | |
| Scripts attached to objects and a game script, listed in Layers and Properties | Base | |
| Blocks (Scratch-like) for events, conditions, objects, sound and variables, and "Show as code" | Base | Blocks to code first; code back to blocks only for code the blocks can show |
| Examples and templates: a door with a key, a boss with phases, a race lap counter, a menu | Base | In English, Spanish and Portuguese |
| The AI pack includes the scripting API, so an AI assistant can write scripts for the game | Later | Like `PROMPT.md` does for the project today; it reuses go-link HD's AI kit ([phase 9](../go-link-hd.md#phases)), which also lets an AI make a whole game without Willy Maker |
| A debugger: breakpoints, step frame by frame, watch variables | Later | The debug view's frame step already planned for go-link HD |
| Sharing scripts between games, a library of behaviors | Idea | |

## In go-link Console

Nothing changes for rooms: a scripted game is a `.glhd` package like any other, and the device runs it with the same engine. Validation level 4 (the power-on test on the linked device) covers scripted games, and a script that stops a game shows its message in the room as well.

## Phases

| Phase | What | Rough size | Depends on |
|---|---|---|---|
| C1 | Experiment: the runtime (wasm3, WAMR, Lua for comparison) and the language; measure cost per frame, determinism across platforms and save state size | 1-2 sessions | — |
| C2 | The engine: modules in packages (format 3), the scripting API's first version, fuel, memory, save states, hash gates | 3-4 sessions | C1 |
| C3 | Willy Maker: code editor, compile in the browser, play mode with scripts, console, scripts on objects | 3-4 sessions | C2, the play mode in WebAssembly |
| C4 | Blocks and "Show as code", examples and templates | 3-4 sessions | C3 |
| C5 | Debugger, hot reload, the AI pack with the scripting API | by parts | C4 |

The second evolution starts after phase 4 of [go-link-hd.md](../go-link-hd.md#phases) (genres and editors) and the play mode in WebAssembly, since code builds on both.

## Open questions

1. AssemblyScript or another language that compiles to WebAssembly in the browser; whether Lua is worth a second language.
2. How much fuel a frame gets, and whether a game may ask for more (with the engine's 4 ms budget in mind).
3. Whether code edited by hand can always go back to blocks, or only the part blocks can show.
4. How a script's errors are shown to players in a room (a message, or the game pauses for the host).
