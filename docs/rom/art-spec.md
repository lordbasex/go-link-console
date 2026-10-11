# Art and level specification

How to deliver characters, backgrounds and levels so they go into the ROM **as they are**, with no touching up: the build converts them straight into CPS-1 graphics, palettes and maps. Everything here comes from the board's real limits ([hardware.md](hardware.md)) and from what the Step 4 prototype measured ([journal.md](journal.md)).

If a delivery follows this page, the build accepts it or says exactly which rule a file breaks.

## 1. The screen and the scale

| Fact | Value |
|---|---|
| Screen | **384 × 224** pixels (CPS-1), shown 2× in go-link Console rooms (768 × 448) |
| Frame rate | 60 frames per second |
| Grid | everything snaps to **16 px** (the playfield's tile size) |
| Hero height | **44 px** = 19.6 % of the screen height (Metal Slug's soldiers are about 40 px of 224) |
| Hero jump | peaks at **61.9 px** (−7 px per frame, gravity 6/16): a ledge **48 px** up (3 tiles) is reachable, 64 px is not (experiment 1, case C) |
| Step climbed by pushing | up to **32 px** (one crate) |

Sizes as a share of the screen height (224 px), to keep every character in proportion:

| Who | Height (px) | % of screen height | Frame cell (w × h) |
|---|---|---|---|
| Willy, Vera, Uplink recruits | 44 | 19.6 % | 48 × 48 (64 × 48 for wide poses: shooting, kicks) |
| Glitch-9 (android) | 48 | 21.4 % | 64 × 64 |
| Jitter (alien) | 46 | 20.5 % | 64 × 64 |
| Lag troopers | 44 | 19.6 % | 48 × 48 |
| Woman / elderly man | 42 | 18.8 % | 32 × 48 |
| Child | 32 | 14.3 % | 32 × 48 |
| Baby | 20 | 8.9 % | 32 × 32 |
| Spinner drone, Pinger turret | 16–24 | 7–11 % | 32 × 32 |
| Bosses | 64–160 | 29–71 % | multiples of 16, up to 192 × 192 |
| Muzzle flash / bullet | 8–16 | — | 16 × 16 |
| Explosions | 32 (small), 64–96 (big) | — | 32 × 32 / 96 × 96 |

## 2. Character sprites

### Drawing rules

- **Draw at the final size, 1 pixel = 1 pixel.** Never draw big and shrink: that is what blurred the prototype. A 44 px hero is drawn 44 px tall.
- **Transparent background: pure magenta `#FF00FF`.** Never black or a dark color, because a black shirt or outline would be cut out with the background. No semi-transparent pixels.
- **No anti-aliasing against the background, no soft gradients, no glow.** Shading in hard steps (ramps of 2 to 4 tones).
- **A 1 px dark outline** around the character (the same outline color for the whole character).
- **Facing right only.** The board mirrors the sprites for the left side; nothing asymmetric that would look wrong mirrored (the shirt logo is the only exception, and it is small enough).
- **Feet on the bottom row of the cell**, centered horizontally: that point is the anchor where the character stands. It must not jump between frames.

### Colors

The board gives every 16 × 16 piece of a sprite its own palette of **15 colors** (plus transparency). Paint by **zones**, each zone with at most 15 colors, all zones sharing the outline color:

| Zone | Colors for a hero (max 15 per zone) |
|---|---|
| Head | skin ramp 4, hair 3, eyes and beard 2, outline 1 |
| Torso | shirt ramp 3, logo 3, skin 2, outline 1 |
| Legs and feet | jeans ramp 3, sneakers 3, outline 1 |
| Weapon and flash | metal 3, fire 4, outline 1 |

- At most **4 palettes per hero**, 2–3 per civilian or enemy. The whole game has 32 sprite palettes, shared.
- **The shirt in its own exact colors**, used nowhere else on the character: players 2 to 4 are made by changing only those colors (palette swaps).
- **Use the board's colors:** each channel (R, G, B) a multiple of 17: 00, 11, 22 … EE, FF in hex (`#3355AA` yes, `#3456AB` no). That is 4096 colors, all exact on the board. The build snaps any other color to the nearest one and lists the change.

### Sheets

- One PNG per animation: `characters/<character>/<animation>.png`, frames left to right in cells of the size in the table above, no gaps, no labels, no guide lines. Indexed PNG is welcome; RGB is fine if it follows the color rules.
- A `characters/<character>/sheet.json` saying, per animation: frames, frames per second, loop yes/no, and the special points per frame when they exist (`muzzle`: where bullets leave; `hand`: where the special weapon is held).

### Frame list

Heroes (Willy, Vera, the recruits; Glitch-9 and Jitter have their own moves):

| Animation | Frames | FPS | Loop |
|---|---|---|---|
| `idle` | 4 | 6 | yes |
| `walk` | 8 | 12 | yes |
| `run` | 6 | 14 | yes |
| `jump` | 3 (up, top, down) | — | no |
| `land` | 1 | — | no |
| `climb` (ladder, seen from behind) | 4 | 10 | yes |
| `climb_crate` (pulling up onto a ledge) | 3 | 12 | no |
| `crouch` | 1 | — | — |
| `crawl` | 4 | 10 | yes |
| `shoot` straight / up / diagonal up / down in the air | 3 each | 15 | yes |
| `knife` | 4 | 15 | no |
| `grenade` | 4 | 12 | no |
| `special` (holding the picked-up weapon: bazooka, flamethrower…) | 2 per weapon | 10 | no |
| `hit` | 2 | 10 | no |
| `death` | 6 | 10 | no |
| `thumbs_up` (rescue) | 3 | 8 | no |
| `victory` | 4 | 8 | no |

Enemies: `walk` 6, `shoot` 3, `melee` 4, `hit` 2, `death` 6. Civilians: `worried` 3, `follow` 6, `thanks` 3.

## 3. Backgrounds

The board has three scrolling layers. Each one is a grid of tiles, and every background is delivered as those tiles' picture plus a map.

| Layer | Tile size | Use | Scroll |
|---|---|---|---|
| **Far** (scroll3) | 32 × 32 | sky, moon, far skyline | slowest (half the camera speed) |
| **Play** (scroll2) | 16 × 16 | everything the players stand on, climb or break, and the buildings right behind them | 1:1 with the camera |
| **Text** (scroll1) | 8 × 8 | score, messages; not used for art | fixed |

Rules:

- **Every element on the 16 px grid** (32 px for the far layer): floors, crate tops, platform edges and ladder rungs start on a multiple of 16.
- **15 colors per tile** (plus transparency on the play layer), from the same 4096 board colors. The whole play layer can use up to 32 palettes, the far layer another 32: group similar tiles (brick, glass, metal, neon) so they share palettes.
- **Repeat tiles.** Every unique tile costs ROM; a window, a brick or a floor stretch drawn once and repeated keeps the game within the board's graphics ROM (6 MB, shared with the sprites; the build reports the usage).
- **Magenta `#FF00FF`** for the transparent parts of the play layer (the far layer shows behind them).
- **Readability first:** the play layer slightly brighter and more contrasted than the far layer, so characters and platforms stand out; nothing bright and busy right behind where the players walk.

Delivery per level:

- `levels/<n>/far.png`: the whole far layer at full size.
- `levels/<n>/play.png`: the whole play layer at full size (what the players walk on and in front of).
- `levels/<n>/level.tmx` or `.json`: the map (next section).

The build cuts the PNGs into tiles, removes duplicates and builds the maps; you never deliver tiles one by one.

## 4. Telling the build what every object is

Paint is not enough: the game needs to know what is a floor, what is climbable, what breaks. Deliver each level as a **[Tiled](https://www.mapeditor.org/) map** (free editor; `.tmx` or `.json`) with these layers:

1. **`play`** (tile layer, 16 × 16): the play layer's art, as tiles from `play.png`.
2. **`collision`** (tile layer, 16 × 16): one invisible tile per cell, from a small set whose *type* says what the cell is:

| Type | What it means |
|---|---|
| `solid` | blocks and can be stood on (ground, walls, solid buildings) |
| `oneway` | can be stood on, jumped through from below, and dropped through with down + jump (ledges, roofs, scaffolds, fire escapes) |
| `ladder` | up and down on the stick climbs it |
| `crate` | a solid block that can be climbed by pushing (32 px) and broken (`hp`) |
| `breakable` | solid until destroyed (walls, glass, signs): `hp`, and the tile it turns into |
| `hazard` | hurts (fire, electricity, spikes): `damage` |
| `water` | slows down, no jetpack |
| (empty) | air |

3. **`objects`** (object layer): everything that is not a tile, as a point or a rectangle with a *type* and its properties:

| Type | Properties |
|---|---|
| `player_start` | `player` (1–4) |
| `enemy` | `kind` (trooper, shield_trooper, spinner, pinger, glitch9, vera, jitter…), `facing`, `patrol` (px) |
| `civilian` | `kind` (woman, child, baby, elder), `trapped_in` (the crate or wall that hides them, if any) |
| `crate` | `size` (32 or 16), `hp`, `contains` (nothing, weapon, health, civilian) |
| `pickup` | `item` (bazooka, flamethrower, spread, grenades, health, lattenza_page, coin, spring, pipe, knife), `look` (optional: a character id that draws it) |
| `platform` | a moving platform: `w` (32-128 px), `axis` (`x` across, `y` up and down), `range` (px, there and back), `speed` (1-4 px a frame), `falls` (true: no track, it shakes once stood on, falls and comes back); (x, y) its top left at the start |
| `camera_lock` | a rectangle where the camera stops until its enemies are defeated |
| `checkpoint` | where players come back after losing a life |
| `boss` | `kind`, and the arena rectangle (armored_truck, gunship); the beat 'em up's `brawler` has `facing` and `hp` instead |
| `exit` | the end of the level |

If you cannot use Tiled, deliver instead a **`collision.png`** the size of `play.png`, painted on the same 16 px grid with these colors, plus a list of the objects with their pixel positions:

| Color | Type |
|---|---|
| `#000000` | solid |
| `#00FF00` | oneway |
| `#0000FF` | ladder |
| `#FFAA00` | crate |
| `#FF0000` | breakable |
| `#FF00FF` | hazard |
| `#00FFFF` | water |
| `#FFFFFF` | air |

## 5. Level size and Mission 1: Buenos Aires

A Metal Slug mission takes about 3 to 5 minutes to play, mostly moving forward to the right with short climbs to an upper path and back. For our first mission the target is **about 5 minutes**:

- **Canvas: 8192 × 672 px** (about 21 screens wide, 3 screens tall at the highest point). Most of the level is 224–448 px tall; the extra height is only where there is an upper route (cranes, the tower).
- The camera moves right, never back more than a few pixels; it stops at `camera_lock` areas until the enemies there are beaten.
- **Climbing rules for level design:**
  - a ledge up to 64 px above the floor is reached with a jump;
  - a 32 px step or crate is climbed by pushing against it;
  - anything higher needs a ladder, a crate stair, or a jump from a higher point;
  - every upper route has a way down (down + jump through a one-way ledge, or a drop).

Mission 1, "Dead Air" (see [story.md](story.md)), sectioned on that canvas:

| Section | x from – to (px) | What is there |
|---|---|---|
| 1. Puerto Madero docks, rain at night | 0 – 1536 | the tutorial: walking, jumping, shooting; containers and crates to climb; the first civilians |
| 2. The cranes | 1536 – 3072 | an upper route along the crane arms (ladders and one-way beams) and the lower dock route; Spinners appear |
| 3. The tower lobby | 3072 – 4608 | laser fences to crawl under, breakable glass walls, the armored truck mid-boss (camera lock) |
| 4. The server floors | 4608 – 6656 | the level goes down: break the floor to drop between floors (`breakable` floors), the jetpack to go back up; hostages in server cages |
| 5. The rooftop helipad | 6656 – 8192 | the gunship boss arena (camera lock), the Lattenza page in a server rack |

## 6. Checklist before delivering

- [ ] Magenta `#FF00FF` background, no semi-transparent pixels, no anti-aliasing against it.
- [ ] Every color a multiple of 17 per channel.
- [ ] At most 15 colors per zone (sprites) or per tile (backgrounds), sharing the outline color.
- [ ] Characters at the heights of section 1, drawn 1:1, facing right, feet on the bottom row.
- [ ] The shirt's colors used only on the shirt (palette swaps).
- [ ] Backgrounds on the 16 px grid (32 px for the far layer), with repeated tiles.
- [ ] The Tiled map with `play`, `collision` and `objects`, or `collision.png` plus the object list.
- [ ] File names and folders as in this page.
