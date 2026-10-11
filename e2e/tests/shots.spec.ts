// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { expect, test, type BrowserContext, type Page } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, readdirSync, rmSync, statSync, writeFileSync } from "node:fs";
import { homedir, hostname, userInfo } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import * as A from "../android";
import { PORTS } from "../ports";
import { pairingCode } from "../stack";
import { en } from "../../frontend/apps/web/src/i18n/en";
import { es } from "../../frontend/apps/web/src/i18n/es";
import { pt } from "../../frontend/apps/web/src/i18n/pt";

// The landing page's screenshots (npm run shots). Real pages of the test
// stack in every language: the device's window (Fyne's test driver, through
// the Go test TestShots), the website (the code form, the test pattern
// room, the invitation, My device and its ROMs) and, when an emulator
// or phone is attached, the go-link Player app. The device's library is an
// invented one (backend-device/internal/shots: made-up games and covers);
// only the test pattern room is ever played. Before every picture the
// computer's name, the user name and local paths are replaced on the page,
// and the picture is refused if any of them (or an IPv4 address) is left.
//
// Output: frontend/apps/web/public/shots/<lang>/<name>.webp (cwebp) and
// their sizes in frontend/apps/web/src/components/landing/shots.json.

const here = dirname(fileURLToPath(import.meta.url));
const root = resolve(here, "../..");
const RAW = resolve(here, "../test-results/shots");
const OUT = join(root, "frontend/apps/web/public/shots");
const MANIFEST = join(root, "frontend/apps/web/src/components/landing/shots.json");
const APK = process.env.ANDROID_APK || join(root, "mobile/android/app/build/outputs/apk/debug/app-debug.apk");
const SIGNAL_URL = process.env.ANDROID_SIGNAL_URL || `ws://10.0.2.2:${PORTS.signal}/ws`;

const LANGS = ["en", "es", "pt"] as const;
type Lang = (typeof LANGS)[number];
const M = { en, es, pt } as const;

// What must never show in a picture, and what it becomes.
const home = homedir();
const short = hostname().split(".")[0]!;
const MASK: [string, string][] = [
  [join(here, "..", ".state", "home"), "~"],
  [root, "~/go-link"],
  [home, "~"],
  [hostname(), "arcade-pc"],
  [short, "arcade-pc"],
  [userInfo().username, "player"],
  // The test stack's addresses read as the public ones.
  [`ws://127.0.0.1:${PORTS.signal}/ws`, "wss://signal.go-link.org/ws"],
  [`http://localhost:${PORTS.web}`, "https://console.go-link.org"],
  [`localhost:${PORTS.web}`, "console.go-link.org"],
  [`127.0.0.1:${PORTS.signal}`, "signal.go-link.org"],
  // The site's search hint names real games: never in a picture.
  ["Metal Slug, Turtles…", "Sky Pirates, Glacier…"],
].sort((a, b) => b[0].length - a[0].length) as [string, string][];
const SECRETS = [hostname(), short, userInfo().username, home, "Metal Slug", "Turtles"];

/**
 * A short room chat per language, in order (gallery.sampleChat in the
 * website's i18n files; "app" lines are typed on the phone by adb, plain
 * ASCII). The room keeps its chat history, so each language writes enough
 * lines to push the previous one out of view.
 */
const CHAT: Record<Lang, { who: "owner" | "app"; text: string }[]> = {
  en: en.gallery.sampleChat,
  es: es.gallery.sampleChat,
  pt: pt.gallery.sampleChat,
};

test.describe.configure({ mode: "serial" });

let owner: BrowserContext;
let page: Page;
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

/** Replaces private strings in text, titles, labels and fields, now and on every change. */
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
    (window as unknown as { __mask: () => void }).__mask = () => run(document);
    new MutationObserver(() => run(document)).observe(document, { subtree: true, childList: true, characterData: true, attributes: true, attributeFilter: ["title", "aria-label", "placeholder"] });
  }, MASK);
}

/** Saves a viewport picture after checking that nothing private is on the page. */
async function shot(p: Page, lang: Lang, name: string) {
  await p.evaluate(() => (window as unknown as { __mask: () => void }).__mask());
  const text = await p.evaluate(() => {
    const fields = [...document.querySelectorAll("input")].map((i) => i.value);
    const attrs = [...document.querySelectorAll("[title],[aria-label],[placeholder]")].map((e) => ["title", "aria-label", "placeholder"].map((a) => e.getAttribute(a) ?? "").join(" "));
    return [document.body.innerText, ...fields, ...attrs].join("\n");
  });
  for (const s of SECRETS) expect(text.includes(s), `${lang}/${name} shows "${s}"`).toBe(false);
  const ip = text.match(/.{0,40}\b(?:\d{1,3}\.){3}\d{1,3}\b.{0,40}/);
  expect(ip?.[0] ?? null, `${lang}/${name} shows an IP address`).toBeNull();
  const file = join(RAW, lang, `${name}.png`);
  mkdirSync(dirname(file), { recursive: true });
  await p.screenshot({ path: file, animations: "disabled", caret: "hide" });
}

