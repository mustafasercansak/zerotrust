import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { renderToString } from "react-dom/server";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, opts?: any) => (opts ? `${key}(${JSON.stringify(opts)})` : key),
  }),
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

let capturedFormSubmit: any = null;
let capturedDialogClose: any = null;
let capturedCancelClick: any = null;

vi.mock("@mui/material/Dialog", () => ({
  default: (props: any) => {
    capturedDialogClose = props.onClose;
    return props.open ? React.createElement("div", { role: "dialog" }, props.children) : null;
  },
}));
vi.mock("@mui/material/Box", () => ({
  default: (props: any) => {
    if (props.onSubmit) capturedFormSubmit = props.onSubmit;
    return React.createElement("div", null, props.children);
  },
}));
vi.mock("@mui/material/Button", () => ({
  default: (props: any) => {
    if (props.color === "inherit") capturedCancelClick = props.onClick;
    return React.createElement("button", { type: props.type, disabled: props.disabled }, props.children);
  },
}));
vi.mock("@mui/material/TextField", () => ({
  default: (props: any) =>
    React.createElement("input", {
      value: props.value,
      disabled: props.disabled,
      "data-error": props.error ? "true" : undefined,
      "data-helpertext": props.helperText,
    }),
}));
vi.mock("@mui/material/DialogTitle", () => ({ default: (p: any) => React.createElement("h2", null, p.children) }));
vi.mock("@mui/material/DialogContent", () => ({ default: (p: any) => React.createElement("div", null, p.children) }));
vi.mock("@mui/material/DialogActions", () => ({ default: (p: any) => React.createElement("div", null, p.children) }));
vi.mock("@mui/material/Typography", () => ({ default: (p: any) => React.createElement("p", null, p.children) }));
vi.mock("@mui/icons-material/Lock", () => ({ default: () => null }));

import { StepUpMfaDialog } from "./StepUpMfaDialog";

describe("StepUpMfaDialog", () => {
  const onSubmit = vi.fn();
  const onClose = vi.fn();

  const runRender = (props: Partial<React.ComponentProps<typeof StepUpMfaDialog>> = {}) => {
    callIdx = 0;
    capturedFormSubmit = null;
    capturedDialogClose = null;
    capturedCancelClick = null;
    return renderToString(
      React.createElement(StepUpMfaDialog, { open: true, onSubmit, onClose, ...props })
    );
  };

  beforeEach(() => {
    stateStore = {};
    stateSetters = {};
    callIdx = 0;
    onSubmit.mockClear();
    onClose.mockClear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders title, description and action buttons", () => {
    const html = runRender();
    expect(html).toContain("stepUpMfa.title");
    expect(html).toContain("stepUpMfa.description");
    expect(html).toContain("stepUpMfa.verify");
    expect(html).toContain("cancel");
  });

  it("renders nothing when open=false", () => {
    const html = runRender({ open: false });
    expect(html).toBe("");
  });

  it("handleSubmit: does not call onSubmit when code is empty", () => {
    runRender(); // stateStore[0] = "" from useState("")
    capturedFormSubmit({ preventDefault: vi.fn() });
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("handleSubmit: does not call onSubmit when code is whitespace only", () => {
    stateStore[0] = "   ";
    runRender();
    capturedFormSubmit({ preventDefault: vi.fn() });
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("handleSubmit: calls onSubmit with trimmed code and clears code", () => {
    stateStore[0] = " 123456 ";
    runRender();
    capturedFormSubmit({ preventDefault: vi.fn() });
    expect(onSubmit).toHaveBeenCalledWith("123456");
    expect(stateStore[0]).toBe("");
  });

  it("handleClose via Dialog onClose: clears code and calls onClose", () => {
    stateStore[0] = "999999";
    runRender();
    capturedDialogClose();
    expect(stateStore[0]).toBe("");
    expect(onClose).toHaveBeenCalled();
  });

  it("handleClose via cancel button: clears code and calls onClose", () => {
    stateStore[0] = "555";
    runRender();
    capturedCancelClick();
    expect(stateStore[0]).toBe("");
    expect(onClose).toHaveBeenCalled();
  });

  it("shows error helperText when error prop is provided", () => {
    const html = runRender({ error: "invalid_code" });
    expect(html).toContain("errors.invalid_code");
  });

  it("disables input and buttons when loading=true", () => {
    const html = runRender({ loading: true });
    const disabledCount = (html.match(/disabled=""/g) ?? []).length;
    expect(disabledCount).toBeGreaterThanOrEqual(2); // both buttons + input
  });
});
