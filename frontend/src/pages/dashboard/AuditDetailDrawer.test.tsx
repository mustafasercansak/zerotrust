import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { AuditDetailDrawer } from "./AuditPage";
import { renderToString } from "react-dom/server";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: "en" },
  }),
}));

// Drawer renders its children unconditionally (always open)
vi.mock("@mui/material/Drawer", () => ({
  default: (props: any) => React.createElement("div", null, props.children),
}));

// State mocking
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
      return [stateStore[idx], stateSetters[idx]];
    },
  };
});

const capturedIconButtonClicks: any[] = [];
vi.mock("@mui/material/IconButton", () => ({
  default: (props: any) => {
    if (props.onClick) capturedIconButtonClicks.push(props.onClick);
    return React.createElement("button", { onClick: props.onClick }, props.children);
  },
}));

describe("AuditDetailDrawer component", () => {
  const onClose = vi.fn();

  const getMockEntry = (overrides: any = {}): any => ({
    id: "a1",
    action: "auth.login",
    resource: "users",
    user_id: "u1",
    user_email: "alice@example.com",
    ip_address: "1.1.1.1",
    user_agent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0",
    created_at: "2026-06-04T12:00:00Z",
    metadata: {
      outcome: "success",
      status: 200,
      reason: "OK",
      location: { city: "New York", country: "US" },
      client_info: { browser: "Chrome", browser_version: "120.0", os: "Windows", os_version: "10" },
    },
    ...overrides,
  });

  beforeEach(() => {
    stateStore = {};
    stateSetters = {};
    callIdx = 0;
    capturedIconButtonClicks.length = 0;
    onClose.mockClear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  const runRender = (entry?: any) => {
    callIdx = 0;
    capturedIconButtonClicks.length = 0;
    return renderToString(
      React.createElement(AuditDetailDrawer, { entry: entry ?? getMockEntry(), onClose }),
    );
  };

  // ── Info section (default activeSection = "info") ─────────────────────────

  it("renders info section by default with action, ip, location, status and reason", () => {
    const html = runRender();
    expect(html).toContain("auth.login");
    expect(html).toContain("1.1.1.1");
    expect(html).toContain("New York");
    expect(html).toContain("US");
    expect(html).toContain("200");
    expect(html).toContain("OK");
    expect(html).toContain("eventDetails");
  });

  it("renders success outcome chip in header", () => {
    const html = runRender();
    expect(html).toContain("success");
  });

  it("renders failure outcome chip in header", () => {
    const html = runRender(getMockEntry({ metadata: { outcome: "failure", status: 401 } }));
    expect(html).toContain("failure");
    expect(html).toContain("401");
  });

  it("renders unknown outcome chip with default color", () => {
    const html = runRender(getMockEntry({ metadata: { outcome: "partial" } }));
    expect(html).toContain("partial");
  });

  it("omits outcome chip when metadata has no outcome", () => {
    const html = runRender(getMockEntry({ metadata: { status: 200 } }));
    expect(html).toContain("200");
    expect(html).not.toContain("success");
  });

  it("renders location with country only when city is absent", () => {
    const html = runRender(getMockEntry({ metadata: { location: { country: "TR" } } }));
    expect(html).toContain("TR");
    expect(html).not.toContain("New York");
  });

  it("omits location row when metadata has no location", () => {
    const html = runRender(getMockEntry({ metadata: { status: 200, reason: "OK" } }));
    expect(html).not.toContain("location");
  });

  it("omits status and reason rows when absent from metadata", () => {
    const html = runRender(getMockEntry({ metadata: { outcome: "success" } }));
    // Check the translation-key labels are not rendered (safe: these won't appear in CSS)
    expect(html).not.toContain("reason");
    expect(html).toContain("eventDetails");
  });

  it("renders raw metadata toggle in collapsed state by default", () => {
    const html = runRender();
    expect(html).toContain("rawMetadata");
    expect(html).toContain("▼");
  });

  it("renders expanded raw metadata when showRaw is true", () => {
    stateStore[1] = true;
    const html = runRender();
    expect(html).toContain("▲");
    // JSON strings are HTML-encoded in SSR output
    expect(html).toContain("&quot;outcome&quot;");
  });

  it("omits raw metadata section when entry has no metadata", () => {
    const html = runRender(getMockEntry({ metadata: undefined }));
    expect(html).toContain("auth.login");
    expect(html).not.toContain("rawMetadata");
  });

  // ── Client section ────────────────────────────────────────────────────────

  it("renders client section with browser, OS, and user-agent string", () => {
    stateStore[0] = "client";
    const html = runRender();
    expect(html).toContain("clientDetails");
    expect(html).toContain("Chrome");
    expect(html).toContain("Windows");
    expect(html).toContain("Mozilla/5.0");
  });

  it("renders client section without user-agent block when user_agent is null", () => {
    stateStore[0] = "client";
    const html = runRender(getMockEntry({ user_agent: null }));
    expect(html).toContain("clientDetails");
    expect(html).not.toContain("userAgent");
  });

  it("renders client section with OS dash fallback when osLabel returns empty string", () => {
    stateStore[0] = "client";
    const html = runRender(getMockEntry({ user_agent: "", metadata: {} }));
    expect(html).toContain("—");
  });

  // ── User section ──────────────────────────────────────────────────────────

  it("renders user section with actor email and user_id", () => {
    stateStore[0] = "user";
    const html = runRender();
    expect(html).toContain("actorDetails");
    expect(html).toContain("alice@example.com");
    expect(html).toContain("u1");
  });

  it("renders only email in user section when user_id is absent", () => {
    stateStore[0] = "user";
    const html = runRender(getMockEntry({ user_id: undefined }));
    expect(html).toContain("alice@example.com");
    expect(html).not.toContain("userId");
  });

  it("renders only user_id in user section when email is absent", () => {
    stateStore[0] = "user";
    const html = runRender(getMockEntry({ user_email: "" }));
    expect(html).toContain("u1");
    expect(html).not.toContain("email");
  });

  it("renders anonymousActor when neither email nor user_id are present", () => {
    stateStore[0] = "user";
    const html = runRender(getMockEntry({ user_email: "", user_id: "" }));
    expect(html).toContain("anonymousActor");
    expect(html).not.toContain("actorDetails");
  });

  // ── Section nav and close ────────────────────────────────────────────────

  it("renders all three section nav tabs", () => {
    const html = runRender();
    expect(html).toContain("drawerInfo");
    expect(html).toContain("drawerClient");
    expect(html).toContain("drawerUser");
  });

  it("calls onClose when the close IconButton is clicked", () => {
    runRender();
    expect(capturedIconButtonClicks.length).toBeGreaterThan(0);
    capturedIconButtonClicks[0]();
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("setActiveSection setter changes rendered section on next render", () => {
    runRender();
    stateSetters[0]("client");
    const html = runRender();
    expect(html).toContain("clientDetails");
  });

  it("setShowRaw setter toggles raw metadata display on next render", () => {
    runRender();
    stateSetters[1]((v: boolean) => !v);
    expect(stateStore[1]).toBe(true);
    const html = runRender();
    expect(html).toContain("▲");
  });
});
