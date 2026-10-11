// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Status a device sends to browsers linked with the pairing code, on the
// WebRTC "control" DataChannel (type "device_status").

import { parseRecordingList, type RecordingInfo } from "./recordings";
import { parsePauseAsks, type PauseAskView } from "./room-state";
import { parseRoomPicture, type PictureSettings } from "./picture";
import { parseRoomVideo, parseVideoQuality, type RoomVideo, type VideoQuality } from "./video";

export interface DeviceHardware {
  /** The computer's name (older devices: absent). */
  hostname?: string;
  os: string;
  arch: string;
  platform: string;
  cpu_model: string;
  cores: number;
  mem_total: number;
}

export interface DeviceUsage {
  cpu_percent: number;
  process_cpu_percent: number;
  mem_used: number;
  process_rss: number;
  /** Whole-machine network traffic, bytes per second (older devices: absent). */
  net_sent_bps?: number;
  net_recv_bps?: number;
}

export interface DeviceRom {
  name: string;
  size: number;
  title?: string;
  year?: string;
  maker?: string;
  /** Which of the host's thumbnails exist for the set. */
  thumbs: { boxart: boolean; title: boolean; snap: boolean };
  /** Whether the core can run the set; absent until the core's game list is on the device. */
  check?: RomCheck;
  /**
   * A game go-link made itself: the device matched the SHA-256 of every
   * file inside the zip (never its name), so title, year, maker,
   * description and controls are go-link's, not the original set's.
   */
  own?: boolean;
  /**
   * "glhd" for a go-link HD game package (name.glhd), played by go-link
   * HD's own core; its title, description and controls come from the
   * package's manifest. Absent for a MAME set (name.zip).
   */
  kind?: "glhd";
  description?: string;
  controls?: RomControls;
}

/** The game's file in the host's folder: name.zip, or name.glhd for a go-link HD package. */
export function romFile(r: { name: string; kind?: string }): string {
  return r.name + (r.kind === "glhd" ? ".glhd" : ".zip");
}

/** A game's control panel as the library shows it (go-link's own games). */
export interface RomControls {
  players: number;
  buttons: number;
  /** What each button does, button 1 first, in English ("Jump", "Fire", "Special"). */
  labels: string[];
}

function parseRomControls(v: unknown): RomControls | undefined {
  if (typeof v !== "object" || v === null) return undefined;
  const c = v as Record<string, unknown>;
  const count = (x: unknown, hi: number) => (typeof x === "number" && Number.isFinite(x) ? Math.min(Math.max(Math.round(x), 0), hi) : 0);
  const labels = Array.isArray(c.labels) ? c.labels.filter((l): l is string => typeof l === "string").slice(0, 6).map((l) => l.slice(0, 20)) : [];
  return { players: count(c.players, 4), buttons: count(c.buttons, 6), labels };
}

/** Verdict of the device's ROM check, made without running the game. */
export type RomCheckStatus = "ok" | "missing" | "unsupported" | "bios" | "bad_zip";

export interface RomCheck {
  status: RomCheckStatus;
  /** Up to 10 missing file names. */
  missing?: string[];
  /** Parent or BIOS sets (zips) that are not in the folder, e.g. "neogeo". */
  needs?: string[];
  /** "preliminary" or "protection" when MAME's driver is incomplete. */
  driver?: string;
}

const CHECK_STATUSES: readonly RomCheckStatus[] = ["ok", "missing", "unsupported", "bios", "bad_zip"];

/** A set the core can run: checked ok, or not checked yet. */
export function romPlayable(r: DeviceRom): boolean {
  return !r.check || r.check.status === "ok";
}

/** The states of a set, as the device groups them (status order). */
export type RomKind = "runs" | "missing" | "unsupported" | "broken" | "bios" | "unchecked";
export const ROM_KINDS: readonly RomKind[] = ["runs", "missing", "unsupported", "broken", "bios", "unchecked"];

/**
 * The ROM folder in numbers. device_status never carries the sets
 * themselves (a full MAME collection is thousands, far more than one
 * WebRTC message holds): ask for pages with roms_query and for sets by
 * name with roms_get (romsQuery, romsGet).
 */
