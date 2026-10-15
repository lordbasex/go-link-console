// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useEffect, useRef, useState } from "react";
import { t, useLang, type Lang } from "../../i18n";
import { CloseIcon, PlayIcon } from "../Icons";
import { Shot } from "./Shot";

// The landing's arcade screen: a short silent loop of one of go-link
// Console's free games (ANTÍDOTO, two players together, or TELETRABAJO:
// CÁMARA APAGADA, a day working from home), picked with the capsules over the
// screen, and its trailer with sound in a dialog. The screen sits in a
// laptop, with the Player app on a phone in front of it, and tags for the two
// players who control it from far apart. The videos live in public/media
// (made with go-link HD from bot runs, see docs/web.md); TELETRABAJO's speak
// and show the page's language.
type GameId = "antidoto" | "remote";

interface SceneGame {
  name: string;
  loop: string;
  poster: string;
  trailer: string;
  label: string;
  title: string;
  text: string;
}

function sceneGame(id: GameId, lang: Lang): SceneGame {
  if (id === "remote")
    return {
      name: "TELETRABAJO",
      loop: `/media/teletrabajo-loop-${lang}.mp4`,
      poster: `/media/teletrabajo-poster-${lang}.jpg`,
      trailer: `/media/teletrabajo-trailer-${lang}.mp4`,
      label: t.landing.remoteSceneLabel,
      title: t.landing.remoteTrailerTitle,
      text: t.landing.remoteTrailerText,
    };
  return {
    name: "ANTÍDOTO",
    loop: "/media/antidoto-loop.mp4",
    poster: "/media/antidoto-poster.jpg",
    trailer: "/media/antidoto-trailer.mp4",
    label: t.landing.sceneLabel,
    title: t.landing.trailerTitle,
    text: t.landing.trailerText,
  };
}

const GAMES: readonly GameId[] = ["antidoto", "remote"];

function reducedMotion(): boolean {
  return typeof window.matchMedia === "function" && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

/** The trailer with sound, over the page; Escape or a click outside closes it. */
function TrailerDialog({ game, onClose }: { game: SceneGame; onClose: () => void }) {
  const close = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    close.current?.focus();
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);
  return (
    <div className="dialog-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="dialog lp-trailer" role="dialog" aria-modal="true" aria-labelledby="lp-trailer-title">
        <div className="lp-trailer-head">
          <h2 id="lp-trailer-title" className="dialog-title">
            {game.title}
          </h2>
          <button
            ref={close}
            type="button"
            className="icon-button"
            aria-label={t.landing.trailerClose}
            data-tip={t.landing.trailerClose}
            onClick={onClose}
          >
            <CloseIcon />
          </button>
        </div>
        <video className="lp-trailer-video" src={game.trailer} poster={game.poster} controls autoPlay playsInline preload="auto" />
        <p className="small muted">{game.text}</p>
      </div>
    </div>
  );
}

export function GameScene() {
  const lang = useLang();
  const [id, setId] = useState<GameId>("antidoto");
  const [open, setOpen] = useState(false);
  const game = sceneGame(id, lang);
  const loop = useRef<HTMLVideoElement>(null);
  // the loop rests while the trailer plays, and never moves by itself for
  // people who asked for less motion
  useEffect(() => {
    const v = loop.current;
    if (!v) return;
    if (open || reducedMotion()) v.pause();
    else void v.play?.()?.catch(() => undefined);
  }, [open, game.loop]);
  return (
    <figure className="ld-stage">
      <div className="ld-laptop">
        <div className="ld-bezel">
          <div className="lp-screen">
            <video
              ref={loop}
              className="lp-video"
              key={game.loop}
              poster={game.poster}
              muted
              loop
              playsInline
              autoPlay={!reducedMotion()}
              preload="metadata"
              aria-label={game.label}
              src={game.loop}
            />
            <div className="lp-games" role="group" aria-label={t.landing.gamesLabel}>
              {GAMES.map((g) => (
                <button key={g} type="button" aria-pressed={g === id} className={g === id ? "is-on" : undefined} onClick={() => setId(g)}>
                  {sceneGame(g, lang).name}
                </button>
              ))}
            </div>
            <button type="button" className="lp-trailer-open" onClick={() => setOpen(true)}>
              <span className="lp-trailer-play" aria-hidden="true">
                <PlayIcon size={22} />
              </span>
              <span className="lp-trailer-label">{t.landing.trailerOpen}</span>
            </button>
          </div>
        </div>
        <div className="ld-base" aria-hidden="true" />
      </div>
      <div className="ld-phone ld-phone-hero">
        <Shot name="app-room" alt={t.playerApp.roomAlt} eager />
      </div>
      <figcaption className="ld-tags">
        <span className="ld-tag ld-tag-a">
          <b className="is-p1">P1</b> · {t.landing.p1Place}
        </span>
        <span className="ld-tag ld-tag-b">
          <b className="is-p2">P2</b> · {t.landing.p2Place}
        </span>
        <span className="ld-tag ld-tag-c">{t.landing.linkChip}</span>
      </figcaption>
      {open && <TrailerDialog game={game} onClose={() => setOpen(false)} />}
    </figure>
  );
}
