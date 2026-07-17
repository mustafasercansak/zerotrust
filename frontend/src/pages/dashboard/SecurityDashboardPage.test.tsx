import { describe, expect, it, vi } from "vitest";
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import SecurityDashboardPage from "./SecurityDashboardPage";
import { humanizeSecurityLabel } from "./securityDashboardLabels";
import { useMeContext } from "@/contexts/MeContext";
import { api } from "@/lib/api";

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
    securityDashboard: vi.fn(() => new Promise(() => {})),
  },
}));

describe("SecurityDashboardPage", () => {
  it("turns unknown backend codes into readable fallback labels", () => {
    expect(humanizeSecurityLabel("new_device")).toBe("New Device");
    expect(humanizeSecurityLabel("future_risk_signal")).toBe("Future Risk Signal");
  });

  it("denies access to non-admin users", () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "user-1",
      email: "user@example.com",
      first_name: "Regular",
      last_name: "User",
      has_avatar: false,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      locale: "en",
      roles: ["user"],
    });

    render(React.createElement(SecurityDashboardPage));

    expect(screen.getByText("accessDenied")).toBeDefined();
    expect(screen.queryByText("subtitle")).toBeNull();
  });

  it("renders the dashboard shell for administrators", () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "admin-1",
      email: "admin@example.com",
      first_name: "Admin",
      last_name: "User",
      has_avatar: false,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      locale: "en",
      roles: ["admin"],
    });

    render(React.createElement(SecurityDashboardPage));

    expect(screen.getByText("title")).toBeDefined();
    expect(screen.getByText("ranges.24h")).toBeDefined();
    expect(screen.getByText("ranges.30d")).toBeDefined();
    expect(screen.getByRole("progressbar")).toBeDefined();
  });

  it("renders the metrics, charts, and rankings including risk score and blocked countries", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "admin-1",
      email: "admin@example.com",
      first_name: "Admin",
      last_name: "User",
      has_avatar: false,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      locale: "en",
      roles: ["admin"],
    });

    const mockDashboardData = {
      range: "7d",
      since: "2026-01-01T00:00:00Z",
      generated_at: "2026-01-01T00:00:00Z",
      metrics: {
        successful_logins: 10,
        failed_logins: 5,
        lockouts: 1,
        anomalies: 2,
        active_sessions: 3,
        average_risk_score: 42.5,
      },
      auth_activity: [
        { bucket: "2026-01-01T00:00:00Z", success: 5, failure: 2, average_risk_score: 30 },
        { bucket: "2026-01-02T00:00:00Z", success: 5, failure: 3, average_risk_score: 55 },
      ],
      anomaly_breakdown: [{ name: "new_device", count: 2 }],
      login_countries: [{ name: "US", count: 10 }],
      failed_login_ips: [{ name: "1.2.3.4", count: 5 }],
      blocked_countries: [{ name: "CN", count: 3 }],
    };

    vi.mocked(api.securityDashboard).mockResolvedValue(mockDashboardData as any);

    render(React.createElement(SecurityDashboardPage));

    await waitFor(() => {
      expect(screen.getByText("metrics.averageRiskScore")).toBeDefined();
    });

    expect(screen.getByText("blockedCountriesTitle")).toBeDefined();
    expect(screen.getByText("avgRisk")).toBeDefined();
    expect(screen.getByText(/CN/)).toBeDefined();
  });
});
