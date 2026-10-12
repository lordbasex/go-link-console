// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { lazy, Suspense, useEffect, useRef, useState } from "react";
import { Navigate, Route, Routes, useLocation } from "react-router-dom";
import { CustomServerBanner, ServerDialog } from "./components/ServerSettings";
import { MainHeader } from "./components/Headers";
import { RecordingNotices } from "./components/Recordings";
import { PauseAskNotices } from "./components/PauseAskNotices";
import { useSignal } from "./signal/SignalProvider";
import { SPLASH_MAX_MS, SPLASH_MIN_MS, hideSplash } from "./splash";
import { LobbyPage, readRoomsView } from "./pages/LobbyPage";
// The lobby shows the join form to guests, so it comes with the lobby.
import { GuestJoinPage } from "./pages/GuestJoinPage";
import { HandoffBanner } from "./components/HandoffBanner";
import { HANDOFF_PATH, hasHandoffData, otherSiteAddress } from "./handoff";
import { HAS_PLAY, HAS_SITE, ROLE, servesPath } from "./role";
import { prefetchThumbnails } from "./components/device/useThumbnail";
import { LegalPage } from "./pages/LegalPage";
import { NotFoundPage } from "./pages/NotFoundPage";
import { ANALYTICS_SITE, analyticsLoaded, isPublicPath, startAnalytics, useConsent } from "./analytics";
import { CookieConsent } from "./components/CookieConsent";
import { DestroyLink } from "./destroyLauncher";
// The lobby comes with the first load; the other pages download when
// opened, so a guest joining a room does not get the device dashboard.
// Each build has only its own site's pages (src/role.ts): the check on the
// build's constant is written out in each line so the bundler leaves the
// other site's pages out of the build.
const NotHere = () => null;
const CreateRoomPage = import.meta.env.VITE_ROLE !== "site"
  ? lazy(() => import("./pages/CreateRoomPage").then((m) => ({ default: m.CreateRoomPage })))
  : NotHere;
const RoomPage = import.meta.env.VITE_ROLE !== "site"
  ? lazy(() => import("./pages/RoomPage").then((m) => ({ default: m.RoomPage })))
  : NotHere;
const PanelLoginPage = import.meta.env.VITE_ROLE !== "site"
  ? lazy(() => import("./pages/PanelLoginPage").then((m) => ({ default: m.PanelLoginPage })))
  : NotHere;
const DevicePage = import.meta.env.VITE_ROLE !== "site"
  ? lazy(() => import("./pages/DevicePage").then((m) => ({ default: m.DevicePage })))
  : NotHere;
const HowItWorksPage = import.meta.env.VITE_ROLE !== "play"
  ? lazy(() => import("./pages/HowItWorksPage").then((m) => ({ default: m.HowItWorksPage })))
  : NotHere;
const DocsPage = import.meta.env.VITE_ROLE !== "play"
  ? lazy(() => import("./pages/DocsPage").then((m) => ({ default: m.DocsPage })))
  : NotHere;
const PictureDemoPage = import.meta.env.VITE_ROLE !== "play"
  ? lazy(() => import("./pages/PictureDemoPage").then((m) => ({ default: m.PictureDemoPage })))
  : NotHere;
const ToolsPage = import.meta.env.VITE_ROLE !== "play"
  ? lazy(() => import("./pages/ToolsPage").then((m) => ({ default: m.ToolsPage })))
  : NotHere;
const GamesPage = import.meta.env.VITE_ROLE !== "play"
  ? lazy(() => import("./pages/GamesPage").then((m) => ({ default: m.GamesPage })))
  : NotHere;
const SkinEditorPage = import.meta.env.VITE_ROLE !== "play"
  ? lazy(() => import("./pages/SkinEditorPage").then((m) => ({ default: m.SkinEditorPage })))
  : NotHere;
const MakerRedirect = import.meta.env.VITE_ROLE !== "play"
  ? lazy(() => import("./pages/MakerRedirect").then((m) => ({ default: m.MakerRedirect })))
  : NotHere;
