// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

/**
 * The version of the terms of use and privacy policy (docs/legal.md). A new
 * version asks for acceptance again before linking a device or joining a
 * game.
 */
export const TERMS_VERSION = "2026-10-12";

const KEY = "go-link.terms";

/** Whether this browser accepted the current terms. */
export function termsAccepted(): boolean {
  try {
    const saved = JSON.parse(localStorage.getItem(KEY) ?? "null") as { version?: unknown } | null;
    return saved?.version === TERMS_VERSION;
  } catch {
    return false;
  }
}

/** Remembers that this browser's user accepted the current terms. */
export function acceptTerms(): void {
  try {
    localStorage.setItem(KEY, JSON.stringify({ version: TERMS_VERSION, at: new Date().toISOString() }));
  } catch {
    // Private mode: the checkbox is asked again next time.
  }
}
