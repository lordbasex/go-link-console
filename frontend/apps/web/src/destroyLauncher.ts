// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Starts the "Destroy this page" game on the landing (the header's devil).
// The game is a separate chunk loaded only on the first click, and it plays
// on the whole app (#root); this file only knows its public entry.

import { useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { t } from "./i18n";
import { hideSplash } from "./splash";

type Session = { exit(): void; finish?: () => void };
let active: Session | null = null;
let starting = false;

/** The characters the game loads first (atlas JSON + picture each). */
const ATLASES = ["player", "npcs", "robot", "blonde", "alien"];
let prefetched = false;

/**
 * Starts downloading the game and its characters before they are needed
 * (the pointer over the devil, its focus, the direct link), so the briefing
 * opens with its pictures ready. Safe to call many times.
 */
export function prefetchDestroy(): void {
  if (prefetched || typeof window === "undefined") return;
  prefetched = true;
  void import("./destroy").catch(() => undefined);
  for (const name of ATLASES) {
    // The manifest names its picture (WebP or PNG); both land in the cache.
    void fetch(`/destroy/${name}.json`)
      .then((r) => r.json() as Promise<{ image?: string }>)
      .then((m) => {
        if (!m.image) return;
        const img = new Image();
        img.src = `/destroy/${m.image}`;
      })
      .catch(() => undefined);
  }
}

/**
 * Waits until the landing is on screen (after a navigation to "/"): by
 * default also until the startup logo is gone; `overSplash` only waits for
 * the landing under it (the direct link lifts the logo itself).
 */
function landingReady(overSplash = false, timeoutMs = 6000): Promise<boolean> {
  const t0 = performance.now();
  return new Promise((resolve) => {
    const check = () => {
      const splash = document.getElementById("splash");
      const ready = document.querySelector(".page.lp") && (overSplash || !splash || splash.classList.contains("is-gone"));
      if (ready) return resolve(true);
      if (performance.now() - t0 > timeoutMs) return resolve(false);
      window.setTimeout(check, 100);
    };
    check();
  });
}

/**
 * Opens the mission briefing; on another page it goes to "/" first. With
 * `overSplash` (the direct link) it does not wait for the startup logo: it
 * opens the game as soon as the landing is there and lifts the logo.
 */
export async function launchDestroy(pathname: string, navigate: (to: string) => void, overSplash = false): Promise<void> {
  if (active || starting) return;
  starting = true;
  prefetchDestroy();
  try {
    if (pathname !== "/") navigate("/");
    const [mod, ok] = await Promise.all([import("./destroy"), landingReady(overSplash)]);
    const root = document.getElementById("root");
    if (!ok || !root) return;
    active = mod.startDestroy({
      root,
      messages: t.destroy,
      onExit: () => {
        active = null;
      },
      debug: import.meta.env.DEV,
    });
    if (overSplash) hideSplash();
    // Development only: lets end-to-end checks finish a mission at once.
    if (import.meta.env.DEV) (window as unknown as { __destroy?: Session }).__destroy = active;
  } finally {
    starting = false;
  }
}

/**
 * The game's direct link (console.go-link.org/WillyGorklingo): goes to the landing,
 * replacing the link in the history, and opens the mission briefing there.
 */
export function DestroyLink(): null {
  const navigate = useNavigate();
  useEffect(() => {
    prefetchDestroy();
    void launchDestroy("/WillyGorklingo", (to) => navigate(to, { replace: true }), true);
  }, [navigate]);
  return null;
}
