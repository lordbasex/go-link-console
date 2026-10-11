// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { expect, test, type Browser, type BrowserContext, type Page } from "@playwright/test";
import { expectAccessible } from "../a11y";
import { PORTS } from "../ports";
import { pairingCode, panelToken } from "../stack";

// One story, in order: someone opens the site, links their device, comes
// back after a reload, opens the test pattern room, invites a friend (who
// gets in with the code and the PIN, while a second person with the same
// PIN is refused), and finally manages the device from its local panel.

test.describe.configure({ mode: "serial" });

let owner: BrowserContext;
let page: Page;

test.beforeAll(async ({ browser }) => {
  owner = await browser.newContext();
  page = await owner.newPage();
});

test.afterAll(async () => {
  await owner.close();
});

/** Waits until the page's video is playing real frames. */
async function expectVideoPlaying(p: Page) {
  await expect
    .poll(async () => p.evaluate(() => {
      const v = document.querySelector("video.video") as HTMLVideoElement | null;
      return v ? v.videoWidth > 0 && v.readyState >= 2 && !v.paused : false;
    }), { timeout: 30_000 })
    .toBe(true);
}

test("the landing page opens with the quick way into a game", async () => {
  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1, name: /Your arcade, online/ })).toBeVisible();
  const menu = page.getByRole("navigation", { name: "Main" });
  await expect(menu.getByRole("link")).toHaveText(["How it works", "Rooms", "My device", "Docs", "Tools"]);
  await page.getByRole("button", { name: "Join a game" }).first().click();
  const dialog = page.getByRole("dialog", { name: "Join a game" });
  await expect(dialog.getByPlaceholder("123 456 789")).toBeVisible();
  await dialog.getByRole("button", { name: "Got it" }).click();
  await expect(dialog).toBeHidden();
});

test("links the device with its pairing code and shows it live", async () => {
  await page.goto("/device");
  const code = pairingCode();
  expect(code).toMatch(/^\d{9}$/);
  // Pasting the whole code in the first box fills them all.
  await page.locator("#d1").evaluate((el, text) => {
    const data = new DataTransfer();
    data.setData("text", text);
    el.dispatchEvent(new ClipboardEvent("paste", { clipboardData: data, bubbles: true, cancelable: true }));
  }, code);
  // The host accepts the terms of use before linking.
  await page.getByRole("checkbox", { name: /I have read and accept/ }).check();
  await page.getByRole("button", { name: "Link", exact: true }).click();
  await expect(page.getByText("Linked · live")).toBeVisible();
  // Hardware and usage arrive over the WebRTC data channel.
  await expect(page.getByRole("region", { name: "Live figures" })).toBeVisible();
  await expectAccessible(page, "linked device dashboard");
});

test("comes back after a reload: the device proves itself, then the browser shows its token", async () => {
  await page.reload();
  await expect(page.getByText("Linked · live")).toBeVisible();
});

test("the host picks the video quality of game rooms, and the device keeps it", async () => {
  const card = page.locator(".card", { has: page.getByRole("heading", { name: "Video quality" }) });
  const quality = card.getByRole("combobox", { name: "Quality" });
  await expect(quality).toHaveText(/High/);
  await quality.click();
  await page.getByRole("option", { name: /^Saver/ }).click();
  // The select shows what the device reports back in device_status.
  await expect(quality).toHaveText(/Saver/);
  await page.reload();
  await expect(page.getByText("Linked · live")).toBeVisible();
  await expect(quality).toHaveText(/Saver/);
  await quality.click();
  await page.getByRole("option", { name: /^High/ }).click();
  await expect(quality).toHaveText(/High/);
});

test("the test pattern room streams video", async () => {
  // four accessibility checks and the picture styles: a slow runner needs more than the default minute
  test.setTimeout(120_000);
  await page.getByRole("link", { name: "Test pattern" }).click();
  await expect(page).toHaveURL(/\/r\/[0-9a-f-]{36}$/);
  await expectVideoPlaying(page);
  // The host plays too, so it is asked about the voice; it keeps listening.
  const ask = page.getByRole("dialog", { name: "Do you want to join the voice?" });
  await ask.getByRole("button", { name: "Not now" }).click();
  await expect(ask).toHaveCount(0);
  await expectAccessible(page, "owner's room");
  // The same room in the light theme (the video keeps its dark colors).
  await switchTheme(page, "Light mode");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  await expectAccessible(page, "owner's room (light)");
  await expectOutputList(page, "light");
  await switchTheme(page, "Dark mode");
  await expectOutputList(page, "dark");
  await expectPictureStyles(page);
  await expectControlsBesideTheVideo(page);
  // The stream figures: the test card is sent at its own size (no 2x).
  await clickControl(page, "Connection details");
  const figures = page.getByRole("dialog", { name: "Connection details" });
  await expect(figures).toContainText("640×480");
  await expect(figures).not.toContainText("2×");
  await page.keyboard.press("Escape");
});

/** A room's Picture settings open from the header's "…" (Settings). */
async function openPicture(p: Page) {
  await p.getByRole("button", { name: "Settings" }).click();
  await p.getByRole("menu", { name: "Settings" }).getByRole("menuitem", { name: "Picture" }).click();
  await expect(p.getByRole("dialog", { name: "Picture" })).toBeVisible();
}

/** In a room the theme lives in the header's "…" (Settings). */
async function switchTheme(p: Page, name: "Light mode" | "Dark mode") {
  await p.getByRole("button", { name: "Settings" }).click();
  const menu = p.getByRole("menu", { name: "Settings" });
  await expect(menu.getByRole("menuitem", { name: /Device linked/ })).toBeVisible();
  await menu.getByRole("menuitem", { name, exact: true }).click();
  await p.keyboard.press("Escape");
  await expect(menu).toBeHidden();
}

