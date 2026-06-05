import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import TokenRefreshProvider from "./TokenRefreshProvider";
import { api, ApiError, type Session } from "@/lib/api";
import { toast } from "sonner";
import { scheduleRefresh, cancelRefresh } from "@/lib/tokenManager";
import { renderToString } from "react-dom/server";

// Mock react-router-dom
const mockNavigate = vi.fn();
let mockPathname = "/dashboard";
vi.mock("react-router-dom", () => ({
  useNavigate: () => mockNavigate,
  useLocation: () => ({ pathname: mockPathname }),
}));

// Mock react-i18next
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

// Mock sonner toast
vi.mock("sonner", () => ({
  toast: {
    warning: vi.fn(),
    info: vi.fn(),
    description: vi.fn(),
  },
}));

// Mock tokenManager
vi.mock("@/lib/tokenManager", () => ({
  scheduleRefresh: vi.fn(),
  cancelRefresh: vi.fn(),
}));

// Mock React to run useEffect synchronously and capture cleanup
let capturedCleanup: any = null;
vi.mock("react", async (importOriginal) => {
  const original = await importOriginal<typeof import("react")>();
  return {
    ...original,
    useEffect: (fn: any) => {
      const cleanup = fn();
      if (typeof cleanup === "function") {
        capturedCleanup = cleanup;
      }
    },
  };
});

// Stub EventSource
class MockEventSource {
  url: string;
  onmessage: ((ev: any) => void) | null = null;
  close = vi.fn();
  constructor(url: string) {
    this.url = url;
    MockEventSource.lastInstance = this;
  }
  static lastInstance: MockEventSource | null = null;
}

