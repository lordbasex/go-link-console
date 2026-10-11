// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { describe, expect, it } from "vitest";
import { parseDeviceStatus, parsePauseAskEvent, parseRomsPage, romFile, queryRomsLocally, romKind, romsGet, romsQuery } from "../src/device-status";

describe("device update", () => {
  const base = { type: "device_status", device_id: "d", version: "v0.1.0" };
  it("offers only go-link's own releases", () => {
    const ok = parseDeviceStatus({ ...base, update: { latest: "v0.2.0", url: "https://github.com/lordbasex/go-link-console/releases/tag/v0.2.0" } });
    expect(ok?.update).toEqual({ latest: "v0.2.0", url: "https://github.com/lordbasex/go-link-console/releases/tag/v0.2.0" });
    const evil = parseDeviceStatus({ ...base, update: { latest: "v9.9.9", url: "https://evil.example/go-link.dmg" } });
    expect(evil?.update).toBeUndefined();
    expect(parseDeviceStatus(base)?.update).toBeUndefined();
  });
});

describe("pause requests for the host", () => {
  it("lists a room's requests in device_status", () => {
    const st = parseDeviceStatus({
      type: "device_status",
      device_id: "d",
      rooms: [{ id: "r1", state: "live", name: "Co-op", pause_asks: [{ from: "peer-a", name: "Ana", port: 1, expires_at: "2026-09-29T12:00:30Z" }] }],
    });
    expect(st?.rooms[0]?.pauseAsks).toEqual([{ from: "peer-a", name: "Ana", port: 1, expiresAt: Date.parse("2026-09-29T12:00:30Z") }]);
  });
  it("reads pause_asked and pause_ask_gone", () => {
    expect(parsePauseAskEvent({ type: "pause_asked", id: "r1", from: "peer-a", name: "Ana", port: 2, expires_at: "2026-09-29T12:00:30Z" })).toEqual({
      type: "asked",
      id: "r1",
      ask: { from: "peer-a", name: "Ana", port: 2, expiresAt: Date.parse("2026-09-29T12:00:30Z") },
    });
    expect(parsePauseAskEvent({ type: "pause_ask_gone", id: "r1", from: "peer-a" })).toEqual({ type: "gone", id: "r1", from: "peer-a" });
    expect(parsePauseAskEvent({ type: "pause_asked", id: "r1", from: "peer-a" })).toBeNull(); // no time
    expect(parsePauseAskEvent({ type: "pause_ask_gone", from: "peer-a" })).toBeNull();
    expect(parsePauseAskEvent({ type: "room_result", id: "r1", from: "x" })).toBeNull();
  });
});

describe("video quality", () => {
  it("reads the host's choice and each running room's video", () => {
    const st = parseDeviceStatus({
      type: "device_status",
      device_id: "d",
      video_quality: "normal",
      rooms: [
        { id: "r1", state: "live", name: "A", video: { quality: "saver", fallback: "cpu", scale: 1 } },
        { id: "r2", state: "live", name: "B", video: { quality: "high", scale: 2 } },
        { id: "r3", state: "archived", name: "C" },
        { id: "r4", state: "live", name: "D", video: { quality: "ultra", scale: 9 } },
      ],
    });
    expect(st?.videoQuality).toBe("normal");
    expect(st?.rooms.map((r) => r.video)).toEqual([
      { quality: "saver", fallback: "cpu", scale: 1 },
      { quality: "high", fallback: undefined, scale: 2 },
      undefined,
      undefined,
    ]);
    expect(parseDeviceStatus({ type: "device_status", device_id: "d", video_quality: "4k" })?.videoQuality).toBeUndefined();
  });
});

describe("go-link HD packages in the library", () => {
  it("keeps a package's kind and the controls of its manifest", () => {
    const s = parseRomsPage({
      type: "roms_page",
      req: "r",
      roms: [
          { name: "neon", size: 10, kind: "glhd", title: "Neon Run", description: "go-link HD", controls: { players: 2, buttons: 4, labels: ["Jump", "Jump", "Run", "Run"] }, thumbs: {} },
          { name: "robby", size: 10, kind: "other", controls: { players: 2, buttons: 2 }, thumbs: {} },
      ],
    });
    const [neon, robby] = s!.roms;
    expect(neon!.kind).toBe("glhd");
    expect(neon!.controls?.players).toBe(2);
    expect(romFile(neon!)).toBe("neon.glhd");
    expect(robby!.kind).toBeUndefined();
    expect(robby!.controls).toBeUndefined();
    expect(romFile(robby!)).toBe("robby.zip");
  });
});

