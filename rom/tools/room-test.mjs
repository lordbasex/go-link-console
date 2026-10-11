// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
// Plays rom/build/<set>.zip in a real go-link game room, end to end:
// a signalhub and a headless device of their own (throwaway HOME whose ROM
// folder holds only our set; the core is copied from ~/go-link-console/cores), the
// device's local panel in Chromium, "New game", then coin, start, run,
// shoot, the bazooka and a jump through the room's keyboard input (Z, X, C
// are buttons 1, 2, 3). Saves pictures of the room's WebRTC video in
// rom/build/room/.
//
// Needs: the signalhub repo next to go-link (or SIGNALING_DIR), the core
// downloaded (`device core download`), e2e/node_modules (npm ci in e2e/).
// Usage: node rom/tools/room-test.mjs [set]   (default slammast; build it first)

import { execFileSync, spawn } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const ROM = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const REPO = path.resolve(ROM, "..");
const OUT = path.join(ROM, "build", "room");
const SET = process.argv[2] || "slammast";
const HOME = path.join(OUT, "home");
const PORTS = { signal: 8192, panel: 7392 }; // apart from the e2e stack (8191, 7391) and the developer's
const { chromium } = await import(path.join(REPO, "e2e", "node_modules", "playwright", "index.mjs"));

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function waitFor(what, ok, ms = 60_000) {
  const end = Date.now() + ms;
  while (Date.now() < end) {
    if (await ok().catch(() => false)) return;
    await sleep(250);
  }
  throw new Error(`timed out waiting for ${what}`);
}

// ------------------------------------------------------------ the stack
fs.rmSync(OUT, { recursive: true, force: true });
for (const d of ["bin", "home/go-link/cores", "home/go-link/roms", "shots"]) fs.mkdirSync(path.join(OUT, d), { recursive: true });
const cores = path.join(os.homedir(), "go-link", "cores");
for (const f of fs.readdirSync(cores)) if (/^mame2003_plus/.test(f)) fs.copyFileSync(path.join(cores, f), path.join(HOME, "go-link/cores", f));
fs.copyFileSync(path.join(ROM, "build", `${SET}.zip`), path.join(HOME, `go-link/roms/${SET}.zip`));

const signaling = process.env.SIGNALING_DIR || path.join(REPO, "..", "signaling");
execFileSync("go", ["build", "-o", path.join(OUT, "bin/signal"), "./cmd/signal"], { cwd: signaling, stdio: "inherit" });
execFileSync("go", ["build", "-tags", "headless", "-o", path.join(OUT, "bin/device"), "./cmd/device"], { cwd: path.join(REPO, "backend-device"), stdio: "inherit" });

const log = (name) => fs.openSync(path.join(OUT, `${name}.log`), "a");
const signal = spawn(path.join(OUT, "bin/signal"), [], {
  env: {
    ...process.env,
    ADDR: `127.0.0.1:${PORTS.signal}`,
    ALLOWED_ORIGINS: `http://127.0.0.1:${PORTS.panel}`,
    ALLOWED_APPS: "go-link-console",
    MAX_ROOMS_PER_SESSION: "8",
    LOG_LEVEL: "warn",
  },
  stdio: ["ignore", log("signal"), log("signal")],
});
await waitFor("signalhub", async () => (await fetch(`http://127.0.0.1:${PORTS.signal}/healthz`)).ok);
const config = path.join(OUT, "device.json");
const device = spawn(
  path.join(OUT, "bin/device"),
  ["--headless", "--config", config, "--server-signaling", `ws://127.0.0.1:${PORTS.signal}/ws`, "--panel", `127.0.0.1:${PORTS.panel}`],
  { env: { ...process.env, HOME }, stdio: ["ignore", log("device"), log("device")] },
);
const stop = () => {
  for (const p of [device, signal]) p.kill("SIGTERM");
};
process.on("exit", stop);
await waitFor("the web panel", async () => (await fetch(`http://127.0.0.1:${PORTS.panel}/`)).ok);
const token = execFileSync(path.join(OUT, "bin/device"), ["panel", "token", "--config", config], { encoding: "utf8", env: { ...process.env, HOME } }).trim();

