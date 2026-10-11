# Case A: spec checklist

Acceptance item 6 of [experiment 1](../README.md#acceptance): every item of [Game Spec v1](../README.md#game-spec-v1), met or a gap, with its evidence. "Runs" are the scripts in [runs/](runs/), replayed on the simulator and on the real core by `tools/replay-all.mjs`; their pictures (simulator | core | difference) and summaries are in [evidence/runs/](evidence/runs/). Frame numbers are the harness's (frame N = after N frames from power on).

**Result: every item met, no gaps.** Three items carry a note (a choice the spec leaves open or a number read a particular way): the jump's integration order (D-004), the continue screen without a timer (D-008), and "in a go-link room" (shown on the same core the room uses, not in a room).

## Board and set

| Item | Status | Evidence |
|---|---|---|
| CPS-1, laid out as `slammast` (4 ports × 3 buttons) | met | `rom/tools/build.mjs slammast`: 28 files, names and sizes of the set; level 3 step `files` |
| Runs on the stock mame2003-plus core | met | level 4 (`evidence/acceptance/level4.json`); 5 scripts replayed on the core, 41 checkpoints identical (`evidence/runs/replay-all.json`) |
| Runs in a go-link room | met, note | the room runs the same core (`device romtest` loads the device's own `~/go-link-console/cores` core); not played in a room in this case: `rom/tools/room-test.mjs` starts its own signalhub and device, which this case did not run on the user's machine |

## Screen and size

| Item | Status | Evidence |
|---|---|---|
| 384 × 224 at 60 Hz | met | `core-replay.json`: `width 384, height 224, fps 60` |
| Level 1536 × 448, 4 × 2 screens, 16 px grid | met | `evidence/level-map.txt` (96 × 28 cells); lab state `level_w 1536, level_h 448`; D-003 streams it into the 1024 px tilemap |

## Players

| Item | Status | Evidence |
|---|---|---|
| 2 at once: P1 Willy, P2 a recruit with another shirt | met | clear run: P2 joins at frame 1227 (`events.txt`); `pair-001245.png`, `pair-001376.png` (green shirt) |
| P3 and P4 "coming soon"; their ports do not crash | met | odd-inputs run: 3P/4P Start show "3P/4P COMING SOON" (`pair-000200.png`, `pair-000900.png`), 3P/4P Coin add credits, B3/b4-b6 on all ports; the game keeps running, expectations pass |

## Controls

| Item | Status | Evidence |
|---|---|---|
| Left/right walk | met | clear run, frames 160-364 |
| Double tap to run (250 ms) | met | `RUN_TAP 15` frames; moves run: `run` at frame 700; clear run runs at 2 px per frame |
| B1 jump (64 px high) | met, note | measured peak 69 px (`evidence/rules-check.json`); D-004: the prototype's order peaked at 61.9 px and could not reach the spec's 64 px ledge |
| Down + B1 drops through a one-way ledge | met | clear run frame ~470 (the ledge); moves run frame 1140 (the upper deck) |
| Up/down on ladders | met | moves run: up (frames 790-900), down (910-1020), up again |
| B2 fire, the knife when an enemy is adjacent | met | moves run: one B2 press at 30 px is the knife (frame 632, `pair-000640.png`); clear run shoots all three Troopers |
| B3 special: does nothing, must not crash | met | odd-inputs run frames 240-500 and 2000-2300 (B3 alone and with every direction): nothing happens; D-006 |
| Start joins | met | 1P Start (frame 154), 2P Start (clear run 1227, odd-inputs 1602); a continue with 1P Start (game-over run 902) |
| Coin adds a credit | met | every run's `events.txt` (`credits 0 -> 1`); up to 9 |

## Rules (the prototype's engine numbers)

| Item | Status | Evidence |
|---|---|---|
| Gravity 6/16 px per frame² | met | `GRAVITY 6` (1/16 px) in `main.c` |
| Jump −7 px/frame | met | `JUMP_VY (-7 * 16)`; D-004 |
| A 32 px crate is climbed by pushing | met | clear run frame 300 (y 368) and ~350 (y 336); `PUSH_FRAMES 10` |
| Climbing at 1.5 px per frame | met | measured 1.498 px per frame over 420 frames (`rules-check.json`) |
| The camera only moves forward, 48 px back margin | met | measured: at most 48 px back from its farthest x (`rules-check.json`) |

## The level, left to right

| Item | Status | Evidence |
|---|---|---|
| 1. Start: both players on the dock floor at x 32-96 | met | P1 at x 64, P2 joins 24 px behind (x 40) at the start; `level-map.txt` (`2.1`) |
| 2. Crates 32 then 64 px, climbed by pushing, then jump | met | cells 12-13 (32 px) and 14-15 (64 px); clear run frames 300-410 |
| 3. One-way ledge 64 px up, reached by jumping, a way down | met | y 336, x 272-399; trial t2 (journal 18:14): reached from the floor and from the stack; clear run drops from it |
| 4. Ladder to the upper dock, 160 px up | met | x 640-655, y 400 → 240; clear run frame 800 (y 240) |
| 5. 3 Troopers: lower dock, upper dock, guarding the exit | met | patrols x 496-592 (y 400), 848-944 (y 240), 1344-1440 (y 400) |
| ...walk a 96 px patrol, turn at its ends | met | measured ranges exactly 96 px (`rules-check.json`); D-005 |
| ...take 3 shots | met | `events.txt` of the clear run: hp 3 → 2 → 1 → 0 for each; D-007 (a knife hit is one of the 3) |
| ...hurt on touch: blink 1 s, lose 1 of 3 energy | met | game-over run: energy 3 → 2 → 1 → out; 60 frames of `hurt` per hit (`rules-check.json`); moves run `pair-001560.png`; D-008 |
| ...give 100 points | met | clear run score 1300 = 3 × 100 + 2 × 500 |
| 6. 2 civilians, rescued by touch, 500 points each | met | clear run frames 448 and 1118 |
| 7. Exit at x 1440-1504 on the lower dock; with all enemies down: "SECTION CLEAR" | met | clear run frame 1376 (`pair-001376.png`); moves run: in the exit with Troopers up it does not clear and shows DEFEAT EVERY TROOPER (`pair-001620.png`) |

## Screens

| Item | Status | Evidence |
|---|---|---|
| Title: `WILLY GORKLINGO`, `THE LAG PROTOCOL`, `PUSH START`, `CREDITS n`, `(C) 2026 GO-LINK` | met | `evidence/runs/odd-inputs/pair-000200.png` (all five lines; `PUSH START` blinks with a credit, `INSERT COIN` without); D-009 |
| HUD: score per player, energy, credits | met | every playing checkpoint; D-010 |
| "SECTION CLEAR" | met | clear run `pair-001376.png` |
| "GAME OVER" when every player has no energy and no credit | met, note | game-over run frame 1218 (`pair-001218.png`, `pair-001300.png`); with a credit left the game waits for a continue with no timer (D-008) |

## Art and sound

| Item | Status | Evidence |
|---|---|---|
| The prototype's characters and street tiles | met | `rom/tools/art.mjs` unchanged characters; `level.mjs` uses the prototype's street, platform, ladder, crate and prop tiles; the exit gate is the only new art (D-011) |
| Night palette, dark sky | met | the prototype's backdrop on scroll3 (every picture) |
| Sound not required | met | the prototype's silent QSound Z80 stub |

## Acceptance 1-5

| Test | Result |
|---|---|
| 1. Level 3 | passes (`evidence/acceptance/level3.json`) |
| 2. Level 4 | passes (`evidence/acceptance/level4.json`) |
| 3. Scripted run | clears at frame 1376, 20 of 20 expectations (`runs/clear.json`) |
| 4. Same picture on the core | 10 of 10 checkpoints identical, plus 31 more in the other four scripts |
| 5. Automatic players | see `metrics.json` and `evidence/acceptance/acceptance.json` |
