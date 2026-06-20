import { describe, expect, it, vi } from "vitest";
import React from "react";
import { renderToString } from "react-dom/server";
import { PasswordStrengthBar } from "./PasswordStrengthBar";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

vi.mock("react", async (importOriginal) => {
  const original = await importOriginal<typeof import("react")>();
  return {
    ...original,
    useMemo: (fn: () => unknown) => fn(),
  };
});

function render(password: string) {
  return renderToString(React.createElement(PasswordStrengthBar, { password }));
}

describe("PasswordStrengthBar", () => {
  it("returns empty string when password is empty", () => {
    expect(render("")).toBe("");
  });

  it("score 1 (weak) — length >= 8 only", () => {
    const html = render("abcdefgh");
    expect(html).toContain("passwordStrength.weak");
  });

  it("score 2 (fair) — length >= 8 + uppercase", () => {
    const html = render("Abcdefgh");
    expect(html).toContain("passwordStrength.fair");
  });

  it("score 3 (strong) — length >= 8 + uppercase + digit", () => {
    const html = render("Abcdefg1");
    expect(html).toContain("passwordStrength.strong");
  });

  it("score 4 (veryStrong) — length >= 8 + uppercase + digit + special", () => {
    const html = render("Abcdefg1!");
    expect(html).toContain("passwordStrength.veryStrong");
  });

  it("score 4 (veryStrong) — length >= 14 counts as two criteria", () => {
    // length>=14 (+2), uppercase (+1), digit (+1) → 4
    const html = render("Abcdefghijkl12");
    expect(html).toContain("passwordStrength.veryStrong");
  });

  it("renders segment boxes", () => {
    const html = render("Abcdef1!");
    // 4 Box divs rendered inside the bar container
    const matches = html.match(/MuiBox-root/g) ?? [];
    expect(matches.length).toBeGreaterThanOrEqual(4);
  });

  it("score 0 with no criteria met — falls back to weak label", () => {
    // single lowercase char: no length>=8, no uppercase, no digit, no special → score 0
    const html = render("a");
    expect(html).toContain("passwordStrength.weak");
  });
});