/** The side panel is one canvas with tabs: while a game streams it opens on
 * Controls (the keyboard and gamepads), so someone new sees the keys at
 * once; the video keeps its height, the Chat tab brings the chat back, and
 * the header's chat button closes and reopens the panel. */
async function expectControlsBesideTheVideo(p: Page) {
  const stage = p.locator(".video-stage");
  const before = await stage.boundingBox();
  const canvas = p.getByRole("tablist", { name: "Room panel" });
  await expect(canvas.getByRole("tab", { name: "Controls" })).toHaveAttribute("aria-selected", "true");
  await wake(p);
  await expect(p.getByRole("button", { name: "Hide controls" })).toBeVisible();
  const panel = p.locator("#canvas-controls");
  await expect(panel.getByText("Keyboard").first()).toBeVisible();
  // The host may seat several controllers; by default they are one player.
  await expect(panel.getByRole("switch", { name: "Several controllers" })).toHaveAttribute("aria-checked", "false");
  // Latency: everyone sees the network and the video; the host's switch
  // draws the controllers on the picture and each press is measured on it.
  await expect(panel.getByText("Network")).toBeVisible();
  const hud = panel.getByRole("switch", { name: "Controls on the video" });
  await hud.click();
  await expect(hud).toHaveAttribute("aria-checked", "true");
  await expect(panel.getByText(/Press and release any button/)).toBeVisible();
  await expect(async () => {
    await p.keyboard.down("KeyX");
    await p.waitForTimeout(400);
    await p.keyboard.up("KeyX");
    await p.waitForTimeout(400);
    await expect(panel.getByText(/^Last \d+ ms · \d+ press/)).toBeVisible({ timeout: 100 });
  }).toPass({ timeout: 20_000 });
  await hud.click();
  await expect(hud).toHaveAttribute("aria-checked", "false");
  // The keyboard fits the panel: nothing to scroll sideways.
  expect(await panel.locator(".controls-panel").evaluate((el) => el.scrollWidth <= el.clientWidth + 1)).toBe(true);
  await expect(p.getByRole("textbox", { name: "Message" })).toBeHidden();
  const after = await stage.boundingBox();
  expect(Math.abs((after?.height ?? 0) - (before?.height ?? 0))).toBeLessThan(2);
  await expectAccessible(p, "room controls in the side panel");
  // Chat holds everything else: your place, the chat, the queue and the spectators.
  await canvas.getByRole("tab", { name: "Chat" }).click();
  await expect(p.getByRole("textbox", { name: "Message" })).toBeVisible();
  await expect(p.getByRole("tablist", { name: "Room", exact: true }).getByRole("tab", { name: "Queue" })).toBeVisible();
  const width = (await p.locator(".room-canvas").boundingBox())?.width;
  await canvas.getByRole("tab", { name: "Controls" }).click();
  expect((await p.locator(".room-canvas").boundingBox())?.width).toBe(width);
  await canvas.getByRole("tab", { name: "Chat" }).click();
  await p.getByRole("button", { name: "Close the panel" }).click();
  await expect(canvas).toBeHidden();
  await p.getByRole("button", { name: "Show the chat" }).click();
  await expect(canvas.getByRole("tab", { name: "Chat" })).toHaveAttribute("aria-selected", "true");
}

/** The dock's Picture settings switch the GPU renderer on and off live, without a reload. */
async function expectPictureStyles(p: Page) {
  const stage = p.locator(".video-stage");
  // The site's default: smooth, with the game's colors on the sides (drawn by the GPU).
  await expect(stage).toHaveAttribute("data-picture-style", "smooth");
  await expect(stage).toHaveAttribute("data-picture-bands", "ambient");
  await expect(stage).toHaveAttribute("data-picture", /^webgl2?$/);
  await openPicture(p);
  const settings = p.getByRole("dialog", { name: "Picture" });
  await settings.getByRole("combobox", { name: "Style" }).click();
  await p.getByRole("option", { name: /^Sharp/ }).click();
  await expect(stage).toHaveAttribute("data-picture", /^webgl2?$/);
  await expect(stage.locator("canvas.picture-canvas")).toBeVisible();
  await settings.getByRole("combobox", { name: "Sides" }).click();
  await p.getByRole("option", { name: /^Ambient/ }).click();
  await settings.getByRole("button", { name: "Compare" }).click();
  await expect(p.getByRole("slider", { name: "Comparison divider" })).toBeVisible();
  await expectAccessible(p, "room picture settings");
  // The game keeps playing under the canvas (sound, screenshots, recordings).
  await expectVideoPlaying(p);
  await settings.getByRole("button", { name: "Compare" }).click();
  await settings.getByRole("combobox", { name: "Style" }).click();
  await p.getByRole("option", { name: /^Smooth(?! edges)/ }).click();
  await settings.getByRole("combobox", { name: "Sides" }).click();
  await p.getByRole("option", { name: /^Black/ }).click();
  await expect(stage).toHaveAttribute("data-picture", "off");
  await p.keyboard.press("Escape");
  await expect(settings).toBeHidden();
}

/** The room's Output list (the site's Select) opens inside the voice settings, passes the checker and closes alone. */
async function expectOutputList(p: Page, theme: string) {
  await clickControl(p, "Volume and voice");
  const settings = p.getByRole("dialog", { name: "Game voice" });
  const output = settings.getByRole("combobox", { name: "Output" });
  await output.click();
  await expect(p.getByRole("listbox", { name: "Output" })).toBeVisible();
  await expect(p.getByRole("option", { name: "System default" })).toHaveAttribute("aria-selected", "true");
  await expectAccessible(p, `room output list (${theme})`);
  await p.keyboard.press("Escape");
  await expect(p.getByRole("listbox")).toBeHidden();
  await expect(settings).toBeVisible();
  await expect(output).toBeFocused();
  await p.keyboard.press("Escape");
  await expect(settings).toBeHidden();
}

