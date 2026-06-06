import { describe, expect, it, vi } from "vitest";
import React from "react";
import { renderToString } from "react-dom/server";
import SecurityDashboardPage from "./SecurityDashboardPage";
import { useMeContext } from "@/contexts/MeContext";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: "en" },
  }),
}));

vi.mock("@/contexts/MeContext", () => ({
  useMeContext: vi.fn(),
}));

vi.mock("@/lib/api", () => ({
  api: {
    securityDashboard: vi.fn(),
  },
}));

describe("SecurityDashboardPage", () => {
  it("denies access to non-admin users", () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "user-1",
      email: "user@example.com",
      first_name: "Regular",
      last_name: "User",
      has_avatar: false,
      locale: "en",
      roles: ["user"],
    });

    const html = renderToString(React.createElement(SecurityDashboardPage));

    expect(html).toContain("accessDenied");
    expect(html).not.toContain("subtitle");
  });

  it("renders the dashboard shell for administrators", () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "admin-1",
      email: "admin@example.com",
      first_name: "Admin",
      last_name: "User",
      has_avatar: false,
      locale: "en",
      roles: ["admin"],
    });

    const html = renderToString(React.createElement(SecurityDashboardPage));

    expect(html).toContain("title");
    expect(html).toContain("ranges.24h");
    expect(html).toContain("ranges.30d");
    expect(html).toContain("progressbar");
  });
});