describe("the library in pages", () => {
  it("reads the summary of device_status, never a list of sets", () => {
    const s = parseDeviceStatus({
      type: "device_status",
      device_id: "d",
      library: {
        dir: "/roms",
        roms: [{ name: "ignored" }],
        summary: { revision: 3, total: 5085, bytes: 11e9, playable: 4818, kinds: { runs: 4818, missing: 250, bios: 2, weird: 9 }, biggest: [{ name: "kof2002", title: "KOF", size: 9e7 }, { size: 1 }], thumbs: { boxart: 4000, snap: -1 } },
      },
    });
    const lib = s!.library!;
    expect(lib).not.toHaveProperty("roms");
    expect(lib.summary).toEqual({
      revision: 3,
      total: 5085,
      bytes: 11e9,
      playable: 4818,
      kinds: { runs: 4818, missing: 250, unsupported: 0, broken: 0, bios: 2, unchecked: 0 },
      biggest: [{ name: "kof2002", title: "KOF", size: 9e7 }],
      thumbs: { boxart: 4000, title: 0, snap: 0 },
    });
    expect(parseDeviceStatus({ type: "device_status", device_id: "d", library: { dir: "/x" } })!.library!.summary.total).toBe(0);
  });

  it("asks for pages and reads the replies", () => {
    expect(romsQuery("a1", { q: "street", filter: "playable", sort: "year", offset: 60, limit: 500 })).toEqual({
      type: "roms_query", req: "a1", q: "street", filter: "playable", sort: "year", offset: 60, limit: 100,
    });
    expect(romsGet("a2", ["sf2", "pacman"])).toEqual({ type: "roms_get", req: "a2", names: ["sf2", "pacman"] });
    const page = parseRomsPage({ type: "roms_page", req: "a1", revision: 3, total: 120, offset: 60, roms: [{ name: "sf2", check: { status: "ok" }, thumbs: { boxart: true } }, { title: "no name" }] });
    expect(page).toMatchObject({ req: "a1", revision: 3, total: 120, offset: 60 });
    expect(page!.roms.map((r) => [r.name, romKind(r), r.thumbs.boxart])).toEqual([["sf2", "runs", true]]);
    expect(parseRomsPage({ type: "roms_page" })).toBeNull();
    expect(parseRomsPage({ type: "history", req: "a" })).toBeNull();
  });
});

describe("a device before 0.2.9", () => {
  it("sends the whole list, which the website pages itself", () => {
    const s = parseDeviceStatus({
      type: "device_status",
      device_id: "d",
      library: {
        dir: "/roms",
        roms: [
          { name: "sf2", title: "Street Fighter II", year: "1991", size: 30, check: { status: "ok" }, thumbs: { boxart: true } },
          { name: "pacman", title: "Pac-Man", year: "1980", size: 10, check: { status: "ok" } },
          { name: "looping", title: "Looping", size: 20, check: { status: "missing" } },
        ],
      },
    });
    const lib = s!.library!;
    expect(lib.legacyRoms).toHaveLength(3);
    expect(lib.summary).toMatchObject({ total: 3, bytes: 60, playable: 2, kinds: { runs: 2, missing: 1 }, thumbs: { boxart: 1 } });
    expect(lib.summary.biggest.map((b) => b.name)).toEqual(["sf2", "looping", "pacman"]);
    const names = (q: Parameters<typeof queryRomsLocally>[1]) => queryRomsLocally(lib.legacyRoms!, q).roms.map((r) => r.name);
    expect(names({})).toEqual(["looping", "pacman", "sf2"]);
    expect(names({ sort: "year" })).toEqual(["sf2", "pacman", "looping"]);
    expect(names({ filter: "playable", q: "STREET" })).toEqual(["sf2"]);
    expect(queryRomsLocally(lib.legacyRoms!, { filter: "missing" }).total).toBe(1);
    // a newer device sends the summary only
    expect(parseDeviceStatus({ type: "device_status", device_id: "d", library: { dir: "/r", roms: [{ name: "x" }], summary: { total: 9 } } })!.library!.legacyRoms).toBeUndefined();
  });
});

describe("go-link HD in device_status", () => {
  it("reads its screen, folder and starter games", () => {
    const st = parseDeviceStatus({
      type: "device_status",
      device_id: "d",
      library: { dir: "/r", hd_dir: "/h", hd_games: 1, hd_bytes: 117389971, core: {} },
      hd: { room_size: "1080p", codec: "vp8", starter: [{ name: "antidoto", title: "ANTÍDOTO", installed: true, size: "720p", version: "0.1.0", update: "0.1.1", downloading: "1080p", done: 10, total: 100 }, { title: "no name" }] },
    });
    expect(st?.hd).toEqual({
      roomSize: "1080p",
      codec: "vp8",
      starter: [{ name: "antidoto", title: "ANTÍDOTO", installed: true, size: "720p", version: "0.1.0", update: "0.1.1", downloading: "1080p", done: 10, total: 100, error: undefined }],
    });
    expect([st?.library?.hdDir, st?.library?.hdGames, st?.library?.hdBytes]).toEqual(["/h", 1, 117389971]);
    // An older device: no hd, unknown values fall back to the defaults.
    expect(parseDeviceStatus({ type: "device_status", device_id: "d" })?.hd).toBeUndefined();
    expect(parseDeviceStatus({ type: "device_status", device_id: "d", hd: { room_size: "4k" } })?.hd).toEqual({ roomSize: "720p", codec: "auto", starter: [] });
  });
});