const HandoffPage = import.meta.env.VITE_ROLE !== "play"
  ? lazy(() => import("./pages/HandoffPage").then((m) => ({ default: m.HandoffPage })))
  : NotHere;
const MakerBridgePage = lazy(() => import("./pages/MakerBridgePage").then((m) => ({ default: m.MakerBridgePage })));
const TestControllerPage = lazy(() => import("./pages/TestControllerPage").then((m) => ({ default: m.TestControllerPage })));
// The picture quality lab exists only in development builds (npm run dev).
const PictureLabPage = import.meta.env.DEV
  ? lazy(() => import("./pages/PictureLabPage").then((m) => ({ default: m.PictureLabPage })))
  : null;

/**
 * Keeps the logo of index.html up until the first screen can show real
 * data: a guest (nothing to wait for), the owner whose device is off, or
 * the owner whose linked device already sent its status (rooms and
 * library) and the art of its rooms. Never shorter than SPLASH_MIN_MS
 * (branding), never longer than SPLASH_MAX_MS. Without this, the page
 * showed an empty rooms list or the pairing form for a second.
 */
function useSplash() {
  const {
    demo,
    savedLink,
    hostLink,
    deviceOffline,
    linkNotice,
    linkedDevice,
    onDeviceMessage,
  } = useSignal();
  const status = linkedDevice.status;
  const [artReady, setArtReady] = useState(false);
  const prefetched = useRef<unknown>(null);
  useEffect(() => {
    if (!hostLink || !status || prefetched.current === hostLink) return;
    prefetched.current = hostLink;
    const kind = status.library?.thumbKind ?? "boxart";
    // The device answers "missing" at once for a set without a picture,
    // so the rooms' sets are asked without looking them up first.
    const sets = [...new Set(status.rooms.map((r) => r.rom))].slice(0, 24);
    void prefetchThumbnails(
      hostLink.stream,
      onDeviceMessage,
      sets,
      kind,
      readRoomsView() === "list" ? "mini" : "card",
    ).then(() => setArtReady(true));
  }, [hostLink, status, onDeviceMessage]);

  const linkedReady = hostLink !== null && status !== null && artReady;
  const known =
    demo ||
    !savedLink ||
    linkedReady ||
    linkedDevice.state === "failed" ||
    deviceOffline ||
    linkNotice !== null;
  const [timeUp, setTimeUp] = useState(
    () => performance.now() >= SPLASH_MIN_MS,
  );
  useEffect(() => {
    const left = SPLASH_MIN_MS - performance.now();
    const minTimer =
      left > 0 ? window.setTimeout(() => setTimeUp(true), left) : undefined;
    const maxTimer = window.setTimeout(
      hideSplash,
      Math.max(0, SPLASH_MAX_MS - performance.now()),
    );
    return () => {
      window.clearTimeout(minTimer);
      window.clearTimeout(maxTimer);
    };
  }, []);
  useEffect(() => {
    if (known && timeUp) hideSplash();
  }, [known, timeUp]);
}

/**
 * A path this site does not serve: the other site's page (the landing's
 * pages from play, the rooms from the landing), else "not found".
 */
function Elsewhere() {
  const { pathname, search, hash } = useLocation();
  const away = !servesPath(pathname);
  useEffect(() => {
    if (!away) return;
    let carry = false;
    try {
      carry = hasHandoffData(window.localStorage);
    } catch {
      // storage blocked: nothing to bring
    }
    window.location.replace(otherSiteAddress(`${pathname}${search}${hash}`, ROLE, carry));
  }, [away, pathname, search, hash]);
  return away ? null : <NotFoundPage />;
}

