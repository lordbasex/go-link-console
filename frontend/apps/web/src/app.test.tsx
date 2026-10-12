// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ServerError, type Envelope } from "@go-link/shared";
import { FakeSocket, OFFICIAL, renderApp } from "./test-utils";
import { panelProof } from "@go-link/shared";
import { setLang } from "./i18n";
import { es } from "./i18n/es";
import { TERMS_VERSION } from "./legal";
import { docsEn } from "./i18n/docs-en";
import { docsEs } from "./i18n/docs-es";
import { docsPt } from "./i18n/docs-pt";

const ROOM = "3f2b9c1e-7a4d-4e0b-9c52-1d8e6f0aa71d";
const HOST = "host".padEnd(32, "0");

const lobbyRooms: Envelope = {
  type: "rooms",
  rooms: [
    { room_id: ROOM, meta: { title: "Coop night", game: "Sky Pirates 2099", host: "ana", players: 1, max_players: 2, queue: 0, spectators: 3, mode: "coop" } },
    { room_id: ROOM.replace("aa71d", "aa71e"), meta: { title: "Full house", game: "Pickle Harbor Heist", host: "leo", players: 2, max_players: 2, queue: 2, spectators: 0, mode: "versus" } },
    { room_id: ROOM.replace("aa71d", "aa71f"), meta: { broken: true } },
  ],
};

beforeEach(() => {
  localStorage.clear();
  setLang("en");
});
afterEach(() => cleanup());

describe("lobby", () => {
  it("gives a guest the code and PIN form and no list of games", async () => {
    FakeSocket.reset((env) => (env.type === "rooms_list" ? lobbyRooms : undefined));
    renderApp("/rooms");
    expect(await screen.findByRole("heading", { name: "Join a game" })).toBeInTheDocument();
    expect(screen.queryByText("Coop night")).not.toBeInTheDocument();
    expect(screen.getAllByText("Guest").length).toBeGreaterThan(0);
    expect(screen.getByRole("button", { name: "Join" })).toBeDisabled();
  });

  it("shows the rooms as cards or as a list", async () => {
    FakeSocket.reset();
    renderApp("/rooms", { demo: true });
    expect(await screen.findByText("Sky Pirates Saturday")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "List" }));
    const table = screen.getByRole("table");
    expect(within(table).getByRole("columnheader", { name: "Actions" })).toBeInTheDocument();
    expect(within(table).getAllByRole("row").length).toBeGreaterThan(2);
    expect(localStorage.getItem("go-link.rooms-view")).toBe("list");
  });

  it("opens on the How it works landing, first in the menu, with a quick way into a game", async () => {
    FakeSocket.reset((env) => (env.type === "rooms_list" ? { type: "rooms" } : undefined));
    renderApp("/");
    const menu = screen.getByRole("navigation", { name: "Main" });
    const links = within(menu).getAllByRole("link");
    expect(links.map((a) => a.textContent)).toEqual(["How it works", "Rooms", "My device", "Docs", "Tools"]);
    expect(links.map((a) => a.getAttribute("href"))).toEqual(["/", "/rooms", "/device", "/docs", "/tools"]);
    // The landing is a lazy chunk: a busy machine needs more than a second.
    expect(await screen.findByRole("heading", { level: 1, name: /Your arcade, online/ }, { timeout: 10_000 })).toBeInTheDocument();
    expect(screen.getByText(/MAME 2003-Plus sets/)).toBeInTheDocument();
    await userEvent.click(screen.getAllByRole("button", { name: "Join a game" })[0]!);
    const dialog = screen.getByRole("dialog", { name: "Join a game" });
    await userEvent.type(within(dialog).getByPlaceholderText("123 456 789"), "123456789");
    await userEvent.type(within(dialog).getByPlaceholderText("000000"), "482913");
    await userEvent.click(within(dialog).getByRole("checkbox", { name: /I have read and accept/ }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Join" }));
    // Off to the room (the page loads on demand, so the dialog goes when it arrives).
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Join a game" })).not.toBeInTheDocument(), { timeout: 10_000 });
  }, 20_000);

  it("renders the design sample data in demo mode", () => {
    FakeSocket.reset();
    renderApp("/rooms", { demo: true });
    expect(screen.getAllByRole("article")).toHaveLength(6);
    expect(screen.getByText("Sky Pirates Saturday")).toBeInTheDocument();
  });
});

