// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// The AI pack (docs/willy-maker/file-format.md): everything an AI, or a
// person, needs to produce the ROM with the tools in rom/. The levels as
// Tiled maps with their pictures, each character as animation strips and a
// sheet.json, the project file, the review, our ROM docs and a generated
// PROMPT.md. Pictures are converted to the board's colors. The output is
// deterministic: the same project gives the same bytes (fixed timestamps,
// sorted entries, a PNG writer without a canvas).

import { CELL, DEFAULT_GENRE, genreAvailable, isGenre, layerGrid, TAGS, type AssetRef, type Character, type Level, type Project, type TileLayer, type Tileset } from "../model";
import { boardOf, layoutOf } from "../board/cps1";
import * as R from "../engine/rules";
import { measureJump } from "../engine/jump";
import { levelHeroes } from "../game/settings";
import { checkText, REQUIRED_ANIMS, type Review } from "../editor/validate";
import { exportEn } from "../i18n/export.en";
import { coreEn } from "../i18n/core.en";
import { getAsset } from "./assets";
import { decodePng, encodePng, type RgbaImage } from "./png";
import { COLLISION_TILES, levelToTiled, playLayer, TAG_COLORS } from "./tiledExport";
import { readZip, writeZip, type ZipEntry } from "./zip";
import { checkAiPack, PackBuildError } from "./packCheck";

/** The far layer's tile on the board (scroll3). */
const FAR_TILE = 32;

/** Zip entries get this date, so two exports of the same project are the same bytes. */
export const PACK_DATE = new Date(2026, 0, 1, 0, 0, 0);

/** Our ROM docs as the pack carries them (docs/<name>), from docs/rom/. */
export const DOC_FILES: Record<string, string> = { "README.md": "rom-README.md", "art-spec.md": "art-spec.md", "hardware.md": "hardware.md", "story.md": "story.md", "journal.md": "journal.md" };

/** Willy Maker's own docs the pack carries too (P-21: project.json's format was only named). */
export const MAKER_DOC_FILES: Record<string, string> = { "file-format.md": "file-format.md", "moves.md": "moves.md" };

/** Loads docs/rom/*.md and Willy Maker's docs from the repository (bundled at build time, fetched when the pack is made). */
export async function loadRomDocs(): Promise<Record<string, string>> {
  const rom = import.meta.glob("../../../../../docs/rom/*.md", { query: "?raw", import: "default" }) as Record<string, () => Promise<string>>;
  const maker = import.meta.glob("../../../../../docs/willy-maker/{file-format,moves}.md", { query: "?raw", import: "default" }) as Record<string, () => Promise<string>>;
  const out: Record<string, string> = {};
  for (const [files, names] of [[rom, DOC_FILES], [maker, MAKER_DOC_FILES]] as const)
    for (const [path, load] of Object.entries(files)) {
      const name = names[path.slice(path.lastIndexOf("/") + 1)];
      if (name) out[name] = await load();
    }
  return out;
}

export interface AiPackOptions {
  review: Review;
  /** docs/<name> contents; loadRomDocs() when missing. */
  docs?: Record<string, string>;
  /** Reads a stored picture (IndexedDB by default). */
  loadAsset?: (ref: string) => Promise<{ bytes: Uint8Array; type: string } | null>;
  /** Decodes a picture that is not a PNG (the browser's canvas by default). */
  decode?: (bytes: Uint8Array, type: string) => Promise<RgbaImage>;
  /** Skips the build check (level 2); only tests that break a pack on purpose use it. */
  skipCheck?: boolean;
}

export interface AiPack {
  zip: Uint8Array;
  prompt: string;
  /** The entry names, in zip order. */
  files: string[];
}

