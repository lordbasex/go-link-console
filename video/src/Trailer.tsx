// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { AbsoluteFill, Audio, interpolate, Sequence, spring, staticFile, useCurrentFrame, useVideoConfig } from "remotion";
import { Backdrop, Chip, inAt, KineticTitle, Mark, outAt, Phone, Screen, Sub, Wordmark } from "./components";
import { T } from "./takes";
import { COPY, PLATFORMS } from "./copy";
import { captions, musicVolume, scenes, VOICE_LEAD, type Lang, type SceneId } from "./timeline";
import { C, D, EASE_IN, F, sec } from "./theme";


/** A title block on one side and a take on the other, swapping sides scene by scene. */
function Split({ len, title, sub, chips = [], side = "left", children }: { len: number; title: string; sub?: string; chips?: { text: string; color?: string }[]; side?: "left" | "right"; children: React.ReactNode }) {
  const frame = useCurrentFrame();
  const exit = outAt(frame, len, D.quick);
  const words = (
    <div style={{ width: 560, display: "flex", flexDirection: "column", gap: 26, opacity: exit, transform: `translateY(${(1 - exit) * -24}px)` }}>
      <KineticTitle text={title} start={4} size={76} />
      {sub && <Sub start={14}>{sub}</Sub>}
      {chips.length > 0 && (
        <div style={{ display: "flex", flexWrap: "wrap", gap: 12, marginTop: 6 }}>
          {chips.map((c, i) => (
            <Chip key={c.text} color={c.color} start={20 + i * 3}>
              {c.text}
            </Chip>
          ))}
        </div>
      )}
    </div>
  );
  return (
    <AbsoluteFill style={{ flexDirection: side === "left" ? "row" : "row-reverse", alignItems: "center", justifyContent: "center", gap: 70, padding: "0 90px" }}>
      {words}
      <div style={{ flexShrink: 0 }}>{children}</div>
    </AbsoluteFill>
  );
}

export function Intro({ len, tagline }: { len: number; tagline: string }) {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  // Anticipation: the mark dips, then pops past full size and settles.
  const pop = spring({ frame: frame - 6, fps, config: { damping: 9, stiffness: 160, mass: 0.8 } });
  const dip = interpolate(frame, [0, 6], [1, 0.85], { extrapolateRight: "clamp" });
  const word = inAt(frame, 16, D.reveal);
  const exit = outAt(frame, len, D.standard);
  return (
    <AbsoluteFill style={{ alignItems: "center", justifyContent: "center", gap: 36, opacity: exit, transform: `scale(${0.96 + 0.04 * exit})` }}>
      <div style={{ transform: `scale(${frame < 6 ? dip : pop}) rotate(${(1 - pop) * -12}deg)`, filter: `drop-shadow(0 20px 50px ${C.accent}55)` }}>
        <Mark size={180} />
      </div>
      <div style={{ opacity: word, transform: `translateY(${(1 - word) * 30}px)` }}>
        <Wordmark size={110} />
      </div>
      <Sub start={30} style={{ fontSize: 38 }}>
        {tagline}
      </Sub>
    </AbsoluteFill>
  );
}

export function End({ len, title, free }: { len: number; title: string; free: string }) {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const pop = spring({ frame: frame - 2, fps, config: { damping: 11, stiffness: 150 } });
  const pulse = 1 + Math.sin((frame / fps) * Math.PI * 1.2) * 0.015;
  return (
    <AbsoluteFill style={{ alignItems: "center", justifyContent: "center", gap: 30, opacity: outAt(frame, len, D.reveal) }}>
      <div style={{ display: "flex", alignItems: "center", gap: 30, transform: `scale(${(0.85 + 0.15 * pop) * pulse})` }}>
        <Mark size={130} />
        <Wordmark size={120} />
      </div>
      <KineticTitle text={title} start={12} size={58} align="center" />
      <div style={{ display: "flex", gap: 16, marginTop: 10 }}>
        <Chip start={26}>console.go-link.org</Chip>
        <Chip start={30} color={C.ok}>{free}</Chip>
        <Chip start={34} color={C.voice}>{PLATFORMS}</Chip>
      </div>
      <Sub start={40} style={{ fontFamily: F.mono, fontSize: 26, color: C.faint }}>
        github.com/lordbasex/go-link-console
      </Sub>
    </AbsoluteFill>
  );
}

/** The cut between scenes: a band of the accent color sweeps across. */
export function Wipe({ at: start }: { at: number }) {
  const frame = useCurrentFrame();
  const x = interpolate(frame, [start - 5, start + 5], [-30, 130], { extrapolateLeft: "clamp", extrapolateRight: "clamp", easing: EASE_IN });
  if (frame < start - 5 || frame > start + 5) return null;
  return <AbsoluteFill style={{ background: `linear-gradient(100deg, transparent ${x - 12}%, ${C.accent}22 ${x - 4}%, ${C.accent}55 ${x}%, transparent ${x + 3}%)` }} />;
}

/** The music: "System Awakening", made by the author with ElevenLabs, ducked under the narrator. */
export const MUSIC = { file: "music/system-awakening.wav", from: 0 };

/**
 * Burned-in subtitles: one phrase at a time, low in the frame on a dark
 * capsule, rising in as the narrator says it.
 */
