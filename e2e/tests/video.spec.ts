// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { expect, test, type Browser, type BrowserContext, type Page } from "@playwright/test";
import { copyFileSync, existsSync, mkdirSync, readdirSync, rmSync, statSync } from "node:fs";
import { homedir, hostname, userInfo } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { MAKER_URL, PORTS } from "../ports";
import { pairingCode } from "../stack";
import { en } from "../../frontend/apps/web/src/i18n/en";
import { es } from "../../frontend/apps/web/src/i18n/es";
import { pt } from "../../frontend/apps/web/src/i18n/pt";
import { coreEn } from "../../frontend/willy-maker/src/maker/i18n/core.en";
import { coreEs } from "../../frontend/willy-maker/src/maker/i18n/core.es";
import { corePt } from "../../frontend/willy-maker/src/maker/i18n/core.pt";
import { exportEn } from "../../frontend/willy-maker/src/maker/i18n/export.en";
import { exportEs } from "../../frontend/willy-maker/src/maker/i18n/export.es";
import { exportPt } from "../../frontend/willy-maker/src/maker/i18n/export.pt";

// The takes of go-link's trailer and demo video (npm run video): real pages
// of the test stack, recorded by Playwright, one WebM per take, which the
// Remotion project in video/ puts together with its titles. Like the
// landing's screenshots it runs with E2E_SHOTS=1 (an invented ROM library)
// and replaces every private string on the page. Games on screen are only
// the test pattern and a game made with Willy Maker (E2E_CORE_DIR, for the
// emulator core): never a commercial game.
//
// VIDEO_LANG (en, es or pt; English by default) records the website and
// Willy Maker in that language, for the trailer in that language.
//
// Output: video/public/takes/<lang>/<take>.webm (gitignored).

const here = dirname(fileURLToPath(import.meta.url));
const root = resolve(here, "../..");
const RAW = resolve(here, "../test-results/video-takes");
const LANG = (["es", "pt"].includes(process.env.VIDEO_LANG ?? "") ? process.env.VIDEO_LANG : "en") as "en" | "es" | "pt";
const OUT = join(root, "video/public/takes", LANG);

// The page's words in LANG: each English text is found in the English
// dictionaries and read at the same place in LANG's (the website's, and
// Willy Maker's core and export parts).
const DICTS: [unknown, unknown][] = [
  [en, { en, es, pt }[LANG]],
  [coreEn, { en: coreEn, es: coreEs, pt: corePt }[LANG]],
  [exportEn, { en: exportEn, es: exportEs, pt: exportPt }[LANG]],
];
function pathOf(o: unknown, text: string, partial: boolean): (string | number)[] | null {
  if (typeof o === "string") return o === text || (partial && o.includes(text)) ? [] : null;
  if (o && typeof o === "object") {
    for (const [k, v] of Object.entries(o)) {
      const p = pathOf(v, text, partial);
      if (p) return [Array.isArray(o) ? Number(k) : k, ...p];
    }
  }
  return null;
}
function tr(text: string, partial = false): string {
  if (LANG === "en") return text;
  for (const [from, to] of DICTS) {
    const path = pathOf(from, text, partial);
    if (!path) continue;
    const value = path.reduce<unknown>((o, k) => (o as Record<string | number, unknown>)?.[k], to);
    if (typeof value === "string") return partial ? value.trim() : value;
  }
  throw new Error(`no ${LANG} text for "${text}"`);
}
/** The website's words in LANG, for texts used in several places. */
const W = { en, es, pt }[LANG];

/** A name that contains the (translated) words. */
const has = (text: string) => new RegExp(tr(text, true).replace(/[.*+?^${}()|[\]\\]/g, "\\$&"));
const SIZE = { width: 1920, height: 1080 };

