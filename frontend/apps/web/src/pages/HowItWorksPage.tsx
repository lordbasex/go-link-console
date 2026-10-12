// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { Wizard } from "../components/landing/Wizard";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { Link } from "react-router-dom";
import { t } from "../i18n";
import { JoinForm } from "../components/JoinForm";
import { PLAY_URL, ROLE } from "../role";
import { GameScene } from "../components/landing/GameScene";
import { ControllersShowcase } from "../components/landing/ControllersShowcase";
import { GuestPath } from "../components/landing/GuestPath";
import { PlayerApp } from "../components/landing/PlayerApp";
import { SiteFooter } from "../components/legal/SiteFooter";
import { Shot } from "../components/landing/Shot";
import { CloseIcon, GamepadIcon, MonitorIcon } from "../components/Icons";
import "../styles/landing.css";

/** The features each row of pictures tells, by their place in t.landing.features. */
const ROOM_FEATURES = [0, 1, 2, 3, 7];
const DEVICE_FEATURES = [4, 5, 6];

/** A section's heading: the eyebrow, the title and an optional lead. */
function Head({ id, eyebrow, title, text }: { id: string; eyebrow: string; title: string; text?: ReactNode }) {
  return (
    <header className="ld-head">
      <p className="ld-eyebrow">{eyebrow}</p>
      <h2 id={id}>{title}</h2>
      {text && <p>{text}</p>}
    </header>
  );
}

/** The features of a row, each with its title in bold. */
function Bullets({ items }: { items: number[] }) {
  return (
    <ul className="ld-bullets">
      {items.map((i) => {
        const f = t.landing.features[i]!;
        return (
          <li key={f.title}>
            <span>
              <b>{f.title}.</b> {f.text}
            </span>
          </li>
        );
      })}
    </ul>
  );
}

/**
 * "How it works": the landing page, in the look of go-link Desk's. What
 * go-link is (the hero's arcade screen in a laptop, the Player app on a
 * phone), the guest's quick way into a game someone invited them to (right
 * under the hero: most visitors arrive with an invitation), the four steps
 * to host one, the room and the device in real screenshots, the Player app
 * with a gallery, the controllers, how the game travels, and where the
 * games come from (no game art, no game names: only that it plays MAME
 * 2003-Plus sets you have the right to use).
 */