export function App() {
  useSplash();
  const { panel, savedLink, hostLink } = useSignal();
  // The device's local panel asks for its token before anything else.
  const needsToken = panel && !savedLink && !hostLink;
  // console.go-link.org's public pages carry the cookie notice and, with
  // consent, Google Analytics (analytics.ts); nothing else ever does.
  const { pathname } = useLocation();
  const { consent } = useConsent();
  const counted = ANALYTICS_SITE && !panel && isPublicPath(pathname);
  useEffect(() => {
    if (counted && consent === "granted") startAnalytics();
  }, [counted, consent]);
  // Google's script is in this page: a page that is not public waits for
  // the full page load guardHistory started instead of rendering next to it.
  if (ANALYTICS_SITE && !counted && analyticsLoaded()) return null;
  return (
    <>
      <CustomServerBanner />
      {/* One header for every page: it stays put while the page changes. */}
      <MainHeader />
      {HAS_PLAY && <HandoffBanner />}
      <RecordingNotices />
      <PauseAskNotices />
      {/* The one main landmark: every page renders inside it. */}
      <main className="app-main">
      <Suspense fallback={<div className="page" aria-busy="true" />}>
        {needsToken ? (
          <PanelLoginPage />
        ) : (
          <Routes>
            {/* The home page is the landing. The rooms' own site (and the
                device's local panel) has no landing: it opens on the device
                until it is linked, then on its rooms. */}
            <Route
              path="/"
              element={
                panel || !HAS_SITE ? (
                  <Navigate to={savedLink || hostLink || !panel ? "/rooms" : "/device"} replace />
                ) : (
                  <HowItWorksPage />
                )
              }
            />
            {HAS_SITE && (
              <>
                {/* The Destroy game's direct link (routes match any letter case). */}
                <Route path="/WillyGorklingo" element={<DestroyLink />} />
                <Route path="/how-it-works" element={<Navigate to="/" replace />} />
                <Route path="/tools" element={<ToolsPage />} />
                <Route path="/tools/skin-editor" element={<SkinEditorPage />} />
                <Route path="/tools/games" element={<GamesPage />} />
                {/* Willy Maker has its own site */}
                <Route path="/tools/willy-maker" element={<MakerRedirect />} />
                <Route path="/tools/willy-maker/:gameId" element={<MakerRedirect />} />
                <Route path="/picture-demo" element={<PictureDemoPage />} />
                {PictureLabPage && <Route path="/picture-lab" element={<PictureLabPage />} />}
                <Route path="/docs" element={<DocsPage />} />
                <Route path="/docs/:slug" element={<DocsPage />} />
                <Route path={HANDOFF_PATH} element={<HandoffPage />} />
              </>
            )}
            {HAS_PLAY && (
              <>
                <Route path="/rooms" element={<LobbyPage />} />
                <Route path="/create" element={<CreateRoomPage />} />
                <Route path="/r/:roomId" element={<RoomPage />} />
                {/* An invitation: /g/<invite> from a link or QR, or /g/<code>. */}
                <Route path="/g/:invite" element={<RoomPage />} />
                <Route path="/g" element={<GuestJoinPage />} />
                <Route path="/device" element={<DevicePage />} />
                {/* Rooms moved to the home page. */}
                <Route path="/device/rooms" element={<Navigate to="/rooms" replace />} />
                <Route path="/device/roms" element={<DevicePage tab="roms" />} />
                <Route path="/device/history" element={<DevicePage tab="history" />} />
                <Route path="/device/history/:roomId/network" element={<DevicePage tab="network" />} />
              </>
            )}
            {/* Both sites: the controller settings are the ones rooms and the
                mini-games use; the maker's bridge reaches the device on play
                and the games made on the landing before the maker moved. */}
            <Route path="/test-controller" element={<TestControllerPage />} />
            <Route path="/maker-bridge" element={<MakerBridgePage />} />
            <Route path="/terms" element={<LegalPage doc="terms" />} />
            <Route path="/privacy" element={<LegalPage doc="privacy" />} />
            {/* A page of the other site goes there; anything else is not found. */}
            <Route path="*" element={<Elsewhere />} />
          </Routes>
        )}
      </Suspense>
      </main>
      <ServerDialog />
      {counted && <CookieConsent />}
    </>
  );
}
