// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { mkdirSync } from "node:fs";
import { expect, type Page, test } from "@playwright/test";
import { pairingCode } from "../stack";

// go-link HD's engine (its repository golink-hd, its own API) played in the
// test room through the real WebRTC path: the device runs the core's
// built-in demo (or a package with --hd-content), the guest takes P1 with
// Start, runs and jumps, and the picture must follow. Runs only when asked:
//   E2E_HD_CORE=1 E2E_DEVICE_ARGS="--test-room-hd 1080p --hd-core /path/libgolinkhd.dylib" \
//     npx playwright test tests/hd-core.spec.ts --project=web
// E2E_HD_CORE_SHOTS=DIR keeps the screenshots there.
//
// The whole chain (go-link HD's proof of concept): a Willy Maker platformer
// exported as a .glhd package (willy-maker/src/maker/io/glhdExport.test.tsx
// writes one with WM_GLHD_OUT), put in the device's ROM folder, listed in
// New game and played in its own game room:
//   E2E_HD_CORE=1 E2E_HD_GAME=neon_run E2E_ROMS=/path/neon_run.glhd \
//     E2E_DEVICE_ARGS="--hd-core /path/libgolinkhd.dylib" npx playwright test tests/hd-core.spec.ts --project=web

// The engine's built-in games (the platformer and the showcase), with the
// engine shipped inside a packaged app:
//   E2E_HD_CORE=1 E2E_HD_BUILTIN=1 E2E_HD_GAME=glhd_platformer \
//     E2E_DEVICE_COMMAND="HOME=$PWD/.state/home exec /path/go-link-console.app/Contents/MacOS/go-link-console --headless \
//     --config $PWD/.state/device.json --server-signaling ws://127.0.0.1:8191/ws --panel 127.0.0.1:7391 \
//     --web-url http://localhost:5191" npx playwright test tests/hd-core.spec.ts --project=web

test.skip(!process.env.E2E_HD_CORE, "go-link HD's core runs only with E2E_HD_CORE");
test.use({ viewport: { width: 1600, height: 900 } });

/** How many pixels of the video's middle band are the title's yellow. */
async function titleYellow(page: Page) {
  return page.evaluate(() => {
    const v = document.querySelector("video.video") as HTMLVideoElement;
    const c = document.createElement("canvas");
    c.width = 320;
    c.height = 180;
    const g = c.getContext("2d")!;
    g.drawImage(v, 0, 0, c.width, c.height);
    // the title "GO-LINK HD" sits at rows 96-140 of the 360 row screen
    const d = g.getImageData(40, 46, 240, 26).data;
    let n = 0;
    for (let i = 0; i < d.length; i += 4) if (d[i]! > 200 && d[i + 1]! > 160 && d[i + 2]! < 110) n++;
    return n;
  });
}

async function shot(page: Page, name: string) {
  const dir = process.env.E2E_HD_CORE_SHOTS;
  if (!dir) return;
  mkdirSync(dir, { recursive: true });
  await page.locator(".video-stage").screenshot({ path: `${dir}/${name}.png` });
}

async function link(page: Page) {
  await page.goto("/device");
  await page.locator("#d1").evaluate((el, text) => {
    const data = new DataTransfer();
    data.setData("text", text);
    el.dispatchEvent(new ClipboardEvent("paste", { clipboardData: data, bubbles: true, cancelable: true }));
  }, pairingCode());
  await page.getByRole("checkbox", { name: /I have read and accept/ }).check();
  await page.getByRole("button", { name: "Link", exact: true }).click();
  await expect(page.getByText("Linked · live")).toBeVisible();
}

async function videoPlays(page: Page) {
  await expect
    .poll(async () => page.evaluate(() => {
      const v = document.querySelector("video.video") as HTMLVideoElement | null;
      return v ? v.videoWidth > 0 && v.readyState >= 2 && !v.paused : false;
    }), { timeout: 90_000 })
    .toBe(true);
}

