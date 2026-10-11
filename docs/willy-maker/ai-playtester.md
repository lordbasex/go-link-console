# Willy Maker: the AI playtester (specification)

**Test with AI** lets an AI play a game made in Willy Maker before any person does. It reports how often each level is cleared, where players die, which jumps are almost impossible and where they get stuck, with short videos of each finding. The idea comes from Willy Maker's [vision](vision.md#the-ai-playtester). This page lists, before anything is built, **what the playtester supports and what could be done with it**. It is the base for the roadmap ([status.md](../status.md)); the pieces it builds on exist, but the playtester does not.

Every item is marked:

- **Base:** the first version must have it.
- **Later:** planned, after the base.
- **Idea:** possible and worth keeping in mind; not committed.

## Decisions taken (2026-10-06)

| Question | Decision | Why |
|---|---|---|
| What plays | A **small policy network** trained on each game, and the **route bot** as its teacher and baseline | Experiment 1 showed that a general model is not a player: Laya, asked "next action?" without training, answered "fire" in every state. A small network decides in under a millisecond |
| What it sees | The game's **exact state** (`lab_state`) and the collision map, not the screen | Every Willy Maker ROM keeps its state in RAM each frame ([harness](../experiments/harness.md#the-lab-state)); reading it is free and exact. Reading pixels comes later |
| Where it runs | **In the browser**: the board simulator in Web Workers, the network on WebGPU (WebAssembly on the CPU where WebGPU is missing) | Nothing to install, nothing leaves the user's computer, and the simulator already runs there (validation level 3) |
| First genre | **Platformer**, then the platform shooter | Clear goals (reach the exit), and jumps are what a playtester should judge first. The same order as [go-link HD](../go-link-hd.md) |
| Boards | CPS-1 first; **go-link HD** with the same interface when its engine exists | go-link HD keeps input logs and an exact state by design, so the playtester needs no changes to its method |

## How it works

```
Willy Maker project ──Create ROM──► ROM set (or a .glhd package later)
                                          │
                  ┌───────────────────────┴────────────────────────┐
                  ▼                                                ▼
     Web Workers: N copies of the board simulator      route bot: plays from lab_state
     (each runs a game faster than real time)          and the collision map (teacher)
                  │                                                │
                  └──── states, actions, rewards ──────┬───────────┘
                                                       ▼
                                 policy network (WebGPU): imitation, then reinforcement
                                                       ▼
                         evaluation runs ──► report: clears, deaths, stuck spots, videos
```

1. **Imitation:** the network learns to copy the route bot from the bot's own runs.
2. **Reinforcement:** it then plays by itself in many simulators at once and improves with rewards.
3. **Evaluation:** with learning off, it plays every level many times; the report counts what happened.

Every run is a start state plus one input per frame, so any finding replays exactly in play mode, in the simulator and on the real core.

## 1. Players

| Feature | Level | Notes |
|---|---|---|
| **Route bot** (exists: `rom/tools/lab/bot.mjs`) as the baseline and teacher | Base | Moved from the command line to the browser |
| **Trained player**: a small network (thousands to a million parameters) per game | Base | Under 1 ms per decision |
| Several **skill levels**: a beginner (slower reactions, imperfect inputs), an average player and an expert | Base | The same network with a reaction delay and input noise; the report is given per level |
| **Explorer**: a player rewarded for reaching new places instead of the exit | Base | Finds holes in walls, places to get stuck, and shortcuts the designer did not plan |
| **Adversarial players** (exist in the QA run, `qa.mjs`): run into walls, mash buttons, a seeded 4-port fuzz | Base | They catch crashes and broken rules, not difficulty |
| 2 to 4 AI players at once | Later | Joins, shared camera, players pushing each other off screen |
| A player that reads the **screen** instead of the state | Later | Needed for games where the state does not tell enough, and for boards with no `lab_state` |
| Players that imitate **real people's games** (input logs recorded in rooms, with consent) | Idea | See section 7 |

## 2. Training

| Feature | Level | Notes |
|---|---|---|
| Imitation of the route bot | Base | Quick start: a few minutes |
| Reinforcement learning (PPO or similar) | Base | Rewards below |
| Rewards: progress to the right or up, rescues, pickups, a clear; penalties: a death, standing still, going back for long | Base | Per genre, from the genre's rules; the designer may tune their weights |
| **Many simulators at once**: one per Web Worker, one network step for all of them | Base | The simulator runs a level faster than real time (level 3 runs 10 s of game in about 1 s) |
| Training per level, from the level's start or from a checkpoint | Base | A long level trains in parts |
| Training continues from the last saved network when the game changes a little | Base | A small edit does not mean training from zero |
| A time or quality limit, chosen by the user (Quick, Normal, Thorough) | Base | |
| Training on the linked go-link Console device instead of the browser | Later | For long trainings or slow computers; the device runs the real core |
| Training across games of the same genre (a general platformer player) | Idea | Faster start on a new game |

## 3. The report

