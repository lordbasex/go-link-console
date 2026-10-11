// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { expect, test } from "@playwright/test";
import { PLAY_URL, SITE_URL } from "../ports";

// The website's two builds on their own origins: the landing (console.go-link.org)
// and the rooms (play.go-link.org). Each sends the other's pages to the
// other site, and the rooms can take the device link a browser kept on the
// landing before the split.

test("each site sends the other's pages there", async ({ page }) => {
  await page.goto(`${SITE_URL}/`);
  await expect(page.locator(".lp-hero")).toBeVisible();
  await page.locator("nav.main-nav").getByRole("link", { name: "Rooms" }).click();
  await expect(page).toHaveURL(`${PLAY_URL}/rooms`);

  // the rooms' site has no landing: its logo and "How it works" go to the landing's site
  await expect(page.locator("a.brand").first()).toHaveAttribute("href", `${SITE_URL}/`);
  await page.goto(`${PLAY_URL}/docs/install`);
  await expect(page).toHaveURL(`${SITE_URL}/docs/install`);

  // an invitation shared as play.go-link.org/g/<invite> opens on the rooms' site
  await page.goto(`${SITE_URL}/g/AAAAAAAAAAAAAAAAAAAAAA`);
  await expect(page).toHaveURL(`${PLAY_URL}/g/AAAAAAAAAAAAAAAAAAAAAA`);

  // pages both sites have stay where they are
  await page.goto(`${SITE_URL}/terms`);
  await expect(page).toHaveURL(`${SITE_URL}/terms`);
  await page.goto(`${PLAY_URL}/test-controller`);
  await expect(page).toHaveURL(`${PLAY_URL}/test-controller`);
});

test("the rooms' site brings the link and settings this browser kept on the landing", async ({ page }) => {
  const link = JSON.stringify({ deviceId: "d1", linkId: "l1", token: "tok", signalUrl: "ws://127.0.0.1:1/ws", savedAt: 1 });
  await page.goto(`${SITE_URL}/terms`);
  await page.evaluate((value) => {
    localStorage.setItem("go-link.device-link", value);
    localStorage.setItem("go-link.player-name", "Ana");
    localStorage.setItem("go-link.skin-editor", "{}");
  }, link);

  // the landing's link to My device carries ?import=1 while it keeps a link
  await page.goto(`${SITE_URL}/device`);
  await expect(page).toHaveURL(`${PLAY_URL}/device?import=1`);
  const banner = page.locator(".handoff-banner");
  await expect(banner).toBeVisible();

  const popup = page.waitForEvent("popup");
  await banner.getByRole("button", { name: "Bring my settings" }).click();
  const tab = await popup;
  await expect(tab).toHaveURL(`${SITE_URL}/handoff`);

  // play reloads without ?import=1, with the landing's values
  await expect(page).toHaveURL(`${PLAY_URL}/device`);
  const moved = await page.evaluate(() => [localStorage.getItem("go-link.device-link"), localStorage.getItem("go-link.player-name"), localStorage.getItem("go-link.skin-editor")]);
  expect(moved).toEqual([link, "Ana", null]);

  // the landing forgets the link once the rooms' site has it
  await expect(tab.getByText(/play\.go-link\.org has your settings/)).toBeVisible();
  const left = await page.context().newPage();
  await left.goto(`${SITE_URL}/terms`);
  expect(await left.evaluate(() => [localStorage.getItem("go-link.device-link"), localStorage.getItem("go-link.player-name")])).toEqual([null, "Ana"]);

  // and its links to the rooms no longer ask to bring anything
  await left.goto(`${SITE_URL}/rooms`);
  await expect(left).toHaveURL(`${PLAY_URL}/rooms`);
});

test("a page of another site that opens the landing's handoff receives nothing", async ({ page }) => {
  await page.goto(`${SITE_URL}/terms`);
  await page.evaluate(() => localStorage.setItem("go-link.device-link", "{\"token\":\"secret\"}"));
  // the landing itself plays the stranger: it opens /handoff and listens
  const got = await page.evaluate(
    (url) =>
      new Promise<unknown[]>((resolve) => {
        const seen: unknown[] = [];
        window.addEventListener("message", (e) => seen.push(e.data));
        window.open(url, "_blank");
        setTimeout(() => resolve(seen), 3000);
      }),
    `${SITE_URL}/handoff`,
  );
  expect(got).toEqual([]);
  expect(await page.evaluate(() => localStorage.getItem("go-link.device-link"))).toBe("{\"token\":\"secret\"}");
});
