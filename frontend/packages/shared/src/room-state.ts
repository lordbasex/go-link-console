// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Room state and chat sent by the device's Room Manager on the "control"
// DataChannel. It comes from the host, but names and chat are typed by
// other guests, so everything is validated and rendered as text.

import { parseRoomPicture, type PictureSettings } from "./picture";

export interface SeatView {
  port: number;
  name: string;
  localPlayer: number;
  you: boolean;
  /** The player's peer id: sent only to the host's own browsers. */
  peer?: string;
}

/** Someone in the queue. peer is sent only to the host's own browsers. */
export interface QueueView {
  position: number;
  name: string;
  you: boolean;
  peer?: string;
}

/** Someone watching. */
export interface SpectatorView {
  name: string;
  you: boolean;
  /** The host's own browsers only: what promote, hand_later and remove_guest name. */
  peer?: string;
  /** A raised hand: this spectator asks the host to play. */
  hand?: boolean;
  /** The host's own browsers only: when the hand went up (ms since the epoch). */
  handAt?: number;
}

export interface RoomStateView {
  maxPlayers: number;
  /** Voice between players is enabled in this room. */
  voice: boolean;
  /** The room's chat is on (the host can turn it off). */
  chat: boolean;
  /** The host draws the controllers on the video to measure latency. */
  inputHud: boolean;
  /** The room's name, game, host and picture, sent to guests after they get in. */
  info: RoomInfoView;
  /** Index 0 is P1; null means a free seat. */
  seats: (SeatView | null)[];
  queue: QueueView[];
  spectators: SpectatorView[];
  you: {
    name: string;
    ports: number[];
    queuePositions: number[];
    spectator: boolean;
    /** Requests to swap controllers waiting for your answer. */
    swapOffers: SwapView[];
    /** Your requests waiting for someone else's answer. */
    swapAsked: SwapView[];
    /** This browser is the host's own (it came in with the owner key). */
    owner: boolean;
    /** Owners only: players asking the host for a pause. */
    pauseAsks: PauseAskView[];
    /** Your own request for a pause, while it waits, or null. */
    pauseAsked: { expiresAt: number } | null;
    /** Your hand is up (a spectator asking the host to play). */
    hand: boolean;
    /** When you may raise your hand again (ms since the epoch), or 0. */
    handAgain: number;
    /** The host moved you to watching: you raise a hand instead of joining the queue alone. */
    held: boolean;
  };
  /** A game is running: the host may pause it (the others ask). */
  pausable: boolean;
  /** The host can answer a request for a pause (it is in the room or at its device page). */
  hostOnline: boolean;
  paused: boolean;
  pausedBy: string;
  /** The running game's control panel, for the on-screen gamepad. */
  controls: GameControls;
  /** The host is recording the game, with the players' voices: show REC. */
  recording?: boolean;
  /** The host's default picture style for the room; null: the site's default. */
  picture?: PictureSettings | null;
}

/** What the device tells guests about the room. */
export interface RoomInfoView {
  title: string;
  game: string;
  host: string;
  /** A data: URL of a small JPEG, or null. */
  art: string | null;
  /** The game is one go-link made itself, verified by the device. */
  own?: boolean;
}

/** A seated player asking the host for a pause. */
export interface PauseAskView {
  /** The player's peer id: what pause_answer names. */
  from: string;
  name: string;
  port: number;
  /** When the request expires (ms since the epoch). */
  expiresAt: number;
}

/** A request to swap controllers: from port asks to move to port to. */
export interface SwapView {
  from: number;
  to: number;
  /** The other player. */
  name: string;
}

export interface GameControls {
  /** Players the game takes at once, 1 to 4 (0 = unknown). */
  players: number;
  /** Action buttons per player, 0 to 6. */
  buttons: number;
  /** MAME control type: joy4way, joy8way, stick, dial... ("" = unknown). */
  control: string;
  /** What each button does, button 1 first (go-link's own games only). */
  labels?: string[];
}

/** What an unknown game (or an older device) offers. */
export const DEFAULT_CONTROLS: GameControls = { players: 0, buttons: 6, control: "joy8way" };