export interface LibrarySummary {
  /** Changes with every scan of the folder: pages asked before are stale. */
  revision: number;
  total: number;
  bytes: number;
  /** Sets a room can be opened with (check passed, or no check yet). */
  playable: number;
  kinds: Record<RomKind, number>;
  /** The five largest sets. */
  biggest: { name: string; title?: string; size: number }[];
  /** Sets with each kind of the host's thumbnails. */
  thumbs: Record<ThumbKind, number>;
}

export interface DeviceLibrary {
  dir: string;
  summary: LibrarySummary;
  /**
   * The whole list, from a device before 0.2.9 (it sends every set in
   * device_status and knows no roms_query): pages are then made here,
   * with queryRomsLocally. Absent from newer devices.
   */
  legacyRoms?: DeviceRom[];
  /** catalog: the core's game list, used to check ROMs, is installed. */
  /** The MAME core; hdInstalled tells whether go-link HD's core (for .glhd packages) is there too. */
  core: { name: string; installed: boolean; catalog: boolean; downloading: boolean; error?: string; hdInstalled?: boolean };
  /** Space on the volume that holds the ROM folder, in bytes. */
  disk?: { total: number; free: number };
  /** Folder of the host's own thumbnails on the device. */
  thumbnailsDir: string;
  /** The kind of thumbnail the host chose to show (device Settings). */
  thumbKind: ThumbKind;
  /** Space the host's thumbnail images take, in bytes. */
  thumbnailsBytes: number;
}

/** The kinds of thumbnails (libretro naming). */
export type ThumbKind = "boxart" | "title" | "snap";

/** States of a game room on the device. */
export type ManagedRoomState = "live" | "paused" | "archived" | "trash";

/** A game room the device manages (one game each; several at once). */
export interface ManagedRoom {
  id: string;
  name: string;
  rom: string;
  game: string;
  public: boolean;
  voice: boolean;
  state: ManagedRoomState;
  favorite: boolean;
  /** The signalhub room while the game runs, else "". */
  roomId: string;
  players: number;
  maxPlayers: number;
  spectators: number;
  queue: number;
  since: string;
  deletedAt: string | null;
  saves: { slot: number; name: string; at: string }[];
  autosave: boolean;
  lastError: string;
  /** The emulator does not save this game whole: it always starts from the beginning. */
  noSaves: boolean;
  /**
   * What the host's own browsers send instead of a PIN while the room
   * runs (only the owner sees it). Guests get a PIN per invitation.
   */
  ownerKey: string;
  /** The room's invitation while it runs: for /g/<invite> links and QR codes. */
  invite: string;
  /** Its 9 digit code, to type. */
  inviteCode: string;
  /** The room's group invitation (one link for several people), while alive. */
  groupInvite?: GroupInvite;
  /** The room's spectators' invitation, while alive. */
  watchInvite?: GroupInvite;
  /** People who came with it and wait for the host to let them in. */
  knocks?: Knock[];
  /** The host turned the room's chat off. */
  chatOff: boolean;
  /** The host is recording the game, since recordingSince. */
  recording?: boolean;
  recordingSince?: string;
  /** Players asking the host for a pause (answer with pause_answer). */
  pauseAsks?: PauseAskView[];
  /** The host's default picture style for the room's guests; null: the site's default. */
  picture?: PictureSettings | null;
  /** While it runs: the video quality in use, and why it is lower than the host's choice. */
  video?: RoomVideo;
}

/** Days a deleted room stays in the trash. */
export const TRASH_DAYS = 30;

const ROOM_STATES: readonly ManagedRoomState[] = ["live", "paused", "archived", "trash"];

