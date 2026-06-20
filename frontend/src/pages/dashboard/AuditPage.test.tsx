import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import AuditPage from "./AuditPage";
import { api } from "@/lib/api";
import { useMeContext } from "@/contexts/MeContext";
import { renderToString } from "react-dom/server";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: "en" },
  }),
}));

// State Mocking System
let stateStore: any = {};
let stateSetters: any = {};
let callIdx = 0;
let effectCleanups: Array<() => void> = [];

vi.mock("react", async (importOriginal) => {
  const original = await importOriginal<typeof import("react")>();
  return {
    ...original,
    useState: (init: any) => {
      const idx = callIdx;
      callIdx++;
      if (!(idx in stateStore)) {
        stateStore[idx] = init;
      }
      stateSetters[idx] = (newVal: any) => {
        if (typeof newVal === "function") {
          stateStore[idx] = newVal(stateStore[idx]);
        } else {
          stateStore[idx] = newVal;
        }
      };
      if (callIdx >= 25) {
        callIdx = 0;
      }
      return [stateStore[idx], stateSetters[idx]];
    },
    useEffect: (fn: any) => {
      const cleanup = fn();
      if (typeof cleanup === "function") effectCleanups.push(cleanup);
    },
  };
});

vi.mock("@/contexts/MeContext", () => ({
  useMeContext: vi.fn(),
}));

vi.mock("@mui/x-data-grid", () => ({
  DataGrid: (props: any) => {
    if (props.columns && props.rows) {
      for (const row of props.rows) {
        props.getRowId?.(row);
        for (const col of props.columns) {
          if (col.renderCell) {
            col.renderCell({ row });
          }
        }
      }
    }
    return React.createElement("div", null, `DataGrid rows: ${props.rows?.length ?? 0}`);
  },
}));

