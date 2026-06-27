import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { api } from "@/lib/api";

vi.mock("@/lib/api", () => ({
  api: { getSessionPolicy: vi.fn() },
}));

beforeEach(() => {
  vi.useFakeTimers();
  vi.mocked(api.getSessionPolicy).mockResolvedValue({ idle_timeout_seconds: 120 });
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("useIdleTimeout hook", () => {
  it("returns correct initial shape", async () => {
    const { useIdleTimeout } = await import("./useIdleTimeout");
    const onExpire = vi.fn();
    const { result } = renderHook(() => useIdleTimeout(onExpire));

    expect(result.current.warningVisible).toBe(false);
    expect(result.current.secondsRemaining).toBe(60);
    expect(typeof result.current.extendSession).toBe("function");
    expect(typeof result.current.dismissWarning).toBe("function");
  });

  it("calls getSessionPolicy on mount", async () => {
    const { useIdleTimeout } = await import("./useIdleTimeout");
    const onExpire = vi.fn();
    renderHook(() => useIdleTimeout(onExpire));

    await act(async () => {
      await vi.runAllTimersAsync();
    });

    expect(api.getSessionPolicy).toHaveBeenCalled();
  });

  it("swallows getSessionPolicy errors silently", async () => {
    vi.mocked(api.getSessionPolicy).mockRejectedValueOnce(new Error("network"));
    const { useIdleTimeout } = await import("./useIdleTimeout");
    const onExpire = vi.fn();
    renderHook(() => useIdleTimeout(onExpire));

    await act(async () => {
      await vi.runAllTimersAsync();
    });
    // Expect no unhandled rejection — swallowed silently.
  });

  it("registers activity event listeners", async () => {
    const addSpy = vi.spyOn(window, "addEventListener");
    const { useIdleTimeout } = await import("./useIdleTimeout");
    const onExpire = vi.fn();
    renderHook(() => useIdleTimeout(onExpire));

    expect(addSpy).toHaveBeenCalledWith("mousemove", expect.any(Function), { passive: true });
    expect(addSpy).toHaveBeenCalledWith("keydown", expect.any(Function), { passive: true });
    expect(addSpy).toHaveBeenCalledWith("click", expect.any(Function), { passive: true });
  });

  it("removes activity event listeners on unmount", async () => {
    const removeSpy = vi.spyOn(window, "removeEventListener");
    const { useIdleTimeout } = await import("./useIdleTimeout");
    const onExpire = vi.fn();
    const { unmount } = renderHook(() => useIdleTimeout(onExpire));

    unmount();

    expect(removeSpy).toHaveBeenCalledWith("mousemove", expect.any(Function));
  });
});