function parseRooms(v: unknown): ManagedRoom[] {
  if (!Array.isArray(v)) return [];
  const str = (x: unknown, max = 120) => (typeof x === "string" ? x.slice(0, max) : "");
  const num = (x: unknown) => (typeof x === "number" && Number.isFinite(x) ? x : 0);
  return v.slice(0, 200).flatMap((r) => {
    const o = (typeof r === "object" && r !== null ? r : {}) as Record<string, unknown>;
    const id = str(o.id, 40);
    const state = ROOM_STATES.find((s) => s === o.state);
    if (!id || !state) return [];
    return [
      {
        id,
        name: str(o.name, 80),
        rom: str(o.rom, 16),
        game: str(o.game, 160) || str(o.rom, 16),
        public: o.public === true,
        voice: o.voice === true,
        state,
        favorite: o.favorite === true,
        roomId: str(o.room_id, 40),
        players: num(o.players),
        maxPlayers: num(o.max_players) || 4,
        spectators: num(o.spectators),
        queue: num(o.queue),
        since: str(o.since, 40),
        deletedAt: typeof o.deleted_at === "string" ? o.deleted_at : null,
        saves: (Array.isArray(o.saves) ? o.saves : []).slice(0, 100).flatMap((x) => {
          const sv = (typeof x === "object" && x !== null ? x : {}) as Record<string, unknown>;
          return typeof sv.slot === "number" ? [{ slot: sv.slot, name: str(sv.name, 40), at: str(sv.at, 40) }] : [];
        }),
        autosave: o.autosave === true,
        lastError: str(o.last_error, 300),
        noSaves: o.no_saves === true,
        ownerKey: ownerKeyOf(o.owner_key),
        invite: typeof o.invite === "string" && /^[A-Za-z0-9_-]{22}$/.test(o.invite) ? o.invite : "",
        chatOff: o.chat_off === true,
        inviteCode: typeof o.invite_code === "string" && /^\d{9}$/.test(o.invite_code) ? o.invite_code : "",
        groupInvite: parseGroupInvite(o.group_invite),
        watchInvite: parseGroupInvite(o.watch_invite),
        knocks: parseKnocks(o.knocks),
        recording: o.recording === true,
        recordingSince: str(o.recording_since, 40),
        pauseAsks: parsePauseAsks(o.pause_asks),
        picture: parseRoomPicture(o.picture),
        video: parseRoomVideo(o.video),
      },
    ];
  });
}

/** The room the device is running, as its status reports it. */
export interface DeviceRoom {
  room_id: string;
  viewers: number;
  title?: string;
  game?: string;
  public?: boolean;
  players?: number;
  max_players?: number;
  queue?: number;
  spectators?: number;
  /** The test pattern room's invitation and owner key (the owner's browsers only). */
  invite?: string;
  invite_code?: string;
  owner_key?: string;
  /** The test pattern room's group invitation and the people waiting (parsed). */
  groupInvite?: GroupInvite;
  watchInvite?: GroupInvite;
  knocks?: Knock[];
  /** The test pattern room's default picture style (absent: the site's default). */
  picture?: PictureSettings | null;
}

export interface DeviceStatus {
  device_id: string;
  version: string;
  signal_url: string;
  ice_urls: string[];
  roms_dir: string;
  system?: { hardware: DeviceHardware; usage: DeviceUsage; sampled_at: string };
  room?: DeviceRoom;
  library?: DeviceLibrary;
  linked_browsers: number;
  /** Game rooms (besides the test pattern room). */
  rooms: ManagedRoom[];
  /** Space the rooms' saved games take, in bytes. */
  savesBytes: number;
  /** A newer go-link release than the device runs, when there is one. */
  update?: DeviceUpdate;
  /** The host's video quality for game rooms (older devices: absent). */
  videoQuality?: VideoQuality;
}

export interface DeviceUpdate {
  latest: string;
  /** Its release page: always one of go-link's releases on GitHub. */
  url: string;
}

const RELEASES = "https://github.com/lordbasex/go-link/releases/";

/** Accepts an update only when it links to go-link's own releases. */
function parseUpdate(u: unknown): DeviceUpdate | undefined {
  if (typeof u !== "object" || u === null) return undefined;
  const { latest, url } = u as Record<string, unknown>;
  if (typeof latest !== "string" || !/^v?\d+\.\d+\.\d+[\w.+-]*$/.test(latest)) return undefined;
  if (typeof url !== "string" || !url.startsWith(RELEASES)) return undefined;
  return { latest, url };
}

