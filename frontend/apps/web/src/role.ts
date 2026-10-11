// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// The website is built twice from this one app:
//
//   site   console.go-link.org: the landing, the guide, the tools and the legal pages.
//          It never connects to signaling and keeps no link to a device.
//   play   play.go-link.org: rooms, invitations and My device (the link to
//          the device lives in this origin's storage). The device's local
//          web panel is this build too.
//   all    every page on one origin: development (npm run dev) and tests.
//
// A page of the other build is reached at its own address: links to it go
// through a route that sends the browser there (OtherSite in App.tsx).

export type Role = "all" | "site" | "play";

function readRole(value: string | undefined): Role {
  return value === "site" || value === "play" ? value : "all";
}

/** Which build this is (VITE_ROLE). */
export const ROLE: Role = readRole(import.meta.env.VITE_ROLE);

// Plain comparisons of the build's constant, so the bundler drops the
// other site's pages from each build.
/** This build has the landing's pages. */
export const HAS_SITE = import.meta.env.VITE_ROLE !== "play";
/** This build has the rooms and My device. */
export const HAS_PLAY = import.meta.env.VITE_ROLE !== "site";

const DEV = import.meta.env.DEV;
const trim = (url: string) => url.replace(/\/+$/, "");

/** The landing's address (VITE_SITE_URL). */
export const SITE_URL = trim(import.meta.env.VITE_SITE_URL || (DEV ? "http://localhost:5182" : "https://console.go-link.org"));

/** The rooms' address (VITE_PLAY_URL). */
export const PLAY_URL = trim(import.meta.env.VITE_PLAY_URL || (DEV ? "http://localhost:5183" : "https://play.go-link.org"));

/** The address invitations are shared with (VITE_INVITE_URL): go-link.org, whose /g/ the Player apps open (the landing's address in development). */
export const INVITE_URL = trim(import.meta.env.VITE_INVITE_URL || (DEV ? SITE_URL : "https://go-link.org"));

/** Which build serves a path: "both" for pages each one has. */
export function ownerOf(pathname: string): "site" | "play" | "both" {
  // Routes match any letter case.
  const first = (pathname.split("/")[1] ?? "").toLowerCase();
  switch (first) {
    case "rooms":
    case "create":
    case "r":
    case "g":
    case "device":
      return "play";
    case "test-controller": // its settings are the ones rooms and the mini-games use
    case "maker-bridge": // play: the link for Willy Maker; site: the games made here before
    case "terms":
    case "privacy":
      return "both";
    default:
      return "site";
  }
}

/** Whether this build has the page at this path. */
export function servesPath(pathname: string, role: Role = ROLE): boolean {
  const owner = ownerOf(pathname);
  return role === "all" || owner === "both" || owner === role;
}

/**
 * The address of a page: the path itself when this build has it, else the
 * full address on the other site ("/docs" on play → https://console.go-link.org/docs).
 */
export function pageHref(path: string, role: Role = ROLE): string {
  if (servesPath(path.split(/[?#]/)[0] ?? "/", role)) return path;
  return `${role === "site" ? PLAY_URL : SITE_URL}${path}`;
}

/** The landing: "/" here, or the landing's own site from play. */
export function homeHref(role: Role = ROLE): string {
  return role === "play" ? `${SITE_URL}/` : "/";
}

/** The full address of a page, on whichever site serves it. */
export function pageUrl(path: string, role: Role = ROLE, origin: string = window.location.origin): string {
  const href = pageHref(path, role);
  return href.startsWith("/") ? `${origin}${href}` : href;
}

/**
 * The address to share for an invitation: go-link.org/g/<invite> (INVITE_URL) from the
 * rooms' site, the one the Player apps open (go-link.org sends browsers on
 * to play). The device's local panel and development keep their own.
 */
export function invitationUrl(invite: string, panel: boolean, role: Role = ROLE, origin: string = window.location.origin): string {
  return `${role === "play" && !panel ? INVITE_URL : origin}/g/${invite}`;
}
