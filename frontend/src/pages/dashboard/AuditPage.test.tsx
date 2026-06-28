import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { render, screen, waitFor, cleanup, fireEvent } from "@testing-library/react";
import AuditPage from "./AuditPage";
import { api } from "@/lib/api";
import { useMeContext } from "@/contexts/MeContext";
import { toast } from "sonner";

const mockT = (key: string) => key;

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: mockT,
    i18n: { language: "en" },
  }),
}));

vi.mock("@/contexts/MeContext", () => ({
  useMeContext: vi.fn(),
}));

vi.mock("sonner", () => ({
  toast: {
    error: vi.fn(),
  },
}));

vi.mock("@mui/x-data-grid", () => ({
  DataGrid: (props: any) => {
    const cells: React.ReactNode[] = [];
    if (props.columns && props.rows) {
      for (const row of props.rows) {
        props.getRowId?.(row);
        for (const col of props.columns) {
          if (col.renderCell) {
            cells.push(React.createElement(React.Fragment, { key: `${row.id}-${col.field}` }, col.renderCell({ row })));
          }
        }
      }
    }
    return React.createElement("div", null, `DataGrid rows: ${props.rows?.length ?? 0}`, cells);
  },
}));

describe("AuditPage page component", () => {
  beforeEach(() => {
    vi.useRealTimers();
    vi.mocked(toast.error).mockClear();
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  const getMockTrends = () => [
    { date: "2026-06-01", success: 5, failure: 0 },
    { date: "2026-06-02", success: 8, failure: 2 },
    { date: "Today", success: 0, failure: 0 },
  ];

  const getSingleTrend = () => [
    { date: "2026-06-01", success: 1, failure: 0 },
  ];

  const getMockAuditLogs = (includeEdgeCases = false): any => ({
    data: [
      {
        id: "a1",
        action: "auth.login",
        resource: "users",
        user_id: "u1",
        user_email: "alice@example.com",
        ip_address: "1.1.1.1",
        user_agent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0",
        created_at: "2026-06-04T12:00:00Z",
        metadata: {
          outcome: "success",
          status: 200,
          reason: "OK",
          location: { city: "New York", country: "US" },
          client_info: { browser: "Chrome", browser_version: "120.0", os: "Windows", os_version: "10" },
        },
      },
      {
        id: "a_loc_partial",
        action: "auth.login",
        created_at: "2026-06-04T11:30:00Z",
        ip_address: "3.3.3.3",
        metadata: { location: { country: "TR" } },
      },
      {
        id: "a2",
        action: "users.create",
        resource: "users",
        user_id: "u2",
        user_email: "",
        ip_address: "2.2.2.2",
        user_agent: "Brave/1.0",
        created_at: "2026-06-04T11:00:00Z",
        metadata: {
          outcome: "failure",
          status: 400,
          reason: "Bad Request",
        },
      },
      {
        id: "a_status_only",
        action: "users.update",
        created_at: "2026-06-04T10:30:00Z",
        metadata: { status: 201 },
      },
      {
        id: "a_reason_only",
        action: "users.update",
        created_at: "2026-06-04T10:15:00Z",
        metadata: { reason: "Manual intervention" },
      },
      {
        id: "a3",
        action: "users.update",
        resource: "users",
        user_id: "",
        user_email: "",
        ip_address: "",
        user_agent: "curl/7.68.0",
        created_at: "2026-06-04T10:00:00Z",
        metadata: {},
      },
      {
        id: "a4",
        action: "users.delete",
        resource: "users",
        user_agent: "PostmanRuntime/7.26.8",
        created_at: "2026-06-04T09:00:00Z",
      },
      {
        id: "a5",
        action: "users.delete",
        resource: "users",
        user_agent: "Go-http-client/1.1",
        created_at: "2026-06-04T08:00:00Z",
      },
      {
        id: "a6",
        action: "users.delete",
        resource: "users",
        user_agent: "python-requests/2.25.1",
        created_at: "2026-06-04T07:00:00Z",
      },
      {
        id: "a7",
        action: "users.delete",
        resource: "users",
        user_agent: "OPR/72.0",
        created_at: "2026-06-04T06:00:00Z",
      },
      {
        id: "a8",
        action: "users.delete",
        resource: "users",
        user_agent: "Edg/86.0",
        created_at: "2026-06-04T05:00:00Z",
      },
      {
        id: "a9",
        action: "users.delete",
        resource: "users",
        user_agent: "Firefox/82.0",
        created_at: "2026-06-04T04:00:00Z",
      },
      {
        id: "a10",
        action: "users.delete",
        resource: "users",
        user_agent: "Safari/14.0",
        created_at: "2026-06-04T03:00:00Z",
      },
      {
        id: "a11",
        action: "users.delete",
        resource: "users",
        user_agent: "Mozilla/5.0 (iPhone; CPU iPhone OS 14_0 like Mac OS X)",
        created_at: "2026-06-04T02:00:00Z",
      },
      {
        id: "a12",
        action: "users.delete",
        resource: "users",
        user_agent: "Mozilla/5.0 (Linux; Android 10)",
        created_at: "2026-06-04T01:00:00Z",
      },
      ...(includeEdgeCases ? [
        {
          id: "a_minimal",
          action: "system.ping",
          user_agent: "",
          metadata: {},
          created_at: "2026-06-04T00:30:00Z",
        },
        {
          id: "a_ua_mac",
          action: "users.update",
          user_agent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)",
          metadata: { client_info: { browser: "Chrome" } },
          created_at: "2026-06-04T00:15:00Z",
        },
        {
          id: "a_ua_win",
          action: "users.update",
          user_agent: "Windows NT 10.0",
          created_at: "2026-06-04T00:10:00Z",
        },
        {
          id: "a_ua_android",
          action: "users.update",
          user_agent: "Android 13",
          created_at: "2026-06-04T00:05:00Z",
        },
        {
          id: "a_ua_chrome_only",
          action: "system.ping",
          user_agent: "Mozilla/5.0 Chrome/120.0",
          created_at: "2026-06-04T00:04:00Z",
        },
        {
          id: "a_ua_chromium",
          action: "system.ping",
          user_agent: "Mozilla/5.0 Chrome/120.0 Chromium/120.0",
          created_at: "2026-06-04T00:03:00Z",
        },
        {
          id: "a_ua_iphone",
          action: "system.ping",
          user_agent: "Mozilla/5.0 (iPhone; CPU iPhone OS 14_0)",
          created_at: "2026-06-04T00:02:00Z",
        },
        {
          id: "a_ua_linux",
          action: "system.ping",
          user_agent: "Mozilla/5.0 (X11; Linux x86_64)",
          created_at: "2026-06-04T00:01:00Z",
        },
        {
          id: "a_ua_unknown",
          action: "system.ping",
          user_agent: "Some Custom Browser",
          created_at: "2026-06-04T00:00:00Z",
        },
      ] : []),
    ],
    total: 12,
  });

  it("renders page, trends, and processes all user agents and metadata cases", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u123", roles: ["admin"] } as any);
    const trendsSpy = vi.spyOn(api, "listAuditLogTrends").mockResolvedValue(getMockTrends());
    const listSpy = vi.spyOn(api, "listAuditLog").mockResolvedValue(getMockAuditLogs(true));

    render(<AuditPage />);

    await waitFor(() => {
      expect(screen.getByText("trendsTitle")).toBeDefined();
    });

    expect(trendsSpy).toHaveBeenCalled();
    expect(listSpy).toHaveBeenCalled();
  });

  it("renders a single trend point without invalid SVG coordinates", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u123", roles: ["admin"] } as any);
    vi.spyOn(api, "listAuditLogTrends").mockResolvedValue(getSingleTrend());
    vi.spyOn(api, "listAuditLog").mockResolvedValue({ data: [], total: 0 });

    const { container } = render(<AuditPage />);

    await waitFor(() => {
      expect(screen.getByText("trendsTitle")).toBeDefined();
    });

    const svg = container.querySelector("svg");
    expect(svg?.outerHTML).not.toContain("Infinity");
    expect(svg?.outerHTML).not.toContain("NaN");
  });

  it("renders unknown outcome values without relabeling them as failures", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u123", roles: ["admin"] } as any);
    vi.spyOn(api, "listAuditLogTrends").mockResolvedValue([]);
    vi.spyOn(api, "listAuditLog").mockResolvedValue({
      data: [{
        id: "a_partial",
        action: "auth.login",
        resource: "users",
        created_at: "2026-06-04T12:00:00Z",
        metadata: { outcome: "partial" },
      }],
      total: 1,
    } as any);

    render(<AuditPage />);

    await waitFor(() => {
      expect(screen.getByText("partial")).toBeDefined();
    });
    expect(screen.queryByText("failure")).toBeNull();
  });

  it("handles loading state and empty trend data gracefully", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u123", roles: ["admin"] } as any);
    vi.spyOn(api, "listAuditLogTrends").mockResolvedValue([]);
    vi.spyOn(api, "listAuditLog").mockResolvedValue({ data: [], total: 0 });

    render(<AuditPage />);
    expect(screen.getByRole("progressbar")).toBeDefined();

    await waitFor(() => {
      expect(screen.queryByRole("progressbar")).toBeNull();
    });

    expect(screen.queryByText("trendsTitle")).toBeNull();
  });

  it("renders as non-admin showing no trends table", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u123", roles: ["user"] } as any);
    vi.spyOn(api, "listAuditLog").mockResolvedValue({ data: [], total: 0 });

    render(<AuditPage />);

    await waitFor(() => {
      expect(screen.queryByText("trendsTitle")).toBeNull();
    });
  });

  it("renders as non-admin with empty roles array safely", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u123", roles: [] } as any);
    vi.spyOn(api, "listAuditLog").mockResolvedValue({ data: [], total: 0 });

    render(<AuditPage />);

    await waitFor(() => {
      expect(screen.queryByText("trendsTitle")).toBeNull();
    });
  });

  it("handles null user context safely", async () => {
    vi.mocked(useMeContext).mockReturnValue(null);
    vi.spyOn(api, "listAuditLog").mockResolvedValue({ data: [], total: 0 });

    render(<AuditPage />);

    await waitFor(() => {
      expect(screen.getByText("accessDenied")).toBeDefined();
      expect(screen.queryByText("trendsTitle")).toBeNull();
    });
  });

  it("handles trends fetch failure gracefully", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u123", roles: ["admin"] } as any);
    vi.spyOn(api, "listAuditLogTrends").mockRejectedValue(new Error("fetch failed"));
    vi.spyOn(api, "listAuditLog").mockResolvedValue({ data: [], total: 0 });

    render(<AuditPage />);

    await waitFor(() => {
      expect(screen.queryByText("trendsTitle")).toBeNull();
    });
  });

  it("does not update the trends chart after its refresh effect is cleaned up", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u123", roles: ["admin"] } as any);
    let resolveTrends: (value: any) => void = () => {};
    const trendsSpy = vi.spyOn(api, "listAuditLogTrends").mockReturnValue(new Promise((resolve) => {
      resolveTrends = resolve;
    }));
    vi.spyOn(api, "listAuditLog").mockResolvedValue({ data: [], total: 0 });

    const { unmount } = render(<AuditPage />);
    expect(screen.getByRole("progressbar")).toBeDefined();

    unmount();
    resolveTrends(getMockTrends());
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();

    expect(trendsSpy).toHaveBeenCalled();
  });

  it("renders export button for admin users", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u123", roles: ["admin"] } as any);
    vi.spyOn(api, "listAuditLogTrends").mockResolvedValue([]);
    vi.spyOn(api, "listAuditLog").mockResolvedValue({ data: [], total: 0 });

    render(<AuditPage />);

    await waitFor(() => {
      expect(screen.getByTestId("export-button")).toBeDefined();
    });
  });

  it("exports with the active audit tab filters", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u123", roles: ["admin"] } as any);
    vi.spyOn(api, "listAuditLogTrends").mockResolvedValue([]);
    const listSpy = vi.spyOn(api, "listAuditLog").mockResolvedValue({ data: [], total: 0 });
    const auditExportSpy = vi.spyOn(api.admin, "auditExport").mockResolvedValue({
      ok: true,
      blob: vi.fn().mockResolvedValue(new Blob(["time\n"], { type: "text/csv" })),
    } as any);
    vi.stubGlobal("URL", {
      ...URL,
      createObjectURL: vi.fn(() => "blob:audit"),
      revokeObjectURL: vi.fn(),
    });

    render(<AuditPage />);

    await waitFor(() => {
      expect(screen.getByTestId("export-button")).toBeDefined();
    });

    fireEvent.click(screen.getByText("tabFailures"));

    await waitFor(() => {
      expect(listSpy).toHaveBeenCalledWith(expect.objectContaining({
        filters: { outcome: "failure" },
      }));
    });

    fireEvent.click(screen.getByTestId("export-button"));
    fireEvent.click(await screen.findByText("exportCsv"));

    await waitFor(() => {
      expect(auditExportSpy).toHaveBeenCalledWith({ format: "csv", outcome: "failure" });
    });
  });

  it("shows a toast when audit export fails", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u123", roles: ["admin"] } as any);
    vi.spyOn(api, "listAuditLogTrends").mockResolvedValue([]);
    vi.spyOn(api, "listAuditLog").mockResolvedValue({ data: [], total: 0 });
    vi.spyOn(api.admin, "auditExport").mockResolvedValue(new Response("nope", { status: 500 }) as any);

    render(<AuditPage />);

    await waitFor(() => {
      expect(screen.getByTestId("export-button")).toBeDefined();
    });

    fireEvent.click(screen.getByTestId("export-button"));
    fireEvent.click(await screen.findByText("exportCsv"));

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("exportFailed");
    });
  });

  it("does not render export button for non-admin users", async () => {
    vi.mocked(useMeContext).mockReturnValue({ id: "u123", roles: ["user"] } as any);
    vi.spyOn(api, "listAuditLog").mockResolvedValue({ data: [], total: 0 });

    render(<AuditPage />);

    await waitFor(() => {
      expect(screen.queryByTestId("export-button")).toBeNull();
    });
  });
});
