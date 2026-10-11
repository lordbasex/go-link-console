// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
// The QA run of experiment 1's harness (docs/experiments/harness.md, "QA
// run"; verdict task T-08): plays a ROM set in the board model with a set
// of adversarial players and a seeded 4-port fuzz, checks invariants on the
// lab state every frame, and turns every finding into a short input script
// (ddmin.mjs) that replays on the simulator and on the real core.
//
//   node rom/tools/lab/qa.mjs ZIP --out DIR [options]
//
// Options:
//   --players LIST     newcomer,masher,run-past,skipper,shooter (the default: all)
//   --frames N         frames per adversarial player (default 7200, 2 minutes)
//   --fuzz-minutes N   minutes of seeded 4-port fuzz (default 2; 0 for none)
//   --seed S           the fuzz's seed (default 1)
//   --exit-rule R      auto (the lab flags: an exit needs every enemy down
//                      unless LAB_FLAG_EXIT_TOUCH is set), all or touch
//   --patrol PX        how far an enemy may walk from where it appeared (160)
//   --no-minimize      keep the findings' full inputs only
//   --min-seconds S    the longest minimization of one finding (default 30)
//   --min-budget S     the longest minimization of all findings (default 120)
//   --symbols FILE     the symbol map (default: <set>.symbols.json next to the zip)
//   --quiet            no progress lines
//
// Outputs in DIR: qa.json (runs, findings, the verdict), qa.md (the same,
// readable), findings/NN-KIND.inputs.json (every input up to the finding)
// and findings/NN-KIND.min.json (the minimized script). Every script is a
// harness input script: run.mjs --script and device romtest --input read
// it. The exit code is 0 whenever the run itself worked; the verdict is
// qa.json "ok" (no high finding).

import fs from "node:fs";
import path from "node:path";
import { BUTTONS, Machine, formatScript } from "./lab.mjs";
import { actionButtons, mapText } from "./run.mjs";
import { createBot } from "./bot.mjs";
import { minimize } from "./ddmin.mjs";

export const PLAYERS = ["newcomer", "masher", "run-past", "skipper", "shooter"];
export const SCREEN = { w: 384, h: 224 };
const CELL = { EMPTY: 0, SOLID: 1, ONEWAY: 2, LADDER: 3, CRATE: 4 };
const blocks = (c) => c === CELL.SOLID || c === CELL.CRATE;

/** Every finding kind, its severity and what it means. */
export const KINDS = {
  fault: ["high", "the 68000 faulted"],
  frozen: ["high", "the lab state's frame counter stopped for 2 s (the game hangs)"],
  stuck: ["high", "30 s of trying (a direction held) while playing without progress: no new place, score, hit or rescue (a soft-lock)"],
  "clear-enemies-alive": ["high", "SECTION CLEAR while an enemy is alive and the exit needs every enemy down"],
  "clear-civilians-left": ["high", "SECTION CLEAR while a civilian waits and the section needs every rescue"],
  "lab-lost": ["medium", "the lab state disappeared after the game had one"],
  "nobody-alive": ["medium", "playing for 10 s with nobody alive"],
  "body-in-solid": ["medium", "a player's body inside a solid or crate cell"],
  "feet-in-floor": ["medium", "a player's feet inside a solid floor"],
  "standing-on-air": ["medium", "a player on the ground with nothing under the feet"],
  "player-outside-map": ["medium", "a player outside the level"],
  "camera-outside": ["medium", "the camera shows outside the level"],
  "energy-over-start": ["medium", "a player's energy above what the player started with"],
  "score-down": ["medium", "a player's score went down"],
  "enemy-in-solid": ["medium", "an enemy inside a solid or crate cell"],
  "enemy-outside-map": ["medium", "an enemy outside the level"],
  "enemy-off-patrol": ["medium", "an enemy far from where it appeared (outside its patrol)"],
  "player-off-screen": ["low", "an active player outside the picture"],
  "credits-over-9": ["low", "more than 9 credits"],
};
export const HIGH = new Set(Object.keys(KINDS).filter((k) => KINDS[k][0] === "high"));

// ---------------------------------------------------------------- inputs

const BIT = Object.fromEntries(BUTTONS.map((b, i) => [b, 1 << i]));

/** The buttons of every port on every frame, as bit masks; steps() gives the run-length script. */
export class InputLog {
  constructor() {
    this.ports = [[], [], [], []];
    this.n = 0;
  }

  set(f, ports) {
    for (let p = 0; p < 4; p++) {
      let m = 0;
      for (const b of ports[p]) m |= BIT[b];
      this.ports[p][f] = m;
    }
    this.n = Math.max(this.n, f + 1);
  }

