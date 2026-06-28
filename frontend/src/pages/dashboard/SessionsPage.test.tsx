import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { render, screen, fireEvent, cleanup, waitFor } from "@testing-library/react";
import SessionsPage from "./SessionsPage";
import { api, ApiError, type Session } from "@/lib/api";
import { toast } from "sonner";

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
            renderedCells.push(
              React.createElement("div", { key: `${row.id}-${col.field}`, "data-testid": `cell-${row.id}-${col.field}` }, col.renderCell({ row }))
            );
          }
        }
      }
    }
    return React.createElement("div", { "data-testid": "mock-datagrid" }, ...renderedCells);
  },
}));

describe("SessionsPage page component", () => {
  beforeEach(() => {
    vi.useRealTimers();
    navigateSpy.mockReset();
    vi.mocked(toast.success).mockClear();
    vi.mocked(toast.warning).mockClear();
    vi.mocked(toast.info).mockClear();
    vi.mocked(toast.error).mockClear();
    capturedOnFilterChange = null;
    capturedOnSortChange = null;

    vi.spyOn(window, "setInterval");
    vi.spyOn(window, "clearInterval");
    vi.spyOn(window, "addEventListener");
    vi.spyOn(window, "removeEventListener");
    vi.spyOn(window, "dispatchEvent");
    vi.spyOn(window, "confirm");
    vi.spyOn(window, "alert");
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

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
    const listSpy = vi.spyOn(api, "listSessions").mockImplementation(async () => mockData);

    render(<SessionsPage />);

    await waitFor(() => {
      expect(listSpy).toHaveBeenCalled();
    });

    // Extract the callback registered to setInterval
    const inspectCallback = vi.mocked(window.setInterval).mock.calls.find(call =>
      call[0].toString().includes("inspectSessions")
    )?.[0] as () => void;
    expect(inspectCallback).toBeDefined();

    // 1. Add session s3
    mockData = [
      ...getSessionsMockData(),
      {
        id: "s3_new",
        is_current: false,
        ip_address: "3.3.3.3",
        user_agent: "curl/7.68.0",
        created_at: "2026-06-04T13:00:00Z",
        last_used_at: "2026-06-04T13:00:00Z",
        device_info: null,
      }
    ];

    inspectCallback();
    await waitFor(() => {
      expect(window.dispatchEvent).toHaveBeenCalledWith(
        expect.objectContaining({ type: "session:new_device" })
      );
    });

    // 2. Remove session s3_new
    mockData = getSessionsMockData();
    inspectCallback();
    await waitFor(() => {
      expect(window.dispatchEvent).toHaveBeenCalledWith(
        expect.objectContaining({ type: "session:ended" })
      );
    });
  });

  it("handles revoking current session", async () => {
    vi.spyOn(api, "listSessions").mockResolvedValue(getSessionsMockData());
    const logoutSpy = vi.spyOn(api, "logout").mockResolvedValue({} as any);
    vi.mocked(window.confirm).mockReturnValue(true);

    render(<SessionsPage />);

    const currentRevokeBtn = await screen.findByRole("button", { name: "signOutThisDevice" });
    fireEvent.click(currentRevokeBtn);

    await waitFor(() => {
      expect(logoutSpy).toHaveBeenCalled();
      expect(navigateSpy).toHaveBeenCalledWith("/auth/login");
    });
  });

  it("handles revoking other session", async () => {
    vi.spyOn(api, "listSessions").mockResolvedValue(getSessionsMockData());
    const revokeSpy = vi.spyOn(api, "revokeSession").mockResolvedValue({} as any);
    vi.mocked(window.confirm).mockReturnValue(true);

    render(<SessionsPage />);

    const otherRevokeBtns = await screen.findAllByRole("button", { name: "signOut" });
    fireEvent.click(otherRevokeBtns[0]);

    expect(revokeSpy).toHaveBeenCalledWith("s2");
  });

  it("cancels revocation if confirm is rejected", async () => {
    vi.spyOn(api, "listSessions").mockResolvedValue(getSessionsMockData());
    const logoutSpy = vi.spyOn(api, "logout");
    vi.mocked(window.confirm).mockReturnValue(false);

    render(<SessionsPage />);

    const currentRevokeBtn = await screen.findByRole("button", { name: "signOutThisDevice" });
    fireEvent.click(currentRevokeBtn);

    expect(logoutSpy).not.toHaveBeenCalled();
  });

  it("cancels revoking all other sessions when confirm is rejected", async () => {
    vi.spyOn(api, "listSessions").mockResolvedValue(getSessionsMockData());
    const revokeOthersSpy = vi.spyOn(api, "revokeOtherSessions").mockResolvedValue({} as any);
    vi.mocked(window.confirm).mockReturnValue(false);

    render(<SessionsPage />);

    const revokeOthersBtn = await screen.findByRole("button", { name: "signOutOthers" });
    fireEvent.click(revokeOthersBtn);

    expect(revokeOthersSpy).not.toHaveBeenCalled();
  });

  it("handles API error during session revocation", async () => {
    vi.spyOn(api, "listSessions").mockResolvedValue(getSessionsMockData());
    vi.spyOn(api, "revokeSession").mockRejectedValue(new Error("API Error"));
    vi.mocked(window.confirm).mockReturnValue(true);

    render(<SessionsPage />);

    const otherRevokeBtns = await screen.findAllByRole("button", { name: "signOut" });
    fireEvent.click(otherRevokeBtns[0]);

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("errors.internal_error");
    });
  });

  it("handles revoking all other sessions", async () => {
    vi.spyOn(api, "listSessions").mockResolvedValue(getSessionsMockData());
    const revokeOthersSpy = vi.spyOn(api, "revokeOtherSessions").mockResolvedValue({} as any);
    vi.mocked(window.confirm).mockReturnValue(true);

    render(<SessionsPage />);

    const revokeOthersBtn = await screen.findByRole("button", { name: "signOutOthers" });
    fireEvent.click(revokeOthersBtn);

    expect(revokeOthersSpy).toHaveBeenCalled();
  });

  it("handles API error during revoking all other sessions", async () => {
    vi.spyOn(api, "listSessions").mockResolvedValue(getSessionsMockData());
    vi.spyOn(api, "revokeOtherSessions").mockRejectedValue(new Error("API Error"));
    vi.mocked(window.confirm).mockReturnValue(true);

    render(<SessionsPage />);

    const revokeOthersBtn = await screen.findByRole("button", { name: "signOutOthers" });
    fireEvent.click(revokeOthersBtn);

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("errors.internal_error");
    });
  });

  it("handles API authentication error by redirecting to login page", async () => {
    vi.spyOn(api, "listSessions").mockRejectedValue(new ApiError("missing_token"));

    render(<SessionsPage />);

    await waitFor(() => {
      expect(navigateSpy).toHaveBeenCalledWith("/auth/login", { replace: true });
    });
  });

  it("ignores non-auth session inspection errors", async () => {
    vi.spyOn(api, "listSessions").mockRejectedValue(new Error("network down"));

    render(<SessionsPage />);

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

    const { unmount } = render(<SessionsPage />);
    expect(listSpy).toHaveBeenCalledTimes(1);

    const inspectCallback = vi.mocked(window.setInterval).mock.calls.find(call =>
      call[0].toString().includes("inspectSessions")
    )?.[0] as () => void;
    expect(inspectCallback).toBeDefined();

    inspectCallback();
    inspectCallback();
    expect(listSpy).toHaveBeenCalledTimes(1);

    resolveSessions(getSessionsMockData());
    await waitFor(() => {
      expect(listSpy).toHaveBeenCalledTimes(1);
    });

    unmount();
    expect(window.removeEventListener).toHaveBeenCalledWith("sessions:changed", expect.any(Function));
    expect(window.clearInterval).toHaveBeenCalled();
  });

  it("refreshes from session change events and ignores unchanged notification checks", async () => {
    const listSpy = vi.spyOn(api, "listSessions").mockResolvedValue(getSessionsMockData());

    render(<SessionsPage />);
    await waitFor(() => {
      expect(listSpy).toHaveBeenCalled();
    });

    const onChanged = vi.mocked(window.addEventListener).mock.calls.find(([event]) => event === "sessions:changed")?.[1] as () => void;
    expect(onChanged).toBeDefined();
    onChanged();

    await waitFor(() => {
      expect(listSpy).toHaveBeenCalled();
    });

    const inspectCallback = vi.mocked(window.setInterval).mock.calls.find(call =>
      call[0].toString().includes("inspectSessions")
    )?.[0] as () => void;
    expect(inspectCallback).toBeDefined();
    inspectCallback();

    expect(toast.warning).not.toHaveBeenCalled();
    expect(toast.info).not.toHaveBeenCalled();
  });

  it("handles filtering in list fetcher", async () => {
    const listSpy = vi.spyOn(api, "listSessions").mockResolvedValue(getSessionsMockData());
    render(<SessionsPage />);

    await waitFor(() => {
      expect(capturedOnFilterChange).toBeDefined();
    });

    capturedOnFilterChange({ items: [{ field: "ip_address", value: "2.2.2.2" }] });
    await waitFor(() => {
      expect(listSpy).toHaveBeenCalled();
    });
  });

  it("handles empty filters and no sort key in the table fetcher", async () => {
    const listSpy = vi.spyOn(api, "listSessions").mockResolvedValue(getSessionsMockData());
    render(<SessionsPage />);

    await waitFor(() => {
      expect(capturedOnFilterChange).toBeDefined();
    });

    capturedOnFilterChange({ items: [] });
    await waitFor(() => {
      expect(capturedOnSortChange).toBeDefined();
    });

    capturedOnSortChange([]);
    capturedOnSortChange([{ field: "created_at", sort: "asc" }]);
    capturedOnFilterChange({ items: [{ field: "missing_field", value: "missing" }] });

    await waitFor(() => {
      expect(listSpy).toHaveBeenCalled();
    });
  });
});
