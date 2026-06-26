import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { renderToString } from "react-dom/server";
import { toast } from "sonner";

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

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
      if (callIdx >= 10) callIdx = 0;
      return [stateStore[idx], stateSetters[idx]];
    },
  };
});

let capturedCopyClick: any = null;

vi.mock("@mui/material/Box", () => ({
  default: (p: any) => React.createElement("div", null, p.children),
}));
vi.mock("@mui/material/IconButton", () => ({
  default: (props: any) => {
    if (props.onClick) capturedCopyClick = props.onClick;
    return React.createElement("button", {}, props.children);
  },
}));
vi.mock("@mui/material/Tooltip", () => ({
  default: (p: any) => React.createElement("span", null, p.children),
}));
vi.mock("@mui/material/Typography", () => ({
  default: (p: any) => React.createElement("span", null, p.children),
}));
vi.mock("@mui/icons-material/ContentCopy", () => ({ default: () => React.createElement("i", { "data-icon": "copy" }) }));
vi.mock("@mui/icons-material/Check", () => ({ default: () => React.createElement("i", { "data-icon": "check" }) }));

import { SecretDisplayCard } from "./SecretDisplayCard";

describe("SecretDisplayCard", () => {
  beforeEach(() => {
    stateStore = {};
    stateSetters = {};
    callIdx = 0;
    capturedCopyClick = null;
    vi.mocked(toast.success).mockClear();
    vi.useFakeTimers();
    vi.stubGlobal("navigator", { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } });
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  const runRender = (props: Partial<React.ComponentProps<typeof SecretDisplayCard>> = {}) => {
    callIdx = 0;
    capturedCopyClick = null;
    return renderToString(
      React.createElement(SecretDisplayCard, { label: "API Key", value: "sk-abc123", ...props })
    );
  };

  it("renders label and value", () => {
    const html = runRender();
    expect(html).toContain("API Key");
    expect(html).toContain("sk-abc123");
  });

  it("renders copy icon initially (not check icon)", () => {
    const html = runRender();
    expect(html).toContain('data-icon="copy"');
    expect(html).not.toContain('data-icon="check"');
  });

  it("handleCopy: writes value to clipboard and shows toast with default message", () => {
    runRender();
    capturedCopyClick();
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith("sk-abc123");
    expect(toast.success).toHaveBeenCalledWith("Copied to clipboard");
  });

  it("handleCopy: shows custom successMessage when provided", () => {
    runRender({ successMessage: "Key copied!" });
    capturedCopyClick();
    expect(toast.success).toHaveBeenCalledWith("Key copied!");
  });

  it("handleCopy: sets copied=true immediately then resets to false after 2000ms", () => {
    runRender();
    expect(stateStore[0]).toBe(false);
    capturedCopyClick();
    expect(stateStore[0]).toBe(true);
    vi.advanceTimersByTime(2000);
    expect(stateStore[0]).toBe(false);
  });

  it("renders check icon when copied=true", () => {
    stateStore[0] = true;
    const html = runRender();
    expect(html).toContain('data-icon="check"');
    expect(html).not.toContain('data-icon="copy"');
  });
});
