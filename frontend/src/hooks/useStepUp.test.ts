import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { api, ApiError } from "@/lib/api";

// Flush all pending microtasks so async branches inside the hook settle.
const flush = () => new Promise<void>((r) => setTimeout(r, 0));

describe("useStepUp", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  // ── runWithStepUp ──────────────────────────────────────────────────────────

  it("returns action result directly when action succeeds", async () => {
    const { useStepUp } = await import("./useStepUp");
    const { result } = renderHook(() => useStepUp());
    const value = await act(() => result.current.runWithStepUp(() => Promise.resolve("ok")));
    expect(value).toBe("ok");
    expect(result.current.stepUpOpen).toBe(false);
  });

  it("re-throws non-ApiError without opening dialog", async () => {
    const { useStepUp } = await import("./useStepUp");
    const { result } = renderHook(() => useStepUp());
    const err = new Error("network");
    await expect(act(() => result.current.runWithStepUp(() => Promise.reject(err)))).rejects.toBe(err);
    expect(result.current.stepUpOpen).toBe(false);
  });

  it("re-throws ApiError that is not mfa_required without opening dialog", async () => {
    const { useStepUp } = await import("./useStepUp");
    const { result } = renderHook(() => useStepUp());
    const err = new ApiError("forbidden", 403);
    await expect(act(() => result.current.runWithStepUp(() => Promise.reject(err)))).rejects.toBe(err);
    expect(result.current.stepUpOpen).toBe(false);
  });

  it("opens dialog on mfa_required", async () => {
    const { useStepUp } = await import("./useStepUp");
    const { result } = renderHook(() => useStepUp());
    const action = vi.fn().mockRejectedValue(new ApiError("mfa_required", 403));

    // Don't await — the promise stays pending until dialog is handled
    act(() => { result.current.runWithStepUp(action).catch(() => {}); });
    await act(async () => { await flush(); });

    expect(result.current.stepUpOpen).toBe(true);
  });

  // ── handleStepUpSubmit ─────────────────────────────────────────────────────

  it("does nothing when there is no pending action", async () => {
    const spy = vi.spyOn(api, "mfaStepUp").mockResolvedValue(undefined as any);
    const { useStepUp } = await import("./useStepUp");
    const { result } = renderHook(() => useStepUp());
    await act(async () => { await result.current.handleStepUpSubmit("123456"); });
    expect(spy).not.toHaveBeenCalled();
  });

  it("calls mfaStepUp with code+reason, resolves action result, closes dialog", async () => {
    vi.spyOn(api, "mfaStepUp").mockResolvedValue(undefined as any);
    const { useStepUp } = await import("./useStepUp");
    const { result } = renderHook(() => useStepUp());
    const action = vi.fn()
      .mockRejectedValueOnce(new ApiError("mfa_required", 403))
      .mockResolvedValueOnce("final");

    let promise: Promise<unknown>;
    act(() => { promise = result.current.runWithStepUp(action, "admin_action"); });
    await act(async () => { await flush(); });

    expect(result.current.stepUpOpen).toBe(true);

    await act(async () => { await result.current.handleStepUpSubmit("123456"); });

    expect(api.mfaStepUp).toHaveBeenCalledWith("123456", "admin_action");
    expect(await promise!).toBe("final");
    expect(result.current.stepUpOpen).toBe(false);
    expect(result.current.stepUpSubmitting).toBe(false);
  });

  it("sets error and keeps dialog open on invalid_code", async () => {
    vi.spyOn(api, "mfaStepUp").mockRejectedValue(new ApiError("invalid_code", 400));
    const { useStepUp } = await import("./useStepUp");
    const { result } = renderHook(() => useStepUp());
    const action = vi.fn().mockRejectedValue(new ApiError("mfa_required", 403));

    act(() => { result.current.runWithStepUp(action).catch(() => {}); });
    await act(async () => { await flush(); });

    await act(async () => { await result.current.handleStepUpSubmit("wrong"); });

    expect(result.current.stepUpError).toBe("invalid_code");
    expect(result.current.stepUpOpen).toBe(true);
    expect(result.current.stepUpSubmitting).toBe(false);
  });

  it("sets error and keeps dialog open on too_many_attempts", async () => {
    vi.spyOn(api, "mfaStepUp").mockRejectedValue(new ApiError("too_many_attempts", 429));
    const { useStepUp } = await import("./useStepUp");
    const { result } = renderHook(() => useStepUp());
    const action = vi.fn().mockRejectedValue(new ApiError("mfa_required", 403));

    act(() => { result.current.runWithStepUp(action).catch(() => {}); });
    await act(async () => { await flush(); });

    await act(async () => { await result.current.handleStepUpSubmit("123456"); });

    expect(result.current.stepUpError).toBe("too_many_attempts");
    expect(result.current.stepUpOpen).toBe(true);
  });

  it("rejects pending and closes dialog on unexpected error from action", async () => {
    vi.spyOn(api, "mfaStepUp").mockResolvedValue(undefined as any);
    const downstreamErr = new Error("server down");
    const { useStepUp } = await import("./useStepUp");
    const { result } = renderHook(() => useStepUp());
    const action = vi.fn()
      .mockRejectedValueOnce(new ApiError("mfa_required", 403))
      .mockRejectedValueOnce(downstreamErr);

    let caughtErr: unknown;
    let promise: Promise<unknown>;
    act(() => {
      promise = result.current.runWithStepUp(action);
      promise.catch((e) => { caughtErr = e; });
    });
    await act(async () => { await flush(); });

    await act(async () => { await result.current.handleStepUpSubmit("123456"); });

    // Wait for the rejection to propagate
    await act(async () => { await flush(); });

    expect(caughtErr).toBe(downstreamErr);
    expect(result.current.stepUpOpen).toBe(false);
    expect(result.current.stepUpError).toBe("");
  });

  // ── handleStepUpClose ──────────────────────────────────────────────────────

  it("rejects pending with mfa_required ApiError and closes dialog", async () => {
    const { useStepUp } = await import("./useStepUp");
    const { result } = renderHook(() => useStepUp());
    const action = vi.fn().mockRejectedValue(new ApiError("mfa_required", 403));

    let caughtErr: unknown;
    act(() => {
      const promise = result.current.runWithStepUp(action);
      promise.catch((e) => { caughtErr = e; });
    });
    await act(async () => { await flush(); });

    act(() => { result.current.handleStepUpClose(); });

    await act(async () => { await flush(); });

    expect(caughtErr).toMatchObject({ message: "mfa_required" });
    expect(result.current.stepUpOpen).toBe(false);
    expect(result.current.stepUpError).toBe("");
  });

  it("closes dialog without error even when no pending action", async () => {
    const { useStepUp } = await import("./useStepUp");
    const { result } = renderHook(() => useStepUp());

    act(() => { result.current.handleStepUpClose(); });

    expect(result.current.stepUpOpen).toBe(false);
  });
});