/** Waits until the start logo (index.html's #splash) is gone. */
async function noSplash(p: Page) {
  await expect(p.locator("#splash")).toHaveCount(0, { timeout: 30_000 });
}

/** Goes to a page of the site without reloading it (no start logo). */
async function nav(p: Page, href: string) {
  await p.locator(`a[href="${href}"]`).first().click();
  await expect(p).toHaveURL(new RegExp(`${href.replace(/\//g, "\\/")}$`));
}

async function switchLang(p: Page, lang: Lang) {
  // The language button in the bar, or (in a room, on a phone) the "…" menu's Language.
  const button = p.locator(".lang-menu-button").first();
  if (await button.isVisible()) {
    await button.click();
    await p.locator(`.lang-menu [lang="${lang}"]`).click();
  } else {
    await p.locator(".header-more").click();
    await p.locator(".header-menu [aria-haspopup=menu]").click();
    await p.locator(`.header-menu [lang="${lang}"]`).click();
    await p.keyboard.press("Escape");
  }
  await expect(p.locator("html")).toHaveAttribute("lang", lang);
}

async function expectVideoPlaying(p: Page) {
  await expect
    .poll(async () => p.evaluate(() => {
      const v = document.querySelector("video.video") as HTMLVideoElement | null;
      return v ? v.videoWidth > 0 && v.readyState >= 2 && !v.paused : false;
    }), { timeout: 30_000 })
    .toBe(true);
}

/** Wakes the video's auto-hidden controls. */
async function wakeStage(p: Page) {
  const box = await p.locator(".video-stage").boundingBox();
  if (box) {
    await p.mouse.move(box.x + box.width / 2 - 40, box.y + box.height / 2, { steps: 3 });
    await p.mouse.move(box.x + box.width / 2, box.y + box.height / 2 + 20, { steps: 3 });
  }
}

/** A new invitation from the owner's room (English UI): the code and a one-person PIN. */
async function invite(p: Page = page, lang: Lang = "en"): Promise<{ invite: string; code: string; pin: string }> {
  await p.locator(`[data-tip="${M[lang].invite.button}"]`).first().click();
  const dialog = p.getByRole("dialog");
  await expect(dialog.locator(".invite-code").nth(1)).toHaveText(/^\d{6}$/);
  const url = await dialog.locator(".invite-url").innerText();
  const code = (await dialog.locator(".invite-code").first().innerText()).replace(/\s/g, "");
  const pin = (await dialog.locator(".invite-code").nth(1).innerText()).trim();
  return { invite: url.split("/g/")[1]!.trim(), code, pin };
}

async function closeDialog(p: Page = page) {
  await p.locator(".invite-dialog button.button-primary").last().click();
  await expect(p.getByRole("dialog")).toBeHidden();
}

test.beforeAll(async ({ browser }) => {
  rmSync(RAW, { recursive: true, force: true });
  mkdirSync(RAW, { recursive: true });
  owner = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  await addMask(owner);
  owner.setDefaultTimeout(30_000);
  page = await owner.newPage();
});

test.afterAll(async () => {
  await owner?.close();
});

test("the device window, from Fyne's test driver", async () => {
  execFileSync("go", ["test", "./internal/gui", "-run", "TestShots", "-count=1"], {
    cwd: join(root, "backend-device"),
    env: { ...process.env, GOLINK_SHOTS: RAW },
    stdio: "inherit",
  });
  for (const lang of LANGS) expect(existsSync(join(RAW, lang, "win-roms.png"))).toBe(true);
});

test("the website before linking: the code form", async ({ browser }) => {
  for (const lang of LANGS) {
    const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
    await context.addInitScript((l) => localStorage.setItem("go-link.lang", l), lang);
    await addMask(context);
    const p = await context.newPage();
    await p.goto("/device");
    await noSplash(p);
    await p.locator("#d1").evaluate((el) => {
      const data = new DataTransfer();
      data.setData("text", "482913067");
      el.dispatchEvent(new ClipboardEvent("paste", { clipboardData: data, bubbles: true, cancelable: true }));
    });
    await p.locator(".pair-card input[type=checkbox]").first().check();
    await p.mouse.move(0, 0);
    await sleep(600);
    await shot(p, lang, "web-link");
    await context.close();
  }
});

