// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { describe, expect, it } from "vitest";
import release from "./release.json";
import { armDownloadFor, detectPlatform, downloadFor, PLATFORMS } from "./downloads";

const ua = (userAgent: string) => ({ userAgent });

describe("downloads", () => {
  it("guesses the visitor's computer, and nothing on phones", () => {
    expect(detectPlatform(ua("Mozilla/5.0 (Windows NT 10.0; Win64; x64)"))).toBe("windows");
    expect(detectPlatform(ua("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)"))).toBe("macos");
    expect(detectPlatform(ua("Mozilla/5.0 (X11; Linux x86_64)"))).toBe("linux");
    expect(detectPlatform(ua("Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)"))).toBeNull();
    expect(detectPlatform(ua("Mozilla/5.0 (Linux; Android 14; Pixel 8)"))).toBeNull();
  });

  it("links every platform to a file of the latest release, with its size", () => {
    for (const p of PLATFORMS) {
      const d = downloadFor(p);
      expect(d, p).not.toBeNull();
      expect(d!.url).toContain(`/releases/download/${release.version}/go-link-console-${release.version}-`);
      expect(d!.mb).toBeGreaterThan(0);
    }
    expect(armDownloadFor("windows")?.url).toMatch(/windows-arm64\.zip$/);
    expect(armDownloadFor("macos")).toBeNull();
  });
});