const home = homedir();
const short = hostname().split(".")[0]!;
const MASK: [string, string][] = [
  [join(here, "..", ".state", "home"), "~"],
  [root, "~/go-link"],
  [home, "~"],
  [hostname(), "arcade-pc"],
  [short, "arcade-pc"],
  [userInfo().username, "player"],
  [`ws://127.0.0.1:${PORTS.signal}/ws`, "wss://signal.go-link.org/ws"],
  [`http://localhost:${PORTS.web}`, "https://console.go-link.org"],
  [`localhost:${PORTS.web}`, "console.go-link.org"],
  [`127.0.0.1:${PORTS.signal}`, "signal.go-link.org"],
  ["Metal Slug, Turtles…", "Sky Pirates, Glacier…"],
].sort((a, b) => b[0].length - a[0].length) as [string, string][];

test.describe.configure({ mode: "serial" });

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));
let linked: Awaited<ReturnType<BrowserContext["storageState"]>> | null = null;
let roomHref = "";

/** Replaces private strings in the page, now and on every change. */
async function addMask(context: BrowserContext) {
  await context.addInitScript((pairs: [string, string][]) => {
    const fix = (s: string) => pairs.reduce((acc, [a, b]) => acc.split(a).join(b), s);
    const run = (rootNode: Node) => {
      const tw = document.createTreeWalker(rootNode, NodeFilter.SHOW_TEXT);
      for (let n = tw.nextNode(); n; n = tw.nextNode()) {
        const v = fix(n.nodeValue ?? "");
        if (v !== n.nodeValue) n.nodeValue = v;
      }
      if (rootNode instanceof Element || rootNode instanceof Document) {
        rootNode.querySelectorAll("[title],[aria-label],[placeholder],input").forEach((el) => {
          for (const attr of ["title", "aria-label", "placeholder"]) {
            const v = el.getAttribute(attr);
            if (v !== null && fix(v) !== v) el.setAttribute(attr, fix(v));
          }
          if (el instanceof HTMLInputElement && fix(el.value) !== el.value) el.value = fix(el.value);
        });
      }
    };
    new MutationObserver(() => run(document)).observe(document, { subtree: true, childList: true, characterData: true, attributes: true, attributeFilter: ["title", "aria-label", "placeholder"] });
  }, MASK);
}

/** Records one take: a new browser context whose video is saved as <name>.webm. */
async function take(browser: Browser, name: string, body: (p: Page, ctx: BrowserContext) => Promise<void>, opts: { viewport?: { width: number; height: number }; mobile?: boolean; linked?: boolean } = {}) {
  const dir = join(RAW, name);
  rmSync(dir, { recursive: true, force: true });
  const viewport = opts.viewport ?? SIZE;
  const context = await browser.newContext({
    viewport,
    ...(opts.mobile ? { isMobile: true, hasTouch: true, deviceScaleFactor: 2 } : {}),
    recordVideo: { dir, size: viewport },
    ...(opts.linked !== false && linked ? { storageState: linked } : {}),
  });
  await addMask(context);
  await context.addInitScript((lang) => localStorage.setItem("go-link.lang", lang), LANG);
  context.setDefaultTimeout(30_000);
  const page = await context.newPage();
  try {
    await body(page, context);
  } finally {
    await context.close();
  }
  // One WebM per page, in the order they opened: <name>.webm, then <name>-2.webm...
  const files = readdirSync(dir)
    .filter((f) => f.endsWith(".webm"))
    .map((f) => ({ f, at: statSync(join(dir, f)).birthtimeMs }))
    .sort((a, b) => a.at - b.at);
  if (files.length === 0) throw new Error(`no video for ${name}`);
  mkdirSync(OUT, { recursive: true });
  files.forEach(({ f }, i) => copyFileSync(join(dir, f), join(OUT, i === 0 ? `${name}.webm` : `${name}-${i + 1}.webm`)));
}

async function noSplash(p: Page) {
  await expect(p.locator("#splash")).toHaveCount(0, { timeout: 30_000 });
}

