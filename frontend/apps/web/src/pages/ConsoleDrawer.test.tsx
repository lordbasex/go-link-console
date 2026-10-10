// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, renderHook, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { useState } from "react";
import type { ChatLine } from "@go-link/shared";
import { ConsoleDrawer, type ConsoleDrawerProps, type DrawerTab } from "./ConsoleDrawer";
import { useUnreadChat, type SideActions } from "./RoomSide";
import type { RoomModel } from "./roomModel";

afterEach(() => cleanup());

const model = (over: Partial<RoomModel> = {}): RoomModel => ({
  seats: [
    { name: "Ana", you: true, status: "Playing", tone: "normal" },
    { name: "Bob", you: false, status: "Playing", tone: "normal" },
    null,
    null,
  ],
  queue: [{ pos: "1st", name: "Cleo", you: false, note: "Gets the next free seat" }],
  spectators: [{ name: "Dan", you: false }],
  hands: [],
  chat: [{ name: "Bob", port: 2, role: "P2", text: "Ready?", you: false }],
  me: { kind: "player", ports: [1] },
  voice: "player",
  hearVoice: false,
  ...over,
});

const baseActions = (over: Partial<SideActions> = {}): SideActions => ({
  chatEnabled: true,
  name: "Ana",
  onChat: vi.fn(),
  onName: vi.fn(),
  onSpectate: vi.fn(),
  onQueue: vi.fn(),
  ...over,
});

function props(over: Partial<ConsoleDrawerProps> = {}): ConsoleDrawerProps {
  return {
    open: true,
    onClose: vi.fn(),
    tab: "chat",
    onTab: vi.fn(),
    unread: 0,
    model: model(),
    actions: baseActions(),
    swapFor: () => undefined,
    swapOffers: [],
    onAnswerSwap: vi.fn(),
    fullscreen: false,
    onFullscreen: vi.fn(),
    onVoice: vi.fn(),
    onHelp: vi.fn(),
    ...over,
  };
}

/** The drawer with its tab kept like the room page keeps it. */
function Harness(over: Partial<ConsoleDrawerProps> & { initial?: DrawerTab }) {
  const [tab, setTab] = useState<DrawerTab>(over.initial ?? "chat");
  return <ConsoleDrawer {...props(over)} tab={tab} onTab={setTab} />;
}

const renderIn = (ui: React.ReactElement) => render(<MemoryRouter>{ui}</MemoryRouter>);

const line = (name: string, text: string): ChatLine => ({ kind: "user", name, port: 2, role: "P2", text, ts: Date.now() });