/** "The Lag Protocol" -> "the-lag-protocol.ai-pack.zip". */
export function aiPackName(p: Project): string {
  const slug = p.title
    .normalize("NFKD")
    .replace(/[̀-ͯ]/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return `${slug || "willy-maker-game"}.ai-pack.zip`;
}

const safe = (s: string) => s.replace(/[^A-Za-z0-9_-]+/g, "_") || "_";

async function canvasDecode(bytes: Uint8Array, type: string): Promise<RgbaImage> {
  if (typeof document === "undefined") throw new Error("cannot read this picture here");
  const { decodeImage } = await import("../sprites/image");
  const img = await decodeImage(bytes, type);
  return { w: img.w, h: img.h, data: new Uint8Array(img.data.buffer, img.data.byteOffset, img.data.byteLength) };
}

/** The pixel on the board: each channel to a multiple of 17, alpha on or off. */
function toBoard(img: RgbaImage): RgbaImage {
  const d = new Uint8Array(img.data.length);
  for (let i = 0; i < d.length; i += 4) {
    if (img.data[i + 3]! < 128) continue;
    d[i] = Math.round(img.data[i]! / 17) * 17;
    d[i + 1] = Math.round(img.data[i + 1]! / 17) * 17;
    d[i + 2] = Math.round(img.data[i + 2]! / 17) * 17;
    d[i + 3] = 255;
  }
  return { w: img.w, h: img.h, data: d };
}

function blit(dst: RgbaImage, src: RgbaImage, sx: number, sy: number, w: number, h: number, dx: number, dy: number, opaqueOnly = true): void {
  for (let y = 0; y < h; y++) {
    const ty = dy + y;
    const fy = sy + y;
    if (ty < 0 || ty >= dst.h || fy < 0 || fy >= src.h) continue;
    for (let x = 0; x < w; x++) {
      const tx = dx + x;
      const fx = sx + x;
      if (tx < 0 || tx >= dst.w || fx < 0 || fx >= src.w) continue;
      const s = (fy * src.w + fx) * 4;
      if (opaqueOnly && src.data[s + 3]! < 128) continue;
      const t = (ty * dst.w + tx) * 4;
      dst.data[t] = src.data[s]!;
      dst.data[t + 1] = src.data[s + 1]!;
      dst.data[t + 2] = src.data[s + 2]!;
      dst.data[t + 3] = src.data[s + 3]!;
    }
  }
}

/** Draws a tile layer with its tileset over `dst`; returns false when nothing was drawn. */
function drawTiles(dst: RgbaImage, level: Level, layer: TileLayer, ts: Tileset, img: RgbaImage): boolean {
  const g = layerGrid(level, layer);
  const size = ts.tile;
  const columns = ts.columns || Math.max(1, Math.floor(img.w / size));
  let drew = false;
  for (let r = 0; r < g.rows; r++)
    for (let c = 0; c < g.cols; c++) {
      const v = g.get(c, r);
      if (!v) continue;
      const i = v - 1;
      blit(dst, img, (i % columns) * size, Math.floor(i / columns) * size, size, size, c * layer.grid, r * layer.grid + layer.grid - size);
      drew = true;
    }
  return drew;
}

/**
 * A layer picture cut into the board's tiles, the same ones deduplicated
 * (T-20, E-03: a whole 8192 px level picture weighed about 22 MB): the
 * unique tiles in rows of 16 and, per tile of the picture, 0 when it is
 * empty or the tile's number from 1.
 */
export function cutTiles(img: RgbaImage, size: number): { sheet: RgbaImage; cells: Uint32Array; cols: number; rows: number; count: number; columns: number } {
  const cols = Math.ceil(img.w / size);
  const rows = Math.ceil(img.h / size);
  const cells = new Uint32Array(cols * rows);
  const seen = new Map<string, number>();
  const tiles: Uint8Array[] = [];
  for (let r = 0; r < rows; r++)
    for (let c = 0; c < cols; c++) {
      const t = new Uint8Array(size * size * 4);
      let any = false;
      for (let y = 0; y < size; y++) {
        const sy = r * size + y;
        if (sy >= img.h) break;
        for (let x = 0; x < size; x++) {
          const sx = c * size + x;
          if (sx >= img.w) break;
          const o = (sy * img.w + sx) * 4;
          if (img.data[o + 3]! < 128) continue;
          const q = (y * size + x) * 4;
          t[q] = img.data[o]!;
          t[q + 1] = img.data[o + 1]!;
          t[q + 2] = img.data[o + 2]!;
          t[q + 3] = 255;
          any = true;
        }
      }
      if (!any) continue;
      let key = "";
      for (let i = 0; i < t.length; i += 4) key += String.fromCharCode(t[i]!, t[i + 1]!, t[i + 2]!, t[i + 3]!);
      let n = seen.get(key);
      if (!n) {
        tiles.push(t);
        n = tiles.length;
        seen.set(key, n);
      }
      cells[r * cols + c] = n;
    }
  const columns = 16;
  const sheetRows = Math.max(1, Math.ceil(tiles.length / columns));
  const sheet: RgbaImage = { w: columns * size, h: sheetRows * size, data: new Uint8Array(columns * size * sheetRows * size * 4) };
  tiles.forEach((t, i) => {
    const ox = (i % columns) * size;
    const oy = Math.floor(i / columns) * size;
    for (let y = 0; y < size; y++) sheet.data.set(t.subarray(y * size * 4, (y + 1) * size * 4), ((oy + y) * sheet.w + ox) * 4);
  });
  return { sheet, cells, cols, rows, count: tiles.length, columns };
}

function collisionTileset(): RgbaImage {
  const w = COLLISION_TILES.length * CELL;
  const data = new Uint8Array(w * CELL * 4);
  COLLISION_TILES.forEach((tag, t) => {
    const [r, g, b] = TAG_COLORS[tag]!;
    for (let y = 0; y < CELL; y++)
      for (let x = 0; x < CELL; x++) {
        const i = (y * w + t * CELL + x) * 4;
        data[i] = r;
        data[i + 1] = g;
        data[i + 2] = b;
        data[i + 3] = 255;
      }
  });
  return { w, h: CELL, data };
}

const MAGENTA = [255, 0, 255] as const;

/** An animation as one strip on magenta, feet on one line, and its frame boxes in the strip. */
function animStrip(ch: Character, frameIds: string[], atlas: RgbaImage): { img: RgbaImage; frames: Record<string, unknown>[] } {
  const frames = frameIds.map((id) => ch.frames.find((f) => f.id === id)).filter((f): f is Character["frames"][number] => !!f);
  const base = frames.reduce((m, f) => Math.max(m, f.py), 0);
  const w = Math.max(1, frames.reduce((n, f) => n + f.w, 0));
  const h = Math.max(1, frames.reduce((m, f) => Math.max(m, base - f.py + f.h), 0));
  const img: RgbaImage = { w, h, data: new Uint8Array(w * h * 4) };
  for (let i = 0; i < img.data.length; i += 4) {
    img.data[i] = MAGENTA[0];
    img.data[i + 1] = MAGENTA[1];
    img.data[i + 2] = MAGENTA[2];
    img.data[i + 3] = 255;
  }
  const out: Record<string, unknown>[] = [];
  let x = 0;
  for (const f of frames) {
    const y = base - f.py;
    blit(img, atlas, f.x, f.y, f.w, f.h, x, y);
    out.push({ id: f.id, x, y, w: f.w, h: f.h, px: f.px, py: f.py, zones: f.zones, muzzle: f.muzzle, hand: f.hand });
    x += f.w;
  }
  return { img, frames: out };
}

const actionText: Record<string, string> = {
  jump: "jump (down + jump drops through a one-way ledge)",
  fire: "fire: the machine gun, aimed with the stick; the knife automatically when an enemy is right in front",
  special: "special: the picked-up weapon, with limited ammo (a grenade when there is none)",
};

/** PROMPT.md: the brief for the AI, filled in from the project and the review. */
/**
 * What Willy Maker's engine already decides, so a builder does not have to
 * guess (experiment 1's P-01 to P-26, docs/experiments/verdict.md T-10):
 * each answer says whether it is this game's setting (and where) or the
 * engine's fixed rule.
 */
export function decisions(p: Project, levels: Level[], G: R.GameRules, has: (doc: string) => boolean = () => true): string[] {
  const s = p.settings;
  const L: string[] = [];
  const line = (t = "") => L.push(t);
  const sections = levels.some((l) => l.sections?.length);
  line("## What is already decided");
  line();
  line("Questions an earlier builder had to answer alone, with this game's answers. \"Setting\" means it comes from project.json and Willy Maker's editor; \"fixed\" means every Willy Maker game plays so (play mode and go-link's own ROM engine, rom/engine/engine.c).");
  line();
  line("| Question | Answer |");
  line("|---|---|");
  const rows: [string, string][] = [
    ["How high is a jump? (P-01)", `measured on the engine with this game's rules (T-13): the feet rise ${measureJump(G).peak} px (start speed ${R.JUMP_VY}/16, gravity ${R.GRAVITY}/16 added before the first move${G.doubleJump ? ", the double jump on" : ""}${G.jetpack ? ", the jet pack on" : ""}), so a ledge ${measureJump(G).ledge} px up is reachable and ${measureJump(G).ledge + R.CELL} px is not`],
    ["How wide can a level be? (P-02)", "setting: each level's size in project.json; the prototype writes a 1024 px level once (scroll2's map is 1024 px wide), so a wider level needs the tile columns written around the camera as it moves, as rom/engine/engine.c does; it streams the rows of a level taller than 1024 px the same way, so any level up to 24 576 cells of 16 px fits (parallax bands only on levels up to 1024 px tall)"],
    ["What may the core's log say? (P-03)", "fixed: one \"WRONG CHECKSUMS\" line per file is expected for a set of your own bytes; \"NOT FOUND\" or \"INCORRECT LENGTH\" is an error"],
    ["Crate hits when the rules table and an object differ (P-04)", `setting: a crate object's \`hp\` wins; without one, ${R.CRATE_HP} hits`],
    ["What clears a level? (P-05)", `setting (Rules card): ${G.exitNeedsEnemies ? "a player in the exit zone with every enemy down" : "a player in the exit zone; enemies and civilians are not needed"}`],
    ["How big is the exit? (P-06)", `setting: the exit object's \`w\` (default ${2 * R.CELL} px) from its x to the right, from its y down (\`h\`, default the level's height)`],
    ["Where is an object's point? (P-07)", "fixed: a start's, an enemy's, a civilian's and a pickup's x is the middle of the body and y the feet (the top of the floor under it); a crate's x and y are its top-left cell; an exit's and a camera lock's x and y are their top-left corner; a moving platform's x and y are its top left at the start"],
    ["What hurts? (P-08)", `setting (Rules card): enemy shots ${G.enemiesShoot ? "hurt" : "are off"}, touching an enemy ${G.touchHurts ? "hurts" : "does not hurt"}, hazard cells and falling out of the map always hurt; no knockback; after a hit the player ${G.respawnOnHurt ? "comes back near the camera's left side on a floor" : "blinks in place"} for ${G.hurtFrames} frames. Enemy shots fly ${R.ENEMY_SHOT_SPEED} px per frame, 26 px over the enemy's feet`],
    ["Lives or energy? (P-09)", `fixed: the same thing: a player takes as many hits as the lives (setting: DIP switches, ${s.dip.lives}); the lab state reports them as \`energy\``],
    ["How do enemies move? (P-10)", `setting: an enemy's \`patrol\` (default ${6 * R.CELL} px) centered on its x, its \`facing\` and \`hp\`; Rules card: ${G.enemiesChase ? `they chase a player on their floor within ${R.ENEMY_SIGHT} px (16 px up or down)` : "they keep their patrol"}${G.enemiesShoot ? ` and fire every ${R.ENEMY_FIRE_EVERY} frames at one more than 40 px away (the first shot after ${R.ENEMY_FIRE_EVERY} frames)` : ""}; they walk 1 px every 2 frames`],
    ["What do enemy kinds look like? (P-11)", p.characters.some((c) => c.role === "enemy") ? "setting: an enemy kind with a character of the game uses its pictures (characters/); any other kind uses the engine's android" : "fixed: this game has no enemy characters, so every kind is drawn as the engine's android (rom/tools/art.mjs's robot)"],
    ["What do pickups look like?", p.characters.length ? "setting: a pickup with a `look` (a character id of the game) is drawn with that character's idle animation, in play mode and the ROM; without one it is the engine's icon (coin, spring, bazooka)" : "fixed: this game has no characters, so every pickup is the engine's icon (coin, spring, bazooka)"],
    ["What does the special button do? (P-12)", `fixed: fires the picked-up weapon while it has ammo: the bazooka, ${R.BAZOOKA_AMMO} rockets from a pickup or a crate's contents, standing only, a rocket counts as 9 hits; with no weapon it does nothing`],
    ["What does each player look like? (P-13)", p.characters.some((c) => c.role === "hero") ? "setting: each player's hero in project.json (settings.players and the characters); a player without one is Willy" : "fixed: Willy for every player, player 1 in his own shirt and players 2-4 in the three recruit shirts of rom/tools/art.mjs"],
    ["What do the menus show, and for how long? (P-14)", "setting: settings.menus (texts and blocks); an empty screen gets the engine's default with the 8 × 8 font; the Continue screen counts down from 9, one a second (600 frames); the clear and Game over screens show for 6 s before the title"],
    ["What does \"credits\" on a screen mean? (P-15)", "fixed: the credit counter (CREDITS n, bottom right, hidden in free play); a copyright line is one of the screen's own texts"],
    ["Music and sound? (P-16)", "go-link's engine (T-26): a QSound driver on the Z80 (rom/engine/sound.z80) with synthesized effects and built-in tunes per screen, chosen by each screen's music slot (setting: the Menus tab, \"none\" for silence); a pack-built ROM starts from the prototype's silent Z80 program"],
    ["What do the DIP switches change? (P-17)", `setting (DIP): lives ${s.dip.lives}, free play ${s.dip.freePlay ? "on" : "off"} and difficulty ${s.dip.difficulty} are used (the difficulty sets how often enemies fire and how fast their shots fly: easy every 150 frames at 2 px per frame, normal 90 at 3, hard 60 at 4, lag 40 at 5); demo sound has no effect yet; the EEPROM is not written`],
    ["Credits with four coin slots (P-18)", "fixed: one shared pool (up to 9), every coin slot adds to it, each Start takes one, and any port joins at any time while there are credits"],
    ["Does the harness need anything? (P-19)", "fixed: the lab state (rom/src/lab_state.h, filled every frame) and the symbol map that rom/tools/build.mjs writes; keep both"],
    ["Must the set be added to the device's own sets? (P-20)", "fixed: no; the device runs any set its core knows, and backend-device/pkg/ownsets lists only go-link's prototype"],
    ["Where is project.json's format? (P-21)", has("file-format.md") ? "docs/file-format.md in this pack" : "docs/willy-maker/file-format.md in the repository"],
    ["The play layer comes twice: which wins? (P-22)", "fixed: it comes once now, as the tile layer in levels/<id>.tmj over its tileset (and the far layer as its own 32 px tiles); the pictures are already in board colors, so a project palette that repeats a color changes nothing"],
    ["Sections (P-23)", sections ? "setting: the sections listed above, by x range" : "setting: this game has none, so each level is one section"],
    ["The HUD with 4 players, and a player who has not joined (P-24)", "fixed: the top two rows split in one block per player: 1P and the score, the energy as + marks under it and the ammo when they carry the bazooka; a player who has not joined shows the HUD's join text blinking (setting: the menus' texts), or the insert-coin text without credits"],
    ["Tile counts in the review (P-25)", "fixed: they include the empty tile the board needs for transparent cells and broken crates"],
    ["Must it be played in a go-link room? (P-26)", "optional: room-test.mjs needs the signalhub repository and free ports; the core run with framelab is the required test"],
  ];
  for (const [k, v] of rows) line(`| ${k} | ${v} |`);
  line();
  return L;
}

export function buildPrompt(p: Project, review: Review, notes: { missingPictures?: string[] } = {}, docs?: Record<string, string>): string {
  const board = boardOf(p);
  const layout = layoutOf(p);
  const s = p.settings;
  const order = (s.levels ?? []).map((id) => p.levels.find((l) => l.id === id)).filter((l): l is Level => !!l);
  const levels = order.length ? order : p.levels;
  const b = (s.buttons ?? {}) as unknown as Record<string, string>;
  const G = R.rulesWith(s.rules);
  const J = measureJump(G, Math.min(...levelHeroes(p).heights));
  const L: string[] = [];
  const line = (t = "") => L.push(t);

  line(`# ${p.title}: build the ROM`);
  line();
  // its limits first (T-19, E-02: an outside AI with only a browser read the whole brief before finding it could not build anything)
  line("## Before you start: what this pack can and cannot do");
  line();
  line("- **A browser is not enough.** Building from this pack needs a computer with a shell, the go-link repository (https://github.com/lordbasex/go-link-console, its `rom/` folder), the m68k-elf cross compiler and binutils, z80asm and Node 22.18 or newer; testing it needs the mame2003-plus core that a go-link device downloads.");
  line("- **`rom/tools/build.mjs` builds only go-link's prototype**, its own fixed level, not this game. This pack gives you this game's data; reading it into a ROM is the work (step 3 below).");
  line(`- **The shortcut:** Willy Maker's Export tab has **Create ROM**, which builds this game's ROM in the browser with go-link's engine (\`${layout.id}.zip\`), and \`node rom/tools/willy-rom.mjs GAME.willy.zip\` does the same from the project file (Save project) on the command line. Use this pack when you want a different engine or a different board.`);
  line("- **Nothing here is a ROM:** a ROM set is the `.zip` of the board's files that the build makes; this pack, and a `.willy.zip` project file, are its sources.");
  line();
  line(`You are building an arcade game ROM for go-link with the tools in the go-link repository (rom/). Target board: Capcom ${board.name} (${board.screen.w} × ${board.screen.h} at ${board.screen.fps} Hz, 68000 main CPU), laid out as the files of the \`${layout.id}\` set: ${layout.players} players × ${layout.buttons} buttons, run by the mame2003-plus core (MAME 0.78). Every byte must be original: no code, graphics, music or text from any existing game.`);
  line();
  const has = (name: string) => !docs || name in docs;
  line("Follow docs/rom-README.md, docs/art-spec.md and docs/hardware.md (in this pack). docs/journal.md is the lab journal of the prototype that already runs in the core and in a go-link room: its section \"How to reproduce from zero\" is the build you start from. docs/story.md is the game bible: the world, the heroes and the tone.");
  line();
  line(`The rules the game must keep (a jump peaks at ${J.peak} px, so ledges up to ${J.ledge} px; 32 px crates climbed ${G.crateClimb === "push" ? "by walking into them" : "by jumping"}, one-way ledges, ladders, double-tap run, automatic knife) are the ones Willy Maker's play mode used; they are listed below with their numbers. Where this game's own rules (the Game tab's Rules card) differ from the prototype's rom/src/main.c, this game's win.`);
  line();

  line("## The game");
  line();
  line(`- Title: "${p.title}"${p.author ? ` by ${p.author}` : ""}.`);
  const genre = isGenre(p.genre) ? p.genre : DEFAULT_GENRE;
  line(`- Genre: ${coreEn.genres[genre].name} (\`${genre}\`): ${coreEn.genres[genre].text}${genreAvailable(genre) ? " Willy Maker's play mode and the prototype's engine are this genre." : " Willy Maker has no engine for this genre yet (docs/willy-maker/genres.md in the repository is the plan); build it as a new engine, not on the platform shooter's rules."}`);
  line(`- Board: ${board.name}, layout \`${layout.id}\` (${layout.players} players × ${layout.buttons} buttons). Players: ${s.players}.`);
  line(`- Levels, in play order: ${levels.length}.`);
  levels.forEach((l, i) => {
    const sections = l.sections?.length ? `; sections: ${l.sections.map((x) => `${x.name} (x ${x.x0}-${x.x1})`).join(", ")}` : "";
    line(`  ${i + 1}. "${l.name}" (\`${l.id}\`, ${l.size.w} × ${l.size.h} px, camera ${l.camera.forwardOnly ? "forward only" : "free"} with a ${l.camera.backtrack} px margin back${sections}): levels/${safe(l.id)}.tmj`);
  });
  if (p.characters.length) {
    line(`- Characters: ${p.characters.length}.`);
    for (const ch of p.characters) {
      const anims = Object.entries(ch.anims ?? {})
        .map(([n, a]) => `${n} ${a.frames.length}f @${a.fps} fps${a.loop ? " loop" : ""}`)
        .join(", ");
      line(`  - ${ch.name} (\`${ch.id}\`, ${ch.role}, ${ch.height} px tall): ${anims || "no animations"}. characters/${safe(ch.id)}/`);
    }
  } else line("- Characters: none of its own. Use the prototype's Willy (rom/tools/art.mjs) for every player, with a different shirt per player.");
  line(`- Buttons (Run is a double tap of the stick toward a side, within ${R.RUN_TAP_FRAMES} frames):`);
  for (const slot of ["b1", "b2", "b3"]) {
    const v = b[slot];
    if (!v) continue;
    const n = slot.slice(1);
    if (v === "b1+b2") {
      const used = new Set([b.b1, b.b2]);
      const left = ["jump", "fire", "special"].find((a) => !used.has(a)) ?? "special";
      line(`  - Buttons 1 + 2 together: ${actionText[left] ?? left}`);
    } else if (Number(n) <= layout.buttons) line(`  - Button ${n}: ${actionText[v] ?? v}`);
  }
  line("  - Start and Coin: join and credit, like any arcade board. go-link maps the RetroPad B, A, Y to buttons 1, 2, 3 on every seat.");
  const d = s.dip;
  line(`- DIP switches (the slammast board keeps them in EEPROM): difficulty ${d.difficulty}, ${d.lives} lives, free play ${d.freePlay ? "on" : "off"}, demo sound ${d.demoSound ? "on" : "off"}.`);
  const menus = Object.entries((s.menus ?? {}) as unknown as Record<string, { blocks?: unknown[] } | undefined>)
    .map(([k, m]) => `${k}${Array.isArray(m?.blocks) && m.blocks.length ? ` (${m.blocks.length} blocks)` : ""}`)
    .join(", ");
  line(`- Menus: ${menus || "none"}. Their blocks are in project.json (settings.menus); an empty screen gets a plain default with the 8 × 8 font.`);
  line();

  line("## Rules the engine keeps");
  line();
  line("The numbers of Willy Maker's play mode (its engine/rules.ts) with this game's Rules card applied. Vertical speeds are in 1/16 px per frame, like the 68000 code.");
  line();
  line("| Rule | Value |");
  line("|---|---|");
  const rows: [string, string][] = [
    ["Screen, frame rate", `${R.SCREEN_W} × ${R.SCREEN_H}, ${Math.round(1000 / R.FRAME_MS)} frames per second`],
    ["Collision grid", `${R.CELL} px`],
    ["Player body", `${R.BODY_H} px tall, ${R.HALF_W} px half width at the feet`],
    ["Gravity", `${R.GRAVITY}/16 px per frame, per frame`],
    ["Jump", `start speed ${R.JUMP_VY}/16 px per frame (measured on the engine, the feet rise ${J.peak} px: a ledge ${J.ledge} px up is reachable, ${J.ledge + R.CELL} px is not)`],
    ["Fastest fall", `${R.MAX_FALL}/16 px per frame`],
    ["Ladders", `${R.CLIMB_SPEED}/16 px per frame (up and down on the stick)`],
    ["Climbing a 32 px crate", G.crateClimb === "push" ? `walking into an edge up to ${R.STEP_UP} px climbs it after ${R.PUSH_FRAMES} frames` : "only by jumping: walking into it does nothing"],
    ["Run", `a second tap toward the same side within ${R.RUN_TAP_FRAMES} frames`],
    ["Drop through a one-way ledge", `down + jump, for ${R.DROP_FRAMES} frames`],
    ["Camera", `moves forward; goes back at most ${R.BACKTRACK} px from the farthest point reached`],
    ["Machine gun", `${R.SHOTS_PER_PLAYER} shots per player on screen, speed ${R.SHOT_SPEED} px per frame, one every ${R.FIRE_EVERY} frames`],
    ["Knife", `${R.KNIFE_FRAMES} frames, reach ${R.KNIFE_REACH} px`],
    ["Bazooka", `${R.BAZOOKA_AMMO} rockets, ${R.BAZOOKA_FRAMES} frames each`],
    ["Enemies", `${G.enemyHp} hits unless the object gives its own; ${G.enemiesChase ? `chase a player on their floor within ${R.ENEMY_SIGHT} px` : "keep their patrol and turn at its ends"}; ${G.enemiesShoot ? `fire every ${R.ENEMY_FIRE_EVERY} frames at ${R.ENEMY_SHOT_SPEED} px per frame` : "never shoot"}; touching one ${G.touchHurts ? "hurts" : "does not hurt"}`],
    ["Crates and breakable walls", `${R.CRATE_HP} and ${R.BREAKABLE_HP} hits (a rocket counts 9, a knife 2); a crate whose object says breakable: false never breaks from shots; crates resting on a broken crate with nothing else under them break too`],
    ["Lives", `${R.LIVES} by default (the DIP switch above wins); after a hit the player ${G.respawnOnHurt ? "comes back near the camera's left side" : "blinks in place"} and cannot be hurt for ${G.hurtFrames} frames`],
    ["Score", `crate ${G.crateScore}, enemy ${G.enemyScore}, rescued civilian ${G.rescueScore}`],
    ["The exit", `${G.exitNeedsEnemies ? "clears the level only with every enemy down; reaching it earlier shows a message (DEFEAT EVERY ENEMY) and the HUD shows ENEMY n, the enemies left" : "clears the level when a player reaches it"}; a door is drawn on it`],
    ["Start on a port past the game's players", G.extraPorts === "soon" ? "shows \"3P COMING SOON\" (or 4P) for 2 s; no credit is taken" : "does nothing; no credit is taken"],
    ["Opposite directions", "left with right, or up with down, held together count as neither (the core delivers them so)"],
    [`Moves (${has("moves.md") ? "docs/moves.md" : "docs/willy-maker/moves.md in the repository"})`, `crouch on Down (a ${R.CROUCH_H} px body that standing-height shots pass over, firing ${R.CROUCH_SHOT_Y} px up), crawl with Left or Right (1 px every 2 frames), a jump kick with Down + B2 in the air (2 hits once, ${R.KICK_REACH} px in front), land, turn, a thumbs up on a rescue, victory on the clear and a yawn after ${R.YAWN_AFTER} frames idle`],
    ["Double jump", G.doubleJump ? `on: B1 again in the air once per landing, vertical speed ${R.DOUBLE_JUMP_VY}` : "off"],
    ["Jet pack", G.jetpack ? `on: B1 held while falling (then while held), after gravity vertical speed −${R.JET_LIFT} up to ${R.JET_MAX_UP}, ${R.JET_FUEL} frames of fuel per landing` : "off"],
  ];
  for (const [k, v] of rows) line(`| ${k} | ${v} |`);
  line();

  for (const l of decisions(p, levels, G, has)) line(l);

  line("## Board limits");
  line();
  line(`- ${board.palettes.sprite} sprite palettes, ${board.palettes.play} for the play layer (scroll2, 16 px tiles) and ${board.palettes.far} for the far layer (scroll3, 32 px tiles); the HUD and text use scroll1 (8 px tiles).`);
  line(`- ${board.colors.perPalette} colors plus transparency per palette, per 16 × 16 tile or sprite zone. Colors are 12-bit: every channel a multiple of 17.`);
  line(`- Graphics ROM ${board.rom.graphicsBytes / 1048576} MB, program ROM ${board.rom.programBytes / 1048576} MB, ${board.sprites.perScreen} sprite table entries.`);
  line("- What this game uses now:");
  for (const m of board.meters(p)) line(`  - ${m.id}: ${m.unit === "bytes" ? `${(m.used / 1048576).toFixed(2)} of ${(m.max / 1048576).toFixed(0)} MB` : `${m.used} of ${m.max}`}`);
  line();

  line("## What is in this pack");
  line();
  line("```");
  line("PROMPT.md                 this brief");
  line(`project.json              the Willy Maker project (format in ${has("file-format.md") ? "docs/file-format.md" : "docs/willy-maker/file-format.md of the repository"})`);
  line("review.json               Willy Maker's checks at export (below)");
  line("levels/<id>.tmj           Tiled maps: far, play and collision tile layers and the objects layer (nothing else repeats the level as a whole picture)");
  line("levels/<id>/far-tiles.png the far layer (and the middle one merged in) cut into the board's 32 px tiles, each once; the .tmj's far layer places them");
  line("tilesets/<id>.png         each tileset, in board colors; tilesets/collision.png the tag tiles");
  line("characters/<id>/<anim>.png one strip per animation on magenta #FF00FF, feet on one line");
  line("characters/<id>/sheet.json frames (boxes in the strip), pivots, palette zones, fps, loop");
  line(`docs/                     ${["rom-README.md", "art-spec.md", "hardware.md", "story.md", "journal.md", "file-format.md", "moves.md"].filter(has).join(", ")}`);
  line("```");
  line();
  line(`Collision tags: ${TAGS.slice(1).join(", ")} (air is empty). Objects have unique names and the types and properties of docs/art-spec.md section 4. Every picture is already in board colors; transparency is alpha 0 (strips use magenta).`);
  if (notes.missingPictures?.length) line(`Pictures that were not in this browser and are missing from the pack: ${notes.missingPictures.join(", ")}. A missing tileset is a transparent picture of the right size, so the maps still open.`);
  line();

  line("## Willy Maker's checks at export");
  line();
  const listed = review.checks.filter((c) => c.severity !== "ok");
  line(review.ready ? `Ready: no errors, ${review.warnings} warning${review.warnings === 1 ? "" : "s"}.` : `${review.errors} error${review.errors === 1 ? "" : "s"}: fix them in Willy Maker before building.`);
  for (const c of listed) line(`- ${c.severity}: ${checkText(exportEn, c)}`);
  const passed = review.checks.filter((c) => c.severity === "ok");
  if (passed.length) line(`- Passed: ${passed.map((c) => checkText(exportEn, c)).join("; ")}.`);
  line();

  line("## Build it");
  line();
  line("1. Tools (macOS with Homebrew, as in docs/journal.md; Linux has the same packages): the m68k-elf cross compiler, binutils and z80asm, and **Node 22.18 or newer** (rom/tools imports TypeScript from frontend/packages/cps1).");
  line("   ```sh");
  line("   brew install m68k-elf-binutils m68k-elf-gcc z80asm");
  line("   ```");
  line(`2. Build the prototype once, to prove the toolchain: \`node rom/tools/build.mjs${layout.id === "slammast" ? "" : ` ${layout.id}`}\` writes rom/build/${layout.id}.zip.`);
  line("3. Extend rom/tools/build.mjs (art.mjs, level.mjs and rom/src/main.c) to read this pack instead of the prototype's level: the maps and collision from levels/*.tmj, the tiles from the tilesets and far-tiles.png (already cut and deduplicated; keep 15 colors per tile), the characters from their strips and sheet.json, the objects with their properties, the buttons, players and DIP switches above.");
  line("4. Get the core the device uses (`device core download`), build the frame capture tool and run the set in it:");
  line("   ```sh");
  line("   (cd backend-device && go build -o /tmp/framelab ./cmd/framelab)");
  line(`   /tmp/framelab capture -log -core ~/go-link-console/cores/mame2003_plus_libretro.dylib -rom rom/build/${layout.id}.zip -out /tmp/rom-run -frames 860 -script "60-63:coin 110-113:start 140-400:right"`);
  line("   ```");
  line("   (The core is a .so on Linux.) A set of your own bytes gives one \"WRONG CHECKSUMS\" line per file and still runs; the log must have no \"NOT FOUND\" or \"INCORRECT LENGTH\" line.");
  line(`5. Play it in a real go-link room: \`node rom/tools/room-test.mjs${layout.id === "slammast" ? "" : ` ${layout.id}`}\` (it needs the signalhub repository next to go-link).`);
  line();

  line("## Task");
  line();
  line(`Produce the ROM set (${layout.id}.zip) with rom/tools/build.mjs extended to read this pack. Keep within the budgets above and the rules of docs/hardware.md (watch its pitfalls: sprites wrap at 512 px, write board registers with plain stores). Test it with framelab and in a go-link room, and report what you changed and anything that needs a decision.`);
  line();
  return L.join("\n");
}

/** The review as review.json carries it (messages in English). */
export function reviewJson(p: Project, review: Review): string {
  const layout = layoutOf(p);
  return JSON.stringify(
    {
      format: 1,
      project: p.id,
      board: p.board.id,
      layout: layout.id,
      ready: review.ready,
      errors: review.errors,
      warnings: review.warnings,
      checks: review.checks.map((c) => ({ id: c.id, severity: c.severity, message: checkText(exportEn, c), params: c.params, ...(c.target ? { target: c.target } : {}), ...(c.fix ? { fix: c.fix } : {}) })),
    },
    null,
    2,
  );
}

/**
 * Builds the AI pack. Throws when the review has errors (they block it),
 * and a PackBuildError when the finished pack fails its build check.
 */
export async function buildAiPack(p: Project, opts: AiPackOptions): Promise<AiPack> {
  if (!opts.review.ready) throw new Error(`the review has ${opts.review.errors} error(s)`);
  const enc = new TextEncoder();
  const load = opts.loadAsset ?? (async (ref: string) => getAsset(ref));
  const decode = opts.decode ?? canvasDecode;
  const docs = opts.docs ?? (await loadRomDocs());
  const missing: string[] = [];
  const pictures = new Map<string, RgbaImage | null>();
  const picture = async (ref: AssetRef | null | undefined, what: string): Promise<RgbaImage | null> => {
    if (!ref) return null;
    if (pictures.has(ref)) return pictures.get(ref)!;
    let img: RgbaImage | null = null;
    const a = await load(ref);
    if (a) {
      try {
        img = a.type === "image/png" || (a.bytes[0] === 0x89 && a.bytes[1] === 0x50) ? await decodePng(a.bytes).catch(() => decode(a.bytes, a.type)) : await decode(a.bytes, a.type);
      } catch {
        img = null;
      }
    }
    if (!img) missing.push(what);
    pictures.set(ref, img);
    return img;
  };
  const files = new Map<string, Uint8Array>();
  const png = (name: string, img: RgbaImage) => files.set(name, encodePng(img.w, img.h, img.data));

  // tilesets, in board colors
  const tilesetImages = new Map<string, RgbaImage>();
  for (const ts of p.tilesets) {
    let img = await picture(ts.image, `tileset ${ts.id}`);
    if (!img) {
      // a blank picture of the right size keeps the maps whole (PROMPT.md lists it as missing)
      const columns = Math.max(1, Number(ts.columns) || 8);
      const rows = Math.max(1, Math.ceil((Number(ts.count) || 1) / columns));
      img = { w: columns * ts.tile, h: rows * ts.tile, data: new Uint8Array(columns * ts.tile * rows * ts.tile * 4) };
    }
    const board = toBoard(img);
    tilesetImages.set(ts.id, board);
    png(`tilesets/${safe(ts.id)}.png`, board);
  }
  png("tilesets/collision.png", collisionTileset());

  // levels
  for (const level of p.levels) {
    const dir = `levels/${safe(level.id)}`;
    const tiles = level.layers.filter((l): l is TileLayer => l.kind === "tiles");
    const tilesetOf = (l: TileLayer) => p.tilesets.find((t) => t.id === l.tileset) ?? p.tilesets.find((t) => t.tile === l.grid);
    const draw = (layers: TileLayer[]) => {
      const img: RgbaImage = { w: level.size.w, h: level.size.h, data: new Uint8Array(level.size.w * level.size.h * 4) };
      let any = false;
      for (const l of layers) {
        const ts = tilesetOf(l);
        const src = ts ? tilesetImages.get(ts.id) : undefined;
        if (ts && src && drawTiles(img, level, l, ts, src)) any = true;
      }
      return any ? img : null;
    };
    // the far layer (with the middle one merged in) as the board's 32 px tiles; the play layer is already tiles
    const farPic = draw(tiles.filter((l) => l.id === "far" || l.id === "mid"));
    const far = farPic ? cutTiles(farPic, FAR_TILE) : null;
    if (far) png(`${dir}/far-tiles.png`, far.sheet);
    const play = playLayer(level);
    const ts = play ? tilesetOf(play) : undefined;
    const tsImg = ts ? tilesetImages.get(ts.id) : undefined;
    const map = levelToTiled(level, p, {
      farTiles: far ? { path: `${safe(level.id)}/far-tiles.png`, w: far.sheet.w, h: far.sheet.h, tile: FAR_TILE, columns: far.columns, count: far.count, cols: far.cols, rows: far.rows, cells: far.cells } : null,
      playTileset: ts && tsImg ? { path: `../tilesets/${safe(ts.id)}.png`, w: tsImg.w, h: tsImg.h } : null,
      collisionTileset: "../tilesets/collision.png",
    });
    files.set(`${dir}.tmj`, enc.encode(JSON.stringify(map, null, 1)));
  }

  // characters: a strip per animation and the sheet
  for (const ch of p.characters) {
    const dir = `characters/${safe(ch.id)}`;
    const atlas = await picture(ch.sheet, `character ${ch.name || ch.id}`);
    const board = atlas ? toBoard(atlas) : null;
    const anims: Record<string, unknown> = {};
    for (const [name, anim] of Object.entries(ch.anims ?? {})) {
      const entry: Record<string, unknown> = { fps: anim.fps, loop: anim.loop, frames: anim.frames };
      if (board && anim.frames.length) {
        const strip = animStrip(ch, anim.frames, board);
        png(`${dir}/${safe(name)}.png`, strip.img);
        entry.file = `${safe(name)}.png`;
        entry.frames = strip.frames;
      }
      anims[name] = entry;
    }
    const needs = REQUIRED_ANIMS[ch.role] ?? ["idle"];
    const sheet = {
      id: ch.id,
      name: ch.name,
      role: ch.role,
      height: ch.height,
      background: "#FF00FF",
      picture: board ? "strips" : null,
      required: needs,
      swapColors: ch.swapColors,
      palettes: [...new Set(ch.frames.flatMap((f) => f.zones ?? []))].map((id) => p.palettes.find((x) => x.id === id) ?? { id, missing: true }),
      anims,
    };
    files.set(`${dir}/sheet.json`, enc.encode(JSON.stringify(sheet, null, 2)));
  }

  for (const [name, text] of Object.entries(docs)) files.set(`docs/${name}`, enc.encode(text));
  files.set("project.json", enc.encode(JSON.stringify(p, null, 2)));
  files.set("review.json", enc.encode(reviewJson(p, opts.review)));
  const prompt = buildPrompt(p, opts.review, { missingPictures: missing }, docs);

  const names = [...files.keys()].sort();
  const entries: ZipEntry[] = [{ name: "PROMPT.md", data: enc.encode(prompt) }, ...names.map((name) => ({ name, data: files.get(name)! }))];
  const zip = await writeZip(entries, { date: PACK_DATE });
  // level 2: the pack, read back from its own zip, must be what the prompt promises
  if (!opts.skipCheck) {
    const problems = await checkAiPack(await readZip(zip));
    if (problems.length) throw new PackBuildError(problems);
  }
  return { zip, prompt, files: entries.map((e) => e.name) };
}
