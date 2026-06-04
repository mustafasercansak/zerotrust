import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import LoginPage from "./LoginPage";
import { api, ApiError } from "@/lib/api";
import { scheduleRefresh } from "@/lib/tokenManager";
import { renderToString } from "react-dom/server";

// Mock react-router-dom
const mockNavigate = vi.fn();
vi.mock("react-router-dom", () => ({
  Link: ({ children, to }: any) => React.createElement("a", { href: to }, children),
  useNavigate: () => mockNavigate,
}));

// Mock react-i18next
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, options?: any) => {
      if (options) {
        return `${key} (${JSON.stringify(options)})`;
      }
      return key;
    },
  }),
}));

// Mock tokenManager
vi.mock("@/lib/tokenManager", () => ({
  scheduleRefresh: vi.fn(),
}));

// Mock sonner and qrcode
vi.mock("qrcode.react", () => ({
  QRCodeSVG: () => React.createElement("div", null, "QRCode"),
}));

// State Mocking System
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
      if (callIdx >= 12) {
        callIdx = 0;
      }
      return [stateStore[idx], stateSetters[idx]];
    },
    useEffect: (fn: any) => {
      fn();
    },
  };
});

// Mock Material UI inputs
let capturedOnSubmitCredentials: any = null;
let capturedOnSubmitMFA: any = null;
let capturedOnChangeEmail: any = null;
let capturedOnChangePassword: any = null;
let capturedOnChangeTotpCode: any = null;
let capturedOnChangeCodesSaved: any = null;
let capturedBackButtonClick: any = null;

vi.mock("@mui/material/Box", () => ({
  default: (props: any) => {
    if (props.onSubmit) {
      if (stateStore[0] === "credentials") {
        capturedOnSubmitCredentials = props.onSubmit;
      } else {
        capturedOnSubmitMFA = props.onSubmit;
      }
    }
    return React.createElement("div", null, props.children);
  },
}));

vi.mock("@mui/material/TextField", () => ({
  default: (props: any) => {
    if (props.label === "email") {
      capturedOnChangeEmail = props.onChange;
    } else if (props.label === "password") {
      capturedOnChangePassword = props.onChange;
    } else if (props.label === "mfaCode" || props.label === "MFA Code / Recovery Code") {
      capturedOnChangeTotpCode = props.onChange;
    }
    return React.createElement("input", { value: props.value, onChange: props.onChange });
  },
}));

vi.mock("@mui/material/Checkbox", () => ({
  default: (props: any) => {
    capturedOnChangeCodesSaved = props.onChange;
    return React.createElement("input", { type: "checkbox", checked: props.checked, onChange: props.onChange });
  },
}));

vi.mock("@mui/material/Button", () => ({
  default: (props: any) => {
    if (props.color === "inherit") {
      capturedBackButtonClick = props.onClick;
    }
    return React.createElement("button", { type: props.type, onClick: props.onClick }, props.children);
  },
}));

