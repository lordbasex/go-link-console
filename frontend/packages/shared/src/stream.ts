// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// WebRTC link between a browser and the host's device. The device offers;
// the browser answers. Negotiation messages travel inside signalhub
// "signal" payloads (RTCSignal), which signalhub never reads.

import type { SignalClient } from "./signal-client";
import type { Envelope } from "./protocol";
import { ownerKeyOf } from "./device-status";
import { RecordingDownload, type DownloadOptions } from "./recordings";

export interface RTCSignal {
  kind: "offer" | "answer" | "candidate";
  sdp?: string;
  candidate?: RTCIceCandidateInit;
}

/** Button bits of the "input" DataChannel. Keep in sync with backend-device/pkg/input. */
export const Button = {
  Up: 1 << 0,
  Down: 1 << 1,
  Left: 1 << 2,
  Right: 1 << 3,
  B1: 1 << 4, // bottom face button (Switch: B)
  B2: 1 << 5, // right (Switch: A)
  B3: 1 << 6, // left (Switch: Y)
  B4: 1 << 7, // top (Switch: X)
  B5: 1 << 8, // L1
  B6: 1 << 9, // R1
  Start: 1 << 10,
  Coin: 1 << 11,
  L2: 1 << 12,
  R2: 1 << 13,
  L3: 1 << 14,
  R3: 1 << 15,
  Home: 1 << 16,
  Capture: 1 << 17,
  // The start buttons of players 1 to 4, like the row of start buttons on
  // an arcade panel: any seated player can press any of them.
  Start1: 1 << 18,
  Start2: 1 << 19,
  Start3: 1 << 20,
  Start4: 1 << 21,
} as const;

/** All defined button bits. */
export const BUTTON_MASK = (1 << 22) - 1;

/** The panel start button of a port (1-4), or 0. */
export function startOf(port: number): number {
  return port >= 1 && port <= 4 ? Button.Start1 << (port - 1) : 0;
}

/**
 * How many panel start buttons to show: as many as people seated, but no
 * more than the game takes (players 0 = unknown, up to 4), and at least 1.
 */
export function startButtonCount(gamePlayers: number, seated: number): number {
  const game = gamePlayers >= 1 ? Math.min(gamePlayers, 4) : 4;
  return Math.max(1, Math.min(game, seated));
}

/** Up to 4 people can play from one browser. */
export const MAX_LOCAL_PLAYERS = 4;

/** One controller: button bits and sticks (LX, LY, RX, RY in -127..127). */
export interface Pad {
  buttons: number;
  axes: [number, number, number, number];
}

export const EMPTY_PAD: Pad = { buttons: 0, axes: [0, 0, 0, 0] };

export function padIsIdle(p: Pad): boolean {
  return p.buttons === 0 && p.axes.every((a) => a === 0);
}

export function samePad(a: Pad, b: Pad): boolean {
  return a.buttons === b.buttons && a.axes.every((v, i) => v === b.axes[i]);
}

/**
 * 12 bytes, big endian: uint16 sequence, uint8 local player, uint8
 * reserved, uint32 buttons, 4 x int8 stick axes.
 */
export function encodeInput(seq: number, player: number, pad: Pad): ArrayBuffer {
  const buf = new ArrayBuffer(12);
  const view = new DataView(buf);
  view.setUint16(0, seq & 0xffff);
  view.setUint8(2, player & 0x03);
  view.setUint32(4, (pad.buttons & BUTTON_MASK) >>> 0);
  pad.axes.forEach((a, i) => view.setInt8(8 + i, Math.max(-127, Math.min(127, Math.round(a)))));
  return buf;
}

