// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { expect, test, type Browser, type BrowserContext, type Page } from "@playwright/test";
import { cpSync, existsSync, mkdirSync, readFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import * as A from "../android";
import { enterAlias, freshApp, invite as ownerInvite, joinAsBrowserGuest, joinWithPin, ownerOpensTestPattern, ownSeat, pointAppAt, trackedContext, wakeStage } from "../android-owner";
import { PORTS } from "../ports";
import { stack } from "../stack";

// The go-link Player Android app against the test stack (its own signalhub,
// headless device and website). The owner is a Chromium page that links the
// device, opens the test pattern room (taking P1, with Chromium's fake
// microphone tone) and makes invitations; the app, driven through adb and
// UiAutomator, joins as a guest. The app's debug build logs its room events
// and WebRTC counters (tag GoLinkE2E), and the device runs with --debug so
// its log shows every input packet.
//
// Run with: npm run test:android (needs an emulator or phone in `adb
// devices` and the debug APK: ./gradlew :app:assembleDebug in
// mobile/android). Skipped when no device is attached.

const here = dirname(fileURLToPath(import.meta.url));
const APK = process.env.ANDROID_APK || resolve(here, "../../mobile/android/app/build/outputs/apk/debug/app-debug.apk");
// The emulator reaches the computer at 10.0.2.2; a phone can use
// `adb reverse tcp:8191 tcp:8191` and ANDROID_SIGNAL_URL=ws://127.0.0.1:8191/ws.
const SIGNAL_URL = process.env.ANDROID_SIGNAL_URL || `ws://10.0.2.2:${PORTS.signal}/ws`;
const OUT = resolve(here, "../test-results/android");

const serial = A.attachedDevice();
test.skip(!serial, "no Android device in `adb devices`");
test.describe.configure({ mode: "serial" });

let owner: BrowserContext;
let page: Page;
let browserRef: Browser;
/** Browser guests (their own invitations) that fill the seats. */
const guests: { context: BrowserContext; page: Page }[] = [];
let probe: A.Probe;
let recorder: A.ScreenRecorder;
let shot = 0;
let appPort = 0;
let ownerPort = 0;
const usedPins: string[] = [];

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

function snap(name: string) {
  A.screenshot(join(OUT, `${String(++shot).padStart(2, "0")}-${name}.png`));
}

/** The device log from `from` (a byte offset) on. */
function deviceLog(from = 0): string {
  return readFileSync(stack().deviceLog, "utf8").slice(from);
}

test.beforeAll(async ({ browser }) => {
  if (!existsSync(APK)) throw new Error(`no APK at ${APK}: run ./gradlew :app:assembleDebug in mobile/android`);
  if (process.env.ANDROID_SERIAL === undefined) process.env.ANDROID_SERIAL = serial;
  mkdirSync(OUT, { recursive: true });
  freshApp(APK);
  probe = new A.Probe();
  probe.start();
  recorder = new A.ScreenRecorder();
  recorder.start();
  browserRef = browser;
  owner = await trackedContext(browser);
  page = await owner.newPage();
});

test.afterAll(async () => {
  try {
    A.shell("cmd connectivity airplane-mode disable || true");
    snap("end");
    A.shell(`am force-stop ${A.PACKAGE}`);
  } catch {
    // the device went away
  }
  probe?.stop();
  for (const g of guests) await g.context.close().catch(() => undefined);
  const videos = recorder ? await recorder.stop(OUT) : [];
  await owner?.close();
  // Optional copy of the evidence (screen recording and screenshots).
  const evidence = process.env.E2E_EVIDENCE_DIR;
  if (evidence) {
    mkdirSync(evidence, { recursive: true });
    cpSync(OUT, evidence, { recursive: true });
  }
  console.log(`android evidence: ${OUT} (${videos.length} recording chunk(s))`);
});

/** A fresh one-person invitation from the owner's Invite dialog. */
const invite = () => ownerInvite(page);

/** Waits until the app streams from a seat; returns its port. */
async function waitSeated(from: number): Promise<number> {
  const e = await probe.waitFor((x) => x.ev === "ui" && x.phase === "STREAMING" && /^P\d/.test(String(x.me)), 60_000, from);
  return Number(String(e.me).slice(1, 2));
}

/** Sums a counter of one track over the app's latest stats line. */
function appIn(track: string, key: keyof A.TrackStats): number {
  return Number(probe.lastStats()?.in?.[track]?.[key] ?? 0);
}

async function leaveRoom() {
  A.hideKeyboard();
  await A.tapOn({ desc: "Leave" });
  await A.waitFor({ id: "home-code" });
}

/** Clicks a control on the owner's video dock, waking the auto-hidden overlays first. */
async function clickOnStage(target: ReturnType<Page["locator"]>) {
  await wakeStage(page);
  await target.click();
}

/** The owner's inbound RTP packets for a stream id (e.g. voice-p2), from the SDP's msid lines. */
async function ownerInbound(stream: string): Promise<number> {
  return page.evaluate(async (want) => {
    let total = 0;
    for (const pc of (window as unknown as { __pcs: RTCPeerConnection[] }).__pcs) {
      if (pc.connectionState === "closed" || !pc.remoteDescription) continue;
      const mids = new Map<string, string>();
      for (const section of pc.remoteDescription.sdp.split(/\r?\nm=/)) {
        const mid = section.match(/a=mid:(\S+)/)?.[1];
        const msid = section.match(/a=msid:(\S+)/)?.[1];
        if (mid && msid) mids.set(mid, msid);
      }
      const stats = await pc.getStats();
      stats.forEach((s: { type: string; kind?: string; mid?: string; packetsReceived?: number }) => {
        if (s.type === "inbound-rtp" && s.kind === "audio" && s.mid && mids.get(s.mid) === want) total += s.packetsReceived ?? 0;
      });
    }
    return total;
  }, stream);
}

test("the owner links the device and opens the test pattern room", async () => {
  await ownerOpensTestPattern(page);
});

test("the app is pointed at the local signalhub from Settings (debug build only)", async () => {
  await pointAppAt(SIGNAL_URL);
  snap("settings-local-server");
  A.shell("input keyevent 4");
  await A.waitFor({ id: "home-code" });
});

test("joins by App Link with the PIN and the terms, takes a seat and shows the picture", async () => {
  const inv = await invite();
  usedPins.push(inv.pin);
  const from = probe.mark();
  // The link carries only the invitation; the PIN is typed in the app.
  A.shell(`am start -a android.intent.action.VIEW -d https://play.go-link.org/g/${inv.invite} ${A.PACKAGE}`);
  await A.waitFor({ text: `https://play.go-link.org/g/${inv.invite}` });
  snap("app-link-pin-form");
  await joinWithPin(inv.pin);
  appPort = await waitSeated(from);
  snap("alias-step");
  await enterAlias();
  const seat = await A.waitFor({ id: "room-seat", text: `You are P${appPort}` });
  const ui = probe.lastUi()!;
  ownerPort = Number(String(ui.seats).match(/P(\d)=(?!-)\S+/g)?.map((s) => Number(s[1])).find((p) => p !== appPort) ?? 0);
  expect(ownerPort).toBeGreaterThan(0);
  // The picture: the area between the title and the dock is lit and varied (not black).
  await expect.poll(() => appIn("video", "frames"), { timeout: 30_000 }).toBeGreaterThan(30);
  await sleep(1500);
  const dock = await A.waitFor({ id: "dock-mic" });
  const picture = A.pictureStats([0, seat.bounds[3] + 8, 1_000_000, dock.bounds[1] - 8]);
  snap("room-streaming");
  console.log(`app P${appPort} (owner P${ownerPort}); picture lit ${picture.litShare.toFixed(2)}, ${picture.colors} colors; path ${probe.lastStats()?.path}`);
  expect(picture.litShare).toBeGreaterThan(0.25);
  expect(picture.colors).toBeGreaterThan(8);
});

test("the on-screen gamepad reaches the device", async () => {
  const offset = deviceLog().length;
  const dpad = await A.waitFor({ id: "pad-dpad" });
  const [l, t, r, b] = dpad.bounds;
  A.hold([Math.round((l + r) / 2), t + Math.round((b - t) * 0.15)], 400); // up
  A.hold([l + Math.round((r - l) * 0.85), Math.round((t + b) / 2)], 400); // right
  A.hold(A.center(await A.waitFor({ id: "pad-button-1" })), 400);
  A.hold(A.center(await A.waitFor({ id: "pad-coin" })), 400);
  snap("gamepad");
  await expect
    .poll(() => {
      const lines = [...deviceLog(offset).matchAll(/msg=input peer_id=\S+ player=0 buttons=(\S+)/g)].map((m) => m[1]);
      return ["up", "right", "b1", "coin"].filter((btn) => lines.includes(btn));
    }, { timeout: 15_000 })
    .toEqual(["up", "right", "b1", "coin"]);
});

test("the game's sound arrives and is decoded (inbound RTP and audio level)", async () => {
  const p0 = appIn("game", "packets");
  const e0 = appIn("game", "energy");
  let maxLevel = 0;
  const end = Date.now() + 8_000;
  while (Date.now() < end) {
    maxLevel = Math.max(maxLevel, appIn("game", "level"));
    await sleep(500);
  }
  const p1 = appIn("game", "packets");
  const e1 = appIn("game", "energy");
  console.log(`game audio: packets ${p0} -> ${p1}, energy ${e0.toFixed(3)} -> ${e1.toFixed(3)}, max level ${maxLevel.toFixed(3)}`);
  expect(p1 - p0).toBeGreaterThan(100); // 50 packets per second of Opus
  expect(e1).toBeGreaterThan(e0);
  expect(maxLevel).toBeGreaterThan(0);
});

test("the Sound sheet lists Automatic and the loudspeaker and plays the test chime", async () => {
  await A.tapOn({ id: "dock-players" });
  await A.tapOn({ id: "players-sound" });
  await A.waitFor({ id: "sound-sheet" });
  // Each row is a tagged node with its label inside.
  for (const id of ["sound-in-auto", "sound-in-phone", "sound-out-auto", "sound-out-speaker"]) await A.waitFor({ id });
  await A.waitFor({ text: "Automatic" });
  await A.waitFor({ text: "Speaker" });
  await A.tapOn({ id: "sound-test" });
  snap("sound-sheet");
  A.shell("input keyevent 4"); // close the sheet
  await A.waitGone({ id: "sound-sheet" });
  await A.waitFor({ id: "pad-dpad" });
});

test("chat goes both ways", async () => {
  const fromOwner = `owner-says-hi-${Date.now() % 100000}`;
  const fromApp = `android-says-hi-${Date.now() % 100000}`;
  const mark = probe.mark();
  const chat = page.locator("#chatmsg");
  if (!(await chat.isVisible())) await page.getByRole("button", { name: "Show the chat" }).click();
  await chat.fill(fromOwner);
  await chat.press("Enter");
  await probe.waitFor((e) => e.ev === "chat" && e.text === fromOwner, 20_000, mark);

  await A.tapOn({ id: "dock-chat" });
  await A.waitFor({ text: fromOwner });
  await A.fill({ id: "chat-input" }, fromApp);
  // The keyboard commits typed text a little later on a slow emulator: send
  // only once the whole line is in the field (a cut line never matches).
  await A.waitFor({ id: "chat-input", text: fromApp });
  // A slow CI emulator can miss the tap: send again until the device echoes
  // the line back (an empty input sends nothing, so nothing doubles).
  for (let i = 0; ; i++) {
    await A.tapOn({ id: "chat-send" });
    try {
      await probe.waitFor((e) => e.ev === "chat" && e.text === fromApp, 15_000, mark);
      break;
    } catch (err) {
      if (i === 2) throw err;
    }
  }
  await expect(page.getByText(fromApp)).toBeVisible({ timeout: 30_000 });
  await probe.waitFor((e) => e.ev === "chat" && e.text === fromApp, 20_000, mark);
  A.hideKeyboard();
  snap("chat");
  A.shell("input keyevent 4"); // close the sheet
  await A.waitFor({ id: "pad-dpad" });
});

test("voice goes both ways between the seated players", async () => {
  // The owner's fake microphone (a tone) -> the device -> the app's voice-pN track.
  const mic = page.locator("button.mic-toggle");
  await clickOnStage(mic);
  await expect(mic).toHaveAttribute("aria-pressed", "true");
  const track = `voice-p${ownerPort}`;
  const v0 = appIn(track, "packets");
  await expect.poll(() => appIn(track, "packets"), { timeout: 20_000 }).toBeGreaterThan(v0 + 100);
  console.log(`app ${track}: packets ${v0} -> ${appIn(track, "packets")}, level ${appIn(track, "level")}`);

  // The app's microphone -> the device -> the owner's voice-p<app> track.
  A.shell(`pm grant ${A.PACKAGE} android.permission.RECORD_AUDIO`);
  const mark = probe.mark();
  await A.tapOn({ id: "dock-mic" });
  await probe.waitFor((e) => e.ev === "ui" && e.mic === true, 15_000, mark);
  const ownerTrack = `voice-p${appPort}`;
  const o0 = await ownerInbound(ownerTrack);
  await expect.poll(() => Number(probe.lastStats()?.out?.mic?.packets ?? 0), { timeout: 20_000 }).toBeGreaterThan(50);
  await expect.poll(() => ownerInbound(ownerTrack), { timeout: 20_000 }).toBeGreaterThan(o0 + 50);
  console.log(`app mic out ${probe.lastStats()?.out?.mic?.packets} packets; owner ${ownerTrack}: ${o0} -> ${await ownerInbound(ownerTrack)}`);
  snap("voice");
  await A.tapOn({ id: "dock-mic" });
  await clickOnStage(mic);
  await expect(mic).toHaveAttribute("aria-pressed", "false");
});

test("after the network drops the app comes back to its seat without the PIN (return token)", async () => {
  const mark = probe.mark();
  A.shell("cmd connectivity airplane-mode enable");
  try {
    await probe.waitFor((e) => e.ev === "ui" && e.phase !== "STREAMING", 90_000, mark);
    snap("offline");
  } finally {
    A.shell("cmd connectivity airplane-mode disable");
  }
  await waitSeated(mark);
  const events = probe.events.slice(mark).filter((e) => e.ev === "ui");
  // It never stopped to ask for a PIN: the device's return token was used.
  expect(events.some((e) => e.pin_needed === "true" && e.pin_busy === "false")).toBe(false);
  expect(A.find({ id: "room-pin" })).toBeUndefined();
  console.log(`reconnect: ${events.map((e) => `${e.phase}${e.reconnecting === "true" ? "(reconnecting)" : ""}`).join(" > ")}`);
  snap("back-after-network-drop");
});

test("a wrong PIN and an already used PIN are refused; the new invitation's PIN gets in", async () => {
  await leaveRoom();
  const inv = await invite();
  let wrong = String((Number(inv.pin) + 1) % 1_000_000).padStart(6, "0");
  while ([...usedPins, inv.pin].includes(wrong)) wrong = String((Number(wrong) + 7) % 1_000_000).padStart(6, "0");

  // A new join always asks the PIN (the old return token is not used).
  let mark = probe.mark();
  A.shell(`am start -a android.intent.action.VIEW -d https://play.go-link.org/g/${inv.invite} ${A.PACKAGE}`);
  await A.waitFor({ id: "join-pin" });
  await joinWithPin(wrong);
  const refused = await probe.waitFor((e) => e.ev === "ui" && e.pin_busy === "false" && typeof e.pin_reason === "string", 30_000, mark);
  await A.waitFor({ id: "room-pin-error" });
  snap("wrong-pin");
  console.log(`wrong PIN refused: ${refused.pin_reason}`);
  expect(refused.pin_reason).not.toBe("used");

  // The first invitation's PIN was already used by this app's earlier visit.
  mark = probe.mark();
  await A.fill({ id: "room-pin" }, usedPins[0]!);
  A.hideKeyboard();
  await A.tapOn({ id: "room-pin-send" });
  await probe.waitFor((e) => e.ev === "ui" && e.pin_busy === "false" && e.pin_reason === "used", 30_000, mark);
  await A.waitFor({ id: "room-pin-error" });
  snap("used-pin");

  mark = probe.mark();
  await A.fill({ id: "room-pin" }, inv.pin);
  A.hideKeyboard();
  await A.tapOn({ id: "room-pin-send" });
  usedPins.push(inv.pin);
  appPort = await waitSeated(mark);
  await enterAlias();
  snap("admitted-after-refusals");
});

test("joins with the 9-digit code typed on the home screen", async () => {
  await leaveRoom();
  const inv = await invite();
  const mark = probe.mark();
  await A.tapOn({ id: "home-code" });
  await A.fill({ id: "join-code" }, inv.code);
  await joinWithPin(inv.pin);
  usedPins.push(inv.pin);
  appPort = await waitSeated(mark);
  await enterAlias();
  await A.waitFor({ id: "room-seat", text: `You are P${appPort}` });
  await expect.poll(() => appIn("video", "frames"), { timeout: 30_000 }).toBeGreaterThan(30);
  snap("code-join-streaming");
});

/** Seats taken in the app's latest room state (its GoLinkE2E "seats" summary). */
function seatedCount(): number {
  return (String(probe.lastUi()?.seats ?? "").match(/P\d=(?!-)\S/g) ?? []).length;
}

/**
 * The picture's rectangle in portrait: the room's screen box spans the
 * width, sits right above the dock (4 dp of padding above its buttons) and
 * has the picture's aspect (the test card is 640x480, 4:3), so the video
 * fills it. The video itself (a TextureView) is not in the UiAutomator tree.
 */
async function videoRect(): Promise<[number, number, number, number]> {
  // The picture's own view: the whole 4:3 test card, with or without a skin around it.
  const [l, t, r, b] = (await A.waitFor({ id: "picture" })).bounds;
  const h = Math.round(((r - l) * 3) / 4);
  const top = Math.round((t + b) / 2 - h / 2);
  return [l, top, r, top + h];
}

/**
 * Where the test card draws a player lamp (1-4), as a screen rectangle
 * inside the picture. The geometry follows pkg/testpattern (drawCard: a
 * circle of radius 0.448 h; the gamepad box; drawPad: the lamps between
 * the shoulders). The picture is stretched to its display aspect, so the
 * whole frame maps onto the rectangle.
 */
function lampRect(video: [number, number, number, number], port: number): [number, number, number, number] {
  const [l, t, r, b] = video;
  const w = r - l;
  const h = b - t;
  const k = h / w; // frame height / width
  const radius = 0.448;
  const padX = 0.5 - 0.74 * radius * k;
  const padW = 1.48 * radius * k;
  const padY = 0.5 - radius + 0.615 * 2 * radius;
  const padH = 0.27 * 2 * radius;
  const unit = padH / 8;
  const cx = l + (padX + (0.38 + (port - 1) * 0.08) * padW) * w;
  const cy = t + (padY + 0.08 * padH + unit / 2) * h;
  const half = Math.max(2, Math.round((unit * h) / 4));
  return [Math.round(cx) - half, Math.round(cy) - half, Math.round(cx) + half, Math.round(cy) + half];
}

/** The lamp's lit color is the design accent (orange); off is dark gray. */
const lampLit = ([r, g, b]: [number, number, number]) => r > 150 && r - b > 70 && g > 90;

test("the 1P..NP start buttons reach the device and light the test card's player lamps", async () => {
  // One start button per seated player (startButtonCount), like the website.
  const seated = seatedCount();
  expect(seated).toBeGreaterThanOrEqual(2);
  const pills = A.dump()
    .filter((n) => /^pad-start-\d$/.test(n.id))
    .map((n) => n.id)
    .sort();
  expect(pills).toEqual(Array.from({ length: Math.min(4, seated) }, (_, i) => `pad-start-${i + 1}`));
  const video = await videoRect();
  for (const port of [1, 2]) {
    const offset = deviceLog().length;
    const rect = lampRect(video, port);
    const before = A.averageColor(rect);
    const lifted = A.holdAsync(A.center(await A.waitFor({ id: `pad-start-${port}` })), 3000);
    await sleep(1500);
    const during = A.averageColor(rect);
    snap(`start-${port}P-held`);
    await lifted;
    await expect
      .poll(() => [...deviceLog(offset).matchAll(/msg=input peer_id=\S+ player=0 buttons=(\S+)/g)].some((m) => m[1]!.split("+").includes(`start${port}`)), {
        timeout: 15_000,
      })
      .toBe(true);
    await sleep(1000);
    const after = A.averageColor(rect);
    const fmt = (c: number[]) => c.map((v) => Math.round(v)).join(",");
    console.log(`${port}P: lamp ${port} before ${fmt(before)}, held ${fmt(during)}, after ${fmt(after)}`);
    expect(lampLit(during)).toBe(true);
    expect(lampLit(after)).toBe(false);
  }
});

test("with the four seats taken the app waits in the queue, takes the freed seat, and can just watch and come back", async () => {
  await leaveRoom();
  // The owner holds P1; three browser guests with their own invitations take the rest.
  for (let i = 0; i < 3; i++) {
    const inv = await invite();
    guests.push(await joinAsBrowserGuest(browserRef, inv.code, inv.pin));
  }
  await expect.poll(() => page.locator(".players-capsule .capsule-avatar.is-free").count(), { timeout: 60_000 }).toBe(0);
  for (const g of guests) await expect.poll(() => ownSeat(g.page), { timeout: 30_000 }).toBeGreaterThan(0);

  const inv = await invite();
  usedPins.push(inv.pin);
  let mark = probe.mark();
  A.shell(`am start -a android.intent.action.VIEW -d https://play.go-link.org/g/${inv.invite} ${A.PACKAGE}`);
  await A.waitFor({ id: "join-pin" });
  await joinWithPin(inv.pin);
  await probe.waitFor((e) => e.ev === "ui" && e.phase === "STREAMING" && e.me === "queue:1", 60_000, mark);
  await enterAlias();
  await A.waitFor({ id: "room-seat", text: "You are #1 in the queue" });
  expect(seatedCount()).toBe(4);
  // The queue still watches the picture.
  await expect.poll(() => appIn("video", "frames"), { timeout: 30_000 }).toBeGreaterThan(30);
  snap("queue");

  // A guest leaves: the head of the queue takes the freed seat.
  const leaving = guests.shift()!;
  const freed = await ownSeat(leaving.page);
  mark = probe.mark();
  await leaving.context.close();
  appPort = await waitSeated(mark);
  expect(appPort).toBe(freed);
  await A.waitFor({ id: "room-seat", text: `You are P${appPort}` });
  snap("seated-from-queue");

  // Just watch: the seat is given back; then join again.
  await A.tapOn({ id: "dock-players" });
  mark = probe.mark();
  await A.tapOn({ text: "Just watch" });
  await probe.waitFor((e) => e.ev === "ui" && e.me === "spectator", 20_000, mark);
  await A.waitFor({ text: "Watching" });
  await A.waitFor({ text: / \(you\)$/ });
  await A.waitFor({ text: "Join the queue" });
  snap("spectate");
  mark = probe.mark();
  await A.tapOn({ text: "Join the queue" });
  appPort = await waitSeated(mark);
  A.shell("input keyevent 4"); // close the sheet
  await A.waitFor({ id: "room-seat", text: `You are P${appPort}` });
  snap("back-from-spectating");
});

test("seats are swapped between the app and a browser guest, asked from either side", async () => {
  const g = guests[0]!;
  const guestPort = await ownSeat(g.page);
  const myPort = appPort;
  expect(guestPort).toBeGreaterThan(0);
  expect(guestPort).not.toBe(myPort);

  // The app asks (swap_seat); the guest accepts on the website (swap_answer).
  await A.tapOn({ id: "dock-players" });
  await A.tapOn({ text: `Swap with P${guestPort}` });
  await A.waitFor({ text: `Waiting for P${guestPort} to answer` });
  snap("swap-asked-by-app");
  const offer = g.page.getByRole("alertdialog", { name: /wants to swap controllers/ });
  await expect(offer).toBeVisible();
  let mark = probe.mark();
  await offer.getByRole("button", { name: "Swap", exact: true }).click();
  await probe.waitFor((e) => e.ev === "ui" && e.me === `P${guestPort}`, 20_000, mark);
  await expect.poll(() => ownSeat(g.page)).toBe(myPort);
  A.shell("input keyevent 4"); // close the sheet
  await A.waitFor({ id: "room-seat", text: `You are P${guestPort}` });

  // The guest asks back from the players capsule; the app accepts over the picture.
  // The offer lasts 30 s on the device; a slow emulator can miss it, so the
  // guest asks again (up to three times) until the app's Accept lands.
  for (let i = 0; ; i++) {
    await wakeStage(g.page);
    await g.page.getByRole("button", { name: new RegExp(`^P${guestPort} · `) }).click();
    await g.page.getByRole("menuitem", { name: `Swap controllers: go to P${guestPort}` }).click();
    try {
      await A.waitFor({ text: /asks to swap controllers/ });
      if (i === 0) snap("swap-offer-on-app");
      mark = probe.mark();
      await A.tapOn({ text: "Accept" });
      await probe.waitFor((e) => e.ev === "ui" && e.me === `P${myPort}`, 20_000, mark);
      break;
    } catch (err) {
      if (i === 2) throw err;
      await g.page.keyboard.press("Escape").catch(() => {});
    }
  }
  await expect.poll(() => ownSeat(g.page)).toBe(guestPort);
  await A.waitFor({ id: "room-seat", text: `You are P${myPort}` });
  appPort = myPort;
});
