import { afterEach, describe, expect, it, vi } from "vitest";
import { listenToSessionEvents } from "./sessionEvents";

describe("listenToSessionEvents utility", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("handles missing EventSource gracefully (SSR/Node safety)", () => {
    vi.stubGlobal("EventSource", undefined);
    const cleanup = listenToSessionEvents(vi.fn());
    expect(cleanup).toBeInstanceOf(Function);
    cleanup();
  });

  it("subscribes to EventSource and triggers correct callbacks", () => {
    const mockClose = vi.fn();
    let onMessageCallback: ((e: MessageEvent) => void) | null = null;

    class MockEventSource {
      close = mockClose;
      constructor(public url: string) {}
      set onmessage(cb: (e: MessageEvent) => void) {
        onMessageCallback = cb;
      }
    }

    vi.stubGlobal("EventSource", MockEventSource);

    const onRevoked = vi.fn();
    const onChange = vi.fn();

    const cleanup = listenToSessionEvents(onRevoked, onChange);

    expect(onMessageCallback).toBeInstanceOf(Function);

    // 1. Trigger revoked event
    onMessageCallback!(new MessageEvent("message", { data: "revoked" }));
    expect(onRevoked).toHaveBeenCalledTimes(1);

    // 2. Trigger change event
    onMessageCallback!(new MessageEvent("message", { data: "change" }));
    expect(onChange).toHaveBeenCalledTimes(1);

    // 3. Trigger some other event (should do nothing)
    onMessageCallback!(new MessageEvent("message", { data: "unknown" }));

    // 4. Cleanup closes connection
    cleanup();
    expect(mockClose).toHaveBeenCalledTimes(1);
  });
});
