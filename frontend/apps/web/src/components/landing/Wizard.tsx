// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { t } from "../../i18n";
import {
  CHECKSUMS_URL,
  DOCKER_IMAGE,
  PLATFORMS,
  RELEASE_PAGE,
  RELEASE_VERSION,
  armDownloadFor,
  detectPlatform,
  downloadFor,
  type Platform,
} from "../../downloads";
import { DownloadIcon, PauseIcon, PlayIcon } from "../Icons";
import { Shot, isPhoneShot, type ShotName } from "./Shot";
import { pageUrl } from "../../role";

const STEP_MS = 7000;

/** Each step's real screenshots: a large one and, over its corner, a smaller one. */
const STAGES: { main: ShotName; inset?: ShotName }[] = [
  { main: "win-pairing" },
  { main: "web-link", inset: "win-pairing" },
  { main: "web-device", inset: "win-roms" },
  { main: "web-invite", inset: "app-pin" },
];

/** What step 1 offers: the file for the chosen system, or the Docker command. */
function DownloadChoice({ platform, onPick }: { platform: Platform; onPick: (p: Platform) => void }) {
  const file = downloadFor(platform);
  const arm = armDownloadFor(platform);
  return (
    <div className="wz-downloads">
      <div className="wz-platforms" role="group" aria-label={t.wizard.platformsLabel}>
        {PLATFORMS.map((p) => (
          <button key={p} type="button" className={`wz-platform${p === platform ? " is-on" : ""}`} aria-pressed={p === platform} onClick={() => onPick(p)}>
            {t.wizard.platform[p]}
          </button>
        ))}
      </div>
      {platform === "docker" && (
        <pre className="wz-command">{`docker run -d --name go-link --network host \\
  -v go-link:/data -v ~/roms:/data/go-link-console/roms \\
  ${DOCKER_IMAGE}`}</pre>
      )}
      {file && platform !== "docker" && (
        <a className="button button-primary wz-get" href={file.url}>
          <DownloadIcon size={18} />
          {t.wizard.download(t.wizard.platform[platform], file.mb)}
        </a>
      )}
      <span className="small muted">
        {t.wizard.note[platform]}
        {arm && (
          <>
            {" "}
            <a href={arm.url}>{t.wizard.arm(arm.mb)}</a>
          </>
        )}
        {platform === "docker" && (
          <>
            {" "}
            <Link to="/docs/docker">{t.wizard.dockerGuide}</Link>
            {file && (
              <>
                {" · "}
                <a href={file.url}>{t.wizard.dockerOffline(file.mb)}</a>
              </>
            )}
          </>
        )}
      </span>
      <span className="small muted">
        {RELEASE_VERSION} · <a href={RELEASE_PAGE}>{t.wizard.allDownloads}</a> · <a href={CHECKSUMS_URL}>{t.wizard.checksums}</a>
      </span>
    </div>
  );
}

/** On a phone: go-link installs on a computer, so share the link instead. */
function PhoneShare() {
  const [copied, setCopied] = useState(false);
  const share = async () => {
    const url = pageUrl("/device");
    try {
      if (navigator.share) await navigator.share({ url, title: "go-link" });
      else {
        await navigator.clipboard.writeText(url);
        setCopied(true);
      }
    } catch {
      // the person closed the share sheet
    }
  };
  return (
    <div className="wz-downloads">
      <span className="small muted">{t.wizard.phoneNote}</span>
      <button type="button" className="button button-secondary wz-get" onClick={() => void share()}>
        {copied ? t.wizard.copied : t.wizard.share}
      </button>
    </div>
  );
}

/**
 * "How it works" as a short walkthrough that plays like a video: four steps
 * (download, link, add ROMs and play, share), each with real screenshots of
 * the device's window, the website and the Player app, a progress bar per
 * step, pause and previous/next. Step 1 offers the right download for the
 * visitor's system.
 */
export function Wizard({ onDone }: { onDone?: () => void }) {
  const detected = useMemo(() => detectPlatform(), []);
  const [step, setStep] = useState(1);
  const [elapsed, setElapsed] = useState(0);
  const [playing, setPlaying] = useState(
    () => !(typeof matchMedia === "function" && matchMedia("(prefers-reduced-motion: reduce)").matches),
  );
  const [platform, setPlatform] = useState<Platform>(detected ?? "windows");
  const steps = t.wizard.steps;

  useEffect(() => {
    if (!playing) return;
    const id = window.setInterval(() => setElapsed((e) => e + 100), 100);
    return () => window.clearInterval(id);
  }, [playing]);
  useEffect(() => {
    if (elapsed < STEP_MS) return;
    setStep((s) => (s % steps.length) + 1);
    setElapsed(0);
  }, [elapsed, steps.length]);

  const go = (n: number) => {
    setStep(n);
    setElapsed(0);
  };
  const cur = steps[step - 1] ?? steps[0]!;

  const stage = STAGES[step - 1] ?? STAGES[0]!;
  const alts = t.wizard.shots[step - 1] ?? t.wizard.shots[0]!;

  return (
    <section className="wz" aria-roledescription="carousel" aria-label={t.wizard.title}>
      <div className="wz-stage" key={`stage-${step}`}>
        <Shot className="wz-shot" name={stage.main} alt={alts.main} />
        {stage.inset && (
          <div className={`wz-inset${isPhoneShot(stage.inset) ? " is-phone" : ""}`}>
            <Shot name={stage.inset} alt={alts.inset} />
          </div>
        )}
      </div>
      <div className="wz-text" key={`text-${step}`} aria-live="polite">
        <span className="wz-mono wz-faint">{t.wizard.stepOf(step, steps.length)}</span>
        <h3 className="wz-title">{cur.title}</h3>
        <p className="wz-body">{cur.text}</p>
        {step === 1 &&
          (detected === null && typeof navigator !== "undefined" && /Android|iPhone|iPad|iPod/i.test(navigator.userAgent) ? (
            <PhoneShare />
          ) : (
            <DownloadChoice
              platform={platform}
              onPick={(p) => {
                setPlatform(p);
                setPlaying(false);
              }}
            />
          ))}
        {step === steps.length && (
          <Link to="/create" className="button button-primary wz-get" onClick={onDone}>
            {t.wizard.createFirst}
          </Link>
        )}
      </div>
      <div className="wz-bar">
        <div className="wz-segments">
          {steps.map((s, i) => {
            const n = i + 1;
            const pct = n < step ? 100 : n === step ? (elapsed / STEP_MS) * 100 : 0;
            return (
              <button key={s.short} type="button" className={`wz-seg${n === step ? " is-on" : ""}`} aria-label={t.wizard.goTo(n, s.short)} aria-current={n === step ? "step" : undefined} onClick={() => go(n)}>
                <span className="wz-track">
                  <span style={{ width: `${pct}%` }} />
                </span>
                <span className="wz-seg-label">{s.short}</span>
              </button>
            );
          })}
        </div>
        <div className="wz-controls">
          <button type="button" className="icon-button" aria-label={t.wizard.prev} onClick={() => go(step === 1 ? steps.length : step - 1)}>
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="M15 6l-6 6 6 6" />
            </svg>
          </button>
          <button type="button" className="icon-button wz-toggle" aria-label={playing ? t.wizard.pause : t.wizard.play} onClick={() => setPlaying(!playing)}>
            {playing ? <PauseIcon /> : <PlayIcon />}
          </button>
          <button type="button" className="icon-button" aria-label={t.wizard.next} onClick={() => go((step % steps.length) + 1)}>
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="M9 6l6 6-6 6" />
            </svg>
          </button>
        </div>
      </div>
    </section>
  );
}
