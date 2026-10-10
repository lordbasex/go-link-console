// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { GuestMenu, type HostTools } from "./GuestMenu";
import { PlayersCapsule } from "./PlayersCapsule";
import type { SeatCard } from "../pages/roomModel";

afterEach(() => cleanup());

const seat = (name: string, you = false, peer?: string): SeatCard => ({ name, you, status: "Playing", tone: "normal", ...(peer ? { peer } : {}) });

const tools = (seats: (SeatCard | null)[]) => ({
  seats,
  onPromote: vi.fn<HostTools["onPromote"]>(),
  onLater: vi.fn<HostTools["onLater"]>(),
  onRemove: vi.fn<HostTools["onRemove"]>(),
});

describe("raised hands in the players capsule", () => {
  it("shows the host the hands and plays a friend at once on a free seat", async () => {
    const seats = [seat("Ana", true), null];
    const tl = tools(seats);
    render(<PlayersCapsule seats={seats} swapFor={() => undefined} hands={[{ peer: "p-sol", name: "Sol" }, { peer: "p-leo", name: "Leo" }]} hostTools={tl} />);
    await userEvent.click(screen.getByRole("button", { name: "2 raised hands" }));
    const sol = screen.getByRole("group", { name: "Sol raised a hand" });
    expect(sol).toBeInTheDocument();
    await userEvent.click(screen.getAllByRole("menuitem", { name: "Play now" })[0]!);
    expect(tl.onPromote).toHaveBeenCalledWith("p-sol", "play", 2);
  });

  it("asks whose seat when the room is full, and answers not now and to the queue", async () => {
    const seats = [seat("Ana", true, "p-ana"), seat("Bob", false, "p-bob")];
    const tl = tools(seats);
    render(<PlayersCapsule seats={seats} swapFor={() => undefined} hands={[{ peer: "p-sol", name: "Sol" }]} hostTools={tl} />);
    await userEvent.click(screen.getByRole("button", { name: "1 raised hand" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Play now" }));
    expect(tl.onPromote).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole("menuitem", { name: /Play at P2.*instead of Bob/ }));
    expect(tl.onPromote).toHaveBeenCalledWith("p-sol", "play", 2);

    await userEvent.click(screen.getByRole("button", { name: "1 raised hand" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Not now" }));
    expect(tl.onLater).toHaveBeenCalledWith("p-sol");
    await userEvent.click(screen.getByRole("button", { name: "1 raised hand" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "To the queue" }));
    expect(tl.onPromote).toHaveBeenCalledWith("p-sol", "queue");
  });

  it("takes a seated guest out of the room, never the host itself", async () => {
    const seats = [seat("Ana", true, "p-ana"), seat("Bob", false, "p-bob")];
    const tl = tools(seats);
    render(<PlayersCapsule seats={seats} swapFor={() => undefined} hostTools={tl} />);
    expect(screen.queryByRole("button", { name: /raised hand/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /P1 · Ana/ })).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: /P2 · Bob/ }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Remove Bob from the room" }));
    expect(tl.onRemove).toHaveBeenCalledWith("p-bob");
  });
});

describe("the host's menu beside someone", () => {
  it("offers the queue and not now only to a spectator with a hand", async () => {
    const tl = tools([seat("Ana", true), null]);
    const { rerender } = render(<GuestMenu tools={tl} peer="p-cid" name="Cid" spectator={false} />);
    await userEvent.click(screen.getByRole("button", { name: "What to do with Cid" }));
    expect(screen.queryByRole("menuitem", { name: "To the queue" })).toBeNull();
    expect(screen.queryByRole("menuitem", { name: "Not now" })).toBeNull();
    await userEvent.click(screen.getByRole("menuitem", { name: "Play now" }));
    expect(tl.onPromote).toHaveBeenCalledWith("p-cid", "play", 2);

    rerender(<GuestMenu tools={tl} peer="p-sol" name="Sol" spectator hand />);
    await userEvent.click(screen.getByRole("button", { name: "What to do with Sol" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Not now" }));
    expect(tl.onLater).toHaveBeenCalledWith("p-sol");
    await userEvent.click(screen.getByRole("button", { name: "What to do with Sol" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Remove Sol from the room" }));
    expect(tl.onRemove).toHaveBeenCalledWith("p-sol");
  });
});