test("the owner links the device", async () => {
  await page.goto("/device");
  await page.locator("#d1").evaluate((el, text) => {
    const data = new DataTransfer();
    data.setData("text", text);
    el.dispatchEvent(new ClipboardEvent("paste", { clipboardData: data, bubbles: true, cancelable: true }));
  }, pairingCode());
  await page.getByRole("checkbox", { name: /I have read and accept/ }).check();
  await page.getByRole("button", { name: "Link", exact: true }).click();
  await expect(page.getByText("Linked · live")).toBeVisible();
  await noSplash(page);
});

let roomHref = "";

test("My device, its ROMs, the test pattern room and the invitation", async () => {
  for (const lang of LANGS) {
    await switchLang(page, lang);
    await nav(page, "/device");
    await expect(page.locator(".dash-kpis")).toBeVisible();
    const started = Date.now();
    // The charts show the last minute: they fill while the room plays.
    await page.getByRole("button", { name: "1 min", exact: true }).click().catch(() => undefined);
    roomHref = (await page.locator('main a[href^="/r/"]').first().getAttribute("href")) ?? "";
    expect(roomHref).toMatch(/^\/r\/[0-9a-f-]{36}$/);

    // The test pattern room in a second tab, so My device shows a player.
    const room = await owner.newPage();
    await room.goto(roomHref);
    await noSplash(room);
    await expectVideoPlaying(room);
    await sleep(1500);
    await wakeStage(room);
    await sleep(400);
    await shot(room, lang, "web-room");
    await invite(room, lang);
    await sleep(800);
    await shot(room, lang, "web-invite");
    await closeDialog(room);

    await page.bringToFront();
    await sleep(Math.max(0, 45_000 - (Date.now() - started)));
    await page.mouse.move(0, 0);
    await shot(page, lang, "web-device");
    await room.close();

    await nav(page, "/device/roms");
    await expect.poll(() => page.locator("main img").count(), { timeout: 30_000 }).toBeGreaterThanOrEqual(6);
    await expect
      .poll(() => page.evaluate(() => [...document.querySelectorAll("main img")].every((i) => (i as HTMLImageElement).complete && (i as HTMLImageElement).naturalWidth > 0)), { timeout: 30_000 })
      .toBe(true);
    // Whole cards: the page scrolled to the tabs.
    await page.evaluate(() => window.scrollTo(0, 280));
    await page.mouse.move(0, 0);
    await sleep(400);
    await shot(page, lang, "web-roms");
    await page.evaluate(() => window.scrollTo(0, 0));
  }
  await switchLang(page, "en");
  // The owner stays in the room for the Android pictures.
  await page.goto(roomHref);
  await noSplash(page);
  await expectVideoPlaying(page);
});

