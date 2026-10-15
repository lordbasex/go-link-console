// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { setLang } from "../../i18n";
import { en } from "../../i18n/en";
import { es } from "../../i18n/es";
import { pt } from "../../i18n/pt";
import { renderApp } from "../../test-utils";

afterEach(() => {
  cleanup();
  setLang("en");
});

describe("the landing in Desk's look", () => {
  it("keeps the ANTÍDOTO screen and its trailer, in a laptop with the app on a phone", async () => {
    const { container } = renderApp("/");
    await screen.findByRole("heading", { level: 1 });
    const screenEl = container.querySelector(".ld-bezel > .lp-screen")!;
    expect(screenEl.querySelector("video.lp-video")).toHaveAttribute("src", "/media/antidoto-loop.mp4");
    expect(screenEl.querySelector("video.lp-video")).toHaveAttribute("poster", "/media/antidoto-poster.jpg");
    expect(screenEl.querySelector("button.lp-trailer-open")).toHaveTextContent(en.landing.trailerOpen);
    expect(container.querySelector(".ld-phone-hero img")).toHaveAttribute("src", "/shots/en/app-room.webp");
    for (const n of en.landing.notes) expect(screen.getByText(n)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: en.landing.trailerOpen }));
    expect(screen.getByRole("dialog", { name: en.landing.trailerTitle })).toBeInTheDocument();
  });

  it("switches the screen to TELETRABAJO, with its loop and trailer in the page's language", async () => {
    setLang("es");
    const { container } = renderApp("/");
    await screen.findByRole("heading", { level: 1 });
    await userEvent.click(screen.getByRole("button", { name: "TELETRABAJO" }));
    expect(screen.getByRole("button", { name: "TELETRABAJO" })).toHaveAttribute("aria-pressed", "true");
    const video = container.querySelector(".lp-screen video.lp-video")!;
    expect(video).toHaveAttribute("src", "/media/teletrabajo-loop-es.mp4");
    expect(video).toHaveAttribute("poster", "/media/teletrabajo-poster-es.jpg");
    await userEvent.click(screen.getByRole("button", { name: es.landing.trailerOpen }));
    const dialog = screen.getByRole("dialog", { name: es.landing.remoteTrailerTitle });
    expect(dialog.querySelector("video")).toHaveAttribute("src", "/media/teletrabajo-trailer-es.mp4");
  });

  it("tells the room and the device in real screenshots, with every feature once", async () => {
    renderApp("/");
    expect(await screen.findByRole("heading", { level: 3, name: en.landing.roomTitle })).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 3, name: en.landing.deviceTitle })).toBeInTheDocument();
    expect(screen.getAllByRole("img", { name: en.gallery.items["win-overview"].alt }).length).toBeGreaterThan(0);
    for (const f of en.landing.features.slice(0, 8)) expect(screen.getAllByText(`${f.title}.`).length).toBeGreaterThan(0);
  });

  it("has the new texts in every language", () => {
    for (const m of [en, es, pt]) {
      expect(m.landing.notes).toHaveLength(4);
      for (const k of ["roomTitle", "roomLead", "deviceTitle", "deviceLead", "flowData", "gamesLabel", "remoteSceneLabel", "remoteTrailerTitle", "remoteTrailerText"] as const) expect(m.landing[k]).not.toBe("");
      for (const k of ["title", "text", "reject", "accept", "settings", "language"] as const) expect(m.consent[k]).not.toBe("");
    }
  });
});