/** Returns the status when msg is a device_status message. */
export function parseDeviceStatus(msg: unknown): DeviceStatus | null {
  if (typeof msg !== "object" || msg === null) return null;
  const m = msg as Record<string, unknown>;
  if (m.type !== "device_status" || typeof m.device_id !== "string") return null;
  return {
    device_id: m.device_id,
    version: typeof m.version === "string" ? m.version : "",
    signal_url: typeof m.signal_url === "string" ? m.signal_url : "",
    ice_urls: Array.isArray(m.ice_urls) ? m.ice_urls.filter((u): u is string => typeof u === "string") : [],
    roms_dir: typeof m.roms_dir === "string" ? m.roms_dir : "",
    system: typeof m.system === "object" && m.system !== null ? (m.system as DeviceStatus["system"]) : undefined,
    room: parseDeviceRoom(m.room),
    library: parseLibrary(m.library),
    linked_browsers: typeof m.linked_browsers === "number" ? m.linked_browsers : 0,
    rooms: parseRooms(m.rooms),
    savesBytes: bytesOf(m.saves_bytes),
    update: parseUpdate(m.update),
    videoQuality: parseVideoQuality(m.video_quality),
  };
}

function parseDeviceRoom(v: unknown): DeviceRoom | undefined {
  if (typeof v !== "object" || v === null) return undefined;
  const r = v as DeviceRoom & { picture?: unknown; group_invite?: unknown; watch_invite?: unknown };
  return { ...r, picture: parseRoomPicture(r.picture), groupInvite: parseGroupInvite(r.group_invite), watchInvite: parseGroupInvite(r.watch_invite), knocks: parseKnocks(r.knocks) };
}

/**
 * A group invitation: one link and QR code for several people. The key
 * goes after # in the link (groupInvitationUrl), so no server ever sees it;
 * it lets in `uses` people until expiresAt, each one OK'd by the host when
 * approval is on.
 */
export interface GroupInvite {
  key: string;
  uses: number;
  used: number;
  expiresAt: string;
  approval: boolean;
  /** The spectators' invitation: its people come in to watch. */
  watch?: boolean;
}

/** Someone who came with a group invitation and waits for the host. */
export interface Knock {
  peer: string;
  name: string;
  since: string;
  /** Came with the spectators' invitation, to watch. */
  watch?: boolean;
}

const GROUP_KEY = /^[A-Za-z0-9_-]{43}$/;

function parseGroupInvite(v: unknown): GroupInvite | undefined {
  if (typeof v !== "object" || v === null) return undefined;
  const g = v as Record<string, unknown>;
  if (typeof g.key !== "string" || !GROUP_KEY.test(g.key)) return undefined;
  const n = (x: unknown) => (typeof x === "number" && Number.isInteger(x) && x >= 0 ? Math.min(x, 1000) : 0);
  const gi: GroupInvite = { key: g.key, uses: n(g.uses), used: n(g.used), expiresAt: typeof g.expires_at === "string" ? g.expires_at.slice(0, 40) : "", approval: g.approval === true };
  if (g.watch === true) gi.watch = true;
  return gi;
}

function parseKnocks(v: unknown): Knock[] {
  return (Array.isArray(v) ? v : []).slice(0, 20).flatMap((x) => {
    const k = (typeof x === "object" && x !== null ? x : {}) as Record<string, unknown>;
    if (typeof k.peer !== "string" || !k.peer) return [];
    const knock: Knock = { peer: k.peer.slice(0, 64), name: typeof k.name === "string" ? k.name.slice(0, 24) : "", since: typeof k.since === "string" ? k.since.slice(0, 40) : "" };
    if (k.watch === true) knock.watch = true;
    return [knock];
  });
}

/** The device's answer to invite_group, end_group or knock_answer. */
export interface GroupResult {
  type: "invite_group" | "end_group" | "knock_answer";
  id: string;
  ok: boolean;
  error: string;
  group?: GroupInvite;
  /** About the spectators' invitation (invite_group and end_group). */
  watch: boolean;
}

