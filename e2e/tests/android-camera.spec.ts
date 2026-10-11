// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { expect, test, type BrowserContext, type Page } from "@playwright/test";
import { cpSync, existsSync, mkdirSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import * as A from "../android";
import { enterAlias, freshApp, invite, joinWithPin, ownerOpensTestPattern, pointAppAt, trackedContext } from "../android-owner";
import { PORTS } from "../ports";

// The go-link Player app scans a real QR code with the emulator's back
// camera (npm run test:android:camera, opt-in: it restarts the emulator).
//
// The emulator's "virtualscene" camera renders a 3D room with a poster on
// a wall; `adb emu virtualscene-image wall <png>` swaps the poster's image
// and the emulator's gRPC (setPhysicalModel) moves the virtual device in
// front of it. The test draws the stack's real invitation as a QR code
// with the website's encoder (qrcode-generator, as QrCode.tsx uses it),
// opens the scanner, grants CAMERA through the system dialog, and checks
// that the app goes to the PIN screen with that invitation and joins.
//
// The emulator must run with `-camera-back virtualscene`. When it does not,
// the test stops it, starts it again with the same command line plus that
// flag, and at the end starts it again exactly as it was.

const here = dirname(fileURLToPath(import.meta.url));
const APK = process.env.ANDROID_APK || resolve(here, "../../mobile/android/app/build/outputs/apk/debug/app-debug.apk");
const SIGNAL_URL = process.env.ANDROID_SIGNAL_URL || `ws://10.0.2.2:${PORTS.signal}/ws`;
const OUT = resolve(here, "../test-results/android-camera");

// Where Toren1BD.posters (the SDK's emulator/resources) puts the wall
// poster: its center and its facing (a 2 m square turned -150 degrees).
const POSTER = { x: -0.807, y: 0.32, z: 5.316, yaw: -150 };
// How far in front of it the virtual device stands (the whole code in view).
const DISTANCE = 3.5;

const serial = A.attachedDevice();
test.skip(!serial, "no Android device in `adb devices`");
test.skip(!/^emulator-\d+$/.test(serial), "the virtual camera needs the Android emulator");
test.describe.configure({ mode: "serial" });

let owner: BrowserContext;
let page: Page;
let recorder: A.ScreenRecorder;
let probe: A.Probe;
let grpc: A.EmulatorGrpc;
/** The emulator's arguments before the test, when the test restarted it. */
let restoreArgs: string[] | null = null;
let launcherDir = "";
let shot = 0;

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

function snap(name: string) {
  A.screenshot(join(OUT, `${String(++shot).padStart(2, "0")}-${name}.png`));
}

/** A QR code image of text, drawn like the website's QrCode component (qrcode-generator, level M, 4-module quiet zone). */
async function qrPng(text: string, file: string) {
  const require = createRequire(resolve(here, "../../frontend/package.json"));
  const qrcode = require("qrcode-generator") as (type: number, level: string) => {
    addData(s: string): void;
    make(): void;
    getModuleCount(): number;
    isDark(r: number, c: number): boolean;
  };
  const qr = qrcode(0, "M");
  qr.addData(text);
  qr.make();
  const n = qr.getModuleCount();
  const cell = 20;
  const quiet = 4;
  const size = (n + 2 * quiet) * cell;
  let rects = "";
  for (let r = 0; r < n; r++) for (let c = 0; c < n; c++) if (qr.isDark(r, c)) rects += `<rect x="${(c + quiet) * cell}" y="${(r + quiet) * cell}" width="${cell}" height="${cell}"/>`;
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" shape-rendering="crispEdges"><rect width="100%" height="100%" fill="#fff"/>${rects}</svg>`;
  const p = await owner.newPage();
  await p.setViewportSize({ width: size, height: size });
  await p.setContent(`<body style="margin:0;background:#fff">${svg}</body>`);
  await p.screenshot({ path: file });
  await p.close();
}

test.beforeAll(async ({ browser }) => {
  test.setTimeout(600_000);
  if (!existsSync(APK)) throw new Error(`no APK at ${APK}: run ./gradlew :app:assembleDebug in mobile/android`);
  if (process.env.ANDROID_SERIAL === undefined) process.env.ANDROID_SERIAL = serial;
  mkdirSync(OUT, { recursive: true });
  let info = A.emulatorInfo(serial);
  if (!info) throw new Error("cannot find the emulator's discovery file (pid_<pid>.ini) for its gRPC port");
  const args = A.emulatorArgs(info);
  const back = args.indexOf("-camera-back");
  if (back < 0 || args[back + 1] !== "virtualscene") {
    console.log(`android-camera: restarting the emulator with -camera-back virtualscene (was: ${args.join(" ")})`);
    restoreArgs = args;
    launcherDir = info.launcherDir;
    const withCamera = back < 0 ? [...args, "-camera-back", "virtualscene"] : args.map((a, i) => (i === back + 1 ? "virtualscene" : a));
    await A.stopEmulator(info);
    await A.startEmulator(launcherDir, withCamera, join(OUT, "emulator.log"));
    info = A.emulatorInfo(serial);
    if (!info) throw new Error("the restarted emulator has no discovery file");
  }
  grpc = new A.EmulatorGrpc(info);
  // Only the wall poster: the table's keeps its default picture.
  A.adb("emu", "virtualscene-image", "table");
  freshApp(APK);
  probe = new A.Probe();
  probe.start();
  recorder = new A.ScreenRecorder("golink-camera");
  recorder.start();
  owner = await trackedContext(browser);
  page = await owner.newPage();
});

test.afterAll(async () => {
  test.setTimeout(600_000);
  try {
    snap("end");
    A.shell(`am force-stop ${A.PACKAGE}`);
  } catch {
    // the device went away
  }
  probe?.stop();
  const videos = recorder ? await recorder.stop(OUT) : [];
  await owner?.close();
  try {
    A.adb("emu", "virtualscene-image", "wall");
    await grpc?.setPhysicalModel(A.PhysicalType.POSITION, [0, 0, 0]);
    await grpc?.setPhysicalModel(A.PhysicalType.ROTATION, [0, 0, 0]);
  } catch {
    // restarted below anyway, or gone
  }
  if (restoreArgs) {
    const info = A.emulatorInfo(serial);
    if (info) await A.stopEmulator(info);
    await A.startEmulator(launcherDir, restoreArgs, join(OUT, "emulator.log"));
    console.log(`android-camera: the emulator runs again as before: ${restoreArgs.join(" ")}`);
  }
  const evidence = process.env.E2E_EVIDENCE_DIR;
  if (evidence) {
    mkdirSync(evidence, { recursive: true });
    cpSync(OUT, evidence, { recursive: true });
  }
  console.log(`android-camera evidence: ${OUT} (${videos.length} recording chunk(s))`);
});

test("the owner opens the test pattern room and the app uses the test signalhub", async () => {
  await ownerOpensTestPattern(page);
  await pointAppAt(SIGNAL_URL);
  A.shell("input keyevent 4");
  await A.waitFor({ id: "home-code" });
});

test("the scanner reads the invitation's QR code with the camera, then the PIN joins", async () => {
  const inv = await invite(page);
  // The website's own link (the test stack's address) is not a go-link
  // invitation for the app; the same token on go-link.org is.
  const foreign = join(OUT, "qr-site-link.png");
  const real = join(OUT, "qr-invitation.png");
  await qrPng(inv.url, foreign);
  const link = `https://play.go-link.org/g/${inv.invite}`;
  await qrPng(link, real);

  // The virtual device faces the wall poster.
  const yaw = (POSTER.yaw * Math.PI) / 180;
  const normal = [Math.sin(yaw), 0, Math.cos(yaw)]; // the way the poster faces
  await grpc.setPhysicalModel(A.PhysicalType.ROTATION, [0, POSTER.yaw, 0]);
  await grpc.setPhysicalModel(A.PhysicalType.POSITION, [POSTER.x + normal[0]! * DISTANCE, POSTER.y, POSTER.z + normal[2]! * DISTANCE]);
  A.adb("emu", "virtualscene-image", "wall", foreign);

  // Home -> Scan: the explanation, then the system's camera permission.
  await A.tapOn({ text: "Scan QR code" });
  await A.tapOn({ text: "Allow" });
  await A.tapOn({ id: "com.android.permissioncontroller:id/permission_allow_foreground_only_button" });
  // The camera sees a QR code that is not a go-link invitation: refused, and the scanner goes on.
  await A.waitFor({ text: "That QR code is not a go-link invitation." }, 60_000);
  await sleep(1000);
  snap("scanner-sees-foreign-qr");
  expect(A.find({ id: "join-pin" })).toBeUndefined();

  // The real invitation on the poster: the PIN screen opens with it.
  const mark = probe.mark();
  const swapped = Date.now();
  A.adb("emu", "virtualscene-image", "wall", real);
  // The emulator loads the new poster in the background: allow it time.
  await A.waitFor({ text: link }, 180_000);
  console.log(`the invitation's QR code was read ${((Date.now() - swapped) / 1000).toFixed(1)} s after it was put on the wall`);
  snap("scanned-pin-form");
  await joinWithPin(inv.pin);
  const seated = await probe.waitFor((e) => e.ev === "ui" && e.phase === "STREAMING" && /^P\d/.test(String(e.me)), 60_000, mark);
  await enterAlias();
  await A.waitFor({ id: "room-seat", text: `You are ${String(seated.me)}` });
  snap("joined-after-scan");
});
