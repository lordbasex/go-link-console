// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
// The device downloads of the latest release, from release.json (written by
// scripts/release-info.sh when a release is cut).
import release from "./release.json";
import { REPO_URL } from "./config";

export type Platform = "windows" | "macos" | "linux" | "docker" | "pi";

export const PLATFORMS: readonly Platform[] = ["windows", "macos", "linux", "docker", "pi"];

export const RELEASE_VERSION: string = release.version;
export const RELEASE_PAGE = `${REPO_URL}/releases/tag/${release.version}`;
/** The device image on the GitHub Container Registry (amd64 and arm64). */
export const DOCKER_IMAGE = "ghcr.io/lordbasex/go-link-console:latest";
export const CHECKSUMS_URL = `${REPO_URL}/releases/download/${release.version}/SHA256SUMS`;

const assets: Record<string, number> = release.assets;

/** The file of each platform (and its ARM alternative), by the release's naming. */
const FILES: Record<Platform, { main: string; arm?: string }> = {
  windows: { main: "windows-amd64.zip", arm: "windows-arm64.zip" },
  macos: { main: "macos-universal.dmg" },
  linux: { main: "linux-amd64.tar.gz", arm: "linux-arm64.tar.gz" },
  docker: { main: "docker.oci.tar.gz" },
  pi: { main: "linux-arm64-headless.tar.gz" },
};

export interface Download {
  url: string;
  /** Size in MB, rounded (0 when the release lacks it). */
  mb: number;
}

function asset(key: string | undefined): Download | null {
  if (!key || !(key in assets)) return null;
  return {
    url: `${REPO_URL}/releases/download/${release.version}/go-link-console-${release.version}-${key}`,
    mb: Math.max(1, Math.round((assets[key] ?? 0) / 1_000_000)),
  };
}

/** The main download of a platform, null when this release has none. */
export function downloadFor(p: Platform): Download | null {
  return asset(FILES[p].main);
}

/** The ARM build of a platform, where there is one. */
export function armDownloadFor(p: Platform): Download | null {
  return asset(FILES[p].arm);
}

/** The go-link Console Player app for Android (a signed APK), null when this release has none. */
export function androidDownload(): Download | null {
  return asset("android.apk");
}

/**
 * The visitor's computer, to preselect its download. null on phones and
 * tablets (go-link runs on a computer).
 */
export function detectPlatform(nav: Pick<Navigator, "userAgent"> = navigator): Platform | null {
  const ua = nav.userAgent;
  if (/Android|iPhone|iPad|iPod/i.test(ua)) return null;
  if (/Macintosh/i.test(ua) && typeof navigator !== "undefined" && navigator.maxTouchPoints > 1) return null; // iPadOS
  if (/Windows/i.test(ua)) return "windows";
  if (/Mac OS X|Macintosh/i.test(ua)) return "macos";
  if (/Linux|X11|CrOS/i.test(ua)) return "linux";
  return null;
}
