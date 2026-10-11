// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useEffect, useRef, useState } from "react";
import { t } from "../../i18n";
import { CloseIcon, GamepadIcon, PlayIcon } from "../Icons";

// The landing's arcade screen: a short silent loop of two players in
// ANTÍDOTO (go-link HD's free starter game), and its trailer with sound in
// a dialog. Under the screen, the two players who control it from far
// apart, joined by a live link. The videos live in public/media (made with go-link HD's
// hdrun from a two-player bot run, see docs/web.md).
const LOOP = "/media/antidoto-loop.mp4";
const POSTER = "/media/antidoto-poster.jpg";
const TRAILER = "/media/antidoto-trailer.mp4";

function reducedMotion(): boolean {
  return typeof window.matchMedia === "function" && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

/** The trailer with sound, over the page; Escape or a click outside closes it. */
function TrailerDialog({ onClose }: { onClose: () => void }) {
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
            {t.landing.trailerTitle}
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
        <video className="lp-trailer-video" src={TRAILER} poster={POSTER} controls autoPlay playsInline preload="auto" />
        <p className="small muted">{t.landing.trailerText}</p>
      </div>
    </div>
  );
}

export function GameScene() {
  const [open, setOpen] = useState(false);
  const loop = useRef<HTMLVideoElement>(null);
  // the loop rests while the trailer plays, and never moves by itself for
  // people who asked for less motion
  useEffect(() => {
    const v = loop.current;
    if (!v) return;
    if (open || reducedMotion()) v.pause();
    else void v.play?.()?.catch(() => undefined);
  }, [open]);
  return (
    <figure className="lp-scene">
      <div className="lp-screen">
        <video
          ref={loop}
          className="lp-video"
          poster={POSTER}
          muted
          loop
          playsInline
          autoPlay={!reducedMotion()}
          preload="metadata"
          aria-label={t.landing.sceneLabel}
          src={LOOP}
        />
        <button type="button" className="lp-trailer-open" onClick={() => setOpen(true)}>
          <span className="lp-trailer-play" aria-hidden="true">
            <PlayIcon size={22} />
          </span>
          <span className="lp-trailer-label">{t.landing.trailerOpen}</span>
        </button>
      </div>

      <figcaption className="lp-players">
        <span className="lp-player">
          <span className="lp-player-badge is-p1">P1</span>
          <GamepadIcon size={18} />
          <span className="lp-voice" aria-hidden="true">
            <i />
            <i />
            <i />
          </span>
          <span className="small muted">{t.landing.p1Place}</span>
        </span>
        <span className="lp-link" aria-hidden="true">
          <span className="lp-link-line" />
          <span className="lp-link-chip">{t.landing.linkChip}</span>
        </span>
        <span className="lp-player">
          <span className="small muted">{t.landing.p2Place}</span>
          <span className="lp-voice is-late" aria-hidden="true">
            <i />
            <i />
            <i />
          </span>
          <GamepadIcon size={18} />
          <span className="lp-player-badge is-p2">P2</span>
        </span>
      </figcaption>
      {open && <TrailerDialog onClose={() => setOpen(false)} />}
    </figure>
  );
}
