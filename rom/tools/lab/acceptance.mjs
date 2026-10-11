// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
// Experiment 1's acceptance run (docs/experiments/README.md, Acceptance
// 1-5; docs/experiments/harness.md): every automatic test on one ROM set,
// with every record kept, in one command.
//
//   node rom/tools/lab/acceptance.mjs ZIP --script clear.json --out DIR
//       [--bot-games N] [--laya-games N] [--bot-frames F] [--laya-frames F]
//       [--device PATH] [--no-mp4] [--skip level4,core,bot,laya]
//       [--no-qa] [--qa-minutes M]
//
// DIR gets: level3.json (+ level3.png), level4.json, scripted/sim (the
// runner's outputs), scripted/core (the real core's checkpoints and MP4),
// scripted/compare (difference pictures and compare.json), qa/ (the QA
// run: qa.mjs's qa.json, qa.md and findings/; on by default, --no-qa skips
// it, --qa-minutes sets its fuzz length, default 2), bot/game-NN and
// laya/game-NN (one runner folder per game) and acceptance.json with the
// verdict of each test. Game 1 of each player is its plain (argmax)
// game; games 2..N pass --seed 1..N-1.

import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { REPO, readZip, runPowerOn, setOf, WASM } from "./lab.mjs";
import { run } from "./run.mjs";
import { qa, PLAYERS } from "./qa.mjs";
import { writePng } from "../png.mjs";

const args = process.argv.slice(2);
const opt = (name, def) => (args.includes(name) ? args[args.indexOf(name) + 1] : def);
const valued = ["--script", "--out", "--bot-games", "--laya-games", "--bot-frames", "--laya-frames", "--device", "--skip", "--python", "--laya-every", "--qa-minutes"];
const zip = args.find((a, i) => !a.startsWith("--") && !valued.includes(args[i - 1]));
const script = opt("--script");
const out = opt("--out");
if (!zip || !script || !out) {
  console.error("usage: node rom/tools/lab/acceptance.mjs ZIP --script clear.json --out DIR [--bot-games N] [--laya-games N] [--bot-frames F] [--laya-frames F] [--device PATH] [--no-mp4] [--skip level4,core,bot,laya] [--no-qa] [--qa-minutes M]");
  process.exit(2);
}
const skip = new Set((opt("--skip", "") || "").split(",").filter(Boolean));
const mp4 = !args.includes("--no-mp4");
const device = opt("--device", path.join(REPO, "dist/device/darwin-universal/go-link-console.app/Contents/MacOS/go-link-console"));
const python = opt("--python", path.join(os.homedir(), "go-link-lab/venv/bin/python"));
const botGames = Number(opt("--bot-games", 3));
const layaGames = Number(opt("--laya-games", 1));
const botFrames = Number(opt("--bot-frames", 5400));
const layaFrames = Number(opt("--laya-frames", 3600));
const layaEvery = Number(opt("--laya-every", 6));
const withQa = !args.includes("--no-qa") && !skip.has("qa");
const qaMinutes = Number(opt("--qa-minutes", 2));
fs.mkdirSync(out, { recursive: true });
const report = { zip: path.resolve(zip), script: path.resolve(script), started: new Date().toISOString(), tests: {} };
const save = () => fs.writeFileSync(path.join(out, "acceptance.json"), JSON.stringify(report, null, 2) + "\n");
const say = (s) => console.error(`[acceptance] ${s}`);
const t0 = Date.now();

// 1. Validator level 3
say("level 3 (simulator power-on)");
const l3 = await runPowerOn(readZip(zip), { wasm: fs.readFileSync(WASM), set: setOf(zip) });
if (l3.shot) writePng(path.join(out, "level3.png"), l3.shot.w, l3.shot.h, l3.shot.rgba);
fs.writeFileSync(path.join(out, "level3.json"), JSON.stringify({ ...l3, shot: l3.shot ? "level3.png" : null }, null, 2) + "\n");
report.tests.level3 = { ok: l3.ok, failed: l3.steps.filter((s) => !s.ok).map((s) => `${s.name}: ${s.detail}`) };
save();

// 2. Validator level 4
if (!skip.has("level4")) {
  say("level 4 (device romtest, real core)");
  const r = spawnSync(device, ["romtest", "--json", zip], { encoding: "utf8" });
  let j = null;
  try {
    j = JSON.parse(r.stdout);
  } catch {
    /* reported below */
  }
  fs.writeFileSync(path.join(out, "level4.json"), (j ? JSON.stringify(j, null, 2) : r.stdout + r.stderr) + "\n");
  report.tests.level4 = j ? { ok: j.ok, failed: (j.steps ?? []).filter((s) => !s.ok).map((s) => `${s.name}: ${s.detail}`), error: j.error || null } : { ok: false, error: (r.stderr || "no output").trim() };
  save();
}

// 3. The scripted run in the simulator
say("scripted run (simulator)");
const simDir = path.join(out, "scripted/sim");
const sim = await run({ zip, out: simDir, script, mp4, scale: 2, quiet: true });
report.tests.scripted = { ok: !sim.error && sim.cleared, cleared: sim.cleared, clearFrame: sim.clearFrame, expectOk: sim.expectOk, expect: sim.expect, error: sim.error, labState: sim.labState };
save();

