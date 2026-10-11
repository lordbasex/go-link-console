// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, screen, within } from "@testing-library/react";
import { FakeSocket, renderApp } from "../test-utils";

afterEach(cleanup);

describe("skin editor manual", () => {
  it("is a page of the user guide, listed in its menu", async () => {
    FakeSocket.reset((env) => (env.type === "rooms_list" ? { type: "rooms" } : undefined));
    renderApp("/docs/skin-editor");
    expect(await screen.findByRole("heading", { level: 1, name: "Skin editor" })).toBeInTheDocument();
    const nav = screen.getByRole("navigation", { name: "Documentation menu" });
    expect(within(nav).getByRole("link", { name: "Skin editor" })).toHaveAttribute("href", "/docs/skin-editor");
    expect(within(screen.getByRole("navigation", { name: "On this page" })).getByRole("link", { name: "Keyboard shortcuts" })).toHaveAttribute("href", "#keys");
    expect(screen.getByRole("link", { name: "developer reference" })).toHaveAttribute("href", "https://github.com/lordbasex/go-link-console/blob/main/docs/skins/README.md");
  });
});
