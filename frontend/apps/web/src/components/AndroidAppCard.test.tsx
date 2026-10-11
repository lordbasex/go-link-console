// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { ANDROID_APP_URL, AndroidAppCard, androidAppUrl, isAndroid } from "./AndroidAppCard";

const ANDROID = "Mozilla/5.0 (Linux; Android 15; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Mobile Safari/537.36";
const IPHONE = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1";

describe("AndroidAppCard", () => {
  it("tells Android from other systems", () => {
    expect(isAndroid(ANDROID)).toBe(true);
    expect(isAndroid(IPHONE)).toBe(false);
    expect(isAndroid("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")).toBe(false);
  });

  it("links to the latest release on Android only", () => {
    const { container, rerender } = render(<AndroidAppCard userAgent={IPHONE} />);
    expect(container.innerHTML).toBe("");
    rerender(<AndroidAppCard userAgent={ANDROID} />);
    expect(screen.getByRole("link").getAttribute("href")).toBe(androidAppUrl());
  });

  it("downloads the release's APK, or opens the latest release without one", () => {
    const url = androidAppUrl();
    expect(url === ANDROID_APP_URL || /\/releases\/download\/v[\d.]+\/go-link-console-v[\d.]+-android\.apk$/.test(url)).toBe(true);
  });
});
