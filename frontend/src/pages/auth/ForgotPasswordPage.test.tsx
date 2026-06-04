import { beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import ForgotPasswordPage from "./ForgotPasswordPage";
import { api } from "@/lib/api";
import { renderToString } from "react-dom/server";

// Mock react-router-dom
vi.mock("react-router-dom", () => ({
  Link: ({ children, to }: any) => React.createElement("a", { href: to }, children),
}));

// Mock react-i18next
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

// Mock api
vi.spyOn(api, "forgotPassword").mockResolvedValue({ ok: true } as any);

// Mock Box to capture onSubmit and simulate onChange
let capturedOnSubmit: any = null;
let capturedOnChange: any = null;

vi.mock("@mui/material/Box", () => ({
  default: (props: any) => {
    if (props.onSubmit) {
      capturedOnSubmit = props.onSubmit;
    }
    return React.createElement("div", null, props.children);
  },
}));

vi.mock("@mui/material/TextField", () => ({
  default: (props: any) => {
    if (props.onChange) {
      capturedOnChange = props.onChange;
    }
    return React.createElement("input", { value: props.value, onChange: props.onChange });
  },
}));

// We intercept useState hook using vitest to simulate submitting and loading states
let mockEmail = "";
let mockSubmitted = false;
let mockLoading = false;
const setMockEmail = vi.fn((val) => { mockEmail = val; });
const setMockSubmitted = vi.fn((val) => { mockSubmitted = val; });
const setMockLoading = vi.fn((val) => { mockLoading = val; });

vi.mock("react", async (importOriginal) => {
  const original = await importOriginal<typeof import("react")>();
  let stateIndex = 0;
  return {
    ...original,
    useState: (init: any) => {
      const idx = stateIndex;
      stateIndex = (stateIndex + 1) % 3;
      if (idx === 0) return [mockEmail, setMockEmail];
      if (idx === 1) return [mockSubmitted, setMockSubmitted];
      return [mockLoading, setMockLoading];
    },
  };
});

describe("ForgotPasswordPage component", () => {
  beforeEach(() => {
    mockEmail = "";
    mockSubmitted = false;
    mockLoading = false;
    vi.clearAllMocks();
  });

  it("handles input change successfully", () => {
    renderToString(React.createElement(ForgotPasswordPage));
    expect(capturedOnChange).toBeDefined();
    capturedOnChange({ target: { value: "test@example.com" } });
    expect(setMockEmail).toHaveBeenCalledWith("test@example.com");
  });

  it("handles form submission successfully", async () => {
    mockEmail = "test@example.com";
    renderToString(React.createElement(ForgotPasswordPage));

    expect(capturedOnSubmit).toBeDefined();
    const preventDefault = vi.fn();
    
    const promise = capturedOnSubmit({ preventDefault });
    expect(preventDefault).toHaveBeenCalled();
    expect(setMockLoading).toHaveBeenCalledWith(true);
    
    await promise;
    expect(api.forgotPassword).toHaveBeenCalledWith("test@example.com");
    expect(setMockLoading).toHaveBeenCalledWith(false);
    expect(setMockSubmitted).toHaveBeenCalledWith(true);
  });

  it("renders success state when submitted is true", () => {
    mockSubmitted = true;
    const html = renderToString(React.createElement(ForgotPasswordPage));
    expect(html).toContain("forgotSent");
    expect(html).toContain("backToLogin");
  });

  it("renders loading state correctly", () => {
    mockLoading = true;
    const html = renderToString(React.createElement(ForgotPasswordPage));
    expect(html).toContain("...");
  });
});
