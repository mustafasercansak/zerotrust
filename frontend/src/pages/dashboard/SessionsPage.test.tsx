import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import SessionsPage from "./SessionsPage";
import { api, ApiError, type Session } from "@/lib/api";
import { renderToString } from "react-dom/server";
import { toast } from "sonner";

// State and Ref Mocking System
let stateStore: any = {};
let stateSetters: any = {};
let callIdx = 0;
let refStore: any = {};
let refIdx = 0;
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
      if (callIdx >= 120) {
        callIdx = 0;
      }
      return [stateStore[idx], stateSetters[idx]];
    },
    useRef: (init: any) => {
      const idx = refIdx;
      refIdx++;
      if (!(idx in refStore)) {
        refStore[idx] = { current: init };
      }
      return refStore[idx];
    },
    useEffect: (fn: any) => {
      const cleanup = fn();
      if (typeof cleanup === "function") effectCleanups.push(cleanup);
    },
  };
});

const navigateSpy = vi.fn();
vi.mock("react-router-dom", () => ({
  useNavigate: () => navigateSpy,
}));

vi.mock("sonner", () => ({
  toast: {
    success: vi.fn(),
    warning: vi.fn(),
    info: vi.fn(),
    error: vi.fn(),
  },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: "en" },
  }),
}));

const capturedButtonClicks: any[] = [];
let capturedOnFilterChange: any = null;
let capturedOnSortChange: any = null;

vi.mock("@mui/x-data-grid", () => ({
  DataGrid: (props: any) => {
    if (props.onFilterModelChange) {
      capturedOnFilterChange = props.onFilterModelChange;
    }
    if (props.onSortModelChange) {
      capturedOnSortChange = props.onSortModelChange;
    }
    const renderedCells: any[] = [];
    if (props.columns && props.rows) {
      for (const row of props.rows) {
        props.getRowId?.(row);
        for (const col of props.columns) {
          if (col.renderCell) {
            renderedCells.push(col.renderCell({ row }));
          }
        }
      }
    }
    return React.createElement("div", null, ...renderedCells);
  },
}));

vi.mock("@mui/material/Button", () => ({
  default: (props: any) => {
    if (props.onClick) {
      capturedButtonClicks.push(props.onClick);
    }
    return React.createElement("button", { onClick: props.onClick, disabled: props.disabled }, props.children);
  }
}));