describe("TokenRefreshProvider component", () => {
  const mockDispatchEvent = vi.fn();
  const session = (overrides: Partial<Session>): Session => ({
    id: "s1",
    ip_address: "",
    user_agent: "",
    device_info: null,
    created_at: "2026-06-04T12:00:00Z",
    last_used_at: null,
    is_current: true,
    ...overrides,
  });

  beforeEach(() => {
    mockPathname = "/dashboard";
    vi.stubGlobal("EventSource", MockEventSource);
    vi.stubGlobal("document", { cookie: "at_exp=123" });
    vi.stubGlobal("window", {
      dispatchEvent: mockDispatchEvent,
      setTimeout: (fn: any, delay: number) => {
        fn();
        return 0;
      },
    });
    capturedCleanup = null;
    vi.clearAllMocks();
    vi.useFakeTimers();
    MockEventSource.lastInstance = null;
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("does not register refresh or SSE if pathname is an auth page", () => {
    mockPathname = "/auth/login";
    renderToString(
      React.createElement(
        TokenRefreshProvider,
        null,
        React.createElement("div", null, "Auth Children")
      )
    );

    expect(scheduleRefresh).not.toHaveBeenCalled();
    expect(MockEventSource.lastInstance).toBeNull();
  });

  it("does not register refresh or SSE if the access-token expiry cookie is missing", () => {
    vi.stubGlobal("document", { cookie: "csrf_token=abc" });

    renderToString(
      React.createElement(
        TokenRefreshProvider,
        null,
        React.createElement("div", null, "Dashboard Children")
      )
    );

    expect(scheduleRefresh).not.toHaveBeenCalled();
    expect(MockEventSource.lastInstance).toBeNull();
    expect(capturedCleanup).toBeDefined();
    capturedCleanup();
    expect(cancelRefresh).toHaveBeenCalled();
  });

  it("ignores unknown SSE messages without syncing sessions", async () => {
    const listSpy = vi.spyOn(api, "listSessions").mockResolvedValue([
      session({ id: "s1", is_current: true }),
    ]);

    renderToString(
      React.createElement(
        TokenRefreshProvider,
        null,
        React.createElement("div", null, "Dashboard Children")
      )
    );

    await vi.waitFor(() => {
      expect(listSpy).toHaveBeenCalledTimes(1);
    });

    MockEventSource.lastInstance?.onmessage?.({ data: "connected" });
    MockEventSource.lastInstance?.onmessage?.({ data: "noop" });

    expect(listSpy).toHaveBeenCalledTimes(1);
  });

  it("ignores non-auth session sync failures", async () => {
    const listSpy = vi.spyOn(api, "listSessions").mockRejectedValue(new Error("network down"));

    renderToString(
      React.createElement(
        TokenRefreshProvider,
        null,
        React.createElement("div", null, "Dashboard Children")
      )
    );

    await vi.waitFor(() => {
      expect(listSpy).toHaveBeenCalledTimes(1);
    });

    expect(toast.warning).not.toHaveBeenCalledWith("currentSessionRevoked", expect.any(Object));
    expect(mockNavigate).not.toHaveBeenCalledWith("/auth/login");
  });

  it("ignores sync results after cleanup cancellation", async () => {
    let resolveSessions: (value: Session[]) => void = () => {};
    const listSpy = vi.spyOn(api, "listSessions").mockReturnValue(
      new Promise<Session[]>((resolve) => {
        resolveSessions = resolve;
      }),
    );

    renderToString(
      React.createElement(
        TokenRefreshProvider,
        null,
        React.createElement("div", null, "Dashboard Children")
      )
    );

    expect(listSpy).toHaveBeenCalledTimes(1);
    capturedCleanup?.();
    resolveSessions([session({ id: "s1", is_current: false })]);
    await Promise.resolve();

    expect(toast.warning).not.toHaveBeenCalled();
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it("prevents overlapping session sync requests", async () => {
    let resolveSessions: (value: Session[]) => void = () => {};
    const listSpy = vi.spyOn(api, "listSessions").mockReturnValue(
      new Promise<Session[]>((resolve) => {
        resolveSessions = resolve;
      }),
    );

    renderToString(
      React.createElement(
        TokenRefreshProvider,
        null,
        React.createElement("div", null, "Dashboard Children")
      )
    );

    MockEventSource.lastInstance?.onmessage?.({ data: "change" });
    MockEventSource.lastInstance?.onmessage?.({ data: "change" });
    expect(listSpy).toHaveBeenCalledTimes(1);

    resolveSessions([session({ id: "s1", is_current: true })]);
    await Promise.resolve();
  });

  it("polls as a fallback for session changes", async () => {
    const listSpy = vi.spyOn(api, "listSessions").mockResolvedValue([
      session({ id: "s1", is_current: true }),
    ]);

    renderToString(
      React.createElement(
        TokenRefreshProvider,
        null,
        React.createElement("div", null, "Dashboard Children")
      )
    );

    await vi.waitFor(() => {
      expect(listSpy).toHaveBeenCalledTimes(1);
    });

    vi.runOnlyPendingTimers();

    await vi.waitFor(() => {
      expect(listSpy).toHaveBeenCalledTimes(2);
    });
  });

  it("registers refresh and SSE, handles current session revoked, notifies changes", async () => {
    const listSpy = vi.spyOn(api, "listSessions").mockResolvedValue([
      session({
        id: "s1",
        is_current: false, // current session revoked/not present
        ip_address: "1.2.3.4",
        user_agent: "agent",
        device_info: {
          browser: "Chrome",
          browser_version: "120.0.0",
          os: "Mac",
          os_version: "macOS",
        },
      }),
    ]);

    renderToString(
      React.createElement(
        TokenRefreshProvider,
        null,
        React.createElement("div", null, "Dashboard Children")
      )
    );

    expect(scheduleRefresh).toHaveBeenCalled();
    vi.mocked(scheduleRefresh).mock.calls[0][0]?.();
    expect(mockNavigate).toHaveBeenCalledWith("/auth/login");
    expect(MockEventSource.lastInstance).toBeDefined();
    expect(MockEventSource.lastInstance?.url).toBe("/api/v1/sessions/events");

    // Wait for async syncSessions to run and resolve
    await vi.waitFor(() => {
      expect(listSpy).toHaveBeenCalled();
      expect(toast.warning).toHaveBeenCalledWith("currentSessionRevoked", expect.any(Object));
      expect(cancelRefresh).toHaveBeenCalled();
      expect(mockNavigate).toHaveBeenCalledWith("/auth/login");
    });
  });

  it("notifies when a new session is added, removed, or has fallback labels", async () => {
    const listSpy = vi.spyOn(api, "listSessions")
      .mockResolvedValueOnce([
        session({ id: "s1", is_current: true }),
      ])
      // 2nd call: adding s3 (Safari + Linux -> both defined)
      .mockResolvedValueOnce([
        session({ id: "s1", is_current: true }),
        session({ id: "s3", is_current: false, ip_address: "9.9.9.9", device_info: { browser: "Safari", browser_version: "120", os: "Linux", os_version: "", architecture: "x86_64" } }),
      ])
      // 3rd call: adding s4 (all fields missing -> Unknown device)
      .mockResolvedValueOnce([
        session({ id: "s1", is_current: true }),
        session({ id: "s3", is_current: false, ip_address: "9.9.9.9", device_info: { browser: "Safari", browser_version: "120", os: "Linux", os_version: "", architecture: "x86_64" } }),
        session({ id: "s4", is_current: false, ip_address: "", device_info: { browser: "", browser_version: "", os: "", os_version: "", architecture: "" } }),
      ])
      // 4th call: adding s5 (Safari only)
      .mockResolvedValueOnce([
        session({ id: "s1", is_current: true }),
        session({ id: "s3", is_current: false, ip_address: "9.9.9.9", device_info: { browser: "Safari", browser_version: "120", os: "Linux", os_version: "", architecture: "x86_64" } }),
        session({ id: "s4", is_current: false, ip_address: "", device_info: { browser: "", browser_version: "", os: "", os_version: "", architecture: "" } }),
        session({ id: "s5", is_current: false, ip_address: "", device_info: { browser: "Safari", browser_version: "", os: "", os_version: "", architecture: "" } }),
      ])
      // 5th call: removing s5
      .mockResolvedValueOnce([
        session({ id: "s1", is_current: true }),
        session({ id: "s3", is_current: false, ip_address: "9.9.9.9", device_info: { browser: "Safari", browser_version: "120", os: "Linux", os_version: "", architecture: "x86_64" } }),
        session({ id: "s4", is_current: false, ip_address: "", device_info: { browser: "", browser_version: "", os: "", os_version: "", architecture: "" } }),
      ]);

    renderToString(
      React.createElement(
        TokenRefreshProvider,
        null,
        React.createElement("div", null, "Dashboard Children")
      )
    );

    // Let the first syncSessions finish
    await vi.waitFor(() => {
      expect(listSpy).toHaveBeenCalledTimes(1);
    });

    // Trigger SSE "change" message for s3
    MockEventSource.lastInstance?.onmessage?.({ data: "change" });
    await vi.waitFor(() => {
      expect(listSpy).toHaveBeenCalledTimes(2);
      expect(toast.warning).toHaveBeenCalledWith("newSession", expect.any(Object));
    });

    // Trigger SSE "change" message for s4
    MockEventSource.lastInstance?.onmessage?.({ data: "change" });
    await vi.waitFor(() => {
      expect(listSpy).toHaveBeenCalledTimes(3);
    });

    // Trigger SSE "change" message for s5
    MockEventSource.lastInstance?.onmessage?.({ data: "change" });
    await vi.waitFor(() => {
      expect(listSpy).toHaveBeenCalledTimes(4);
    });

    // Trigger SSE "change" message for removal of s5
    MockEventSource.lastInstance?.onmessage?.({ data: "change" });
    await vi.waitFor(() => {
      expect(listSpy).toHaveBeenCalledTimes(5);
      expect(toast.info).toHaveBeenCalledWith("sessionEnded", expect.any(Object));
    });
  });

  it("builds session descriptions for browser-only, OS-only, and fully populated devices", async () => {
    const listSpy = vi.spyOn(api, "listSessions")
      .mockResolvedValueOnce([
        session({ id: "s1", is_current: true }),
      ])
      .mockResolvedValueOnce([
        session({ id: "s1", is_current: true }),
        session({
          id: "browser-only",
          is_current: false,
          ip_address: "",
          user_agent: "agent-a",
          device_info: { browser: "Firefox" },
        }),
      ])
      .mockResolvedValueOnce([
        session({ id: "s1", is_current: true }),
        session({
          id: "browser-only",
          is_current: false,
          ip_address: "",
          user_agent: "agent-a",
          device_info: { browser: "Firefox" },
        }),
        session({
          id: "os-only",
          is_current: false,
          ip_address: "",
          user_agent: "agent-b",
          device_info: { os: "Windows", os_version: "11", architecture: "x64" },
        }),
      ])
      .mockResolvedValueOnce([
        session({ id: "s1", is_current: true }),
        session({
          id: "browser-only",
          is_current: false,
          ip_address: "",
          user_agent: "agent-a",
          device_info: { browser: "Firefox" },
        }),
        session({
          id: "os-only",
          is_current: false,
          ip_address: "",
          user_agent: "agent-b",
          device_info: { os: "Windows", os_version: "11", architecture: "x64" },
        }),
        session({
          id: "full",
          is_current: false,
          ip_address: "10.0.0.8",
          user_agent: "agent-c",
          device_info: {
            architecture: "arm64",
            browser: "Chrome",
            browser_version: "120.0.0.0",
            mobile: "false",
            os: "macOS",
            os_version: "14",
          },
        }),
      ]);

    renderToString(
      React.createElement(
        TokenRefreshProvider,
        null,
        React.createElement("div", null, "Dashboard Children")
      )
    );

    await vi.waitFor(() => {
      expect(listSpy).toHaveBeenCalledTimes(1);
    });

    MockEventSource.lastInstance?.onmessage?.({ data: "change" });
    await vi.waitFor(() => {
      expect(toast.warning).toHaveBeenLastCalledWith("newSession", expect.objectContaining({
        description: "Firefox",
      }));
    });

    MockEventSource.lastInstance?.onmessage?.({ data: "change" });
    await vi.waitFor(() => {
      expect(toast.warning).toHaveBeenLastCalledWith("newSession", expect.objectContaining({
        description: "Windows 11 x64",
      }));
    });

    MockEventSource.lastInstance?.onmessage?.({ data: "change" });
    await vi.waitFor(() => {
      expect(toast.warning).toHaveBeenLastCalledWith("newSession", expect.objectContaining({
        description: "Chrome 120 — macOS 14 arm64\nIP: 10.0.0.8",
      }));
    });
  });

  it("uses nullish fallbacks for missing session device, ip, and user-agent fields", async () => {
    const oldSession = session({
      id: "old",
      is_current: false,
      ip_address: "8.8.8.8",
      user_agent: undefined as unknown as string,
      device_info: null,
    });
    const nullishSession = session({
      id: "nullish",
      is_current: false,
      ip_address: undefined as unknown as string,
      user_agent: undefined as unknown as string,
      device_info: null,
    });
    const listSpy = vi.spyOn(api, "listSessions")
      .mockResolvedValueOnce([
        session({ id: "s1", is_current: true }),
        oldSession,
      ])
      .mockResolvedValueOnce([
        session({ id: "s1", is_current: true }),
        oldSession,
        nullishSession,
      ])
      .mockResolvedValueOnce([
        session({ id: "s1", is_current: true }),
        nullishSession,
      ]);

    renderToString(
      React.createElement(
        TokenRefreshProvider,
        null,
        React.createElement("div", null, "Dashboard Children")
      )
    );

    await vi.waitFor(() => {
      expect(listSpy).toHaveBeenCalledTimes(1);
    });

    MockEventSource.lastInstance?.onmessage?.({ data: "change" });
    await vi.waitFor(() => {
      expect(toast.warning).toHaveBeenLastCalledWith("newSession", expect.objectContaining({
        description: "Unknown device",
      }));
    });

    MockEventSource.lastInstance?.onmessage?.({ data: "change" });
    await vi.waitFor(() => {
      expect(toast.info).toHaveBeenLastCalledWith("sessionEnded", expect.objectContaining({
        description: "Unknown device\nIP: 8.8.8.8",
      }));
    });
  });

  it("handles SSE revoked messages and ApiError codes", async () => {
    vi.spyOn(api, "listSessions").mockRejectedValue(
      new ApiError("invalid_token", undefined, 401)
    );

    renderToString(
      React.createElement(
        TokenRefreshProvider,
        null,
        React.createElement("div", null, "Dashboard Children")
      )
    );

    // Trigger SSE "revoked" message
    MockEventSource.lastInstance?.onmessage?.({ data: "revoked" });
    expect(toast.warning).toHaveBeenCalledWith("currentSessionRevoked", expect.any(Object));
    expect(cancelRefresh).toHaveBeenCalled();
    expect(mockNavigate).toHaveBeenCalledWith("/auth/login", { replace: true });
  });

  it("runs the cleanup function on unmount correctly", () => {
    renderToString(
      React.createElement(
        TokenRefreshProvider,
        null,
        React.createElement("div", null, "Dashboard Children")
      )
    );

    expect(capturedCleanup).toBeDefined();
    capturedCleanup();
    expect(cancelRefresh).toHaveBeenCalled();
  });
});
