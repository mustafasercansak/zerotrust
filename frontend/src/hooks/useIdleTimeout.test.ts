import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "@/lib/api";

vi.mock("@/lib/api", () => ({
  api: { getSessionPolicy: vi.fn() },
}));

// State and effect store — same pattern as SettingsPage.test.tsx.
let stateStore: Record<number, unknown> = {};
let effectFns: Array<() => void | (() => void)> = [];
let callIdx = 0;

vi.mock("react", async (importOriginal) => {
  const original = await importOriginal<typeof import("react")>();
  return {
    ...original,
    useState: (init: unknown) => {
      const idx = callIdx++;
      if (!(idx in stateStore)) stateStore[idx] = init;
      const setter = (v: unknown) => {
        stateStore[idx] = typeof v === "function" ? (v as (p: unknown) => unknown)(stateStore[idx]) : v;
      };
      return [stateStore[idx], setter];
    },
    useEffect: (fn: () => void | (() => void)) => { effectFns.push(fn); },
    useCallback: (fn: unknown) => fn,
    useRef: (init: unknown) => ({ current: init }),
  };
});

const mockAddEventListener = vi.fn();
const mockRemoveEventListener = vi.fn();

beforeEach(() => {
  vi.useFakeTimers();
  stateStore = {};
  effectFns = [];
  callIdx = 0;
  vi.mocked(api.getSessionPolicy).mockResolvedValue({ idle_timeout_seconds: 120 });
  vi.stubGlobal("window", {
    addEventListener: mockAddEventListener,
    removeEventListener: mockRemoveEventListener,
  });
  mockAddEventListener.mockClear();
  mockRemoveEventListener.mockClear();
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("useIdleTimeout hook", () => {
  it("returns correct initial shape", async () => {
    const { useIdleTimeout } = await import("./useIdleTimeout");
    callIdx = 0;
    const result = useIdleTimeout(vi.fn());

    expect(result.warningVisible).toBe(false);
    expect(result.secondsRemaining).toBe(60);
    expect(typeof result.extendSession).toBe("function");
    expect(typeof result.dismissWarning).toBe("function");
  });

  it("calls getSessionPolicy on mount", async () => {
    const { useIdleTimeout } = await import("./useIdleTimeout");
    callIdx = 0;
    useIdleTimeout(vi.fn());

    // Run all collected effects.
    for (const fn of effectFns) fn();
    await vi.runAllTimersAsync();

    expect(api.getSessionPolicy).toHaveBeenCalled();
  });

  it("swallows getSessionPolicy errors silently", async () => {
    vi.mocked(api.getSessionPolicy).mockRejectedValueOnce(new Error("network"));
    const { useIdleTimeout } = await import("./useIdleTimeout");
    callIdx = 0;
    useIdleTimeout(vi.fn());

    for (const fn of effectFns) fn();
    // Expect no unhandled rejection — swallowed silently.
    await vi.runAllTimersAsync();
  });

  it("registers activity event listeners", async () => {
    const { useIdleTimeout } = await import("./useIdleTimeout");
    callIdx = 0;
    useIdleTimeout(vi.fn());

    for (const fn of effectFns) fn();

    expect(mockAddEventListener).toHaveBeenCalledWith("mousemove", expect.any(Function), { passive: true });
    expect(mockAddEventListener).toHaveBeenCalledWith("keydown", expect.any(Function), { passive: true });
    expect(mockAddEventListener).toHaveBeenCalledWith("click", expect.any(Function), { passive: true });
  });

  it("removes activity event listeners on cleanup", async () => {
    const { useIdleTimeout } = await import("./useIdleTimeout");
    callIdx = 0;
    useIdleTimeout(vi.fn());

    // Run the listeners effect and capture its cleanup.
    const listenerEffect = effectFns.find((_, i) => i === 1);
    const cleanup = listenerEffect?.() as (() => void) | undefined;
    cleanup?.();

    expect(mockRemoveEventListener).toHaveBeenCalledWith("mousemove", expect.any(Function));
  });
});
