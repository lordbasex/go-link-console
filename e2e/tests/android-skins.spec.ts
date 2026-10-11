// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { expect, test } from "@playwright/test";
import { existsSync, mkdirSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import * as A from "../android";

// The gamepad skins on the go-link Console Player Android app, in its debug lab
// (--es lab skin: a skin around the test card, no room): every built-in
// skin in portrait and landscape, each control sends its own bit and then
// 0, and in landscape the menu over the picture folds into its handle
// while in portrait (under the picture) it stays. Mirrors the iOS app's
// testSkinPad. Screenshots go to test-results/android-skins.
//
// Run with: npm run test:android:skins (needs an emulator or phone in
// `adb devices` and the debug APK: ./gradlew :app:assembleDebug in
// mobile/android). Skipped when no device is attached.

const here = dirname(fileURLToPath(import.meta.url));
const APK = process.env.ANDROID_APK || resolve(here, "../../mobile/android/app/build/outputs/apk/debug/app-debug.apk");
const OUT = resolve(here, "../test-results/android-skins");
const serial = A.attachedDevice();
test.skip(!serial, "no Android device in `adb devices`");

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

/** Portrait (0) or landscape (1), with auto-rotate off. */
function rotate(landscape: boolean) {
  A.shell("settings put system accelerometer_rotation 0");
  A.shell(`settings put system user_rotation ${landscape ? 1 : 0}`);
}

async function logText(): Promise<string> {
  return A.find({ id: "pad-log" })?.text ?? "";
}

test("every skin's controls answer in both orientations", async () => {
  if (!existsSync(APK)) throw new Error(`no APK at ${APK}: run ./gradlew :app:assembleDebug in mobile/android`);
  if (process.env.ANDROID_SERIAL === undefined) process.env.ANDROID_SERIAL = serial!;
  mkdirSync(OUT, { recursive: true });
  A.installApp(APK);
  const skins: [string, number, number][] = [["violet", 6, 2], ["blue", 4, 2], ["green", 2, 4], ["orange", 3, 1], ["red", 6, 4], ["smoke", 1, 2]];
  try {
    for (const [skin, buttons, starts] of skins) {
      rotate(false);
      A.shell(`am force-stop ${A.PACKAGE}`);
      A.shell(`am start -n ${A.PACKAGE}/.MainActivity --es lab skin --es skin ${skin} --ei buttons ${buttons} --ei starts ${starts}`);
      for (const landscape of [false, true]) {
        rotate(landscape);
        const name = landscape ? "landscape" : "portrait";
        await A.waitFor({ id: "pad-layout", text: name }, 15_000);
        await sleep(800);
        A.screenshot(join(OUT, `${skin}-${name}.png`));
        const press = async (id: string, bits: number, at?: (n: A.UiNode) => [number, number]) => {
          A.tap(await A.waitFor({ id: "pad-clear" }));
          await expect.poll(logText, { timeout: 5_000 }).toBe("");
          const n = await A.waitFor({ id }, 5_000);
          A.hold(at ? at(n) : A.center(n), 300);
          await expect.poll(logText, { timeout: 5_000, message: `${id} in ${skin} ${name}` }).toBe(`${bits} 0`);
        };
        await press("pad-button-1", 1 << 4);
        await press(`pad-button-${buttons}`, 1 << (3 + buttons));
        await press("pad-coin", 1 << 11);
        await press(`pad-start-${starts}`, 1 << (17 + starts));
        const [l, t, r, b] = [0, 1, 2, 3];
        await press("pad-dpad", 1 << 3, (n) => [Math.round(n.bounds[l] + (n.bounds[r] - n.bounds[l]) * 0.85), Math.round((n.bounds[t] + n.bounds[b]) / 2)]);
        await press("pad-dpad", 1 << 0, (n) => [Math.round((n.bounds[l] + n.bounds[r]) / 2), Math.round(n.bounds[t] + (n.bounds[b] - n.bounds[t]) * 0.15)]);
      }
      // Landscape: the menu over the picture folds into its handle.
      await A.waitFor({ id: "dock-handle" }, 8_000);
      if (skin === "violet") A.screenshot(join(OUT, "violet-landscape-menu-hidden.png"));
      // Portrait: under the picture, it stays.
      rotate(false);
      await A.waitFor({ id: "pad-layout", text: "portrait" }, 15_000);
      await sleep(4_000);
      expect(A.find({ id: "dock-handle" }), `${skin}: the portrait menu must stay`).toBeUndefined();
      expect(A.find({ id: "lab-settings" }), `${skin}: the portrait menu is shown`).toBeDefined();
    }
  } finally {
    rotate(false);
    A.shell(`am force-stop ${A.PACKAGE}`);
  }
});

// Install skin (Settings): an invalid paste shows what is wrong, a valid one
// is checked, previewed in both orientations with its warnings, installed
// as a custom skin, used, and deleted after asking.
const PASTED_ID = "e2e-paste";
const PASTED = JSON.stringify({
  format: 1,
  id: PASTED_ID,
  name: { en: "E2E-Paste" },
  author: "e2e",
  shell: { center: "#2a6f97", edge: "#012a4a", rim: "#a9d6e5" },
  background: { portrait: "back.png" },
});

/**
 * Types into a field in short pieces: one long adb "input text" drops keys
 * in a Compose field (and UiAutomator can't read the field back to check).
 */
async function typeInto(id: string, text: string) {
  await A.fill({ id }, "");
  for (let i = 0; i < text.length; i += 16) {
    A.typeText(text.slice(i, i + 16));
    await sleep(350);
  }
  await sleep(1_000);
}

/** Scrolls Settings until a node shows up. */
async function scrollTo(id: string): Promise<A.UiNode> {
  for (let i = 0; i < 8; i++) {
    const n = A.find({ id });
    if (n) return n;
    A.shell("input swipe 540 1600 540 700 300");
    await sleep(600);
  }
  return A.waitFor({ id }, 5_000);
}

test("installs a pasted skin, previews it, uses it and deletes it", async () => {
  if (!existsSync(APK)) throw new Error(`no APK at ${APK}: run ./gradlew :app:assembleDebug in mobile/android`);
  if (process.env.ANDROID_SERIAL === undefined) process.env.ANDROID_SERIAL = serial!;
  mkdirSync(OUT, { recursive: true });
  A.installApp(APK);
  rotate(false);
  A.shell(`rm -f /sdcard/Android/data/${A.PACKAGE}/files/Skins/skin-${PASTED_ID}.json`);
  A.shell(`am force-stop ${A.PACKAGE}`);
  A.shell(`am start -n ${A.PACKAGE}/.MainActivity`);
  try {
    await A.tapOn({ id: "home-settings" }, 60_000);
    A.tap(await scrollTo("settings-skin-install"));
    // Broken JSON: the error says so, with its line.
    await typeInto("skin-install-json", '{"format":1,"id":"x" "name":{}}');
    A.hideKeyboard();
    await A.tapOn({ id: "skin-install-check" });
    await A.waitFor({ id: "skin-install-error" }, 10_000);
    await A.waitFor({ text: /JSON.*line 1/ }, 5_000);
    A.screenshot(join(OUT, "install-error.png"));
    // A valid skin: preview, the missing picture warning, both orientations.
    await typeInto("skin-install-json", PASTED);
    A.hideKeyboard();
    await A.tapOn({ id: "skin-install-check" });
    await A.waitFor({ id: "skin-install-preview" }, 10_000);
    await A.waitFor({ id: "skin-install-name", text: "E2E-Paste" });
    await A.waitFor({ id: "skin-install-warning" });
    A.screenshot(join(OUT, "install-preview-portrait.png"));
    await A.tapOn({ id: "skin-install-landscape" });
    await sleep(800);
    A.screenshot(join(OUT, "install-preview-landscape.png"));
    await A.tapOn({ id: "skin-install-confirm" });
    await A.waitFor({ id: "skin-install-done" }, 10_000);
    A.screenshot(join(OUT, "install-done.png"));
    await A.tapOn({ id: "skin-install-use" });
    // Listed as a custom skin in Settings, and deleted after asking.
    A.tap(await scrollTo(`skin-delete-${PASTED_ID}`));
    await A.tapOn({ id: "skin-delete-confirm" }, 10_000);
    await A.waitGone({ id: `settings-skin-${PASTED_ID}` }, 10_000);
    expect(A.shell(`ls /sdcard/Android/data/${A.PACKAGE}/files/Skins/`)).not.toContain(PASTED_ID);
  } finally {
    A.shell(`am force-stop ${A.PACKAGE}`);
  }
});
