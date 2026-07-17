import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import ResetPasswordPage from "./ResetPasswordPage";
import { api, ApiError } from "@/lib/api";
import { renderToString } from "react-dom/server";

// Mock react-router-dom
const mockNavigate = vi.fn();
let mockToken = "token123";

vi.mock("react-router-dom", () => ({
  Link: ({ children, to }: any) => React.createElement("a", { href: to }, children),
  useNavigate: () => mockNavigate,
  useSearchParams: () => [
    {
      get: (key: string) => {
        if (key === "token") return mockToken;
        return null;
      },
    },
  ],
}));

// Mock react-i18next
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, options?: any) => {
      if (options && options.defaultValue) {
        return `${key} (${options.defaultValue})`;
      }
      return key;
    },
  }),
}));

// Mock api
vi.spyOn(api, "resetPassword").mockResolvedValue({ ok: true } as any);

// Mock Box, TextField, Alert to capture events
let capturedOnSubmit: any = null;
let capturedOnChangePassword: any = null;
let capturedOnChangeConfirm: any = null;

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
    if (props.label === "newPassword") {
      capturedOnChangePassword = props.onChange;
    } else if (props.label === "confirmPassword") {
      capturedOnChangeConfirm = props.onChange;
    }
    return React.createElement("input", { value: props.value, onChange: props.onChange });
  },
}));

// We intercept useState hook using vitest to simulate submitting and loading states
let mockPassword = "";
let mockConfirm = "";
let mockError: string | null = null;
let mockLoading = false;
let mockDone = false;

const setMockPassword = vi.fn((val) => { mockPassword = val; });
const setMockConfirm = vi.fn((val) => { mockConfirm = val; });
const setMockError = vi.fn((val) => { mockError = val; });
const setMockLoading = vi.fn((val) => { mockLoading = val; });
const setMockDone = vi.fn((val) => { mockDone = val; });

vi.mock("react", async (importOriginal) => {
  const original = await importOriginal<typeof import("react")>();
  let stateIndex = 0;
  return {
    ...original,
    useState: (init: any) => {
      const idx = stateIndex;
      stateIndex = (stateIndex + 1) % 5;
      if (idx === 0) return [mockPassword, setMockPassword];
      if (idx === 1) return [mockConfirm, setMockConfirm];
      if (idx === 2) return [mockError, setMockError];
      if (idx === 3) return [mockLoading, setMockLoading];
      return [mockDone, setMockDone];
    },
  };
});

describe("ResetPasswordPage component", () => {
  beforeEach(() => {
    mockPassword = "";
    mockConfirm = "";
    mockError = null;
    mockLoading = false;
    mockDone = false;
    mockToken = "token123";
    vi.clearAllMocks();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("handles input changes successfully", () => {
    renderToString(React.createElement(ResetPasswordPage));
    capturedOnChangePassword({ target: { value: "pass123" } });
    capturedOnChangeConfirm({ target: { value: "pass123-diff" } });
    expect(setMockPassword).toHaveBeenCalledWith("pass123");
    expect(setMockConfirm).toHaveBeenCalledWith("pass123-diff");
  });

  it("handles password mismatch validation error", async () => {
    mockPassword = "pass123";
    mockConfirm = "pass123-diff";
    renderToString(React.createElement(ResetPasswordPage));

    const preventDefault = vi.fn();
    await capturedOnSubmit({ preventDefault });
    expect(setMockError).toHaveBeenCalledWith("errors.passwords_mismatch");
  });

  it("handles successful submission when passwords match", async () => {
    mockPassword = "pass123";
    mockConfirm = "pass123";
    renderToString(React.createElement(ResetPasswordPage));

    const preventDefault = vi.fn();
    const promise = capturedOnSubmit({ preventDefault });
    expect(setMockLoading).toHaveBeenCalledWith(true);

    await promise;
    expect(api.resetPassword).toHaveBeenCalledWith("token123", "pass123");
    expect(setMockDone).toHaveBeenCalledWith(true);
    expect(setMockLoading).toHaveBeenCalledWith(false);

    vi.advanceTimersByTime(2000);
    expect(mockNavigate).toHaveBeenCalledWith("/auth/login");
  });

  it("handles API errors on submission failure", async () => {
    vi.spyOn(api, "resetPassword").mockRejectedValueOnce(
      new ApiError("invalid_token", undefined, 400)
    );

    mockPassword = "pass";
    mockConfirm = "pass";
    renderToString(React.createElement(ResetPasswordPage));
    
    await capturedOnSubmit({ preventDefault: vi.fn() });
    expect(setMockError).toHaveBeenCalledWith("errors.invalid_token (errors.invalid_token)");
  });

  it("handles non-API generic errors on submission failure", async () => {
    vi.spyOn(api, "resetPassword").mockRejectedValueOnce(new Error("Network fail"));

    mockPassword = "pass";
    mockConfirm = "pass";
    renderToString(React.createElement(ResetPasswordPage));
    
    await capturedOnSubmit({ preventDefault: vi.fn() });
    expect(setMockError).toHaveBeenCalledWith("errors.internal_error");
  });

  it("renders validation error if token is missing in URL search params", () => {
    mockToken = null as any;
    const html = renderToString(React.createElement(ResetPasswordPage));
    expect(html).toContain("errors.invalid_token");
  });

  it("renders success state if done is true", () => {
    mockDone = true;
    const html = renderToString(React.createElement(ResetPasswordPage));
    expect(html).toContain("resetDone");
  });

  it("renders loading state correctly", () => {
    mockLoading = true;
    const html = renderToString(React.createElement(ResetPasswordPage));
    expect(html).toContain("...");
  });
});