function parseControls(v: unknown): GameControls {
  if (typeof v !== "object" || v === null) return DEFAULT_CONTROLS;
  const o = v as Record<string, unknown>;
  const buttons = typeof o.buttons === "number" && Number.isFinite(o.buttons) ? Math.min(Math.max(Math.round(o.buttons), 0), 6) : 6;
  const players = typeof o.players === "number" && Number.isFinite(o.players) ? Math.min(Math.max(Math.round(o.players), 0), 4) : 0;
  const controls: GameControls = { players, buttons, control: typeof o.control === "string" ? o.control.slice(0, 20) : "" };
  if (Array.isArray(o.labels)) controls.labels = o.labels.filter((l): l is string => typeof l === "string").slice(0, 6).map((l) => l.slice(0, 20));
  return controls;
}

/** System chat lines the web shows in the reader's language. */
export type ChatEvent =
  | "recording_started"
  | "recording_stopped"
  | "game_paused"
  | "game_resumed"
  | "now_watching"
  | "moved"
  | "swap_asked"
  | "kept_seat"
  | "swapped"
  | "left_seat"
  | "seat_free"
  | "took_seat"
  | "sent_to_queue"
  | "sent_to_watch"
  | "let_queue"
  | "play_now"
  | "first_in_queue"
  | "removed"
  | "pause_declined"
  | "hand_later";
const CHAT_EVENTS: readonly ChatEvent[] = [
  "recording_started",
  "recording_stopped",
  "game_paused",
  "game_resumed",
  "now_watching",
  "moved",
  "swap_asked",
  "kept_seat",
  "swapped",
  "left_seat",
  "seat_free",
  "took_seat",
  "sent_to_queue",
  "sent_to_watch",
  "let_queue",
  "play_now",
  "first_in_queue",
  "removed",
  "pause_declined",
  "hand_later",
];

/** The values of a chat event (who and which seat). */
export interface ChatArgs {
  name: string;
  port: number;
  name2: string;
  port2: number;
}

export type ChatLine =
  | { kind: "system"; text: string; ts: number; event?: ChatEvent; args?: ChatArgs }
  | { kind: "user"; name: string; port: number | null; role: string; text: string; ts: number };

const str = (v: unknown, max = 300): string => (typeof v === "string" ? v.slice(0, max) : "");
const num = (v: unknown): number => (typeof v === "number" && Number.isFinite(v) ? v : 0);
const obj = (v: unknown): Record<string, unknown> => (typeof v === "object" && v !== null ? (v as Record<string, unknown>) : {});
const arr = (v: unknown): unknown[] => (Array.isArray(v) ? v : []);

function parseInfo(v: unknown): RoomInfoView {
  const o = obj(v);
  const art = typeof o.art === "string" && o.art.length <= 6000 && /^[A-Za-z0-9+/]+={0,2}$/.test(o.art) ? `data:image/jpeg;base64,${o.art}` : null;
  const info: RoomInfoView = { title: str(o.title, 80), game: str(o.game, 160), host: str(o.host, 60), art };
  if (o.own === true) info.own = true;
  return info;
}

function parseSwaps(v: unknown, maxPlayers: number): SwapView[] {
  const port = (x: unknown) => {
    const n = num(x);
    return Number.isInteger(n) && n >= 1 && n <= maxPlayers ? n : 0;
  };
  return arr(v)
    .slice(0, 8)
    .flatMap((x) => {
      const o = obj(x);
      const from = port(o.from);
      const to = port(o.to);
      return from && to && from !== to ? [{ from, to, name: str(o.name, 40) }] : [];
    });
}

const time = (v: unknown): number => {
  const t = typeof v === "string" ? Date.parse(v) : NaN;
  return Number.isFinite(t) ? t : 0;
};

/** Reads pause_asks (device_status rooms, room_state.you, pause_asked). */
export function parsePauseAsks(v: unknown): PauseAskView[] {
  return arr(v)
    .slice(0, 8)
    .flatMap((x) => {
      const o = obj(x);
      const from = str(o.from, 80);
      const port = num(o.port);
      const expiresAt = time(o.expires_at);
      return from && expiresAt ? [{ from, name: str(o.name, 40), port: port >= 1 && port <= 4 ? port : 0, expiresAt }] : [];
    });
}

