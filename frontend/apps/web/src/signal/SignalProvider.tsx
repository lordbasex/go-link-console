// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { TERMS_VERSION, termsAccepted } from "../legal";
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import {
  APP,
  HostStream,
  SignalClient,
  parseDeviceStatus,
  type DeviceStatus,
  type StreamState,
  clearCustomSignalUrl,
  resolveSignalUrl,
  saveCustomSignalUrl,
  testSignalServer,
  clearSavedLink,
  loadSavedLink,
  parseAuthOk,
  linkProof,
  panelProof,
  randomNonce,
  sameString,
  saveLink,
  type ConnectionState,
  type Envelope,
  type SavedLink,
  type SignalUrlChoice,
  type Handshake,
} from "@go-link/shared";
import { cleanPanelToken, PANEL_TOKEN_KEY } from "../panel";

/** The device's answer to a wrong panel token. */
export const PANEL_BAD_TOKEN = "invalid panel token";

/** Why a remembered link ended: shown once on the device page. */
export type LinkNotice = "unlinked" | "auth_failed";

/** A browser linked to a device through the /device pairing code. */
export interface HostLink {
  client: SignalClient;
  sessionId: string;
  devicePeerId: string;
  /** WebRTC data link to the device (status and latency). */
  stream: HostStream;
}

/** Live data from the linked device, received over WebRTC. */
export interface LinkedDeviceView {
  status: DeviceStatus | null;
  /** Peer-to-peer round trip from the browser's ICE stats. */
  rttMs: number | null;
  /** "direct" or "relay" (through TURN), from the selected ICE pair. */
  path: "direct" | "relay" | null;
  state: StreamState | "idle";
}

export interface SignalContextValue {
  /** Shared connection for the lobby and for joining rooms. */
  client: SignalClient;
  state: ConnectionState;
  server: SignalUrlChoice;
  defaultUrl: string;
  /** Switches to a custom server (already validated and tested). */
  setCustomServer: (url: string) => void;
  resetToOfficialServer: () => void;
  hostLink: HostLink | null;
  linkedDevice: LinkedDeviceView;
  /** Sends a request to the linked device on its WebRTC control channel. */
  sendToDevice: (msg: unknown) => boolean;
  /** Listens to the device's replies (room_created, room_error...). */
  onDeviceMessage: (fn: (msg: unknown) => void) => () => void;
  /** Redeems a pairing code on a dedicated connection. */
  linkDevice: (code: string) => Promise<void>;
  /** Forgets the device: this browser will need a new code. */
  unlinkDevice: () => void;
  /** The device this browser linked before, for the current server. */
  savedLink: SavedLink | null;
  /** A saved device that is not connected right now (retrying). */
  deviceOffline: boolean;
  linkNotice: LinkNotice | null;
  clearLinkNotice: () => void;
  demo: boolean;
  /** This page is a headless device's local web panel. */
  panel: boolean;
  /** Checks that a server answers with hello before it is saved. */
  testServer: (url: string) => Promise<void>;
  /** Opens the signaling server dialog from anywhere. */
  serverDialogOpen: boolean;
  setServerDialogOpen: (open: boolean) => void;
}

const SignalContext = createContext<SignalContextValue | null>(null);

export interface SignalProviderProps {
  defaultUrl: string;
  demo?: boolean;
  storage?: Storage;
  /** Injectable for tests. */
  createClient?: (url: string, handshake?: Handshake) => SignalClient;
  testServer?: (url: string) => Promise<void>;
  /**
   * The device's own WebSocket when this page is a headless device's local
   * web panel: the link then goes there, with the panel token.
   */
  panelUrl?: string;
  /**
   * No connection at all and no device link: the landing's own site
   * (console.go-link.org), whose pages never need the signaling server.
   */
  offline?: boolean;
  children: ReactNode;
}

function safeStorage(): Storage | undefined {
  try {
    return window.localStorage;
  } catch {
    return undefined;
  }
}

/** The panel token as a saved link, so the rest of the app treats the panel like a remembered device. */
function panelLink(storage: Storage | undefined, panelUrl: string): SavedLink | null {
  let token = "";
  try {
    token = cleanPanelToken(storage?.getItem(PANEL_TOKEN_KEY) ?? "");
  } catch {
    return null;
  }
  return token ? { deviceId: "panel", linkId: "panel", token, signalUrl: panelUrl, savedAt: 0 } : null;
}

