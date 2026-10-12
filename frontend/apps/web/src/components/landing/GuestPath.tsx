// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { t } from "../../i18n";
import { AppDownloads } from "./AppDownloads";
import { Shot, type ShotName } from "./Shot";

/** The three pictures of the quick path, in step order. */
const STEP_SHOTS: ShotName[] = ["app-home", "app-pin", "app-room"];

/**
 * "Someone sent you a QR?": the guest's way into a game in three steps
 * (scan, type the PIN, play and talk), each with a real screenshot of the
 * Player app, then the app's downloads and the browser as the other way.
 */
export function GuestPath({ onJoin }: { onJoin: () => void }) {
  const g = t.guestPath;
  return (
    <section className="ld-section ld-sunken ld-guest" aria-labelledby="lp-guest">
      <header className="ld-head ld-head-row">
        <div>
          <p className="ld-eyebrow">{g.eyebrow}</p>
          <h2 id="lp-guest">{g.title}</h2>
        </div>
        <span className="ld-chip ld-chip-info">{g.only}</span>
      </header>
      <ol className="ld-ways">
        {g.steps.map((s, i) => (
          <li key={s.title} className="ld-way">
            <div className="ld-shot">
              <Shot name={STEP_SHOTS[i]!} alt={s.alt} />
            </div>
            <div className="ld-way-body">
              <span className="ld-step-num" aria-hidden="true">
                {i + 1}
              </span>
              <h3>{s.title}</h3>
              <p>{s.text}</p>
            </div>
          </li>
        ))}
      </ol>
      <div className="ld-bar">
        <AppDownloads showMeta={false} />
        <p className="ld-bar-note">
          {g.browser}{" "}
          <button type="button" className="ld-textlink" onClick={onJoin}>
            {g.browserCta}
          </button>
        </p>
      </div>
    </section>
  );
}
