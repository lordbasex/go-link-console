// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
// Fonts are served by the site itself (never a third-party font service,
// which would receive every visitor's IP address).
import "@fontsource/chakra-petch/600.css";
import "@fontsource/chakra-petch/700.css";
import "@fontsource/ibm-plex-sans/400.css";
import "@fontsource/ibm-plex-sans/500.css";
import "@fontsource/ibm-plex-sans/600.css";
import "@fontsource/jetbrains-mono/500.css";
import "@go-link/shared/tokens.css";
import "./styles/global.css";
import { ANALYTICS_SITE, guardHistory } from "./analytics";
import { App } from "./App";
import { DEFAULT_SIGNAL_URL, DEMO_DATA } from "./config";
import { SignalProvider } from "./signal/SignalProvider";
import { migrateLegacyStorage } from "@go-link/shared";
import { useLang } from "./i18n";
import { panelSocketUrl } from "./panel";
import { installIntro } from "./intro";
import { ROLE } from "./role";

/** The device's socket when this page is its local web panel. */
const PANEL_URL = panelSocketUrl();

// Settings saved when the project was called MAME WebRTC.
try {
  migrateLegacyStorage(window.localStorage);
} catch {
  // storage blocked
}

// The startup intro's coin sound and tap to skip.
installIntro();

// Before anything navigates and before Google's script could load (only
// console.go-link.org asks for it, see analytics.ts).
if (ANALYTICS_SITE) guardHistory();

/** Re-renders the whole tree when the language changes. Nothing remounts,
 * so connections and a running game survive the switch. */
function LangRoot() {
  useLang();
  return (
    <BrowserRouter>
      <SignalProvider defaultUrl={DEFAULT_SIGNAL_URL} demo={DEMO_DATA} panelUrl={PANEL_URL} offline={ROLE === "site"}>
        <App />
      </SignalProvider>
    </BrowserRouter>
  );
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <LangRoot />
  </StrictMode>,
);