  /** Run-length steps of frames 0 .. n-1, one button per step (as run.mjs writes inputs.json). */
  steps(n = this.n) {
    const out = [];
    for (let p = 0; p < 4; p++)
      for (let i = 0; i < BUTTONS.length; i++) {
        let from = -1;
        for (let f = 0; f <= n; f++) {
          const on = f < n && (this.ports[p][f] ?? 0) & (1 << i);
          if (on && from < 0) from = f;
          else if (!on && from >= 0) {
            out.push({ from, to: f - 1, port: p + 1, buttons: [BUTTONS[i]] });
            from = -1;
          }
        }
      }
    return out.sort((a, b) => a.from - b.from || a.port - b.port || a.buttons[0].localeCompare(b.buttons[0]));
  }
}

/** Per-frame masks from a script's steps (frames 0 .. n-1). */
export function masksOf(steps, n) {
  const ports = [new Uint16Array(n), new Uint16Array(n), new Uint16Array(n), new Uint16Array(n)];
  for (const s of steps) {
    let m = 0;
    for (const b of s.buttons ?? []) m |= BIT[b];
    const p = ports[(s.port ?? 1) - 1];
    for (let f = Math.max(0, s.from); f <= Math.min(s.to, n - 1); f++) p[f] |= m;
  }
  return ports;
}

/** The button sets of one frame from masks. */
export function portsOf(masks, f) {
  return masks.map((p) => {
    const set = new Set();
    const m = p[f] ?? 0;
    if (m) for (let i = 0; i < BUTTONS.length; i++) if (m & (1 << i)) set.add(BUTTONS[i]);
    return set;
  });
}

// ---------------------------------------------------------------- the checker

/**
 * The invariants, checked on the lab state after every frame. check()
 * returns the raw findings of that frame ({ kind, player, x, y, detail });
 * all its memory is in this.s (a plain object) so a minimizer can copy it.
 */
export class Checker {
  constructor({ exitRule = "auto", patrol = 160, stuckFrames = 1800, nobodyFrames = 600, frozenFrames = 120 } = {}) {
    this.o = { exitRule, patrol, stuckFrames, nobodyFrames, frozenFrames };
    this.s = { hadLab: false, lostOpen: false, labFrame: -1, frozen: 0, frozenOpen: false, credits9: false, game: null };
  }

  copyState() {
    return structuredClone(this.s);
  }

  setState(s) {
    this.s = structuredClone(s);
  }

  newGame(n) {
    this.s.game = { visited: {}, lastProgress: n, effort: 0, stuckOpen: false, nobodySince: null, players: [null, null, null, null], enemies: [], civ: [], cleared: false, active: 0, persist: {} };
  }

  /** A condition that must hold `need` frames in a row before it counts. */
  persist(key, on, need) {
    const p = this.s.game.persist;
    if (!on) {
      delete p[key];
      return false;
    }
    p[key] = (p[key] ?? 0) + 1;
    return p[key] === need;
  }

