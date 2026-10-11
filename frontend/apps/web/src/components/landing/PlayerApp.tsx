// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { t } from "../../i18n";
import { UserPlusIcon } from "../Icons";
import { AppDownloads } from "./AppDownloads";
import { Gallery } from "./Gallery";
import { Shot } from "./Shot";

/**
 * The go-link Console Player app for invited players (Android now, iPhone later),
 * with two real screenshots, and below it the gallery of real screenshots
 * of the website, the device's window and the app.
 */
export function PlayerApp() {
  const a = t.playerApp;
  return (
    <section className="lp-section lp-band lp-app" aria-labelledby="lp-app">
      <div className="lp-app-split">
        <div className="lp-app-text">
          <span className="eyebrow eyebrow-accent">{a.eyebrow}</span>
          <h2 id="lp-app" className="lp-h2">
            {a.title}
          </h2>
          <p className="lp-lead">{a.text}</p>
          <p className="lp-app-note small-plus">
            <UserPlusIcon size={20} />
            <span>
              <strong>{a.noteStrong}</strong> {a.note}
            </span>
          </p>
          <AppDownloads />
        </div>
        <div className="lp-app-phones">
          <div className="lp-phone is-back">
            <Shot name="app-chat" alt={a.chatAlt} />
          </div>
          <div className="lp-phone is-front">
            <Shot name="app-room" alt={a.roomAlt} />
          </div>
        </div>
      </div>
      <Gallery />
    </section>
  );
}