export function SignalProvider({ defaultUrl, demo = false, storage = safeStorage(), createClient, testServer = testSignalServer, panelUrl = "", offline = false, children }: SignalProviderProps) {
  const make = useCallback(
    (url: string, handshake?: Handshake) => (createClient ? createClient(url, handshake) : new SignalClient({ url, handshake })),
    [createClient],
  );
  const [server, setServer] = useState<SignalUrlChoice>(() => resolveSignalUrl(defaultUrl, storage));
  const [savedLink, setSavedLink] = useState<SavedLink | null>(() =>
    demo || offline ? null : panelUrl ? panelLink(storage, panelUrl) : loadSavedLink(storage, server.url),
  );
  const panelToken = panelUrl ? (savedLink?.token ?? "") : "";
  // On the device's local panel, rooms go through the device's own socket
  // (signalhub refuses the panel's LAN origin), which proves the panel
  // token for playing (no data link) after every (re)connection.
  const client = useMemo(
    () =>
      panelUrl
        ? make(panelUrl, (link) =>
            link
              .request({ type: "panel", app: APP }, ["panel_nonce"])
              .then((n) => link.request({ type: "panel", app: APP, mode: "room", proof: panelProof(panelToken, n.nonce ?? "") }, ["paired"]))
              .then(() => undefined),
          )
        : make(server.url),
    [make, panelUrl, panelToken, server.url],
  );
  const [state, setState] = useState<ConnectionState>(client.state);
  const [hostLink, setHostLink] = useState<HostLink | null>(null);
  const [linkedDevice, setLinkedDevice] = useState<LinkedDeviceView>({ status: null, rttMs: null, path: null, state: "idle" });
  const deviceListeners = useMemo(() => new Set<(msg: unknown) => void>(), []);
  const onDeviceMessage = useCallback(
    (fn: (msg: unknown) => void) => {
      deviceListeners.add(fn);
      return () => {
        deviceListeners.delete(fn);
      };
    },
    [deviceListeners],
  );
  const [serverDialogOpen, setServerDialogOpen] = useState(false);

  useEffect(() => {
    // The panel's socket needs the token: without it the device hangs up.
    if ((panelUrl && !panelToken) || offline) return;
    const off = client.onState(setState);
    client.connect();
    setState(client.state);
    return () => {
      off();
      client.close();
    };
  }, [client, panelUrl, panelToken, offline]);
  const [deviceOffline, setDeviceOffline] = useState(false);
  const [linkNotice, setLinkNotice] = useState<LinkNotice | null>(null);
  const clearLinkNotice = useCallback(() => setLinkNotice(null), []);

  // Closes the data link without forgetting the device (server change).
  const closeLink = useCallback(() => {
    setHostLink((link) => {
      link?.stream.close();
      link?.client.close();
      return null;
    });
  }, []);

  const unlinkDevice = useCallback(() => {
    if (panelUrl) {
      // The panel: forget the token here; the device keeps it.
      try {
        storage?.removeItem(PANEL_TOKEN_KEY);
      } catch {
        // storage blocked
      }
      setSavedLink(null);
      setDeviceOffline(false);
      setHostLink((link) => {
        link?.stream.close();
        link?.client.close();
        return null;
      });
      return;
    }
    clearSavedLink(storage);
    setSavedLink(null);
    setDeviceOffline(false);
    setHostLink((link) => {
      if (link) {
        // Tell the device to forget this browser, then hang up once the
        // message had time to leave.
        link.stream.sendControl({ type: "unlink" });
        setTimeout(() => {
          link.stream.close();
          link.client.close();
        }, 300);
      }
      return null;
    });
  }, [storage, panelUrl]);

  // Poll the peer-to-peer round trip while a device is linked.
  useEffect(() => {
    if (!hostLink) {
      setLinkedDevice({ status: null, rttMs: null, path: null, state: "idle" });
      return;
    }
    const timer = setInterval(() => {
      hostLink.stream
        .stats()
        .then((st) => setLinkedDevice((cur) => ({ ...cur, rttMs: st.rttMs, path: st.path })))
        .catch(() => undefined);
    }, 2000);
    return () => clearInterval(timer);
  }, [hostLink]);

  // Changing server invalidates everything tied to the old one; a link
  // saved for the new server (if any) takes over.
  useEffect(() => {
    if (!demo && !panelUrl && !offline) setSavedLink(loadSavedLink(storage, server.url));
    return panelUrl ? undefined : closeLink;
  }, [server.url, closeLink, storage, demo, panelUrl, offline]);

  const setCustomServer = useCallback(
    (url: string) => {
      if (storage) {
        if (url === defaultUrl) clearCustomSignalUrl(storage);
        else saveCustomSignalUrl(url, storage);
      }
      setServer({ url, custom: url !== defaultUrl });
    },
    [defaultUrl, storage],
  );

  const resetToOfficialServer = useCallback(() => {
    if (storage) clearCustomSignalUrl(storage);
    setServer({ url: defaultUrl, custom: false });
  }, [defaultUrl, storage]);

  // Opens the data link to the device, after a code (claim) or coming
  // back (reach). The first control message is auth: empty after a code,
  // with the saved token when coming back.
  const openLink = useCallback(
    async (request: Envelope, saved: SavedLink | null) => {
      const link = make(panelUrl || server.url);
      link.connect();
      try {
        // The local panel: answer its nonce with a proof; the token itself
        // never leaves this browser.
        const paired =
          request.type === "panel"
            ? await link
                .request({ type: "panel", app: APP }, ["panel_nonce"])
                .then((n) => link.request({ type: "panel", app: APP, proof: panelProof(request.token ?? "", n.nonce ?? "") }, ["paired"]))
            : await link.request(request, ["paired"]);
        const devicePeerId = paired.remote ?? "";
        let drop = () => {};
        const nonce = randomNonce();
        // The device opens a data-only WebRTC connection right after
        // pairing; listen before its offer arrives.
        const stream: HostStream = new HostStream({
          client: link,
          hostPeerId: devicePeerId,
          onState: (state) => {
            setLinkedDevice((cur) => ({ ...cur, state }));
            // A dead connection (the device hung up, or the network) is
            // released: a saved device is then reached again.
            if (state === "failed" || state === "closed") drop();
          },
          onControlOpen: () => {
            // A remembered browser asks the device to prove itself first:
            // its token must never reach someone else answering for the
            // device_id on signalhub.
            stream.sendControl(
              saved ? { type: "auth_challenge", link_id: saved.linkId, nonce } : { type: "auth", terms: TERMS_VERSION },
            );
          },
          onControl: (msg) => {
            const proof = msg as { type?: unknown; link_id?: unknown; proof?: unknown };
            if (saved && proof?.type === "auth_proof") {
              if (typeof proof.proof === "string" && sameString(proof.proof, linkProof(saved.token, nonce))) {
                stream.sendControl({
                  type: "auth",
                  link_id: saved.linkId,
                  token: saved.token,
                  ...(termsAccepted() ? { terms: TERMS_VERSION } : {}),
                });
              } else {
                // Not the device this browser linked to: hang up, keep the token.
                drop();
              }
              return;
            }
            const ok = parseAuthOk(msg);
            if (ok) {
              // Only a link that proved itself clears an old notice.
              setLinkNotice(null);
              if (ok.token) {
                const fresh: SavedLink = { deviceId: ok.device_id, linkId: ok.link_id, token: ok.token, signalUrl: server.url, savedAt: Date.now() };
                saveLink(storage, fresh);
                setSavedLink(fresh);
              }
              return;
            }
            const kind = (msg as { type?: string } | null)?.type;
            if (kind === "auth_failed" || kind === "unlinked") {
              clearSavedLink(storage);
              setSavedLink(null);
              setLinkNotice(kind);
              drop();
              return;
            }
            const status = parseDeviceStatus(msg);
            if (status) setLinkedDevice((cur) => ({ ...cur, status }));
            else deviceListeners.forEach((fn) => fn(msg));
          },
        });
        stream.start();
        const next: HostLink = { client: link, sessionId: paired.session_id ?? "", devicePeerId, stream };
        // The link dies with the device or with this connection.
        const offMsg = link.onMessage((env) => {
          if (env.type === "peer_left" && env.from === next.devicePeerId) drop();
        });
        const offState = link.onState((s) => {
          if (s !== "open") drop();
        });
        let dropped = false;
        drop = () => {
          if (dropped) return;
          dropped = true;
          offMsg();
          offState();
          stream.close();
          link.close();
          setHostLink((cur) => (cur === next ? null : cur));
        };
        setDeviceOffline(false);
        setHostLink((cur) => {
          cur?.stream.close();
          cur?.client.close();
          return next;
        });
      } catch (err) {
        link.close();
        throw err;
      }
    },
    [make, server.url, deviceListeners, storage, panelUrl],
  );

  const linkDevice = useCallback(
    async (code: string) => {
      if (!panelUrl) return openLink({ type: "claim", app: APP, code }, null);
      // The panel: the "code" is the panel token.
      const token = cleanPanelToken(code);
      await openLink({ type: "panel", app: APP, token }, null);
      try {
        storage?.setItem(PANEL_TOKEN_KEY, token);
      } catch {
        // storage blocked: the panel stays open for this page
      }
      setSavedLink(panelLink(storage, panelUrl) ?? { deviceId: "panel", linkId: "panel", token, signalUrl: panelUrl, savedAt: 0 });
    },
    [openLink, panelUrl, storage],
  );

  // Come back to the saved device without a code: after a reload, or when
  // the device restarts. The waits grow so the retries stay under the
  // server's rate limit (reach, claim and join share it).
  // The panel's link does not depend on the shared socket's state: without
  // this, every state change would start another link attempt in parallel
  // (and a stale token would use the panel's tries up several at a time).
  const linkGate = panelUrl ? "panel" : state;
  useEffect(() => {
    // The panel does not need signalhub: its link is the device's socket.
    if (demo || !savedLink || hostLink || (linkGate !== "panel" && linkGate !== "open")) return;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let wait = 3000;
    const attempt = () => {
      if (cancelled) return;
      const request: Envelope = panelUrl
        ? { type: "panel", app: APP, token: savedLink.token }
        : { type: "reach", app: APP, device_id: savedLink.deviceId };
      openLink(request, panelUrl ? null : savedLink).catch((err: unknown) => {
        if (cancelled) return;
        if (panelUrl && err instanceof Error && err.message === PANEL_BAD_TOKEN) {
          // The token changed on the device: ask for the new one.
          try {
            storage?.removeItem(PANEL_TOKEN_KEY);
          } catch {
            // storage blocked
          }
          setSavedLink(null);
          setLinkNotice("auth_failed");
          return;
        }
        setDeviceOffline(true);
        timer = setTimeout(attempt, wait);
        wait = Math.min(wait * 2, 30000);
      });
    };
    attempt();
    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
    };
  }, [demo, savedLink, hostLink, linkGate, openLink, panelUrl, storage]);

  const sendToDevice = useCallback((msg: unknown) => hostLink?.stream.sendControl(msg) ?? false, [hostLink]);

  const value = useMemo<SignalContextValue>(
    () => ({
      client,
      state,
      server,
      defaultUrl,
      setCustomServer,
      resetToOfficialServer,
      hostLink,
      linkedDevice,
      sendToDevice,
      onDeviceMessage,
      linkDevice,
      unlinkDevice,
      savedLink,
      deviceOffline,
      linkNotice,
      clearLinkNotice,
      demo,
      panel: panelUrl !== "",
      testServer,
      serverDialogOpen,
      setServerDialogOpen,
    }),
    [client, state, server, defaultUrl, setCustomServer, resetToOfficialServer, hostLink, linkedDevice, sendToDevice, onDeviceMessage, linkDevice, unlinkDevice, savedLink, deviceOffline, linkNotice, clearLinkNotice, demo, panelUrl, testServer, serverDialogOpen],
  );

  return <SignalContext.Provider value={value}>{children}</SignalContext.Provider>;
}

export function useSignal(): SignalContextValue {
  const ctx = useContext(SignalContext);
  if (!ctx) throw new Error("useSignal must be used inside SignalProvider");
  return ctx;
}
