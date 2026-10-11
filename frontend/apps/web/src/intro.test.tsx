// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { DevilButton } from "./components/Headers";
import { JoinForm } from "./components/JoinForm";
import { installIntro } from "./intro";

/** Counts the notes the coin sound schedules. */
function fakeAudio() {
  const started: number[] = [];
  const node = () => ({ connect: (n: unknown) => n ?? node(), gain: { setValueAtTime() {}, exponentialRampToValueAtTime() {} } });
  class FakeContext {
    state = "running";
    currentTime = 0;
    destination = {};
    createGain = node;
    createOscillator() {
      return { ...node(), type: "", frequency: { setValueAtTime: (f: number) => started.push(f) }, start() {}, stop() {} };
    }
  }
  vi.stubGlobal("AudioContext", FakeContext);
  return started;
}

beforeEach(() => {
  document.body.innerHTML = '<div id="splash"></div>';
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("startup intro", () => {
  it("plays the coin and skips the intro on the first key", () => {
    const notes = fakeAudio();
    installIntro();
    fireEvent.keyDown(window, { key: "a" });
    expect(document.getElementById("splash")?.classList.contains("is-gone")).toBe(true);
    expect(notes.map(Math.round)).toEqual([880, 1109, 1397]);
    // Only the first gesture counts.
    fireEvent.keyDown(window, { key: "b" });
    expect(notes).toHaveLength(3);
  });

  it("the header's devil (in place of the old sound switch) invites to destroy the page", () => {
    render(
      <MemoryRouter>
        <DevilButton />
      </MemoryRouter>,
    );
    expect(screen.getByRole("button", { name: "Destroy this page!" })).toHaveAttribute("data-tip", "Destroy this page!");
  });
});

describe("room code field", () => {
  it("groups the code 3-3-3 while typing and keeps links as pasted", async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <JoinForm />
      </MemoryRouter>,
    );
    const field = screen.getByLabelText(/code/i, { selector: "input:not([autocomplete='one-time-code'])" }) as HTMLInputElement;
    await user.type(field, "9153556");
    expect(field.value).toBe("915 355 6");
    await user.type(field, "36");
    expect(field.value).toBe("915 355 636");
    // Backspace right after a space deletes the digit before it.
    field.setSelectionRange(4, 4);
    await user.keyboard("{Backspace}");
    expect(field.value).toBe("913 556 36");
    expect(field.selectionStart).toBe(2);

    await user.clear(field);
    await user.click(field);
    await user.paste("https://play.go-link.org/g/AbCdEfGhIjKlMnOpQrStUv");
    expect(field.value).toBe("https://play.go-link.org/g/AbCdEfGhIjKlMnOpQrStUv");
  });
});