export function parseGroupResult(msg: unknown): GroupResult | null {
  if (typeof msg !== "object" || msg === null) return null;
  const m = msg as Record<string, unknown>;
  const kind = typeof m.type === "string" ? m.type.replace(/_result$/, "") : "";
  if (!/_result$/.test(String(m.type)) || !["invite_group", "end_group", "knock_answer"].includes(kind) || typeof m.id !== "string") return null;
  const group = parseGroupInvite(m.group);
  return { type: kind as GroupResult["type"], id: m.id.slice(0, 40), ok: m.ok === true, error: typeof m.error === "string" ? m.error.slice(0, 200) : "", group, watch: m.watch === true || group?.watch === true };
}

/** A byte count from the device: a finite number, never negative. */
function bytesOf(v: unknown): number {
  return typeof v === "number" && Number.isFinite(v) && v > 0 ? v : 0;
}

function parseLibrary(v: unknown): DeviceLibrary | undefined {
  if (typeof v !== "object" || v === null) return undefined;
  const l = v as Record<string, unknown>;
  return {
    dir: text(l.dir, 300) ?? "",
    thumbnailsDir: text(l.thumbnails_dir, 300) ?? "",
    thumbKind: l.thumb_kind === "title" || l.thumb_kind === "snap" ? l.thumb_kind : "boxart",
    thumbnailsBytes: bytesOf(l.thumbnails_bytes),
    ...(() => {
      // A device before 0.2.9: the whole list instead of a summary.
      if (Array.isArray(l.roms) && (typeof l.summary !== "object" || l.summary === null)) {
        const legacyRoms = l.roms.slice(0, 5000).flatMap((r) => parseRom(r) ?? []);
        return { legacyRoms, summary: summarize(legacyRoms) };
      }
      return { summary: parseSummary(l.summary) };
    })(),
    core: (() => {
      const c = (typeof l.core === "object" && l.core !== null ? l.core : {}) as Record<string, unknown>;
      return { name: text(c.name, 40) ?? "", installed: c.installed === true, catalog: c.catalog === true, downloading: c.downloading === true, error: text(c.error, 200), hdInstalled: c.hd_installed === true };
    })(),
    disk: (() => {
      const k = (typeof l.disk === "object" && l.disk !== null ? l.disk : {}) as Record<string, unknown>;
      return typeof k.total === "number" && typeof k.free === "number" && k.total > 0 ? { total: k.total, free: Math.min(k.free, k.total) } : undefined;
    })(),
  };
}

const text = (x: unknown, max = 80) => (typeof x === "string" ? x.slice(0, max) : undefined);
const count = (x: unknown) => (typeof x === "number" && Number.isInteger(x) && x > 0 ? x : 0);

function parseSummary(v: unknown): LibrarySummary {
  const s = (typeof v === "object" && v !== null ? v : {}) as Record<string, unknown>;
  const k = (typeof s.kinds === "object" && s.kinds !== null ? s.kinds : {}) as Record<string, unknown>;
  return {
    revision: count(s.revision),
    total: count(s.total),
    bytes: bytesOf(s.bytes),
    playable: count(s.playable),
    kinds: Object.fromEntries(ROM_KINDS.map((x) => [x, count(k[x])])) as Record<RomKind, number>,
    thumbs: (() => {
      const th = (typeof s.thumbs === "object" && s.thumbs !== null ? s.thumbs : {}) as Record<string, unknown>;
      return { boxart: count(th.boxart), title: count(th.title), snap: count(th.snap) };
    })(),
    biggest: (Array.isArray(s.biggest) ? s.biggest : []).slice(0, 5).flatMap((b) => {
      const o = (typeof b === "object" && b !== null ? b : {}) as Record<string, unknown>;
      const name = text(o.name, 16);
      return name ? [{ name, title: text(o.title, 160), size: bytesOf(o.size) }] : [];
    }),
  };
}