| Feature | Level | Notes |
|---|---|---|
| **Clear rate** per level and per skill level | Base | "The average player clears level 2 in 34 % of the tries" |
| **Time to clear** (median and spread) | Base | |
| **Death map**: where players die, drawn over the level in the editor | Base | Each spot opens the moment in play mode |
| **Stuck spots**: where a player stays a long time without progress | Base | Holes, walls with no way up, ladders that cannot be reached |
| **Hard jumps**: jumps that need frame-perfect inputs (the window of frames that works) | Base | "This jump works only if you press within 2 frames" |
| **Unreachable** parts: pickups, rescues or exits no player reached | Base | |
| **Short videos** of each finding (the frames before and after) | Base | Recorded from the replay, not during training |
| A **difficulty curve** across levels | Base | Levels sorted by how hard they turned out, against the order the designer gave them |
| Suggestions in words ("lower this ledge 8 px", "add a checkpoint before this boss") | Later | From fixed rules first; an AI text model could phrase them |
| **Compare two versions** of the game (before and after an edit) | Later | Did the edit make level 3 easier? |
| Export the report (a page or a file to share) | Later | |

## 4. In Willy Maker

| Feature | Level | Notes |
|---|---|---|
| A **Test with AI** button, next to Create ROM | Base | |
| Progress while training: plays so far, clear rate rising, a live picture of one simulator | Base | |
| Findings on the level in the editor: death and stuck marks, hard-jump arcs | Base | |
| Click a finding to replay it in play mode at that moment | Base | |
| Stop, resume, and train again after an edit | Base | |
| Choose the levels and skill levels to test | Base | |
| A **Rules** check: the genre's own invariants on every frame (from the QA run) | Base | A crash or a broken rule is a finding too |
| Run Test with AI automatically when the game is saved | Idea | |

## 5. Genres

| Genre | Level | What changes |
|---|---|---|
| Platformer | Base | Rewards for progress, coins, the exit; hard jumps |
| Platform shooter | Base | Plus enemies, weapons, rescues |
| Beat 'em up | Later | Waves, health, combos |
| Shooters (horizontal, vertical, top-down) | Later | Dodging, power-ups, bosses |
| Maze | Later | Routes and chasers |
| Versus fighting | Later | Two AIs against each other; balance between characters |
| Puzzle, quiz, sports, racing, light gun | Later | Each needs its own rewards |

## 6. Limits and budgets (proposed, to be measured)

| Item | Budget |
|---|---|
| Decision time | ≤ 1 ms per frame per game |
| Simulators at once | One per CPU core but one (8 on an 8-core computer) |
| Quick test | ≤ 5 minutes per level |
| Normal test | ≤ 20 minutes per level |
| Network size | ≤ 1 million parameters (≤ 4 MB) |
| Memory | ≤ 1 GB for the whole test |
| Evaluation | 100 plays per level and skill level |

## 7. Data and privacy

| Feature | Level | Notes |
|---|---|---|
| Everything runs on the user's computer; nothing is sent anywhere | Base | Networks and reports are kept with the project |
| Networks saved in the project `.zip` (optional, they can be retrained) | Base | |
| **Real games as training data**: a room records only each frame's input packets of every seat, which are tiny; the simulator rebuilds every frame offline | Idea | Only with every player's consent, off by default, explained in the [privacy policy](../legal.md) before it exists |
| Sharing trained players between users | Idea | |

## 8. QA bots in real rooms

Separate from the playtester but built from the same players.

| Feature | Level | Notes |
|---|---|---|
| Bots that join a room as ordinary guests (invitation, PIN, WebRTC, the `input` channel) and play for hours | Later | Stress-test the device, WebRTC and the core together: memory over time, frame pacing, input latency, reconnections, seat changes, four players |
| A soak test before each release | Idea | |

## 9. Not in scope

- **Playing for the user** or helping in real games: the playtester only tests games being made.
- **Commercial MAME games:** it needs the `lab_state` of Willy Maker's own ROMs (or go-link HD's state).
- **Large general models** in the loop: a decision every frame needs a small network.

## Phases (when it is built)

| Phase | What | Rough size |
|---|---|---|
| 0 | **Experiment** on one platformer ROM: route bot, Laya asked small yes/no questions, and a trained network, judged on the same levels | 1-2 sessions |
| 1 | The simulator in Web Workers driven from the browser; the route bot in the browser; the report from the route bot alone (clear, deaths, stuck spots) | 2-3 sessions |
| 2 | The trained network: imitation, then reinforcement on WebGPU; skill levels; the explorer | 3-4 sessions |
| 3 | Test with AI in Willy Maker: findings on the level, replays, videos, difficulty curve | 2-3 sessions |
| 4 | More genres, version comparison, suggestions, training on the device | by parts |
| 5 | QA bots in real rooms | 2 sessions |

## Open questions

1. Whether reinforcement learning on WebGPU trains fast enough in a browser tab, or whether the device should train by default (to be measured in phase 0).
2. How to judge "almost impossible" fairly: a frame window, or how often the average player fails there.
3. Whether the playtester should also judge **fun** (pacing, quiet stretches), not only difficulty.
4. How it works with go-link HD games, which have no 68000 `lab_state` but a state of their own.
