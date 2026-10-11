// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Recordings of game rooms, only for the host: what the device reports on
// the linked "control" channel, and the chunked download on the "files"
// channel (the browser pulls the file piece by piece).

import { Sha256 } from "./hmac";

/** Why a recording stopped. */
export type RecordingReason = "stopped" | "paused" | "limit_time" | "limit_size" | "room_stopped" | "device_stopped" | "interrupted";
const REASONS: readonly RecordingReason[] = ["stopped", "paused", "limit_time", "limit_size", "room_stopped", "device_stopped", "interrupted"];

/** A track of a recording: the game's picture and sound, and each player's voice. */
export type RecordingTrack = "video" | "game" | "voice-p1" | "voice-p2" | "voice-p3" | "voice-p4";
const TRACKS: readonly RecordingTrack[] = ["video", "game", "voice-p1", "voice-p2", "voice-p3", "voice-p4"];

/** A recording stops by itself at 2 hours or 2 GiB, whichever comes first. */
export const RECORDING_MAX_MS = 2 * 60 * 60 * 1000;
export const RECORDING_MAX_BYTES = 2 * 1024 ** 3;

/** One finished recording, kept on the device (~/go-link-console/rec). */
export interface RecordingInfo {
  /** "<room id>/<file>": what download and delete_recording name. */
  id: string;
  roomId: string;
  /** The room's name when it was recorded. */
  room: string;
  /** File name, e.g. night-2026-09-28-2130.webm. */
  file: string;
  startedAt: string;
  durationMs: number;
  size: number;
  /** Hex SHA-256, to check a download. */
  sha256: string;
  tracks: RecordingTrack[];
  reason: RecordingReason;
}

const str = (v: unknown, max = 200): string => (typeof v === "string" ? v.slice(0, max) : "");
const num = (v: unknown): number => (typeof v === "number" && Number.isFinite(v) && v > 0 ? Math.floor(v) : 0);
const obj = (v: unknown): Record<string, unknown> => (typeof v === "object" && v !== null ? (v as Record<string, unknown>) : {});

const RECORDING_ID = /^[0-9a-f]{1,64}\/[a-z0-9][a-z0-9-]{0,80}\.webm$/;

/** A recording from the device, or null when it is not one. */
export function parseRecordingInfo(v: unknown): RecordingInfo | null {
  const o = obj(v);
  const id = str(o.id, 160);
  if (!RECORDING_ID.test(id)) return null;
  return {
    id,
    roomId: str(o.room_id, 64),
    room: str(o.room, 80),
    file: str(o.file, 90),
    startedAt: str(o.started_at, 40),
    durationMs: num(o.duration_ms),
    size: num(o.size),
    sha256: typeof o.sha256 === "string" && /^[0-9a-f]{64}$/.test(o.sha256) ? o.sha256 : "",
    tracks: (Array.isArray(o.tracks) ? o.tracks : []).filter((t): t is RecordingTrack => TRACKS.includes(t as RecordingTrack)),
    reason: REASONS.find((r) => r === o.reason) ?? "stopped",
  };
}

export function parseRecordingList(v: unknown): RecordingInfo[] {
  return (Array.isArray(v) ? v : []).slice(0, 1000).flatMap((r) => {
    const info = parseRecordingInfo(r);
    return info ? [info] : [];
  });
}

/** The device's "recordings" reply (get_recordings, delete_recording). */
export interface RecordingsReply {
  items: RecordingInfo[];
  /** Space the recordings take on the device. */
  bytes: number;
  error?: string;
  code?: string;
}

export function parseRecordings(msg: unknown): RecordingsReply | null {
  const m = obj(msg);
  if (m.type !== "recordings") return null;
  const reply: RecordingsReply = { items: parseRecordingList(m.items), bytes: num(m.bytes) };
  if (typeof m.error === "string") reply.error = str(m.error);
  if (typeof m.code === "string") reply.code = str(m.code, 40);
  return reply;
}

/** A recording ended: saved (offer to download it) or lost. */
export type RecordingEvent =
  | { type: "recording_saved"; room: string; reason: RecordingReason; recording: RecordingInfo }
  | { type: "recording_error"; room: string; reason: RecordingReason; error: string };

export function parseRecordingEvent(msg: unknown): RecordingEvent | null {
  const m = obj(msg);
  const room = str(m.room, 64);
  const reason = REASONS.find((r) => r === m.reason) ?? "stopped";
  if (m.type === "recording_saved") {
    const recording = parseRecordingInfo(m.recording);
    return recording ? { type: "recording_saved", room, reason, recording } : null;
  }
  if (m.type === "recording_error") return { type: "recording_error", room, reason, error: str(m.error) };
  return null;
}

/** Room actions for recording (room_action on the linked control channel). */
export const recordStart = (id: string) => ({ type: "room_action", id, action: "record_start" }) as const;
export const recordStop = (id: string) => ({ type: "room_action", id, action: "record_stop" }) as const;

/**
 * The factory reset message: the device deletes rooms, saved games,
 * history, recordings, links and settings. Ask the person first.
 */
export const FACTORY_RESET = { type: "factory_reset", confirm: "factory_reset" } as const;

export function parseFactoryReset(msg: unknown): { ok: boolean; error: string } | null {
  const m = obj(msg);
  if (m.type !== "factory_reset_result") return null;
  return { ok: m.ok === true, error: str(m.error) };
}