/** One set as the device describes it, or null when it is not one. */
export function parseRom(r: unknown): DeviceRom | null {
  const o = (typeof r === "object" && r !== null ? r : {}) as Record<string, unknown>;
  const name = text(o.name, 16);
  if (!name) return null;
  const rom: DeviceRom = { name, size: bytesOf(o.size), title: text(o.title, 160), year: text(o.year, 4), maker: text(o.maker), thumbs: parseThumbs(o.thumbs), check: parseCheck(o.check) };
  if (o.own === true || o.kind === "glhd") {
    if (o.own === true) rom.own = true;
    else rom.kind = "glhd";
    rom.description = text(o.description, 400);
    rom.controls = parseRomControls(o.controls);
  }
  return rom;
}

/** The kind of a set (its check's verdict), as the device groups it. */
export function romKind(r: DeviceRom): RomKind {
  switch (r.check?.status) {
    case undefined:
      return "unchecked";
    case "ok":
      return "runs";
    case "missing":
    case "unsupported":
    case "bios":
      return r.check.status;
    default:
      return "broken";
  }
}

/** The summary of a whole list (a device before 0.2.9 sends the list). */
export function summarize(roms: DeviceRom[]): LibrarySummary {
  const kinds = Object.fromEntries(ROM_KINDS.map((k) => [k, 0])) as Record<RomKind, number>;
  roms.forEach((r) => (kinds[romKind(r)] += 1));
  return {
    revision: 0,
    total: roms.length,
    bytes: roms.reduce((a, r) => a + r.size, 0),
    playable: roms.filter(romPlayable).length,
    kinds,
    biggest: [...roms].sort((a, b) => b.size - a.size).slice(0, 5).map((r) => ({ name: r.name, title: r.title, size: r.size })),
    thumbs: {
      boxart: roms.filter((r) => r.thumbs.boxart).length,
      title: roms.filter((r) => r.thumbs.title).length,
      snap: roms.filter((r) => r.thumbs.snap).length,
    },
  };
}

/** roms_query answered here, the device's way, over a whole list (a device before 0.2.9). */
export function queryRomsLocally(
  roms: DeviceRom[],
  q: { q?: string; filter?: RomsFilter; sort?: RomsSort; offset?: number; limit?: number },
): { total: number; roms: DeviceRom[] } {
  const words = (q.q ?? "").toLowerCase().split(/\s+/).filter(Boolean);
  const filter = q.filter ?? "all";
  const title = (r: DeviceRom) => (r.title || r.name).toLowerCase();
  const byTitle = (a: DeviceRom, b: DeviceRom) => (title(a) < title(b) ? -1 : title(a) > title(b) ? 1 : a.name < b.name ? -1 : a.name > b.name ? 1 : 0);
  const year = (r: DeviceRom) => Number(r.year) || 0;
  const sorts: Record<RomsSort, (a: DeviceRom, b: DeviceRom) => number> = {
    name: byTitle,
    size: (a, b) => b.size - a.size || byTitle(a, b),
    year: (a, b) => year(b) - year(a) || byTitle(a, b),
    status: (a, b) => ROM_KINDS.indexOf(romKind(a)) - ROM_KINDS.indexOf(romKind(b)) || byTitle(a, b),
  };
  const all = roms
    .filter((r) => filter === "all" || (filter === "playable" ? romPlayable(r) : filter === "unplayable" ? !romPlayable(r) : romKind(r) === filter))
    .filter((r) => {
      const hay = `${r.name} ${r.title ?? ""} ${r.maker ?? ""}`.toLowerCase();
      return words.every((w) => hay.includes(w));
    })
    .sort(sorts[q.sort ?? "name"] ?? byTitle);
  const offset = Math.max(q.offset ?? 0, 0);
  return { total: all.length, roms: all.slice(offset, offset + (q.limit ?? ROMS_PAGE_MAX)) };
}

/** Sorts of roms_query: by title, the largest or newest first, or by state. */
export type RomsSort = "name" | "size" | "year" | "status";
/** Filters of roms_query: a state, or the sets a room can (not) be opened with. */
export type RomsFilter = "all" | "playable" | "unplayable" | RomKind;