  /**
   * The findings of frame n. colOf() gives the collision map (read only when
   * needed); ports are the buttons held on that frame (a soft-lock needs a
   * player who tries: frames with no direction held do not count).
   */
  check(n, lab, colOf, ports = null) {
    const s = this.s;
    const out = [];
    const add = (kind, player, x, y, detail = {}) => out.push({ kind, player, x, y, frame: n, detail });
    if (!lab) {
      if (s.hadLab && !s.lostOpen) {
        s.lostOpen = true;
        add("lab-lost", null, null, null);
      }
      return out;
    }
    s.hadLab = true;
    s.lostOpen = false;
    if (lab.frame === s.labFrame) {
      if (++s.frozen === this.o.frozenFrames && !s.frozenOpen) {
        s.frozenOpen = true;
        add("frozen", null, lab.cam.x + SCREEN.w / 2, lab.cam.y + SCREEN.h / 2, { labFrame: lab.frame, mode: lab.mode });
      }
    } else {
      s.frozen = 0;
      s.frozenOpen = false;
    }
    s.labFrame = lab.frame;
    if (lab.credits > 9 && !s.credits9) {
      s.credits9 = true;
      add("credits-over-9", null, null, null, { credits: lab.credits });
    }
    const inGame = lab.mode === "playing" || lab.mode === "clear";
    if (!inGame) {
      s.game = null;
      return out;
    }
    if (!s.game) this.newGame(n);
    const g = s.game;
    const W = lab.level.w, H = lab.level.h;
    const progress = () => {
      g.lastProgress = n;
      g.effort = 0;
      g.stuckOpen = false;
    };
    if (lab.cam.x < 0 || lab.cam.y < 0 || lab.cam.x + SCREEN.w > W || lab.cam.y + SCREEN.h > H) add("camera-outside", null, lab.cam.x, lab.cam.y, { cam: lab.cam, level: lab.level });

    const col = colOf();
    const cell = (x, y) => {
      if (!col) return CELL.EMPTY;
      const c = Math.floor(x / 16), r = Math.floor(y / 16);
      if (c < 0 || r < 0 || c >= col.cols || r >= col.rows) return -1;
      return col.cells[r * col.cols + c];
    };
    let alive = 0, active = 0;
    lab.players.forEach((p, i) => {
      const id = i + 1;
      const st = g.players[i];
      if (!p.active) {
        g.players[i] = null;
        return;
      }
      active++;
      if (!st) g.players[i] = { start: p.energy, score: p.score };
      const me = g.players[i];
      if (p.score > me.score) progress();
      if (p.score < me.score) add("score-down", id, p.x, p.y, { from: me.score, to: p.score });
      me.score = p.score;
      if (p.energy > me.start) add("energy-over-start", id, p.x, p.y, { start: me.start, energy: p.energy });
      if (p.state === "dead" || p.state === "off") return;
      alive++;
      const key = `${id}:${p.x >> 4},${p.y >> 4}`;
      if (!g.visited[key]) {
        g.visited[key] = 1;
        progress();
      }
      if (p.x < 0 || p.x >= W || p.y < 0 || p.y > H) {
        add("player-outside-map", id, p.x, p.y, { state: p.state });
        return;
      }
      if (p.x + 8 < lab.cam.x || p.x - 8 > lab.cam.x + SCREEN.w || p.y < lab.cam.y || p.y - 40 > lab.cam.y + SCREEN.h) add("player-off-screen", id, p.x, p.y, { cam: lab.cam, state: p.state });
      if (!col) return;
      const mid = blocks(cell(p.x, p.y - 24));
      const feet = !mid && blocks(cell(p.x, p.y - 1));
      if (this.persist(`body${id}`, mid, 2)) add("body-in-solid", id, p.x, p.y, { state: p.state, cell: cell(p.x, p.y - 24) });
      if (this.persist(`feet${id}`, feet, 2)) add("feet-in-floor", id, p.x, p.y, { state: p.state, floorTop: (p.y >> 4) << 4 });
      let air = false;
      if (p.ground && !p.climbing && p.y % 16 === 0) {
        air = true;
        for (let dx = -6; dx <= 6; dx += 3) if (cell(p.x + dx, p.y) !== CELL.EMPTY) air = false;
      }
      if (this.persist(`air${id}`, air, 3)) add("standing-on-air", id, p.x, p.y, { state: p.state });
    });
    if (active > g.active) progress(); // somebody joined
    g.active = active;

    lab.enemies.forEach((e, i) => {
      const prev = g.enemies[i];
      if (prev && (e.hp < prev.hp || (!e.alive && prev.alive))) progress();
      g.enemies[i] = { alive: e.alive, hp: e.hp, home: prev?.home ?? (e.alive ? { x: e.x, y: e.y } : null) };
      if (!e.alive) return;
      const home = g.enemies[i].home ?? (g.enemies[i].home = { x: e.x, y: e.y });
      if (e.x < 0 || e.x >= W || e.y < 0 || e.y > H) return add("enemy-outside-map", null, e.x, e.y, { enemy: i });
      if (this.persist(`esolid${i}`, blocks(cell(e.x, e.y - 8)), 2)) add("enemy-in-solid", null, e.x, e.y, { enemy: i, state: e.state });
      if (Math.abs(e.x - home.x) > this.o.patrol || Math.abs(e.y - home.y) > 8) add("enemy-off-patrol", null, e.x, e.y, { enemy: i, home });
    });
    lab.civilians.forEach((c, i) => {
      if (c.rescued && !g.civ[i]) progress();
      g.civ[i] = c.rescued;
    });

    if (lab.sectionClear && !g.cleared) {
      g.cleared = true;
      progress();
      const lead = lab.players.find((p) => p.active) ?? { x: null, y: null };
      const needAll = this.o.exitRule === "all" || (this.o.exitRule === "auto" && lab.flags.exit && !lab.flags.exitTouch);
      const left = lab.enemies.map((e, i) => (e.alive ? i : -1)).filter((i) => i >= 0);
      if (needAll && left.length) add("clear-enemies-alive", null, lead.x, lead.y, { alive: left });
      const waiting = lab.civilians.map((c, i) => (c.rescued ? -1 : i)).filter((i) => i >= 0);
      if (lab.flags.rescueAll && waiting.length) add("clear-civilians-left", null, lead.x, lead.y, { waiting });
    }

    if (lab.mode === "playing") {
      if (alive) {
        g.nobodySince = null;
        const trying = !ports || lab.players.some((p, i) => p.active && p.state !== "dead" && p.state !== "off" && ["left", "right", "up", "down"].some((b) => ports[i].has(b)));
        if (trying) g.effort++;
        if (!g.stuckOpen && g.effort >= this.o.stuckFrames) {
          g.stuckOpen = true;
          const i = lab.players.findIndex((p) => p.active && p.state !== "dead" && p.state !== "off");
          const p = lab.players[i];
          const needAll = this.o.exitRule === "all" || (this.o.exitRule === "auto" && lab.flags.exit && !lab.flags.exitTouch);
          const behind = lab.enemies.map((e, k) => ({ enemy: k, x: e.x, y: e.y, alive: e.alive })).filter((e) => e.alive && e.x < lab.cam.x).map(({ alive, ...e }) => e);
          const why = needAll && behind.length ? "an enemy the exit needs is behind the camera" : "no headway";
          add("stuck", i + 1, p.x, p.y, { why, since: g.lastProgress, cam: lab.cam, enemiesAlive: lab.enemies.filter((e) => e.alive).map((e) => ({ x: e.x, y: e.y })), behind });
        }
      } else {
        g.nobodySince ??= n;
        if (n - g.nobodySince === this.o.nobodyFrames) add("nobody-alive", null, lab.cam.x + SCREEN.w / 2, lab.cam.y + SCREEN.h / 2);
        g.lastProgress = n; // nobody to blame for not moving
        g.effort = 0;
      }
    }
    return out;
  }
}

