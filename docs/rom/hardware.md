# CPS-1 hardware facts for the prototype

What go-link Console's own ROM relies on, read from the driver of the core the device runs (mame2003-plus, MAME 0.78) and confirmed by the prototype where marked **(tested)**. Source: [libretro/mame2003-plus-libretro](https://github.com/libretro/mame2003-plus-libretro) at commit `0d9a325c`; links point to that commit.

- `cps1.c` = [src/drivers/cps1.c](https://github.com/libretro/mame2003-plus-libretro/blob/0d9a325c96aa96460be7b2fe4310a7096981ff75/src/drivers/cps1.c)
- `cps1_vidhrdw.c` = [src/vidhrdw/cps1_vidhrdw.c](https://github.com/libretro/mame2003-plus-libretro/blob/0d9a325c96aa96460be7b2fe4310a7096981ff75/src/vidhrdw/cps1_vidhrdw.c)
- `kabuki.c` = [src/machine/kabuki.c](https://github.com/libretro/mame2003-plus-libretro/blob/0d9a325c96aa96460be7b2fe4310a7096981ff75/src/machine/kabuki.c) (the Z80 encryption of the QSound boards)
- `common.c` = [src/common.c](https://github.com/libretro/mame2003-plus-libretro/blob/0d9a325c96aa96460be7b2fe4310a7096981ff75/src/common.c) (the ROM loader)

## Which CPS-1

Capcom made the CPS-1 (CP System) as several boards between 1988 and 1994: the first A/B boards (1988), later B boards with other CPS-B chips (1989, 1990), the QSound board in its grey case, also called CPS Dash (1992), the last B-21 boards (1992), and the CPS Changer, a home console (1994). They share the 68000, the video chips (384 × 224 at 60 Hz, 4096 colors on screen out of 65536, 16 × 16 sprites, three scroll layers) and the graphics format; they differ in the CPS-B chip, the sound (YM2151 and OKI 6295 on the plain boards, QSound on the QSound board) and the 68000's clock.

**Willy Maker builds for the 1992 QSound board, as the `slammast` set** (Saturday Night Slam Masters, which with its sibling `mbombrd` is the only one with 4 players and 3 buttons). Create ROM always lays the game out as `slammast`; a game with 2 buttons only changes its controls. What that board has, as the core runs it:

| | Real board | The core (mame2003-plus) | Willy Maker |
|---|---|---|---|
| Main CPU | 68000 at 12 MHz | **68000 at 10 MHz** (`cps1.c` machine `qsound` imports `cps1`'s `M68000, 10000000`) | the engine's frame budget is measured at 10 MHz |
| Sound CPU | Z80 | **Z80 at 6 MHz** (`Z80, 6000000`), 250 Hz IRQ | `rom/engine/sound.z80` |
| Sound chip | QSound | QSound, 16 channels, stereo, 4 MB of samples | the effects and music of [A game's own sound](../willy-maker/engine.md#a-games-own-sound) |
| Video | 384 × 224, 60 Hz | same | same |
| Colors | 4096 on screen of 65536 | same (12-bit words) | every color snapped to the board's (`board/cps1.ts` `snapColor`) |
| Sprites | 256 per screen, 16 × 16 tiles | same | up to 248 entries a frame |
| Palettes | 32 for sprites, 32 per scroll layer, 15 colors + transparent | same | `board/cps1.ts` `palettes` |
| ROM | 6 MB graphics, 2 MB program, 4 MB samples | same | the meters in the editor |
| Work RAM | 64 KB | same | `lab_state` at 0xff0000 |

**The clock.** In this core only Pang! 3 gets a 12 MHz 68000 (`MDRV_CPU_REPLACE("main", M68000, 12000000)`); every other CPS-1 set, `slammast` included, runs at 10 MHz. A ROM made for 10 MHz has, on a real 12 MHz board, a fifth more time each frame than it needs, so it runs the same. Going to 12 MHz would mean another set (Pang! 3: 2 players, 2 buttons) or a newer MAME on the device; the engine does not need it today (every lab run fits its frames at 10 MHz).

The New game wizard shows this as the board's spec sheet (`ui/studio/NewGameWizard.tsx` `BoardSpec`, from `board/cps1.ts` `hardware`), so another board's profile brings its own.

## The sets we lay our files out as

The prototype builds two layouts (`node rom/tools/build.mjs [slammast|captcomm]`):
- **`slammast`**, the chosen one: 4 players × 3 buttons, on the QSound board ([journal, step 0b](journal.md#step-0b--rethinking-the-set-buttons));
- **`captcomm`**: 4 × 2, used in steps 1 and 2 and still built.

The 68000 side, video, graphics format and palette are the same on both. The CPS-B registers, the inputs of players 3-4 and the sound side differ.

## `slammast` (QSound board) (tested)

Config `QSOUND_4` (`cps1_vidhrdw.c` L261 and L373), machine `qsound`, init `slammast` (`cps1.c` L4060-4080, L8080-8084).

ROM layout (`cps1.c` L7542-7578):

| Region | File | Size | How it loads |
|---|---|---|---|
| CPU1 (68000) | `mbe_23e.rom` | 0x80000 | `ROM_LOAD16_WORD_SWAP` at 0x000000 (our program lives here) |
| | `mbe_24b.rom` / `mbe_28b.rom` | 0x20000 each | `ROM_LOAD16_BYTE` at 0x080000 / 0x080001 |
| | `mbe_25b.rom` / `mbe_29b.rom` | 0x20000 each | `ROM_LOAD16_BYTE` at 0x0c0000 / 0x0c0001 |
| | `mbe_21a.rom`, `mbe_20a.rom` | 0x80000 each | `ROM_LOAD16_WORD_SWAP` at 0x100000, 0x180000 |
| GFX1 (0x600000) | `mb_gfx01.rom`, `mb_gfx03.rom`, `mb_gfx02.rom`, `mb_gfx04.rom` | 0x80000 each | `ROM_GROUPWORD \| ROM_SKIP(6)` at 0x000000-0x000006 |
| | `mb_05.bin`, `mb_07.bin`, `mb_06.bin`, `mb_08.bin` | 0x80000 each | same, at 0x200000 |
| | `mb_10.bin`, `mb_12.bin`, `mb_11.bin`, `mb_13.bin` | 0x80000 each | same, at 0x400000 |
| CPU2 (QSound Z80) | `mb_qa.rom` | 0x20000 | first 0x8000 at 0x0000 (**Kabuki-encrypted**), `ROM_CONTINUE` 0x18000 at 0x10000 (banked, plain) |
| SOUND1 (QSound samples, 0x400000) | `mb_q1.bin` … `mb_q8.bin` | 0x80000 each | at 0x000000 … 0x380000 |

CPS-B-21 registers (`QSOUND_4`):

| Offset | Meaning |
|---|---|
| 0x56 | layer control: the same drawing-order fields (bits 6-13); enable bits **0x04 scroll1, 0x08 scroll2, 0x10 scroll3** |
| 0x40, 0x42, 0x68, 0x6a | priority masks 0-3 |
| 0x6c | control register |
| 0x6e | reads the board ID 0x0c01; only programs that check it care |

Inputs (`cps1.c` L3002-3062, L327-343), all active low:
- 0x800000: P1 low byte, P2 high byte; per byte bit 0 right, 1 left, 2 down, 3 up, 4 button 1, 5 button 2, **6 button 3**; **bit 7 is P3's button 3 and bit 15 P4's button 3**.
- 0x800018: system (coin 1 bit 0, coin 2 bit 1, start 1 bit 4, start 2 bit 5).
- 0xf1c000 (P3) and 0xf1c002 (P4), also readable at 0x800176/0x800178: bits 0-3 stick, 4 button 1, 5 button 2, 6 coin, 7 start.
- No DIP switches: settings live in EEPROM/NVRAM.

The 68000 interrupt is the same level 2 vblank (`cps1_qsound_interrupt`). The Z80 runs at 6 MHz with a 250 Hz IRQ; its map (`cps1.c` L385-401): ROM 0x0000-0x7fff, bank 0x8000-0xbfff, shared RAM 0xc000-0xcfff and 0xf000-0xffff (the 68000 sees them at 0xf18000 and 0xf1e000, low bytes), QSound chip at 0xd000-0xd003, status at 0xd007.

**Kabuki** (`kabuki.c` L1-80 for the description, `bytedecode` and `cps1_decode` L105-201) (tested): at start the core decodes the Z80 ROM's first 0x8000 bytes twice, into an opcode table (`select = address + addr_key`) and a data table (`select = (address ^ 0x1fc0) + addr_key + 1`). Opcode fetches read the first, operands and data the second. Each decode is a bijection over 0-255 for a given address, so `kabuki.ts` in `frontend/packages/cps1` stores our own Z80 code by searching the byte that decodes to each one we want. `slammast` keys: swap1 0x54321076, swap2 0x65432107, addr 0x3131, xor 0x19.

## `captcomm` (CPS-1 board)

Why this set (see the [journal](journal.md#step-0--research)): plain CPS-1 (no QSound, no encryption), **4 players**, CPS-B config `BATTRY_3` with no ID check, no tile banks, no sprite kludge, full tile ranges. Machine driver `cps1`, init `cps1` (`cps1.c` L8174).

ROM layout (`cps1.c` L6219-6246, `ROM_START( captcomm )`). Only names and sizes matter to us; every byte is ours.

| Region | File | Size | How it loads |
|---|---|---|---|
| CPU1 (68000 program, 0x200000) | `cce_23d.rom` | 0x80000 | `ROM_LOAD16_WORD_SWAP` at 0x000000: the file holds every word **byte-swapped** **(tested)** |
| | `cc_22d.rom` | 0x80000 | `ROM_LOAD16_WORD_SWAP` at 0x080000 |
| | `cc_24d.rom` | 0x20000 | `ROM_LOAD16_BYTE` at 0x100000 (even bytes) |
| | `cc_28d.rom` | 0x20000 | `ROM_LOAD16_BYTE` at 0x100001 (odd bytes) |
| GFX1 (0x400000) | `gfx_01.rom`, `gfx_03.rom`, `gfx_02.rom`, `gfx_04.rom` | 0x80000 each | `ROMX_LOAD ... ROM_GROUPWORD \| ROM_SKIP(6)` at 0x000000, 0x000002, 0x000004, 0x000006 |
| | `gfx_05.rom`, `gfx_07.rom`, `gfx_06.rom`, `gfx_08.rom` | 0x80000 each | same, at 0x200000, 0x200002, 0x200004, 0x200006 |
| CPU2 (Z80, 0x18000) | `cc_09.rom` | 0x10000 | first 0x8000 at 0x0000, `ROM_CONTINUE` 0x8000 at 0x10000 (banked) |
| SOUND1 (OKI samples) | `cc_18.rom`, `cc_19.rom` | 0x20000 each | at 0x00000 and 0x20000 |

ROM loading (`common.c` L1301-1385) **(tested)**: a file with the right name and a **wrong CRC or length is a warning** ("WRONG CHECKSUMS", a 180-frame on-screen message), not an error; only a **missing** file stops the game. go-link Console's own check (`backend-device/pkg/romcheck`, `Checker.has`) follows the same rule: a file counts when its name is there.

## 68000 memory map (`cps1.c` L327-363)

| Address | Read | Write |
|---|---|---|
| 0x000000-0x1fffff | program ROM | — |
| 0x800000 | P1 (low byte) and P2 (high byte), active low | — |
| 0x800018 | system: coins, starts, service (`cps1_input_r` offset 0) | — |
| 0x80001a / 0x80001c / 0x80001e | DIP switches A / B / C | — |
| 0x800030 | — | coin counters |
| 0x800100-0x8001ff | CPS-A/CPS-B registers (`cps1_output_r`) | CPS-A/CPS-B registers (`cps1_output_w`) |
| 0x800176 | P3 (`cps1_input2_r`, both bytes) | — |
| 0x800178 | P4 (`cps1_input3_r`, both bytes) | — |
| 0x800180 | — | sound command to the Z80 |
| 0x900000-0x92ffff | graphics RAM | graphics RAM (`cps1_gfxram_w`) |
| 0xff0000-0xffffff | work RAM | work RAM |

Interrupt **(tested)**: level 2 at every vblank, `HOLD_LINE` (`cps1.c` L160-169), so autovector 26 (vector address 0x68). 68000 at 10 MHz, 60 Hz (`cps1.c` L3969-3986).

## Video registers (0x800100 + offset) (`cps1_vidhrdw.c` L54-110, L654-659) (tested: bases, scroll, layer control)

| Offset | Meaning |
|---|---|
| 0x00 | sprite table base (value × 256, inside graphics RAM) |
| 0x02 / 0x04 / 0x06 | scroll1 (8×8) / scroll2 (16×16) / scroll3 (32×32) tilemap base (× 256, 0x4000 aligned) |
| 0x08 | "other" base (row scroll) |
| 0x0a | palette base (× 256) |
| 0x0c / 0x0e | scroll1 X / Y |
| 0x10 / 0x12 | scroll2 X / Y |
| 0x14 / 0x16 | scroll3 X / Y |
| 0x22 | video control (bit 0 row scroll on scroll2, bit 15 flip screen) |

The base registers point inside graphics RAM: address = (value × 256) & 0x3ffff, from 0x900000 (`cps1_base`, L527-541). The driver's defaults (`VIDEO_START( cps )`, L1153-1220): sprites 0x9200, scroll1 0x9000, scroll2 0x9040, scroll3 0x9080, other 0x9100, palette 0x90c0.

CPS-B registers for `captcomm` (config `BATTRY_3`, L253 and L333):

| Offset | Meaning |
|---|---|
| 0x60 | layer control: bits 6-13 the drawing order (four 2-bit fields, 0 = sprites, 1/2/3 = scroll1/2/3, drawn first to last), enable bits 0x20 = scroll1, 0x12 = scroll2 and scroll3 |
| 0x6e, 0x6c, 0x6a, 0x68 | priority masks 0-3 (tile pens that go over sprites) |
| 0x70 | control register, always 0x003f |
| 0x46 × 0x44 → 0x42 (low), 0x40 (high) | the 16×16 → 32-bit multiplier |

No CPS-B ID check for this set (`cpsb_addr` 0).

## Tilemaps (`cps1_vidhrdw.c` L1025-1131, L1153-1160)

- scroll1: 64×64 tiles of 8×8; scroll2: 64×64 of 16×16; scroll3: 64×64 of 32×32.
- Each tile is two words: code, then attribute (bits 0-4 palette, bit 5 X flip, bit 6 Y flip, bits 7-8 priority group).
- Memory order is column-major: scroll1 word offset = 2 × ((row & 0x1f) + (col << 5) + ((row & 0x20) << 6)); scroll2 = 2 × ((row & 0x0f) + (col << 4) + ((row & 0x30) << 6)); scroll3 = 2 × ((row & 0x07) + (col << 3) + ((row & 0x38) << 6)).
- scroll1 code 0x0020 is never drawn on CPS-1 (a space).
- Pen 15 is transparent.

## Sprites (`cps1_vidhrdw.c` L1290-1500, L1975-1985) (tested)

- 8 bytes each: X, Y, code, attribute. Attribute bits 0-4 palette, 0x20 X flip, 0x40 Y flip, 0x0f00 width-1 and 0xf000 height-1 in 16×16 tiles for a block sprite. Tile (x, y) of a block is `code + x + 0x10 × y`.
- An attribute of 0xff00 ends the table.
- Sprites are drawn last to first and **delayed one frame** (the table is copied at the end of each frame).
- Pen 15 is transparent.
- The palette is per entry: a block sprite has one palette, separate entries can each have their own (the prototype draws one entry per 16×16 tile). The table is 0x800 bytes = 256 entries; the core has no per-line limit.
- **Pitfall (tested twice: the sprite table in step 2, palettes in step 4):** write to the board with plain `volatile` stores of a value held in a register (`copy_words` in `main.c`). gcc compiled a RAM-to-table copy into `move.w (a0)+,(0,a0,dN.l)`, which the emulated 68000 evaluates with the already incremented a0, so the data lands one word late ([journal, step 2](journal.md#step-2--a-mini-level), [step 4](journal.md#step-4--a-metal-slug-style-level)). `rom/tools/build.mjs` refuses any program that contains that addressing pattern.

## Scrolling (tested, step 4)

- With scroll registers (X, Y), the tilemap pixel at the screen's top-left is (X + 64, Y + 16), the visible area's offset in the 512×256 bitmap. A camera at world (cx, cy) on a layer whose tilemap pixel = world pixel needs scroll = (cx − 64, cy − 16). Half-speed parallax: (cx/2 − 64, cy/2 − 16).
- Tilemaps wrap at their size: scroll2 is 64×64 cells of 16 px = 1024×1024, scroll3 64×64 of 32 px = 2048×2048.
- Sprite X and Y are 9 bits (`& 0x1ff`) and **wrap at 512**: an object 500 px off to the right shows up on screen, so off-screen entries must be skipped, not just clipped.

## Screen

512×256 bitmap, visible area x 64-447, y 16-239: **384×224** (`cps1.c` L3985-3986). A sprite at screen pixel (sx, sy) has X = sx + 64, Y = sy + 16. The background behind everything is palette entry 4095 (`cps1_vidhrdw.c` L1891).

## Palette (`cps1_vidhrdw.c` L625, L1253-1290) (tested)

The exact formula (`cps1_build_palette`): for a word `0xBRGB`, each channel is `value × (B + 2)` when B ≠ 0, and black when B = 0. That gives 56 402 distinct colors (`frontend/packages/cps1/src/color.ts`, `cps1Colors`): a low brightness gives finer dark steps than `value × 17`.


- 256 palettes of 16 colors (4096 words): palettes 0-31 sprites, 32-63 scroll1, 64-95 scroll2, 96-127 scroll3, then the starfields.
- Each color is a word `0xBRGB`: brightness in the top 4 bits; on CPS-1 a channel is `value × (B + 2)` when B is not 0, so B = 0xF gives full range (15 × 17 = 255).

## Graphics ROM format (`cps1_vidhrdw.c` L710-741; `cps1.c` L3885-3925) (tested: 8×8, 16×16 and 32×32)

- The four ROMs of each bank are interleaved two bytes at a time (`ROM_GROUPWORD | ROM_SKIP(6)`), making 8-byte groups: bytes 0-1 from `gfx_01`, 2-3 from `gfx_03`, 4-5 from `gfx_02`, 6-7 from `gfx_04` (and 05/07/06/08 for the second bank).
- At start the driver turns every 4 bytes (one bitplane per byte, 8 pixels) into 8 packed 4-bit pixels: pixel j takes bit (7 - j) of byte 0 (plane 0), byte 1 (plane 1), byte 2 (plane 2) and byte 3 (plane 3).
- A row of a 16×16 tile is 8 bytes (pixels 0-7, then 8-15) and a tile is 128 bytes: tile `c` is at `c × 128`.
- An 8×8 tile `c` is at `c × 64`, using the **right** 4 bytes of each 8-byte row.
- A 32×32 tile `c` is at `c × 512`, with 16 bytes per row.
- All three sizes share one address space, so a game must keep them apart.

## `captcomm` inputs (`cps1.c` L2452-2577, `INPUT_PORTS_START( captcomm )`) (tested: P1, coin 1, start 1)

All active low.
- System (0x800018): bit 0 coin 1, bit 1 coin 2, bit 2 service, bit 4 start 1, bit 5 start 2, bit 6 test.
- Players 1/2 (0x800000), per byte: bit 0 right, bit 1 left, bit 2 down, bit 3 up, bit 4 button 1, bit 5 button 2.
- Players 3/4 (0x800176 / 0x800178): same bits plus bit 6 coin and bit 7 start of that player.
- Two buttons per player.

## `captcomm` sound side (`cps1.c` L366-385)

- Z80 at 4 MHz: ROM 0x0000-0x7fff, banked ROM 0x8000-0xbfff, RAM 0xd000-0xd7ff.
- YM2151 at 0xf000/0xf001, OKI6295 at 0xf002, sound latch (the 68000's command) at 0xf008, bank select at 0xf004.
- The YM2151 interrupt goes to the Z80's IRQ.
