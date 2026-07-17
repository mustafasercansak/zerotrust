import { beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { renderToString } from "react-dom/server";
import type { AuditEntry } from "@/lib/api";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("@/lib/dateUtils", () => ({
  formatDateTime: vi.fn().mockReturnValue("Jun 1, 2026"),
}));

let capturedPaperClick: any = null;

vi.mock("@mui/material/Paper", () => ({
  default: (props: any) => {
    if (props.onClick) capturedPaperClick = props.onClick;
    return React.createElement("div", { "data-paper": true }, props.children);
  },
}));
vi.mock("@mui/material/Box", () => ({
  default: (p: any) => React.createElement("div", null, p.children),
}));
vi.mock("@mui/material/Typography", () => ({
  default: (p: any) => React.createElement("span", null, p.children),
}));
vi.mock("@mui/material/Chip", () => ({
  default: (props: any) =>
    React.createElement("span", { "data-chip": props.color }, props.label),
}));

import { AuditEntryCard } from "./AuditEntryCard";

const baseEntry: AuditEntry = {
  id: "e1",
  user_id: "u1",
  user_email: "test@example.com",
  action: "auth.login_success",
  resource: "/api/auth/login",
  ip_address: "10.0.0.1",
  user_agent: "Mozilla/5.0",
  metadata: {},
  created_at: "2026-06-01T12:00:00Z",
};

describe("AuditEntryCard", () => {
  beforeEach(() => {
    capturedPaperClick = null;
  });

  const runRender = (props: Partial<React.ComponentProps<typeof AuditEntryCard>> = {}) =>
    renderToString(
      React.createElement(AuditEntryCard, { entry: baseEntry, locale: "en-US", ...props })
    );

  it("renders action, IP address and formatted date", () => {
    const html = runRender();
    expect(html).toContain("auth.login_success");
    expect(html).toContain("10.0.0.1");
    expect(html).toContain("Jun 1, 2026");
  });

  it("shows success chip when outcome is 'success'", () => {
    const html = runRender({ entry: { ...baseEntry, metadata: { outcome: "success" } } });
    expect(html).toContain('data-chip="success"');
    expect(html).toContain("success"); // label via t("success")
  });

  it("shows error chip when outcome is 'failure'", () => {
    const html = runRender({ entry: { ...baseEntry, metadata: { outcome: "failure" } } });
    expect(html).toContain('data-chip="error"');
    expect(html).toContain("failure");
  });

  it("shows default chip with raw label for unknown outcome", () => {
    const html = runRender({ entry: { ...baseEntry, metadata: { outcome: "partial" } } });
    expect(html).toContain('data-chip="default"');
    expect(html).toContain("partial");
  });

  it("renders no chip when metadata has no outcome", () => {
    const html = runRender({ entry: { ...baseEntry, metadata: {} } });
    expect(html).not.toContain("data-chip");
  });

  it("hides IP address in compact mode", () => {
    const html = runRender({ compact: true });
    expect(html).not.toContain("10.0.0.1");
  });

  it("shows dash for IP when ip_address is null", () => {
    const html = runRender({ entry: { ...baseEntry, ip_address: null } });
    expect(html).toContain("—");
  });

  it("fires onClick with the entry when card is clicked", () => {
    const onClick = vi.fn();
    runRender({ onClick });
    expect(capturedPaperClick).toBeDefined();
    capturedPaperClick();
    expect(onClick).toHaveBeenCalledWith(baseEntry);
  });

  it("does not set onClick on Paper when onClick prop is omitted", () => {
    runRender({ onClick: undefined });
    expect(capturedPaperClick).toBeNull();
  });

  it("renders location and flag emoji when location is present in metadata", () => {
    const html = runRender({
      entry: {
        ...baseEntry,
        metadata: {
          location: { country: "Turkey", city: "Istanbul" },
        },
      },
    });
    expect(html).toContain("Istanbul, Turkey");
    expect(html).toContain("🇹🇷");
  });

  it("renders compact location in compact mode when location is present in metadata", () => {
    const html = runRender({
      compact: true,
      entry: {
        ...baseEntry,
        metadata: {
          location: { country: "United States", city: "New York" },
        },
      },
    });
    expect(html).toContain("New York, United States");
    expect(html).toContain("🇺🇸");
  });
});