function Captions({ voice, subs }: { voice: Lang; subs: Lang }) {
  const frame = useCurrentFrame();
  const c = captions(voice, subs).find((x) => frame >= x.from && frame < x.to);
  if (!c) return null;
  const v = Math.min(1, (frame - c.from) / 5, (c.to - frame) / 4);
  return (
    <AbsoluteFill style={{ justifyContent: "flex-end", alignItems: "center", paddingBottom: 54 }}>
      <div
        style={{
          maxWidth: 1300,
          padding: "12px 28px",
          borderRadius: 16,
          background: "rgba(8, 9, 13, 0.78)",
          color: C.text,
          fontFamily: F.text,
          fontWeight: 600,
          fontSize: 38,
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

/**
 * The trailer narrated in voice (its words on screen, its narrator and its
 * recordings of the app), with subtitles in subs.
 */
export function Trailer({ voice = "en", subs = voice }: { voice?: Lang; subs?: Lang }) {
  const list = scenes(voice);
  const s = (id: SceneId) => list.find((x) => x.id === id)!;
  const t = COPY[voice];
  const take = (name: string) => `${voice}/${name}`;
  return (
    <AbsoluteFill style={{ background: C.bg }}>
      <Backdrop />
      <Sequence from={s("intro").from} durationInFrames={s("intro").len}>
        <Intro len={s("intro").len} tagline={t.tagline} />
      </Sequence>
      <Sequence from={s("home").from} durationInFrames={s("home").len}>
        <Split len={s("home").len} title={t.home.title} sub={t.home.sub} side="left">
          <Screen take={take("landing")} from={T.landing.hero} url="console.go-link.org" duration={s("home").len} />
        </Split>
      </Sequence>
      <Sequence from={s("friends").from} durationInFrames={s("friends").len}>
        <Split len={s("friends").len} title={t.friends.title} sub={t.friends.sub} side="right">
          <Screen take={take("guest")} from={T.guest.join} rate={1.5} duration={s("friends").len} />
        </Split>
      </Sequence>
      <Sequence from={s("invite").from} durationInFrames={s("invite").len}>
        <Split len={s("invite").len} title={t.invite.title} chips={[{ text: t.invite.chips[0] }, { text: t.invite.chips[1], color: C.ok }]} side="left">
          <Screen take={take("room")} from={T.room.invite} duration={s("invite").len} focus={{ x: 0.5, y: 0.45, zoom: 1.18, at: 20 }} />
        </Split>
      </Sequence>
      <Sequence from={s("controls").from} durationInFrames={s("controls").len}>
        <Split len={s("controls").len} title={t.controls.title} chips={[{ text: t.controls.chips[0] }, { text: t.controls.chips[1], color: C.voice }, { text: t.controls.chips[2], color: C.p4 }]} side="right">
          <Screen take={take("room")} from={T.room.controls} duration={s("controls").len} focus={{ x: 0.82, y: 0.5, zoom: 1.25, at: 30 }} />
        </Split>
      </Sequence>
      <Sequence from={s("phone").from} durationInFrames={s("phone").len}>
        <Split len={s("phone").len} title={t.phone.title} sub={t.phone.sub} side="left">
          <Phone take={take("phone")} from={T.phone.play} duration={s("phone").len} height={820} />
        </Split>
      </Sequence>
      <Sequence from={s("maker").from} durationInFrames={s("maker").len}>
        <Split len={s("maker").len} title={t.maker.title} sub={t.maker.sub} side="right">
          <Screen
            duration={s("maker").len}
            clips={[
              { take: take("maker"), from: T.maker.wizard, rate: 2.4, len: sec(2.6), url: "maker.go-link.org" },
              { take: take("maker-2"), from: T.makerRoom.play, len: s("maker").len - sec(2.6), url: "play.go-link.org" },
            ]}
          />
        </Split>
      </Sequence>
      <Sequence from={s("metrics").from} durationInFrames={s("metrics").len}>
        <Split len={s("metrics").len} title={t.metrics.title} chips={[{ text: t.metrics.chips[0], color: C.ok }, { text: t.metrics.chips[1], color: C.p3 }, { text: t.metrics.chips[2] }]} side="left">
          <Screen
            duration={s("metrics").len}
            clips={[
              { take: take("room"), from: T.room.latency, len: sec(2.4) },
              { take: take("network"), from: T.network.charts, len: s("metrics").len - sec(2.4) },
            ]}
          />
        </Split>
      </Sequence>
      <Sequence from={s("end").from} durationInFrames={s("end").len}>
        <End len={s("end").len} title={t.end.title} free={t.end.free} />
      </Sequence>
      {list.slice(1).map((sc) => (
        <Wipe key={sc.id} at={sc.from} />
      ))}
      {list.map((sc) => (
        <Sequence key={`voice-${sc.id}`} from={sc.from + VOICE_LEAD} durationInFrames={sc.voice + 15}>
          <Audio src={staticFile(`voice/${voice}/${sc.id}.wav`)} volume={1} />
        </Sequence>
      ))}
      <Audio src={staticFile(MUSIC.file)} startFrom={MUSIC.from} volume={(f) => musicVolume(voice, f)} />
      <Captions voice={voice} subs={subs} />
    </AbsoluteFill>
  );
}