export function HowItWorksPage() {
  const [joinOpen, setJoinOpen] = useState(false);
  // The PIN travels in the page's history state, which stays on this
  // origin: the landing's own site sends guests to the join page of the
  // rooms' site instead of asking here.
  const join = () => (ROLE === "site" ? window.location.assign(`${PLAY_URL}/g`) : setJoinOpen(true));
  const l = t.landing;
  const actions = (centered?: boolean) => (
    <div className={`ld-actions${centered ? " is-centered" : ""}`}>
      <button type="button" className="ld-btn ld-btn-primary" onClick={join}>
        <GamepadIcon size={18} />
        {l.joinCta}
      </button>
      <Link to="/device" className="ld-btn ld-btn-ghost">
        <MonitorIcon size={18} />
        {l.hostCta}
      </Link>
    </div>
  );
  return (
    <div className="page lp ld">
      <section className="ld-hero">
        <div className="ld-hero-text">
          <p className="ld-eyebrow">{l.eyebrow}</p>
          <h1 className="ld-title">
            {l.titleStart} <em>{l.titleAccent}</em>
          </h1>
          <p className="ld-lead">{l.lead}</p>
          {actions()}
          <ul className="ld-notes">
            {l.notes.map((n) => (
              <li key={n}>{n}</li>
            ))}
          </ul>
          <p className="ld-platforms">{l.platforms}</p>
        </div>
        <GameScene />
      </section>

      <GuestPath onJoin={join} />

      <section className="ld-section" id="how" aria-labelledby="lp-steps">
        <Head id="lp-steps" eyebrow={t.wizard.eyebrow} title={t.wizard.title} />
        <Wizard />
      </section>

      <section className="ld-section" aria-labelledby="lp-features">
        <Head id="lp-features" eyebrow={l.featuresEyebrow} title={l.featuresTitle} />
        <div className="ld-feature">
          <div className="ld-media">
            <Shot name="web-room" alt={t.gallery.items["web-room"].alt} />
            <div className="ld-phone ld-phone-tall">
              <Shot name="app-chat" alt={t.playerApp.chatAlt} />
            </div>
          </div>
          <div className="ld-feature-text">
            <h3>{l.roomTitle}</h3>
            <p className="ld-muted">{l.roomLead}</p>
            <Bullets items={ROOM_FEATURES} />
          </div>
        </div>
        <div className="ld-feature ld-flip">
          <div className="ld-media">
            <Shot name="win-overview" alt={t.gallery.items["win-overview"].alt} />
          </div>
          <div className="ld-feature-text">
            <h3>{l.deviceTitle}</h3>
            <p className="ld-muted">{l.deviceLead}</p>
            <Bullets items={DEVICE_FEATURES} />
          </div>
        </div>
      </section>

      <PlayerApp />

      <section className="ld-section" aria-labelledby="lp-pads">
        <Head
          id="lp-pads"
          eyebrow={l.padsEyebrow}
          title={l.padsTitle}
          text={
            <>
              {l.padsText} <Link to="/test-controller">{t.testController.link}</Link>
            </>
          }
        />
        <ControllersShowcase />
      </section>

      <section className="ld-section ld-sunken" aria-labelledby="lp-flow">
        <Head id="lp-flow" eyebrow={l.flowEyebrow} title={l.flowTitle} text={l.flowText} />
        <div className="ld-path">
          <svg viewBox="0 0 1080 175" role="img" aria-label={l.flowTitle}>
            <rect x="10" y="30" width="340" height="70" rx="12" className="ld-svg-box" />
            <text x="180" y="60" textAnchor="middle" className="ld-svg-title">
              {l.flow[0]!.title}
            </text>
            <text x="180" y="84" textAnchor="middle" className="ld-svg-small">
              {l.flow[0]!.text}
            </text>
            <rect x="730" y="30" width="340" height="70" rx="12" className="ld-svg-box" />
            <text x="900" y="60" textAnchor="middle" className="ld-svg-title">
              {l.flow[2]!.title}
            </text>
            <text x="900" y="84" textAnchor="middle" className="ld-svg-small">
              {l.flow[2]!.text}
            </text>
            <line x1="350" y1="65" x2="730" y2="65" className="ld-svg-line" />
            <text x="540" y="52" textAnchor="middle" className="ld-svg-data">
              {l.flowData}
            </text>
            <rect x="440" y="112" width="200" height="30" rx="8" className="ld-svg-hub" />
            <text x="540" y="132" textAnchor="middle" className="ld-svg-small">
              {l.flow[1]!.title}
            </text>
            <text x="540" y="164" textAnchor="middle" className="ld-svg-small">
              {l.flow[1]!.text}
            </text>
            <line x1="380" y1="67" x2="460" y2="112" className="ld-svg-dash" />
            <line x1="700" y1="67" x2="620" y2="112" className="ld-svg-dash" />
          </svg>
        </div>
        <div className="ld-card ld-legal">
          <span className="ld-k">MAME 2003-Plus · 0.78</span>
          <h3 id="lp-legal">{l.legalTitle}</h3>
          <p>{l.legalText}</p>
          <p className="ld-fine">{l.legalRights}</p>
        </div>
      </section>

      <section className="ld-section ld-final">
        <h2>{l.finalTitle}</h2>
        <p className="ld-muted">{l.finalText}</p>
        {actions(true)}
      </section>

      {joinOpen && <JoinDialog onClose={() => setJoinOpen(false)} />}
      <SiteFooter />
    </div>
  );
}

/** The quick way in: the invitation's code and PIN, in a modal. */
function JoinDialog({ onClose }: { onClose: () => void }) {
  const titleId = useId();
  const closeRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => (e.key === "Escape" || e.code === "Escape") && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);
  return (
    <div className="dialog-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="dialog lp-join" role="dialog" aria-modal="true" aria-labelledby={titleId}>
        <div className="lp-join-head">
          <h2 id={titleId} className="dialog-title">
            {t.landing.joinTitle}
          </h2>
          <button ref={closeRef} type="button" className="icon-button" aria-label={t.help.close} data-tip={t.help.close} onClick={onClose}>
            <CloseIcon />
          </button>
        </div>
        <p className="muted small-plus">{t.landing.joinText}</p>
        <JoinForm className="stack-md" autoFocus />
      </div>
    </div>
  );
}
