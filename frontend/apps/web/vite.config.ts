// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { rmSync } from "node:fs";
import { resolve } from "node:path";
import { defineConfig, searchForWorkspaceRoot, type Plugin } from "vite";
import react from "@vitejs/plugin-react";

// Which site this build is (src/role.ts): "site" (console.go-link.org: the landing,
// guide and tools), "play" (play.go-link.org and the device's local panel:
// rooms and My device), or both on one origin (development and tests).
const ROLE = process.env.VITE_ROLE === "site" || process.env.VITE_ROLE === "play" ? process.env.VITE_ROLE : "all";
const OUT_DIR = ROLE === "all" ? "dist" : `dist-${ROLE}`;

// Files in public/ that only the other site uses.
const NOT_HERE: Record<string, string[]> = {
  // My device's in-browser MP4 converter, and the apps' invitation link
  // files (invitations are play.go-link.org/g/...)
  site: ["mp4", ".well-known"],
  // the landing's screenshots and the Destroy game's sprites
  play: ["shots", "destroy"],
  all: [],
};

function otherSitesFiles(): Plugin {
  return {
    name: "other-sites-files",
    apply: "build",
    closeBundle() {
      for (const dir of NOT_HERE[ROLE] ?? []) rmSync(resolve(import.meta.dirname, OUT_DIR, dir), { recursive: true, force: true });
    },
  };
}

// Strict Content Security Policy for the production build. connect-src
// allows any wss: host because users may pick their own signaling server.
// The landing's site (console.go-link.org) also allows Google Analytics'
// hosts, used only with the visitor's consent on its public pages
// (src/analytics.ts); the rooms' site and the device's panel never do.
// It is not applied in dev: Vite's hot reload needs inline scripts.
const GA = ROLE === "site" ? " https://*.googletagmanager.com" : "";
const GA_HITS = ROLE === "site" ? " https://*.google-analytics.com https://*.analytics.google.com https://*.googletagmanager.com" : "";
const CSP = [
  "default-src 'self'",
  `script-src 'self' 'wasm-unsafe-eval'${GA}`,
  "style-src 'self'",
  "font-src 'self'",
  `img-src 'self' data: blob:${GA_HITS}`,
  `connect-src 'self' wss:${GA_HITS}`,
  "media-src 'self' blob:",
  "base-uri 'none'",
  "form-action 'self'",
  "object-src 'none'",
].join("; ");

function contentSecurityPolicy(): Plugin {
  return {
    name: "content-security-policy",
    apply: "build",
    transformIndexHtml(html) {
      // The build's version, to check which one a server is serving
      // (curl -s https://console.go-link.org/ | grep go-link-version).
      const version = (process.env.VITE_APP_VERSION || "dev").replace(/[^\w.+-]/g, "");
      // The rooms' site is an app, not pages to find in a search engine.
      const robots = ROLE === "play" ? `\n    <meta name="robots" content="noindex" />` : "";
      return html.replace(
        "<head>",
        `<head>\n    <meta http-equiv="Content-Security-Policy" content="${CSP}" />\n    <meta name="go-link-version" content="${version}" />${robots}`,
      );
    },
  };
}

export default defineConfig({
  plugins: [react(), contentSecurityPolicy(), otherSitesFiles()],
  server: {
    port: 5180,
    strictPort: true,
    // The skin editor reads the apps' built-in skins from docs/skins/builtin.
    fs: { allow: [searchForWorkspaceRoot(process.cwd()), "../../../docs/skins/builtin", "../../../docs/skins/screens.json"] },
  },
  // Source maps stay off the published files (the deploy also deletes them).
  build: {
    outDir: OUT_DIR,
    sourcemap: "hidden",
    // Small fonts would be inlined as data: URLs, which the CSP's
    // font-src 'self' blocks: they stay files.
    assetsInlineLimit: (file) => (/\.(woff2?|ttf|otf)$/.test(file) ? false : undefined),
  },
});
