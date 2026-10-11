// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import { describe, expect, it } from "vitest";
import { formatCode, normalizeCode, parseRoomInput, isRoomId, parseRoomMeta, freeSeats, parseInvite } from "../src";

const ID = "3f2b9c1e-7a4d-4e0b-9c52-1d8e6f0aa71d";

describe("pairing code", () => {
  it("normalizes what people type", () => {
    expect(normalizeCode("113 134 323")).toBe("113134323");
    expect(normalizeCode(" 113-134.323 ")).toBe("113134323");
    expect(normalizeCode("11313432")).toBeNull();
    expect(normalizeCode("11313432a")).toBeNull();
  });
  it("formats in groups of three", () => {
    expect(formatCode("113134323")).toBe("113 134 323");
  });
});

describe("room input", () => {
  it("accepts a bare id or a link", () => {
    expect(isRoomId(ID)).toBe(true);
    expect(parseRoomInput(ID)).toBe(ID);
    expect(parseRoomInput(`https://go-link.org/r/${ID}`)).toBe(ID);
    expect(parseRoomInput(`go-link.org/r/${ID.toUpperCase()}?x=1`)).toBe(ID);
  });
  it("rejects anything else", () => {
    expect(parseRoomInput("")).toBeNull();
    expect(parseRoomInput("hello")).toBeNull();
    expect(parseRoomInput(`/r/${ID}extra`)).toBeNull();
  });
});

describe("room meta", () => {
  it("parses a valid meta", () => {
    const meta = parseRoomMeta({ title: "Coop night", game: "Sky Pirates 2099", host: "ana", players: 1, max_players: 2, queue: 3, spectators: 7, mode: "coop" });
    expect(meta).toEqual({ title: "Coop night", game: "Sky Pirates 2099", host: "ana", players: 1, maxPlayers: 2, queue: 3, spectators: 7, mode: "coop", paused: false, art: null });
    expect(freeSeats(meta!)).toBe(1);
  });
  it("bounds untrusted values", () => {
    const meta = parseRoomMeta({ title: "x".repeat(500), game: "g", players: 99, max_players: 50, queue: -4, spectators: "lots", mode: "battle" });
    expect(meta!.title).toHaveLength(80);
    expect(meta!.maxPlayers).toBe(4);
    expect(meta!.players).toBe(4);
    expect(meta!.queue).toBe(0);
    expect(meta!.spectators).toBe(0);
    expect(meta!.mode).toBeNull();
  });
  it("rejects incomplete meta", () => {
    expect(parseRoomMeta(null)).toBeNull();
    expect(parseRoomMeta({ title: "only title" })).toBeNull();
    expect(parseRoomMeta("text")).toBeNull();
  });
});

describe("invitations", () => {
  it("reads links, bare invites and typed codes", () => {
    const inv = "abcdefghijklmnopqrstu_";
    expect(parseInvite(`https://play.go-link.org/g/${inv}`)).toEqual({ invite: inv });
    expect(parseInvite(inv)).toEqual({ invite: inv });
    expect(parseInvite(" 123 456-789 ")).toEqual({ code: "123456789" });
    expect(parseInvite("12345678")).toBeNull();
    expect(parseInvite("hello")).toBeNull();
    expect(parseInvite(`/r/${ID}`)).toBeNull();
  });
});
