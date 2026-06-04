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
      fn();
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
  ];

  const getMockAuditLogs = () => ({
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
    ],
    total: 12,
  });

  it("renders page, trends, and processes all user agents and metadata cases", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u123", roles: ["admin"] } as any);
    const trendsSpy = vi.spyOn(api, "listAuditLogTrends").mockResolvedValue(getMockTrends());
    const listSpy = vi.spyOn(api, "listAuditLog").mockResolvedValue(getMockAuditLogs());

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();

    const html = runRender();
    expect(html).toContain("trendsTitle");
    expect(trendsSpy).toHaveBeenCalled();
    expect(listSpy).toHaveBeenCalled();
  });

  it("handles loading state and empty trend data gracefully", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u123", roles: ["admin"] } as any);
    vi.spyOn(api, "listAuditLogTrends").mockResolvedValue([]);
    vi.spyOn(api, "listAuditLog").mockResolvedValue({ data: [], total: 0 });

    const html = runRender(); // Loading is true initially
    expect(html).toContain("CircularProgress");
  });

  it("renders as non-admin showing no trends table", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u123", roles: ["user"] } as any);
    vi.spyOn(api, "listAuditLog").mockResolvedValue({ data: [], total: 0 });

    const html = runRender();
    expect(html).not.toContain("trendsTitle");
  });
});