test("an invitation lets one person in; the same PIN refuses the next one", async ({ browser }) => {
  await page.getByRole("button", { name: "Invite", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: /Invite to/ });
  const code = (await dialog.locator(".invite-code").first().innerText()).replace(/\s/g, "");
  await expect(dialog.locator(".invite-code").nth(1)).toHaveText(/^\d{6}$/);
  const pin = (await dialog.locator(".invite-code").nth(1).innerText()).trim();
  expect(code).toMatch(/^\d{9}$/);
  await dialog.getByRole("button", { name: "Got it" }).click();

  const guest = await joinAsGuest(browser, code, pin);
  // After the PIN: "What's your name?", checked as it is typed.
  const step = guest.page.getByRole("dialog", { name: "What\u2019s your name?" });
  await expect(step).toBeVisible();
  await expectAccessible(guest.page, "name step");
  await enterName(guest.page, "Zoë");
  await expectVideoPlaying(guest.page);
  await expectAccessible(guest.page, "guest's room");

  const late = await joinAsGuest(browser, code, pin);
  await expect(late.page.getByText("Someone already came in with this invitation.", { exact: false })).toBeVisible();
  await guest.context.close();
  await late.context.close();
});

test("a group invitation lets several people in with one QR code, each one let in by the host", async ({ browser }) => {
  // E2E_GROUP_SHOTS=dir keeps screenshots of each step.
  const shot = async (p: Page, name: string) => {
    if (process.env.E2E_GROUP_SHOTS) await p.screenshot({ path: `${process.env.E2E_GROUP_SHOTS}/${name}.png` });
  };
  await page.getByRole("button", { name: "Invite", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: /Invite to/ });
  await dialog.getByRole("button", { name: "Group" }).click();
  await expect(dialog.getByRole("checkbox", { name: /Ask me first/ })).toBeChecked();
  await dialog.getByRole("combobox", { name: "Up to" }).selectOption("2");
  await dialog.getByRole("button", { name: "Make the group QR code" }).click();
  await expect(dialog.getByText("0 of 2 people came in")).toBeVisible();
  await shot(page, "1-host-group-qr");
  const link = (await dialog.locator(".invite-url[data-link]").getAttribute("data-link")) ?? "";
  expect(link).toMatch(/\/g\/[A-Za-z0-9_-]{22}#k=[A-Za-z0-9_-]{43}$/);
  await dialog.getByRole("button", { name: "Got it" }).click();

  // A friend opens the one link: a name for the host, then a wait.
  const knock = async (name: string) => {
    const context = await browser.newContext();
    const p = await context.newPage();
    await p.goto(link);
    // The startup intro covers the page for its first seconds.
    await expect(p.locator("#splash")).toBeHidden();
    const form = p.locator(".knock-prompt");
    await form.getByRole("textbox", { name: "Your name" }).fill(name);
    await shot(p, `2-guest-${name}-form`);
    await form.getByRole("checkbox", { name: /I have read and accept/ }).check();
    await form.getByRole("button", { name: "Ask to come in" }).click();
    await expect(p.getByText("Waiting for the host")).toBeVisible();
    // The key left the address bar once the page read it.
    expect(p.url()).not.toContain("#k=");
    await shot(p, `3-guest-${name}-waiting`);
    return { context, page: p };
  };
  const ana = await knock("Ana");
  // The host sees who it is, wherever it is, and lets Ana in: no PIN, no
  // name step (she gave it).
  await expect(page.getByRole("button", { name: "Let Ana in", exact: true })).toBeVisible();
  await shot(page, "4-host-notice");
  await page.getByRole("button", { name: "Let Ana in", exact: true }).click();
  await expectVideoPlaying(ana.page);
  await expect(ana.page.getByRole("dialog", { name: "What\u2019s your name?" })).toHaveCount(0);
  await shot(ana.page, "5-guest-in");

  // Bea is not let in this time.
  const bea = await knock("Bea");
  await page.getByRole("button", { name: "Do not let Bea in" }).click();
  await expect(bea.page.getByText("The host did not let you in this time.")).toBeVisible();
  await shot(bea.page, "6-guest-declined");

  // Stopping the QR code: Ana stays, the link no longer works.
  await page.getByRole("button", { name: "Invite", exact: true }).click();
  await dialog.getByRole("button", { name: "Group" }).click();
  await expect(dialog.getByText("1 of 2 people came in")).toBeVisible();
  await dialog.getByRole("button", { name: "Stop this QR code" }).click();
  await expect(dialog.getByRole("button", { name: "Make the group QR code" })).toBeVisible();
  // Back to one person for the next tests.
  await dialog.getByRole("button", { name: "One person" }).click();
  await dialog.getByRole("button", { name: "Got it" }).click();
  const late = await browser.newContext();
  const lp = await late.newPage();
  await lp.goto(link);
  await expect(lp.locator("#splash")).toBeHidden();
  const lateForm = lp.locator(".knock-prompt");
  await lateForm.getByRole("textbox", { name: "Your name" }).fill("Cid");
  await lateForm.getByRole("checkbox", { name: /I have read and accept/ }).check();
  await lateForm.getByRole("button", { name: "Ask to come in" }).click();
  await expect(lp.getByText("This invitation does not work anymore.", { exact: false })).toBeVisible();
  await expectVideoPlaying(ana.page);
  for (const c of [ana.context, bea.context, late]) await c.close();
});

test("the host's network report shows the room's charts, freezes and log", async () => {
  // A second tab of the host: the room's tab stays for the next tests.
  const report = await owner.newPage();
  await report.goto(new URL("/device/history/test/network", page.url()).toString());
  await expect(report.getByRole("heading", { name: /^Network/ })).toBeVisible({ timeout: 30_000 });
  // The test room ran with the host (and Zoë) in it: samples and events are there.
  await expect(report.getByRole("img", { name: "Latency" })).toBeVisible({ timeout: 30_000 });
  await expect(report.getByRole("img", { name: "Frames per second" })).toBeVisible();
  const log = report.getByRole("region", { name: "Event log" });
  await expect(log.getByRole("cell", { name: "run_start" })).toBeVisible();
  await expect(log.getByRole("cell", { name: "peer_path" }).first()).toBeVisible();
  await expect(report.getByRole("heading", { name: /^Freezes/ })).toBeVisible();
  await expectAccessible(report, "network report");
  await report.close();
});

test("the host's picture default reaches guests who never chose one; a guest's own choice wins", async ({ browser }) => {
  test.setTimeout(150_000);
  const stage = page.locator(".video-stage");
  // The owner picks CRT arcade and makes it the room's default.
  await openPicture(page);
  const settings = page.getByRole("dialog", { name: "Picture" });
  await settings.getByRole("combobox", { name: "Style" }).click();
  await page.getByRole("option", { name: /^CRT arcade/ }).click();
  await expect(stage).toHaveAttribute("data-picture-style", "crt");
  await settings.getByRole("button", { name: "Set as the room's default" }).click();
  // The device sends it back to everyone in room_state.
  await expect(settings.getByText("Room default: CRT arcade · Black")).toBeVisible();
  await expect(settings.getByText("This is the room's default.")).toBeVisible();
  await expectAccessible(page, "owner's picture settings");
  await page.keyboard.press("Escape");

  // A guest who never chose a picture gets the room's default.
  let inv = await newInvitation();
  const fresh = await joinAsGuest(browser, inv.code, inv.pin);
  await enterName(fresh.page, "Ana");
  await expectVideoPlaying(fresh.page);
  const freshStage = fresh.page.locator(".video-stage");
  await expect(freshStage).toHaveAttribute("data-picture-style", "crt");
  await expect(freshStage).toHaveAttribute("data-picture-bands", "black");
  await expect(freshStage).toHaveAttribute("data-picture", /^webgl2?$/);
  await openPicture(fresh.page);
  const guestSettings = fresh.page.getByRole("dialog", { name: "Picture" });
  await expect(guestSettings.getByText("Room default: CRT arcade · Black")).toBeVisible();
  // Guests never change the room.
  await expect(guestSettings.getByRole("button", { name: "Set as the room's default" })).toHaveCount(0);
  await fresh.context.close();

  // A guest who chose Sharp before keeps Sharp, and may go back to the room's default.
  inv = await newInvitation();
  const picky = await joinAsGuest(browser, inv.code, inv.pin, (context) =>
    context.addInitScript(() => {
      if (!localStorage.getItem("go-link.picture-style")) localStorage.setItem("go-link.picture-style", "sharp");
    }),
  );
  await enterName(picky.page, "Bea");
  await expectVideoPlaying(picky.page);
  const pickyStage = picky.page.locator(".video-stage");
  await expect(pickyStage).toHaveAttribute("data-picture-style", "sharp");
  await openPicture(picky.page);
  const pickySettings = picky.page.getByRole("dialog", { name: "Picture" });
  await expect(pickySettings.getByText("Room default: CRT arcade · Black")).toBeVisible();
  await expect(pickySettings.getByRole("button", { name: "Use the room's default" })).toBeVisible();
  await picky.context.close();

  // My device shows the test pattern room's default, and puts back the site's.
  // In a room the header has no sections: the device is in the "…" menu.
  await page.getByRole("button", { name: "Settings" }).click();
  await page.getByRole("menu", { name: "Settings" }).getByRole("menuitem", { name: /Device linked/ }).click();
  await page.getByRole("button", { name: "Picture default" }).click();
  const dialog = page.getByRole("dialog", { name: "Picture default for \u201cTest pattern\u201d" });
  await expect(dialog.getByText("Now: CRT arcade · Black")).toBeVisible();
  await expectAccessible(page, "picture default dialog");
  await dialog.getByRole("button", { name: "Use the default" }).click();
  await expect(dialog).toBeHidden();
  await page.getByRole("button", { name: "Picture default" }).click();
  await expect(dialog.getByText("Now: the site's default (Smooth · Ambient)")).toBeVisible();
  await dialog.getByRole("button", { name: "Cancel" }).click();

  // Back to the room, with the owner's own picture back to Smooth.
  await page.getByRole("link", { name: "Test pattern" }).click();
  await expectVideoPlaying(page);
  await openPicture(page);
  await expect(settings.getByText("Room default:")).toHaveCount(0);
  await settings.getByRole("combobox", { name: "Style" }).click();
  await page.getByRole("option", { name: /^Smooth(?! edges)/ }).click();
  await expect(stage).toHaveAttribute("data-picture", "off");
  await page.keyboard.press("Escape");
});

/** Answers "What's your name?": a symbol is refused as typed, then the name goes in. */
async function enterName(p: Page, name: string) {
  const step = p.getByRole("dialog", { name: "What\u2019s your name?" });
  const field = step.getByRole("textbox", { name: "Your name" });
  await field.fill(`${name}!`);
  await expect(field).toHaveAttribute("aria-invalid", "true");
  await expect(step.getByRole("button", { name: "Enter the room" })).toBeDisabled();
  await field.fill(name);
  await step.getByRole("button", { name: "Enter the room" }).click();
  await expect(step).toBeHidden();
}

/** Wakes a room page's auto-hidden controls (mouse over the video). */
async function wake(p: Page) {
  const stage = p.locator(".video-stage");
  const box = await stage.boundingBox();
  if (box) await p.mouse.move(box.x + box.width / 2 + Math.random() * 10, box.y + box.height / 3, { steps: 4 });
  await expect(stage).not.toHaveClass(/is-idle/);
}

/**
 * Clicks one of the video's controls, once. They hide again after 3 s
 * without the mouse moving, which a slow runner can spend between waking
 * them and a pointer click (the video then takes the click, and retrying
 * could land a second click that closes what the first one opened). So the
 * controls are woken and seen, then the button gets a single click event.
 */
async function clickControl(p: Page, name: string) {
  const button = p.getByRole("button", { name });
  await wake(p);
  await expect(button).toBeVisible();
  await button.dispatchEvent("click");
}

/** A new invitation from the owner's room: the 9 digit code and its PIN. */
async function newInvitation() {
  await page.getByRole("button", { name: "Invite", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: /Invite to/ });
  await expect(dialog.locator(".invite-code").nth(1)).toHaveText(/^\d{6}$/);
  const code = (await dialog.locator(".invite-code").first().innerText()).replace(/\s/g, "");
  const pin = (await dialog.locator(".invite-code").nth(1).innerText()).trim();
  await dialog.getByRole("button", { name: "Got it" }).click();
  return { code, pin };
}

test("only the host pauses: a player asks, the host accepts or declines, a guest's pause is refused", async ({ browser }) => {
  // Several rounds with two browsers: slower CI machines need more than the default minute.
  test.setTimeout(180_000);
  const { code, pin } = await newInvitation();
  // This guest's browser is tampered with: every hello carries a name with
  // symbols and emoji, and the page keeps the control channel to send what
  // the website would never send. The device must clean and refuse.
  const context = await browser.newContext();
  // This guest uses the light theme.
  await context.addInitScript(() => localStorage.setItem("go-link.theme", "light"));
  await context.addInitScript(() => {
    const send = RTCDataChannel.prototype.send;
    RTCDataChannel.prototype.send = function (this: RTCDataChannel, data: string | Blob | ArrayBuffer | ArrayBufferView) {
      if (this.label === "control") {
        (window as unknown as { __control: RTCDataChannel }).__control = this;
        if (typeof data === "string" && data.includes('"hello"')) {
          const m = JSON.parse(data) as { name?: string };
          m.name = "<b>Zoë</b> 😀!!";
          data = JSON.stringify(m);
        }
      }
      return send.call(this, data as string);
    } as typeof RTCDataChannel.prototype.send;
  });
  const g = await context.newPage();
  await g.goto("/");
  await g.getByRole("button", { name: "Join a game" }).first().click();
  const join = g.getByRole("dialog", { name: "Join a game" });
  await join.getByPlaceholder("123 456 789").fill(code);
  await join.getByPlaceholder("000000").fill(pin);
  await join.getByRole("checkbox", { name: /I have read and accept/ }).check();
  await join.getByRole("button", { name: "Join", exact: true }).click();
  await enterName(g, "Zoe");
  await expectVideoPlaying(g);
  await expectAccessible(g, "guest's room (light)");

  // The device cleaned the name: everyone sees "bZoëb".
  await expect(page.locator(".players-capsule [aria-label*='bZoëb']").first()).toBeAttached();

  // A guest's own pause is refused by the device, whatever its page sends.
  await g.evaluate(() => (window as unknown as { __control: RTCDataChannel }).__control.send(JSON.stringify({ type: "pause", paused: true })));
  await expect(g.getByText("Only the host pauses the game: ask for a pause instead.")).toBeVisible();
  await expect(g.locator(".video-paused")).toHaveCount(0);

  // Ask; the host accepts: paused for everyone, in the requester's name.
  await clickControl(g, "Ask the host for a pause");
  await expect(g.getByText("You asked the host for a pause…")).toBeVisible();
  const ask = page.getByRole("alertdialog", { name: "bZoëb wants to pause" });
  await expect(ask).toBeVisible();
  await expectAccessible(page, "host's pause request");
  await ask.getByRole("button", { name: "Pause", exact: true }).click();
  await expect(g.locator(".video-paused-label")).toHaveText("Paused by bZoëb");
  await expect(page.locator(".video-paused-label")).toHaveText("Paused by bZoëb");
  // The side panel opens on Controls: the line is in the Chat tab.
  await g.getByRole("tablist", { name: "Room panel" }).getByRole("tab", { name: "Chat" }).click();
  await expect(g.getByText("bZoëb asked for a pause and", { exact: false }).first()).toBeVisible();
  // Only the host resumes.
  await expect(g.locator(".video-paused").getByRole("button", { name: "Resume" })).toHaveCount(0);
  await page.locator(".video-paused").getByRole("button", { name: "Resume" }).click();
  await expect(g.locator(".video-paused")).toHaveCount(0);

  // Ask again; the host would rather keep playing.
  await clickControl(g, "Ask the host for a pause");
  await page.getByRole("alertdialog", { name: "bZoëb wants to pause" }).getByRole("button", { name: "Keep playing" }).click();
  await expect(g.locator(".pause-note")).toHaveText("The host would rather keep playing");
  await expect(g.locator(".video-paused")).toHaveCount(0);

  // Asking and withdrawing leaves nothing for the host.
  await clickControl(g, "Ask the host for a pause");
  await expect(page.getByRole("alertdialog", { name: "bZoëb wants to pause" })).toBeVisible();
  await g.getByRole("button", { name: "Cancel the request for a pause" }).first().click();
  await expect(page.getByRole("alertdialog", { name: "bZoëb wants to pause" })).toHaveCount(0);

  // The host away from the room, on another page of the linked website,
  // still gets the request, naming the room.
  const roomUrl = page.url();
  await page.getByRole("link", { name: "Leave room" }).click(); // the room's header has no sections
  await clickControl(g, "Ask the host for a pause");
  const notice = page.getByRole("alertdialog", { name: "bZoëb wants to pause \u201cTest pattern\u201d" });
  await expect(notice).toBeVisible();
  await expectAccessible(page, "pause request notice");
  await notice.getByRole("button", { name: "Pause", exact: true }).click();
  await expect(g.locator(".video-paused-label")).toHaveText("Paused by bZoëb");
  await page.goto(roomUrl);
  await page.locator(".video-paused").getByRole("button", { name: "Resume" }).click();
  await expect(g.locator(".video-paused")).toHaveCount(0);
  await context.close();
});

test("a spectator raises a hand: the host says not now, then seats them at once; the host takes someone out for good", async ({ browser }) => {
  test.setTimeout(180_000);
  // E2E_HAND_SHOTS=dir keeps screenshots of each step.
  const shot = async (p: Page, name: string) => {
    if (process.env.E2E_HAND_SHOTS) await p.screenshot({ path: `${process.env.E2E_HAND_SHOTS}/${name}.png` });
  };
  const inv = await newInvitation();
  const sol = await joinAsGuest(browser, inv.code, inv.pin);
  await enterName(sol.page, "Sol");
  await expectVideoPlaying(sol.page);
  // The host moves Sol to watching: Sol can no longer just join the queue.
  await wake(page);
  await page.getByRole("button", { name: /^P2 · Sol/ }).click();
  await page.getByRole("menuitem", { name: "Make Sol a spectator" }).click();
  // "Your place" is in the side panel's Chat tab (it opens on Controls while a game streams).
  await sol.page.getByRole("tablist", { name: "Room panel" }).getByRole("tab", { name: "Chat" }).click();
  const place = sol.page.locator(".side-status");
  await expect(place.getByText("You are watching: raise your hand to ask the host to play")).toBeVisible();
  await expect(place.getByRole("button", { name: "Join the queue" })).toHaveCount(0);

  // Sol raises a hand from the dock; the host hears it and sees it on the players.
  await wake(sol.page);
  await sol.page.locator(".video-hand").dispatchEvent("click");
  await expect(place.getByText("Your hand is up: the host will see it")).toBeVisible();
  await shot(sol.page, "1-spectator-hand-up");
  await wake(page);
  const hands = page.getByRole("button", { name: "1 raised hand" });
  await expect(hands).toBeVisible();
  await expect(page.getByText("Sol raised a hand")).toBeVisible();
  await shot(page, "2-host-sees-hand");
  await hands.click();
  await expect(page.getByRole("group", { name: "Sol raised a hand" })).toBeVisible();
  await shot(page, "3-host-hand-list");
  await expectAccessible(page, "host's raised hands");
  await page.getByRole("menuitem", { name: "Not now" }).click();
  await expect(sol.page.locator(".hand-note")).toHaveText("The host will let you play later");
  await expect(hands).toHaveCount(0);
  await shot(sol.page, "4-spectator-not-now");
  // Again at once: once a minute.
  await wake(sol.page);
  await sol.page.locator(".video-hand").dispatchEvent("click");
  await expect(sol.page.getByText(/You can raise it again in \d+ s/)).toBeVisible();

  // The host seats Sol right away from the spectators' list: a free seat, no one loses a turn.
  await page.getByRole("tablist", { name: "Room", exact: true }).getByRole("tab", { name: /Spectators/ }).click();
  await page.getByRole("button", { name: "What to do with Sol" }).click();
  await shot(page, "5-host-guest-menu");
  await page.getByRole("menuitem", { name: "Play now" }).click();
  await expect(place.getByText("You are playing")).toBeVisible();
  await expect(sol.page.getByText(/seated Sol at P\d/).first()).toBeVisible();
  await shot(sol.page, "6-spectator-plays");

  // Leo comes in, and the host takes Leo out: the stream stops and a reload does not bring Leo back.
  const inv2 = await newInvitation();
  const leo = await joinAsGuest(browser, inv2.code, inv2.pin);
  await enterName(leo.page, "Leo");
  await expectVideoPlaying(leo.page);
  await wake(page);
  await page.getByRole("button", { name: /· Leo/ }).click();
  await page.getByRole("menuitem", { name: "Remove Leo from the room" }).click();
  await expect(leo.page.getByRole("heading", { name: "The host took you out of this room" })).toBeVisible();
  await shot(leo.page, "7-removed");
  await expect(sol.page.getByText("took Leo out of the room").first()).toBeVisible();
  await leo.page.reload();
  await expect(leo.page.getByRole("heading", { name: "The host took you out of this room" })).toBeVisible({ timeout: 30_000 });
  await expectVideoPlaying(sol.page);
  await sol.context.close();
  await leo.context.close();
});

test("the queue talks with the players; spectators hear the voices until the host turns that off", async ({ browser }) => {
  test.setTimeout(240_000);
  const shot = async (p: Page, name: string) => {
    if (process.env.E2E_VOICE_SHOTS) await p.screenshot({ path: `${process.env.E2E_VOICE_SHOTS}/${name}.png` });
  };
  const join = async (name: string) => {
    const inv = await newInvitation();
    const g = await joinAsGuest(browser, inv.code, inv.pin);
    await enterName(g.page, name);
    await expectVideoPlaying(g.page);
    // "Your place", the queue and the spectators are in the Chat tab.
    await g.page.getByRole("tablist", { name: "Room panel" }).getByRole("tab", { name: "Chat" }).click();
    return g;
  };
  // Zed only watches; Sol, Leo and Ivy fill the seats; Max waits in the queue.
  const zed = await join("Zed");
  await zed.page.locator(".side-status").getByRole("button", { name: "Watch instead" }).click();
  await expect(zed.page.locator(".side-status").getByText("You are watching")).toBeVisible();
  const others = [await join("Sol"), await join("Leo"), await join("Ivy")];
  const max = await join("Max");
  await expect(max.page.locator(".side-status").getByText("You are next in the queue")).toBeVisible();

  // Max, in the queue, turns the microphone on (Chrome's fake one beeps).
  await clickControl(max.page, "microphone muted");
  const speakingMax = (p: Page) => p.locator(".queue-row", { hasText: "Max" }).getByRole("img", { name: "Speaking" });
  const queueTab = (p: Page) => p.getByRole("tablist", { name: "Room", exact: true }).getByRole("tab", { name: /Queue/ });
  await queueTab(page).click();
  await queueTab(zed.page).click();
  await expect(speakingMax(page)).toBeVisible({ timeout: 30_000 });
  await shot(page, "1-host-hears-the-queue");
  await expect(speakingMax(zed.page)).toBeVisible({ timeout: 30_000 });
  await shot(zed.page, "2-spectator-hears");

  // The host turns off "Spectators hear the voices": Zed no longer hears Max, the host still does.
  await clickControl(page, "Volume and voice");
  const hearSwitch = page.getByRole("switch", { name: "Spectators hear the voices" });
  await expect(hearSwitch).toHaveAttribute("aria-checked", "true");
  await shot(page, "3-host-switch");
  await hearSwitch.click();
  await expect(hearSwitch).toHaveAttribute("aria-checked", "false");
  await page.keyboard.press("Escape");
  await zed.page.waitForTimeout(1500);
  for (let i = 0; i < 6; i++) {
    await expect(speakingMax(zed.page)).toHaveCount(0);
    await zed.page.waitForTimeout(500);
  }
  await expect(speakingMax(page)).toBeVisible({ timeout: 30_000 });
  // Back on for the next tests.
  await clickControl(page, "Volume and voice");
  await hearSwitch.click();
  await expect(hearSwitch).toHaveAttribute("aria-checked", "true");
  await page.keyboard.press("Escape");
  for (const g of [zed, ...others, max]) await g.context.close();
});

test("a spectators' QR code lets people in to watch, each one let in by the host; spectators get the lighter video", async ({ browser }) => {
  test.setTimeout(150_000);
  const shot = async (p: Page, name: string) => {
    if (process.env.E2E_WATCH_SHOTS) await p.screenshot({ path: `${process.env.E2E_WATCH_SHOTS}/${name}.png` });
  };
  await page.getByRole("button", { name: "Invite", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: /Invite to/ });
  await dialog.getByRole("button", { name: "Spectators" }).click();
  await expect(dialog.getByRole("checkbox", { name: /Ask me first/ })).toBeChecked();
  const saver = dialog.getByRole("switch", { name: "Lighter video for spectators" });
  await expect(saver).toHaveAttribute("aria-checked", "true");
  await dialog.getByRole("button", { name: "Make the spectators' QR code" }).click();
  await expect(dialog.getByText("0 of 10 people came in")).toBeVisible();
  await shot(page, "1-host-spectators-qr");
  await expectAccessible(page, "spectators' QR code");
  const link = (await dialog.locator(".invite-url[data-link]").getAttribute("data-link")) ?? "";
  expect(link).toMatch(/\/g\/[A-Za-z0-9_-]{22}#k=[A-Za-z0-9_-]{43}$/);
  await dialog.getByRole("button", { name: "Got it" }).click();

  // Ana opens the link: a name for the host, then a wait.
  const context = await browser.newContext();
  const ana = await context.newPage();
  await ana.goto(link);
  await expect(ana.locator("#splash")).toBeHidden();
  const form = ana.locator(".knock-prompt");
  await form.getByRole("textbox", { name: "Your name" }).fill("Ana");
  await form.getByRole("checkbox", { name: /I have read and accept/ }).check();
  await form.getByRole("button", { name: "Ask to come in" }).click();
  await expect(ana.getByText("Waiting for the host")).toBeVisible();
  // The host sees that Ana comes to watch, and lets her in.
  const row = page.locator(".knock-row", { hasText: "Ana" });
  await expect(row.getByText("to watch")).toBeVisible();
  await shot(page, "2-host-sees-who-comes-to-watch");
  await page.getByRole("button", { name: "Let Ana in", exact: true }).click();
  await expectVideoPlaying(ana);
  // Ana watches, and plays only through a raised hand.
  await ana.getByRole("tablist", { name: "Room panel" }).getByRole("tab", { name: "Chat" }).click();
  const place = ana.locator(".side-status");
  await expect(place.getByText("You are watching: raise your hand to ask the host to play")).toBeVisible();
  await expect(place.getByRole("button", { name: "Raise your hand to play" })).toBeVisible();
  await expect(place.getByRole("button", { name: "Join the queue" })).toHaveCount(0);
  await shot(ana, "3-spectator-in");

  // The host's switch: the players' video for spectators, and back.
  await page.getByRole("button", { name: "Invite", exact: true }).click();
  await dialog.getByRole("button", { name: "Spectators" }).click();
  await expect(dialog.getByText("1 of 10 people came in")).toBeVisible();
  await saver.click();
  await expect(saver).toHaveAttribute("aria-checked", "false");
  await saver.click();
  await expect(saver).toHaveAttribute("aria-checked", "true");
  // Stopping the QR code: Ana stays.
  await dialog.getByRole("button", { name: "Stop this QR code" }).click();
  await expect(dialog.getByRole("button", { name: "Make the spectators' QR code" })).toBeVisible();
  await dialog.getByRole("button", { name: "One person" }).click();
  await dialog.getByRole("button", { name: "Got it" }).click();
  await expectVideoPlaying(ana);
  await context.close();
});

test("joining as a player asks \"Do you want to join the voice?\", and so does moving from watching to playing", async ({ browser }) => {
  test.setTimeout(150_000);
  const shot = async (p: Page, name: string) => {
    if (process.env.E2E_ASK_SHOTS) await p.screenshot({ path: `${process.env.E2E_ASK_SHOTS}/${name}.png` });
  };
  const join = async (name: string) => {
    const inv = await newInvitation();
    const g = await joinAsGuest(browser, inv.code, inv.pin);
    await enterName(g.page, name);
    await expectVideoPlaying(g.page);
    return g;
  };
  const ask = (p: Page) => p.getByRole("dialog", { name: "Do you want to join the voice?" });
  // Tom says yes: his microphone turns on.
  const tom = await join("Tom");
  await expect(ask(tom.page)).toBeVisible();
  await shot(tom.page, "1-asked");
  await expectAccessible(tom.page, "the voice question");
  await ask(tom.page).getByRole("button", { name: "Turn on my microphone" }).click();
  await expect(ask(tom.page)).toHaveCount(0);
  await wake(tom.page);
  await expect(tom.page.getByRole("button", { name: "microphone on" })).toBeVisible();
  await shot(tom.page, "2-microphone-on");
  // Uma says not now: her microphone stays off.
  const uma = await join("Uma");
  await ask(uma.page).getByRole("button", { name: "Not now" }).click();
  await expect(ask(uma.page)).toHaveCount(0);
  await wake(uma.page);
  await expect(uma.page.getByRole("button", { name: "microphone muted" })).toBeVisible();
  // Watching, Uma is not asked; seated again by the host, she is.
  await wake(page);
  await page.getByRole("button", { name: /· Uma/ }).click();
  await page.getByRole("menuitem", { name: "Make Uma a spectator" }).click();
  await uma.page.getByRole("tablist", { name: "Room panel" }).getByRole("tab", { name: "Chat" }).click();
  await expect(uma.page.locator(".side-status").getByText("You are watching", { exact: true })).toBeVisible();
  await expect(ask(uma.page)).toHaveCount(0);
  await page.getByRole("tablist", { name: "Room", exact: true }).getByRole("tab", { name: /Spectators/ }).click();
  await page.getByRole("button", { name: "What to do with Uma" }).click();
  await page.getByRole("menuitem", { name: "Play now" }).click();
  await expect(uma.page.locator(".side-status").getByText("You are playing")).toBeVisible();
  await expect(ask(uma.page)).toBeVisible();
  await shot(uma.page, "3-asked-again");
  await tom.context.close();
  await uma.context.close();
});

test("the controller test page lights the keyboard's buttons", async ({ browser }) => {
  const context = await browser.newContext();
  const p = await context.newPage();
  await p.goto("/test-controller");
  await expect(p.getByRole("heading", { level: 1, name: "Test controller" })).toBeVisible();
  await p.keyboard.down("ArrowLeft");
  await p.keyboard.down("KeyX");
  await expect(p.getByText("Left · Button 2")).toBeVisible();
  await expect(p.locator(".test-pad .tc-lamp.is-on")).toHaveCount(2);
  await expect(p.locator(".test-controller-latency")).toContainText("ms");
  await p.keyboard.up("ArrowLeft");
  await p.keyboard.up("KeyX");
  await expect(p.getByText("Nothing pressed yet")).toBeVisible();
  await context.close();
});

/** A browser with no device of its own joins with the code and the PIN. */
async function joinAsGuest(browser: Browser, code: string, pin: string, prepare?: (context: BrowserContext) => Promise<void>) {
  const context = await browser.newContext();
  await prepare?.(context);
  const p = await context.newPage();
  await p.goto("/");
  await p.getByRole("button", { name: "Join a game" }).first().click();
  const dialog = p.getByRole("dialog", { name: "Join a game" });
  await dialog.getByPlaceholder("123 456 789").fill(code);
  await dialog.getByPlaceholder("000000").fill(pin);
  await dialog.getByRole("checkbox", { name: /I have read and accept/ }).check();
  await dialog.getByRole("button", { name: "Join", exact: true }).click();
  return { context, page: p };
}

test("the device's local web panel opens with its token and plays its rooms", async ({ browser }) => {
  const context = await browser.newContext();
  const p = await context.newPage();
  await p.goto(`http://127.0.0.1:${PORTS.panel}/`);
  await expect(p.getByRole("heading", { name: "Device panel" })).toBeVisible();
  await p.getByLabel("Panel token").fill(panelToken());
  await p.getByRole("checkbox", { name: /I have read and accept/ }).check();
  await p.getByRole("button", { name: "Open the panel" }).click();
  // The panel opens on the rooms; the device page shows it linked.
  await p.getByRole("navigation", { name: "Main" }).getByRole("link", { name: "My device" }).click();
  await expect(p.getByText("Linked · live")).toBeVisible();
  // Playing from the panel: signalhub refuses the panel's origin, so the
  // room goes through the device's own socket.
  await p.getByRole("link", { name: "Test pattern" }).click();
  await expectVideoPlaying(p);
  await context.close();
});