describe("destroy easter egg", () => {
  it("has a direct link that lands on the home page", async () => {
    renderApp("/WillyGorklingo");
    expect(await screen.findByRole("button", { name: "Destroy this page!" })).toBeInTheDocument();
  });

  it("shows the devil only on the landing", async () => {
    const { unmount } = renderApp("/");
    expect(await screen.findByRole("button", { name: "Destroy this page!" })).toBeInTheDocument();
    unmount();
    for (const path of ["/rooms", "/device", "/docs", "/tools", "/tools/skin-editor", "/tools/games", "/test-controller"]) {
      const r = renderApp(path);
      await screen.findByRole("navigation", { name: "Main" });
      expect(screen.queryByRole("button", { name: "Destroy this page!" })).toBeNull();
      r.unmount();
    }
  });
});

describe("languages", () => {
  it("switches between English, Spanish and Portuguese without reloading", async () => {
    FakeSocket.reset((env) => (env.type === "rooms_list" ? lobbyRooms : undefined));
    renderApp("/rooms");
    expect(await screen.findByRole("heading", { name: "Join a game" })).toBeInTheDocument();
    // One button with the current language; its menu picks another.
    const pick = async (name: string) => {
      await userEvent.click(screen.getByRole("button", { name: /^(Language|Idioma): / }));
      await userEvent.click(screen.getByRole("menuitemradio", { name: new RegExp(name) }));
    };
    expect(screen.getByRole("button", { name: "Language: English" })).toHaveAttribute("aria-haspopup", "menu");

    await pick("Español");
    expect(screen.getByRole("button", { name: es.guest.join })).toBeInTheDocument();
    expect(document.documentElement.lang).toBe("es");
    expect(localStorage.getItem("go-link.lang")).toBe("es");
    expect(screen.getByRole("heading", { name: es.guest.title })).toBeInTheDocument(); // same page, nothing reloaded

    await pick("Português");
    await userEvent.click(screen.getByRole("button", { name: /: Português$/ }));
    expect(screen.getByRole("menuitemradio", { name: /Português/ })).toHaveAttribute("aria-checked", "true");
    await userEvent.click(screen.getByRole("menuitemradio", { name: /English/ }));
    expect(screen.getByRole("button", { name: "Join" })).toBeInTheDocument();
  });
});

