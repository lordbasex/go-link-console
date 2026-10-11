// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useEffect, useRef, useState, type RefObject } from "react";
import {
  DEFAULT_INPUT,
  EMPTY_PAD,
  HostStream,
  MAX_LOCAL_PLAYERS,
  gamepadName,
  parseChat,
  parseRoomState,
  parseTyping,
  type TypingView,
  readGamepad,
  forgetGamepad,
  gamepadSlot,
  keyboardButtons,
  padBits,
  padMapFor,
  bindingOf,
  type InputConfig,
  type ChatLine,
  type RoomStateView,
  samePad,
  type Envelope,
  type Pad,
  type PinEvent,
  type StreamState,
  type StreamStats,
  type StreamVideo,
  type StreamWindow,
  parseStreamVideo,
  parseHudBeacons,
  type HudRect,
} from "@go-link/shared";
import { useSignal } from "./SignalProvider";

export interface ControllerInfo {
  player: number;
  /** Stable key of this pad (model + position) for its player setting. */
  slot: string;
  /** The player comes from the automatic order, not a setting. */
  auto: boolean;
  name: string;
  /** Raw browser id, used to tell the button layout (Nintendo, Xbox...). */
  id: string;
  pad: Pad;
}

/** When a local player last started or stopped pressing anything. */
export interface PressEdge {
  active: boolean;
  /** performance.now() of the change. */
  at: number;
}

export interface HostStreamView {
  /** Where each seat's beacon is while the host draws the controllers. */
  hud: HudRect[];
  /** The last press or release of each local player (latency test). */
  pressEdges: RefObject<PressEdge[]>;
  /** Keyboard and every gamepad play as one player (InputOptions.multi). */
  single: boolean;
  media: MediaStream | null;
  state: StreamState | "idle";
  stats: StreamStats;
  /** Frames per second the device reports sending. */
  sentFps: number | null;
  /** Display aspect ratio of the picture (e.g. 4/3), from the device. */
  aspect: number | null;
  /** The picture the device sends: its scale (2: the game enlarged 2x), the game's size and quality. */
  video: StreamVideo | null;
  /** Physical controllers in use, with their local player number. */
  controllers: ControllerInfo[];
  /** Keyboard keys (KeyboardEvent.code) held right now. */
  heldKeys: ReadonlySet<string>;
  /** Seats, queue and spectators from the device's Room Manager. */
  room: RoomStateView | null;
  chat: ChatLine[];
  controlOpen: boolean;
  sendChat: (text: string) => void;
  /** Tells the device the size the video is shown at (device pixels): go-link HD sends the picture that fills it. */
  setVideoWant: (width: number, height: number) => void;
  /** Pauses or resumes the game for everyone (the host only: the device refuses anyone else). */
  setPaused: (paused: boolean) => void;
  /** Asks the host for a pause, or withdraws the request (cancel). */
  requestPause: (cancel?: boolean) => void;
  /** The host answers a player's request for a pause (from: its peer id). */
  answerPause: (from: string, accept: boolean) => void;
  /** The last request the device refused (its code), with a counter so a repeat shows again. */
  refused: { code: string; n: number } | null;
  spectate: () => void;
  joinQueue: () => void;
  /** Moves your seat from port from to port to (a taken one asks first). */
  swapSeat: (from: number, to: number) => void;
  /** The host frees a seat: its player to the end of the queue or to watching. */
  releaseSeat: (port: number, mode: "queue" | "watch") => void;
  /** Answers a request to swap controllers. */
  answerSwap: (from: number, to: number, accept: boolean) => void;
  /** A spectator raises (or lowers) a hand to ask the host to play. */
  raiseHand: (up: boolean) => void;
  /** The host: someone watching to the queue, or anyone straight to a seat ("play"). */
  promote: (peer: string, mode: "queue" | "play", port?: number) => void;
  /** The host's "not now" to a raised hand. */
  handLater: (peer: string) => void;
  /** The host takes someone out of the room for good. */
  removeGuest: (peer: string) => void;
  /** The four voice tracks (1-4): each carries whoever speaks now (room_state.voice_slots). */
  voiceStreams: Record<number, MediaStream>;
  /** How loud each voice track is now, 0 to 1 (from WebRTC, no Web Audio). */
  voiceLevels: Record<number, number>;
  /** Sends (or stops, with null) the microphone. */
  setMicTrack: (track: MediaStreamTrack | null) => void;
  /** Buttons held on the on-screen gamepad; they drive the keyboard's player. */
  setTouchButtons: (buttons: number) => void;
  /** Other people typing a chat message right now. */
  typing: TypingView[];
  /** Tells the room you are typing (repeat while typing) or stopped. */
  sendTyping: (on: boolean) => void;
  /** A private room waiting for its PIN. */
  pin: PinView;
  sendPin: (pin: string) => void;
  /** Sends the host's key or a return token instead of a PIN. */
  sendToken: (token: string) => void;
  /** Sends a group invitation's key with the guest's name (the host may have to OK it). */
  sendKey: (key: string, name: string) => void;
}