async function expectVideoPlaying(p: Page) {
  await expect
    .poll(async () => p.evaluate(() => {
      const v = document.querySelector("video.video") as HTMLVideoElement | null;
      return v ? v.videoWidth > 0 && v.readyState >= 2 && !v.paused : false;
    }), { timeout: 60_000 })
    .toBe(true);
}

/** A smooth scroll by dy pixels over ms. */
async function glide(p: Page, dy: number, ms: number) {
  const steps = Math.max(1, Math.round(ms / 16));
  for (let i = 0; i < steps; i++) {
    await p.mouse.wheel(0, dy / steps);
    await sleep(16);
  }
}

/** Holds a key for ms (the video shows it lit). */
async function hold(p: Page, key: string, ms: number) {
  await p.keyboard.down(key);
  await sleep(ms);
  await p.keyboard.up(key);
}

/** The pointer glides to a locator's middle (the cursor shows in the video), then clicks. */
async function point(p: Page, target: ReturnType<Page["locator"]>, click = true) {
  const box = await target.boundingBox();
  if (box) await p.mouse.move(box.x + box.width / 2, box.y + box.height / 2, { steps: 20 });
  await sleep(250);
  if (click) await target.click();
}

test.beforeAll(() => {
  rmSync(RAW, { recursive: true, force: true });
  mkdirSync(RAW, { recursive: true });
});

test("landing", async ({ browser }) => {
  await take(browser, "landing", async (p) => {
    await p.goto("/");
    await noSplash(p);
    await sleep(7000); // the pixel fight plays
    await glide(p, 900, 4000);
    await sleep(2500);
    await glide(p, 900, 4000);
    await sleep(2500);
  }, { linked: false });
});

test("link the device and its dashboard", async ({ browser }) => {
  await take(browser, "link", async (p, ctx) => {
    await p.goto("/device");
    await noSplash(p);
    await sleep(1200);
    // The code typed digit by digit, the way a person does.
    await p.locator("#d1").click();
    for (const d of pairingCode()) {
      await p.keyboard.type(d);
      await sleep(140);
    }
    await sleep(400);
    await point(p, p.getByRole("checkbox", { name: has("I have read and accept") }));
    await point(p, p.getByRole("button", { name: W.pairing.submit, exact: true }));
    await expect(p.getByText(W.dash.live)).toBeVisible();
    linked = await ctx.storageState();
    await sleep(1000);
    await p.getByRole("button", { name: tr("1 min"), exact: true }).click().catch(() => undefined);
    await sleep(9000); // the charts fill
    await point(p, p.locator('a[href="/device/roms"]').first());
    await expect.poll(() => p.locator("main img").count(), { timeout: 30_000 }).toBeGreaterThanOrEqual(6);
    await sleep(1500);
    await glide(p, 500, 2500);
    await sleep(2500);
    roomHref = "";
  });
});

let code = "";
let pin = "";

test("the test pattern room: controls, latency test and an invitation", async ({ browser }) => {
  await take(browser, "room", async (p) => {
    await p.goto("/device");
    await noSplash(p);
    roomHref = (await p.locator('main a[href^="/r/"]').first().getAttribute("href")) ?? "";
    expect(roomHref).toMatch(/^\/r\/[0-9a-f-]{36}$/);
    await p.goto(roomHref);
    await noSplash(p);
    await expectVideoPlaying(p);
    await sleep(2500);
    // Controls (open by default): the keyboard map lights up with every key.
    await point(p, p.getByRole("tab", { name: W.controls.title }).first());
    await sleep(1200);
    for (const k of ["ArrowRight", "ArrowRight", "KeyZ", "KeyX", "ArrowUp", "KeyC"]) await hold(p, k, 380), await sleep(120);
    // The latency test: the controllers on the video, each press measured.
    await point(p, p.getByRole("switch", { name: W.controls.hud }));
    await sleep(800);
    for (let i = 0; i < 8; i++) await hold(p, i % 2 ? "KeyX" : "ArrowLeft", 300), await sleep(350);
    await sleep(2500);
    await point(p, p.getByRole("switch", { name: W.controls.hud }));
    // An invitation: a link, a 9-digit code and a one-person PIN.
    await point(p, p.locator(`[data-tip="${W.invite.button}"]`).first());
    const dialog = p.getByRole("dialog");
    await expect(dialog.locator(".invite-code").nth(1)).toHaveText(/^\d{6}$/);
    code = (await dialog.locator(".invite-code").first().innerText()).replace(/\s/g, "");
    pin = (await dialog.locator(".invite-code").nth(1).innerText()).trim();
    await sleep(5000);
    await point(p, p.locator(".invite-dialog button.button-primary").last());
    await sleep(1500);
  });
});