describe("device pairing", () => {
  it("links a device with a pasted code", async () => {
    FakeSocket.reset((env) => {
      if (env.type === "rooms_list") return { type: "rooms" };
      if (env.type === "claim") return env.code === "113134323" ? { type: "paired", session_id: "S", remote: HOST } : { type: "error", error: ServerError.InvalidCode };
    });
    renderApp("/device");
    const first = await screen.findByLabelText("Digit 1 of 9");
    await userEvent.click(first);
    await userEvent.paste("113 134 323");
    expect(screen.getByLabelText("Digit 9 of 9")).toHaveValue("3");
    // The host must accept the terms of use first: nothing is claimed.
    await userEvent.click(screen.getByRole("button", { name: "Link" }));
    expect(screen.getByRole("alert")).toHaveTextContent("Accept the terms of use to continue.");
    expect(FakeSocket.allSent().some((e) => e.type === "claim")).toBe(false);
    await userEvent.click(screen.getByRole("checkbox", { name: /I have read and accept/ }));
    await userEvent.click(screen.getByRole("button", { name: "Link" }));
    // The page stays on /device and turns into the device panel.
    expect(await screen.findByRole("heading", { name: "Your device" })).toBeInTheDocument();
    expect(screen.queryByLabelText("Digit 1 of 9")).not.toBeInTheDocument();
    expect(FakeSocket.allSent().find((e) => e.type === "claim")).toMatchObject({ app: "go-link-console", code: "113134323" });
  });

  it("comes back to a remembered device without a code", async () => {
    FakeSocket.reset((env) => {
      if (env.type === "rooms_list") return { type: "rooms" };
      if (env.type === "reach") return { type: "error", error: ServerError.DeviceOffline };
    });
    localStorage.setItem(
      "go-link.device-link",
      JSON.stringify({
        deviceId: "c2c5f76f-1a25-45d8-abe7-91f51d11f75d",
        linkId: "3f9c0a1b2c3d4e5f",
        token: "q8VY3n1s0bPq0n8oJ1m3Zr2yX7eW5vT4uS6rQ9pO0nM", // gitleaks:allow (test value)
        signalUrl: "wss://signal.test/ws",
        savedAt: 1,
      }),
    );
    renderApp("/device");
    expect(screen.queryByLabelText("Digit 1 of 9")).not.toBeInTheDocument();
    expect(await screen.findByText("Your device is offline")).toBeInTheDocument();
    expect(FakeSocket.allSent().find((e) => e.type === "reach")).toMatchObject({ app: "go-link-console", device_id: "c2c5f76f-1a25-45d8-abe7-91f51d11f75d" });
    // Forgetting the device brings the code form back.
    await userEvent.click(screen.getByRole("button", { name: "Forget this device" }));
    expect(screen.getByLabelText("Digit 1 of 9")).toBeInTheDocument();
    expect(localStorage.getItem("go-link.device-link")).toBeNull();
  });

  it("explains an invalid code and offers the server setting", async () => {
    FakeSocket.reset((env) => (env.type === "claim" ? { type: "error", error: ServerError.InvalidCode } : undefined));
    renderApp("/device");
    await userEvent.click(screen.getByLabelText("Digit 1 of 9"));
    await userEvent.paste("999999999");
    await userEvent.click(screen.getByRole("checkbox", { name: /I have read and accept/ }));
    await userEvent.click(screen.getByRole("button", { name: "Link" }));
    expect(await screen.findByText(/That code is invalid or expired/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Change signaling server" }));
    expect(screen.getByRole("dialog", { name: "Signaling server" })).toBeInTheDocument();
  });

  it("asks for all nine digits", async () => {
    FakeSocket.reset();
    renderApp("/device");
    await userEvent.type(screen.getByLabelText("Digit 1 of 9"), "12");
    await userEvent.click(screen.getByRole("button", { name: "Link" }));
    expect(screen.getByRole("alert")).toHaveTextContent("Type all 9 digits.");
  });
});

describe("room", () => {
  it("shows help when the room does not exist on this server", async () => {
    FakeSocket.reset((env) => {
      if (env.type === "rooms_list") return { type: "rooms" };
      if (env.type === "join") return { type: "error", error: ServerError.RoomNotFound };
    });
    renderApp(`/r/${ROOM}`);
    expect(await screen.findByRole("heading", { name: "Room not found" })).toBeInTheDocument();
    expect(screen.getByText("Is the host using their own server?")).toBeInTheDocument();
  });

  it("joins, then ends when the host leaves", async () => {
    FakeSocket.reset((env) => {
      if (env.type === "rooms_list") return lobbyRooms;
      if (env.type === "join") return { type: "joined", session_id: "S", remote: HOST, room_id: env.room_id };
    });
    renderApp(`/r/${ROOM}`);
    expect(await screen.findByText("Connected to the host. Waiting for the game video…")).toBeInTheDocument();
    expect(await screen.findByRole("heading", { name: "Coop night" })).toBeInTheDocument();
    // A guest (not the device's owner) cannot invite or close the game.
    expect(screen.queryByRole("button", { name: "Invite" })).toBeNull();
    expect(screen.queryByRole("button", { name: /Copy invite/ })).toBeNull();
    expect(screen.queryByRole("button", { name: "Close game" })).toBeNull();
    const socket = FakeSocket.sockets.find((s) => s.sent.some((e) => e.type === "join"))!;
    socket.push({ type: "peer_left", from: HOST, session_id: "S" });
    expect(await screen.findByRole("heading", { name: "The host closed the room." })).toBeInTheDocument();
  });

  it("joins through an invitation link and learns the room from joined", async () => {
    const invite = "abcdefghijklmnopqrstuv";
    FakeSocket.reset((env) => {
      if (env.type === "rooms_list") return lobbyRooms;
      if (env.type === "join") return env.invite === invite ? { type: "joined", session_id: "S", remote: HOST, room_id: ROOM } : { type: "error", error: ServerError.InvalidInvite };
    });
    renderApp(`/g/${invite}`);
    expect(await screen.findByText("Connected to the host. Waiting for the game video…")).toBeInTheDocument();
    expect(await screen.findByRole("heading", { name: "Coop night" })).toBeInTheDocument();
    const join = FakeSocket.allSent().find((e) => e.type === "join")!;
    expect(join.invite).toBe(invite);
    expect(join.room_id).toBeUndefined();
  });

  it("joins with the code and PIN a guest typed", async () => {
    FakeSocket.reset((env) => {
      if (env.type === "rooms_list") return lobbyRooms;
      if (env.type === "join") return env.code === "123456789" ? { type: "joined", session_id: "S", remote: HOST, room_id: ROOM } : { type: "error", error: ServerError.InvalidInvite };
    });
    renderApp("/rooms");
    await userEvent.type(await screen.findByLabelText("Game code"), "123 456 789");
    await userEvent.type(screen.getByLabelText("PIN"), "482913");
    await userEvent.click(screen.getByRole("checkbox", { name: /I have read and accept/ }));
    await userEvent.click(screen.getByRole("button", { name: "Join" }));
    await vi.waitFor(() => expect(FakeSocket.allSent().find((e) => e.type === "join")?.code).toBe("123456789"));
    // The device asks for the PIN: the page sends the one typed, never in the URL.
    const socket = FakeSocket.sockets.find((s) => s.sent.some((e) => e.type === "join"))!;
    socket.push({ type: "signal", from: HOST, payload: { kind: "pin_required" } });
    await vi.waitFor(() =>
      expect(FakeSocket.allSent().find((e) => e.type === "signal" && (e.payload as { kind?: string })?.kind === "pin")?.payload).toEqual({ kind: "pin", pin: "482913" }),
    );
  });

  it("rejects a malformed room id without asking the server", async () => {
    FakeSocket.reset();
    renderApp("/r/not-a-room");
    expect(screen.getByText("That room ID is not valid.")).toBeInTheDocument();
    await new Promise((r) => setTimeout(r, 20));
    expect(FakeSocket.allSent().some((e) => e.type === "join")).toBe(false);
  });

  it("renders the spectator variant in demo mode", async () => {
    FakeSocket.reset();
    renderApp(`/r/${ROOM}?perspective=spectator&spectatorsHearVoice=true`, { demo: true });
    expect(await screen.findByText("You are next in the queue")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Microphone not available" })).toBeDisabled();
    // The voice rules sit in the dock's voice settings.
    await userEvent.click(screen.getByRole("button", { name: "Volume and voice" }));
    expect(screen.getByText(/You hear the players' commentary/)).toBeInTheDocument();
  });

  it("explains every control in the how to play dialog", async () => {
    FakeSocket.reset();
    renderApp(`/r/${ROOM}`, { demo: true });
    await userEvent.click(screen.getByRole("button", { name: "How to play" }));
    const dialog = screen.getByRole("dialog", { name: "How to play" });
    for (const title of ["Keyboard", "Gamepad", "Phone and tablet", "Full screen", "Sound and voice", "Connection"])
      expect(within(dialog).getByRole("heading", { name: title })).toBeInTheDocument();
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
});

describe("custom signaling server", () => {
  it("validates, tests and saves a custom server, then goes back", async () => {
    FakeSocket.reset((env) => (env.type === "rooms_list" ? { type: "rooms" } : undefined));
    renderApp("/rooms");
    await userEvent.click(screen.getByRole("button", { name: "Signaling server" }));
    const dialog = screen.getByRole("dialog", { name: "Signaling server" });
    const input = within(dialog).getByLabelText("Server address");

    await userEvent.clear(input);
    await userEvent.type(input, "ws://signal.mine.example/ws");
    await userEvent.click(within(dialog).getByRole("button", { name: "Test and save" }));
    expect(within(dialog).getByRole("alert")).toHaveTextContent("Only encrypted addresses (wss://) are allowed.");

    await userEvent.clear(input);
    await userEvent.type(input, "wss://signal.mine.example/ws");
    await userEvent.click(within(dialog).getByRole("button", { name: "Test and save" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(localStorage.getItem("go-link.signal-url")).toBe("wss://signal.mine.example/ws");
    expect(screen.getByText("You are using a custom signaling server:")).toBeInTheDocument();
    await waitFor(() => expect(FakeSocket.sockets.at(-1)!.url).toBe("wss://signal.mine.example/ws?v=1"));

    await userEvent.click(screen.getByRole("button", { name: "Back to the official server" }));
    expect(localStorage.getItem("go-link.signal-url")).toBeNull();
    expect(screen.queryByText("You are using a custom signaling server:")).not.toBeInTheDocument();
  });

  it("does not save a server that does not answer", async () => {
    FakeSocket.reset();
    renderApp("/rooms");
    await userEvent.click(screen.getByRole("button", { name: "Signaling server" }));
    const dialog = screen.getByRole("dialog");
    const input = within(dialog).getByLabelText("Server address");
    await userEvent.clear(input);
    await userEvent.type(input, "wss://down.example/ws");
    await userEvent.click(within(dialog).getByRole("button", { name: "Test and save" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("The server did not answer");
    expect(localStorage.getItem("go-link.signal-url")).toBeNull();
  });

  it("never takes the server from the URL", () => {
    FakeSocket.reset();
    renderApp(`/?signal=wss://evil.example/ws`);
    expect(FakeSocket.sockets.every((s) => s.url.startsWith("wss://signal.test/ws"))).toBe(true);
  });
});

describe("local web panel", () => {
  const PANEL = "ws://192.168.1.20:7373/ws";
  const TOKEN = "0b5c7c1e-9d7a-4b8e-8a31-2f6f3c9d1e24"; // gitleaks:allow (test value)

  it("asks for the panel token and links through the device's own socket", async () => {
    localStorage.removeItem("go-link.panel-token");
    FakeSocket.reset((env) => {
      if (env.type === "rooms_list") return { type: "rooms" };
      if (env.type !== "panel") return undefined;
      if (!env.proof) return { type: "panel_nonce", nonce: "n-1" };
      return env.proof === panelProof(TOKEN, "n-1") ? { type: "paired", session_id: "panel", remote: "device" } : { type: "error", error: "invalid panel token" };
    });
    renderApp("/", { panelUrl: PANEL });
    expect(await screen.findByRole("heading", { name: "Device panel" })).toBeInTheDocument();
    const input = screen.getByLabelText("Panel token");
    await userEvent.type(input, "0b5c7c1e-9d7a-4b8e-8a31-2f6f3c9d1e25");
    await userEvent.click(screen.getByRole("checkbox", { name: /I have read and accept/ }));
    await userEvent.click(screen.getByRole("button", { name: "Open the panel" }));
    expect(await screen.findByText("That token is not right. Check it and try again.")).toBeInTheDocument();

    await userEvent.clear(input);
    await userEvent.type(input, TOKEN.toUpperCase());
    await userEvent.click(screen.getByRole("button", { name: "Open the panel" }));
    await waitFor(() => expect(localStorage.getItem("go-link.panel-token")).toBe(TOKEN));
    expect(screen.queryByRole("heading", { name: "Device panel" })).not.toBeInTheDocument();
    // Only proofs went to the device's socket, never the token, and
    // nothing panel related went to signalhub.
    const panelSockets = FakeSocket.sockets.filter((s) => s.url.startsWith("ws://192.168.1.20:7373/ws"));
    const sent = panelSockets.flatMap((s) => s.sent);
    expect(sent.filter((e) => e.type === "panel" && e.proof && !e.mode)).toHaveLength(2);
    // A second socket proves the token for playing in the rooms.
    await waitFor(() =>
      expect(FakeSocket.sockets.filter((s) => s.url.startsWith("ws://192.168.1.20:7373/ws")).flatMap((s) => s.sent).some((e) => e.type === "panel" && e.mode === "room" && e.proof)).toBe(true),
    );
    expect(JSON.stringify(FakeSocket.allSent())).not.toContain(TOKEN);
    // signalhub refuses the panel's origin: nothing goes there.
    expect(FakeSocket.sockets.filter((s) => s.url.startsWith(OFFICIAL))).toHaveLength(0);
  });

  it("joins the device's rooms through the device's own socket", async () => {
    localStorage.setItem("go-link.panel-token", TOKEN);
    FakeSocket.reset((env) => {
      if (env.type === "panel") return env.proof ? { type: "paired", session_id: "panel", remote: "device" } : { type: "panel_nonce", nonce: "n-3" };
      if (env.type === "join") return { type: "joined", session_id: ROOM, room_id: ROOM, remote: "device" };
      return undefined;
    });
    vi.useFakeTimers({ shouldAdvanceTime: true });
    renderApp(`/r/${ROOM}`, { panelUrl: PANEL });
    // A linked browser first waits for its device's status (which says the
    // invitation of its own rooms); without one it joins after the wait.
    await act(() => vi.advanceTimersByTimeAsync(8000));
    vi.useRealTimers();
    await waitFor(() => expect(FakeSocket.allSent().some((e) => e.type === "join" && e.room_id === ROOM)).toBe(true));
    const joinedOn = FakeSocket.sockets.find((s) => s.sent.some((e) => e.type === "join"));
    expect(joinedOn?.url.startsWith("ws://192.168.1.20:7373/ws")).toBe(true);
    // The join came after the socket proved the token for playing.
    const types = joinedOn?.sent.map((e) => (e.type === "panel" ? `panel:${e.mode ?? ""}:${e.proof ? "proof" : "ask"}` : e.type));
    expect(types?.slice(0, 3)).toEqual(["panel::ask", "panel:room:proof", "join"]);
    localStorage.removeItem("go-link.panel-token");
  });

  it("opens one data link on load, not one per connection change", async () => {
    localStorage.setItem("go-link.panel-token", TOKEN);
    FakeSocket.reset((env) =>
      env.type !== "panel" ? undefined : env.proof ? { type: "paired", session_id: "panel", remote: "device" } : { type: "panel_nonce", nonce: "n-4" },
    );
    renderApp("/device", { panelUrl: PANEL });
    // The playing socket opens too; wait until it proved the token.
    await waitFor(() => expect(FakeSocket.allSent().some((e) => e.type === "panel" && e.mode === "room" && e.proof)).toBe(true));
    await new Promise((r) => setTimeout(r, 100));
    expect(FakeSocket.allSent().filter((e) => e.type === "panel" && e.proof && !e.mode)).toHaveLength(1);
    localStorage.removeItem("go-link.panel-token");
  });

  it("has no landing page: the menu is the device's rooms and the device", async () => {
    localStorage.setItem("go-link.panel-token", TOKEN);
    FakeSocket.reset((env) =>
      env.type !== "panel" ? undefined : env.proof ? { type: "paired", session_id: "panel", remote: "device" } : { type: "panel_nonce", nonce: "n-5" },
    );
    renderApp("/how-it-works", { panelUrl: PANEL });
    const menu = await screen.findByRole("navigation", { name: "Main" });
    await waitFor(() => expect(within(menu).getAllByRole("link").map((a) => a.textContent)).toEqual(["Rooms", "My device", "Docs", "Tools"]));
    // A linked panel opens on its rooms.
    expect(await screen.findByRole("heading", { level: 1, name: "Rooms" })).toBeInTheDocument();
    localStorage.removeItem("go-link.panel-token");
  });

  it("comes back by itself with the saved token", async () => {
    localStorage.setItem("go-link.panel-token", TOKEN);
    FakeSocket.reset((env) =>
      env.type !== "panel" ? undefined : env.proof ? { type: "paired", session_id: "panel", remote: "device" } : { type: "panel_nonce", nonce: "n-2" },
    );
    renderApp("/device", { panelUrl: PANEL });
    await waitFor(() => expect(FakeSocket.allSent().some((e) => e.type === "panel" && e.proof === panelProof(TOKEN, "n-2"))).toBe(true));
    expect(screen.queryByRole("heading", { name: "Device panel" })).not.toBeInTheDocument();
    localStorage.removeItem("go-link.panel-token");
  });
});

describe("legal", () => {
  it("shows the terms of use and the privacy policy, linked from the footer", async () => {
    FakeSocket.reset((env) => (env.type === "rooms_list" ? { type: "rooms" } : undefined));
    renderApp("/terms");
    expect(await screen.findByRole("heading", { level: 1, name: "Terms of use" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /zero-content policy/ })).toBeInTheDocument();
    expect(screen.getByText(/MAME® is a registered trademark of Gregory Ember\. go-link Console is not affiliated with, endorsed/)).toBeInTheDocument();
    const footer = screen.getByRole("contentinfo");
    expect(within(footer).getByRole("link", { name: "Privacy policy" })).toHaveAttribute("href", "/privacy");
    expect(within(footer).getByRole("link", { name: "Third-party licenses" })).toHaveAttribute(
      "href",
      "https://github.com/lordbasex/go-link-console/blob/main/THIRD_PARTY_NOTICES.md",
    );
    await userEvent.click(within(footer).getByRole("link", { name: "Privacy policy" }));
    expect(await screen.findByRole("heading", { level: 1, name: "Privacy policy" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Games, voice and chat travel peer to peer" })).toBeInTheDocument();
  });

  it("asks a guest once for the terms, then remembers them", async () => {
    FakeSocket.reset((env) => (env.type === "rooms_list" ? { type: "rooms" } : undefined));
    localStorage.setItem("go-link.terms", JSON.stringify({ version: TERMS_VERSION }));
    renderApp("/g");
    expect(await screen.findByLabelText("PIN")).toBeInTheDocument();
    expect(screen.queryByRole("checkbox", { name: /I have read and accept/ })).not.toBeInTheDocument();
  });
});

describe("docs", () => {
  it("opens the first page, lists every page, filters them and moves page by page", async () => {
    FakeSocket.reset((env) => (env.type === "rooms_list" ? { type: "rooms" } : undefined));
    renderApp("/docs");
    expect(await screen.findByRole("heading", { level: 1, name: "Introduction" })).toBeInTheDocument();
    const nav = screen.getByRole("navigation", { name: "Documentation menu" });
    expect(within(nav).getAllByRole("link")).toHaveLength(docsEn.pages.length);
    expect(within(screen.getByRole("navigation", { name: "On this page" })).getByRole("link", { name: "How it works" })).toHaveAttribute("href", "#how");
    await userEvent.type(within(nav).getByRole("searchbox"), "raspberry");
    expect(within(nav).getAllByRole("link").map((a) => a.textContent)).toContain("Install");
    expect(within(nav).queryByRole("link", { name: "Voice and chat" })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("link", { name: /Next.*Install/ }));
    expect(await screen.findByRole("heading", { level: 1, name: "Install" })).toBeInTheDocument();
    // One tab per system; arrows move between them.
    const tabs = screen.getByRole("tablist", { name: "Your system" });
    const macos = within(tabs).getByRole("tab", { name: "macOS" });
    await userEvent.click(macos);
    expect(screen.getByRole("tabpanel", { name: "macOS" })).toHaveTextContent("macos-universal.dmg");
    await userEvent.keyboard("{ArrowRight}");
    expect(within(tabs).getByRole("tab", { name: "Windows" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tabpanel", { name: "Windows" })).toHaveTextContent("go-link-console.exe");
  });

  it("has the same pages in every language", () => {
    type Blocks = (typeof docsEn)["pages"][number]["blocks"];
    const shape = (blocks: Blocks): string =>
      blocks.map((b) => (b.t === "tabs" ? `tabs(${b.tabs.map((tab) => `${tab.id}:${shape(tab.blocks)}`).join(";")})` : b.t)).join(",");
    const slugs = (d: typeof docsEn) => d.pages.map((p) => `${p.slug}:${shape(p.blocks)}`);
    expect(slugs(docsEs)).toEqual(slugs(docsEn));
    expect(slugs(docsPt)).toEqual(slugs(docsEn));
  });
});
