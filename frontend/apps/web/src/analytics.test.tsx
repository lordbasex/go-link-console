// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  ANALYTICS_SITE,
  GA_ID,
  analyticsLoaded,
  guardHistory,
  isPublicPath,
  resetAnalyticsForTests,
  setConsent,
  startAnalytics,
  stopAnalytics,
} from "./analytics";

/** A window with its own location and history, so navigation can be watched. */
function fakeWindow(path: string) {
  const doc = document.implementation.createHTMLDocument("t");
  const pushed: string[] = [];
  const win = {
    document: doc,
    location: { pathname: path, href: `https://console.go-link.org${path}`, hostname: "console.go-link.org", assign: vi.fn(), reload: vi.fn() },
    history: {
      pushState: (_d: unknown, _u: string, url?: string | URL | null) => pushed.push(String(url)),
      replaceState: (_d: unknown, _u: string, url?: string | URL | null) => pushed.push(String(url)),
    },
    listeners: {} as Record<string, () => void>,
    addEventListener(type: string, fn: () => void) {
      this.listeners[type] = fn;
    },
  };
  return { win, pushed };
}
type W = Parameters<typeof startAnalytics>[0];

afterEach(() => {
  window.localStorage.clear();
  resetAnalyticsForTests();
});

describe("analytics", () => {
  it("is off in the tests' build: only the landing's site (VITE_ROLE=site) asks", () => {
    expect(ANALYTICS_SITE).toBe(false);
  });

  it("counts only the public pages, never the ones with the device's keys", () => {
    for (const p of ["/", "/docs", "/docs/rooms", "/tools", "/tools/skin-editor", "/test-controller", "/terms", "/privacy"]) {
      expect(isPublicPath(p)).toBe(true);
    }
    for (const p of ["/handoff", "/maker-bridge", "/rooms", "/device", "/r/abc", "/g/xyz", "/docs-x"]) expect(isPublicPath(p)).toBe(false);
  });

  it("loads nothing without consent, on another build, or on a page that is not public", () => {
    const home = fakeWindow("/");
    startAnalytics(home.win as unknown as W, true);
    setConsent("granted");
    startAnalytics(home.win as unknown as W, false);
    startAnalytics(fakeWindow("/handoff").win as unknown as W, true);
    expect(home.win.document.querySelector("script")).toBeNull();
    expect(analyticsLoaded()).toBe(false);
  });

  it("loads gtag.js once after Accept on a public page", () => {
    setConsent("granted");
    const home = fakeWindow("/docs");
    startAnalytics(home.win as unknown as W, true);
    startAnalytics(home.win as unknown as W, true);
    const scripts = home.win.document.querySelectorAll("script");
    expect(scripts).toHaveLength(1);
    expect(scripts[0]?.src).toBe(`https://www.googletagmanager.com/gtag/js?id=${GA_ID}`);
  });

  it("rejecting turns it off and deletes this site's Google cookies only", () => {
    const win = { document, location: window.location } as unknown as W;
    document.cookie = "console__ga=GA1.1.1; path=/";
    document.cookie = "_ga=GA1.1.2; path=/";
    stopAnalytics(win);
    expect((win as unknown as Record<string, unknown>)[`ga-disable-${GA_ID}`]).toBe(true);
    expect(document.cookie).not.toContain("console__ga");
    expect(document.cookie).toContain("_ga=GA1.1.2");
    document.cookie = "_ga=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/";
  });

  it("leaves for a page that is not public with a full page load once Google's script is in", () => {
    const { win, pushed } = fakeWindow("/");
    guardHistory(win as unknown as W);
    setConsent("granted");
    startAnalytics(win as unknown as W, true);
    win.history.pushState(null, "", "/docs");
    win.history.pushState(null, "", "/handoff");
    expect(pushed).toEqual(["/docs"]);
    expect(win.location.assign).toHaveBeenCalledWith("/handoff");
    win.location.pathname = "/maker-bridge";
    win.listeners.popstate?.();
    expect(win.location.reload).toHaveBeenCalled();
  });
});
