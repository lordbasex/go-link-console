// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { execFileSync, spawn } from "node:child_process";
import { closeSync, cpSync, mkdirSync, openSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join, resolve, basename } from "node:path";
import { fileURLToPath } from "node:url";
import { PORTS } from "./ports";
import { STATE_FILE, type Stack } from "./stack";

const here = dirname(fileURLToPath(import.meta.url));
const root = resolve(here, "..");

async function waitFor(what: string, ok: () => Promise<boolean>, ms = 60_000) {
  const end = Date.now() + ms;
  while (Date.now() < end) {
    if (await ok().catch(() => false)) return;
    await new Promise((r) => setTimeout(r, 250));
  }
  throw new Error(`timed out waiting for ${what}`);
}

/**
 * Builds and starts the test stack: a signalhub and a headless device of
 * their own, the device with a throwaway HOME (its config, rooms, history
 * and logs never mix with the developer's).
 */
export default async function globalSetup() {
  const bin = join(here, ".bin");
  const state = join(here, ".state");
  rmSync(state, { recursive: true, force: true });
  mkdirSync(bin, { recursive: true });
  mkdirSync(join(state, "home"), { recursive: true });

  // signalhub is its own repository: next to go-link, or where SIGNALING_DIR says.
  const signaling = process.env.SIGNALING_DIR || join(root, "..", "signaling");
  execFileSync("go", ["build", "-o", join(bin, "signal"), "./cmd/signal"], { cwd: signaling, stdio: "inherit" });
  // The device serves the website as its local panel: build it fresh and put
  // it in the device (like `make panel`), so the panel under test is today's.
  execFileSync("npm", ["run", "build"], {
    cwd: join(root, "frontend"),
    stdio: "inherit",
    env: { ...process.env, VITE_SIGNAL_URL: "wss://signal.go-link.org/ws", VITE_DEMO_DATA: "false" },
  });
  const panelDist = join(root, "backend-device", "web", "panel", "dist");
  rmSync(panelDist, { recursive: true, force: true });
  // the rooms' build, like `make panel`: the panel has no landing
  cpSync(join(root, "frontend", "apps", "web", "dist-play"), panelDist, { recursive: true });
  // The screenshots show the released version (and no "new version" notice).
  const release = JSON.parse(readFileSync(join(root, "frontend", "apps", "web", "src", "release.json"), "utf8")) as { version: string };
  const ldflags = process.env.E2E_SHOTS === "1" ? ["-ldflags", `-X main.version=${release.version.replace(/^v/, "")}`] : [];
  execFileSync("go", ["build", "-tags", "headless", ...ldflags, "-o", join(bin, "device"), "./cmd/device"], {
    cwd: join(root, "backend-device"),
    stdio: "inherit",
  });

  // npm run shots: an invented ROM library (made-up games and covers) in
  // the device's throwaway HOME, for the landing page's screenshots.
  if (process.env.E2E_SHOTS === "1") {
    execFileSync("go", ["run", "./cmd/shotseed", "-data", join(state, "home", "go-link-console")], {
      cwd: join(root, "backend-device"),
      stdio: "inherit",
    });
  }

  // E2E_CORE_DIR: a folder with the emulator core (like ~/go-link-console/cores), copied into the
  // device's throwaway HOME so game rooms can run (tests/maker-room.spec.ts); CI has none.
  if (process.env.E2E_CORE_DIR) cpSync(process.env.E2E_CORE_DIR, join(state, "home", "go-link-console", "cores"), { recursive: true });
  // E2E_ROMS: files (comma separated) copied into the device's ROM folder, e.g. a go-link HD package (tests/hd-core.spec.ts)
  for (const rom of (process.env.E2E_ROMS ?? "").split(",").filter(Boolean)) cpSync(rom, join(state, "home", "go-link-console", "roms", basename(rom)));

  const signalLog = openSync(join(state, "signal.log"), "a");
  const signal = spawn(join(bin, "signal"), [], {
    env: {
      ...process.env,
      ADDR: `127.0.0.1:${PORTS.signal}`,
      ALLOWED_ORIGINS: [`http://localhost:${PORTS.web}`, `http://127.0.0.1:${PORTS.web}`, `http://localhost:${PORTS.play}`, `http://127.0.0.1:${PORTS.panel}`].join(","),
      ALLOWED_APPS: "go-link-console",
      RATE_LIMIT_PER_MIN: "1000",
      OWNER_RATE_PER_MIN: "1000",
      MAX_ROOMS_PER_SESSION: "8",
      // Every test opens browsers from 127.0.0.1: the per-IP cap must not throttle the suite.
      MAX_CONNS_PER_IP: "500",
      LOG_LEVEL: "warn",
    },
    stdio: ["ignore", signalLog, signalLog],
    detached: true,
  });
  closeSync(signalLog);
  await waitFor("signalhub", async () => (await fetch(`http://127.0.0.1:${PORTS.signal}/healthz`)).ok);

  const config = join(state, "device.json");
  const deviceLog = join(state, "device.log");
  const out = openSync(deviceLog, "a");
  // E2E_DEVICE_COMMAND runs the device elsewhere instead (go-link HD on another computer, tests/hd.spec.ts):
  // a shell command whose output is the device's log, reaching this signalhub and giving its panel on the same ports
  const remote = process.env.E2E_DEVICE_COMMAND;
  const device = remote ? spawn("/bin/sh", ["-c", remote], { stdio: ["ignore", openSync(deviceLog, "a"), openSync(deviceLog, "a")], detached: true }) : spawn(
    join(bin, "device"),
    [
      "--headless",
      "--config", config,
      "--server-signaling", `ws://127.0.0.1:${PORTS.signal}/ws`,
      "--panel", `127.0.0.1:${PORTS.panel}`,
      "--web-url", `http://localhost:${PORTS.web}`,
      // The test pattern room pauses like a game, to test the host's pause
      // and the players' requests for one.
      "--test-room-pause",
      // The Android test reads the device's input log lines (npm run test:android).
      ...(process.env.E2E_DEVICE_DEBUG === "1" ? ["--debug"] : []),
      // Extra device flags, e.g. go-link HD's scene in the test room (tests/hd.spec.ts).
      ...(process.env.E2E_DEVICE_ARGS ? process.env.E2E_DEVICE_ARGS.split(" ").filter(Boolean) : []),
    ],
    { env: { ...process.env, HOME: join(state, "home") }, stdio: ["ignore", out, out], detached: true },
  );
  closeSync(out);
  const stack: Stack = { signalPid: signal.pid!, devicePid: device.pid!, deviceLog, config, bin: join(bin, "device") };
  writeFileSync(STATE_FILE, JSON.stringify(stack));
  await waitFor("the device's pairing code", async () => (await import("./stack")).pairingCode(stack) !== "");
  await waitFor("the web panel", async () => (await fetch(`http://127.0.0.1:${PORTS.panel}/`)).ok);
  signal.unref();
  device.unref();
}