describe("SessionsPage page component", () => {
  let confirmMock = vi.fn().mockReturnValue(true);
  let alertMock = vi.fn();
  const intervalCallbacks: any[] = [];

  beforeEach(() => {
    stateStore = {};
    stateSetters = {};
    refStore = {};
    callIdx = 0;
    refIdx = 0;
    effectCleanups = [];
    capturedButtonClicks.length = 0;
    capturedOnFilterChange = null;
    capturedOnSortChange = null;
    confirmMock = vi.fn().mockReturnValue(true);
    alertMock = vi.fn();
    navigateSpy.mockReset();
    vi.mocked(toast.success).mockClear();
    vi.mocked(toast.warning).mockClear();
    vi.mocked(toast.info).mockClear();
    vi.mocked(toast.error).mockClear();
    intervalCallbacks.length = 0;

    vi.stubGlobal("document", {
      visibilityState: "visible",
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    });
    vi.stubGlobal("window", {
      setInterval: vi.fn((fn: any, delay: number) => {
        intervalCallbacks.push(fn);
        return 123;
      }),
      clearInterval: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    });
    vi.stubGlobal("confirm", confirmMock);
    vi.stubGlobal("alert", alertMock);
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  const runRender = () => {
    callIdx = 0;
    refIdx = 0;
    capturedButtonClicks.length = 0;
    return renderToString(React.createElement(SessionsPage));
  };

  const getSessionsMockData = (): Session[] => [
    {
      id: "s1",
      is_current: true,
      ip_address: "1.1.1.1",
      user_agent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0",
      created_at: "2026-06-04T12:00:00Z",
      last_used_at: "2026-06-04T12:00:00Z",
      device_info: { browser: "Chrome", browser_version: "120", os: "Windows", os_version: "10" },
    },
    {
      id: "s2",
      is_current: false,
      ip_address: "2.2.2.2",
      user_agent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Safari/15.0",
      created_at: "2026-06-04T10:00:00Z",
      last_used_at: "2026-06-04T11:00:00Z",
      device_info: { browser: "Safari", browser_version: "15.0", os: "macOS", os_version: "10.15.7" },
    },
    {
      id: "s3",
      is_current: false,
      ip_address: "",
      user_agent: "",
      created_at: "2026-06-04T09:00:00Z",
      last_used_at: null,
      device_info: null,
    },
    {
      id: "s4",
      is_current: false,
      ip_address: "4.4.4.4",
      user_agent: "Mozilla/5.0 (Linux x86_64) OPR/",
      created_at: "2026-06-04T08:00:00Z",
      last_used_at: "2026-06-04T08:30:00Z",
      device_info: null,
    },
    {
      id: "s5",
      is_current: false,
      ip_address: "5.5.5.5",
      user_agent: "Mozilla/5.0 (Windows NT; Win64; x64) Edg/",
      created_at: "2026-06-04T07:00:00Z",
      last_used_at: "2026-06-04T07:30:00Z",
      device_info: null,
    },
    {
      id: "s6",
      is_current: false,
      ip_address: "6.6.6.6",
      user_agent: "Mozilla/5.0 (X11; Linux x86_64) Firefox/",
      created_at: "2026-06-04T06:00:00Z",
      last_used_at: "2026-06-04T06:30:00Z",
      device_info: null,
    },
    {
      id: "s7",
      is_current: false,
      ip_address: "7.7.7.7",
      user_agent: "Mozilla/5.0 (iPhone; CPU iPhone OS 16_5 like Mac OS X) Version/16.5 Mobile/15E148 Safari/604.1",
      created_at: "2026-06-04T05:00:00Z",
      last_used_at: "2026-06-04T05:30:00Z",
      device_info: null,
    },
    {
      id: "s8",
      is_current: false,
      ip_address: "8.8.8.8",
      user_agent: "Mozilla/5.0 (Linux; Android 13; Pixel) Chrome/114.0.0.0 Mobile Safari/537.36",
      created_at: "2026-06-04T04:00:00Z",
      last_used_at: "2026-06-04T04:30:00Z",
      device_info: null,
    },
    {
      id: "s9",
      is_current: false,
      ip_address: "9.9.9.9",
      user_agent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) Version/17.0 Safari/605.1.15",
      created_at: "2026-06-04T03:00:00Z",
      last_used_at: "2026-06-04T03:30:00Z",
      device_info: null,
    },
    {
      id: "s10",
      is_current: false,
      ip_address: "10.10.10.10",
      user_agent: "Mozilla/5.0 (Windows NT 6.1; Win64; x64) Chrome/120.0.0.0",
      created_at: "2026-06-04T02:00:00Z",
      last_used_at: "2026-06-04T02:30:00Z",
      device_info: null,
    },
    {
      id: "s11",
      is_current: false,
      ip_address: "11.11.11.11",
      user_agent: "Mozilla/5.0 (Windows x64) Chrome",
      created_at: "2026-06-04T01:00:00Z",
      last_used_at: "2026-06-04T01:30:00Z",
      device_info: null,
    },
    {
      id: "s12",
      is_current: false,
      ip_address: "12.12.12.12",
      user_agent: "Mozilla/5.0 (iPhone) Safari/604.1",
      created_at: "2026-06-04T00:00:00Z",
      last_used_at: "2026-06-04T00:30:00Z",
      device_info: null,
    },
    {
      id: "s13",
      is_current: false,
      ip_address: "13.13.13.13",
      user_agent: "Mozilla/5.0 (Linux; Android) Chrome Mobile Safari/537.36",
      created_at: "2026-06-03T23:00:00Z",
      last_used_at: "2026-06-03T23:30:00Z",
      device_info: null,
    },
    {
      id: "s14",
      is_current: false,
      ip_address: "14.14.14.14",
      user_agent: "Mozilla/5.0 (Macintosh; Intel Mac OS X) Version Safari/605.1.15",
      created_at: "2026-06-03T22:00:00Z",
      last_used_at: "2026-06-03T22:30:00Z",
      device_info: null,
    },
    {
      id: "s15",
      is_current: false,
      ip_address: undefined as unknown as string,
      user_agent: undefined as unknown as string,
      created_at: "2026-06-03T21:00:00Z",
      last_used_at: "2026-06-03T21:30:00Z",
      device_info: { browser: "Brave", os: "Windows", architecture: "x64" },
    },
    {
      id: "s16",
      is_current: false,
      ip_address: "16.16.16.16",
      user_agent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chromium/120.0.0.0 Safari/537.36",
      created_at: "2026-06-03T20:00:00Z",
      last_used_at: "2026-06-03T20:30:00Z",
      device_info: null,
    },
    {
      id: "s17",
      is_current: false,
      ip_address: "17.17.17.17",
      user_agent: "Mozilla/5.0 Brave",
      created_at: "2026-06-03T19:00:00Z",
      last_used_at: "2026-06-03T19:30:00Z",
      device_info: { browser: "Brave", os: "Windows", architecture: "x64" },
    },
  ];

  it("renders page, handles device list and detects session changes", async () => {
    let mockData = getSessionsMockData();
    const listSpy = vi.spyOn(api, "listSessions").mockImplementation(async () => {
      return mockData;
    });
    if (!window.dispatchEvent) {
      Object.defineProperty(window, "dispatchEvent", { value: vi.fn(), writable: true });
    }
    const dispatchEventSpy = vi.spyOn(window, "dispatchEvent");

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    expect(listSpy).toHaveBeenCalled();

    // Now test interval triggers and notification:
    // 1. Add session s3
    mockData = [
      ...getSessionsMockData(),
      {
        id: "s3",
        is_current: false,
        ip_address: "3.3.3.3",
        user_agent: "curl/7.68.0",
        created_at: "2026-06-04T13:00:00Z",
        last_used_at: "2026-06-04T13:00:00Z",
        device_info: null,
      }
    ];

    const inspectCallback = intervalCallbacks.find(fn => fn.toString().includes("inspectSessions"));
    expect(inspectCallback).toBeDefined();

    inspectCallback(); // trigger inspectSessions(true)
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();

    expect(dispatchEventSpy).toHaveBeenCalledWith(
      expect.objectContaining({ type: "session:new_device" }),
    );

    // 2. Remove session s3
    mockData = getSessionsMockData();
    inspectCallback(); // trigger inspectSessions(true)
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();

    expect(dispatchEventSpy).toHaveBeenCalledWith(
      expect.objectContaining({ type: "session:ended" }),
    );
  });

  it("handles revoking current session", async () => {
    vi.spyOn(api, "listSessions").mockResolvedValue(getSessionsMockData());
    const logoutSpy = vi.spyOn(api, "logout").mockResolvedValue({} as any);

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    expect(capturedButtonClicks[1]).toBeDefined();
    await capturedButtonClicks[1]();

    expect(logoutSpy).toHaveBeenCalled();
    expect(navigateSpy).toHaveBeenCalledWith("/auth/login");
  });

  it("handles revoking other session", async () => {
    vi.spyOn(api, "listSessions").mockResolvedValue(getSessionsMockData());
    const revokeSpy = vi.spyOn(api, "revokeSession").mockResolvedValue({} as any);

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    expect(capturedButtonClicks[2]).toBeDefined();
    await capturedButtonClicks[2]();

    expect(revokeSpy).toHaveBeenCalledWith("s2");
  });

  it("cancels revocation if confirm is rejected", async () => {
    confirmMock.mockReturnValue(false);
    vi.spyOn(api, "listSessions").mockResolvedValue(getSessionsMockData());
    const logoutSpy = vi.spyOn(api, "logout");

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    await capturedButtonClicks[1]();
    expect(logoutSpy).not.toHaveBeenCalled();
  });

  it("cancels revoking all other sessions when confirm is rejected", async () => {
    confirmMock.mockReturnValue(false);
    vi.spyOn(api, "listSessions").mockResolvedValue(getSessionsMockData());
    const revokeOthersSpy = vi.spyOn(api, "revokeOtherSessions").mockResolvedValue({} as any);

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    await capturedButtonClicks[0]();

    expect(revokeOthersSpy).not.toHaveBeenCalled();
  });

  it("handles API error during session revocation", async () => {
    vi.spyOn(api, "listSessions").mockResolvedValue(getSessionsMockData());
    vi.spyOn(api, "revokeSession").mockRejectedValue(new Error("API Error"));

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    await capturedButtonClicks[2]();
    expect(toast.error).toHaveBeenCalledWith("errors.internal_error");
  });

  it("handles revoking all other sessions", async () => {
    vi.spyOn(api, "listSessions").mockResolvedValue(getSessionsMockData());
    const revokeOthersSpy = vi.spyOn(api, "revokeOtherSessions").mockResolvedValue({} as any);

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    expect(capturedButtonClicks[0]).toBeDefined();
    await capturedButtonClicks[0]();

    expect(revokeOthersSpy).toHaveBeenCalled();
  });

  it("handles API error during revoking all other sessions", async () => {
    vi.spyOn(api, "listSessions").mockResolvedValue(getSessionsMockData());
    vi.spyOn(api, "revokeOtherSessions").mockRejectedValue(new Error("API Error"));

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    await capturedButtonClicks[0]();
    expect(toast.error).toHaveBeenCalledWith("errors.internal_error");
  });

  it("handles API authentication error by redirecting to login page", async () => {
    vi.spyOn(api, "listSessions").mockRejectedValue(new ApiError("missing_token"));

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    expect(navigateSpy).toHaveBeenCalledWith("/auth/login", { replace: true });
  });

  it("ignores non-auth session inspection errors", async () => {
    vi.spyOn(api, "listSessions").mockRejectedValue(new Error("network down"));

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();

    expect(navigateSpy).not.toHaveBeenCalled();
  });

  it("prevents overlapping session inspection and cleans up listeners", async () => {
    let resolveSessions: (value: Session[]) => void = () => {};
    const listSpy = vi.spyOn(api, "listSessions").mockReturnValue(
      new Promise<Session[]>((resolve) => {
        resolveSessions = resolve;
      }),
    );

    runRender();
    expect(listSpy).toHaveBeenCalledTimes(1);

    const inspectCallback = intervalCallbacks.find(fn => fn.toString().includes("inspectSessions"));
    expect(inspectCallback).toBeDefined();
    inspectCallback();
    inspectCallback();
    expect(listSpy).toHaveBeenCalledTimes(1);

    resolveSessions(getSessionsMockData());
    await Promise.resolve();

    expect(effectCleanups[0]).toBeDefined();
    effectCleanups[0]();
    expect(window.removeEventListener).toHaveBeenCalledWith("sessions:changed", expect.any(Function));
    expect(window.clearInterval).toHaveBeenCalledWith(123);
  });

  it("refreshes from session change events and ignores unchanged notification checks", async () => {
    const listSpy = vi.spyOn(api, "listSessions").mockResolvedValue(getSessionsMockData());

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();

    const onChanged = vi.mocked(window.addEventListener).mock.calls.find(([event]) => event === "sessions:changed")?.[1] as () => void;
    expect(onChanged).toBeDefined();
    onChanged();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();

    const inspectCallback = intervalCallbacks.find(fn => fn.toString().includes("inspectSessions"));
    expect(inspectCallback).toBeDefined();
    inspectCallback();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();

    expect(listSpy).toHaveBeenCalled();
    expect(toast.warning).not.toHaveBeenCalled();
    expect(toast.info).not.toHaveBeenCalled();
  });

  it("handles filtering in list fetcher", async () => {
    vi.spyOn(api, "listSessions").mockResolvedValue(getSessionsMockData());
    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    expect(capturedOnFilterChange).toBeDefined();
    capturedOnFilterChange({ items: [{ field: "ip_address", value: "2.2.2.2" }] });
    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();
  });

  it("handles empty filters and no sort key in the table fetcher", async () => {
    vi.spyOn(api, "listSessions").mockResolvedValue(getSessionsMockData());
    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    expect(capturedOnFilterChange).toBeDefined();
    capturedOnFilterChange({ items: [] });
    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    expect(capturedOnSortChange).toBeDefined();
    capturedOnSortChange([]);
    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    capturedOnSortChange([{ field: "created_at", sort: "asc" }]);
    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    capturedOnFilterChange({ items: [{ field: "missing_field", value: "missing" }] });
    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();
  });
});