/** "video/H264" -> "H.264", "video/VP8" -> "VP8". */
export function codecName(mime: string): string {
  const name = mime.replace(/^video\//i, "");
  return /^h264$/i.test(name) ? "H.264" : /^h265$/i.test(name) ? "H.265" : name.toUpperCase();
}

export interface StreamStats {
  /** Frames per second being decoded. */
  fps: number | null;
  /** Round-trip time of the active ICE pair, in milliseconds. */
  rttMs: number | null;
  /** How the media flows: straight to the device, or through the TURN relay. */
  path: "direct" | "relay" | null;
  /** The video codec in use ("H.264", "VP8"), from the inbound stream's codec. */
  codec?: string | null;
  /** How long the frames of the last second waited in the jitter buffer (ms). */
  videoBufferMs?: number | null;
  /** How long the browser took to decode each of them (ms). */
  decodeMs?: number | null;
  /** Since the last call: video packets lost (%), frozen time and freezes,
   * frames dropped, jitter, kilobits per second received, and the game's
   * and players' audio packets lost (%). For the room's telemetry. */
  window?: StreamWindow;
}

export interface StreamWindow {
  videoLossPct: number;
  freezeMs: number;
  freezes: number;
  dropped: number;
  jitterMs: number;
  kbps: number;
  audioLossPct: number;
  seconds: number;
}

export type StreamState = "connecting" | "connected" | "failed" | "closed";

/**
 * A private room asks for a PIN before the device streams anything:
 * "required" first, then one "result" per PIN or token sent. Every
 * invitation has its own PIN, good for one person: the first browser in
 * gets a token to come back without a PIN. reason is "wrong" (left tries
 * remain), "used" (someone already came in with that invitation),
 * "blocked" (no tries left) or "locked" (too many wrong PINs in the
 * room: wait retryAfter seconds). A group invitation's key also gets
 * "waiting" (the host decides), "declined", "full", "expired" or "busy".
 */
export type PinEvent =
  | { kind: "required" }
  | { kind: "result"; ok: boolean; token: string; reason: string; left: number; retryAfter: number };

export interface HostStreamOptions {
  client: SignalClient;
  hostPeerId: string;
  onTrack?: (stream: MediaStream) => void;
  onState?: (state: StreamState) => void;
  onControl?: (message: unknown) => void;
  /** Voice of the player at a port (1-4), relayed by the device. */
  onVoiceTrack?: (port: number, stream: MediaStream) => void;
  /** Runs when the control channel opens. */
  onControlOpen?: () => void;
  /** The room asks for its PIN, or answers one. */
  onPin?: (event: PinEvent) => void;
  /** Injectable for tests. */
  RTCPeerConnectionImpl?: typeof RTCPeerConnection;
}

/**
 * Receives the device's media and DataChannels. It waits for the device's
 * offer; a later offer (renegotiation) reuses the same connection.
 */
export class HostStream {
  private pc: RTCPeerConnection | null = null;
  private pending: RTCIceCandidateInit[] = [];
  /** The receiver of each voice track (1-4), for its level. */
  private voiceReceivers = new Map<number, RTCRtpReceiver>();
  private input: RTCDataChannel | null = null;
  private players = new Map<number, { seq: number; pad: Pad; repeat: ReturnType<typeof setInterval> | null }>();
  private offMessage: (() => void) | null = null;
  private closed = false;
  private micSender: RTCRtpSender | null = null;
  private micTrack: MediaStreamTrack | null = null;

  constructor(private readonly opts: HostStreamOptions) {}

  /**
   * Starts listening for the device's offer. backlog holds messages that
   * arrived before start (the device offers as soon as someone joins, which
   * can be before the page is ready); they are replayed first.
   */
  start(backlog: Envelope[] = []): void {
    const accept = (env: Envelope) => {
      if (env.type !== "signal" || env.from !== this.opts.hostPeerId) return;
      void this.handleSignal(env.payload as RTCSignal);
    };
    this.offMessage = this.opts.client.onMessage(accept);
    backlog.forEach(accept);
  }

  /** Sends an invitation's PIN to the device (after a "required" PinEvent). */
  sendPin(pin: string): void {
    this.opts.client.send({ type: "signal", to: this.opts.hostPeerId, payload: { kind: "pin", pin } });
  }

  /**
   * Sends a group invitation's key and the guest's name (shown to the host
   * while it decides): the answer is ok, or "waiting" until the host does.
   */
  sendKey(key: string, name: string): void {
    this.opts.client.send({ type: "signal", to: this.opts.hostPeerId, payload: { kind: "pin", key, name: name.slice(0, 24) } });
  }

  /** Sends a token instead: the host's key, or the one got on the first way in. */
  sendToken(token: string): void {
    this.opts.client.send({ type: "signal", to: this.opts.hostPeerId, payload: { kind: "pin", token } });
  }

  close(): void {
    this.closed = true;
    this.offMessage?.();
    this.players.forEach((p) => p.repeat && clearInterval(p.repeat));
    this.players.clear();
    this.voiceReceivers.clear();
    this.pc?.close();
    this.pc = null;
    this.opts.onState?.("closed");
  }

  /**
   * Sets the controller state of one local player. Packets go out on
   * every change and are repeated every 100 ms while anything is held,
   * because the channel drops lost packets instead of retransmitting.
   */
  setPad(player: number, pad: Pad): void {
    let p = this.players.get(player);
    if (!p) {
      p = { seq: 0, pad: EMPTY_PAD, repeat: null };
      this.players.set(player, p);
    }
    const entry = p;
    const changed = !samePad(pad, entry.pad);
    entry.pad = { buttons: pad.buttons, axes: [...pad.axes] as Pad["axes"] };
    if (changed) this.sendInput(player);
    const idle = padIsIdle(entry.pad);
    if (!idle && !entry.repeat) {
      entry.repeat = setInterval(() => this.sendInput(player), 100);
    } else if (idle && entry.repeat) {
      clearInterval(entry.repeat);
      entry.repeat = null;
      // A couple of extra releases in case one is lost.
      setTimeout(() => this.sendInput(player), 50);
      setTimeout(() => this.sendInput(player), 150);
    }
  }

  /** Shortcut for keyboard-only use: buttons of local player 0. */
  setButtons(buttons: number): void {
    this.setPad(0, { buttons, axes: [0, 0, 0, 0] });
  }

  /**
   * Sends a JSON message on the reliable "control" channel. Messages sent
   * before the channel opens are kept (up to 20) and flushed on open.
   */
  sendControl(message: unknown): boolean {
    const text = JSON.stringify(message);
    const ch = this.control;
    if (!ch || ch.readyState !== "open") {
      if (this.pendingControl.length < 20) this.pendingControl.push(text);
      return false;
    }
    ch.send(text);
    return true;
  }

  private control: RTCDataChannel | null = null;
  private pendingControl: string[] = [];
  private files: RTCDataChannel | null = null;

  /** true once the device's "files" channel is open (linked owner only). */
  get canSendFiles(): boolean {
    return this.files?.readyState === "open";
  }

  /**
   * Sends a file to the device on the "files" DataChannel: a begin
   * message, 16 KB binary chunks, and an end message. It waits when more
   * than 4 MB are buffered so large ROMs do not flood the connection.
   * The device answers upload_result (with the same id) on control.
   * purpose "rom_test" sends a set for rom_test instead of the library
   * (see rom-test.ts), and "maker" the game Willy Maker made, for its own
   * room (maker-play.ts).
   */
  async sendFile(id: string, file: Blob, name: string, onProgress?: (sent: number, total: number) => void, purpose?: "rom_test" | "maker"): Promise<void> {
    const ch = this.files;
    if (!ch || ch.readyState !== "open") throw new Error("files channel not open");
    const CHUNK = 16 * 1024;
    const HIGH = 4 * 1024 * 1024;
    ch.bufferedAmountLowThreshold = 1024 * 1024;
    ch.send(JSON.stringify(purpose ? { type: "begin", id, name, size: file.size, purpose } : { type: "begin", id, name, size: file.size }));
    for (let offset = 0; offset < file.size; offset += CHUNK) {
      if (ch.bufferedAmount > HIGH) {
        await new Promise<void>((resolve) => {
          ch.onbufferedamountlow = () => {
            ch.onbufferedamountlow = null;
            resolve();
          };
        });
      }
      if (ch.readyState !== "open") throw new Error("connection closed");
      ch.send(await file.slice(offset, offset + CHUNK).arrayBuffer());
      onProgress?.(Math.min(offset + CHUNK, file.size), file.size);
    }
    ch.send(JSON.stringify({ type: "end", id }));
  }

  private downloads = new Set<RecordingDownload>();

  /**
   * Downloads a recording from the device on the "files" channel, in
   * pieces the browser asks for (see RecordingDownload). Await .result;
   * .cancel() stops it.
   */
  download(file: string, onProgress?: DownloadOptions["onProgress"]): RecordingDownload {
    const ch = this.files;
    if (!ch || ch.readyState !== "open") throw new Error("files channel not open");
    const d = new RecordingDownload(file, {
      send: (text) => {
        if (ch.readyState === "open") ch.send(text);
      },
      onProgress,
    });
    this.downloads.add(d);
    void d.result.then(() => this.downloads.delete(d));
    d.start();
    return d;
  }

  private videoTotals: { emitted: number; buffered: number; decoded: number; decoding: number } | null = null;
  private windowTotals: Record<string, number> | null = null;
  private windowAt = 0;

  /**
   * How loud each voice track (1-4) is right now, 0 to 1, from the level the
   * device forwards with the voice (RTCRtpReceiver synchronization sources):
   * no Web Audio, so it works before any tap and whatever plays the sound.
   * A track with no packet in the last half second is 0.
   */
  voiceLevels(): Record<number, number> {
    const out: Record<number, number> = {};
    for (const [slot, receiver] of this.voiceReceivers) {
      const src = receiver.getSynchronizationSources?.()[0];
      if (!src) continue;
      // The timestamp is on performance.now()'s clock (older browsers: the epoch).
      const now = src.timestamp > 1e12 ? Date.now() : performance.now();
      out[slot] = now - src.timestamp < 500 ? (src.audioLevel ?? 0) : 0;
    }
    return out;
  }

  async stats(): Promise<StreamStats> {
    const result: StreamStats = { fps: null, rttMs: null, path: null, codec: null };
    if (!this.pc) return result;
    const report = await this.pc.getStats();
    const byId = new Map<string, Record<string, unknown>>();
    let selected: Record<string, unknown> | undefined;
    let codecId: string | undefined;
    report.forEach((s: Record<string, unknown>) => {
      byId.set(String(s.id), s);
      if (s.type === "inbound-rtp" && s.kind === "video") {
        // Averages over the frames since the last call, not the whole call.
        const n = (k: string) => (typeof s[k] === "number" ? (s[k] as number) : null);
        const emitted = n("jitterBufferEmittedCount");
        const buffered = n("jitterBufferDelay");
        const decoded = n("framesDecoded");
        const decoding = n("totalDecodeTime");
        const last = this.videoTotals;
        if (emitted !== null && buffered !== null && last && emitted > last.emitted)
          result.videoBufferMs = Math.round(((buffered - last.buffered) / (emitted - last.emitted)) * 1000);
        if (decoded !== null && decoding !== null && last && decoded > last.decoded)
          result.decodeMs = Math.round(((decoding - last.decoding) / (decoded - last.decoded)) * 10000) / 10;
        if (emitted !== null && buffered !== null && decoded !== null && decoding !== null)
          this.videoTotals = { emitted, buffered, decoded, decoding };
      }
      if (s.type === "inbound-rtp" && s.kind === "video" && typeof s.framesPerSecond === "number") result.fps = s.framesPerSecond;
      if (s.type === "inbound-rtp" && s.kind === "video" && typeof s.codecId === "string") codecId = s.codecId;
      if (s.type === "candidate-pair" && s.nominated && typeof s.currentRoundTripTime === "number") result.rttMs = Math.round(s.currentRoundTripTime * 1000);
    });
    // The pair in use: the transport names it (Chrome, Safari), else the
    // nominated one that succeeded (Firefox).
    report.forEach((s: Record<string, unknown>) => {
      if (s.type === "transport" && typeof s.selectedCandidatePairId === "string") selected = byId.get(s.selectedCandidatePairId);
    });
    if (!selected) report.forEach((s: Record<string, unknown>) => {
      if (s.type === "candidate-pair" && s.nominated && s.state === "succeeded") selected = s;
    });
    // Totals of the inbound streams, for the window since the last call.
    const totals: Record<string, number> = { vLost: 0, vRecv: 0, freezes: 0, freezeS: 0, dropped: 0, bytes: 0, aLost: 0, aRecv: 0, jitter: 0 };
    report.forEach((s: Record<string, unknown>) => {
      if (s.type !== "inbound-rtp") return;
      const n = (k: string) => (typeof s[k] === "number" ? (s[k] as number) : 0);
      if (s.kind === "video") {
        totals.vLost! += n("packetsLost");
        totals.vRecv! += n("packetsReceived");
        totals.freezes! += n("freezeCount");
        totals.freezeS! += n("totalFreezesDuration");
        totals.dropped! += n("framesDropped");
        totals.bytes! += n("bytesReceived");
        totals.jitter = Math.max(totals.jitter!, n("jitter"));
      } else if (s.kind === "audio") {
        totals.aLost! += n("packetsLost");
        totals.aRecv! += n("packetsReceived");
        totals.bytes! += n("bytesReceived");
      }
    });
    const now = Date.now();
    const prev = this.windowTotals;
    if (prev && this.windowAt > 0 && now > this.windowAt) {
      const d = (k: string) => Math.max(0, totals[k]! - prev[k]!);
      const pct = (lost: number, recv: number) => (lost + recv > 0 ? Math.round((lost / (lost + recv)) * 1000) / 10 : 0);
      const seconds = (now - this.windowAt) / 1000;
      result.window = {
        videoLossPct: pct(d("vLost"), d("vRecv")),
        freezeMs: Math.round(d("freezeS") * 1000),
        freezes: d("freezes"),
        dropped: d("dropped"),
        jitterMs: Math.round(totals.jitter! * 10000) / 10,
        kbps: Math.round((d("bytes") * 8) / 1000 / seconds),
        audioLossPct: pct(d("aLost"), d("aRecv")),
        seconds,
      };
    }
    this.windowTotals = totals;
    this.windowAt = now;
    const mime = codecId ? byId.get(codecId)?.mimeType : undefined;
    if (typeof mime === "string") result.codec = codecName(mime);
    if (selected) {
      const kind = (id: unknown) => byId.get(String(id))?.candidateType;
      result.path = kind(selected.localCandidateId) === "relay" || kind(selected.remoteCandidateId) === "relay" ? "relay" : "direct";
    }
    return result;
  }

  /**
   * Sets the microphone track (null stops sending). It uses replaceTrack
   * on the line the device reserved for it, so no renegotiation happens.
   */
  setMicTrack(track: MediaStreamTrack | null): void {
    this.micTrack = track;
    void this.micSender?.replaceTrack(track).catch(() => undefined);
  }

  /** The device offers the microphone line as recvonly: answer sendonly. */
  private prepareMicrophone(pc: RTCPeerConnection, offer: string): void {
    const mid = micMid(offer);
    if (mid === null) return;
    const transceiver = pc.getTransceivers().find((t) => t.mid === mid);
    if (!transceiver) return;
    transceiver.direction = "sendonly";
    this.micSender = transceiver.sender;
    if (this.micTrack) void this.micSender.replaceTrack(this.micTrack).catch(() => undefined);
  }

  private sendInput(player: number): void {
    const p = this.players.get(player);
    if (!p || this.input?.readyState !== "open") return;
    p.seq = (p.seq + 1) & 0xffff;
    this.input.send(encodeInput(p.seq, player, p.pad));
  }

  private sendSignal(sig: RTCSignal): void {
    this.opts.client.send({ type: "signal", to: this.opts.hostPeerId, payload: sig });
  }

  private ensurePeerConnection(): RTCPeerConnection {
    if (this.pc) return this.pc;
    const Impl = this.opts.RTCPeerConnectionImpl ?? globalThis.RTCPeerConnection;
    // STUN/TURN come from signalhub's hello; nothing is hardcoded.
    const pc = new Impl({ iceServers: this.opts.client.iceServers });
    pc.onicecandidate = (ev) => {
      if (ev.candidate) this.sendSignal({ kind: "candidate", candidate: ev.candidate.toJSON() });
    };
    pc.ontrack = (ev) => {
      const stream = ev.streams[0] ?? new MediaStream([ev.track]);
      const voice = /^voice-p([1-4])$/.exec(stream.id);
      if (voice) {
        this.voiceReceivers.set(Number(voice[1]), ev.receiver);
        this.opts.onVoiceTrack?.(Number(voice[1]), stream);
      }
      else this.opts.onTrack?.(stream);
    };
    pc.ondatachannel = (ev) => {
      const ch = ev.channel;
      if (ch.label === "input") {
        ch.binaryType = "arraybuffer";
        this.input = ch;
      } else if (ch.label === "files") {
        ch.binaryType = "arraybuffer";
        this.files = ch;
        // Download replies and pieces go to the download they belong to.
        ch.onmessage = (m) => {
          for (const d of this.downloads) {
            const mine = typeof m.data === "string" ? d.handleText(m.data) : d.handleBinary(m.data as ArrayBuffer);
            if (mine) break;
          }
        };
      } else if (ch.label === "control") {
        this.control = ch;
        const flush = () => {
          for (const text of this.pendingControl.splice(0)) ch.send(text);
          this.opts.onControlOpen?.();
        };
        if (ch.readyState === "open") flush();
        else ch.onopen = flush;
        ch.onmessage = (m) => {
          let msg: unknown;
          try {
            msg = JSON.parse(String(m.data));
          } catch {
            return; // ignore malformed control messages
          }
          // The device measures the peer-to-peer round trip with pings.
          if (typeof msg !== "object" || msg === null) return;
          const ping = msg as { type?: unknown; id?: unknown };
          if (ping.type === "ping" && ch.readyState === "open") {
            ch.send(JSON.stringify({ type: "pong", id: ping.id }));
            return;
          }
          this.opts.onControl?.(msg);
        };
      }
    };
    pc.onconnectionstatechange = () => {
      const s = pc.connectionState;
      if (s === "connected") this.opts.onState?.("connected");
      else if (s === "failed") this.opts.onState?.("failed");
    };
    this.pc = pc;
    this.opts.onState?.("connecting");
    return pc;
  }

  private async handleSignal(sig: RTCSignal | undefined): Promise<void> {
    if (!sig || this.closed) return;
    const pin = sig as unknown as { kind?: string; ok?: unknown; token?: unknown; reason?: unknown; left?: unknown; retry_after?: unknown };
    if (pin.kind === "pin_required") {
      this.opts.onPin?.({ kind: "required" });
      return;
    }
    if (pin.kind === "pin_result") {
      const num = (v: unknown) => (typeof v === "number" && Number.isFinite(v) ? Math.max(0, Math.round(v)) : 0);
      this.opts.onPin?.({
        kind: "result",
        ok: pin.ok === true,
        token: ownerKeyOf(pin.token),
        reason: typeof pin.reason === "string" ? pin.reason.slice(0, 20) : "",
        left: num(pin.left),
        retryAfter: num(pin.retry_after),
      });
      return;
    }
    try {
      if (sig.kind === "offer" && sig.sdp) {
        const pc = this.ensurePeerConnection();
        await pc.setRemoteDescription({ type: "offer", sdp: sig.sdp });
        this.prepareMicrophone(pc, sig.sdp);
        for (const c of this.pending) await pc.addIceCandidate(c);
        this.pending = [];
        const answer = await pc.createAnswer();
        await pc.setLocalDescription(answer);
        this.sendSignal({ kind: "answer", sdp: answer.sdp ?? "" });
      } else if (sig.kind === "candidate" && sig.candidate) {
        if (this.pc?.remoteDescription) await this.pc.addIceCandidate(sig.candidate);
        // Before the answer a device sends a handful, never hundreds.
        else if (this.pending.length < 50) this.pending.push(sig.candidate);
      }
    } catch {
      this.opts.onState?.("failed");
    }
  }
}

/** mid of the audio m-line the device offers as recvonly (the microphone). */
export function micMid(sdp: string): string | null {
  for (const section of sdp.split(/\r?\nm=/).slice(1)) {
    if (!section.startsWith("audio") || !/\na=recvonly/.test(section)) continue;
    const mid = /\na=mid:([^\r\n]+)/.exec(section);
    if (mid?.[1]) return mid[1];
  }
  return null;
}

/**
 * Default keyboard layout, MAME style: 1 to 4 are the start buttons of
 * players 1 to 4, Enter is your own start and 5 inserts a coin.
 */
export const DEFAULT_KEYMAP: Record<string, number> = {
  ArrowUp: Button.Up,
  ArrowDown: Button.Down,
  ArrowLeft: Button.Left,
  ArrowRight: Button.Right,
  KeyZ: Button.B1,
  KeyX: Button.B2,
  KeyC: Button.B3,
  KeyA: Button.B4,
  KeyS: Button.B5,
  KeyD: Button.B6,
  KeyQ: Button.L2,
  KeyW: Button.R2,
  Digit1: Button.Start1,
  Digit2: Button.Start2,
  Digit3: Button.Start3,
  Digit4: Button.Start4,
  Enter: Button.Start,
  Digit5: Button.Coin,
};