// 4. The same script on the real core, and the comparison
if (!skip.has("core")) {
  say("scripted run (real core) and comparison");
  const coreDir = path.join(out, "scripted/core");
  fs.mkdirSync(coreDir, { recursive: true });
  const r = spawnSync(device, ["romtest", "--json", "--input", script, "--frames-dir", path.join(coreDir, "frames"), ...(mp4 ? ["--mp4", path.join(coreDir, "run.mp4")] : []), zip], { encoding: "utf8" });
  fs.writeFileSync(path.join(coreDir, "replay.json"), r.stdout || r.stderr);
  const cmp = spawnSync(process.execPath, [path.join(REPO, "rom/tools/lab/compare.mjs"), path.join(simDir, "frames"), path.join(coreDir, "frames"), "--out", path.join(out, "scripted/compare"), "--json"], { encoding: "utf8" });
  let c = null;
  try {
    c = JSON.parse(cmp.stdout);
  } catch {
    /* reported below */
  }
  report.tests.sameAsCore = c ? { ok: c.frames > 0 && c.worstDifferPct === 0, frames: c.frames, worstDifferPct: c.worstDifferPct, perFrame: c.results.map((x) => ({ frame: x.frame, differPct: x.differPct ?? null })) } : { ok: false, error: (r.stderr + cmp.stderr).trim().slice(0, 2000) };
  save();
}

// 5. The QA run: adversarial players, a seeded 4-port fuzz, per-frame invariants, minimized repro scripts
if (withQa) {
  say(`QA run (adversarial players, ${qaMinutes} min of fuzz)`);
  const q = await qa({ zip, out: path.join(out, "qa"), players: [...PLAYERS], frames: 7200, fuzzMinutes: qaMinutes, seed: 1, exitRule: "auto", patrol: 160, minimize: true, minSeconds: 30, minBudget: 120, quiet: true });
  report.tests.qa = {
    ok: q.ok,
    verdict: q.verdict,
    counts: q.counts,
    framesPlayed: q.framesPlayed,
    cleared: q.cleared,
    // how hard the game is on someone who plays it plainly (T-15)
    naive: (({ name, frames, cleared, gameOver, energyLost }) => ({ player: name, frames, cleared, gameOver, energyLost }))(q.runs.find((r) => r.name === "newcomer") ?? { name: null, frames: 0, cleared: false, gameOver: false, energyLost: null }),
    seconds: q.seconds.total,
    findings: q.findings.map((x) => ({ id: x.id, severity: x.severity, kind: x.kind, runs: x.runs, frame: x.frame, x: x.x, y: x.y, why: x.detail?.why ?? null, script: `qa/${x.minimized?.file ?? x.inputsFile}`, steps: x.minimized?.ok ? x.minimized.steps : x.inputSteps })),
    report: "qa/qa.md",
  };
  save();
}

// 6. Automatic players
async function games(kind, n, cmd, frames, every) {
  const list = [];
  for (let g = 1; g <= n; g++) {
    const dir = path.join(out, kind, `game-${String(g).padStart(2, "0")}`);
    say(`${kind} game ${g}/${n}`);
    const seed = g === 1 ? "" : ` --seed ${g - 1}`;
    const s = await run({ zip, out: dir, player: cmd + seed, frames, every, port: 1, timeoutMs: 120000, afterClear: 120, mp4, scale: 2, quiet: true });
    list.push({ game: g, dir: path.relative(out, dir), seed: g === 1 ? null : g - 1, cleared: s.cleared, clearFrame: s.clearFrame, frames: s.frames, decisions: s.decisions, thinkMsMean: s.thinkMs.mean, energyLost: s.energyLost, falls: s.falls, rescued: s.final?.rescued ?? null, enemiesDown: s.final?.enemiesDown ?? null, score: s.final?.scores?.[0] ?? null, error: s.error });
  }
  const clears = list.filter((x) => x.cleared);
  const times = clears.map((x) => x.clearFrame).sort((a, b) => a - b);
  return {
    games: n,
    clears: clears.length,
    clearRate: n ? clears.length / n : 0,
    clearFrame: times.length ? { best: times[0], worst: times[times.length - 1], median: times[times.length >> 1] } : null,
    energyLost: list.reduce((a, x) => a + x.energyLost, 0),
    falls: list.reduce((a, x) => a + x.falls, 0),
    errors: list.filter((x) => x.error).map((x) => `game ${x.game}: ${x.error}`),
    list,
  };
}
if (!skip.has("bot") && botGames > 0) {
  report.tests.bot = await games("bot", botGames, `${process.execPath} ${path.join(REPO, "rom/tools/lab/bot.mjs")}`, botFrames, 6);
  save();
}
if (!skip.has("laya") && layaGames > 0) {
  report.tests.laya = await games("laya", layaGames, `${python} ${path.join(REPO, "rom/tools/lab/laya_player.py")}`, layaFrames, layaEvery);
  save();
}

report.seconds = Math.round((Date.now() - t0) / 1000);
report.summary = Object.fromEntries(Object.entries(report.tests).map(([k, v]) => [k, "ok" in v ? v.ok : `${v.clears}/${v.games} cleared`]));
save();
console.log(JSON.stringify(report.summary, null, 2));