/** The PIN a private room asks for before it streams. */
export interface PinView {
  needed: boolean;
  /** A PIN was sent and the answer has not come yet. */
  busy: boolean;
  /** The last wrong answer. */
  last: Extract<PinEvent, { kind: "result" }> | null;
  /** The token the device gave on the way in, to come back without a PIN. */
  token: string;
}

const NO_PIN: PinView = { needed: false, busy: false, last: null, token: "" };

const MAX_CHAT_LINES = 200;

/**
 * Connects to the host's device over WebRTC while hostPeerId is set and
 * forwards input. Each gamepad drives the local player of its index (first
 * pad = player 0). The keyboard drives keyboardPlayer; when it matches a
 * pad, both control the same player.
 */
export interface InputOptions {
  input?: InputConfig;
  /** A gamepad button mapped to "pause" was pressed. */
  onPauseButton?: () => void;
  /** While the remap screen listens, nothing is sent to the game. */
  suspended?: boolean;
  /**
   * The host wants several controllers to take seats of their own. Without
   * it (and always for guests, whom the device seats once) every gamepad
   * drives the keyboard's player, so a pad plugged in by mistake never
   * takes a new seat.
   */
  multi?: boolean;
  /** The latency test's measurement, sent with the telemetry report. */
  probe?: { median: number; last: number } | null;
}