/** The finding's 16 px cell, the key that deduplicates findings of one kind. */
export const cellKey = (f) =>
  f.kind === "stuck" && f.detail?.why?.includes("behind") ? `stuck@behind:${f.detail.behind.map((e) => e.enemy).join(",")}` : cellKey16(f);
/** A soft-lock behind the camera is one finding per set of enemies left behind, wherever the players stand. */
const cellKey16 = (f) => `${f.kind}@${f.x === null ? "-" : f.x >> 4},${f.y === null ? "-" : (f.y - 1) >> 4}`; // feet y 416 stand in the row above 416

// ---------------------------------------------------------------- players

/** At most this many cells are kept per kind and player run (the rest are counted). */
const PER_KIND = 5;
const PRELUDE = { coin: 120, start: 150, hold: 6, giveUp: 900 };
const on = (t, period, width, offset = 0) => (t + offset) % period < width;

/** Coin and Start on these ports (the second port's 15 frames later), until the game plays. */
function prelude(f, ports, which) {
  which.forEach((p, k) => {
    const d = k * 15;
    if (f >= PRELUDE.coin + d && f < PRELUDE.coin + d + PRELUDE.hold) ports[p - 1].add("coin");
    if (f >= PRELUDE.start + d && f < PRELUDE.start + d + PRELUDE.hold) ports[p - 1].add("start");
  });
}

/** A frame-driven player: `pattern(t, ports)` from the first playing frame (t = 0). */
function patterned(name, joins, pattern, what) {
  return {
    name,
    what,
    create() {
      let start = null;
      return (f, lab, ports) => {
        if (start === null && lab?.mode === "playing") start = f;
        if (start === null) {
          if (f >= PRELUDE.giveUp) throw new Error(`the game did not start by frame ${PRELUDE.giveUp}`);
          prelude(f, ports, joins);
          return;
        }
        if (lab?.mode === "playing") pattern(f - start, ports);
      };
    },
  };
}

/**
 * The skipper: the route bot with every ladder hidden from its map, so it
 * follows the route but never climbs; once it makes no headway for 6 s it
 * gets the real map and tries to go back (left and up) to what it skipped.
 */
function skipper() {
  return {
    name: "skipper",
    what: "follows the route bot's path but never climbs a ladder; after 6 s without headway it sees the real map and tries to walk back and climb",
    create() {
      let bot = createBot({});
      let phase = 1, action = "wait", last = null, windowAt = -1, sentMap = null, anchor = null, started = false;
      const EVERY = 6;
      return (f, lab, ports, m) => {
        if (lab?.mode !== "playing") {
          if (!started) {
            if (f >= PRELUDE.giveUp) throw new Error(`the game did not start by frame ${PRELUDE.giveUp}`);
            prelude(f, ports, [1]);
          }
          windowAt = f + 1;
          return;
        }
        started = true;
        const me = lab.players[0];
        if (f === windowAt) {
          if (phase === 1 && me.active) {
            if (!anchor || Math.abs(me.x - anchor.x) >= 16 || Math.abs(me.y - anchor.y) >= 16) anchor = { x: me.x, y: me.y, f };
            else if (f - anchor.f >= 360) {
              phase = 2;
              bot = createBot({});
              sentMap = null;
            }
          }
          const msg = { type: "state", frame: f, port: 1, last_action: action, lab };
          const col = m.collisionFast(lab);
          if (col) {
            let text = mapText(col);
            if (phase === 1) text = text.map((r) => r.replaceAll("H", "."));
            const key = text.join("\n");
            if (key !== sentMap) {
              msg.map = { cols: col.cols, rows: col.rows, cell: 16, rows_text: text };
              sentMap = key;
            }
          }
          const ans = bot.handle(msg);
          if (ans?.error) throw new Error(ans.error);
          last = action;
          action = ans.action;
          windowAt = f + EVERY;
        }
        const k = f - (windowAt - EVERY);
        for (const b of actionButtons(action, k, EVERY, { facing: me.facing || 1, last })) ports[0].add(b);
      };
    },
  };
}

