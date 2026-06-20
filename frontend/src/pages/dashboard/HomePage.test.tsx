import { describe, expect, it, vi } from "vitest";
import React from "react";
import HomePage from "./HomePage";
import { useMeContext } from "@/contexts/MeContext";
import { renderToString } from "react-dom/server";

vi.mock("react-router-dom", () => ({
  useNavigate: () => vi.fn(),
}));

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

const baseMeData = {
  user_id: "user-abc-123",
  email: "testuser@example.com",
  first_name: "Test",
  last_name: "User",
  has_avatar: false,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-06-01T00:00:00Z",
  locale: "en",
  roles: ["admin", "operator"],
};

describe("HomePage component", () => {
  it("renders null if user info is not loaded yet", () => {
    vi.mocked(useMeContext).mockReturnValue(null);
    const html = renderToString(React.createElement(HomePage));
    expect(html).toBe("");
  });

  it("renders welcome text, user ID, locale, and roles correctly", () => {
    vi.mocked(useMeContext).mockReturnValue(baseMeData);

    const html = renderToString(React.createElement(HomePage));

    expect(html).toContain("Test User");
    expect(html).toContain("testuser@example.com");
    expect(html).toContain("user-abc-123");
    expect(html).toContain("EN");
    expect(html).toContain("admin");
    expect(html).toContain("operator");
  });

  it("renders joined row using created_at from MeData", () => {
    vi.mocked(useMeContext).mockReturnValue(baseMeData);
    const html = renderToString(React.createElement(HomePage));
    expect(html).toContain("joined");
    expect(html).toContain("2026"); // formatDate output contains the year
  });

  it("renders locale label via t() translation key (not hardcoded)", () => {
    vi.mocked(useMeContext).mockReturnValue(baseMeData);
    const html = renderToString(React.createElement(HomePage));
    expect(html).toContain("locale");
  });

  it("renders userId label via t() translation key", () => {
    vi.mocked(useMeContext).mockReturnValue(baseMeData);
    const html = renderToString(React.createElement(HomePage));
    expect(html).toContain("userId");
  });

  it("uses email as display name when first_name and last_name are both empty", () => {
    vi.mocked(useMeContext).mockReturnValue({ ...baseMeData, first_name: "", last_name: "" });
    const html = renderToString(React.createElement(HomePage));
    expect(html).toContain("testuser@example.com");
  });

  it("renders initials from email when no name parts are present", () => {
    vi.mocked(useMeContext).mockReturnValue({ ...baseMeData, first_name: "", last_name: "" });
    const html = renderToString(React.createElement(HomePage));
    expect(html).toContain("TE"); // "testuser" → slice(0,2).toUpperCase()
  });

  it("renders avatar src when has_avatar is true", () => {
    vi.mocked(useMeContext).mockReturnValue({ ...baseMeData, has_avatar: true });
    const html = renderToString(React.createElement(HomePage));
    expect(html).toContain("/api/v1/me/avatar");
  });

  it("renders securityTitle, sessionsTitle, and activityTitle section headings", () => {
    vi.mocked(useMeContext).mockReturnValue(baseMeData);
    const html = renderToString(React.createElement(HomePage));
    expect(html).toContain("securityTitle");
    expect(html).toContain("sessionsTitle");
    expect(html).toContain("activityTitle");
  });

  it("renders mfaManage link when mfaEnabled is null (initial state)", () => {
    vi.mocked(useMeContext).mockReturnValue(baseMeData);
    const html = renderToString(React.createElement(HomePage));
    // mfaEnabled initial value is null → t("mfaManage") branch (not mfaSetup)
    expect(html).toContain("mfaManage");
  });

  it("renders user with no roles without crashing", () => {
    vi.mocked(useMeContext).mockReturnValue({ ...baseMeData, roles: [] });
    const html = renderToString(React.createElement(HomePage));
    expect(html).toContain("user-abc-123");
  });

  it("renders adminPostureTitle and healthTitle for admin users", () => {
    vi.mocked(useMeContext).mockReturnValue({ ...baseMeData, roles: ["admin"] });
    const html = renderToString(React.createElement(HomePage));
    expect(html).toContain("adminPostureTitle");
    expect(html).toContain("healthTitle");
  });

  it("does not render adminPostureTitle or healthTitle for non-admin users", () => {
    vi.mocked(useMeContext).mockReturnValue({ ...baseMeData, roles: ["user"] });
    const html = renderToString(React.createElement(HomePage));
    expect(html).not.toContain("adminPostureTitle");
    expect(html).not.toContain("healthTitle");
  });

  it("renders health skeleton placeholders while loading for admin", () => {
    vi.mocked(useMeContext).mockReturnValue({ ...baseMeData, roles: ["admin"] });
    const html = renderToString(React.createElement(HomePage));
    // health data loads async; initial render shows Skeleton elements
    expect(html).toContain("MuiSkeleton");
  });
});
