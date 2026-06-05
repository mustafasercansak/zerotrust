import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { ResourceTablePage } from "./ResourceTablePage";
import { renderToString } from "react-dom/server";

let capturedDataGridProps: any = null;
let capturedTabsProps: any = null;
let effectCleanups: Array<() => void> = [];
let lastEventSource: any = null;

// Mock DataGrid from MUI
vi.mock("@mui/x-data-grid", () => ({
  DataGrid: (props: any) => {
    capturedDataGridProps = props;
    return React.createElement("div", null, `DataGrid rows: ${props.rows?.length ?? 0}`);
  },
}));

vi.mock("@mui/material/Tabs", () => ({
  default: (props: any) => {
    capturedTabsProps = props;
    return React.createElement("div", null, props.children);
  }
}));

vi.mock("@mui/material/Tab", () => ({
  default: (props: any) => {
    return React.createElement("div", null, props.label);
  }
}));

// State Mocking System
let stateStore: any = {};
let stateSetters: any = {};
let callIdx = 0;

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
      if (callIdx >= 200) {
        callIdx = 0;
      }
      return [stateStore[idx], stateSetters[idx]];
    },
    useEffect: (fn: any) => {
      // Execute useEffect hooks synchronously
      const cleanup = fn();
      if (typeof cleanup === "function") effectCleanups.push(cleanup);
    },
  };
});

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