export const PLAYER_DEFS = {
  newcomer: patterned("newcomer", [1], (t, p) => {
    p[0].add("right");
    if (on(t, 30, 4)) p[0].add("b2");
    if (on(t, 90, 4, 45)) p[0].add("b1");
  }, "one player holds right, fires every 30 frames and jumps every 90"),
  masher: patterned("masher", [1, 2], (t, p) => {
    for (const q of [p[0], p[1]]) {
      q.add("right");
      if (on(t, 20, 4)) q.add("b2");
      if (on(t, 20, 4, 10)) q.add("b1");
    }
  }, "two players hold right, fire and jump every 20 frames"),
  "run-past": patterned("run-past", [1], (t, p) => {
    p[0].add("right");
    if (on(t, 20, 4)) p[0].add("b1");
  }, "one player holds right and jumps every 20 frames, never fires"),
  skipper: skipper(),
  shooter: patterned("shooter", [1], (t, p) => {
    if (on(t, 8, 4)) p[0].add("b2");
    if (t >= 600) p[0].add(on(t, 240, 10) ? "left" : "right");
  }, "one player fires from the start for 10 s standing, then walks right firing, turning back for 10 frames every 4 s"),
};

/** mulberry32: a small seeded generator (the same one as bot.mjs). */
function rng(seed) {
  let s = seed >>> 0;
  return () => {
    s = (s + 0x6d2b79f5) >>> 0;
    let t = s;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

/**
 * The seeded 4-port fuzz: every port holds a random direction (P1 mostly
 * right) and random buttons for 3 to 40 frames, never opposite directions;
 * each port inserts a coin and presses Start every 40 s (staggered) so
 * players keep joining and continuing.
 */
export function fuzzer(seed) {
  return {
    name: `fuzz`,
    what: `seeded 4-port random input (seed ${seed}): P1 mostly right, holds of 3-40 frames, coin and Start on every port every 40 s`,
    create() {
      const r = rng(seed);
      const hold = [0, 0, 0, 0].map(() => ({ until: -1, set: [] }));
      const DIRS = [[], ["left"], ["right"], ["up"], ["down"], ["right", "up"], ["left", "up"], ["right", "down"], ["left", "down"]];
      return (f, lab, ports) => {
        for (let p = 0; p < 4; p++) {
          const h = hold[p];
          if (f > h.until) {
            const set = [];
            const d = r();
            if (p === 0) set.push(...(d < 0.5 ? ["right"] : d < 0.6 ? ["left"] : d < 0.7 ? ["up"] : d < 0.8 ? ["down"] : d < 0.85 ? ["right", "up"] : []));
            else set.push(...DIRS[Math.floor(d * DIRS.length)]);
            if (r() < 0.3) set.push("b1");
            if (r() < 0.35) set.push("b2");
            if (r() < 0.1) set.push("b3");
            if (r() < 0.02) set.push("start");
            if (r() < 0.01) set.push("coin");
            h.set = set;
            h.until = f + 3 + Math.floor(r() * 38);
          }
          if (f >= 100) for (const b of h.set) ports[p].add(b);
          const k = (f - 100 + p * 600) % 2400;
          if (f >= 100 && k < 6) ports[p].add("coin");
          if (f >= 100 && k >= 30 && k < 36) ports[p].add("start");
        }
      };
    },
  };
}

// ---------------------------------------------------------------- a game

/**
 * Plays one player definition: every frame the controller fills the
 * ports, the board runs it, the checker looks at the lab state. Stops at
 * `frames`, 120 frames after the section clears, 60 after game over, and
 * (stopOnStuck) at the first stuck finding.
 */
export async function playOne(zip, def, { frames, symbols, checker, stopOnStuck = true, endOnGameOver = true }) {
  const t0 = performance.now();
  const m = await Machine.open(zip, { symbols });
  const ctl = def.create();
  const log = new InputLog();
  const raw = []; // the first finding of each kind and cell, with its inputs (at most PER_KIND cells per kind)
  const seen = new Map(); // cell key -> finding
  const perKind = {};
  const keep = (x, n) => {
    perKind[x.kind] = (perKind[x.kind] ?? 0) + 1;
    const key = cellKey(x);
    const had = seen.get(key);
    if (had) return had.count++;
    if (raw.filter((r) => r.kind === x.kind).length >= PER_KIND) return;
    const f = { ...x, count: 1, inputs: log.steps(n) };
    seen.set(key, f);
    raw.push(f);
  };
  // energyLost: the hits its players took (T-15: how hard the game is on someone who plays it plainly)
  const sum = { name: def.name, what: def.what, frames: 0, startFrame: null, cleared: false, clearFrame: null, gameOver: false, gameOverFrame: null, energyLost: 0, error: null };
  let lab = null;
  let energy = [null, null, null, null];
  let stopAt = frames;
  for (let f = 0; f < stopAt; f++) {
    const ports = [new Set(), new Set(), new Set(), new Set()];
    try {
      ctl(f, lab, ports, m);
    } catch (e) {
      sum.error = e.message;
      break;
    }
    log.set(f, ports);
    try {
      m.step(ports);
    } catch (e) {
      keep({ kind: "fault", player: null, x: null, y: null, frame: f + 1, detail: { error: e.message } }, f + 1);
      sum.frames = f + 1;
      break;
    }
    const n = m.frame;
    lab = m.labFast();
    for (const x of checker.check(n, lab, () => m.collisionFast(lab), ports)) {
      keep(x, n);
      if (x.kind === "stuck" && stopOnStuck) stopAt = Math.min(stopAt, f + 1);
    }
    if (lab) {
      lab.players.forEach((p, i) => {
        const was = energy[i];
        if (p.active && was !== null && p.energy < was) sum.energyLost += was - p.energy;
        if (!p.active && was !== null && was > 0 && p.state === "dead") sum.energyLost += was;
        energy[i] = p.active ? p.energy : null;
      });
      if (lab.mode === "playing" && sum.startFrame === null) sum.startFrame = n;
      if (lab.sectionClear && !sum.cleared) {
        sum.cleared = true;
        sum.clearFrame = n;
        stopAt = Math.min(stopAt, f + 121);
      }
      if (lab.mode === "game_over" && !sum.gameOver) {
        sum.gameOver = true;
        sum.gameOverFrame = n;
        if (endOnGameOver) stopAt = Math.min(stopAt, f + 61);
      }
    }
    sum.frames = n;
  }
  sum.wallMs = Math.round(performance.now() - t0);
  sum.findings = perKind;
  return { sum, raw };
}

// ---------------------------------------------------------------- the whole run

export async function qa(o) {
  const t0 = performance.now();
  const say = (s) => !o.quiet && process.stderr.write(`[qa] ${s}\n`);
  const dir = path.join(o.out, "findings");
  fs.rmSync(dir, { recursive: true, force: true });
  fs.mkdirSync(dir, { recursive: true });
  const checkerOpts = { exitRule: o.exitRule, patrol: o.patrol };
  const defs = o.players.map((p) => {
    if (!PLAYER_DEFS[p]) throw new Error(`unknown player ${p} (${PLAYERS.join(", ")})`);
    return PLAYER_DEFS[p];
  });
  const runs = [];
  const found = new Map(); // cell key -> finding
  const take = (run, raw) => {
    for (const x of raw) {
      const key = cellKey(x);
      const had = found.get(key);
      if (had) {
        had.count += x.count;
        if (!had.runs.includes(run)) had.runs.push(run);
        // keep the instance with the fewest inputs: the cheapest to minimize
        if (x.inputs.length < had.inputs.length) Object.assign(had, { ...x, count: had.count, runs: had.runs, run, severity: had.severity, key });
        continue;
      }
      found.set(key, { ...x, severity: KINDS[x.kind][0], run, runs: [run], key });
    }
  };
  for (const def of defs) {
    say(`${def.name}: ${def.what}`);
    const { sum, raw } = await playOne(o.zip, def, { frames: o.frames, symbols: o.symbols, checker: new Checker(checkerOpts) });
    take(def.name, raw);
    runs.push(sum);
    say(`  ${sum.frames} frames in ${(sum.wallMs / 1000).toFixed(1)} s${sum.cleared ? `, cleared at ${sum.clearFrame}` : ""}${sum.error ? `, error: ${sum.error}` : ""}; ${raw.length} raw findings`);
  }
  if (o.fuzzMinutes > 0) {
    const def = fuzzer(o.seed);
    say(`fuzz: ${o.fuzzMinutes} min, seed ${o.seed}`);
    const { sum, raw } = await playOne(o.zip, def, { frames: Math.round(o.fuzzMinutes * 3600), symbols: o.symbols, checker: new Checker(checkerOpts), stopOnStuck: false, endOnGameOver: false });
    sum.seed = o.seed;
    take("fuzz", raw);
    runs.push(sum);
    say(`  ${sum.frames} frames in ${(sum.wallMs / 1000).toFixed(1)} s; ${raw.length} raw findings`);
  }

  // order: high first, then by frame; number them and write their inputs
  const order = { high: 0, medium: 1, low: 2 };
  const findings = [...found.values()].sort((a, b) => order[a.severity] - order[b.severity] || a.frame - b.frame);
  findings.forEach((x, i) => {
    x.id = `${String(i + 1).padStart(2, "0")}-${x.kind}`;
    const script = { name: `qa ${x.id}: ${x.kind} (${x.run})`, frames: x.frame, checkpoints: [x.frame], steps: x.inputs };
    fs.writeFileSync(path.join(dir, `${x.id}.inputs.json`), formatScript(script));
    x.inputsFile = `findings/${x.id}.inputs.json`;
    x.inputSteps = x.inputs.length;
  });

  // minimize: every high finding, then the first finding of each other kind, within the budget
  const minT0 = performance.now();
  if (o.minimize) {
    const done = new Set();
    for (const x of findings) {
      if (x.severity !== "high" && done.has(x.kind)) continue;
      const left = o.minBudget - (performance.now() - minT0) / 1000;
      if (left <= 1) {
        say("minimization budget spent");
        break;
      }
      done.add(x.kind);
      say(`minimizing ${x.id} (${x.inputs.length} steps, frame ${x.frame})`);
      const r = await minimize({ zip: o.zip, symbols: o.symbols, steps: x.inputs, target: x, checkerOpts, maxSeconds: Math.min(o.minSeconds, left) });
      x.minimized = { ok: r.ok, steps: r.steps.length, frame: r.frame, runs: r.runs, seconds: r.seconds, complete: r.complete, file: null };
      if (r.ok) {
        const script = { name: `qa ${x.id}: ${x.kind}, minimized`, note: `${KINDS[x.kind][1]}; found by ${x.run} at frame ${x.frame}, reproduced at frame ${r.frame}`, frames: r.frame + 60, checkpoints: [r.frame], expect: expectFor(x, r.found), steps: r.steps };
        fs.writeFileSync(path.join(dir, `${x.id}.min.json`), formatScript(script));
        x.minimized.file = `findings/${x.id}.min.json`;
      }
      say(`  ${r.ok ? `${r.steps.length} steps (reproduced at frame ${r.frame})` : "could not reproduce"}, ${r.runs} replays in ${r.seconds.toFixed(1)} s${r.complete ? "" : " (budget)"}`);
    }
  }

  const high = findings.filter((x) => x.severity === "high");
  const report = {
    zip: path.resolve(o.zip),
    ok: high.length === 0,
    verdict: high.length ? `${high.length} high finding(s): ${[...new Set(high.map((x) => x.kind))].join(", ")}` : "no high finding",
    options: { players: o.players, frames: o.frames, fuzzMinutes: o.fuzzMinutes, seed: o.seed, exitRule: o.exitRule, patrol: o.patrol, minimize: o.minimize },
    framesPlayed: runs.reduce((a, r) => a + r.frames, 0),
    cleared: runs.some((r) => r.cleared),
    runs,
    counts: Object.fromEntries(["high", "medium", "low"].map((s) => [s, findings.filter((x) => x.severity === s).length])),
    findings: findings.map(({ inputs, key, ...x }) => ({ ...x, what: KINDS[x.kind][1] })),
    seconds: { play: +((minT0 - t0) / 1000).toFixed(1), minimize: +((performance.now() - minT0) / 1000).toFixed(1), total: +((performance.now() - t0) / 1000).toFixed(1) },
  };
  fs.writeFileSync(path.join(o.out, "qa.json"), JSON.stringify(report, null, 1) + "\n");
  fs.writeFileSync(path.join(o.out, "qa.md"), markdown(report));
  return report;
}

/** Expectations a minimized script checks on the simulator (the core ignores them). */
function expectFor(x, got) {
  const f = got.frame;
  if (x.kind === "clear-enemies-alive") return [{ frame: f, path: "sectionClear", equals: true }, ...got.detail.alive.map((i) => ({ frame: f, path: `enemies.${i}.alive`, equals: true }))];
  if (x.kind === "clear-civilians-left") return [{ frame: f, path: "sectionClear", equals: true }, ...got.detail.waiting.map((i) => ({ frame: f, path: `civilians.${i}.rescued`, equals: false }))];
  if (x.kind === "stuck") return [{ frame: f, path: "mode", equals: "playing" }, { frame: f, path: "sectionClear", equals: false }];
  if (got.player) return [{ frame: f, path: `players.${got.player - 1}.x`, equals: got.x }, { frame: f, path: `players.${got.player - 1}.y`, equals: got.y }];
  return [];
}

function markdown(r) {
  const L = [];
  L.push(`# QA run`, ``, `\`${r.zip}\``, ``, `**${r.ok ? "OK" : "NOT OK"}**: ${r.verdict}. ${r.framesPlayed} frames played${r.cleared ? ", the section was cleared at least once" : ", no run cleared the section"}; ${r.seconds.total} s (${r.seconds.play} s playing, ${r.seconds.minimize} s minimizing).`, ``);
  L.push(`## Players`, ``, `| Player | What it does | Frames | Cleared | Energy lost | Findings | Time |`, `|---|---|---|---|---|---|---|`);
  for (const x of r.runs) L.push(`| ${x.name} | ${x.what} | ${x.frames} | ${x.cleared ? `frame ${x.clearFrame}` : x.gameOver ? `game over at ${x.gameOverFrame}` : "no"}${x.error ? ` (error: ${x.error})` : ""} | ${x.energyLost ?? "-"} | ${Object.entries(x.findings).map(([k, v]) => `${k} ${v}`).join(", ") || "none"} | ${(x.wallMs / 1000).toFixed(1)} s |`);
  const naive = r.runs.find((x) => x.name === "newcomer");
  if (naive) L.push(``, `**A naive player** (the newcomer) lost ${naive.energyLost} energy in ${naive.frames} frames${naive.cleared ? " and cleared the section" : naive.gameOver ? " and reached game over" : ""}.`);
  L.push(``, `## Findings`, ``, `Deduplicated by kind and 16 px cell; ${r.counts.high} high, ${r.counts.medium} medium, ${r.counts.low} low.`, ``);
  if (!r.findings.length) L.push(`None.`);
  else {
    L.push(`| Id | Severity | Kind | Found by | Player | Frame | x, y | Inputs | Minimized |`, `|---|---|---|---|---|---|---|---|---|`);
    for (const x of r.findings)
      L.push(`| ${x.id} | ${x.severity} | ${x.kind} | ${x.runs.join(", ")}${x.count > 1 ? ` (${x.count} times)` : ""} | ${x.player ?? "-"} | ${x.frame} | ${x.x ?? "-"}, ${x.y ?? "-"} | [${x.inputSteps} steps](${x.inputsFile}) | ${x.minimized ? (x.minimized.ok ? `[${x.minimized.steps} steps](${x.minimized.file}), frame ${x.minimized.frame}` : "not reproduced") : "-"} |`);
    L.push(``, `### What each kind means`, ``);
    for (const k of new Set(r.findings.map((x) => x.kind))) L.push(`- **${k}** (${KINDS[k][0]}): ${KINDS[k][1]}.`);
  }
  L.push(``, `## Replay a finding`, ``, "```sh", `node rom/tools/lab/run.mjs ZIP --out DIR --script findings/ID.min.json --mp4`, `go-link-console romtest --input findings/ID.min.json --frames-dir CORE_DIR ZIP`, "```", ``);
  return L.join("\n");
}

// ---------------------------------------------------------------- command line

function parseArgs(argv) {
  const o = { players: [...PLAYERS], frames: 7200, fuzzMinutes: 2, seed: 1, exitRule: "auto", patrol: 160, minimize: true, minSeconds: 30, minBudget: 120 };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    const v = () => {
      if (i + 1 >= argv.length) throw new Error(`${a} needs a value`);
      return argv[++i];
    };
    if (a === "--out") o.out = v();
    else if (a === "--players") o.players = v().split(",").filter(Boolean);
    else if (a === "--frames") o.frames = Number(v());
    else if (a === "--fuzz-minutes") o.fuzzMinutes = Number(v());
    else if (a === "--seed") o.seed = Number(v());
    else if (a === "--exit-rule") o.exitRule = v();
    else if (a === "--patrol") o.patrol = Number(v());
    else if (a === "--no-minimize") o.minimize = false;
    else if (a === "--min-seconds") o.minSeconds = Number(v());
    else if (a === "--min-budget") o.minBudget = Number(v());
    else if (a === "--symbols") o.symbols = v();
    else if (a === "--quiet") o.quiet = true;
    else if (a.startsWith("--")) throw new Error(`unknown option ${a}`);
    else o.zip = a;
  }
  if (!o.zip || !o.out) throw new Error("usage: qa.mjs ZIP --out DIR [options]; see the header");
  if (!["auto", "all", "touch"].includes(o.exitRule)) throw new Error("--exit-rule must be auto, all or touch");
  return o;
}

if (import.meta.url === `file://${process.argv[1]}`) {
  let o;
  try {
    o = parseArgs(process.argv.slice(2));
  } catch (e) {
    console.error(e.message);
    process.exit(2);
  }
  fs.mkdirSync(o.out, { recursive: true });
  const r = await qa(o);
  console.log(JSON.stringify({ ok: r.ok, verdict: r.verdict, counts: r.counts, findings: r.findings.map((x) => `${x.id} ${x.run} f${x.frame} (${x.x}, ${x.y})${x.minimized?.ok ? ` -> ${x.minimized.steps} steps` : ""}`), seconds: r.seconds }, null, 1));
}
