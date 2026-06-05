import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cancelRefresh, scheduleRefresh } from "./tokenManager";

function jsonResponse(status: number, body: Record<string, unknown> | unknown[]): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("tokenManager utility", () => {
  let cookies: Record<string, string> = {};

  beforeEach(() => {
    cookies = {};
    vi.stubGlobal("document", {
      get cookie() {
        return Object.entries(cookies)
          .map(([k, v]) => `${k}=${v}`)
          .join("; ");
      },
      set cookie(val: string) {
        const parts = val.split(";")[0].split("=");
        if (parts.length === 2) {
          cookies[parts[0].trim()] = parts[1].trim();
        }
      },
    });
  });

  afterEach(() => {
    cancelRefresh();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("calls onExpired immediately if at_exp cookie is missing", () => {
    const onExpired = vi.fn();
    scheduleRefresh(onExpired);
    expect(onExpired).toHaveBeenCalledTimes(1);
  });

  it("does not call onExpired if at_exp is NaN", () => {
    cookies["at_exp"] = "invalid-number";
    const onExpired = vi.fn();
    scheduleRefresh(onExpired);
    expect(onExpired).not.toHaveBeenCalled();
  });

  it("triggers refresh immediately if at_exp is expired (ttl <= 0)", async () => {
    const pastTimeSec = Math.floor(Date.now() / 1000) - 10;
    cookies["at_exp"] = pastTimeSec.toString();
    cookies["csrf_token"] = "csrf123";

    const fetchMock = vi.fn().mockImplementation(() => {
      // Clear at_exp to prevent infinite recursion on callback
      delete cookies["at_exp"];
      return Promise.resolve(jsonResponse(200, {}));
    });
    vi.stubGlobal("fetch", fetchMock);

    const onExpired = vi.fn();
    scheduleRefresh(onExpired);

    await vi.waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(1);
    });

    const [, options] = fetchMock.mock.calls[0];
    expect(options.headers["X-CSRF-Token"]).toBe("csrf123");
  });

  it("schedules timeout and triggers refresh at threshold of remaining TTL", async () => {
    vi.useFakeTimers();
    const futureTimeSec = Math.floor(Date.now() / 1000) + 10; // 10s in future
    cookies["at_exp"] = futureTimeSec.toString();
    cookies["csrf_token"] = "csrf123";
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, {}));
    vi.stubGlobal("fetch", fetchMock);

    const spySetTimeout = vi.spyOn(global, "setTimeout");

    scheduleRefresh();

    expect(spySetTimeout).toHaveBeenCalled();
    const delay = spySetTimeout.mock.calls[0][1];
    expect(delay).toBeGreaterThan(7000);
    expect(delay).toBeLessThan(9000);

    await vi.advanceTimersByTimeAsync(Number(delay));
    expect(fetchMock).toHaveBeenCalledWith("/api/v1/auth/refresh", expect.any(Object));
    vi.useRealTimers();
  });

  it("calls onExpired and cancels further refreshes if refresh request fails", async () => {
    const pastTimeSec = Math.floor(Date.now() / 1000) - 10;
    cookies["at_exp"] = pastTimeSec.toString();

    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(401, { error: "unauthorized" }));
    vi.stubGlobal("fetch", fetchMock);

    const onExpired = vi.fn();
    scheduleRefresh(onExpired);

    await vi.waitFor(() => {
      expect(onExpired).toHaveBeenCalledTimes(1);
    });
  });

  it("returns null from getCookie when document is undefined", () => {
    vi.stubGlobal("document", undefined);
    const onExpired = vi.fn();
    scheduleRefresh(onExpired);
    expect(onExpired).toHaveBeenCalledTimes(1);
  });
});
