import { describe, expect, it, vi } from "vitest";
import React from "react";
import HomePage from "./HomePage";
import { useMeContext } from "@/contexts/MeContext";
import { renderToString } from "react-dom/server";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    i18n: { language: "en" },
    t: (key: string, options?: any) => {
      if (options && options.email) {
        return `${key} - welcome:${options.email}`;
      }
      return key;
    },
  }),
}));

vi.mock("@/contexts/MeContext", () => ({
  useMeContext: vi.fn(),
  MeContext: React.createContext(null),
}));

describe("HomePage component", () => {
  it("renders null if user info is not loaded yet", () => {
    vi.mocked(useMeContext).mockReturnValue(null);
    const html = renderToString(React.createElement(HomePage));
    expect(html).toBe("");
  });

  it("renders welcome text, user ID, locale, and roles correctly", () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "user-abc-123",
      email: "testuser@example.com",
      first_name: "Test",
      last_name: "User",
      is_active: true,
      locale: "en",
      roles: ["admin", "operator"],
      created_at: "2026-06-04T12:00:00Z",
      updated_at: "2026-06-04T12:00:00Z",
    });

    const html = renderToString(React.createElement(HomePage));

    expect(html).toContain("title"); // t("title")
    expect(html).toContain("welcome - welcome:testuser@example.com"); // t("welcome", { email })
    expect(html).toContain("user-abc-123");
    expect(html).toContain("EN"); // i18n.language.toUpperCase()
    expect(html).toContain("admin");
    expect(html).toContain("operator");
  });
});
