import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api, ApiError } from "@/lib/api";

// Keep ApiError as the real class so instanceof checks in the hook work correctly.
// Only spy on individual api methods per test.

const fakeRef = { current: null as any };
let stateStore: any = {};
let stateSetters: any = {};
let callIdx = 0;

vi.mock("react", async (importOriginal) => {
  const original = await importOriginal<typeof import("react")>();
  return {
    ...original,
    useState: (init: any) => {
      const idx = callIdx++;
      if (!(idx in stateStore)) stateStore[idx] = init;
      stateSetters[idx] = (v: any) => {
        stateStore[idx] = typeof v === "function" ? v(stateStore[idx]) : v;
      };
      return [stateStore[idx], stateSetters[idx]];
    },
    useRef: () => fakeRef,
  };
});

import { useStepUp } from "./useStepUp";

// State slot indices inside useStepUp:
//   0 = open, 1 = error, 2 = submitting

const useTestStepUp = () => {
  callIdx = 0;
  return useStepUp();
};

// Flush all pending microtasks so async branches inside the hook settle.
const flush = () => new Promise<void>((r) => setTimeout(r, 0));

describe("useStepUp", () => {
  beforeEach(() => {
    stateStore = {};
    stateSetters = {};
    callIdx = 0;
    fakeRef.current = null;
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  // ── runWithStepUp ──────────────────────────────────────────────────────────

  it("returns action result directly when action succeeds", async () => {
    const { runWithStepUp } = useTestStepUp();
    const result = await runWithStepUp(() => Promise.resolve("ok"));
    expect(result).toBe("ok");
    expect(stateStore[0]).toBe(false); // dialog never opened
  });

  it("re-throws non-ApiError without opening dialog", async () => {
    const { runWithStepUp } = useTestStepUp();
    const err = new Error("network");
    await expect(runWithStepUp(() => Promise.reject(err))).rejects.toBe(err);
    expect(stateStore[0]).toBe(false);
  });

  it("re-throws ApiError that is not mfa_required without opening dialog", async () => {
    const { runWithStepUp } = useTestStepUp();
    const err = new ApiError("forbidden", 403);
    await expect(runWithStepUp(() => Promise.reject(err))).rejects.toBe(err);
    expect(stateStore[0]).toBe(false);
  });

  it("opens dialog and stores pending action on mfa_required", async () => {
    const { runWithStepUp } = useTestStepUp();
    const action = vi.fn().mockRejectedValue(new ApiError("mfa_required", 403));

    runWithStepUp(action).catch(() => {}); // stays pending until dialog handled
    await flush();

    expect(stateStore[0]).toBe(true); // dialog open
    expect(fakeRef.current).not.toBeNull(); // pending action stored
  });

  it("passes reason to pending action when provided", async () => {
    const { runWithStepUp } = useTestStepUp();
    const action = vi.fn().mockRejectedValue(new ApiError("mfa_required", 403));

    runWithStepUp(action, "delete_account").catch(() => {});
    await flush();

    expect(fakeRef.current?.reason).toBe("delete_account");
  });

  // ── handleStepUpSubmit ─────────────────────────────────────────────────────

  it("does nothing when there is no pending action", async () => {
    const spy = vi.spyOn(api, "mfaStepUp").mockResolvedValue(undefined as any);
    const { handleStepUpSubmit } = useTestStepUp();
    await handleStepUpSubmit("123456");
    expect(spy).not.toHaveBeenCalled();
  });

  it("calls mfaStepUp with code+reason, resolves action result, closes dialog", async () => {
    vi.spyOn(api, "mfaStepUp").mockResolvedValue(undefined as any);
    const { runWithStepUp, handleStepUpSubmit } = useTestStepUp();
    const action = vi.fn()
      .mockRejectedValueOnce(new ApiError("mfa_required", 403))
      .mockResolvedValueOnce("final");

    const promise = runWithStepUp(action, "admin_action");
    await flush();

    await handleStepUpSubmit("123456");

    expect(api.mfaStepUp).toHaveBeenCalledWith("123456", "admin_action");
    expect(await promise).toBe("final");
    expect(stateStore[0]).toBe(false); // dialog closed
    expect(stateStore[2]).toBe(false); // submitting reset
    expect(fakeRef.current).toBeNull(); // pending cleared
  });

  it("sets error and keeps dialog open on invalid_code", async () => {
    vi.spyOn(api, "mfaStepUp").mockRejectedValue(new ApiError("invalid_code", 400));
    const { runWithStepUp, handleStepUpSubmit } = useTestStepUp();
    const action = vi.fn().mockRejectedValue(new ApiError("mfa_required", 403));

    runWithStepUp(action).catch(() => {});
    await flush();

    await handleStepUpSubmit("wrong");

    expect(stateStore[1]).toBe("invalid_code");
    expect(stateStore[0]).toBe(true); // dialog stays open
    expect(stateStore[2]).toBe(false); // submitting reset
  });

  it("sets error and keeps dialog open on too_many_attempts", async () => {
    vi.spyOn(api, "mfaStepUp").mockRejectedValue(new ApiError("too_many_attempts", 429));
    const { runWithStepUp, handleStepUpSubmit } = useTestStepUp();
    const action = vi.fn().mockRejectedValue(new ApiError("mfa_required", 403));

    runWithStepUp(action).catch(() => {});
    await flush();

    await handleStepUpSubmit("123456");

    expect(stateStore[1]).toBe("too_many_attempts");
    expect(stateStore[0]).toBe(true);
  });

  it("rejects pending and closes dialog on unexpected error from action", async () => {
    vi.spyOn(api, "mfaStepUp").mockResolvedValue(undefined as any);
    const downstreamErr = new Error("server down");
    const { runWithStepUp, handleStepUpSubmit } = useTestStepUp();
    const action = vi.fn()
      .mockRejectedValueOnce(new ApiError("mfa_required", 403))
      .mockRejectedValueOnce(downstreamErr);

    const promise = runWithStepUp(action);
    await flush();

    await handleStepUpSubmit("123456");

    await expect(promise).rejects.toBe(downstreamErr);
    expect(stateStore[0]).toBe(false); // dialog closed
    expect(stateStore[1]).toBe(""); // error cleared
    expect(fakeRef.current).toBeNull();
  });

  // ── handleStepUpClose ──────────────────────────────────────────────────────

  it("rejects pending with mfa_required ApiError and closes dialog", async () => {
    const { runWithStepUp, handleStepUpClose } = useTestStepUp();
    const action = vi.fn().mockRejectedValue(new ApiError("mfa_required", 403));

    const promise = runWithStepUp(action);
    await flush();

    handleStepUpClose();

    await expect(promise).rejects.toMatchObject({ message: "mfa_required" });
    expect(stateStore[0]).toBe(false);
    expect(stateStore[1]).toBe("");
    expect(fakeRef.current).toBeNull();
  });

  it("closes dialog without error even when no pending action", () => {
    stateStore[0] = true;
    const { handleStepUpClose } = useTestStepUp();
    handleStepUpClose();
    expect(stateStore[0]).toBe(false);
  });
});
