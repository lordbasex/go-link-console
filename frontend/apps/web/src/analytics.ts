// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useSyncExternalStore } from "react";

/**
 * Google Analytics 4 on the landing's site (console.go-link.org) only, and
 * only with the visitor's consent. The rooms' site (play.go-link.org) and
 * the device's local panel never run it, and neither do the pages that
 * carry the device's keys (/handoff, /maker-bridge): once Google's script
 * is in a page, moving to one of those is a full page load
 * (guardHistory), so they start clean under the strict CSP.
 */
export const GA_ID = "G-JF0SPS21MX";
const KEY = "go-link.consent";
// Console's own cookies (console__ga, console__ga_<id>) on this host only,
// apart from the other go-link sites': a choice here never touches theirs.
const COOKIE_PREFIX = "console";

export type Consent = "granted" | "denied" | null;

/**
 * Whether this build may ask at all: only the landing's own site. Written
 * against the build's constant (like src/role.ts) so the other builds leave
 * this code out.
 */
export const ANALYTICS_SITE = import.meta.env.VITE_ROLE === "site";

/** The pages that may count visits: no keys, codes or PINs in them. */
export function isPublicPath(path: string): boolean {
  const p = path.toLowerCase();
  return (
    p === "/" ||
    p === "/docs" ||
    p.startsWith("/docs/") ||
    p === "/tools" ||
    p.startsWith("/tools/") ||
    p === "/test-controller" ||
    p === "/picture-demo" ||
    p === "/terms" ||
    p === "/privacy"
  );
}

function readConsent(): Consent {
  try {
    const v = window.localStorage.getItem(KEY);
    return v === "granted" || v === "denied" ? v : null;
  } catch {
    return null;
  }
}

let consent: Consent = typeof window === "undefined" ? null : readConsent();
let asking = false;
let loaded = false;
const listeners = new Set<() => void>();
const notify = () => listeners.forEach((l) => l());

function subscribe(l: () => void): () => void {
  listeners.add(l);
  return () => listeners.delete(l);
}

/** The visitor's choice and whether the notice is open, for the banner. */
export function useConsent(): { consent: Consent; asking: boolean } {
  const c = useSyncExternalStore(subscribe, () => consent);
  const a = useSyncExternalStore(subscribe, () => asking);
  return { consent: c, asking: a };
}

/**
 * Saves the choice. Rejecting stops counting at once; accepting lets App
 * start it, on a public page only.
 */
export function setConsent(value: "granted" | "denied"): void {
  consent = value;
  asking = false;
  try {
    window.localStorage.setItem(KEY, value);
  } catch {
    // storage blocked: the choice lasts until the page closes
  }
  if (value === "denied") stopAnalytics();
  notify();
}

/** Opens the notice again ("Cookie settings"). */
export function askConsent(): void {
  asking = true;
  notify();
}

type GtagWindow = Window & { dataLayer?: unknown[]; gtag?: (...args: unknown[]) => void } & Record<string, unknown>;

/** Loads gtag.js once; only on this site, on a public page and with consent. */
export function startAnalytics(win: GtagWindow = window as unknown as GtagWindow, site = ANALYTICS_SITE): void {
  if (!site || consent !== "granted" || !isPublicPath(win.location.pathname)) return;
  win[`ga-disable-${GA_ID}`] = false;
  if (loaded) return;
  loaded = true;
  win.dataLayer = win.dataLayer || [];
  win.gtag = function gtag() {
    // gtag.js reads the arguments object itself, as Google's snippet does.
    win.dataLayer!.push(arguments);
  };
  win.gtag("js", new Date());
  win.gtag("config", GA_ID, { cookie_prefix: COOKIE_PREFIX, cookie_domain: win.location.hostname });
  const s = win.document.createElement("script");
  s.async = true;
  s.src = `https://www.googletagmanager.com/gtag/js?id=${GA_ID}`;
  win.document.head.appendChild(s);
}

/** Stops sending and deletes Console's Google cookies (console__ga, console__ga_<id>). */
export function stopAnalytics(win: GtagWindow = window as unknown as GtagWindow): void {
  win[`ga-disable-${GA_ID}`] = true;
  for (const part of win.document.cookie.split(";")) {
    const name = (part.split("=")[0] ?? "").trim();
    if (!name.startsWith(`${COOKIE_PREFIX}_`)) continue;
    for (const domain of ["", `; domain=${win.location.hostname}`]) {
      win.document.cookie = `${name}=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/${domain}`;
    }
  }
}

/** Whether Google's script was loaded in this page. */
export function analyticsLoaded(): boolean {
  return loaded;
}

/**
 * Once Google's script is in the page, a move to a page that is not public
 * is a full page load instead of an in-page one. Installed before gtag.js,
 * so it also runs before gtag's own history listeners and nothing is sent
 * about those URLs.
 */
export function guardHistory(win: GtagWindow = window as unknown as GtagWindow): void {
  const leaving = (url: string | URL | null | undefined) =>
    loaded && url != null && !isPublicPath(new URL(String(url), win.location.href).pathname);
  for (const method of ["pushState", "replaceState"] as const) {
    const original = win.history[method].bind(win.history);
    win.history[method] = (data: unknown, unused: string, url?: string | URL | null) => {
      if (leaving(url)) {
        win[`ga-disable-${GA_ID}`] = true;
        win.location.assign(String(url));
        return;
      }
      original(data, unused, url);
    };
  }
  // Back and forward change the URL before anyone can stop it: turn
  // Google off at once and load that page fresh.
  win.addEventListener("popstate", () => {
    if (loaded && !isPublicPath(win.location.pathname)) {
      win[`ga-disable-${GA_ID}`] = true;
      win.location.reload();
    }
  });
}

/** For tests: forget the state of this module. */
export function resetAnalyticsForTests(): void {
  consent = readConsent();
  asking = false;
  loaded = false;
}
