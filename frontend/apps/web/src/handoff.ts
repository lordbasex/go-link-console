// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Moving a browser's settings from go-link.org to play.go-link.org.
//
// Before the website was split, everything ran on go-link.org, so the link
// to the device, the rooms' return passes and the player's settings are in
// that origin's storage, which play.go-link.org cannot read. When the
// landing sends such a browser to play, it adds ?import=1; play then offers
// to bring them. On a click, play opens go-link.org/handoff in a new tab:
//
//   handoff → play   data {entries}     posted with play's origin as the only
//                                        allowed receiver (targetOrigin)
//   play → handoff   done {stored}      checked: play's origin and the opener
//
// After "done" the landing forgets the secrets (the device link and the
// passes): it never needs them again. Play keeps its own values when it
// already has a key, and accepts only "go-link." keys of a bounded size.

import { PLAY_URL, SITE_URL, type Role } from "./role";

export const HANDOFF_TAG = "go-link.handoff";
export const HANDOFF_PATH = "/handoff";

/** The query parameter that makes play offer to bring the settings. */
export const IMPORT_PARAM = "import";

/** The secrets: moved, then removed from the landing. */
const SECRETS = ["go-link.device-link", "go-link.room-passes"];

/** Keys of the landing's own pages (or of no page at all): they stay where they are. */
const STAYS = ["go-link.wm.", "go-link.skin-editor", "go-link.landing-sound", "go-link.panel-token", "go-link.handoff", "go-link.consent"];

const KEY = /^go-link\.[a-z0-9-]+(\.[a-z0-9-]+)*$/;
const MAX_KEY = 64;
const MAX_VALUE = 256 * 1024;
const MAX_ENTRIES = 64;

function moves(key: string): boolean {
  return key.length <= MAX_KEY && KEY.test(key) && !STAYS.some((s) => key === s || key.startsWith(s.endsWith(".") ? s : `${s}.`));
}

/** The keys play takes from the landing: every go-link setting but the landing's own. */
export function handoffEntries(storage: Storage): [string, string][] {
  const out: [string, string][] = [];
  for (let i = 0; i < storage.length; i++) {
    const key = storage.key(i);
    if (!key || !moves(key)) continue;
    const value = storage.getItem(key);
    if (value !== null && value.length <= MAX_VALUE) out.push([key, value]);
  }
  return out.slice(0, MAX_ENTRIES);
}

/** Whether the landing still keeps something only play uses: a device link or room passes. */
export function hasHandoffData(storage: Storage | undefined): boolean {
  try {
    return !!storage && SECRETS.some((k) => storage.getItem(k) !== null);
  } catch {
    return false;
  }
}

/** Writes the landing's settings that play does not have yet; returns how many. */
export function acceptEntries(entries: unknown, storage: Storage): number {
  if (!Array.isArray(entries)) return 0;
  let stored = 0;
  for (const entry of entries.slice(0, MAX_ENTRIES)) {
    if (!Array.isArray(entry) || entry.length !== 2) continue;
    const [key, value] = entry as unknown[];
    if (typeof key !== "string" || typeof value !== "string" || !moves(key) || value.length > MAX_VALUE) continue;
    if (storage.getItem(key) !== null) continue;
    storage.setItem(key, value);
    stored++;
  }
  return stored;
}

/** Removes the secrets from the landing once play has them. */
export function forgetSecrets(storage: Storage): void {
  for (const key of SECRETS) storage.removeItem(key);
}

/**
 * The address on the other site for a path of this one: the landing adds
 * ?import=1 when it still keeps a link or passes play should take.
 */
export function otherSiteAddress(path: string, role: Role, carry: boolean): string {
  const url = new URL(path, "http://x");
  if (role === "site" && carry) url.searchParams.set(IMPORT_PARAM, "1");
  const base = role === "site" ? PLAY_URL : SITE_URL;
  return `${base}${url.pathname}${url.search}${url.hash}`;
}

/** A handoff message's body, or null for anything else. */
export function handoffBody(data: unknown): Record<string, unknown> | null {
  if (!data || typeof data !== "object") return null;
  const d = data as Record<string, unknown>;
  return d.tag === HANDOFF_TAG && typeof d.kind === "string" ? d : null;
}
