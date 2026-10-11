// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import narration from "./demo-narration.json";
import durations from "./demo-narration-durations.json";
import { FPS } from "./timeline";
import type { Lang } from "./timeline";

// The demo's timeline: chapters, each with a title card and its shots of the
// app, every shot with the narrator's line. A shot lasts its planned time or
// longer when its line needs it; the video (Demo.tsx) and the subtitles
// (scripts/srt.ts) both read it, in frames at 30 fps.

const sec = (s: number) => Math.round(s * FPS);

export type LineId = keyof typeof narration.lines.en;

/** When the narrator starts after a cut, and how long the shot stays after the line. */
export const DEMO_LEAD = sec(0.5);
const TAIL = sec(1.5);

/** A piece of a recorded take (seconds into its WebM, e2e: npm run video). */
export interface Shot {
  id: LineId;
  take: string;
  from: number;
  rate?: number;
  phone?: boolean;
  focus?: { x: number; y: number; zoom: number; at: number };
  /** Where the shot starts in another language's take, when it differs. */
  fromBy?: Partial<Record<Lang, number>>;
}

export interface Chapter {
  n: number;
  title: Record<Lang, string>;
  shots: Shot[];
}

/** Where a shot starts in the take of a language. */
export const shotFrom = (shot: Shot, lang: Lang) => shot.fromBy?.[lang] ?? shot.from;

export const CHAPTERS: Chapter[] = [
  {
    n: 1,
    title: { en: "Your arcade, online", es: "Tu arcade, online", pt: "Seu fliperama, online" },
    shots: [
      { id: "hero", take: "landing", from: 3.5 },
      { id: "steps", take: "landing", from: 15 },
    ],
  },
  {
    n: 2,
    title: { en: "Link your computer", es: "Vincula tu computadora", pt: "Conecte seu computador" },
    shots: [
      { id: "code", take: "link", from: 3, rate: 0.6, focus: { x: 0.5, y: 0.3, zoom: 1.25, at: 30 } },
      { id: "dashboard", take: "link", from: 10 },
      { id: "roms", take: "link", from: 21 },
    ],
  },
  {
    n: 3,
    title: { en: "Open a room", es: "Abre una sala", pt: "Abra uma sala" },
    shots: [
      { id: "room", take: "room", from: 10 },
      { id: "controls", take: "room", from: 15, focus: { x: 0.86, y: 0.45, zoom: 1.45, at: 20 } },
      { id: "latency", take: "room", from: 25, focus: { x: 0.55, y: 0.75, zoom: 1.2, at: 15 } },
    ],
  },
  {
    n: 4,
    title: { en: "Invite your friends", es: "Invita a tus amigos", pt: "Convide seus amigos" },
    shots: [
      { id: "invite", take: "room", from: 39, rate: 0.9, focus: { x: 0.45, y: 0.55, zoom: 1.35, at: 25 } },
      { id: "join", take: "guest", from: 6, focus: { x: 0.5, y: 0.4, zoom: 1.4, at: 12 } },
      { id: "play", take: "guest", from: 15 },
    ],
  },
  {
    n: 5,
    title: { en: "Your phone is a console", es: "Tu celular es una consola", pt: "Seu celular é um console" },
    shots: [{ id: "phone", take: "phone", from: 4, phone: true }],
  },
  {
    n: 6,
    title: { en: "Make your own game", es: "Crea tu propio juego", pt: "Crie seu próprio jogo" },
    shots: [
      { id: "wizard", take: "maker", from: 0.5, rate: 0.9 },
      { id: "editor", take: "maker", from: 9 },
      { id: "build", take: "maker", from: 18 },
      { id: "makerRoom", take: "maker-2", from: 6 },
    ],
  },
  {
    n: 7,
    title: { en: "Every millisecond, measured", es: "Cada milisegundo, medido", pt: "Cada milissegundo, medido" },
    shots: [
      { id: "charts", take: "network", from: 3.5 },
      { id: "log", take: "network", from: 20 },
    ],
  },
];