test("the go-link Player app on Android", async () => {
  const serial = A.attachedDevice();
  test.skip(!serial || !existsSync(APK), "no Android device in `adb devices`, or no debug APK");
  if (process.env.ANDROID_SERIAL === undefined) process.env.ANDROID_SERIAL = serial;
  // Another build of the app (a release) may be installed: keep a copy,
  // put the debug build in its place, and put it back at the end.
  let previous = "";
  try {
    A.adb("install", "-r", APK);
  } catch {
    const remote = A.shell(`pm path ${A.PACKAGE}`).trim().split("\n")[0]?.replace(/^package:/, "") ?? "";
    previous = join(RAW, "previous-app.apk");
    A.adb("pull", remote, previous);
    A.adb("uninstall", A.PACKAGE);
    A.adb("install", APK);
  }
  A.shell(`am force-stop ${A.PACKAGE}`);
  A.shell(`pm clear ${A.PACKAGE}`);
  A.shell(`pm grant ${A.PACKAGE} android.permission.BLUETOOTH_CONNECT || true`);
  A.shell(`pm grant ${A.PACKAGE} android.permission.RECORD_AUDIO || true`);
  A.shell("input keyevent KEYCODE_WAKEUP");
  A.shell("wm dismiss-keyguard || true");
  // A clean status bar: fixed clock, full battery, no notifications.
  A.shell("settings put global sysui_demo_allowed 1");
  const demo = (cmd: string) => A.shell(`am broadcast -a com.android.systemui.demo -e command ${cmd}`);
  demo("enter");
  demo("clock -e hhmm 1200");
  demo("battery -e level 100 -e plugged false");
  demo("network -e wifi show -e level 4 -e mobile hide");
  demo("notifications -e visible false");
  const appShot = (lang: Lang, name: string) => A.screenshot(join(RAW, lang, `${name}.png`));
  const start = (lang: Lang) => {
    A.shell(`cmd locale set-app-locales ${A.PACKAGE} --locales ${lang}`);
    A.shell(`am force-stop ${A.PACKAGE}`);
  };
  try {
    // The home screen, before any custom server is set (it would show its address).
    for (const lang of LANGS) {
      start(lang);
      A.shell(`am start -n ${A.PACKAGE}/.MainActivity`);
      await A.waitFor({ id: "home-code" }, 60_000);
      await sleep(1200);
      appShot(lang, "app-home");
    }
    // Point the debug build at the test stack (English, like the Android e2e).
    start("en");
    A.shell(`am start -n ${A.PACKAGE}/.MainActivity`);
    await A.tapOn({ id: "home-settings" }, 60_000);
    await A.fill({ id: "settings-server-url" }, SIGNAL_URL);
    A.hideKeyboard();
    await A.tapOn({ id: "settings-server-save" });
    await A.waitFor({ text: "Saved: new rooms use this server." }, 20_000);

    const chat = page.locator("#chatmsg");
    for (const lang of LANGS) {
      start(lang);
      const inv = await invite();
      await closeDialog();
      A.shell(`am start -a android.intent.action.VIEW -d https://go-link.org/g/${inv.invite} ${A.PACKAGE}`);
      await A.fill({ id: "join-pin" }, inv.pin);
      A.hideKeyboard();
      const terms = await A.waitFor({ id: "terms-check" });
      if (!terms.checked) A.tap(terms);
      await sleep(600);
      appShot(lang, "app-pin");
      await A.tapOn({ id: "join-button" });
      await A.waitFor({ id: "dock-mic" }, 60_000);
      await sleep(3000);
      await A.tapOn({ id: "dock-mic" });
      await sleep(2500);
      appShot(lang, "app-room");

      if (!(await chat.isVisible())) await page.getByRole("button", { name: "Show the chat" }).click();
      await A.tapOn({ id: "dock-chat" });
      for (const { who, text } of CHAT[lang]) {
        if (who === "owner") {
          await chat.fill(text);
          await chat.press("Enter");
          await A.waitFor({ text });
        } else {
          await A.fill({ id: "chat-input" }, text);
          await A.tapOn({ id: "chat-send" });
          await expect(page.getByText(text).last()).toBeVisible();
        }
      }
      A.hideKeyboard();
      await sleep(800);
      appShot(lang, "app-chat");
      A.shell(`am force-stop ${A.PACKAGE}`);
      await sleep(3000);
    }
  } finally {
    demo("exit");
    A.shell(`cmd locale set-app-locales ${A.PACKAGE} --locales en || true`);
    if (previous) {
      A.adb("uninstall", A.PACKAGE);
      A.adb("install", previous);
      rmSync(previous, { force: true });
    }
  }
});

/** The pictures' final width: desktop pages and the window 1440 px, phones 600 px. */
function widthFor(name: string): number {
  return name.startsWith("app-") ? 600 : 1440;
}

test("converts the pictures to WebP for the website", async () => {
  const manifest: Record<string, [number, number]> = {};
  let total = 0;
  for (const lang of LANGS) {
    const dir = join(RAW, lang);
    const out = join(OUT, lang);
    mkdirSync(out, { recursive: true });
    for (const file of readdirSync(dir).filter((f) => f.endsWith(".png"))) {
      const name = file.replace(/\.png$/, "");
      const target = join(out, `${name}.webp`);
      // Lower the quality until the picture fits in 150 KB.
      for (const q of [80, 72, 64, 56, 48]) {
        execFileSync("cwebp", ["-quiet", "-q", String(q), "-m", "6", "-resize", String(widthFor(name)), "0", join(dir, file), "-o", target]);
        if (statSync(target).size <= 150_000) break;
      }
      total += statSync(target).size;
      const info = execFileSync("webpinfo", [target], { encoding: "utf8" });
      const w = Number(info.match(/Width: (\d+)/)?.[1]);
      const h = Number(info.match(/Height: (\d+)/)?.[1]);
      manifest[name] = [w, h];
    }
  }
  const lines = Object.entries(manifest)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([name, size]) => `  ${JSON.stringify(name)}: ${JSON.stringify(size)}`);
  writeFileSync(MANIFEST, `{\n${lines.join(",\n")}\n}\n`);
  console.log(`shots: ${lines.length} pictures per language, ${(total / 1_000_000).toFixed(2)} MB in all`);
});