describe("LoginPage component", () => {
  beforeEach(() => {
    stateStore = {};
    stateSetters = {};
    callIdx = 0;
    capturedOnSubmitCredentials = null;
    capturedOnSubmitMFA = null;
    capturedOnChangeEmail = null;
    capturedOnChangePassword = null;
    capturedOnChangeTotpCode = null;
    capturedOnChangeCodesSaved = null;
    capturedBackButtonClick = null;
    vi.clearAllMocks();
    vi.useFakeTimers();

    vi.stubGlobal("window", {
      setInterval: vi.fn((fn: any, delay: number) => {
        fn();
        return 123;
      }),
      clearInterval: vi.fn(),
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  const runRender = () => {
    callIdx = 0;
    renderToString(React.createElement(LoginPage));
  };

  it("handles standard input typing", () => {
    runRender();

    capturedOnChangeEmail({ target: { value: "user@example.com" } });
    capturedOnChangePassword({ target: { value: "password123" } });

    expect(stateStore[1]).toBe("user@example.com");
    expect(stateStore[2]).toBe("password123");
  });

  it("handles successful login without MFA", async () => {
    const loginSpy = vi.spyOn(api, "login").mockResolvedValue({
      ok: true,
      mfa_required: false,
    });

    stateStore[1] = "test@example.com";
    stateStore[2] = "pass";
    runRender();

    const preventDefault = vi.fn();
    await capturedOnSubmitCredentials({ preventDefault });

    expect(loginSpy).toHaveBeenCalledWith("test@example.com", "pass");
    expect(scheduleRefresh).toHaveBeenCalled();
    expect(mockNavigate).toHaveBeenCalledWith("/dashboard");
  });

  it("handles login requiring MFA setup", async () => {
    vi.spyOn(api, "login").mockResolvedValue({
      ok: true,
      mfa_required: true,
      mfa_token: "mfa-token-123",
      mfa_setup_url: "otpauth://totp/...",
      mfa_setup_secret: "secret-xyz",
      mfa_recovery_codes: ["code1", "code2"],
    });

    runRender();

    await capturedOnSubmitCredentials({ preventDefault: vi.fn() });

    expect(stateStore[0]).toBe("mfa"); // stage is updated to mfa
    expect(stateStore[3]).toBe("mfa-token-123");
    expect(stateStore[5]).toBe("otpauth://totp/...");
    expect(stateStore[4]).toBe("secret-xyz");
    expect(stateStore[6]).toEqual(["code1", "code2"]);
  });

  it("handles login requiring MFA verification (already setup)", async () => {
    vi.spyOn(api, "login").mockResolvedValue({
      ok: true,
      mfa_required: true,
      mfa_token: "mfa-token-abc",
    });

    runRender();
    await capturedOnSubmitCredentials({ preventDefault: vi.fn() });

    expect(stateStore[0]).toBe("mfa");
    expect(stateStore[5]).toBe(""); // no setup url

    // Pre-populate values for MFA stage layout render
    stateStore[8] = "123456";
    runRender();

    expect(capturedOnChangeTotpCode).toBeDefined();
    capturedOnChangeTotpCode({ target: { value: "123456" } });

    const mfaSpy = vi.spyOn(api, "mfaChallenge").mockResolvedValue({ ok: true } as any);
    await capturedOnSubmitMFA({ preventDefault: vi.fn() });

    expect(mfaSpy).toHaveBeenCalledWith("mfa-token-abc", "123456");
    expect(mockNavigate).toHaveBeenCalledWith("/dashboard");
  });

  it("handles locked account and rate limiting API errors", async () => {
    vi.spyOn(api, "login").mockRejectedValueOnce(
      new ApiError("account_locked", 180, 423)
    );

    runRender();
    await capturedOnSubmitCredentials({ preventDefault: vi.fn() });
    expect(stateStore[9]).toContain("errors.account_locked");

    // Rate Limit Error
    vi.spyOn(api, "login").mockRejectedValueOnce(
      new ApiError("rate_limit_exceeded", 30, 429)
    );
    await capturedOnSubmitCredentials({ preventDefault: vi.fn() });
    expect(stateStore[11]).toBe(30); // retryAfter set to 30

    // Test countdown timer in useEffect
    runRender();
    expect(stateStore[11]).toBe(29);
  });

  it("handles regular API and generic failures during credentials login", async () => {
    vi.spyOn(api, "login").mockRejectedValueOnce(new ApiError("invalid_credentials", undefined, 401));
    runRender();
    await capturedOnSubmitCredentials({ preventDefault: vi.fn() });
    expect(stateStore[9]).toContain("errors.invalid_credentials");

    vi.spyOn(api, "login").mockRejectedValueOnce(new Error("Net fail"));
    await capturedOnSubmitCredentials({ preventDefault: vi.fn() });
    expect(stateStore[9]).toContain("errors.internal_error");
  });

  it("handles MFA challenge failures including rate limiting", async () => {
    // Navigate to MFA stage
    stateStore[0] = "mfa";
    stateStore[3] = "mfa-token-xyz";
    runRender();

    // Rate limit
    vi.spyOn(api, "mfaChallenge").mockRejectedValueOnce(
      new ApiError("rate_limit_exceeded", 10, 429)
    );
    await capturedOnSubmitMFA({ preventDefault: vi.fn() });
    expect(stateStore[11]).toBe(10);

    // Other failure
    vi.spyOn(api, "mfaChallenge").mockRejectedValueOnce(new Error("wrong code"));
    await capturedOnSubmitMFA({ preventDefault: vi.fn() });
    expect(stateStore[9]).toContain("errors.invalid_credentials");
  });

  it("allows backing to credentials stage from MFA layout", () => {
    stateStore[0] = "mfa";
    stateStore[5] = "otpauth://totp/...";
    stateStore[6] = ["code1"]; // recovery codes defined
    runRender();

    // Verify recovery codes saving checkbox
    expect(capturedOnChangeCodesSaved).toBeDefined();
    capturedOnChangeCodesSaved({ target: { checked: true } });
    expect(stateStore[7]).toBe(true);

    // Verify back button clicks resets state
    expect(capturedBackButtonClick).toBeDefined();
    capturedBackButtonClick();
    expect(stateStore[0]).toBe("credentials");
    expect(stateStore[9]).toBeNull();
  });
});
