// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { t } from "../i18n";
import { androidDownload } from "../downloads";

/** Where the Android app is published when this release has no APK: the latest GitHub release. */
export const ANDROID_APP_URL = "https://github.com/lordbasex/go-link-console/releases/latest";

/** The APK of this release (a direct download), or the latest release page. */
export function androidAppUrl(): string {
  return androidDownload()?.url ?? ANDROID_APP_URL;
}

/** An Android phone or tablet, from its user agent. */
export function isAndroid(userAgent: string = typeof navigator === "undefined" ? "" : navigator.userAgent): boolean {
  return /\bAndroid\b/i.test(userAgent);
}

/**
 * A small card on an invitation page opened on Android: the go-link Player
 * app, with the browser as the other way to play. With the app installed,
 * the same link opens the app (Android App Links); the PIN is always typed
 * there, never passed along.
 */
export function AndroidAppCard({ userAgent }: { userAgent?: string }) {
  if (!isAndroid(userAgent)) return null;
  return (
    <aside className="android-app-card stack-xs" aria-label={t.androidApp.title}>
      <strong className="small-plus">{t.androidApp.title}</strong>
      <p className="small muted">{t.androidApp.text}</p>
      <a className="button button-secondary button-block" href={androidAppUrl()} target="_blank" rel="noopener noreferrer">
        {t.androidApp.get}
      </a>
      <p className="small faint">{t.androidApp.browser}</p>
    </aside>
  );
}
