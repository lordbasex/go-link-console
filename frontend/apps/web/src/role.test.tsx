// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { beforeEach, describe, expect, it } from "vitest";
import { PLAY_URL, SITE_URL, homeHref, invitationUrl, ownerOf, pageHref, pageUrl, servesPath } from "./role";
import { acceptEntries, forgetSecrets, handoffEntries, hasHandoffData, otherSiteAddress } from "./handoff";

describe("the two sites", () => {
  it("knows which site serves each page", () => {
    expect(ownerOf("/rooms")).toBe("play");
    expect(ownerOf("/r/abc")).toBe("play");
    expect(ownerOf("/g/xyz")).toBe("play");
    expect(ownerOf("/Device/roms")).toBe("play");
    expect(ownerOf("/")).toBe("site");
    expect(ownerOf("/docs/install")).toBe("site");
    expect(ownerOf("/tools/games")).toBe("site");
    expect(ownerOf("/test-controller")).toBe("both");
    expect(ownerOf("/maker-bridge")).toBe("both");
    expect(ownerOf("/terms")).toBe("both");
  });

  it("links to the other site's pages by their full address", () => {
    expect(servesPath("/docs", "play")).toBe(false);
    expect(servesPath("/docs", "all")).toBe(true);
    expect(pageHref("/docs/install", "play")).toBe(`${SITE_URL}/docs/install`);
    expect(pageHref("/device?x=1", "site")).toBe(`${PLAY_URL}/device?x=1`);
    expect(pageHref("/device", "play")).toBe("/device");
    expect(pageHref("/terms", "site")).toBe("/terms");
    expect(homeHref("play")).toBe(`${SITE_URL}/`);
    expect(homeHref("site")).toBe("/");
    expect(pageUrl("/device", "site", "https://here")).toBe(`${PLAY_URL}/device`);
    expect(pageUrl("/device", "all", "https://here")).toBe("https://here/device");
  });

  it("shares invitations on the rooms' address, which the apps open", () => {
    expect(invitationUrl("abc", false, "play", "https://p")).toBe(`${PLAY_URL}/g/abc`);
    // the device's local panel and development keep their own address
    expect(invitationUrl("abc", true, "play", "http://192.0.2.7:7373")).toBe("http://192.0.2.7:7373/g/abc");
    expect(invitationUrl("abc", false, "all", "http://localhost:5180")).toBe("http://localhost:5180/g/abc");
  });
});

describe("handoff", () => {
  let store: Storage;
  beforeEach(() => {
    window.localStorage.clear();
    store = window.localStorage;
  });

  it("sends the landing's go-link settings, never its own pages' data", () => {
    store.setItem("go-link.device-link", '{"token":"t"}');
    store.setItem("go-link.input", "{}");
    store.setItem("go-link.wm.index", "[]");
    store.setItem("go-link.skin-editor", "{}");
    store.setItem("go-link.skin-editor.welcome", "1");
    store.setItem("go-link.landing-sound", "on");
    store.setItem("other.key", "x");
    expect(handoffEntries(store).map(([k]) => k).sort()).toEqual(["go-link.device-link", "go-link.input"]);
  });

  it("asks play to take them only while the landing keeps a link or passes", () => {
    expect(hasHandoffData(store)).toBe(false);
    store.setItem("go-link.room-passes", "{}");
    expect(hasHandoffData(store)).toBe(true);
    expect(otherSiteAddress("/device?tab=1#x", "site", true)).toBe(`${PLAY_URL}/device?tab=1&import=1#x`);
    expect(otherSiteAddress("/g/abc", "site", false)).toBe(`${PLAY_URL}/g/abc`);
    expect(otherSiteAddress("/docs", "play", true)).toBe(`${SITE_URL}/docs`);
    forgetSecrets(store);
    expect(hasHandoffData(store)).toBe(false);
  });

  it("keeps play's own values and refuses anything that is not a go-link setting", () => {
    store.setItem("go-link.lang", "pt");
    const stored = acceptEntries(
      [
        ["go-link.lang", "es"],
        ["go-link.device-link", '{"token":"t"}'],
        ["go-link.wm.p.1", "{}"],
        ["evil", "x"],
        ["go-link.input", 5],
        ["go-link.big", "x".repeat(300 * 1024)],
        "junk",
      ],
      store,
    );
    expect(stored).toBe(1);
    expect(store.getItem("go-link.lang")).toBe("pt");
    expect(store.getItem("go-link.device-link")).toBe('{"token":"t"}');
    expect(store.getItem("go-link.wm.p.1")).toBeNull();
    expect(store.getItem("evil")).toBeNull();
    expect(acceptEntries("nope", store)).toBe(0);
  });
});