export function useHostStream(
  hostPeerId: string | null,
  takeBacklog?: () => Envelope[],
  keyboardPlayer = 0,
  playerName = "",
  opts: InputOptions = {},
): HostStreamView {
  const inputRef = useRef<InputConfig>(opts.input ?? DEFAULT_INPUT);
  inputRef.current = opts.input ?? DEFAULT_INPUT;
  const pauseRef = useRef(opts.onPauseButton);
  pauseRef.current = opts.onPauseButton;
  const probeRef = useRef<InputOptions["probe"]>(null);
  probeRef.current = opts.probe ?? null;
  const suspendRef = useRef(false);
  suspendRef.current = opts.suspended ?? false;
  const singleRef = useRef(true);
  const { client } = useSignal();
  const [media, setMedia] = useState<MediaStream | null>(null);
  const [state, setState] = useState<StreamState | "idle">("idle");
  const [stats, setStats] = useState<StreamStats>({
    fps: null,
    rttMs: null,
    path: null,
  });
  const [sentFps, setSentFps] = useState<number | null>(null);
  const [aspect, setAspect] = useState<number | null>(null);
  const [video, setVideo] = useState<StreamVideo | null>(null);
  const [hud, setHud] = useState<HudRect[]>([]);
  const [voiceLevels, setVoiceLevels] = useState<Record<number, number>>({});
  const pressEdges = useRef<PressEdge[]>(
    Array.from({ length: MAX_LOCAL_PLAYERS }, () => ({ active: false, at: 0 })),
  );
  const [controllers, setControllers] = useState<ControllerInfo[]>([]);
  const [heldKeys, setHeldKeys] = useState<ReadonlySet<string>>(new Set());
  const [room, setRoom] = useState<RoomStateView | null>(null);
  const [chat, setChat] = useState<ChatLine[]>([]);
  const [controlOpen, setControlOpen] = useState(false);
  const [pin, setPin] = useState<PinView>(NO_PIN);
  const [typing, setTyping] = useState<TypingView[]>([]);
  const [refused, setRefused] = useState<{ code: string; n: number } | null>(null);
  const [voiceStreams, setVoiceStreams] = useState<Record<number, MediaStream>>(
    {},
  );
  // Only the device says who the host is (room_state.you.owner).
  const single = !(opts.multi && room?.you.owner);
  singleRef.current = single;
  const streamRef = useRef<HostStream | null>(null);
  // The voices' levels, a few times a second while there are voices.
  const hasVoices = Object.keys(voiceStreams).length > 0;
  useEffect(() => {
    if (!hasVoices) return;
    let last = "";
    const timer = setInterval(() => {
      const next = streamRef.current?.voiceLevels() ?? {};
      // Re-render only when a level moves enough to show.
      const key = Object.entries(next).map(([k, v]) => `${k}:${Math.round(v * 30)}`).join(",");
      if (key !== last) {
        last = key;
        setVoiceLevels(next);
      }
    }, 120);
    return () => clearInterval(timer);
  }, [hasVoices]);
  const touchRef = useRef(0);
  const flushRef = useRef<(() => void) | null>(null);

  useEffect(() => {
    if (!hostPeerId) return;
    setPin(NO_PIN);
    const stream = new HostStream({
      client,
      hostPeerId,
      onPin: (ev) =>
        setPin(
          ev.kind === "required"
            ? { ...NO_PIN, needed: true }
            : ev.ok
              ? { ...NO_PIN, token: ev.token }
              : { needed: true, busy: false, last: ev, token: "" },
        ),
      onTrack: setMedia,
      onState: setState,
      onControlOpen: () => setControlOpen(true),
      onVoiceTrack: (port, voice) =>
        setVoiceStreams((cur) => ({ ...cur, [port]: voice })),
      onControl: (msg) => {
        const m = msg as { type?: unknown; fps?: unknown; aspect?: unknown };
        if (m.type === "stream_stats" && typeof m.fps === "number" && m.fps > 0)
          setSentFps(m.fps);
        if (m.type === "stream_stats") {
          const beacons = parseHudBeacons(msg);
          if (beacons)
            setHud((cur) => (JSON.stringify(cur) === JSON.stringify(beacons) ? cur : beacons));
          const v = parseStreamVideo(msg);
          if (v) setVideo((cur) => (cur && sameVideo(cur, v) ? cur : v));
        }
        if (
          m.type === "stream_stats" &&
          typeof m.aspect === "number" &&
          m.aspect > 0.2 &&
          m.aspect < 5
        )
          setAspect(m.aspect);
        const state = parseRoomState(msg);
        if (state) setRoom(state);
        const line = parseChat(msg);
        if (line) setChat((cur) => [...cur, line].slice(-MAX_CHAT_LINES));
        const who = parseTyping(msg);
        if (who) setTyping(who);
        const e = msg as { type?: unknown; code?: unknown };
        if (e.type === "error" && typeof e.code === "string")
          setRefused((cur) => ({ code: String(e.code).slice(0, 40), n: (cur?.n ?? 0) + 1 }));
      },
    });
    streamRef.current = stream;
    stream.start(takeBacklog?.() ?? []);
    // Every two seconds the room's telemetry gets what this browser saw:
    // lost packets, freezes, dropped frames, jitter, its round trip and
    // the latency test's measurement (client_report).
    let acc: StreamWindow[] = [];
    const timer = setInterval(() => {
      stream
        .stats()
        .then((st) => {
          setStats(st);
          if (!st.window) return;
          acc.push(st.window);
          if (acc.length < 2) return;
          const w = acc;
          acc = [];
          const sum = (k: keyof StreamWindow) => w.reduce((n, x) => n + x[k], 0);
          const top = (k: keyof StreamWindow) => Math.max(...w.map((x) => x[k]));
          const probe = probeRef.current;
          const report: Record<string, unknown> = {
            type: "client_report",
            video_loss_pct: top("videoLossPct"),
            audio_loss_pct: top("audioLossPct"),
            freeze_ms: sum("freezeMs"),
            freezes: sum("freezes"),
            dropped: sum("dropped"),
            jitter_ms: top("jitterMs"),
            kbps: Math.round(sum("kbps") / w.length),
            hidden: document.hidden ? 1 : 0,
          };
          if (st.rttMs != null) report.rtt_ms = st.rttMs;
          if (st.fps != null) report.fps = st.fps;
          if (st.videoBufferMs != null) report.buffer_ms = st.videoBufferMs;
          if (st.decodeMs != null) report.decode_ms = st.decodeMs;
          if (probe) {
            report.e2e_ms = probe.median;
            report.e2e_last_ms = probe.last;
          }
          stream.sendControl(report);
        })
        .catch(() => undefined);
    }, 1000);
    return () => {
      clearInterval(timer);
      stream.close();
      streamRef.current = null;
      setMedia(null);
      setState("idle");
      setSentFps(null);
      setVideo(null);
      setHud([]);
      setRoom(null);
      setChat([]);
      setTyping([]);
      setControlOpen(false);
      setVoiceStreams({});
    };
    // takeBacklog belongs to the same join as hostPeerId.
  }, [client, hostPeerId]);

  useEffect(() => {
    if (!hostPeerId) return;
    const kbPlayer = Math.min(
      Math.max(keyboardPlayer, 0),
      MAX_LOCAL_PLAYERS - 1,
    );
    const held = new Set<string>();
    let keyboard = 0;
    const last: Pad[] = Array.from(
      { length: MAX_LOCAL_PLAYERS },
      () => EMPTY_PAD,
    );
    let lastNames = "";
    const pauseHeld = new Set<string>(); // slots whose pause button is down

    // Merge keyboard and gamepads into one pad per local player and send
    // whatever changed.
    const flush = () => {
      const input = inputRef.current;
      const pads: Pad[] = Array.from({ length: MAX_LOCAL_PLAYERS }, () => ({
        buttons: 0,
        axes: [0, 0, 0, 0] as Pad["axes"],
      }));
      pads[kbPlayer]!.buttons |= keyboard | touchRef.current;
      const found: ControllerInfo[] = [];
      const gamepads =
        typeof navigator.getGamepads === "function"
          ? navigator.getGamepads()
          : [];
      const seen = new Map<string, number>();
      for (const gp of gamepads) {
        if (!gp || !gp.connected) continue;
        const nth = seen.get(gp.id) ?? 0;
        seen.set(gp.id, nth + 1);
        const slot = gamepadSlot(gp.id, nth);
        const chosen = input.players[slot];
        const player = singleRef.current ? kbPlayer : (chosen ?? gp.index);
        if (player >= MAX_LOCAL_PLAYERS) continue;
        const map = padMapFor(input, gp.id);
        const pad = readGamepad(gp, padBits(map));
        const target = pads[player]!;
        target.buttons |= pad.buttons;
        if (pad.axes.some((a) => a !== 0) || target.axes.every((a) => a === 0))
          target.axes = pad.axes;
        found.push({
          player,
          slot,
          auto: singleRef.current || chosen === undefined,
          name: gamepadName(gp),
          id: gp.id,
          pad,
        });
        // Pause on the press, not while held.
        const pauseIndex = bindingOf(map, "pause");
        const down =
          pauseIndex !== undefined && !!gp.buttons[Number(pauseIndex)]?.pressed;
        if (down && !pauseHeld.has(slot) && !suspendRef.current)
          pauseRef.current?.();
        if (down) pauseHeld.add(slot);
        else pauseHeld.delete(slot);
      }
      if (suspendRef.current)
        pads.forEach((p) => ((p.buttons = 0), (p.axes = [0, 0, 0, 0])));
      const now = performance.now();
      pads.forEach((pad, player) => {
        // Pressing anything (as the device's beacon shows it) starts the
        // clock of the latency test.
        const active = pad.buttons !== 0 || pad.axes.some((a) => a >= 64 || a <= -64);
        const edge = pressEdges.current[player]!;
        if (edge.active !== active) pressEdges.current[player] = { active, at: now };
        if (!samePad(pad, last[player]!)) {
          last[player] = pad;
          streamRef.current?.setPad(player, pad);
        }
      });
      const names = JSON.stringify(found);
      if (names !== lastNames) {
        lastNames = names;
        setControllers(found);
      }
    };

    flushRef.current = flush;
    // Gamepads have no events for button changes: poll every frame.
    let frame = 0;
    const loop = () => {
      flush();
      frame = requestAnimationFrame(loop);
    };
    frame = requestAnimationFrame(loop);

    const typing = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      return (
        !!el &&
        (el.tagName === "INPUT" ||
          el.tagName === "TEXTAREA" ||
          el.isContentEditable)
      );
    };
    const recompute = () => {
      keyboard = 0;
      keyboard = keyboardButtons(held, inputRef.current.keyboard);
      setHeldKeys(new Set(held));
      flush();
    };
    const down = (e: KeyboardEvent) => {
      if (typing(e) || e.metaKey || e.ctrlKey || e.altKey) return;
      if (e.code in inputRef.current.keyboard) e.preventDefault(); // arrows must not scroll the page
      if (!held.has(e.code)) {
        held.add(e.code); // unmapped keys too, so the on-screen keyboard shows them
        recompute();
      }
    };
    const up = (e: KeyboardEvent) => {
      if (held.delete(e.code)) recompute();
    };
    const release = () => {
      held.clear();
      recompute();
    };
    window.addEventListener("keydown", down);
    window.addEventListener("keyup", up);
    window.addEventListener("blur", release);
    // A pad plugged in again gets a fresh rest state.
    const unplugged = (e: GamepadEvent) => forgetGamepad(e.gamepad);
    window.addEventListener("gamepaddisconnected", unplugged);
    return () => {
      flushRef.current = null;
      window.removeEventListener("gamepaddisconnected", unplugged);
      cancelAnimationFrame(frame);
      window.removeEventListener("keydown", down);
      window.removeEventListener("keyup", up);
      window.removeEventListener("blur", release);
      // Release everything this layout was sending before a reassignment.
      last.forEach((pad, player) => {
        if (!samePad(pad, EMPTY_PAD))
          streamRef.current?.setPad(player, EMPTY_PAD);
      });
    };
  }, [hostPeerId, keyboardPlayer]);

  // Tell the Room Manager who we are and which local players want seats:
  // the keyboard's player plus one per connected gamepad (only the
  // keyboard's in single mode, where every pad drives it).
  const localPlayers = [
    ...new Set([
      keyboardPlayer,
      ...(single ? [] : controllers.map((c) => c.player)),
    ]),
  ]
    .sort()
    .join(",");
  useEffect(() => {
    if (!hostPeerId) return;
    streamRef.current?.sendControl({
      type: "hello",
      name: playerName,
      local_players: localPlayers.split(",").map(Number),
    });
  }, [hostPeerId, playerName, localPlayers]);

  const send = (msg: unknown) => streamRef.current?.sendControl(msg);
  return {
    hud,
    voiceLevels,
    pressEdges,
    single,
    media,
    state,
    stats,
    sentFps,
    aspect,
    video,
    controllers,
    heldKeys,
    room,
    chat,
    controlOpen,
    sendChat: (text) => send({ type: "chat", text }),
    setVideoWant: (width, height) => send({ type: "video_want", width, height }),
    setPaused: (paused) => send({ type: "pause", paused }),
    requestPause: (cancel) => send(cancel ? { type: "pause_request", cancel: true } : { type: "pause_request" }),
    answerPause: (from, accept) => send({ type: "pause_answer", from, accept }),
    refused,
    spectate: () => send({ type: "spectate" }),
    joinQueue: () => send({ type: "queue" }),
    swapSeat: (from, to) => send({ type: "swap_seat", from, to }),
    releaseSeat: (port, mode) => send({ type: "release_seat", port, mode }),
    answerSwap: (from, to, accept) => send({ type: "swap_answer", from, to, accept }),
    raiseHand: (up) => send({ type: "hand", on: up }),
    promote: (peer, mode, port) => send(mode === "play" ? { type: "promote", from: peer, mode, port } : { type: "promote", from: peer, mode }),
    handLater: (peer) => send({ type: "hand_later", from: peer }),
    removeGuest: (peer) => send({ type: "remove_guest", from: peer }),
    voiceStreams,
    setMicTrack: (track) => streamRef.current?.setMicTrack(track),
    typing,
    sendTyping: (on) => send({ type: "typing", on }),
    pin,
    sendPin: (value) => {
      setPin((cur) => ({ ...cur, busy: true }));
      streamRef.current?.sendPin(value);
    },
    sendToken: (value) => {
      setPin((cur) => ({ ...cur, busy: true }));
      streamRef.current?.sendToken(value);
    },
    sendKey: (key, name) => {
      setPin((cur) => ({ ...cur, busy: true }));
      streamRef.current?.sendKey(key, name);
    },
    setTouchButtons: (buttons) => {
      if (touchRef.current === buttons) return;
      touchRef.current = buttons;
      flushRef.current?.(); // send now, not on the next frame
    },
  };
}

function sameVideo(a: StreamVideo, b: StreamVideo): boolean {
  return a.scale === b.scale && a.width === b.width && a.height === b.height && a.quality === b.quality && a.fallback === b.fallback;
}
