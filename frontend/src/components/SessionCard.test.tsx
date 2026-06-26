import { beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { renderToString } from "react-dom/server";
import type { Session } from "@/lib/api";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("@/lib/sessionUtils", () => ({
  formatSessionDevice: vi.fn().mockReturnValue("Chrome on macOS"),
}));

vi.mock("@/lib/dateUtils", () => ({
  formatDateTime: vi.fn().mockReturnValue("Jun 1, 2026"),
}));

let capturedRevokeClick: any = null;

vi.mock("@mui/material/Paper", () => ({
  default: (p: any) => React.createElement("div", { "data-paper": true }, p.children),
}));
vi.mock("@mui/material/Box", () => ({
  default: (p: any) => React.createElement("div", null, p.children),
}));
vi.mock("@mui/material/Typography", () => ({
  default: (p: any) => React.createElement("span", null, p.children),
}));
vi.mock("@mui/material/Button", () => ({
  default: (props: any) => {
    if (props.onClick) capturedRevokeClick = props.onClick;
    return React.createElement("button", {}, props.children);
  },
}));
vi.mock("@mui/material/Chip", () => ({
  default: (props: any) => React.createElement("span", { "data-chip": props.label }, props.label),
}));

import { SessionCard } from "./SessionCard";
import { formatSessionDevice } from "@/lib/sessionUtils";
import { formatDateTime } from "@/lib/dateUtils";

const baseSession: Session = {
  id: "s1",
  ip_address: "10.0.0.1",
  user_agent: "Mozilla/5.0",
  device_info: { browser: "Chrome", os: "macOS" },
  created_at: "2026-01-01T00:00:00Z",
  last_used_at: "2026-06-01T00:00:00Z",
  is_current: false,
};

describe("SessionCard", () => {
  beforeEach(() => {
    capturedRevokeClick = null;
    vi.mocked(formatSessionDevice).mockReturnValue("Chrome on macOS");
  });

  const runRender = (props: Partial<React.ComponentProps<typeof SessionCard>> = {}) =>
    renderToString(
      React.createElement(SessionCard, { session: baseSession, locale: "en-US", ...props })
    );

  it("renders device name, IP address and formatted date", () => {
    const html = runRender();
    expect(html).toContain("Chrome on macOS");
    expect(html).toContain("10.0.0.1");
    expect(html).toContain("Jun 1, 2026");
  });

  it("uses created_at when last_used_at is null", () => {
    vi.mocked(formatDateTime).mockClear();
    runRender({ session: { ...baseSession, last_used_at: null } });
    expect(formatDateTime).toHaveBeenCalledWith(baseSession.created_at, "en-US");
  });

  it("shows current chip and accent bar when is_current=true", () => {
    const html = runRender({ session: { ...baseSession, is_current: true } });
    expect(html).toContain("current");
  });

  it("hides current chip when is_current=false", () => {
    const html = runRender({ session: { ...baseSession, is_current: false } });
    expect(html).not.toContain("data-chip");
  });

  it("renders revoke button and fires onRevoke with session when clicked", () => {
    const onRevoke = vi.fn();
    runRender({ onRevoke });
    expect(capturedRevokeClick).toBeDefined();
    capturedRevokeClick();
    expect(onRevoke).toHaveBeenCalledWith(baseSession);
  });

  it("hides revoke button when onRevoke is not provided", () => {
    const html = runRender({ onRevoke: undefined });
    expect(html).not.toContain("revokeSession");
  });

  it("shows dash when ip_address is empty", () => {
    const html = runRender({ session: { ...baseSession, ip_address: "" } });
    expect(html).toContain("—");
  });
});