describe("AuditPage page component", () => {
  beforeEach(() => {
    stateStore = {};
    stateSetters = {};
    callIdx = 0;
    effectCleanups = [];
    vi.stubGlobal("document", {
      visibilityState: "visible",
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    });
    vi.stubGlobal("window", {
      setInterval: vi.fn((fn: any, delay: number) => {
        fn();
        return 123;
      }),
      clearInterval: vi.fn(),
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  const runRender = () => {
    callIdx = 0;
    return renderToString(React.createElement(AuditPage));
  };

  const getMockTrends = () => [
    { date: "2026-06-01", success: 5, failure: 0 },
    { date: "2026-06-02", success: 8, failure: 2 },
    { date: "Today", success: 0, failure: 0 }, // Hits the m.length >= 3 false branch
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
          user_agent: "", // Hits line 43 (osLabel) and line 22 (clientLabel) fallback
          metadata: {},   // Hits line 158 early return (no status/reason)
          created_at: "2026-06-04T00:30:00Z",
        },
        {
          id: "a_ua_mac",
          action: "users.update",
          user_agent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)", // Hits line 45 (osLabel macOS)
          metadata: { client_info: { browser: "Chrome" } }, // Hits line 15 (browserVersion empty branch)
          created_at: "2026-06-04T00:15:00Z",
        },
        {
          id: "a_ua_win",
          action: "users.update",
          user_agent: "Windows NT 10.0", // Hits line 44 (osLabel Windows)
          created_at: "2026-06-04T00:10:00Z",
        },
        {
          id: "a_ua_android",
          action: "users.update",
          user_agent: "Android 13", // Hits line 46 (osLabel Android)
          created_at: "2026-06-04T00:05:00Z",
        },
        {
          id: "a_ua_chrome_only",
          action: "system.ping",
          user_agent: "Mozilla/5.0 Chrome/120.0", // Hits line 27 true path
          created_at: "2026-06-04T00:04:00Z",
        },
        {
          id: "a_ua_chromium",
          action: "system.ping",
          user_agent: "Mozilla/5.0 Chrome/120.0 Chromium/120.0", // Hits line 27 false path
          created_at: "2026-06-04T00:03:00Z",
        },
        {
          id: "a_ua_iphone",
          action: "system.ping",
          user_agent: "Mozilla/5.0 (iPhone; CPU iPhone OS 14_0)", // Hits line 42 iOS without Mac OS X
          created_at: "2026-06-04T00:02:00Z",
        },
        {
          id: "a_ua_linux",
          action: "system.ping",
          user_agent: "Mozilla/5.0 (X11; Linux x86_64)", // Hits line 43 Linux without Android
          created_at: "2026-06-04T00:01:00Z",
        },
        {
          id: "a_ua_unknown",
          action: "system.ping",
          user_agent: "Some Custom Browser", // Hits line 44 osLabel fallback & line 33 clientLabel fallback
          created_at: "2026-06-04T00:00:00Z",
        },
      ] : []),
    ],
    total: 12,
  });

  it("renders page, trends, and processes all user agents and metadata cases", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u123", roles: ["admin"] } as any);
    const trendsSpy = vi.spyOn(api, "listAuditLogTrends").mockResolvedValue(getMockTrends());
    const listSpy = vi.spyOn(api, "listAuditLog").mockResolvedValue(getMockAuditLogs(true));

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();

    const html = runRender();
    expect(html).toContain("trendsTitle");
    expect(trendsSpy).toHaveBeenCalled();
    expect(listSpy).toHaveBeenCalled();
    expect(trendsSpy.mock.calls.length).toBeGreaterThan(1);
  });

  it("handles loading state and empty trend data gracefully", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u123", roles: ["admin"] } as any);
    vi.spyOn(api, "listAuditLogTrends").mockResolvedValue([]);
    vi.spyOn(api, "listAuditLog").mockResolvedValue({ data: [], total: 0 });

    let html = runRender(); // Loading is true initially
    expect(html).toContain("CircularProgress");

    // Resolve trends as empty to hit line 67
    await Promise.resolve(); await Promise.resolve(); await Promise.resolve();
    html = runRender();
    expect(html).not.toContain("trendsTitle"); // TrendsChart returns null when empty
  });

  it("renders as non-admin showing no trends table", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u123", roles: ["user"] } as any);
    vi.spyOn(api, "listAuditLog").mockResolvedValue({ data: [], total: 0 });

    const html = runRender();
    expect(html).not.toContain("trendsTitle");
  });

  it("renders as non-admin with empty roles array safely", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u123", roles: [] } as any);
    vi.spyOn(api, "listAuditLog").mockResolvedValue({ data: [], total: 0 });

    const html = runRender();
    expect(html).not.toContain("trendsTitle");
  });

  it("handles null user context safely", async () => {
    // Hits line 139 (me?.roles... ?? false)
    vi.mocked(useMeContext).mockReturnValue(null);
    vi.spyOn(api, "listAuditLog").mockResolvedValue({ data: [], total: 0 });
    
    const html = runRender();
    expect(html).not.toContain("trendsTitle");
    expect(html).toContain("accessDenied");
  });

  it("handles trends fetch failure gracefully", async () => {
    // Hits line 51 (.catch(() => {}))
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u123", roles: ["admin"] } as any);
    vi.spyOn(api, "listAuditLogTrends").mockRejectedValue(new Error("fetch failed"));
    vi.spyOn(api, "listAuditLog").mockResolvedValue({ data: [], total: 0 });

    runRender();
    await Promise.resolve(); await Promise.resolve(); await Promise.resolve();
    const html = runRender();
    expect(html).not.toContain("trendsTitle"); // TrendsChart returns null on fetch failure
  });

  it("does not update the trends chart after its refresh effect is cleaned up", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u123", roles: ["admin"] } as any);
    let resolveTrends: (value: ReturnType<typeof getMockTrends>) => void = () => {};
    vi.spyOn(api, "listAuditLogTrends").mockReturnValue(new Promise((resolve) => {
      resolveTrends = resolve;
    }));
    vi.spyOn(api, "listAuditLog").mockResolvedValue({ data: [], total: 0 });

    const html = runRender();
    expect(html).toContain("CircularProgress");
    expect(effectCleanups[0]).toBeDefined();

    effectCleanups[0]();
    resolveTrends(getMockTrends());
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();

    expect(runRender()).toContain("CircularProgress");
  });

  it("renders export button for admin users", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u123", roles: ["admin"] } as any);
    vi.spyOn(api, "listAuditLogTrends").mockResolvedValue([]);
    vi.spyOn(api, "listAuditLog").mockResolvedValue({ data: [], total: 0 });

    const html = runRender();
    expect(html).toContain("export");
  });

  it("does not render export button for non-admin users", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u123", roles: ["user"] } as any);
    vi.spyOn(api, "listAuditLog").mockResolvedValue({ data: [], total: 0 });

    const html = runRender();
    // export button is admin-only
    expect(html).not.toContain(">export<");
  });
});
