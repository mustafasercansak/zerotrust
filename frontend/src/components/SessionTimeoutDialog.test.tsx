import { afterEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { renderToString } from "react-dom/server";
import { SessionTimeoutDialog } from "./SessionTimeoutDialog";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("@mui/material/Dialog", () => ({
  default: (props: { open: boolean; children: React.ReactNode }) =>
    props.open ? React.createElement("div", { "data-testid": "dialog" }, props.children) : null,
}));

vi.mock("@mui/material/DialogTitle", () => ({
  default: (props: { children: React.ReactNode }) =>
    React.createElement("div", null, props.children),
}));

vi.mock("@mui/material/DialogContent", () => ({
  default: (props: { children: React.ReactNode }) =>
    React.createElement("div", null, props.children),
}));

vi.mock("@mui/material/DialogActions", () => ({
  default: (props: { children: React.ReactNode }) =>
    React.createElement("div", null, props.children),
}));

vi.mock("@mui/material/LinearProgress", () => ({
  default: (props: { value: number; color: string }) =>
    React.createElement("div", { "data-value": String(props.value), "data-color": props.color }),
}));

vi.mock("@mui/material/Typography", () => ({
  default: (props: { children: React.ReactNode }) =>
    React.createElement("span", null, props.children),
}));

vi.mock("@mui/material/Button", () => ({
  default: (props: { children: React.ReactNode; onClick?: () => void }) =>
    React.createElement("button", { onClick: props.onClick }, props.children),
}));

vi.mock("@mui/material/Box", () => ({
  default: (props: { children: React.ReactNode }) =>
    React.createElement("div", null, props.children),
}));

vi.mock("@mui/icons-material/TimerOutlined", () => ({
  default: () => React.createElement("span", null, "TimerIcon"),
}));

afterEach(() => {
  vi.restoreAllMocks();
});

describe("SessionTimeoutDialog", () => {
  it("renders nothing when closed", () => {
    const html = renderToString(
      React.createElement(SessionTimeoutDialog, {
        open: false,
        secondsRemaining: 45,
        onExtend: vi.fn(),
        onLogout: vi.fn(),
      })
    );
    expect(html).toBe("");
  });

  it("renders dialog content when open", () => {
    const html = renderToString(
      React.createElement(SessionTimeoutDialog, {
        open: true,
        secondsRemaining: 45,
        onExtend: vi.fn(),
        onLogout: vi.fn(),
      })
    );
    expect(html).toContain("title");
    expect(html).toContain("45");
    expect(html).toContain("extend");
    expect(html).toContain("logoutNow");
  });

  it("shows error color when secondsRemaining <= 15", () => {
    const html = renderToString(
      React.createElement(SessionTimeoutDialog, {
        open: true,
        secondsRemaining: 10,
        onExtend: vi.fn(),
        onLogout: vi.fn(),
      })
    );
    expect(html).toContain("error");
  });

  it("shows warning color when secondsRemaining > 15", () => {
    const html = renderToString(
      React.createElement(SessionTimeoutDialog, {
        open: true,
        secondsRemaining: 30,
        onExtend: vi.fn(),
        onLogout: vi.fn(),
      })
    );
    expect(html).toContain("warning");
  });

  it("progress bar value reflects secondsRemaining out of 60", () => {
    const html = renderToString(
      React.createElement(SessionTimeoutDialog, {
        open: true,
        secondsRemaining: 30,
        onExtend: vi.fn(),
        onLogout: vi.fn(),
      })
    );
    // 30/60 * 100 = 50
    expect(html).toContain("50");
  });
});