test("a Willy Maker game exported for go-link HD is listed, starts a game room and plays", async ({ page }) => {
  const game = process.env.E2E_HD_GAME;
  test.skip(!game, "needs E2E_HD_GAME (and the package in E2E_ROMS)");
  test.setTimeout(240_000);
  await link(page);
  await page.goto(`/create?rom=${game}`);
  // the package's own title and file, playable with go-link HD's core
  await expect(page.getByText(`${game}.glhd`).first()).toBeVisible({ timeout: 30_000 });
  await page.getByRole("button", { name: "Start the game" }).click();
  await expect(page).toHaveURL(/\/r\//, { timeout: 60_000 });
  await videoPlays(page);
  await expect.poll(() => titleYellow(page), { timeout: 30_000 }).toBeGreaterThan(200);
  await shot(page, "game-1-title");
  await page.locator(".video-stage").click({ position: { x: 20, y: 20 } });
  await expect(async () => {
    await page.keyboard.down("Enter");
    await page.waitForTimeout(150);
    await page.keyboard.up("Enter");
    await expect.poll(() => titleYellow(page), { timeout: 2000 }).toBeLessThan(50);
  }).toPass({ timeout: 30_000 });
  await page.keyboard.down("ArrowRight");
  await page.keyboard.down("KeyC");
  for (let i = 0; i < 6; i++) {
    await page.keyboard.down("KeyZ");
    await page.waitForTimeout(350);
    await page.keyboard.up("KeyZ");
    await page.waitForTimeout(350);
  }
  await page.keyboard.up("KeyC");
  await page.keyboard.up("ArrowRight");
  await shot(page, "game-2-playing");
  const v = await page.evaluate(() => {
    const el = document.querySelector("video.video") as HTMLVideoElement;
    return { w: el.videoWidth, h: el.videoHeight, frames: el.getVideoPlaybackQuality().totalVideoFrames };
  });
  console.log(`go-link HD game room: ${v.w}x${v.h}, ${v.frames} frames shown`);
  expect(v.w * 9).toBe(v.h * 16);
});

test("go-link HD's built-in games are listed with the engine and the showcase plays in a game room", async ({ page }) => {
  test.skip(!process.env.E2E_HD_BUILTIN, "needs E2E_HD_BUILTIN (a device with go-link HD's engine, shipped or --hd-core)");
  test.setTimeout(180_000);
  await link(page);
  await page.goto("/create?rom=glhd_showcase");
  await page.getByRole("button", { name: "Start the game" }).click();
  await expect(page).toHaveURL(/\/r\//, { timeout: 60_000 });
  await videoPlays(page);
  // the showcase's first scene, Mode 7, moves by itself: the picture keeps changing
  const frame = () => page.evaluate(() => {
    const v = document.querySelector("video.video") as HTMLVideoElement;
    const c = document.createElement("canvas");
    c.width = 64;
    c.height = 36;
    const g = c.getContext("2d")!;
    g.drawImage(v, 0, 0, 64, 36);
    return Array.from(g.getImageData(0, 0, 64, 36).data).join(",");
  });
  const first = await frame();
  await expect.poll(frame, { timeout: 10_000 }).not.toBe(first);
  await shot(page, "showcase");
  const v = await page.evaluate(() => {
    const el = document.querySelector("video.video") as HTMLVideoElement;
    return { w: el.videoWidth, h: el.videoHeight };
  });
  expect(v.w * 9).toBe(v.h * 16);
});

test("go-link HD's core plays in a room: 1080p, and the guest's buttons move the game", async ({ page }) => {
  test.skip(!/--test-room-hd/.test(process.env.E2E_DEVICE_ARGS ?? ""), "needs --test-room-hd in E2E_DEVICE_ARGS");
  test.setTimeout(180_000);
  await link(page);
  await page.getByRole("link", { name: "Test pattern" }).click();
  await videoPlays(page);
  // the core's 640x360 screen enlarged x3; each guest gets the tier its window shows
  const size = await page.evaluate(() => {
    const v = document.querySelector("video.video") as HTMLVideoElement;
    return { w: v.videoWidth, h: v.videoHeight };
  });
  expect(size.w * 9).toBe(size.h * 16);
  expect(size.h).toBeGreaterThanOrEqual(360);

  // the title screen
  await expect.poll(() => titleYellow(page), { timeout: 20_000 }).toBeGreaterThan(300);
  await shot(page, "1-title");

  // Start joins P1 and the game begins: the title goes away
  await page.locator(".video-stage").click({ position: { x: 20, y: 20 } });
  await expect(async () => {
    await page.keyboard.down("Enter");
    await page.waitForTimeout(150);
    await page.keyboard.up("Enter");
    await expect.poll(() => titleYellow(page), { timeout: 2000 }).toBeLessThan(50);
  }).toPass({ timeout: 30_000 });
  await shot(page, "2-start");

  // run right, jumping now and then: the picture keeps changing as the camera follows
  await page.keyboard.down("ArrowRight");
  await page.keyboard.down("KeyC");
  for (let i = 0; i < 8; i++) {
    await page.keyboard.down("KeyZ");
    await page.waitForTimeout(350);
    await page.keyboard.up("KeyZ");
    await page.waitForTimeout(350);
  }
  await page.keyboard.up("KeyC");
  await page.keyboard.up("ArrowRight");
  await shot(page, "3-running");
  const stats = await page.evaluate(() => {
    const v = document.querySelector("video.video") as HTMLVideoElement;
    const q = v.getVideoPlaybackQuality();
    return { frames: q.totalVideoFrames, dropped: q.droppedVideoFrames, w: v.videoWidth, h: v.videoHeight };
  });
  console.log(`go-link HD in a room: ${stats.w}x${stats.h}, ${stats.frames} frames shown, ${stats.dropped} dropped`);
  expect(stats.frames).toBeGreaterThan(300);
});
