// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

const sendToDevice = vi.fn();
vi.mock("../signal/SignalProvider", () => ({
  useSignal: () => ({ sendToDevice, onDeviceMessage: () => () => {} }),
}));

import { InviteDialog } from "./InviteDialog";

const room = { id: "test", name: "Test pattern", invite: "AAAAAAAAAAAAAAAAAAAAAA", inviteCode: "123456789" };
const key = "K".repeat(43);

afterEach(() => {
  cleanup();
  sendToDevice.mockClear();
  localStorage.clear();
});

describe("InviteDialog", () => {
  it("closes with Escape, also after the page re-renders it", () => {
    const onClose = vi.fn();
    const { rerender } = render(<InviteDialog room={room} fallbackUrl="" onAction={() => {}} onClose={() => onClose()} />);
    // The room page passes a new onClose on every render.
    rerender(<InviteDialog room={room} fallbackUrl="" onAction={() => {}} onClose={() => onClose()} />);
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    fireEvent.keyDown(document.activeElement ?? window, { key: "Escape", code: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("makes a group QR code in the same dialog, asking the host first by default", () => {
    render(<InviteDialog room={room} fallbackUrl="" onAction={() => {}} onClose={() => {}} />);
    expect(sendToDevice).toHaveBeenCalledWith({ type: "invite", id: "test" }); // one person: a PIN
    fireEvent.click(screen.getByRole("button", { name: "Group" }));
    expect(screen.getByRole("checkbox", { name: /Ask me first/ })).toBeChecked();
    fireEvent.change(screen.getByRole("combobox", { name: "Up to" }), { target: { value: "5" } });
    fireEvent.click(screen.getByRole("button", { name: "Make the group QR code" }));
    expect(sendToDevice).toHaveBeenLastCalledWith({ type: "invite_group", id: "test", uses: 5, hours: 6, approval: true });
    // The choice stays for the next time.
    expect(localStorage.getItem("go-link.invite-mode")).toBe("group");
  });

  it("shows the room's group QR code with the key after #, and stops it", () => {
    const groupInvite = { key, uses: 10, used: 3, expiresAt: new Date(Date.now() + 3600e3).toISOString(), approval: true };
    localStorage.setItem("go-link.invite-mode", "group");
    render(<InviteDialog room={{ ...room, groupInvite }} fallbackUrl="" onAction={() => {}} onClose={() => {}} />);
    expect(screen.getByText("3 of 10 people came in")).toBeInTheDocument();
    expect(screen.getByText("You let each person in")).toBeInTheDocument();
    expect(screen.getByText(/#k=…/)).toBeInTheDocument(); // the key is not printed whole
    fireEvent.click(screen.getByRole("button", { name: "Stop this QR code" }));
    expect(sendToDevice).toHaveBeenLastCalledWith({ type: "end_group", id: "test" });
  });

  it("makes the spectators' QR code with its own link, and switches the lighter video", () => {
    const onSaver = vi.fn();
    const groupInvite = { key, uses: 10, used: 3, expiresAt: new Date(Date.now() + 3600e3).toISOString(), approval: true };
    render(<InviteDialog room={{ ...room, groupInvite }} fallbackUrl="" onAction={() => {}} onClose={() => {}} saver={{ on: true, onChange: onSaver }} />);
    fireEvent.click(screen.getByRole("button", { name: "Spectators" }));
    // The players' group link is not the spectators' one.
    expect(screen.queryByText("3 of 10 people came in")).toBeNull();
    expect(screen.getByRole("checkbox", { name: /Ask me first/ })).toBeChecked();
    fireEvent.click(screen.getByRole("button", { name: "Make the spectators' QR code" }));
    expect(sendToDevice).toHaveBeenLastCalledWith({ type: "invite_group", id: "test", uses: 10, hours: 6, approval: true, watch: true });
    const sw = screen.getByRole("switch", { name: "Lighter video for spectators" });
    expect(sw).toHaveAttribute("aria-checked", "true");
    fireEvent.click(sw);
    expect(onSaver).toHaveBeenCalledWith(false);
  });

  it("shows and stops the room's spectators' QR code", () => {
    const watchInvite = { key, uses: 20, used: 1, expiresAt: new Date(Date.now() + 3600e3).toISOString(), approval: false, watch: true };
    localStorage.setItem("go-link.invite-mode", "watch");
    render(<InviteDialog room={{ ...room, watchInvite }} fallbackUrl="" onAction={() => {}} onClose={() => {}} />);
    expect(screen.getByText("1 of 20 people came in")).toBeInTheDocument();
    // Without the room's state the lighter video switch is not shown.
    expect(screen.queryByRole("switch")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Stop this QR code" }));
    expect(sendToDevice).toHaveBeenLastCalledWith({ type: "end_group", id: "test", watch: true });
  });
});
