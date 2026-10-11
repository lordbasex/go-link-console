// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { defineConfig } from "@playwright/test";
import { MAKER_URL, PLAY_URL, PORTS, SITE_URL } from "./ports";

// A real Chromium against a real signalhub, a real headless device and the
// website, all started by global-setup.ts on their own ports, so nothing of
// the developer's (signalhub, device, rooms) is touched. The steps depend on
// each other (pair, then reconnect, then invite...), so they run in order.
//
// Four projects: "web" (npm test), "android" (npm run test:android: the
// go-link Console Player app on an emulator or phone through adb, skipped when no
// device is attached), "android-camera" (npm run test:android:camera: the
// app scans a QR code with the emulator's virtual camera; it restarts the
// emulator, so it is opt-in) and "shots" (npm run shots: the landing page's
// screenshots, with an invented ROM library). E2E_CHROMIUM_SINGLE_PROCESS=1 runs Chromium as one
// process, only for a local session whose macOS launchd context is broken.
const singleProcess = process.env.E2E_CHROMIUM_SINGLE_PROCESS === "1" ? ["--single-process", "--no-zygote"] : [];

// Tests that need no signalhub or device (Willy Maker, the two sites, the
// Android skins lab, which runs offline): when they
// are the only files named on the command line, no signalhub or device is
// built or started.
const WEB_ONLY = /(willy-maker|willy-maker-games|two-sites|android-skins)\.spec\.ts$/;
const named = process.argv.filter((a) => /\.spec\.ts$/.test(a));
const webOnly = named.length > 0 && named.every((a) => WEB_ONLY.test(a));

export default defineConfig({
  testDir: "tests",
  fullyParallel: false,
  workers: 1,
  timeout: 60_000,
  expect: { timeout: 20_000 },
  retries: 0,
  reporter: [["list"], ["html", { open: "never" }]],
  globalSetup: webOnly ? undefined : "./global-setup.ts",
  globalTeardown: webOnly ? undefined : "./global-teardown.ts",
  use: {
    baseURL: `http://localhost:${PORTS.web}`,
    locale: "en-US",
    trace: "retain-on-failure",
    launchOptions: {
      args: [
        "--use-fake-ui-for-media-stream",
        "--use-fake-device-for-media-stream",
        "--autoplay-policy=no-user-gesture-required",
        // Rooms play a 1 kHz tone: never through the developer's speakers.
        "--mute-audio",
        ...singleProcess,
      ],
    },
  },
  projects: [
    { name: "web", testIgnore: /(android|android-camera|android-skins|shots|video)\.spec\.ts$/ },
    // One long story on a real Android device: minutes, not seconds.
    { name: "android", testMatch: /\/android\.spec\.ts$/, timeout: 240_000 },
    // The gamepad skins in the app's debug lab (no room): npm run test:android:skins.
    { name: "android-skins", testMatch: /android-skins\.spec\.ts$/, timeout: 1_200_000 },
    { name: "android-camera", testMatch: /android-camera\.spec\.ts$/, timeout: 300_000 },
    // The landing page's screenshots (npm run shots), never part of npm test.
    { name: "shots", testMatch: /shots\.spec\.ts$/, timeout: 1_800_000 },
    // The takes of the trailer and demo video (npm run video), never part of npm test.
    { name: "video", testMatch: /video\.spec\.ts$/, timeout: 600_000 },
  ],
  webServer: [
    {
      // The website in dev mode (no CSP, so ws:// to the local signalhub works).
      command: `npm --prefix ../frontend run dev -w @go-link/web -- --port ${PORTS.web} --strictPort`,
      url: `http://localhost:${PORTS.web}`,
      reuseExistingServer: false,
      timeout: 120_000,
      env: { VITE_SIGNAL_URL: `ws://127.0.0.1:${PORTS.signal}/ws`, VITE_DEMO_DATA: "false", VITE_MAKER_URL: MAKER_URL },
    },
    {
      // Willy Maker's own site, which reaches the device through the website's /maker-bridge tab.
      command: `npm --prefix ../frontend run dev -w @go-link/willy-maker -- --port ${PORTS.maker} --strictPort`,
      url: MAKER_URL,
      reuseExistingServer: false,
      timeout: 120_000,
      env: { VITE_SITE_URL: `http://localhost:${PORTS.web}` },
    },
    {
      // The website as the landing's own site (console.go-link.org), and as the rooms' (play.go-link.org).
      command: `npm --prefix ../frontend run dev -w @go-link/web -- --port ${PORTS.site} --strictPort`,
      url: SITE_URL,
      reuseExistingServer: false,
      timeout: 120_000,
      env: { VITE_ROLE: "site", VITE_SITE_URL: SITE_URL, VITE_PLAY_URL: PLAY_URL, VITE_SIGNAL_URL: `ws://127.0.0.1:${PORTS.signal}/ws`, VITE_MAKER_URL: MAKER_URL },
    },
    {
      command: `npm --prefix ../frontend run dev -w @go-link/web -- --port ${PORTS.play} --strictPort`,
      url: PLAY_URL,
      reuseExistingServer: false,
      timeout: 120_000,
      env: { VITE_ROLE: "play", VITE_SITE_URL: SITE_URL, VITE_PLAY_URL: PLAY_URL, VITE_SIGNAL_URL: `ws://127.0.0.1:${PORTS.signal}/ws`, VITE_MAKER_URL: MAKER_URL },
    },
  ],
});
