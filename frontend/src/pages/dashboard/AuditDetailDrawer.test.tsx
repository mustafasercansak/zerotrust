import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { AuditDetailDrawer } from "./AuditPage";
import type { AuditEntry } from "@/lib/api";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: "en" },
  }),
}));

// Drawer renders its children unconditionally (always open)
vi.mock("@mui/material/Drawer", () => ({
  default: (props: any) => React.createElement("div", { "data-testid": "drawer" }, props.children),
}));

describe("AuditDetailDrawer component", () => {
  const onClose = vi.fn();

  const getMockEntry = (overrides: any = {}): AuditEntry => ({
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
    onClose.mockClear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  // ── Info section (default activeSection = "info") ─────────────────────────

  it("renders info section by default with action, ip, location, status and reason", () => {
    render(React.createElement(AuditDetailDrawer, { entry: getMockEntry(), onClose }));

    expect(screen.getAllByText("auth.login").length).toBeGreaterThan(0);
    expect(screen.getByText("1.1.1.1")).toBeDefined();
    expect(screen.getByText("New York, US")).toBeDefined();
    expect(screen.getByText("200")).toBeDefined();
    expect(screen.getByText("OK")).toBeDefined();
    expect(screen.getByText("eventDetails")).toBeDefined();
  });

  it("renders success outcome chip in header", () => {
    render(React.createElement(AuditDetailDrawer, { entry: getMockEntry(), onClose }));
    expect(screen.getByText("success")).toBeDefined();
  });

  it("renders failure outcome chip in header", () => {
    render(React.createElement(AuditDetailDrawer, { entry: getMockEntry({ metadata: { outcome: "failure", status: 401 } }), onClose }));
    expect(screen.getByText("failure")).toBeDefined();
    expect(screen.getByText("401")).toBeDefined();
  });

  it("renders unknown outcome chip with default color", () => {
    render(React.createElement(AuditDetailDrawer, { entry: getMockEntry({ metadata: { outcome: "partial" } }), onClose }));
    expect(screen.getByText("partial")).toBeDefined();
  });

  it("omits outcome chip when metadata has no outcome", () => {
    render(React.createElement(AuditDetailDrawer, { entry: getMockEntry({ metadata: { status: 200 } }), onClose }));
    expect(screen.queryByText("success")).toBeNull();
    expect(screen.getByText("200")).toBeDefined();
  });

  it("renders location with country only when city is absent", () => {
    render(React.createElement(AuditDetailDrawer, { entry: getMockEntry({ metadata: { location: { country: "TR" } } }), onClose }));
    expect(screen.getByText("TR")).toBeDefined();
    expect(screen.queryByText("New York")).toBeNull();
  });

  it("omits location row when metadata has no location", () => {
    render(React.createElement(AuditDetailDrawer, { entry: getMockEntry({ metadata: { status: 200, reason: "OK" } }), onClose }));
    expect(screen.queryByText("location")).toBeNull();
  });

  it("omits status and reason rows when absent from metadata", () => {
    render(React.createElement(AuditDetailDrawer, { entry: getMockEntry({ metadata: { outcome: "success" } }), onClose }));
    expect(screen.queryByText("reason")).toBeNull();
    expect(screen.getByText("eventDetails")).toBeDefined();
  });

  it("renders raw metadata toggle in collapsed state by default", () => {
    render(React.createElement(AuditDetailDrawer, { entry: getMockEntry(), onClose }));
    expect(screen.getByText("rawMetadata")).toBeDefined();
    expect(screen.getByText("▼")).toBeDefined();
  });

  it("renders expanded raw metadata when clicked", () => {
    render(React.createElement(AuditDetailDrawer, { entry: getMockEntry(), onClose }));
    fireEvent.click(screen.getByText("rawMetadata"));
    expect(screen.getByText("▲")).toBeDefined();
    expect(screen.getByText(/"outcome": "success"/)).toBeDefined();
  });

  it("omits raw metadata section when entry has no metadata", () => {
    render(React.createElement(AuditDetailDrawer, { entry: getMockEntry({ metadata: undefined }), onClose }));
    expect(screen.getAllByText("auth.login").length).toBeGreaterThan(0);
    expect(screen.queryByText("rawMetadata")).toBeNull();
  });

  // ── Client section ────────────────────────────────────────────────────────

  it("renders client section with browser, OS, and user-agent string", () => {
    render(React.createElement(AuditDetailDrawer, { entry: getMockEntry(), onClose }));
    fireEvent.click(screen.getByText("drawerClient"));

    expect(screen.getByText("clientDetails")).toBeDefined();
    expect(screen.getByText("Chrome")).toBeDefined();
    expect(screen.getByText("Windows 10")).toBeDefined();
    expect(screen.getByText("Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0")).toBeDefined();
  });

  it("renders client section without user-agent block when user_agent is null", () => {
    render(React.createElement(AuditDetailDrawer, { entry: getMockEntry({ user_agent: null }), onClose }));
    fireEvent.click(screen.getByText("drawerClient"));

    expect(screen.getByText("clientDetails")).toBeDefined();
    expect(screen.queryByText("userAgent")).toBeNull();
  });

  it("renders client section with OS dash fallback when osLabel returns empty string", () => {
    render(React.createElement(AuditDetailDrawer, { entry: getMockEntry({ user_agent: "", metadata: {} }), onClose }));
    fireEvent.click(screen.getByText("drawerClient"));
    expect(screen.getByText("—")).toBeDefined();
  });

  // ── User section ──────────────────────────────────────────────────────────

  it("renders user section with actor email and user_id", () => {
    render(React.createElement(AuditDetailDrawer, { entry: getMockEntry(), onClose }));
    fireEvent.click(screen.getByText("drawerUser"));

    expect(screen.getByText("actorDetails")).toBeDefined();
    expect(screen.getByText("alice@example.com")).toBeDefined();
    expect(screen.getByText("u1")).toBeDefined();
  });

  it("renders only email in user section when user_id is absent", () => {
    render(React.createElement(AuditDetailDrawer, { entry: getMockEntry({ user_id: undefined }), onClose }));
    fireEvent.click(screen.getByText("drawerUser"));

    expect(screen.getByText("alice@example.com")).toBeDefined();
    expect(screen.queryByText("userId")).toBeNull();
  });

  it("renders only user_id in user section when email is absent", () => {
    render(React.createElement(AuditDetailDrawer, { entry: getMockEntry({ user_email: "" }), onClose }));
    fireEvent.click(screen.getByText("drawerUser"));

    expect(screen.getByText("u1")).toBeDefined();
    expect(screen.queryByText("email")).toBeNull();
  });

  it("renders anonymousActor when neither email nor user_id are present", () => {
    render(React.createElement(AuditDetailDrawer, { entry: getMockEntry({ user_email: "", user_id: "" }), onClose }));
    fireEvent.click(screen.getByText("drawerUser"));

    expect(screen.getByText("anonymousActor")).toBeDefined();
    expect(screen.queryByText("actorDetails")).toBeNull();
  });

  // ── Section nav and close ────────────────────────────────────────────────

  it("renders all three section nav tabs", () => {
    render(React.createElement(AuditDetailDrawer, { entry: getMockEntry(), onClose }));
    expect(screen.getByText("drawerInfo")).toBeDefined();
    expect(screen.getByText("drawerClient")).toBeDefined();
    expect(screen.getByText("drawerUser")).toBeDefined();
  });

  it("calls onClose when the close button is clicked", () => {
    render(React.createElement(AuditDetailDrawer, { entry: getMockEntry(), onClose }));
    const closeBtn = screen.getByTestId("CloseIcon");
    fireEvent.click(closeBtn);
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
