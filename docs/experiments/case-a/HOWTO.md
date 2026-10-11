# Case A: from a clean clone to the finished ROM

A copy-and-paste guide. It starts from a clean clone of go-link at **`23806b8`** (the experiment's base, on `main`), applies case A's changes as two exact patches, builds `rom/build/slammast.zip`, proves it is the same ROM (the SHA-256 of all 28 files), runs every test of the experiment, and ends in a go-link room.

Every step was run again, literally, on 2026-10-01 in a fresh clone (Intel Mac, macOS 15.7.3, x86_64): the rebuilt set's 28 files matched the case's final zip byte for byte. The times below are from that run, except the two marked *estimated* (the tools were already installed and the room step was not run).

| Part | Time (reproduction) | Time in the case |
|---|---|---|
| 1. Tools (first install) | *estimated* 5-15 min, mostly Homebrew's gcc; the device build about 10 min; Laya's venv and model about 10 min more | already installed |
| 2. Clone, apply the patches | 20 s | (the case wrote them: 18:06-18:32) |
| 3. `npm ci` in `frontend/` | 2 s warm cache, about 1 min cold | 0 (a symlink, see "When it goes wrong") |
| 4. Build and compare the 28 files | 3 s | 6 builds, no failure |
| 5. Level 3 | 0.2 s | |
| 6. Scripted clear: simulator, core, compare | 11 s + 3 s + under 1 s | 4 trial runs |
| 7. Every script on both machines | 4 min 24 s | about 4 min |
| 8. Running the acceptance | 12 min 52 s (most of it the 3 Laya games) | about 14 min |
| 9. In a go-link room | *estimated* 2-5 min by hand | not run in the case |
| The whole case, by hand | | 42 min (18:06-18:48, `metrics.json`) |

## 1. Tools

The versions the case and this guide used. Homebrew installs its current version (it cannot pin an old one); a different gcc may give different bytes for the 68000 program (see "When it goes wrong").

| Tool | Version used | Install |
|---|---|---|
| macOS | 15.7.3, Intel (x86_64) | |
| Homebrew | 7.0.7 | https://brew.sh |
| m68k-elf-gcc | 16.2.0 | `brew install m68k-elf-gcc` |
| m68k-elf-binutils | 2.47 | `brew install m68k-elf-binutils` |
| z80asm | 1.8 | `brew install z80asm` |
| nvm | 0.39.7 | https://github.com/nvm-sh/nvm |
| Node | 22.23.2 (npm 10.9.8) | `nvm install 22.23.2` |
| ffmpeg | 8.1.2 | `brew install ffmpeg` |
| git | 2.50.1 | Xcode command line tools |
| Go (only to build the device) | 1.25.3 | https://go.dev/dl |
| The device CLI with `romtest --input` | v0.1.7-9-g3a0b738 or newer | built from the repo, step 1c |
| mame2003-plus core | 3141930, `mame2003_plus_libretro.dylib` SHA-256 `361821b1c8df0cd4f37621e52770bb78a7822e7f64ef2d7260e2a774bc3ee61b` | `$DEVICE core download` |
| Laya player (only for acceptance's Laya games) | Python 3.9.6, torch 2.2.2, numpy 1.26.4, transformers 4.57.6, laya 0.3.4, huggingface_hub 0.36.2 | step 1d |

### 1a. The ROM toolchain and Node (as `rom/README.md` and `docs/experiments/harness.md` install them)

```sh
brew install m68k-elf-binutils m68k-elf-gcc z80asm ffmpeg
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.7/install.sh | bash
source ~/.nvm/nvm.sh
nvm install 22.23.2
nvm use 22.23.2
```

Check:

```sh
m68k-elf-gcc --version | head -1      # m68k-elf-gcc (GCC) 16.2.0
m68k-elf-ld --version | head -1       # GNU ld (GNU Binutils) 2.47...
z80asm --version 2>&1 | head -1       # Z80 assembler version 1.8
node --version                        # v22.23.2
ffmpeg -version | head -1             # ffmpeg version 8.1.2 ...
```

### 1b. The clone

```sh
git clone https://github.com/lordbasex/go-link-console.git go-link-case-a
cd go-link-case-a
git checkout 23806b8
git log --oneline -1                  # 23806b8 rom/tools/lab: the route bot, the Laya player, ...
ls rom/tools/lab                      # acceptance.mjs bot.mjs compare.mjs laya_player.py run.mjs runs validate.mjs ...
```

All the commands below run from this folder (`go-link-case-a`).

### 1c. The device CLI and its core (for the real-core tests and the room)

The real-core steps need a device with `romtest --input` (added in `3a0b738`, so `23806b8` has it). Build it once from the clone (Go and Xcode; the first build compiles libvpx and Opus from source, about 10 min, nasm needed for x86_64: `brew install nasm`):

```sh
make device-darwin-universal
DEVICE=$PWD/dist/device/darwin-universal/go-link.app/Contents/MacOS/go-link-device
$DEVICE core download                 # once: ~/go-link/cores/mame2003_plus_libretro.dylib and its game list
```

Build it **after** step 2 to get a device that knows case A's set (its `pkg/ownsets/sets.json` is part of the patch): `romtest` then says `go-link set ... verified by the SHA-256 of its 28 files`. A device built before the patch (or the released app) still runs the set; it only names it after the original game. The case used a device built at `3a0b738` (`go-link device v0.1.7-9-g3a0b738-dirty`).

### 1d. Laya (only for the 3 Laya games of the acceptance)

As `docs/experiments/harness.md` ("Setup") installs it; on an Intel Mac use the system Python 3.9 (Python 3.14 finds no `torch` wheel):

```sh
/usr/bin/python3 -m venv ~/go-link-lab/venv
~/go-link-lab/venv/bin/pip install "torch==2.2.2" "numpy<2"
~/go-link-lab/venv/bin/pip install "transformers==4.57.6" "laya==0.3.4" "huggingface_hub==0.36.2"
~/go-link-lab/venv/bin/python -c "from huggingface_hub import snapshot_download; \
  snapshot_download('convaiinnovations/laya-multilingual', local_dir='$HOME/go-link-lab/models/laya-multilingual')"
```

The model is 647 MB. Without Laya, run the acceptance with `--laya-games 0` (or `--skip laya`).

## 2. Apply case A's changes

Case A changed four files of the repository and added its scripts and tools. Both are exact `git diff --full-index` patches against `23806b8`, kept next to this guide:

| Patch | SHA-256 | What it changes |
|---|---|---|
| [howto/1-rom.patch](howto/1-rom.patch) | `35d4825344923e682ac8443f756f3fbf205b683213e3315744b9ea1238a5984b` | `rom/src/main.c` (the Game Spec v1 engine), `rom/tools/level.mjs` (the docks level), `rom/tools/art.mjs` (start and exit defines), `backend-device/pkg/ownsets/sets.json` (the 28 file hashes) |
| [howto/2-scripts.patch](howto/2-scripts.patch) | `70faf849040fbfc0ca609171a386b5365098ee0dfb848e8d58706f967acdaba0` | `docs/experiments/case-a/runs/` (5 input scripts, 4 plans) and `docs/experiments/case-a/tools/` (7 tools) |

Put the two files in a folder outside the clone (`HOWTO_DIR`), from this branch or from wherever you got this guide:

```sh
HOWTO_DIR=/tmp/case-a-howto
mkdir -p $HOWTO_DIR
# either copy docs/experiments/case-a/howto/*.patch there, or take them from the branch:
git fetch <repo with exp1/case-a> exp1/case-a      # only if the branch is not in this clone
git show FETCH_HEAD:docs/experiments/case-a/howto/1-rom.patch > $HOWTO_DIR/1-rom.patch
git show FETCH_HEAD:docs/experiments/case-a/howto/2-scripts.patch > $HOWTO_DIR/2-scripts.patch
shasum -a 256 $HOWTO_DIR/*.patch                   # the two SHA-256 of the table
```

Apply them:

```sh
git apply --check $HOWTO_DIR/1-rom.patch $HOWTO_DIR/2-scripts.patch && \
git apply $HOWTO_DIR/1-rom.patch $HOWTO_DIR/2-scripts.patch
git status --short
```

Expected:

```
 M backend-device/pkg/ownsets/sets.json
 M rom/src/main.c
 M rom/tools/art.mjs
 M rom/tools/level.mjs
?? docs/experiments/case-a/
```

`git diff --stat` says `4 files changed, 442 insertions(+), 343 deletions(-)` for the tracked files. On the branch these changes are commits `52480aa` (level), `209bc95` (engine), `2833d04` (set list) and `bc71dba` (D-013, opposite directions cancel); `git log -p 23806b8..exp1/case-a -- rom backend-device` shows them one by one with their `Decision:` lines.

## 3. Node packages

```sh
source ~/.nvm/nvm.sh && nvm use 22.23.2
(cd frontend && npm ci)
```

Ends with `found 0 vulnerabilities`. The build itself does not need it; the lab tools do (they import `@go-link/cps1` and `@go-link/cps1-sim`).

## 4. Build, and check it is the same ROM

```sh
node rom/tools/build.mjs
```

The last lines:

```
art: sprites up to tile 0x1122, backdrop 268 tiles, level 24 tiles
lab_state at 0xff0000 (200 bytes)
68000 program: 28694 bytes
built .../go-link-case-a/rom/build/slammast.zip (28 files)
own set list: slammast (28 files), unchanged -> backend-device/pkg/ownsets/sets.json
```

**`unchanged` is the first proof**: the build hashed the 28 files and found exactly the hashes the patch put in `sets.json`. `updated` means a different ROM (and `git diff backend-device/pkg/ownsets/sets.json` shows which files differ). The case's own build log is [evidence/build.log](evidence/build.log) (it says `updated`, because there the build wrote the list for the first time).

The zip's own SHA-256 changes on every build (its entries carry the build's file times), so compare the files inside it:

```sh
rm -rf /tmp/case-a-zip && mkdir /tmp/case-a-zip && unzip -q rom/build/slammast.zip -d /tmp/case-a-zip
(cd /tmp/case-a-zip && shasum -a 256 * )
```

It must print exactly:

```
da499a8b6a8831c12c611c99cf8f73490220604108707fb804c39ea4a71a75b1  mb_05.bin
6dae84a71a707ce6240b70010645e4932576abf4c5be6db2c18066207df98f9f  mb_06.bin
023b96de5e001bd313dec95470b18fca1eecd579479a72dfc162810c403f0b63  mb_07.bin
b82f2b44c55723a1c4981c296c45fbd34d305888040d1c82004af4739ce84f97  mb_08.bin
043e238a765f7cfbc62596a50e53c8ffb6b188a99357b0ebede251725d67589f  mb_10.bin
043e238a765f7cfbc62596a50e53c8ffb6b188a99357b0ebede251725d67589f  mb_11.bin
043e238a765f7cfbc62596a50e53c8ffb6b188a99357b0ebede251725d67589f  mb_12.bin
043e238a765f7cfbc62596a50e53c8ffb6b188a99357b0ebede251725d67589f  mb_13.bin
b4d741df5084795c8c72d0d11ec99976be79be75017f188349f85f6665581340  mb_gfx01.rom
85592e3c45405fbcf70a289465cea570eacc84f7e3ec1267d0678d5ad652e39d  mb_gfx02.rom
3ff72adbb222feff9db82599c80905b056f002325492f9ad7cf77beeac2cb579  mb_gfx03.rom
d1b603ae61904b8702b2a3f69a5c057c836c958b0a376a23c2c14704b918eef6  mb_gfx04.rom
07854d2fef297a06ba81685e660c332de36d5d18d546927d30daad6d7fda1541  mb_q1.bin
07854d2fef297a06ba81685e660c332de36d5d18d546927d30daad6d7fda1541  mb_q2.bin
07854d2fef297a06ba81685e660c332de36d5d18d546927d30daad6d7fda1541  mb_q3.bin
07854d2fef297a06ba81685e660c332de36d5d18d546927d30daad6d7fda1541  mb_q4.bin
07854d2fef297a06ba81685e660c332de36d5d18d546927d30daad6d7fda1541  mb_q5.bin
07854d2fef297a06ba81685e660c332de36d5d18d546927d30daad6d7fda1541  mb_q6.bin
07854d2fef297a06ba81685e660c332de36d5d18d546927d30daad6d7fda1541  mb_q7.bin
07854d2fef297a06ba81685e660c332de36d5d18d546927d30daad6d7fda1541  mb_q8.bin
97aa20f433373110eb6c1a4cad8b2fa80eeaf275578c40326a5fca073abd4cb1  mb_qa.rom
043e238a765f7cfbc62596a50e53c8ffb6b188a99357b0ebede251725d67589f  mbe_20a.rom
043e238a765f7cfbc62596a50e53c8ffb6b188a99357b0ebede251725d67589f  mbe_21a.rom
ce33a1166cebf3fbb978dcaad47f93e3b51b64bd0f61057261b180d3d597775b  mbe_23e.rom
b5a41c3758763bbec72769fab4a2533bf2db0b6312d93d25a695f9e4b9e02260  mbe_24b.rom
b5a41c3758763bbec72769fab4a2533bf2db0b6312d93d25a695f9e4b9e02260  mbe_25b.rom
b5a41c3758763bbec72769fab4a2533bf2db0b6312d93d25a695f9e4b9e02260  mbe_28b.rom
b5a41c3758763bbec72769fab4a2533bf2db0b6312d93d25a695f9e4b9e02260  mbe_29b.rom
```

These are the files of the case's final zip (61496 bytes, 28 files, 12713984 bytes unpacked; that zip's own SHA-256 was `1efb70984aab74a87257df05ff5239141a112d3c970a3e36862d762c9a5d8b22`, yours will differ). Repeated hashes are expected: the 68000 program is `mbe_23e.rom`, and the other program slots the board has (`mbe_20a`, `mbe_21a`, `mbe_24b`, `mbe_25b`, `mbe_28b`, `mbe_29b`) are filler, as are the unused graphics ROMs `mb_10`-`mb_13`; the QSound sample ROMs `mb_q1`-`mb_q8` are the same silence; `mb_qa.rom` is the encrypted Z80 sound stub; `mb_gfx01`-`mb_gfx04` and `mb_05`-`mb_08` hold the graphics (the patch changes `mb_05`-`mb_08`, where the level's tiles are). A quick one-line check:

```sh
(cd /tmp/case-a-zip && shasum -a 256 * | shasum -a 256)
```

## 5. Level 3: the simulator powers it on

```sh
node rom/tools/lab/validate.mjs rom/build/slammast.zip
```

```
Power-on test (level 3) of rom/build/slammast.zip (slammast)
  ok    files     All 28 files of slammast are there, each with its size
  ok    program   The program ROMs join into 28694 bytes of 68000 code and data
  ok    vectors   Reset at 0x000400, stack at 0xfff000, vblank handler at 0x000440
  ok    run       300 frames (5 s of game) with no crash
  ok    vblank    The vblank interrupt ran on 291 of 300 frames
  ok    palette   Palettes written from frame 9, 239 colors set
  ok    layers    Layers on: scroll1, scroll2, scroll3; 12 sprites in the table
  ok    picture   A picture on screen from frame 30
  ok    alive     The game keeps drawing (last change on frame 300)
  ok    inputs    Coin and Start change the game
PASSED: 300 frames in 198 ms
```

## 6. The scripted clear, in the simulator and on the real core

```sh
OUT=/tmp/case-a-out && rm -rf $OUT && mkdir -p $OUT
node rom/tools/lab/run.mjs rom/build/slammast.zip --out $OUT/sim --script docs/experiments/case-a/runs/clear.json --mp4
$DEVICE romtest --input docs/experiments/case-a/runs/clear.json --frames-dir $OUT/core --mp4 $OUT/core.mp4 rom/build/slammast.zip
node rom/tools/lab/compare.mjs $OUT/sim/frames $OUT/core --tolerance 0
```

- `run.mjs` (11 s) prints the summary; `$OUT/sim/summary.json` has `"cleared": true`, `"clearFrame": 1376`, `"expectOk": true`, `"frames": 1500`; `$OUT/sim/run.mp4` is the run.
- `romtest` (3 s) prints `Replay of slammast.zip on MAME 2003-Plus 3141930: 1500 frames (384x224 at 60.00 Hz) in 2.1 s, 10 PNG` and one line per checkpoint, `frame 150 0df9f9e9 ...` to `frame 1480 11deb2df ...` (the second column is a short hash of the picture: 150 `0df9f9e9`, 300 `4ede5c87`, 448 `edcf45c1`, 537 `9db6c3ee`, 800 `2de8536f`, 938 `f2e4759c`, 1118 `94234dbf`, 1245 `73220a06`, 1376 `fe9119f8`, 1480 `11deb2df`).
- `compare.mjs`:

```
frame     differ%   exact%   meanAbs  (tolerance 0)
150             0      100        0
300             0      100        0
448             0      100        0
537             0      100        0
800             0      100        0
938             0      100        0
1118            0      100        0
1245            0      100        0
1376            0      100        0
1480            0      100        0
worst: 0% of the pixels differ
```

What the script plays (journal, 18:18): push onto the crates, jump to the ledge and rescue its civilian (frame 448); drop with down + B1 and shoot the first Trooper (3 shots); run to the ladder, climb to the upper dock (frame 800); shoot the second Trooper (938); rescue the second civilian (1118); player 2 inserts a coin and joins (1227); the guard falls (1245); player 1 walks into the exit: **SECTION CLEAR at frame 1376**, 1300 points, no energy lost.

`runs/clear.json` is generated from its plan; this writes the same file byte for byte:

```sh
node docs/experiments/case-a/tools/gen.mjs /tmp/clear-regen.json docs/experiments/case-a/runs/plans/clear.plan.json
#   /tmp/clear-regen.json: 20 steps, segments end at frame 1421
cmp /tmp/clear-regen.json docs/experiments/case-a/runs/clear.json && echo IDENTICAL
```

## 7. Every script on both machines

```sh
node docs/experiments/case-a/tools/replay-all.mjs rom/build/slammast.zip $OUT/runs $DEVICE
node docs/experiments/case-a/tools/rules-check.mjs $OUT/runs/*/sim
```

`replay-all.mjs` prints one JSON line per script (4 min 24 s in all, the `seconds` vary by machine; the long run is 21600 frames):

```
{"name":"clear","simExit":0,"coreExit":0,"expectOk":true,"failed":[],"error":null,"cleared":true,"clearFrame":1376,"gameOverFrame":null,"checkpoints":10,"worstDifferPct":0,"seconds":16.739}
{"name":"game-over","simExit":0,"coreExit":0,"expectOk":true,"failed":[],"error":null,"cleared":false,"clearFrame":null,"gameOverFrame":1218,"checkpoints":8,"worstDifferPct":0,"seconds":14.396}
{"name":"long-run","simExit":0,"coreExit":0,"expectOk":true,"failed":[],"error":null,"cleared":false,"clearFrame":null,"gameOverFrame":null,"checkpoints":5,"worstDifferPct":0,"seconds":194.394}
{"name":"moves","simExit":0,"coreExit":0,"expectOk":true,"failed":[],"error":null,"cleared":false,"clearFrame":null,"gameOverFrame":null,"checkpoints":9,"worstDifferPct":0,"seconds":17.591}
{"name":"odd-inputs","simExit":0,"coreExit":0,"expectOk":true,"failed":[],"error":null,"cleared":false,"clearFrame":null,"gameOverFrame":null,"checkpoints":9,"worstDifferPct":0,"seconds":20.716}
```

Every line must have `"expectOk":true` and `"worstDifferPct":0` (41 checkpoints in all, identical pixels). `rules-check.mjs` prints the measured rules, as in [evidence/rules-check.json](evidence/rules-check.json):

```
{
 "camBackMax": 48,
 "jumpPeak": 69,
 "climbPxPerFrame": { "frames": 420, "mean": 1.498 },
 "patrol": {
  "0": { "min": 496, "max": 592, "range": 96 },
  "1": { "min": 848, "max": 944, "range": 96 },
  "2": { "min": 1344, "max": 1440, "range": 96 }
 },
 "blink": [
  { "dir": ".../runs/game-over/sim", "f": 608, "hurt": 60 },
  { "dir": ".../runs/game-over/sim", "f": 702, "hurt": 60 },
  { "dir": ".../runs/game-over/sim", "f": 943, "hurt": 60 },
  { "dir": ".../runs/game-over/sim", "f": 1158, "hurt": 60 },
  { "dir": ".../runs/moves/sim", "f": 1554, "hurt": 60 },
  { "dir": ".../runs/moves/sim", "f": 1966, "hurt": 60 }
 ]
}
```

That is: the camera went back at most 48 px, the highest jump rose 69 px, climbing moved about 1.5 px per frame, each Trooper stayed within 96 px, and every hit started 60 frames of blinking.

## 8. Running the acceptance

```sh
node rom/tools/lab/acceptance.mjs rom/build/slammast.zip --script docs/experiments/case-a/runs/clear.json \
    --out $OUT/acceptance --bot-games 5 --laya-games 3 --device $DEVICE
```

It prints each stage as it starts, then the verdict:

```
[acceptance] level 3 (simulator power-on)
[acceptance] level 4 (device romtest, real core)
[acceptance] scripted run (simulator)
[acceptance] scripted run (real core) and comparison
[acceptance] bot game 1/5
[acceptance] bot game 2/5
[acceptance] bot game 3/5
[acceptance] bot game 4/5
[acceptance] bot game 5/5
[acceptance] laya game 1/3
[acceptance] laya game 2/3
[acceptance] laya game 3/3
{
  "level3": true,
  "level4": true,
  "scripted": true,
  "sameAsCore": true,
  "bot": "5/5 cleared",
  "laya": "0/3 cleared"
}
```

The details are in `$OUT/acceptance/acceptance.json` (compare with the case's [evidence/acceptance/acceptance.json](evidence/acceptance/acceptance.json)): level 3, level 4, the scripted run (20 of 20 expectations, clear at 1376) and the same picture on the core (10 checkpoints at 0 %) pass; the route bot clears **5 of 5** (frames 1183, 1265, 1399, 1230, 1286; none takes the ledge, see below); Laya clears **0 of 3** (575 decisions per game, almost all `fire`, it never passes x 82, 0 points). Laya's 0 is the expected zero-shot result, not a fault of the ROM. In this guide's reproduction every bot and Laya game gave the case's numbers exactly (bot clear frames, scores 800, 800, 800, 300, 800, and 575 decisions per Laya game). With the device built at `3a0b738`, `level4.json` says `identity: not a go-link set: it shows as the core's original game` and `core.files: 28 files differ from the original set (the core warns, and runs them)`: both are `ok`; a device built after step 2 names it as go-link's set.

## 9. In a go-link room

The set is a normal `slammast.zip` for the device: it powers on the stock mame2003-plus core and the rooms stream it like any game.

**On your own go-link device** (the app, or `$DEVICE` from step 1c):

1. Check it with the device's own test first:
   ```sh
   $DEVICE romtest rom/build/slammast.zip
   ```
   Every step `ok`; with a device built after step 2 the identity step says `go-link set "..." verified by the SHA-256 of its 28 files`.
2. Put it in the device's ROM folder. Either from the website (My device › ROMs, drop `rom/build/slammast.zip` on the drop zone; it never overwrites a file of the same name, so if a `slammast.zip` is already there, move that one out first), or by copying it into the folder the device uses:
   ```sh
   $DEVICE roms dir                      # prints the ROM folder
   cp rom/build/slammast.zip "<that folder>/"
   $DEVICE roms check                    # slammast  ok  Saturday Night Slam Masters (World 930713)
   ```
   `roms check` names the core's game for the set's layout; a device built after step 2 shows the game as go-link's own in the website.
3. On the website: **My device › Rooms › New game**, pick `slammast`, start it. In the room: `5` inserts a coin, `Enter` is your Start, the arrows move, `Z` jumps (B1), `X` fires (B2), `C` is B3 (does nothing in this section). A double tap on → runs. A second browser invited to the room (Invite, then the code and the one-time PIN) takes P2: coin and 2P Start.
4. Expected: the title (`WILLY GORKLINGO`, `THE LAG PROTOCOL`, `PUSH START`, `CREDITS n`), then the docks; the same pictures as `$OUT/sim/run.mp4`.

**Automatic, with its own throwaway stack:** `node rom/tools/room-test.mjs` (needs the signalhub repo next to go-link or `SIGNALING_DIR`, `npm ci` in `e2e/`, and the core in `~/go-link/cores`) builds a signalhub and a headless device, opens the device's panel in Chromium, starts "New game" with only this set in a throwaway ROM folder and saves pictures of the room's WebRTC video in `rom/build/room/shots/`. It uses ports 8192 and 7392, and its key sequence was written for the prototype (its picture names say "bazooka"; in this ROM C does nothing). Case A did not run it (`checklist.md`): the room plays the same core that steps 6-8 test.

## When it goes wrong

Every item happened in the case (journal and decisions) or in this guide's reproduction.

| What you see | Why | Fix |
|---|---|---|
| `ERR_MODULE_NOT_FOUND: Cannot find package '@go-link/cps1'` from `validate.mjs`, `run.mjs` or the case's tools | `frontend/node_modules` is missing (the case's worktree had none at 18:06; it used a symlink to the main checkout's) | step 3: `(cd frontend && npm ci)`. `build.mjs` works without it, the lab tools do not |
| `rom/tools/lab` does not exist, or `romtest` says `flag provided but not defined: -input` | the clone is older than the harness (the case's worktree first started on `76cd798`) or the device is | `git checkout 23806b8`; build the device from it (step 1c) |
| `git apply` says `patch does not apply` | not at `23806b8`, or the files were edited, or the patch's line ends were changed by a copy | `git status` must be clean at `23806b8`; check the patch's SHA-256 (step 2) |
| The build says `updated` instead of `unchanged`, and the 28 hashes differ in `mbe_23e.rom` | a different m68k-elf-gcc (Homebrew only installs its current version) compiled the program differently | use gcc 16.2.0 / binutils 2.47; the game is still valid if steps 5-8 pass, it is only not byte-identical |
| The hashes differ in `mb_05`-`mb_08` (graphics) | `level.mjs` or `art.mjs` did not get the patch, or Node is not 22.18+ (TypeScript imports) | re-apply `1-rom.patch`; `node --version` |
| `68000 code with a post-increment source...` | the build's guard against a known emulated-68000 trap (`rom/src/main.c`, `copy_words`) | it should not appear with the patch; if you edit `main.c`, keep word copies as `copy_words` writes them |
| `does not fit` | the program grew past its ROM | only after your own edits; the case's program is 28694 bytes |
| The zip's SHA-256 is not `1efb7098...` | the zip carries each build's file times | compare the files inside (step 4), never the zip |
| A jump never reaches the 64 px ledge (peak 61.9 px) | the prototype's order (gravity, then move); the first dead end of the case | D-004: move, then add gravity (peak 68.9 px); it is in the patch, in `update_player` |
| Shots never reach a Trooper | bullets vanish 32 px past the screen's edge; 3 of the clear script's 4 trial runs failed this way | walk closer before firing (the script's plan does) |
| A script's expectations fail on the coin count | the case's game-over and odd-input expectations had wrong coin counts twice: the expectations were wrong, not the game | count every `coin` step on every port; 2P Start with no credit does nothing |
| `compare.mjs` shows 1.65 % and 1.76 % at frames 1100 and 1300 of `odd-inputs` | left + right held together: the simulator moved left, the core moved neither; the ROM before D-013 | D-013 (commit `bc71dba`, in the patch): opposite directions cancel. Rebuild and re-run every script |
| An acceptance run started before a rebuild gives old results | it plays the zip as it was when each stage started | stop it and run it again on the new build (the case did, 18:28) |
| The route bot clears but never takes the ledge, and rescues at most one civilian | the bot's route model has no jump straight up onto a ledge above a floor, nor a level jump across a gap | not a ROM fault (the scripted run takes the ledge); seed 3 skips both civilians (300 points) |
| Laya clears 0 of 3, answering `fire` | an untrained (zero-shot) decision model, the same on the prototype | expected; `--laya-games 0` to skip |
| `pip` finds no `torch` (Laya setup) | an Intel Mac with Python 3.14: PyTorch 2.2.2 has wheels only for 3.8-3.12 | the system Python 3.9 (`/usr/bin/python3`), step 1d |
| `romtest` fails to find the core | `~/go-link/cores` is empty | `$DEVICE core download` |
| `room-test.mjs` fails at the signalhub build | no signalhub repo next to go-link | `git clone https://github.com/lordbasex/signalhub ../signaling` or set `SIGNALING_DIR` |
| `2UP INSERT COIN` touches `ENEMY 3` in the HUD | the first layout (D-010) | fixed in the patch (2UP at x 20) |
