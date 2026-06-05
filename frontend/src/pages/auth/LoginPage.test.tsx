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
let effectCleanups: Array<() => void> = [];

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
      if (callIdx >= 100) {
        callIdx = 0;
      }
      return [stateStore[idx], stateSetters[idx]];
    },
    useEffect: (fn: any) => {
      const cleanup = fn();
      if (typeof cleanup === "function") effectCleanups.push(cleanup);
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
    effectCleanups = [];
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
    return renderToString(React.createElement(LoginPage));
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
    vi.mocked(scheduleRefresh).mock.calls[0][0]?.();
    expect(mockNavigate).toHaveBeenCalledWith("/auth/login");
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
    const lastScheduleRefreshCall = vi.mocked(scheduleRefresh).mock.calls[vi.mocked(scheduleRefresh).mock.calls.length - 1];
    lastScheduleRefreshCall?.[0]?.();
    expect(mockNavigate).toHaveBeenCalledWith("/auth/login");
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
    expect(effectCleanups[0]).toBeDefined();
    effectCleanups[0]();
    expect(window.clearInterval).toHaveBeenCalledWith(123);
  });

  it("does not submit credentials or MFA while retry countdown is active", async () => {
    const loginSpy = vi.spyOn(api, "login").mockResolvedValue({ ok: true, mfa_required: false });
    stateStore[11] = 5;
    runRender();

    await capturedOnSubmitCredentials({ preventDefault: vi.fn() });
    expect(loginSpy).not.toHaveBeenCalled();

    stateStore[0] = "mfa";
    stateStore[3] = "mfa-token";
    runRender();
    const mfaSpy = vi.spyOn(api, "mfaChallenge").mockResolvedValue({ ok: true } as any);

    await capturedOnSubmitMFA({ preventDefault: vi.fn() });
    expect(mfaSpy).not.toHaveBeenCalled();
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
    capturedOnChangeCodesSaved({ target: { checked: false } }); // Hit the false branch
    expect(stateStore[7]).toBe(false);

    // Verify back button clicks resets state
    expect(capturedBackButtonClick).toBeDefined();
    capturedBackButtonClick();
    expect(stateStore[0]).toBe("credentials");
    expect(stateStore[9]).toBeNull();
  });

  it("handles recovery code (length 14) during MFA verification", async () => {
    stateStore[0] = "mfa";
    stateStore[3] = "mfa-token-abc";
    stateStore[8] = "xxxx-xxxx-xxxx"; // Hits the totpCode.length !== 14 branch
    runRender();

    const mfaSpy = vi.spyOn(api, "mfaChallenge").mockResolvedValue({ ok: true } as any);
    await capturedOnSubmitMFA({ preventDefault: vi.fn() });

    expect(mfaSpy).toHaveBeenCalledWith("mfa-token-abc", "xxxx-xxxx-xxxx");
  });

  it("handles ApiErrors without retryAfter properties safely", async () => {
    // Hits account_locked without retryAfter
    vi.spyOn(api, "login").mockRejectedValueOnce(new ApiError("account_locked", undefined, 423));
    runRender();
    await capturedOnSubmitCredentials({ preventDefault: vi.fn() });
    expect(stateStore[9]).toContain("errors.account_locked");

    // Hits rate_limit_exceeded without retryAfter on login
    vi.spyOn(api, "login").mockRejectedValueOnce(new ApiError("rate_limit_exceeded", undefined, 429));
    runRender();
    await capturedOnSubmitCredentials({ preventDefault: vi.fn() });
    expect(stateStore[9]).toContain("errors.rate_limit_exceeded");

    // Hits rate_limit_exceeded without retryAfter on MFA verification
    stateStore[0] = "mfa";
    stateStore[3] = "mfa-token-xyz";
    runRender();
    vi.spyOn(api, "mfaChallenge").mockRejectedValueOnce(new ApiError("rate_limit_exceeded", undefined, 429));
    await capturedOnSubmitMFA({ preventDefault: vi.fn() });
    expect(stateStore[9]).toContain("errors.invalid_credentials");
  });

  it("renders loading state for both credentials and MFA stages", async () => {
    // This hits the `loading ? "..."` branches on lines 191 and 220
    let resolveLogin: any;
    vi.spyOn(api, "login").mockImplementation(() => new Promise((res) => { resolveLogin = res; }));
    
    stateStore[0] = "credentials";
    runRender();
    capturedOnSubmitCredentials({ preventDefault: vi.fn() });
    
    let html = runRender(); // render while loading is true
    expect(html).toContain("...");
    
    resolveLogin({ ok: true, mfa_required: false });
    await Promise.resolve(); await Promise.resolve();

    let resolveMFA: any;
    vi.spyOn(api, "mfaChallenge").mockImplementation(() => new Promise((res) => { resolveMFA = res; }));
    
    stateStore[0] = "mfa";
    runRender();
    capturedOnSubmitMFA({ preventDefault: vi.fn() });

    html = runRender(); // render while loading is true
    expect(html).toContain("...");

    resolveMFA({ ok: true });
    await Promise.resolve(); await Promise.resolve();
  });

  it("renders MFA stage errors and rate limit alerts", () => {
    // Hits the MFA stage Alert branches (lines 174-176)
    stateStore[0] = "mfa";
    stateStore[9] = "some-error"; // error state
    let html = runRender();
    expect(html).toContain("some-error");

    stateStore[9] = null;
    stateStore[11] = 10; // retryAfter state
    html = runRender();
    expect(html).toContain("errors.rate_limit_exceeded_countdown");
  });

  it("handles logical edge cases for mfa setup conditions", async () => {
    // Covers line 50/52 && false branches and line 55 ?? [] branch
    vi.spyOn(api, "login").mockResolvedValue({
      ok: true,
      mfa_required: true, // true, but no token to fail line 50
    });
    stateStore[0] = "credentials";
    runRender();
    await capturedOnSubmitCredentials({ preventDefault: vi.fn() });
    expect(mockNavigate).toHaveBeenCalledWith("/dashboard");

    vi.spyOn(api, "login").mockResolvedValue({
      ok: true,
      mfa_required: true,
      mfa_token: "token",
      mfa_setup_url: "url", // true, but no secret to fail line 52
    });
    await capturedOnSubmitCredentials({ preventDefault: vi.fn() });
    expect(stateStore[0]).toBe("mfa");
    expect(stateStore[4]).toBe(""); // mfaSetupSecret is not set

    vi.spyOn(api, "login").mockResolvedValue({
      ok: true,
      mfa_required: true,
      mfa_token: "token",
      mfa_setup_url: "url",
      mfa_setup_secret: "secret",
      // missing mfa_recovery_codes to hit ?? []
    });
    await capturedOnSubmitCredentials({ preventDefault: vi.fn() });
    expect(stateStore[6]).toEqual([]);
  });

  it("evaluates MFA button disabled branches fully", () => {
    // Bypass the totpCode length short-circuit to evaluate the rest of the disabled conditions
    stateStore[0] = "mfa";
    stateStore[3] = "mfa-token-xyz";
    stateStore[8] = "123456"; // valid length bypasses length check

    // Case 1: retryAfter > 0 evaluates to true
    stateStore[11] = 5; 
    runRender();

    // Case 2: retryAfter = 0, isSetup = true, codesSaved = false evaluates to true
    stateStore[11] = 0;
    stateStore[5] = "otpauth://setup";
    stateStore[7] = false;
    runRender();

    // Case 3: retryAfter = 0, isSetup = true, codesSaved = true evaluates to false
    stateStore[7] = true;
    runRender();
  });
});
