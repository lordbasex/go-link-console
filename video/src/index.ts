// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { Composition, registerRoot } from "remotion";
import { createElement, Fragment } from "react";
import { Trailer } from "./Trailer";
import { Demo } from "./Demo";
import { demoTotal } from "./demoTimeline";
import { FPS } from "./theme";
import { total, type Lang } from "./timeline";

// One trailer per language (its words, narrator, recordings and
// subtitles), plus the English one with Spanish or Portuguese subtitles.
const VERSIONS: { id: string; voice: Lang; subs: Lang }[] = [
  { id: "Trailer", voice: "en", subs: "en" },
  { id: "TrailerES", voice: "es", subs: "es" },
  { id: "TrailerPT", voice: "pt", subs: "pt" },
  { id: "TrailerEN-subsES", voice: "en", subs: "es" },
  { id: "TrailerEN-subsPT", voice: "en", subs: "pt" },
];

function Root() {
  return createElement(
    Fragment,
    null,
    ...VERSIONS.map((v) =>
      createElement(Composition, { key: v.id, id: v.id, component: Trailer, durationInFrames: total(v.voice), fps: FPS, width: 1920, height: 1080, defaultProps: { voice: v.voice, subs: v.subs } }),
    ),
    // The demo in each language: its narrator, subtitles and takes.
    ...(["en", "es", "pt"] as const).map((lang) =>
      createElement(Composition, { key: `Demo-${lang}`, id: lang === "en" ? "Demo" : `Demo${lang.toUpperCase()}`, component: Demo, durationInFrames: demoTotal(lang), fps: FPS, width: 1920, height: 1080, defaultProps: { lang } }),
    ),
  );
}

registerRoot(Root);