test("a friend joins from the browser", async ({ browser }) => {
  // The host stays in the room (not recorded) so the friend has company.
  const host = await browser.newContext({ viewport: SIZE, storageState: linked ?? undefined });
  host.setDefaultTimeout(30_000);
  const hp = await host.newPage();
  await hp.goto(roomHref);
  await expectVideoPlaying(hp);
  await take(browser, "guest", async (p) => {
    await p.goto("/");
    await noSplash(p);
    await sleep(1500);
    await point(p, p.getByRole("button", { name: W.landing.joinCta }).first());
    const join = p.getByRole("dialog", { name: W.landing.joinTitle });
    await join.getByPlaceholder("123 456 789").click();
    await p.keyboard.type(code, { delay: 120 });
    await sleep(300);
    await join.getByPlaceholder("000000").click();
    await p.keyboard.type(pin, { delay: 140 });
    await point(p, join.getByRole("checkbox", { name: has("I have read and accept") }));
    await point(p, join.getByRole("button", { name: W.guest.join, exact: true }));
    const name = p.getByRole("dialog", { name: W.alias.title });
    await expect(name).toBeVisible({ timeout: 60_000 });
    await name.locator("input").first().click();
    await p.keyboard.type("Nico", { delay: 160 });
    await p.keyboard.press("Enter");
    await expectVideoPlaying(p);
    await sleep(2000);
    for (let i = 0; i < 6; i++) await hold(p, ["ArrowRight", "KeyZ", "ArrowUp"][i % 3]!, 350), await sleep(300);
    // A line in the chat from the host, seen by the friend.
    const message = hp.getByRole("textbox", { name: W.room.chatLabel });
    if (!(await message.isVisible())) await hp.getByRole("button", { name: W.room.chatShow }).click({ timeout: 5000 }).catch(() => undefined);
    await message.fill("Ready? Insert coin!", { timeout: 10_000 }).catch(() => undefined);
    await message.press("Enter", { timeout: 5000 }).catch(() => undefined);
    await sleep(5000);
  }, { linked: false });
  await host.close();
});

test("a phone becomes a console", async ({ browser }) => {
  // A new invitation for the phone, from the host's room.
  const host = await browser.newContext({ viewport: SIZE, storageState: linked ?? undefined });
  host.setDefaultTimeout(30_000);
  const hp = await host.newPage();
  await hp.goto(roomHref);
  await expectVideoPlaying(hp);
  await hp.locator(`[data-tip="${W.invite.button}"]`).first().click();
  const dialog = hp.getByRole("dialog");
  await expect(dialog.locator(".invite-code").nth(1)).toHaveText(/^\d{6}$/);
  const pcode = (await dialog.locator(".invite-code").first().innerText()).replace(/\s/g, "");
  const ppin = (await dialog.locator(".invite-code").nth(1).innerText()).trim();
  await take(browser, "phone", async (p) => {
    await p.goto("/");
    await noSplash(p);
    await p.getByRole("button", { name: W.landing.joinCta }).first().tap();
    const join = p.getByRole("dialog", { name: W.landing.joinTitle });
    await join.getByPlaceholder("123 456 789").fill(pcode);
    await join.getByPlaceholder("000000").fill(ppin);
    await join.getByRole("checkbox", { name: has("I have read and accept") }).check();
    await join.getByRole("button", { name: W.guest.join, exact: true }).tap();
    const name = p.getByRole("dialog", { name: W.alias.title });
    await expect(name).toBeVisible({ timeout: 60_000 });
    await name.locator("input").first().fill("Sol");
    await p.keyboard.press("Enter");
    await expectVideoPlaying(p);
    await sleep(1500);
    // Taps on the on-screen gamepad.
    for (let i = 0; i < 10; i++) {
      const btn = p.locator(".touchpad button, .tpad button, [class*=pad] button").nth(i % 4);
      await btn.tap().catch(() => undefined);
      await sleep(350);
    }
    await sleep(3000);
  }, { viewport: { width: 430, height: 932 }, mobile: true, linked: false });
  await host.close();
});