describe("ResourceTablePage component", () => {
  beforeEach(() => {
    stateStore = {};
    stateSetters = {};
    callIdx = 0;
    capturedDataGridProps = null;
    capturedTabsProps = null;
    effectCleanups = [];
    lastEventSource = null;
    vi.stubGlobal("document", {
      visibilityState: "visible",
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    });
    vi.stubGlobal("window", {
      setInterval: vi.fn((fn: any, delay: number) => {
        fn();
        return 123;
      }),
      clearInterval: vi.fn(),
    });
    class MockEventSource {
      onmessage: ((event: { data: string }) => void) | null = null;
      close = vi.fn();
      addEventListener = vi.fn();
      removeEventListener = vi.fn();
      constructor(public url: string) {
        lastEventSource = this;
      }
    }
    vi.stubGlobal("EventSource", MockEventSource);
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("fetches rows on mount and renders tabs and actions", async () => {
    const fetcher = vi.fn().mockResolvedValue({
      data: [{ id: "1", name: "Row 1" }],
      total: 1,
    });

    const tabs = [
      { key: "all", label: "All Items" },
      { key: "active", label: "Active Items", preset: { status: "active" } },
    ];

    const html = renderToString(
      React.createElement(ResourceTablePage, {
        columns: [],
        fetcher,
        tabs,
        action: React.createElement("button", null, "Add Row"),
        eventSourceUrl: "/api/v1/events",
      })
    );

    expect(html).toContain("All Items");
    expect(html).toContain("Active Items");
    expect(html).toContain("Add Row");

    await vi.waitFor(() => {
      expect(fetcher).toHaveBeenCalled();
    });
  });

  it("triggers filter, sort, and tab selection onChange event handlers", async () => {
    const fetcher = vi.fn().mockResolvedValue({
      data: [{ id: "1", name: "Row 1" }],
      total: 1,
    });

    const tabs = [
      { key: "all", label: "All Items" },
      { key: "active", label: "Active Items", preset: { status: "active" } },
    ];

    renderToString(
      React.createElement(ResourceTablePage, {
        columns: [],
        fetcher,
        tabs,
        action: React.createElement("button", null, "Add Row"),
        eventSourceUrl: "/api/v1/events",
      })
    );

    // Trigger tab change
    expect(capturedTabsProps).toBeDefined();
    capturedTabsProps.onChange(null, "active");

    // Trigger sort model change
    expect(capturedDataGridProps).toBeDefined();
    capturedDataGridProps.onSortModelChange([{ field: "name", sort: "asc" }]);

    // Trigger filter model change
    capturedDataGridProps.onFilterModelChange({ items: [{ field: "name", operator: "contains", value: "test" }] });
  });

  it("cleans up live refresh and ignores connected SSE messages", async () => {
    const fetcher = vi.fn().mockResolvedValue({
      data: [{ id: "1", name: "Row 1" }],
      total: 1,
    });

    renderToString(
      React.createElement(ResourceTablePage, {
        columns: [],
        fetcher,
        eventSourceUrl: "/api/v1/events",
      })
    );

    expect(lastEventSource?.url).toBe("/api/v1/events");
    const visibilityListener = (document.addEventListener as any).mock.calls.find(
      ([eventName]: [string]) => eventName === "visibilitychange",
    )[1];
    visibilityListener();
    lastEventSource.onmessage?.({ data: "connected" });
    lastEventSource.onmessage?.({ data: "change" });

    effectCleanups.forEach((cleanup) => cleanup());
    expect(window.clearInterval).toHaveBeenCalledWith(123);
    expect(document.removeEventListener).toHaveBeenCalledWith("visibilitychange", expect.any(Function));
    expect(lastEventSource.close).toHaveBeenCalled();
  });

  it("skips hidden live refreshes and shows fetch errors", async () => {
    const fetcher = vi.fn().mockRejectedValue(new Error("load failed"));
    vi.stubGlobal("document", {
      visibilityState: "hidden",
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    });

    renderToString(
      React.createElement(ResourceTablePage, {
        columns: [],
        fetcher,
      })
    );

    await vi.waitFor(() => {
      expect(fetcher).toHaveBeenCalled();
    });
    await Promise.resolve();
    await Promise.resolve();

    renderToString(
      React.createElement(ResourceTablePage, {
        columns: [],
        fetcher,
      })
    );
    expect(fetcher).toHaveBeenCalled();
  });

  it("ignores incomplete filter model items", () => {
    const fetcher = vi.fn().mockResolvedValue({ data: [], total: 0 });

    renderToString(
      React.createElement(ResourceTablePage, {
        columns: [],
        fetcher,
      })
    );

    capturedDataGridProps.onFilterModelChange({
      items: [
        { field: "name", value: "" },
        { field: "", value: "ignored" },
      ],
    });

    callIdx = 0;
    renderToString(
      React.createElement(ResourceTablePage, {
        columns: [],
        fetcher,
      })
    );
  });

  it("passes completed filter model items to the fetcher", async () => {
    const fetcher = vi.fn().mockResolvedValue({ data: [], total: 0 });

    renderToString(
      React.createElement(ResourceTablePage, {
        columns: [],
        fetcher,
      })
    );

    capturedDataGridProps.onFilterModelChange({
      items: [{ field: "name", operator: "contains", value: "alice" }],
    });

    callIdx = 0;
    renderToString(
      React.createElement(ResourceTablePage, {
        columns: [],
        fetcher,
      })
    );

    await vi.waitFor(() => {
      expect(fetcher).toHaveBeenCalledWith(expect.objectContaining({
        filters: { name: "alice" },
      }));
    });
  });

  it("renders the translated error after a failed fetch", async () => {
    const fetcher = vi.fn().mockRejectedValue(new Error("load failed"));

    renderToString(
      React.createElement(ResourceTablePage, {
        columns: [],
        fetcher,
      })
    );

    await vi.waitFor(() => {
      expect(stateStore[3]).toBe("error");
    });

    callIdx = 0;
    const html = renderToString(
      React.createElement(ResourceTablePage, {
        columns: [],
        fetcher,
      })
    );

    expect(html).toContain("error");
  });

  it("ignores fetch errors after the loading effect is cleaned up", async () => {
    let rejectFetch: (error: Error) => void = () => {};
    const fetcher = vi.fn().mockReturnValue(new Promise((_, reject) => {
      rejectFetch = reject;
    }));

    renderToString(
      React.createElement(ResourceTablePage, {
        columns: [],
        fetcher,
      })
    );

    effectCleanups.forEach((cleanup) => cleanup());
    rejectFetch(new Error("load failed"));

    await Promise.resolve();
    await Promise.resolve();

    expect(stateStore[3]).toBe("");
  });
});
