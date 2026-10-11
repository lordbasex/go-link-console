// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { readFileSync } from "node:fs";
import { defineConfig, searchForWorkspaceRoot, type Plugin } from "vite";
import react from "@vitejs/plugin-react";

// Strict Content Security Policy for the production build (console.go-link.org's).
// The device is reached through the main site's tab with postMessage, so
// nothing here connects to it. Not applied in dev: hot reload needs inline scripts.
const CSP = [
  "default-src 'self'",
  "script-src 'self' 'wasm-unsafe-eval'",
  "style-src 'self'",
  "font-src 'self'",
  "img-src 'self' data: blob:",
  "connect-src 'self'",
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
      // The build's version (curl -s https://maker.go-link.org/ | grep go-link-version).
      const version = (process.env.VITE_APP_VERSION || "dev").replace(/[^\w.+-]/g, "");
      return html.replace(
        "<head>",
        `<head>\n    <meta http-equiv="Content-Security-Policy" content="${CSP}" />\n    <meta name="go-link-version" content="${version}" />`,
      );
    },
  };
}

// Chrome origin trial tokens (comma separated, VITE_ORIGIN_TRIAL_TOKENS) for
// built-in AI APIs still in trial, such as the Proofreader: each token is
// registered for one origin. Without any, those APIs stay off and Willy
// Maker falls back or hides what needs them.
function originTrials(): Plugin {
  let list = "";
  return {
    name: "origin-trials",
    configResolved(config) {
      list = String(config.env.VITE_ORIGIN_TRIAL_TOKENS ?? "");
    },
    transformIndexHtml(html) {
      const tokens = list
        .split(",")
        .map((t) => t.trim())
        .filter((t) => /^[A-Za-z0-9+/=]+$/.test(t));
      if (!tokens.length) return html;
      return html.replace("<head>", `<head>\n${tokens.map((t) => `    <meta http-equiv="origin-trial" content="${t}" />`).join("\n")}`);
    },
  };
}

// Play mode draws Willy, the robots and the people with the character atlases
// console.go-link.org keeps for its own game (apps/web/public/destroy). The maker's
// site serves the three it needs at the same path, from that one copy.
const ATLAS_DIR = new URL("../apps/web/public/destroy/", import.meta.url);
const ATLASES = ["player", "robot", "npcs"].flatMap((n) => [`${n}.json`, `${n}.webp`]);

function playAtlases(): Plugin {
  return {
    name: "play-atlases",
    configureServer(server) {
      server.middlewares.use("/destroy", (req, res, next) => {
        const file = (req.url ?? "").replace(/^\//, "").split("?")[0]!;
        if (!ATLASES.includes(file)) return next();
        res.setHeader("Content-Type", file.endsWith(".json") ? "application/json" : "image/webp");
        res.end(readFileSync(new URL(file, ATLAS_DIR)));
      });
    },
    generateBundle() {
      for (const file of ATLASES) this.emitFile({ type: "asset", fileName: `destroy/${file}`, source: readFileSync(new URL(file, ATLAS_DIR)) });
    },
  };
}

export default defineConfig({
  plugins: [react(), contentSecurityPolicy(), originTrials(), playAtlases()],
  server: {
    port: 5181,
    strictPort: true,
    // The AI pack carries the ROM docs (docs/rom) and Willy Maker's own (docs/willy-maker).
    fs: { allow: [searchForWorkspaceRoot(process.cwd()), "../../docs/rom", "../../docs/willy-maker"] },
  },
  build: {
    sourcemap: "hidden",
    // Small fonts would be inlined as data: URLs, which the CSP's font-src 'self' blocks.
    assetsInlineLimit: (file) => (/\.(woff2?|ttf|otf)$/.test(file) ? false : undefined),
  },
});
