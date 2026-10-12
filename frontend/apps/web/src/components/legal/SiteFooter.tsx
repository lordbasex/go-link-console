// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { Link } from "react-router-dom";
import { ANALYTICS_SITE, askConsent } from "../../analytics";
import { LANGS, setLang, t, useLang } from "../../i18n";
import { Brand } from "../Headers";
import { GithubIcon } from "../Icons";
import { APP_VERSION, BUILD_DATE, REPO_URL } from "../../config";
import { ROLE, homeHref } from "../../role";

/** The site's footer: links, licenses, trademarks and the legal pages. */
export function SiteFooter() {
  const f = t.legal.footer;
  const lang = useLang();
  return (
    <footer className="site-footer">
      <div className="site-footer-inner">
        <div className="site-footer-brand stack-sm">
          <Brand />
          <p className="small muted">{f.tagline}</p>
          <a className="icon-button" href={REPO_URL} target="_blank" rel="noopener" aria-label={f.github} data-tip={f.github}>
            <GithubIcon />
          </a>
        </div>
        <nav className="site-footer-col" aria-label={f.product}>
          <h2 className="site-footer-title">{f.product}</h2>
          {ROLE === "play" ? <a href={homeHref()}>{t.nav.howItWorks}</a> : <Link to="/">{t.nav.howItWorks}</Link>}
          <Link to="/rooms">{t.nav.rooms}</Link>
          <Link to="/device">{t.nav.myDevice}</Link>
          <Link to="/docs">{t.nav.docs}</Link>
        </nav>
        <nav className="site-footer-col" aria-label={f.project}>
          <h2 className="site-footer-title">{f.project}</h2>
          <a href={REPO_URL} target="_blank" rel="noopener">
            {f.source}
          </a>
          <a href={`${REPO_URL}/releases`} target="_blank" rel="noopener">
            {f.releases}
          </a>
          <a href={`${REPO_URL}/blob/main/LICENSE`} target="_blank" rel="noopener">
            {f.license}
          </a>
          <a href={`${REPO_URL}/blob/main/THIRD_PARTY_NOTICES.md`} target="_blank" rel="noopener">
            {f.notices}
          </a>
        </nav>
        <nav className="site-footer-col" aria-label={f.legal}>
          <h2 className="site-footer-title">{f.legal}</h2>
          <Link to="/terms">{t.legal.terms.title}</Link>
          <Link to="/privacy">{t.legal.privacy.title}</Link>
          <a href={`${REPO_URL}/blob/main/docs/legal.md`} target="_blank" rel="noopener">
            {f.legalDoc}
          </a>
        </nav>
      </div>
      <div className="site-footer-bottom small faint">
        <p>
          © 2026 {f.rights} {f.noRoms}
        </p>
        <p>{f.trademark}</p>
        <div className="site-footer-row">
          <p className="site-footer-version">
            {f.version}: <code>{APP_VERSION}</code>
            {BUILD_DATE && ` · ${BUILD_DATE}`}
            {ANALYTICS_SITE && (
              <>
                {" · "}
                <button type="button" className="site-footer-link" onClick={askConsent}>
                  {t.consent.settings}
                </button>
              </>
            )}
          </p>
          <div className="site-footer-langs" role="group" aria-label={t.consent.language}>
            {LANGS.map((l) => (
              <button key={l.id} type="button" lang={l.id} aria-pressed={l.id === lang} onClick={() => setLang(l.id)}>
                {l.label}
              </button>
            ))}
          </div>
        </div>
      </div>
    </footer>
  );
}