/** The most sets one reply carries (the device's RomsPageMax). */
export const ROMS_PAGE_MAX = 100;

/** Asks the device for a page of its library: every word of q in the set's name, title or maker. */
export function romsQuery(req: string, q: { q?: string; filter?: RomsFilter; sort?: RomsSort; offset?: number; limit?: number }) {
  return { type: "roms_query", req, q: q.q ?? "", filter: q.filter ?? "all", sort: q.sort ?? "name", offset: q.offset ?? 0, limit: Math.min(q.limit ?? ROMS_PAGE_MAX, ROMS_PAGE_MAX) };
}

/** Asks the device for some sets by name (at most ROMS_PAGE_MAX); missing ones are left out. */
export function romsGet(req: string, names: string[]) {
  return { type: "roms_get", req, names: names.slice(0, ROMS_PAGE_MAX) };
}

export interface RomsPage {
  req: string;
  revision: number;
  /** The sets that match (roms_query), or the sets found (roms_get). */
  total: number;
  offset: number;
  roms: DeviceRom[];
}

/** The device's reply to roms_query or roms_get, or null for another message. */
export function parseRomsPage(msg: unknown): RomsPage | null {
  if (typeof msg !== "object" || msg === null) return null;
  const m = msg as Record<string, unknown>;
  if (m.type !== "roms_page" || typeof m.req !== "string") return null;
  return {
    req: m.req,
    revision: count(m.revision),
    total: count(m.total),
    offset: count(m.offset),
    roms: (Array.isArray(m.roms) ? m.roms : []).slice(0, ROMS_PAGE_MAX).flatMap((r) => parseRom(r) ?? []),
  };
}

/** 1536 MiB -> "1.5 GB"; 300 MiB -> "300 MB". */
export function formatBytes(n: number | undefined): string {
  if (!n) return "–";
  const gb = n / 1024 ** 3;
  if (gb >= 1024) return `${(gb / 1024).toFixed(1)} TB`;
  if (gb >= 1) return `${gb.toFixed(1)} GB`;
  return n >= 1024 ** 2 ? `${Math.round(n / 1024 ** 2)} MB` : `${Math.max(1, Math.round(n / 1024))} KB`;
}

function parseCheck(v: unknown): RomCheck | undefined {
  if (typeof v !== "object" || v === null) return undefined;
  const c = v as Record<string, unknown>;
  const status = CHECK_STATUSES.find((x) => x === c.status);
  if (!status) return undefined;
  const names = (x: unknown) =>
    Array.isArray(x) ? x.filter((n): n is string => typeof n === "string").slice(0, 10).map((n) => n.slice(0, 40)) : undefined;
  return { status, missing: names(c.missing), needs: names(c.needs), driver: typeof c.driver === "string" ? c.driver.slice(0, 20) : undefined };
}

function parseThumbs(v: unknown): DeviceRom["thumbs"] {
  const t = (typeof v === "object" && v !== null ? v : {}) as Record<string, unknown>;
  return { boxart: t.boxart === true, title: t.title === true, snap: t.snap === true };
}

/** Why a game session ended. */
export type HistoryReason = "archived" | "deleted" | "failed" | "device_stopped";
const HISTORY_REASONS: readonly HistoryReason[] = ["archived", "deleted", "failed", "device_stopped"];

/** One past game of the device: from the moment a room was turned on to the moment it stopped. */
export interface HistoryItem {
  /** Names the game for delete_history; "" for games saved before ids existed. */
  id?: string;
  roomId: string;
  name: string;
  rom: string;
  game: string;
  startedAt: string;
  endedAt: string;
  peakPlayers: number;
  peakSpectators: number;
  reason: HistoryReason;
  /** Everyone who joined, in order of arrival. */
  people: HistoryPerson[];
  /** Recordings the host made of this game (deleted with it). */
  recordings?: RecordingInfo[];
}

/** One browser in a past game: its name, the ports it played at (none = spectator) and how it connected. */
export interface HistoryPerson {
  name: string;
  ports: number[];
  /** Its address as the device saw it; "" when it came through its own relay. */
  ip: string;
  path: "direct" | "relay" | "";
}