test("Willy Maker: a game made in the browser plays in a room", async ({ browser }) => {
  const cores = join(here, "..", ".state", "home", "go-link", "cores");
  test.skip(!existsSync(cores) || !readdirSync(cores).some((f) => f.startsWith("mame2003_plus_libretro")), "no emulator core: set E2E_CORE_DIR");
  test.setTimeout(600_000);
  await take(browser, "maker", async (p, ctx) => {
    await p.goto(`${MAKER_URL}/?editor=classic`);
    await sleep(2000);
    await point(p, p.getByRole("button", { name: has("Next: the board") }));
    await sleep(800);
    await point(p, p.getByRole("radio", { name: has("Buenos Aires template") }));
    await sleep(800);
    await point(p, p.getByRole("button", { name: has("Next: name and players") }));
    await p.getByLabel(tr("Game title")).click();
    await p.keyboard.type("Neon Rescue", { delay: 110 });
    await point(p, p.getByRole("button", { name: has("Next: the first level") }));
    await sleep(800);
    await point(p, p.getByRole("button", { name: tr("Create the game") }));
    await expect(p.getByRole("button", { name: tr("Build") })).toBeVisible();
    await sleep(4000);
    await point(p, p.getByRole("button", { name: tr("Export") }));
    await point(p, p.getByRole("button", { name: tr("Create ROM"), exact: true }));
    await expect(p.getByText(tr("It boots"), { exact: false }).first()).toBeVisible({ timeout: 180_000 });
    await sleep(1500);
    await point(p, p.getByRole("button", { name: tr("Play on my go-link") }));
    const [room] = await Promise.all([ctx.waitForEvent("page"), p.getByRole("button", { name: tr("Connect my go-link") }).click()]);
    await expect(room).toHaveURL(/\/r\/[0-9a-f-]{36}$/, { timeout: 120_000 });
    await expectVideoPlaying(room);
    await room.bringToFront();
    await sleep(3000);
    // Coin, start, then play a little.
    await hold(room, "Digit5", 200);
    await sleep(800);
    await hold(room, "Enter", 200);
    await sleep(2500);
    for (let i = 0; i < 24; i++) {
      await hold(room, ["ArrowRight", "ArrowRight", "KeyZ", "ArrowRight", "KeyX", "ArrowLeft"][i % 6]!, 260 + (i % 3) * 120);
      await sleep(120);
    }
    await sleep(2000);
  });
});

test("the network report", async ({ browser }) => {
  await take(browser, "network", async (p) => {
    await p.goto("/device/history/test/network");
    await noSplash(p);
    await expect(p.getByRole("img", { name: W.net.charts.latency })).toBeVisible({ timeout: 60_000 });
    await sleep(2500);
    // The cursor over a chart lists every value.
    const chart = p.locator(".tchart-svg").first();
    const box = await chart.boundingBox();
    if (box) {
      for (let x = 0.15; x <= 0.85; x += 0.05) {
        await p.mouse.move(box.x + box.width * x, box.y + box.height / 2, { steps: 4 });
        await sleep(90);
      }
    }
    await sleep(800);
    await glide(p, 700, 3500);
    await sleep(2000);
    await glide(p, 900, 3500);
    await sleep(2500);
  });
});