export function parseRoomState(msg: unknown): RoomStateView | null {
  const m = obj(msg);
  if (m.type !== "room_state") return null;
  const maxPlayers = Math.min(Math.max(num(m.max_players), 1), 4);
  const seats: (SeatView | null)[] = Array.from({ length: maxPlayers }, () => null);
  arr(m.seats).forEach((s, i) => {
    if (i >= maxPlayers || s === null) return;
    const o = obj(s);
    const seat: SeatView = { port: i + 1, name: str(o.name, 40), localPlayer: num(o.local_player), you: o.you === true };
    const peer = str(o.peer, 80);
    if (peer) seat.peer = peer;
    seats[i] = seat;
  });
  const you = obj(m.you);
  return {
    maxPlayers,
    voice: m.voice !== false,
    chat: m.chat !== false,
    inputHud: m.input_hud === true,
    info: parseInfo(m.info),
    seats,
    queue: arr(m.queue).slice(0, 64).map((q) => {
      const o = obj(q);
      const row: QueueView = { position: num(o.position), name: str(o.name, 40), you: o.you === true };
      const peer = str(o.peer, 80);
      if (peer) row.peer = peer;
      return row;
    }),
    spectators: arr(m.spectators).slice(0, 64).map((p) => {
      const o = obj(p);
      const row: SpectatorView = { name: str(o.name, 40), you: o.you === true };
      const peer = str(o.peer, 80);
      if (peer) row.peer = peer;
      if (o.hand === true) row.hand = true;
      const at = time(o.hand_at);
      if (at) row.handAt = at;
      return row;
    }),
    you: {
      name: str(you.name, 40),
      ports: arr(you.ports).slice(0, 4).map(num),
      queuePositions: arr(you.queue_positions).slice(0, 4).map(num),
      spectator: you.spectator === true,
      swapOffers: parseSwaps(you.swap_offers, maxPlayers),
      swapAsked: parseSwaps(you.swap_asked, maxPlayers),
      owner: you.owner === true,
      pauseAsks: parsePauseAsks(you.pause_asks),
      pauseAsked: time(obj(you.pause_asked).expires_at) ? { expiresAt: time(obj(you.pause_asked).expires_at) } : null,
      hand: you.hand === true,
      handAgain: time(you.hand_again),
      held: you.held === true,
    },
    pausable: m.pausable === true,
    hostOnline: m.host_online === true,
    paused: m.paused === true,
    pausedBy: str(m.paused_by, 40),
    controls: parseControls(m.controls),
    recording: m.recording === true,
    picture: parseRoomPicture(m.picture),
  };
}

export function parseChat(msg: unknown): ChatLine | null {
  const m = obj(msg);
  if (m.type !== "chat") return null;
  const ts = num(m.ts);
  if (typeof m.system === "string") {
    const event = CHAT_EVENTS.find((e) => e === m.event);
    if (!event) return { kind: "system", text: str(m.system), ts };
    if (m.args === undefined) return { kind: "system", text: str(m.system), ts, event };
    const a = obj(m.args);
    const seat = (v: unknown) => (Number.isInteger(num(v)) && num(v) >= 1 && num(v) <= 4 ? num(v) : 0);
    const args: ChatArgs = { name: str(a.name, 60), port: seat(a.port), name2: str(a.name2, 60), port2: seat(a.port2) };
    return { kind: "system", text: str(m.system), ts, event, args };
  }
  if (typeof m.text !== "string" || typeof m.name !== "string") return null;
  const port = num(m.port);
  return { kind: "user", name: str(m.name, 40), port: port >= 1 && port <= 4 ? port : null, role: str(m.role, 20), text: str(m.text), ts };
}

/** Someone else typing a chat message ("typing" from the device). */
export interface TypingView {
  name: string;
  port: number | null;
}

/** Returns who is typing when msg is a "typing" message. */
export function parseTyping(msg: unknown): TypingView[] | null {
  const m = obj(msg);
  if (m.type !== "typing") return null;
  return arr(m.names)
    .slice(0, 20)
    .map((x) => {
      const o = obj(x);
      const port = num(o.port);
      return { name: str(o.name, 40), port: port >= 1 && port <= 4 ? port : null };
    })
    .filter((x) => x.name !== "");
}

/** 1 -> "1st", 2 -> "2nd", 11 -> "11th"... */
export function ordinal(n: number): string {
  const tens = n % 100;
  if (tens >= 11 && tens <= 13) return `${n}th`;
  const suffix = ["th", "st", "nd", "rd"][n % 10] ?? "th";
  return `${n}${suffix}`;
}
