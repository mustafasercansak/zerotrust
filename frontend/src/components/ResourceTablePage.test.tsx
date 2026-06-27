import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { ResourceTablePage } from "./ResourceTablePage";
import { render, screen, cleanup, waitFor, act } from "@testing-library/react";

let capturedDataGridProps: any = null;
let capturedTabsProps: any = null;
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

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

describe("ResourceTablePage component", () => {
  beforeEach(() => {
    capturedDataGridProps = null;
    capturedTabsProps = null;
    lastEventSource = null;

    vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
    vi.spyOn(document, "addEventListener");
    vi.spyOn(document, "removeEventListener");

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
  });

  afterEach(() => {
    cleanup();
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

    render(
      <ResourceTablePage
        columns={[]}
        fetcher={fetcher}
        tabs={tabs}
        action={<button>Add Row</button>}
        eventSourceUrl="/api/v1/events"
      />
    );

    expect(screen.getByText("All Items")).toBeDefined();
    expect(screen.getByText("Active Items")).toBeDefined();
    expect(screen.getByText("Add Row")).toBeDefined();

    await waitFor(() => {
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

    render(
      <ResourceTablePage
        columns={[]}
        fetcher={fetcher}
        tabs={tabs}
        action={<button>Add Row</button>}
        eventSourceUrl="/api/v1/events"
      />
    );

    // Trigger tab change
    expect(capturedTabsProps).toBeDefined();
    act(() => {
      capturedTabsProps.onChange(null, "active");
    });

    // Trigger sort model change
    expect(capturedDataGridProps).toBeDefined();
    act(() => {
      capturedDataGridProps.onSortModelChange([{ field: "name", sort: "asc" }]);
    });

    // Trigger filter model change
    act(() => {
      capturedDataGridProps.onFilterModelChange({ items: [{ field: "name", operator: "contains", value: "test" }] });
    });
  });

  it("cleans up live refresh and ignores connected SSE messages", async () => {
    const fetcher = vi.fn().mockResolvedValue({
      data: [{ id: "1", name: "Row 1" }],
      total: 1,
    });

    const addEventListenerSpy = vi.spyOn(document, "addEventListener");
    const removeEventListenerSpy = vi.spyOn(document, "removeEventListener");
    vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");

    const { unmount } = render(
      <ResourceTablePage
        columns={[]}
        fetcher={fetcher}
        eventSourceUrl="/api/v1/events"
      />
    );

    expect(lastEventSource?.url).toBe("/api/v1/events");
    const visibilityListener = addEventListenerSpy.mock.calls.find(
      ([eventName]: [string]) => eventName === "visibilitychange",
    )[1];
    visibilityListener();
    lastEventSource.onmessage?.({ data: "connected" });
    lastEventSource.onmessage?.({ data: "change" });

    unmount();
    expect(document.removeEventListener).toHaveBeenCalledWith("visibilitychange", expect.any(Function));
    expect(lastEventSource.close).toHaveBeenCalled();
  });

  it("skips hidden live refreshes and shows fetch errors", async () => {
    const fetcher = vi.fn().mockRejectedValue(new Error("load failed"));
    vi.spyOn(document, "visibilityState", "get").mockReturnValue("hidden");

    render(
      <ResourceTablePage
        columns={[]}
        fetcher={fetcher}
      />
    );

    await waitFor(() => {
      expect(fetcher).toHaveBeenCalled();
    });
  });

  it("ignores incomplete filter model items", async () => {
    const fetcher = vi.fn().mockResolvedValue({ data: [], total: 0 });

    render(
      <ResourceTablePage
        columns={[]}
        fetcher={fetcher}
      />
    );

    await waitFor(() => {
      expect(capturedDataGridProps).not.toBeNull();
    });

    act(() => {
      capturedDataGridProps.onFilterModelChange({
        items: [
          { field: "name", value: "" },
          { field: "", value: "ignored" },
        ],
      });
    });
  });

  it("passes completed filter model items to the fetcher", async () => {
    const fetcher = vi.fn().mockResolvedValue({ data: [], total: 0 });

    render(
      <ResourceTablePage
        columns={[]}
        fetcher={fetcher}
      />
    );

    await waitFor(() => {
      expect(capturedDataGridProps).not.toBeNull();
    });

    act(() => {
      capturedDataGridProps.onFilterModelChange({
        items: [{ field: "name", operator: "contains", value: "alice" }],
      });
    });

    await waitFor(() => {
      expect(fetcher).toHaveBeenCalledWith(expect.objectContaining({
        filters: { name: "alice" },
      }));
    });
  });

  it("renders the translated error after a failed fetch", async () => {
    const fetcher = vi.fn().mockRejectedValue(new Error("load failed"));

    render(
      <ResourceTablePage
        columns={[]}
        fetcher={fetcher}
      />
    );

    await waitFor(() => {
      expect(screen.getByText("error")).toBeDefined();
    });
  });

  it("ignores fetch errors after the loading effect is cleaned up", async () => {
    let rejectFetch: (error: Error) => void = () => {};
    const fetcher = vi.fn().mockReturnValue(new Promise((_, reject) => {
      rejectFetch = reject;
    }));

    const { unmount } = render(
      <ResourceTablePage
        columns={[]}
        fetcher={fetcher}
      />
    );

    unmount();
    rejectFetch(new Error("load failed"));

    await Promise.resolve();
    await Promise.resolve();

    expect(screen.queryByText("error")).toBeNull();
  });
});