// ------------------------------------------------------------- the room
const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required", "--mute-audio"] });
const page = await browser.newPage({ viewport: { width: 1280, height: 860 }, locale: "en-US" });
try {
  await page.goto(`http://127.0.0.1:${PORTS.panel}/`);
  await page.getByLabel("Panel token").fill(token);
  await page.getByRole("checkbox", { name: /I have read and accept/ }).check();
  await page.getByRole("button", { name: "Open the panel" }).click();
  await page.getByRole("navigation", { name: "Main" }).getByRole("link", { name: "My device" }).click();
  await page.getByText("Linked · live").waitFor({ timeout: 30_000 });
  // New game: the library has only our set
  await page.evaluate(() => {
    history.pushState({}, "", "/create");
    dispatchEvent(new PopStateEvent("popstate"));
  });
  const submit = page.getByRole("button", { name: "Start the game" });
  await waitFor("the game picker", async () => submit.isEnabled(), 30_000);
  await page.screenshot({ path: path.join(OUT, "shots", "1-new-game.png") });
  await submit.click();
  await page.waitForURL(/\/r\//, { timeout: 60_000 });
  await waitFor(
    "the room's video",
    async () =>
      page.evaluate(() => {
        const v = document.querySelector("video.video");
        return !!v && v.videoWidth > 0 && v.readyState >= 2 && !v.paused;
      }),
    60_000,
  );
  // one frame of the WebRTC video, exactly as received
  const grab = async (name) => {
    const data = await page.evaluate(() => {
      const v = document.querySelector("video.video");
      const c = document.createElement("canvas");
      c.width = v.videoWidth;
      c.height = v.videoHeight;
      c.getContext("2d").drawImage(v, 0, 0);
      return { url: c.toDataURL("image/png"), w: v.videoWidth, h: v.videoHeight };
    });
    fs.writeFileSync(path.join(OUT, "shots", `${name}.png`), Buffer.from(data.url.split(",")[1], "base64"));
    console.log(`${name}: ${data.w}x${data.h}`);
  };
  await sleep(2500);
  await grab("2-attract");
  await page.screenshot({ path: path.join(OUT, "shots", "2-room-page.png") });
  // play from the room's keyboard: 5 coin, Enter start, arrows, Z jump
  // (button 1), X machine gun (button 2), C special (button 3)
  await page.locator("video.video").click();
  const tap = async (key, ms = 100) => {
    await page.keyboard.down(key);
    await sleep(ms);
    await page.keyboard.up(key);
  };
  await tap("Digit5");
  await sleep(800);
  await grab("3-credit");
  await tap("Enter");
  await sleep(800);
  // a double tap to the right runs (and picks up the bazooka crate)
  await tap("ArrowRight", 60);
  await sleep(60);
  await page.keyboard.down("ArrowRight");
  await sleep(900);
  await grab("4-run");
  await page.keyboard.up("ArrowRight");
  await page.keyboard.down("KeyX");
  await sleep(450);
  await grab("5-machine-gun");
  await page.keyboard.up("KeyX");
  await sleep(200);
  await tap("KeyC");
  await sleep(250);
  await grab("6-bazooka");
  await sleep(1200);
  await tap("KeyZ");
  await sleep(250);
  await grab("7-jump");
  await page.keyboard.down("ArrowRight");
  await sleep(6000);
  await page.keyboard.up("ArrowRight");
  await sleep(400);
  await grab("8-later");
  await page.screenshot({ path: path.join(OUT, "shots", "8-room-page.png") });
  const stats = await page.evaluate(async () => {
    const v = document.querySelector("video.video");
    const q = v.getVideoPlaybackQuality?.();
    return { decoded: q?.totalVideoFrames, dropped: q?.droppedVideoFrames };
  });
  console.log("video frames", stats);
} finally {
  await browser.close();
  stop();
}
console.log(`pictures in ${path.join(OUT, "shots")}`);
