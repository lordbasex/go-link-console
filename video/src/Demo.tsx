// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { AbsoluteFill, Audio, interpolate, Sequence, staticFile, useCurrentFrame } from "remotion";
import { Backdrop, inAt, KineticTitle, outAt, Phone, Screen } from "./components";
import { COPY } from "./copy";
import { CHAPTERS, DEMO_LEAD, demoCaptions, demoItems, demoMusicVolume, shotFrom, type Chapter, type Item } from "./demoTimeline";
import { End, Intro, MUSIC, Wipe } from "./Trailer";
import type { Lang } from "./timeline";
import { C, D, EASE_IN, F } from "./theme";

// The demo: go-link from start to end in seven chapters, each a title card
// and the real app (the takes e2e records), with the narrator and burned-in
// subtitles. The trailer sells; the demo shows how it works.

/** The trailer's music ("System Awakening", by the author), looped and quiet under the narrator. */
const DEMO_MUSIC = MUSIC.file;

/** A chapter's title card: its number counts up and the title slides in. */
function ChapterCard({ chapter, len, lang }: { chapter: Chapter; len: number; lang: Lang }) {
  const frame = useCurrentFrame();
  const num = inAt(frame, 2, D.standard);
  const bar = interpolate(frame, [6, 6 + D.reveal], [0, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp", easing: EASE_IN });
  const exit = outAt(frame, len, D.standard);
  return (
    <AbsoluteFill style={{ alignItems: "center", justifyContent: "center", opacity: exit, transform: `scale(${0.97 + 0.03 * exit})` }}>
      <div style={{ display: "flex", flexDirection: "column", alignItems: "flex-start", gap: 22, width: 1200 }}>
        <div style={{ display: "flex", alignItems: "baseline", gap: 18, opacity: num, transform: `translateX(${(1 - num) * -40}px)` }}>
          <span style={{ fontFamily: F.mono, fontSize: 30, color: C.accent, letterSpacing: 4 }}>CHAPTER</span>
          <span style={{ fontFamily: F.display, fontWeight: 700, fontSize: 96, color: C.accent, lineHeight: 1 }}>{String(chapter.n).padStart(2, "0")}</span>
          <span style={{ fontFamily: F.mono, fontSize: 30, color: C.faint }}>/ {String(CHAPTERS.length).padStart(2, "0")}</span>
        </div>
        <div style={{ height: 4, width: 1200 * bar, background: `linear-gradient(90deg, ${C.accent}, ${C.accent}00)`, borderRadius: 4 }} />
        <KineticTitle text={chapter.title[lang]} start={10} size={92} />
      </div>
    </AbsoluteFill>
  );
}

/** The chapter's name, small in the corner while its shots play. */
function ChapterTag({ chapter, lang }: { chapter: Chapter; lang: Lang }) {
  const frame = useCurrentFrame();
  const v = inAt(frame, 4, D.standard);
  return (
    <div
      style={{
        position: "absolute",
        top: 22,
        left: 240,
        display: "flex",
        alignItems: "center",
        gap: 12,
        padding: "8px 18px",
        borderRadius: 999,
        background: "rgba(8, 9, 13, 0.7)",
        border: `1px solid ${C.border}`,
        fontFamily: F.text,
        fontWeight: 600,
        fontSize: 22,
        color: C.text,
        opacity: v,
      }}
    >
      <span style={{ fontFamily: F.mono, color: C.accent }}>{String(chapter.n).padStart(2, "0")}</span>
      {chapter.title[lang]}
    </div>
  );
}

/** One shot: the take large in its browser window (or a phone), with the chapter's tag. */
function ShotView({ item, lang }: { item: Extract<Item, { kind: "shot" }>; lang: Lang }) {
  const { shot, len, chapter } = item;
  // The app in the demo's language, recorded with VIDEO_LANG.
  const take = `${lang}/${shot.take}`;
  const from = shotFrom(shot, lang);
  return (
    <AbsoluteFill style={{ alignItems: "center", justifyContent: "flex-start", paddingTop: 84 }}>
      {shot.phone ? (
        <div style={{ marginTop: 10 }}>
          <Phone take={take} from={from} rate={shot.rate} duration={len} height={820} />
        </div>
      ) : (
        <Screen take={take} from={from} rate={shot.rate} duration={len} width={1440} focus={shot.focus} url={shot.take.startsWith("maker") && shot.take !== "maker-2" ? "maker.go-link.org" : shot.take === "landing" ? "go-link.org" : "play.go-link.org"} />
      )}
      <ChapterTag chapter={chapter} lang={lang} />
    </AbsoluteFill>
  );
}

/** Burned-in subtitles, one phrase at a time, over the bottom of the picture. */
function DemoCaptions({ lang }: { lang: Lang }) {
  const frame = useCurrentFrame();
  const c = demoCaptions(lang).find((x) => frame >= x.from && frame < x.to);
  if (!c) return null;
  const v = Math.min(1, (frame - c.from) / 5, (c.to - frame) / 4);
  return (
    <AbsoluteFill style={{ justifyContent: "flex-end", alignItems: "center", paddingBottom: 40 }}>
      <div
        style={{
          maxWidth: 1400,
          padding: "12px 28px",
          borderRadius: 16,
          background: "rgba(8, 9, 13, 0.82)",
          color: C.text,
          fontFamily: F.text,
          fontWeight: 600,
          fontSize: 36,
          lineHeight: 1.25,
          textAlign: "center",
          opacity: v,
          transform: `translateY(${(1 - v) * 10}px)`,
        }}
      >
        {c.text}
      </div>
    </AbsoluteFill>
  );
}

export function Demo({ lang = "en" }: { lang?: Lang }) {
  const items = demoItems(lang);
  const t = COPY[lang];
  return (
    <AbsoluteFill style={{ background: C.bg }}>
      <Backdrop />
      {items.map((it, i) => (
        <Sequence key={i} from={it.from} durationInFrames={it.len}>
          {it.kind === "intro" && <Intro len={it.len} tagline={t.tagline} />}
          {it.kind === "card" && <ChapterCard chapter={it.chapter} len={it.len} lang={lang} />}
          {it.kind === "shot" && <ShotView item={it} lang={lang} />}
          {it.kind === "end" && <End len={it.len} title={t.end.title} free={t.end.free} />}
        </Sequence>
      ))}
      {items
        .filter((it) => it.kind !== "shot")
        .slice(1)
        .map((it) => (
          <Wipe key={`w${it.from}`} at={it.from} />
        ))}
      {items.map((it) =>
        it.kind === "card" ? null : (
          <Sequence key={`v${it.from}`} from={it.from + DEMO_LEAD} durationInFrames={it.voice + 15}>
            <Audio src={staticFile(`voice/demo/${lang}/${it.id}.wav`)} volume={1} />
          </Sequence>
        ),
      )}
      <Audio src={staticFile(DEMO_MUSIC)} loop volume={(f) => demoMusicVolume(lang, f)} />
      <DemoCaptions lang={lang} />
    </AbsoluteFill>
  );
}
