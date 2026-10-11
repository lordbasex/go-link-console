// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { describe, expect, it } from "vitest";
import { editCode, editCodeFromChange, groupCode, parseInvite } from "../src";

const LINK = "https://play.go-link.org/g/AbCdEfGhIjKlMnOpQrStUv";

/** Types `keys` one by one at the end of the field. */
function type(keys: string): { text: string; caret: number } {
  let state = { text: "", caret: 0 };
  for (const k of keys) state = editCode(state.text, state.caret, state.caret, k);
  return state;
}

describe("groupCode", () => {
  it("groups up to nine digits 3-3-3", () => {
    expect(groupCode("")).toBe("");
    expect(groupCode("91")).toBe("91");
    expect(groupCode("9153")).toBe("915 3");
    expect(groupCode("915355636")).toBe("915 355 636");
  });
});

describe("editCode", () => {
  it("groups digit by digit and keeps the caret at the end", () => {
    expect(type("915")).toEqual({ text: "915", caret: 3 });
    expect(type("9153")).toEqual({ text: "915 3", caret: 5 });
    expect(type("915355636")).toEqual({ text: "915 355 636", caret: 11 });
  });

  it("types in the middle and keeps the caret after the new digit", () => {
    // "915 355 6" with the caret after "91".
    expect(editCode("915 355 6", 2, 2, "0")).toEqual({ text: "910 535 56", caret: 3 });
    // After the third digit, before the space.
    expect(editCode("915 355", 3, 3, "7")).toEqual({ text: "915 735 5", caret: 5 });
  });

  it("backspace over a space deletes the digit before it", () => {
    expect(editCode("915 355", 3, 4, "")).toEqual({ text: "913 55", caret: 2 });
    expect(editCode("915 355 636", 7, 8, "")).toEqual({ text: "915 356 36", caret: 6 });
  });

  it("backspace on a digit regroups the rest", () => {
    expect(editCode("915 355 636", 4, 5, "")).toEqual({ text: "915 556 36", caret: 3 });
    expect(editCode("915 3", 4, 5, "")).toEqual({ text: "915", caret: 3 });
  });

  it("deletes a selection across groups", () => {
    expect(editCode("915 355 636", 2, 9, "")).toEqual({ text: "913 6", caret: 2 });
    expect(editCode("915 355 636", 0, 11, "")).toEqual({ text: "", caret: 0 });
  });

  it("pastes a code with or without separators", () => {
    for (const pasted of ["915355636", "915 355 636", "915-355-636", " 915.355.636 "]) {
      expect(editCode("", 0, 0, pasted)).toEqual({ text: "915 355 636", caret: 11 });
    }
  });

  it("keeps nine digits at most", () => {
    expect(editCode("", 0, 0, "1234567890123")).toEqual({ text: "123 456 789", caret: 11 });
    expect(editCode("123 456 789", 11, 11, "5")).toEqual({ text: "123 456 789", caret: 11 });
  });

  it("leaves a pasted link as it is", () => {
    const r = editCode("", 0, 0, LINK);
    expect(r).toEqual({ text: LINK, caret: LINK.length });
    expect(parseInvite(r.text)).toEqual({ invite: "AbCdEfGhIjKlMnOpQrStUv" });
  });

  it("drops letters when the field only takes codes", () => {
    expect(editCode("915", 3, 3, "a5b", false)).toEqual({ text: "915 5", caret: 5 });
  });

  it("gives a code that parseInvite reads", () => {
    expect(parseInvite(type("915355636").text)).toEqual({ code: "915355636" });
  });
});

describe("editCodeFromChange", () => {
  it("works out a typed digit", () => {
    expect(editCodeFromChange("915", "9153", 4)).toEqual({ text: "915 3", caret: 5 });
  });

  it("works out a backspace over a space", () => {
    // "915 |355": the browser removes the space and leaves the caret at 3.
    expect(editCodeFromChange("915 355", "915355", 3)).toEqual({ text: "913 55", caret: 2 });
  });

  it("works out a paste over a selection", () => {
    expect(editCodeFromChange("915 355", "915 355 636", 11)).toEqual({ text: "915 355 636", caret: 11 });
    expect(editCodeFromChange("123", "915355636", 9)).toEqual({ text: "915 355 636", caret: 11 });
  });

  it("works out a typed link", () => {
    expect(editCodeFromChange("", LINK, LINK.length)).toEqual({ text: LINK, caret: LINK.length });
  });
});
