// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Wire format of the signalhub protocol v1. The protocol document in
// signalhub's README.md (its own repository) is the contract; keep these types in sync with it.

export const PROTOCOL_VERSION = "1";

/** signalhub namespace of this project. */
export const APP = "go-link-console";

export type MessageType =
  | "hello"
  | "register"
  | "code"
  | "claim"
  | "paired"
  | "reach"
  | "reached"
  /** Only the device's local web panel: {type: "panel"} asks for a nonce, {type: "panel", proof} logs in. */
  | "panel"
  | "panel_nonce"
  | "room_open"
  | "room_opened"
  | "room_update"
  | "room_close"
  | "invite_create"
  | "invite_created"
  | "rooms_list"
  | "rooms"
  | "join"
  | "joined"
  | "peer_joined"
  | "signal"
  | "peer_left"
  | "error";

/** Same shape as the browser's RTCIceServer. */
export interface ICEServer {
  urls: string[];
  username?: string;
  credential?: string;
}

export interface RoomInfo {
  room_id: string;
  meta?: unknown;
}

export interface Envelope {
  type: MessageType;
  app?: string;
  device_id?: string;
  code?: string;
  room_id?: string;
  public?: boolean;
  /** room_open: guests may join only through the room's invitation. */
  invite_only?: boolean;
  /** join / invite_created: the invitation of a room (links and QR codes). */
  invite?: string;
  meta?: unknown;
  rooms?: RoomInfo[];
  session_id?: string;
  peer_id?: string;
  remote?: string;
  to?: string;
  from?: string;
  ice_servers?: ICEServer[];
  payload?: unknown;
  error?: string;
  /** panel: the device's panel token (a UUID v4); kept in the browser, never sent. */
  token?: string;
  /** panel_nonce: the panel's one-time challenge. */
  nonce?: string;
  /** panel: HMAC of the nonce with the panel token. */
  proof?: string;
  /** panel: "room" for a socket that plays in the device's rooms (no data link). */
  mode?: "room";
}

/** Fixed error texts sent by signalhub (see "Errores" in the protocol). */
export const ServerError = {
  InvalidCode: "invalid or expired code",
  AppNotAllowed: "app not allowed",
  InvalidDeviceID: "invalid device_id",
  AlreadyInSession: "peer already belongs to a session",
  CodeUnavailable: "could not allocate a pairing code",
  NotInSession: "not in a session",
  TargetForbidden: "target not allowed",
  PeerNotConnected: "peer not connected",
  NotOwner: "only the session owner can manage rooms",
  RoomLimit: "room limit reached",
  RoomNotFound: "room not found",
  DeviceOffline: "device not connected",
  InvalidMeta: "invalid meta",
  InvalidInvite: "invalid or expired invite",
  RateLimited: "rate limit exceeded",
  InvalidMessage: "invalid message",
  UnsupportedType: "unsupported message type",
} as const;

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** Reports whether s looks like a room_id (canonical UUID text). */
export function isRoomId(s: string): boolean {
  return UUID.test(s);
}

/**
 * Extracts a room_id from what a user pasted: a bare ID or any link that
 * ends in /r/<room_id>. Returns null when nothing valid is found.
 */
export function parseRoomInput(input: string): string | null {
  const text = input.trim();
  if (isRoomId(text)) return text.toLowerCase();
  const match = /\/r\/([0-9a-f-]{36})(?:[/?#]|$)/i.exec(text);
  return match?.[1] && isRoomId(match[1]) ? match[1].toLowerCase() : null;
}

const INVITE = /^[A-Za-z0-9_-]{22}$/;

/** What an invitation link or typed code points at (see join in the protocol). */
export type InviteTarget = { invite: string } | { code: string };

/**
 * Reads an invitation: the 22 character invite of a /g/<invite> link, or a
 * 9 digit code (spaces and dashes allowed). Returns null otherwise.
 */
export function parseInvite(input: string): InviteTarget | null {
  const text = input.trim();
  const link = /\/g\/([A-Za-z0-9_-]{22})(?:[/?#]|$)/.exec(text);
  if (link?.[1]) return { invite: link[1] };
  if (INVITE.test(text)) return { invite: text };
  const digits = text.replace(/[\s.-]/g, "");
  return /^\d{9}$/.test(digits) ? { code: digits } : null;
}
