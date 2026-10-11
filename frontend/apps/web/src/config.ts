// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

/** The project's public signaling server. */
export const OFFICIAL_SIGNAL_URL = "wss://signal.go-link.org/ws";

/** Default server for this build (VITE_SIGNAL_URL), else the official one. */
export const DEFAULT_SIGNAL_URL = import.meta.env.VITE_SIGNAL_URL || OFFICIAL_SIGNAL_URL;

/** true when the build shows the design's sample data. */
export const DEMO_DATA = import.meta.env.VITE_DEMO_DATA === "true";

/** How often the lobby refreshes the public room list. */
export const LOBBY_REFRESH_MS = 15_000;

/** The project's source repository. */
export const REPO_URL = "https://github.com/lordbasex/go-link-console";

/** This build of the website (git describe, set by `make web-build`), and its date. */
export const APP_VERSION = import.meta.env.VITE_APP_VERSION || "dev";
export const BUILD_DATE = import.meta.env.VITE_BUILD_DATE || "";

/** Willy Maker's own site (VITE_MAKER_URL), which uses this site's link to the device through /maker-bridge. */
export const MAKER_URL = (import.meta.env.VITE_MAKER_URL || (import.meta.env.DEV ? "http://localhost:5181" : "https://maker.go-link.org")).replace(/\/+$/, "");