/** Returns the list when msg is the device's "history" reply (newest first). */
export function parseHistory(msg: unknown): HistoryItem[] | null {
  if (typeof msg !== "object" || msg === null) return null;
  const m = msg as Record<string, unknown>;
  if (m.type !== "history" || !Array.isArray(m.items)) return null;
  const str = (x: unknown, max = 120) => (typeof x === "string" ? x.slice(0, max) : "");
  const num = (x: unknown) => (typeof x === "number" && Number.isFinite(x) && x > 0 ? Math.floor(x) : 0);
  return m.items.slice(0, 1000).flatMap((e) => {
    const o = (typeof e === "object" && e !== null ? e : {}) as Record<string, unknown>;
    const startedAt = str(o.started_at, 40);
    if (!startedAt) return [];
    return [
      {
        id: typeof o.id === "string" && /^[0-9a-f]{16}$/.test(o.id) ? o.id : "",
        roomId: str(o.room_id, 40),
        name: str(o.name, 80),
        rom: str(o.rom, 16),
        game: str(o.game, 160) || str(o.rom, 16),
        startedAt,
        endedAt: str(o.ended_at, 40),
        peakPlayers: num(o.peak_players),
        peakSpectators: num(o.peak_spectators),
        reason: HISTORY_REASONS.find((r) => r === o.reason) ?? "archived",
        people: (Array.isArray(o.people) ? o.people : []).slice(0, 64).map((x) => {
          const q = (typeof x === "object" && x !== null ? x : {}) as Record<string, unknown>;
          return {
            name: str(q.name, 40),
            ports: (Array.isArray(q.ports) ? q.ports : []).filter((n): n is number => Number.isInteger(n) && n >= 1 && n <= 4),
            ip: typeof q.ip === "string" && /^[0-9a-fA-F:.]{2,45}$/.test(q.ip) ? q.ip : "",
            path: q.path === "direct" || q.path === "relay" ? q.path : "",
          };
        }),
        recordings: parseRecordingList(o.recordings),
      },
    ];
  });
}

/** A host key or a guest's return token: 43 base64url characters. */
export function ownerKeyOf(v: unknown): string {
  return typeof v === "string" && /^[A-Za-z0-9_-]{43}$/.test(v) ? v : "";
}

/** A new invitation's PIN, good for one person (the device's "invite_pass"). */
export type InvitePass = { id: string; pin: string; expiresAt: string } | { id: string; error: string };

/** Returns the invitation when msg is the device's "invite_pass" reply. */
export function parseInvitePass(msg: unknown): InvitePass | null {
  if (typeof msg !== "object" || msg === null) return null;
  const m = msg as Record<string, unknown>;
  if (m.type !== "invite_pass" || typeof m.id !== "string") return null;
  const id = m.id.slice(0, 40);
  if (typeof m.pin === "string" && /^\d{6}$/.test(m.pin)) {
    return { id, pin: m.pin, expiresAt: typeof m.expires_at === "string" ? m.expires_at.slice(0, 40) : "" };
  }
  return { id, error: typeof m.error === "string" ? m.error.slice(0, 200) : "unknown error" };
}

/** A player asked the host for a pause (pause_asked), or the request is gone (pause_ask_gone). */
export type PauseAskEvent = { type: "asked"; id: string; ask: PauseAskView } | { type: "gone"; id: string; from: string };

/** Reads the device's pause_asked / pause_ask_gone, sent to linked browsers. */
export function parsePauseAskEvent(msg: unknown): PauseAskEvent | null {
  if (typeof msg !== "object" || msg === null) return null;
  const m = msg as Record<string, unknown>;
  const id = typeof m.id === "string" ? m.id.slice(0, 40) : "";
  const from = typeof m.from === "string" ? m.from.slice(0, 80) : "";
  if (!id || !from) return null;
  if (m.type === "pause_ask_gone") return { type: "gone", id, from };
  if (m.type !== "pause_asked") return null;
  const [ask] = parsePauseAsks([m]);
  return ask ? { type: "asked", id, ask } : null;
}
