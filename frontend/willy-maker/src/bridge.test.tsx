// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { beforeEach, describe, expect, it, vi } from "vitest";
import { BRIDGE_TAG, MAKER_STORAGE_PREFIX } from "@go-link/shared";
import { BridgeClient } from "./bridge";
import { importGames } from "./importGames";

const SITE = "https://console.go-link.org";

/** A page that opens the bridge tab: the tab is a fake window, its messages are handed in by hand. */
function setup() {
  let listener: ((e: MessageEvent) => void) | null = null;
  const tab = { closed: false, postMessage: vi.fn(), focus: vi.fn() };
  const page = {
    addEventListener: (_: string, fn: (e: MessageEvent) => void) => (listener = fn),
    removeEventListener: () => (listener = null),
    open: vi.fn(() => tab),
  };
  const client = new BridgeClient(SITE, page as unknown as Window);
  client.start();
  const say = (body: Record<string, unknown>, from: { origin?: string; source?: unknown } = {}) =>
    listener?.({ origin: from.origin ?? SITE, source: from.source ?? tab, data: { tag: BRIDGE_TAG, ...body } } as MessageEvent);
  return { client, tab, page, say, sent: () => tab.postMessage.mock.calls.map((c) => c[0] as Record<string, unknown>) };
}

describe("BridgeClient", () => {
  it("opens the console.go-link.org bridge tab and follows the link's state", () => {
    const { client, page, say } = setup();
    expect(client.getStatus()).toBeNull();
    client.connect();
    expect(page.open).toHaveBeenCalledWith(`${SITE}/maker-bridge`, expect.stringMatching(/^go-link-maker-bridge/));
    say({ kind: "status", linked: true, online: true, name: "arcade" });
    expect(client.getStatus()).toMatchObject({ linked: true, online: true, name: "arcade" });
  });

  it("ignores messages from other sites or other tabs", () => {
    const { client, say } = setup();
    client.connect();
    say({ kind: "status", linked: true, online: true }, { origin: "https://evil.example" });
    say({ kind: "status", linked: true, online: true }, { source: {} });
    expect(client.getStatus()).toBeNull();
  });

  it("sends controls and files through the tab, with progress and the device's answers", async () => {
    const { client, say, sent, tab } = setup();
    expect(client.link.sendControl({ type: "rom_test" })).toBe(false); // no tab yet
    client.connect();
    expect(client.link.sendControl({ type: "rom_test", id: "a", set: "s" })).toBe(true);
    expect(sent()[0]).toMatchObject({ kind: "control", message: { type: "rom_test" } });
    expect(tab.postMessage.mock.calls[0]?.[1]).toBe(SITE);

    const progress = vi.fn();
    const done = client.link.sendFile("f1", new Blob([new Uint8Array([1, 2, 3])]), "slammast.zip", progress, "maker");
    await vi.waitFor(() => expect(sent()[1]).toMatchObject({ kind: "file", id: "f1", name: "slammast.zip", purpose: "maker" }));
    const req = sent()[1]?.req;
    say({ kind: "file_progress", req, sent: 3, total: 3 });
    say({ kind: "file_done", req });
    await done;
    expect(progress).toHaveBeenCalledWith(3, 3);

    const answers: unknown[] = [];
    client.onDeviceMessage((m) => answers.push(m));
    say({ kind: "device", message: { type: "room_created", room_id: "r1" } });
    expect(answers).toEqual([{ type: "room_created", room_id: "r1" }]);
  });

  it("fails a file when the tab is closed, and opens a new tab after a room", async () => {
    const { client, say, tab, page } = setup();
    client.connect();
    say({ kind: "status", linked: true, online: true });
    client.openRoom("room-1");
    expect(client.getStatus()).toBeNull();
    client.connect();
    expect(page.open).toHaveBeenCalledTimes(2);
    tab.closed = true;
    await expect(client.link.sendFile("f", new Blob([new Uint8Array([1])]), "a.zip")).rejects.toThrow(/channel not open/);
  });
});

describe("importGames", () => {
  beforeEach(() => window.localStorage.clear());

  it("brings the games this site does not have, and keeps its own", async () => {
    const P = `${MAKER_STORAGE_PREFIX}p.`;
    const INDEX = `${MAKER_STORAGE_PREFIX}index`;
    window.localStorage.setItem(`${P}mine`, '{"id":"mine"}');
    window.localStorage.setItem(INDEX, JSON.stringify([{ id: "mine", title: "Mine" }]));
    const n = await importGames({
      entries: [
        [INDEX, JSON.stringify([{ id: "mine", title: "Old copy" }, { id: "old", title: "Old" }])],
        [`${P}mine`, '{"id":"mine","old":true}'],
        [`${P}old`, '{"id":"old"}'],
      ],
      assets: [["sha256:x", { type: "image/png", bytes: new Uint8Array([137, 80, 78, 71]) }]],
    });
    expect(n).toBe(1);
    expect(window.localStorage.getItem(`${P}mine`)).toBe('{"id":"mine"}');
    expect(window.localStorage.getItem(`${P}old`)).toBe('{"id":"old"}');
    expect(JSON.parse(window.localStorage.getItem(INDEX) ?? "[]").map((p: { id: string }) => p.id)).toEqual(["mine", "old"]);
  });
});
