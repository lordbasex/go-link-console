// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { type ChatLine, type RoomStateView } from "@go-link/shared";
import { t } from "../i18n";
import { DEMO_MESSAGES, DEMO_QUEUE, DEMO_SPECTATORS, demoPlayers } from "../fixtures";

// One view model for the room screen, built either from the design's
// sample data (demo) or from the device's Room Manager (live).

export interface SeatCard {
  name: string;
  you: boolean;
  status: string;
  tone: "speaking" | "muted" | "normal";
  /** Live only: the listener silenced this player locally. */
  silenced?: boolean;
  /** The host's view only: the player's peer id. */
  peer?: string;
}

/** Live voice state used to decorate the seats. */
export interface VoiceView {
  /** Ports whose voice is loud right now (includes your own). */
  speaking: ReadonlySet<number>;
  /** Ports you silenced. */
  silenced: ReadonlySet<number>;
  /** Your microphone is sending. */
  talking: boolean;
}

export interface QueueRow {
  pos: string;
  name: string;
  you: boolean;
  note: string;
  /** The host's view only: the person's peer id. */
  peer?: string;
}

/** Someone watching; peer only in the host's view. */
export interface SpectatorRow {
  name: string;
  you: boolean;
  peer?: string;
  hand?: boolean;
}

/** A raised hand, for the host: oldest first. */
export interface HandRow {
  peer: string;
  name: string;
  at: number;
}

/** The device's notices come in English; the ones it marks are translated. */
/** The device names guests "Guest 9F3A": shown in the reader's language. */
export function localName(name: string): string {
  if (name === "The host") return t.pauseAsk.theHost; // the linked browser's pauses
  const m = /^Guest ([0-9A-F]{1,8})$/.exec(name);
  return m ? t.room.guestName(m[1]!) : name;
}

function systemText(c: ChatLine & { kind: "system" }): string {
  const a = c.args;
  const n = localName(a?.name ?? "");
  const n2 = localName(a?.name2 ?? "");
  const p = a?.port ?? 0;
  const p2 = a?.port2 ?? 0;
  const e = t.room.chatEvents;
  switch (c.event) {
    case "recording_started":
      return t.rec.started;
    case "recording_stopped":
      return t.rec.stopped;
    case "game_paused":
      // name asked for the pause and name2 (the host) agreed.
      if (a?.name2 === "The host") return t.pauseAsk.grantedByHost(n);
      return n2 ? t.pauseAsk.granted(n, n2) : e.paused(n);
    case "game_resumed":
      return e.resumed(n);
    case "now_watching":
      return e.watching(n);
    case "moved":
      return e.moved(n, p);
    case "swap_asked":
      return e.swapAsked(n, p, n2, p2);
    case "kept_seat":
      return e.kept(n, p);
    case "swapped":
      return e.swapped(n, p, n2, p2);
    case "left_seat":
      return e.left(n, p);
    case "seat_free":
      return e.free(p);
    case "took_seat":
      return e.took(n, p);
    case "sent_to_queue":
      return e.sentToQueue(n, n2, p);
    case "sent_to_watch":
      return e.sentToWatch(n, n2, p);
    case "let_queue":
      return e.letQueue(n, n2);
    case "play_now":
      return e.playNow(n, n2, p);
    case "first_in_queue":
      return e.firstInQueue(n);
    case "removed":
      return e.removed(n, n2);
    case "hand_later":
      return t.hand.later;
    case "pause_declined":
      return t.pauseAsk.declined;
    default:
      return c.text;
  }
}

export type ChatRow = { system: string } | { name: string; port: number | null; role: string; text: string; you: boolean };

export type Me =
  | { kind: "player"; ports: number[] }
  | { kind: "queue"; position: number }
  /** held: the host moved you to watching (you raise a hand to play); hand: yours is up. */
  | { kind: "spectator"; held?: boolean; hand?: boolean }
  | { kind: "unknown" };

