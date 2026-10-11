// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { execFileSync, spawn, spawnSync, type ChildProcess } from "node:child_process";
import { closeSync, existsSync, mkdirSync, openSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import http2 from "node:http2";
import { homedir, tmpdir, userInfo } from "node:os";
import { join } from "node:path";

// Drives the go-link Console Player Android app on an emulator or a phone with adb
// and UiAutomator, for e2e/tests/android.spec.ts. Controls are found by
// their Compose test tags (exposed as resource ids) or their text; the
// app's debug build logs its room events and WebRTC counters with the tag
// GoLinkE2E (mobile/android/app/src/debug/.../E2eProbe.kt).

export const PACKAGE = "org.golink.player";

/** The adb binary: ADB, the Android SDK's platform-tools, or the PATH. */
export function adbPath(): string {
  if (process.env.ADB) return process.env.ADB;
  const sdk = process.env.ANDROID_HOME || process.env.ANDROID_SDK_ROOT || join(homedir(), "Library", "Android", "sdk");
  const p = join(sdk, "platform-tools", "adb");
  return existsSync(p) ? p : "adb";
}

/** The attached device's serial (ANDROID_SERIAL wins), or "" when none is ready. */
export function attachedDevice(): string {
  try {
    const out = execFileSync(adbPath(), ["devices"], { encoding: "utf8", timeout: 10_000 });
    const ready = out
      .split("\n")
      .slice(1)
      .map((l) => l.trim().split(/\s+/))
      .filter((f) => f.length >= 2 && f[1] === "device")
      .map((f) => f[0]!);
    if (process.env.ANDROID_SERIAL) return ready.includes(process.env.ANDROID_SERIAL) ? process.env.ANDROID_SERIAL : "";
    return ready[0] ?? "";
  } catch {
    return "";
  }
}

export function adb(...args: string[]): string {
  return execFileSync(adbPath(), args, { encoding: "utf8", timeout: 60_000, maxBuffer: 64 * 1024 * 1024 });
}

export function shell(cmd: string): string {
  return adb("shell", cmd);
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

/** One node of a UiAutomator dump. */
export interface UiNode {
  text: string;
  desc: string;
  id: string;
  cls: string;
  checked: boolean;
  enabled: boolean;
  pkg: string;
  bounds: [number, number, number, number];
}

function attr(tag: string, name: string): string {
  const m = tag.match(new RegExp(` ${name}="([^"]*)"`));
  return m ? decodeEntities(m[1]!) : "";
}

function decodeEntities(s: string): string {
  return s
    .replace(/&quot;/g, '"')
    .replace(/&apos;/g, "'")
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&#(\d+);/g, (_, n) => String.fromCharCode(Number(n)))
    .replace(/&amp;/g, "&");
}

/** The screen's accessibility tree, as UiAutomator sees it, after closing system dialogs. */
export function dump(): UiNode[] {
  let nodes = rawDump();
  // A slow emulator can show "... isn't responding" or another app's crash
  // over the app; close those and look again (a few times at most).
  for (let i = 0; i < 3 && dismissSystemDialog(nodes); i++) {
    spawnSync("sleep", ["1"]);
    nodes = rawDump();
  }
  return nodes;
}

/** The app's name as Android shows it in system dialogs. */
const APP_LABEL = "go-link Console Player";

/**
 * Handles a system dialog on screen: an ANR ("X isn't responding") gets
 * Wait, another app's crash ("X has stopped", "X keeps stopping") gets
 * Close. A crash of this app is never hidden: it fails the test with the
 * dialog's text. Returns true when it tapped something.
 */
export function dismissSystemDialog(nodes: UiNode[]): boolean {
  const title = nodes.find((n) => n.id === "android:id/alertTitle")?.text ?? "";
  const wait = nodes.find((n) => n.id === "android:id/aerr_wait");
  const close = nodes.find((n) => n.id === "android:id/aerr_close");
  if (!wait && !close) return false;
  const ours = title.includes(APP_LABEL) || title.includes(PACKAGE);
  if (/isn.t responding/i.test(title) && wait) {
    console.log(`android: system dialog "${title}": Wait`);
    tap(wait);
    return true;
  }
  if (ours) throw new Error(`Android: the app crashed: "${title}"`);
  if (close) {
    console.log(`android: system dialog "${title}": Close`);
    tap(close);
    return true;
  }
  return false;
}

function rawDump(): UiNode[] {
  // uiautomator refuses while the UI is busy animating; try a few times.
  for (let i = 0; i < 5; i++) {
    const r = spawnSync(adbPath(), ["exec-out", "uiautomator", "dump", "/dev/tty"], { encoding: "utf8", timeout: 30_000 });
    const xml = r.stdout ?? "";
    const start = xml.indexOf("<?xml");
    if (start >= 0) {
      const nodes: UiNode[] = [];
      for (const m of xml.slice(start).matchAll(/<node [^>]*>/g)) {
        const tag = m[0];
        const b = attr(tag, "bounds").match(/\[(\d+),(\d+)\]\[(\d+),(\d+)\]/);
        nodes.push({
          text: attr(tag, "text"),
          desc: attr(tag, "content-desc"),
          id: attr(tag, "resource-id"),
          cls: attr(tag, "class"),
          checked: attr(tag, "checked") === "true",
          enabled: attr(tag, "enabled") !== "false",
          pkg: attr(tag, "package"),
          bounds: b ? [Number(b[1]), Number(b[2]), Number(b[3]), Number(b[4])] : [0, 0, 0, 0],
        });
      }
      return nodes;
    }
    spawnSync("sleep", ["0.5"]);
  }
  return [];
}

export type Match = { id?: string; text?: string | RegExp; desc?: string | RegExp };

function matches(n: UiNode, q: Match): boolean {
  if (q.id !== undefined && n.id !== q.id) return false;
  const test = (v: string, p: string | RegExp) => (typeof p === "string" ? v === p : p.test(v));
  if (q.text !== undefined && !test(n.text, q.text)) return false;
  if (q.desc !== undefined && !test(n.desc, q.desc)) return false;
  return true;
}

export function find(q: Match, nodes = dump()): UiNode | undefined {
  return nodes.find((n) => matches(n, q));
}

/** Waits for a node; throws with what the screen showed on timeout. */
export async function waitFor(q: Match, timeout = 30_000): Promise<UiNode> {
  const end = Date.now() + timeout;
  let last: UiNode[] = [];
  while (Date.now() < end) {
    last = dump();
    const n = last.find((x) => matches(x, q));
    if (n) return n;
    await sleep(700);
  }
  const seen = last
    .filter((n) => n.text || n.desc || n.id)
    .map((n) => [n.id, n.text, n.desc].filter(Boolean).join("|"))
    .join("; ");
  throw new Error(`Android: no node ${JSON.stringify(q, (_, v) => (v instanceof RegExp ? String(v) : v))}; screen: ${seen}`);
}

export async function waitGone(q: Match, timeout = 30_000): Promise<void> {
  const end = Date.now() + timeout;
  while (Date.now() < end) {
    if (!find(q)) return;
    await sleep(700);
  }
  throw new Error(`Android: node still shown: ${JSON.stringify(q)}`);
}

export function center(n: UiNode): [number, number] {
  return [Math.round((n.bounds[0] + n.bounds[2]) / 2), Math.round((n.bounds[1] + n.bounds[3]) / 2)];
}

export function tap(n: UiNode | [number, number]) {
  const [x, y] = Array.isArray(n) ? n : center(n);
  shell(`input tap ${x} ${y}`);
}

/** Holds a finger at a point for ms (a swipe that does not move). */
export function hold(at: [number, number], ms: number) {
  shell(`input swipe ${at[0]} ${at[1]} ${at[0]} ${at[1]} ${ms}`);
}

export async function tapOn(q: Match, timeout?: number) {
  const n = await waitFor(q, timeout);
  tap(n);
  return n;
}

/** Types ASCII text into the focused field (spaces as %s, like adb wants). */
export function typeText(text: string) {
  const escaped = text.replace(/ /g, "%s").replace(/([\\"'`$&|;<>()])/g, "\\$1");
  shell(`input text "${escaped}"`);
}

/** Focuses a field, clears it and types. */
export async function fill(q: Match, text: string) {
  const n = await tapOn(q);
  await sleep(300);
  // Select everything and delete (ctrl+a, del), then type.
  shell("input keycombination 113 29");
  shell("input keyevent 67");
  typeText(text);
  await sleep(300);
  return n;
}

export function hideKeyboard() {
  // Back closes the keyboard when it is shown; check first so Back never leaves a screen.
  const out = shell("dumpsys input_method | grep -c 'mInputShown=true' || true").trim();
  if (out !== "0" && out !== "") shell("input keyevent 4");
}

/** Saves a PNG screenshot of the device. */
export function screenshot(file: string) {
  mkdirSync(join(file, ".."), { recursive: true });
  const r = spawnSync(adbPath(), ["exec-out", "screencap", "-p"], { maxBuffer: 64 * 1024 * 1024, timeout: 30_000 });
  writeFileSync(file, r.stdout);
}

/** The screen's raw RGBA pixels. */
export function rawScreen(): { width: number; height: number; px: Buffer } {
  const r = spawnSync(adbPath(), ["exec-out", "screencap"], { maxBuffer: 128 * 1024 * 1024, timeout: 30_000 });
  const b = r.stdout as Buffer;
  const width = b.readUInt32LE(0);
  const height = b.readUInt32LE(4);
  // The header is 12 bytes (width, height, format) or 16 with the color space.
  const header = b.length - width * height * 4;
  return { width, height, px: b.subarray(header) };
}

/**
 * How much of a screen rectangle is lit and how varied it is: the share
 * of sampled pixels brighter than a dark gray and the number of distinct
 * coarse colors. A black or frozen-blank picture scores ~0 on both.
 */
export function pictureStats(rect: [number, number, number, number]) {
  const { width, height, px } = rawScreen();
  const [l, t, r, bt] = [Math.max(0, rect[0]), Math.max(0, rect[1]), Math.min(width, rect[2]), Math.min(height, rect[3])];
  let lit = 0;
  let total = 0;
  const colors = new Set<number>();
  for (let y = t + 2; y < bt - 2; y += 6) {
    for (let x = l + 2; x < r - 2; x += 6) {
      const i = (y * width + x) * 4;
      const R = px[i]!;
      const G = px[i + 1]!;
      const B = px[i + 2]!;
      total++;
      if (R + G + B > 90) lit++;
      colors.add(((R >> 5) << 6) | ((G >> 5) << 3) | (B >> 5));
    }
  }
  return { litShare: total ? lit / total : 0, colors: colors.size };
}

/** Records the screen in chunks (screenrecord stops at 180 s), pulled at the end. */
export class ScreenRecorder {
  private chunk = 0;
  private proc: ChildProcess | null = null;
  private pid = "";
  private stopped = false;
  private readonly files: string[] = [];

  private readonly prefix: string;

  constructor(prefix = "golink-e2e") {
    this.prefix = prefix;
  }

  start() {
    this.stopped = false;
    this.next();
  }

  private next() {
    if (this.stopped) return;
    const remote = `/sdcard/${this.prefix}-${++this.chunk}.mp4`;
    this.files.push(remote);
    // The shell prints its PID and becomes screenrecord (exec), so stop()
    // signals exactly the recorder this test started.
    this.pid = "";
    this.proc = spawn(adbPath(), ["shell", `echo $$; exec screenrecord --time-limit 175 --bit-rate 4000000 ${remote}`], {
      stdio: ["ignore", "pipe", "ignore"],
    });
    this.proc.stdout!.setEncoding("utf8");
    this.proc.stdout!.on("data", (d: string) => {
      if (!this.pid) this.pid = d.trim().split(/\s+/)[0] ?? "";
    });
    this.proc.on("exit", () => {
      this.proc = null;
      if (!this.stopped) this.next();
    });
  }

  /** Stops the recording and copies the chunks to dir; returns the local files. */
  async stop(dir: string): Promise<string[]> {
    this.stopped = true;
    if (this.pid && /^\d+$/.test(this.pid)) {
      try {
        // SIGINT makes screenrecord finish the file properly.
        shell(`kill -INT ${this.pid} || true`);
      } catch {
        // already finished
      }
    }
    const end = Date.now() + 15_000;
    while (this.proc && Date.now() < end) await sleep(300);
    await sleep(1500);
    mkdirSync(dir, { recursive: true });
    const out: string[] = [];
    for (const remote of this.files) {
      const local = join(dir, remote.split("/").pop()!);
      try {
        adb("pull", remote, local);
        shell(`rm -f ${remote}`);
        out.push(local);
      } catch {
        // a chunk that never started
      }
    }
    return out;
  }
}

/** One GoLinkE2E line from the app. */
export type ProbeEvent = Record<string, unknown> & { ev: string };

/** Follows the app's GoLinkE2E logcat lines. */
export class Probe {
  readonly events: ProbeEvent[] = [];
  private proc: ChildProcess | null = null;
  private buf = "";

  start() {
    adb("logcat", "-c");
    this.proc = spawn(adbPath(), ["logcat", "-v", "raw", "-s", "GoLinkE2E:I"], { stdio: ["ignore", "pipe", "ignore"] });
    this.proc.stdout!.setEncoding("utf8");
    this.proc.stdout!.on("data", (chunk: string) => {
      this.buf += chunk;
      let nl: number;
      while ((nl = this.buf.indexOf("\n")) >= 0) {
        const line = this.buf.slice(0, nl).trim();
        this.buf = this.buf.slice(nl + 1);
        if (!line.startsWith("{")) continue;
        try {
          this.events.push(JSON.parse(line) as ProbeEvent);
        } catch {
          // a line cut by logcat
        }
      }
    });
  }

  stop() {
    this.proc?.kill();
    this.proc = null;
  }

  /** Index to read "events after now" from. */
  mark(): number {
    return this.events.length;
  }

  lastUi(): ProbeEvent | undefined {
    for (let i = this.events.length - 1; i >= 0; i--) if (this.events[i]!.ev === "ui") return this.events[i];
    return undefined;
  }

  lastStats(): StatsLine | undefined {
    for (let i = this.events.length - 1; i >= 0; i--) if (this.events[i]!.ev === "stats") return this.events[i] as unknown as StatsLine;
    return undefined;
  }

  /** Waits for an event after `from` that passes ok. */
  async waitFor(ok: (e: ProbeEvent) => boolean, timeout = 30_000, from = 0): Promise<ProbeEvent> {
    const end = Date.now() + timeout;
    let i = from;
    while (Date.now() < end) {
      for (; i < this.events.length; i++) if (ok(this.events[i]!)) return this.events[i]!;
      await sleep(250);
    }
    throw new Error(`GoLinkE2E: no matching event in ${timeout} ms; last ui: ${JSON.stringify(this.lastUi())}`);
  }
}

export interface TrackStats {
  packets?: number;
  bytes?: number;
  level?: number;
  energy?: number;
  samples?: number;
  frames?: number;
  width?: number;
}

export interface StatsLine {
  ev: "stats";
  in: Record<string, TrackStats>;
  out: { mic?: { packets?: number; bytes?: number }; mic_level?: number };
  path?: string;
}

/**
 * Installs the APK under test. A release build of the app on the device
 * (another signing key, a higher version code) is uninstalled first.
 */
export function installApp(apk: string) {
  try {
    adb("install", "-r", apk);
  } catch (e) {
    const msg = String((e as { stderr?: unknown }).stderr ?? e);
    if (!/INSTALL_FAILED_(VERSION_DOWNGRADE|UPDATE_INCOMPATIBLE)|signatures do not match/i.test(msg)) throw e;
    console.log("android: replacing another build of the app (release key or newer version)");
    adb("uninstall", PACKAGE);
    adb("install", apk);
  }
}

/** The average color of a screen rectangle (RGB 0-255). */
export function averageColor(rect: [number, number, number, number]): [number, number, number] {
  const { width, height, px } = rawScreen();
  const [l, t, r, b] = [Math.max(0, rect[0]), Math.max(0, rect[1]), Math.min(width, rect[2]), Math.min(height, rect[3])];
  let n = 0;
  const sum: [number, number, number] = [0, 0, 0];
  for (let y = t; y < b; y++) {
    for (let x = l; x < r; x++) {
      const i = (y * width + x) * 4;
      sum[0] += px[i]!;
      sum[1] += px[i + 1]!;
      sum[2] += px[i + 2]!;
      n++;
    }
  }
  return n ? [sum[0] / n, sum[1] / n, sum[2] / n] : [0, 0, 0];
}

/** Holds a finger at a point for ms without waiting; resolves when it lifts. */
export function holdAsync(at: [number, number], ms: number): Promise<void> {
  return new Promise((resolve) => {
    const p = spawn(adbPath(), ["shell", `input swipe ${at[0]} ${at[1]} ${at[0]} ${at[1]} ${ms}`], { stdio: "ignore" });
    p.on("exit", () => resolve());
  });
}

// --- The emulator's own controls (gRPC) --------------------------------------
//
// The Android emulator serves gRPC (EmulatorController in the SDK's
// emulator/lib/emulator_controller.proto) on localhost with a token that it
// writes to a discovery file; it moves the virtual scene's camera. The one
// message used here is encoded by hand, so no gRPC library is needed.
// (injectAudio, to feed the emulated microphone, crashed emulator 37.1.11
// on macOS every time it was tried, so the microphone test does not use it.)

/** One running emulator, from its discovery file (pid_<pid>.ini). */
export interface EmulatorInfo {
  pid: number;
  grpcPort: number;
  token: string;
  /** The emulator's command line (qemu binary first). */
  cmdline: string[];
  launcherDir: string;
}

function discoveryDirs(): string[] {
  const dirs = [join(homedir(), "Library", "Caches", "TemporaryItems", "avd", "running")];
  if (process.env.XDG_RUNTIME_DIR) dirs.push(join(process.env.XDG_RUNTIME_DIR, "avd", "running"));
  if (typeof process.getuid === "function") dirs.push(`/run/user/${process.getuid()}/avd/running`);
  dirs.push(join(tmpdir(), `android-${userInfo().username}`, "avd", "running"), join(homedir(), ".android", "avd", "running"));
  return dirs;
}

function parseCmdline(s: string): string[] {
  return [...s.matchAll(/"((?:[^"\\]|\\.)*)"|(\S+)/g)].map((m) => m[1] ?? m[2]!);
}

/** The emulator behind an adb serial (emulator-5554), or null for a phone or when unknown. */
export function emulatorInfo(serial: string): EmulatorInfo | null {
  const port = serial.match(/^emulator-(\d+)$/)?.[1];
  if (!port) return null;
  for (const dir of discoveryDirs()) {
    if (!existsSync(dir)) continue;
    for (const f of readdirSync(dir)) {
      if (!/^pid_\d+\.ini$/.test(f)) continue;
      const ini = readFileSync(join(dir, f), "utf8");
      const get = (k: string) => ini.match(new RegExp(`^${k.replace(/\./g, "\\.")}=(.*)$`, "m"))?.[1]?.trim() ?? "";
      if (get("port.serial") !== port) continue;
      const pid = Number(f.slice(4, -4));
      try {
        process.kill(pid, 0); // still running (signal 0 only checks)
      } catch {
        continue;
      }
      return { pid, grpcPort: Number(get("grpc.port")), token: get("grpc.token"), cmdline: parseCmdline(get("cmdline")), launcherDir: get("launcher.dir") };
    }
  }
  return null;
}

function varint(n: number): Buffer {
  const out: number[] = [];
  let v = n;
  while (v > 0x7f) {
    out.push((v & 0x7f) | 0x80);
    v = Math.floor(v / 128);
  }
  out.push(v);
  return Buffer.from(out);
}

/** A protobuf field: varint (wire type 0) for numbers, length-delimited (2) for buffers. */
function field(num: number, value: number | Buffer): Buffer {
  if (typeof value === "number") return Buffer.concat([varint(num << 3), varint(value)]);
  return Buffer.concat([varint((num << 3) | 2), varint(value.length), value]);
}

function grpcFrame(msg: Buffer): Buffer {
  const head = Buffer.alloc(5);
  head.writeUInt32BE(msg.length, 1);
  return Buffer.concat([head, msg]);
}

export const PhysicalType = { POSITION: 0, ROTATION: 1 } as const;

export class EmulatorGrpc {
  private readonly info: EmulatorInfo;

  constructor(info: EmulatorInfo) {
    this.info = info;
  }

  private request(method: string): { session: http2.ClientHttp2Session; req: http2.ClientHttp2Stream; done: Promise<void> } {
    const session = http2.connect(`http://127.0.0.1:${this.info.grpcPort}`);
    session.on("error", () => undefined);
    const req = session.request({
      ":method": "POST",
      ":path": `/android.emulation.control.EmulatorController/${method}`,
      "content-type": "application/grpc",
      te: "trailers",
      authorization: `Bearer ${this.info.token}`,
    });
    req.resume();
    const done = new Promise<void>((resolve, reject) => {
      let status = "";
      let message = "";
      const take = (h: http2.IncomingHttpHeaders) => {
        if (h["grpc-status"] !== undefined) status = String(h["grpc-status"]);
        if (h["grpc-message"] !== undefined) message = decodeURIComponent(String(h["grpc-message"]));
      };
      req.on("response", take);
      req.on("trailers", take);
      req.on("error", reject);
      req.on("close", () => {
        session.close();
        if (status === "0") resolve();
        else reject(new Error(`emulator gRPC ${method}: status ${status || "none"} ${message}`));
      });
    });
    return { session, req, done };
  }

  /** setPhysicalModel: moves (POSITION, meters) or turns (ROTATION, degrees) the virtual device at once. */
  async setPhysicalModel(type: number, values: [number, number, number]) {
    const floats = Buffer.alloc(12);
    values.forEach((v, i) => floats.writeFloatLE(v, i * 4));
    const msg = Buffer.concat([field(1, type), field(3, field(1, floats)), field(4, 1 /* STEP */)]);
    const { req, done } = this.request("setPhysicalModel");
    req.end(grpcFrame(msg));
    await done;
  }
}

/** The emulator's launcher and its arguments, from a discovery file's command line. */
export function emulatorArgs(info: EmulatorInfo): string[] {
  return info.cmdline.slice(1);
}

/** Stops an emulator through its console and waits for its process to end. */
export async function stopEmulator(info: EmulatorInfo) {
  adb("emu", "kill");
  const end = Date.now() + 60_000;
  while (Date.now() < end) {
    try {
      process.kill(info.pid, 0);
    } catch {
      return;
    }
    await sleep(500);
  }
  throw new Error(`the emulator (pid ${info.pid}) did not stop`);
}

/** Starts an emulator in the background with these arguments and waits until Android has booted. */
export async function startEmulator(launcherDir: string, args: string[], log: string) {
  const out = openSync(log, "a");
  const p = spawn(join(launcherDir, "emulator"), args, { stdio: ["ignore", out, out], detached: true });
  closeSync(out);
  p.unref();
  const end = Date.now() + 300_000;
  while (Date.now() < end) {
    await sleep(2000);
    const r = spawnSync(adbPath(), ["shell", "getprop", "sys.boot_completed"], { encoding: "utf8", timeout: 10_000 });
    if ((r.stdout ?? "").trim() === "1") return;
  }
  throw new Error(`the emulator did not boot (log: ${log})`);
}