const PLANNED_SHOT = sec(7);
const CARD = sec(2.6);
const INTRO = sec(4);
const END = sec(5);

export type Item =
  | { kind: "intro" | "end"; id: LineId; from: number; len: number; voice: number; line: string }
  | { kind: "card"; chapter: Chapter; from: number; len: number }
  | { kind: "shot"; chapter: Chapter; shot: Shot; id: LineId; from: number; len: number; voice: number; line: string };

/** Every item of the demo in a language, in order, with its frames. */
export function demoItems(lang: Lang = "en"): Item[] {
  const lines = (narration.lines as Record<string, Record<LineId, string>>)[lang] ?? narration.lines.en;
  const d = ((durations as Record<string, Record<LineId, number>>)[lang] ?? durations.en) as Record<LineId, number>;
  const out: Item[] = [];
  let at = 0;
  const spoken = (id: LineId, planned: number) => Math.max(planned, DEMO_LEAD + sec(d[id]) + TAIL);
  const push = (it: Item) => {
    out.push(it);
    at += it.len;
  };
  push({ kind: "intro", id: "intro", from: at, len: spoken("intro", INTRO), voice: sec(d.intro), line: lines.intro });
  for (const chapter of CHAPTERS) {
    push({ kind: "card", chapter, from: at, len: CARD });
    for (const shot of chapter.shots) {
      push({ kind: "shot", chapter, shot, id: shot.id, from: at, len: spoken(shot.id, PLANNED_SHOT), voice: sec(d[shot.id]), line: lines[shot.id] });
    }
  }
  push({ kind: "end", id: "end", from: at, len: spoken("end", END), voice: sec(d.end), line: lines.end });
  return out;
}

export const demoTotal = (lang: Lang = "en") => demoItems(lang).reduce((n, it) => n + it.len, 0);

/** One subtitle, in frames of the whole demo. */
export interface DemoCaption {
  from: number;
  to: number;
  text: string;
}

/** A line cut into phrases at its punctuation (short pieces ride with the next). */
function phrases(line: string): string[] {
  // Split after a stop followed by a space, so web addresses keep their dots.
  const parts = line.split(/(?<=[.:?!])\s+/).map((p) => p.trim()).filter(Boolean);
  const merged: string[] = [];
  for (const p of parts) {
    if (merged.length > 0 && merged[merged.length - 1]!.length < 18) merged[merged.length - 1] += ` ${p}`;
    else merged.push(p);
  }
  return merged;
}

/** The demo's subtitles: each phrase for its share of the line's spoken time. */
export function demoCaptions(lang: Lang = "en"): DemoCaption[] {
  const out: DemoCaption[] = [];
  for (const it of demoItems(lang)) {
    if (it.kind === "card") continue;
    const parts = phrases(it.line);
    const chars = parts.reduce((n, p) => n + p.length, 0);
    let at = it.from + DEMO_LEAD;
    parts.forEach((p, i) => {
      const len = Math.round((it.voice * p.length) / chars);
      const end = i === parts.length - 1 ? it.from + DEMO_LEAD + it.voice + sec(0.25) : at + len;
      out.push({ from: at, to: end, text: p });
      at += len;
    });
  }
  return out;
}

/** The music's volume: low under the narrator, louder on the chapter cards, faded in and out. */
export function demoMusicVolume(lang: Lang, f: number, base = 0.26, duck = 0.11): number {
  let v = base;
  for (const it of demoItems(lang)) {
    if (it.kind === "card") continue;
    const a = it.from + DEMO_LEAD - 6;
    const b = it.from + DEMO_LEAD + it.voice + 6;
    if (f >= a - 10 && f <= b + 10) {
      const t = f < a ? (a - f) / 10 : f > b ? (f - b) / 10 : 0;
      v = Math.min(v, duck + (base - duck) * Math.min(1, t));
    }
  }
  const end = demoTotal(lang);
  return Math.max(0, v * Math.min(1, f / sec(1.5)) * Math.min(1, (end - f) / sec(3)));
}
