// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { setConsent, useConsent } from "../analytics";
import { t } from "../i18n";

/**
 * The cookie notice of console.go-link.org's public pages: go-link keeps
 * nothing; with the visitor's yes, Google Analytics counts visits to this
 * website. Shown until they choose, and again from "Cookie settings".
 */
export function CookieConsent() {
  const { consent, asking } = useConsent();
  if (consent !== null && !asking) return null;
  const c = t.consent;
  return (
    <section className="consent" aria-labelledby="consent-title">
      <div className="consent-text">
        <strong id="consent-title">{c.title}</strong>
        <p>{c.text}</p>
      </div>
      <div className="consent-actions">
        <button type="button" className="button button-secondary" onClick={() => setConsent("denied")}>
          {c.reject}
        </button>
        <button type="button" className="button button-primary" onClick={() => setConsent("granted")} autoFocus={asking}>
          {c.accept}
        </button>
      </div>
    </section>
  );
}