export interface RoomModel {
  seats: (SeatCard | null)[];
  queue: QueueRow[];
  spectators: SpectatorRow[];
  /** The host's view: raised hands, oldest first. */
  hands: HandRow[];
  chat: ChatRow[];
  me: Me;
  /** "off" when the host created the room without voice. */
  voice: "player" | "spectator" | "pending" | "off";
  hearVoice: boolean;
}

export function demoModel(isPlayer: boolean, micOn: boolean, hearVoice: boolean): RoomModel {
  const you = isPlayer ? "Alex" : "Pedro";
  return {
    seats: demoPlayers(isPlayer, micOn).map((p) => ({
      name: p.name,
      you: p.you,
      status: p.speaking ? t.room.speaking : p.muted ? t.room.muted : t.room.micOn,
      tone: p.speaking ? "speaking" : p.muted ? "muted" : "normal",
    })),
    queue: DEMO_QUEUE.map((q, i) => ({ pos: q.pos, name: q.name, you: i === 0 && !isPlayer, note: q.note })),
    spectators: DEMO_SPECTATORS.map((name) => ({ name, you: false })),
    hands: [],
    chat: DEMO_MESSAGES.map((m) => ("system" in m ? { system: m.system } : { name: m.name, port: m.port, role: m.role, text: m.text, you: m.name === you })),
    me: isPlayer ? { kind: "player", ports: [1] } : { kind: "queue", position: 1 },
    voice: isPlayer ? "player" : "spectator",
    hearVoice,
  };
}

export function liveModel(room: RoomStateView | null, chat: ChatLine[], voice: VoiceView): RoomModel {
  const seats: (SeatCard | null)[] = room
    ? room.seats.map((s, i) => {
        if (!s) return null;
        const port = i + 1;
        const silenced = voice.silenced.has(port);
        const speaking = voice.speaking.has(port) && !silenced;
        const micOff = s.you && !voice.talking;
        return {
          name: localName(s.name),
          you: s.you,
          silenced,
          status: speaking ? t.room.speaking : silenced ? t.room.silenced : micOff ? t.room.muted : t.room.seatPlaying,
          tone: speaking ? "speaking" : silenced || micOff ? "muted" : "normal",
          ...(s.peer ? { peer: s.peer } : {}),
        };
      })
    : [null, null, null, null];
  let me: Me = { kind: "unknown" };
  if (room) {
    if (room.you.ports.length > 0) me = { kind: "player", ports: room.you.ports };
    else if (room.you.queuePositions.length > 0) me = { kind: "queue", position: Math.min(...room.you.queuePositions) };
    else me = { kind: "spectator", held: room.you.held, hand: room.you.hand };
  }
  return {
    seats,
    queue: (room?.queue ?? []).map((q) => ({
      pos: t.ordinal(q.position),
      name: localName(q.name),
      you: q.you,
      note: q.position === 1 ? t.room.queueNext : t.room.queueWaiting,
      ...(q.peer ? { peer: q.peer } : {}),
    })),
    spectators: (room?.spectators ?? []).map((sp) => ({
      name: localName(sp.name),
      you: sp.you,
      ...(sp.peer ? { peer: sp.peer } : {}),
      ...(sp.hand ? { hand: true } : {}),
    })),
    hands: (room?.spectators ?? [])
      .flatMap((sp) => (sp.hand && sp.peer ? [{ peer: sp.peer, name: localName(sp.name), at: sp.handAt ?? 0 }] : []))
      .sort((a, b) => a.at - b.at),
    chat: chat.map((c) => (c.kind === "system" ? { system: systemText(c) } : { name: localName(c.name), port: c.port, role: c.role, text: c.text, you: !!room && c.name === room.you.name })),
    me,
    voice: room?.voice === false ? "off" : me.kind === "player" ? "player" : "spectator",
    hearVoice: false,
  };
}