describe("console drawer", () => {
  it("switches between Chat, Players and You, by tap and by arrow keys", async () => {
    renderIn(<Harness />);
    const tabs = screen.getAllByRole("tab");
    expect(tabs.map((b) => b.textContent)).toEqual(["Chat", "Players", "You"]);
    expect(screen.getByRole("tab", { name: "Chat" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tabpanel", { name: "Chat" })).toHaveTextContent("Ready?");
    // The chat has the message box at the bottom.
    expect(screen.getByRole("textbox", { name: "Message" })).toBeInTheDocument();

    await userEvent.click(screen.getByRole("tab", { name: "Players" }));
    expect(screen.getByRole("tab", { name: "Players" })).toHaveAttribute("aria-selected", "true");
    const players = screen.getByRole("tabpanel", { name: "Players" });
    expect(within(players).getAllByRole("listitem")).toHaveLength(4);
    expect(players).toHaveTextContent("Queue: 1 · Spectators: 1");
    expect(players).toHaveTextContent("Cleo");
    expect(players).toHaveTextContent("Dan");
    // Only the selected panel shows.
    expect(screen.queryByRole("tabpanel", { name: "Chat" })).toBeNull();

    // Arrow keys move along the tabs and wrap.
    await userEvent.keyboard("{ArrowRight}");
    expect(screen.getByRole("tab", { name: "You" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: "You" })).toHaveFocus();
    await userEvent.keyboard("{ArrowRight}");
    expect(screen.getByRole("tab", { name: "Chat" })).toHaveAttribute("aria-selected", "true");
    await userEvent.keyboard("{End}");
    expect(screen.getByRole("tab", { name: "You" })).toHaveAttribute("aria-selected", "true");
    await userEvent.keyboard("{ArrowLeft}");
    expect(screen.getByRole("tab", { name: "Players" })).toHaveAttribute("aria-selected", "true");
  });

  it("counts chat lines that arrive while the chat is not visible, and clears them on seeing it", async () => {
    // The room page's wiring: the chat is out of sight on another tab.
    function Counted({ chat }: { chat: ChatLine[] }) {
      const [tab, setTab] = useState<DrawerTab>("players");
      const unread = useUnreadChat(chat, "Ana", tab !== "chat");
      return <ConsoleDrawer {...props()} tab={tab} onTab={setTab} unread={unread} />;
    }
    const first = [{ ...line("Bob", "hi"), ts: Date.now() - 60_000 }];
    const view = renderIn(<Counted chat={first} />);
    const chatTab = () => screen.getByRole("tab", { name: /^Chat/ });
    // What was there before counts as read.
    expect(chatTab()).toHaveAccessibleName("Chat");

    view.rerender(<MemoryRouter><Counted chat={[...first, line("Bob", "a"), line("Cleo", "b"), line("Ana", "mine")]} /></MemoryRouter>);
    expect(chatTab()).toHaveTextContent("2");
    expect(chatTab()).toHaveAccessibleName("Chat, 2 unread messages");

    await userEvent.click(chatTab());
    expect(chatTab()).toHaveAccessibleName("Chat");
    await userEvent.click(screen.getByRole("tab", { name: "Players" }));
    // Seen: going away again starts from zero.
    expect(chatTab()).toHaveAccessibleName("Chat");
  });

  it("wires the You tab's buttons to the room's actions", async () => {
    const p = props({ tab: "you" });
    renderIn(<ConsoleDrawer {...p} />);
    const you = screen.getByRole("tabpanel", { name: "You" });
    expect(you).toHaveTextContent("You are playing");
    expect(within(you).getByRole("textbox", { name: "Your name" })).toHaveValue("Ana");

    await userEvent.click(within(you).getByRole("button", { name: "Volume and voice" }));
    expect(p.onVoice).toHaveBeenCalledTimes(1);
    await userEvent.click(within(you).getByRole("button", { name: "Full screen" }));
    expect(p.onFullscreen).toHaveBeenCalledTimes(1);
    await userEvent.click(within(you).getByRole("button", { name: "How to play" }));
    expect(p.onHelp).toHaveBeenCalledTimes(1);
    expect(within(you).getByRole("link", { name: "Leave room" })).toHaveAttribute("href", "/rooms");

    await userEvent.clear(within(you).getByRole("textbox", { name: "Your name" }));
    await userEvent.type(within(you).getByRole("textbox", { name: "Your name" }), "Anita");
    await userEvent.click(within(you).getByRole("button", { name: "Save" }));
    expect(p.actions.onName).toHaveBeenCalledWith("Anita");
  });

  it("checks the name like the device does before saving it", async () => {
    const p = props({ tab: "you" });
    renderIn(<ConsoleDrawer {...p} />);
    const you = screen.getByRole("tabpanel", { name: "You" });
    const field = within(you).getByRole("textbox", { name: "Your name" });
    await userEvent.clear(field);
    await userEvent.type(field, "Ana 😀");
    expect(field).toHaveAttribute("aria-invalid", "true");
    expect(you).toHaveTextContent("Only letters, digits and spaces (no symbols or emoji).");
    expect(within(you).getByRole("button", { name: "Save" })).toBeDisabled();
    await userEvent.clear(field);
    await userEvent.type(field, "A");
    expect(you).toHaveTextContent("At least 2 characters.");
    expect(within(you).getByRole("button", { name: "Save" })).toBeDisabled();
    await userEvent.type(field, "na   María ");
    expect(field).not.toHaveAttribute("aria-invalid");
    expect(you).toHaveTextContent("9 of 20 characters");
    await userEvent.click(within(you).getByRole("button", { name: "Save" }));
    expect(p.actions.onName).toHaveBeenCalledWith("Ana María");
  });

  it("gives the host Invite and Close the game in the You tab", async () => {
    const guest = props({ tab: "you" });
    const { unmount } = renderIn(<ConsoleDrawer {...guest} />);
    let you = screen.getByRole("tabpanel", { name: "You" });
    expect(within(you).queryByRole("button", { name: "Invite" })).toBeNull();
    expect(within(you).queryByRole("button", { name: "Close game" })).toBeNull();
    unmount();

    const host = props({ tab: "you", onInvite: vi.fn(), onCloseGame: vi.fn() });
    renderIn(<ConsoleDrawer {...host} />);
    you = screen.getByRole("tabpanel", { name: "You" });
    await userEvent.click(within(you).getByRole("button", { name: "Invite" }));
    expect(host.onInvite).toHaveBeenCalledTimes(1);
    await userEvent.click(within(you).getByRole("button", { name: "Close game" }));
    expect(host.onCloseGame).toHaveBeenCalledTimes(1);
  });

  it("offers the seat actions on the Players tab", async () => {
    const onSwap = vi.fn();
    const onSilence = vi.fn();
    const p = props({
      tab: "players",
      swapFor: (port) => (port === 1 ? undefined : { onSwap: () => onSwap(port), waiting: false }),
      onToggleSilence: onSilence,
    });
    renderIn(<ConsoleDrawer {...p} />);
    await userEvent.click(screen.getByRole("button", { name: "Swap controllers: go to P2" }));
    expect(onSwap).toHaveBeenCalledWith(2);
    await userEvent.click(screen.getByRole("button", { name: "Move to P3" }));
    expect(onSwap).toHaveBeenCalledWith(3);
    await userEvent.click(screen.getByRole("button", { name: "Silence Bob" }));
    expect(onSilence).toHaveBeenCalledWith(2);
    await userEvent.click(screen.getByRole("button", { name: "Watch instead" }));
    expect(p.actions.onSpectate).toHaveBeenCalled();
  });

  it("shows the chat is off instead of the message box", () => {
    renderIn(<ConsoleDrawer {...props({ actions: baseActions({ chatOff: true }) })} />);
    expect(screen.getByText("The host turned the chat off for this room.")).toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "Message" })).toBeNull();
  });

  it("closes with Esc and with its close button", async () => {
    const p = props();
    renderIn(<ConsoleDrawer {...p} />);
    await userEvent.keyboard("{Escape}");
    expect(p.onClose).toHaveBeenCalledTimes(1);
    await userEvent.click(screen.getByRole("button", { name: "Back to the game" }));
    expect(p.onClose).toHaveBeenCalledTimes(2);
  });
});

describe("unread chat", () => {
  it("ignores the history and your own lines, and rings for others", () => {
    const ding = vi.fn();
    const old: ChatLine = { ...line("Bob", "old"), ts: Date.now() - 60_000 };
    const { result, rerender } = renderHook(({ chat, hidden }) => useUnreadChat(chat, "Ana", hidden, ding), {
      initialProps: { chat: [] as ChatLine[], hidden: true },
    });
    rerender({ chat: [old], hidden: true });
    expect(result.current).toBe(0);
    rerender({ chat: [old, line("Ana", "me")], hidden: true });
    expect(result.current).toBe(0);
    expect(ding).not.toHaveBeenCalled();
    rerender({ chat: [old, line("Ana", "me"), line("Bob", "new")], hidden: true });
    expect(result.current).toBe(1);
    expect(ding).toHaveBeenCalledTimes(1);
    act(() => rerender({ chat: [old, line("Ana", "me"), line("Bob", "new")], hidden: false }));
    expect(result.current).toBe(0);
  });
});
