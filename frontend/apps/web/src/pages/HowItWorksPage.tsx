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
import {
  CloseIcon,
  ControllerIcon,
  GamepadIcon,
  LockIcon,
  MicIcon,
  MonitorIcon,
  PhoneIcon,
  PlayIcon,
  RecordIcon,
} from "../components/Icons";

/** Icons of the features, in their order. */
const FEATURE_ICONS: ReactNode[] = [
  <PlayIcon key="live" />,
  <GamepadIcon key="seats" />,
  <MicIcon key="voice" size={20} />,
  <ControllerIcon key="pads" />,
  <LockIcon key="private" size={19} />,
  <MonitorIcon key="home" size={20} />,
  <RecordIcon key="record" size={18} />,
  <ControllerIcon key="console" />,
  <PhoneIcon key="android" />,
];

/**
 * "How it works": the landing page. What go-link is, the guest's quick way
 * into a game someone invited them to (right under the hero: most visitors
 * arrive with an invitation), the four steps to host one, what it does, the
 * Player app with a gallery of real screenshots, and where the games come
 * from (no game art, no game names: only that it plays MAME 2003-Plus sets
 * you have the right to use).
 */
export function HowItWorksPage() {
  const [joinOpen, setJoinOpen] = useState(false);
  // The PIN travels in the page's history state, which stays on this
  // origin: the landing's own site sends guests to the join page of the
  // rooms' site instead of asking here.
  const join = () => (ROLE === "site" ? window.location.assign(`${PLAY_URL}/g`) : setJoinOpen(true));
  return (
    <div className="page lp">
      <section className="lp-hero">
        <div className="lp-hero-text">
          <span className="eyebrow eyebrow-accent">{t.landing.eyebrow}</span>
          <h1 className="lp-title">
            {t.landing.titleStart} <span className="lp-accent">{t.landing.titleAccent}</span>
          </h1>
          <p className="lp-lead">{t.landing.lead}</p>
          <div className="lp-actions">
            <button type="button" className="button button-primary lp-cta" onClick={join}>
              <GamepadIcon size={18} />
              {t.landing.joinCta}
            </button>
            <Link to="/device" className="button button-secondary lp-cta">
              <MonitorIcon size={18} />
              {t.landing.hostCta}
            </Link>
          </div>
          <p className="small faint">{t.landing.platforms}</p>
        </div>
        <GameScene />
      </section>

      <GuestPath onJoin={join} />

      <section className="lp-section" id="how" aria-labelledby="lp-steps">
        <span className="eyebrow eyebrow-accent">{t.wizard.eyebrow}</span>
        <h2 id="lp-steps" className="lp-h2">
          {t.wizard.title}
        </h2>
        <Wizard />
      </section>

      <section className="lp-section" aria-labelledby="lp-features">
        <span className="eyebrow eyebrow-accent">{t.landing.featuresEyebrow}</span>
        <h2 id="lp-features" className="lp-h2">
          {t.landing.featuresTitle}
        </h2>
        <ul className="lp-features">
          {t.landing.features.map((f, i) => (
            <li key={f.title} className="lp-feature">
              <span className="lp-feature-icon" aria-hidden="true">
                {FEATURE_ICONS[i]}
              </span>
              <h3 className="lp-h3">{f.title}</h3>
              <p className="muted">{f.text}</p>
            </li>
          ))}
        </ul>
      </section>

      <PlayerApp />

      <section className="lp-section" aria-labelledby="lp-pads">
        <span className="eyebrow eyebrow-accent">{t.landing.padsEyebrow}</span>
        <h2 id="lp-pads" className="lp-h2">
          {t.landing.padsTitle}
        </h2>
        <p className="muted lp-section-lead">
          {t.landing.padsText}{" "}
          <Link to="/test-controller">{t.testController.link}</Link>
        </p>
        <ControllersShowcase />
      </section>

      <section className="lp-section lp-flow" aria-labelledby="lp-flow">
        <div className="stack-sm">
          <span className="eyebrow eyebrow-accent">{t.landing.flowEyebrow}</span>
          <h2 id="lp-flow" className="lp-h2">
            {t.landing.flowTitle}
          </h2>
          <p className="muted">{t.landing.flowText}</p>
        </div>
        <ol className="lp-flow-diagram" aria-label={t.landing.flowTitle}>
          {t.landing.flow.map((node, i) => (
            <li key={node.title} className={`lp-node${i === 1 ? " is-middle" : ""}`}>
              <strong>{node.title}</strong>
              <span className="small muted">{node.text}</span>
            </li>
          ))}
        </ol>
      </section>

      <section className="lp-section lp-legal" aria-labelledby="lp-legal">
        <h2 id="lp-legal" className="lp-h3">
          {t.landing.legalTitle}
        </h2>
        <p className="muted">{t.landing.legalText}</p>
        <p className="small faint">{t.landing.legalRights}</p>
      </section>

      <section className="lp-section lp-final">
        <h2 className="lp-h2">{t.landing.finalTitle}</h2>
        <p className="muted">{t.landing.finalText}</p>
        <div className="lp-actions is-centered">
          <button type="button" className="button button-primary lp-cta" onClick={join}>
            <GamepadIcon size={18} />
            {t.landing.joinCta}
          </button>
          <Link to="/device" className="button button-secondary lp-cta">
            <MonitorIcon size={18} />
            {t.landing.hostCta}
          </Link>
        </div>
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