/** The largest piece the device sends (it cuts bigger reads). */
export const DOWNLOAD_CHUNK = 60 * 1024;

/** Progress of a download. */
export interface DownloadProgress {
  received: number;
  size: number;
  /** Bytes per second over the last seconds. */
  rate: number;
}

export type DownloadResult =
  | { ok: true; name: string; size: number; sha256: string; parts: Uint8Array[] }
  | { ok: false; cancelled: boolean; error: string; code?: string };

export interface DownloadOptions {
  /** Sends a text message on the files channel. */
  send: (text: string) => void;
  onProgress?: (p: DownloadProgress) => void;
  /** Reads in flight at once (default 8). */
  window?: number;
  chunk?: number;
  now?: () => number;
}

/**
 * Downloads one recording over the files channel. Feed it every message
 * of that channel with handleText and handleBinary (they return false for
 * messages that are someone else's, like upload replies). The pieces come
 * back in the order asked, so the file is hashed as it arrives and checked
 * against the device's SHA-256 at the end.
 */
export class RecordingDownload {
  readonly id: string;
  readonly result: Promise<DownloadResult>;
  private resolve!: (r: DownloadResult) => void;
  private opts: Required<Omit<DownloadOptions, "onProgress">> & Pick<DownloadOptions, "onProgress">;
  private size = 0;
  private name = "";
  private expected = "";
  private next = 0; // next offset to ask for
  private received = 0;
  private inFlight = 0;
  private parts: Uint8Array[] = [];
  private hash = new Sha256();
  private done = false;
  private rateMark = { at: 0, bytes: 0, rate: 0 };

  constructor(
    readonly file: string,
    opts: DownloadOptions,
  ) {
    this.id = `d${Math.random().toString(36).slice(2, 10)}`;
    this.opts = { window: 8, chunk: DOWNLOAD_CHUNK, now: () => Date.now(), ...opts };
    this.result = new Promise((r) => (this.resolve = r));
  }

  start(): void {
    this.rateMark.at = this.opts.now();
    this.opts.send(JSON.stringify({ type: "download", id: this.id, file: this.file }));
  }

  /** Stops the download: what arrived is dropped. */
  cancel(): void {
    if (this.done) return;
    this.opts.send(JSON.stringify({ type: "cancel", id: this.id }));
    this.finish({ ok: false, cancelled: true, error: "cancelled" });
  }

  handleText(text: string): boolean {
    let m: Record<string, unknown>;
    try {
      m = obj(JSON.parse(text));
    } catch {
      return false;
    }
    if (m.id !== this.id || (m.type !== "download_ready" && m.type !== "download_error")) return false;
    if (this.done) return true;
    if (m.type === "download_error") {
      this.finish({ ok: false, cancelled: false, error: str(m.error), code: typeof m.code === "string" ? str(m.code, 40) : undefined });
      return true;
    }
    this.size = num(m.size);
    this.name = str(m.name, 90);
    this.expected = str(m.sha256, 64);
    if (this.size === 0) {
      this.complete();
      return true;
    }
    this.pump();
    return true;
  }

  /** A piece: 8 bytes of offset (big endian), then the data. */
  handleBinary(data: ArrayBuffer | Uint8Array): boolean {
    if (this.done || this.size === 0) return false;
    const bytes = data instanceof Uint8Array ? data : new Uint8Array(data);
    if (bytes.length < 8) return false;
    const view = new DataView(bytes.buffer, bytes.byteOffset, 8);
    const offset = view.getUint32(0) * 2 ** 32 + view.getUint32(4);
    if (offset !== this.received) {
      this.opts.send(JSON.stringify({ type: "cancel", id: this.id }));
      this.finish({ ok: false, cancelled: false, error: "a piece arrived out of order" });
      return true;
    }
    const piece = bytes.slice(8);
    this.parts.push(piece);
    this.hash.update(piece);
    this.received += piece.length;
    this.inFlight--;
    this.progress();
    if (this.received >= this.size) this.complete();
    else this.pump();
    return true;
  }

  private pump(): void {
    while (!this.done && this.inFlight < this.opts.window && this.next < this.size) {
      const length = Math.min(this.opts.chunk, this.size - this.next);
      this.opts.send(JSON.stringify({ type: "read", id: this.id, offset: this.next, length }));
      this.next += length;
      this.inFlight++;
    }
  }

  private progress(): void {
    const now = this.opts.now();
    const mark = this.rateMark;
    if (now - mark.at >= 1000) {
      mark.rate = ((this.received - mark.bytes) * 1000) / (now - mark.at);
      mark.at = now;
      mark.bytes = this.received;
    }
    this.opts.onProgress?.({ received: this.received, size: this.size, rate: mark.rate });
  }

  private complete(): void {
    this.opts.send(JSON.stringify({ type: "done", id: this.id }));
    const sum = Array.from(this.hash.digest(), (b) => b.toString(16).padStart(2, "0")).join("");
    if (this.expected && sum !== this.expected) {
      this.finish({ ok: false, cancelled: false, error: "the file arrived damaged (its SHA-256 does not match)", code: "checksum" });
      return;
    }
    this.finish({ ok: true, name: this.name, size: this.received, sha256: sum, parts: this.parts });
  }

  private finish(r: DownloadResult): void {
    if (this.done) return;
    this.done = true;
    if (!r.ok) this.parts = [];
    this.resolve(r);
  }
}
