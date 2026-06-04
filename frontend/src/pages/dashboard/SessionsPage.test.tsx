import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import SessionsPage from "./SessionsPage";
import { api, ApiError } from "@/lib/api";
import { renderToString } from "react-dom/server";
import { toast } from "sonner";

// State and Ref Mocking System
let stateStore: any = {};
let stateSetters: any = {};
let callIdx = 0;
let refStore: any = {};
let refIdx = 0;

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
      if (callIdx >= 40) {
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
      fn();
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

vi.mock("@mui/x-data-grid", () => ({
  DataGrid: (props: any) => {
    if (props.onFilterModelChange) {
      capturedOnFilterChange = props.onFilterModelChange;
    }
    const renderedCells: any[] = [];
    if (props.columns && props.rows) {
      for (const row of props.rows) {
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
    capturedButtonClicks.length = 0;
    capturedOnFilterChange = null;
    confirmMock = vi.fn().mockReturnValue(true);
    alertMock = vi.fn();
    navigateSpy.mockReset();
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

  const getSessionsMockData = () => [
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
  ];

  it("renders page, handles device list and detects session changes", async () => {
    let mockData = getSessionsMockData();
    const listSpy = vi.spyOn(api, "listSessions").mockImplementation(async () => {
      return mockData;
    });

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

    expect(toast.warning).toHaveBeenCalled();

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

    expect(toast.info).toHaveBeenCalled();
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

  it("handles API error during session revocation", async () => {
    vi.spyOn(api, "listSessions").mockResolvedValue(getSessionsMockData());
    vi.spyOn(api, "revokeSession").mockRejectedValue(new Error("API Error"));

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    await capturedButtonClicks[2]();
    expect(alertMock).toHaveBeenCalledWith("errors.internal_error");
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
    expect(alertMock).toHaveBeenCalledWith("errors.internal_error");
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
});
