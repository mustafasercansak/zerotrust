import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import LoginPage from "./LoginPage";
import { api, ApiError } from "@/lib/api";
import { scheduleRefresh } from "@/lib/tokenManager";
import { toast } from "sonner";
import { render, screen, fireEvent, waitFor, cleanup, act } from "@testing-library/react";
import { isWebAuthnSupported, performAssertion } from "@/lib/webauthn";

const mockNavigate = vi.fn();
const mockSearchParams = new URLSearchParams();
vi.mock("react-router-dom", () => ({
  Link: ({ children, to }: any) => React.createElement("a", { href: to }, children),
  useNavigate: () => mockNavigate,
  useSearchParams: () => [mockSearchParams],
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
vi.mock("sonner", () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}));
vi.mock("qrcode.react", () => ({
  QRCodeSVG: () => React.createElement("div", null, "QRCode"),
}));
vi.mock("@/lib/webauthn", () => ({
  isWebAuthnSupported: vi.fn().mockReturnValue(false),
  performAssertion: vi.fn(),
}));

describe("LoginPage component", () => {
  let mockLocation: { href: string };
  let setIntervalCallback: any;

  beforeEach(() => {
    mockNavigate.mockClear();
    vi.mocked(scheduleRefresh).mockClear();
    vi.mocked(toast.error).mockClear();
    vi.mocked(toast.success).mockClear();
    vi.mocked(isWebAuthnSupported).mockReturnValue(false);
    mockSearchParams.delete("redirect_to");

    mockLocation = { href: "" };
    const windowProxy = new Proxy(window, {
      get(target, prop) {
        if (prop === "location") {
          return mockLocation;
        }
        const val = Reflect.get(target, prop);
        if (typeof val === "function") {
          return val.bind(target);
        }
        return val;
      },
      set(target, prop, value) {
        if (prop === "location") {
          mockLocation = value;
          return true;
        }
        return Reflect.set(target, prop, value);
      }
    });
    vi.stubGlobal("window", windowProxy);

    setIntervalCallback = null;
    vi.spyOn(window, "setInterval").mockImplementation((fn: any) => {
      setIntervalCallback = fn;
      return 123 as any;
    });
    vi.spyOn(window, "clearInterval").mockImplementation(() => {});
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("redirect_to=https://evil.example falls back to the default dashboard route", async () => {
    mockSearchParams.set("redirect_to", "https://evil.example/fake-login");
    vi.spyOn(api, "login").mockResolvedValue({ ok: true, mfa_required: false });

    render(React.createElement(LoginPage));
    fireEvent.submit(document.querySelector("form")!);

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith("/dashboard");
    });
    expect(mockNavigate).not.toHaveBeenCalledWith("https://evil.example/fake-login");
    expect(mockLocation.href).toBe("");
  });

  it("redirect_to=//evil.example (protocol-relative) falls back to the default dashboard route", async () => {
    mockSearchParams.set("redirect_to", "//evil.example/fake-login");
    vi.spyOn(api, "login").mockResolvedValue({ ok: true, mfa_required: false });

    render(React.createElement(LoginPage));
    fireEvent.submit(document.querySelector("form")!);

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith("/dashboard");
    });
    expect(mockLocation.href).toBe("");
  });

  it("redirect_to with a same-origin /oauth2/ path still navigates to the backend consent flow", async () => {
    mockSearchParams.set("redirect_to", "/oauth2/authorize?client_id=demo&response_type=code");
    vi.spyOn(api, "login").mockResolvedValue({ ok: true, mfa_required: false });

    render(React.createElement(LoginPage));
    fireEvent.submit(document.querySelector("form")!);

    await waitFor(() => {
      expect(mockLocation.href).toBe("/oauth2/authorize?client_id=demo&response_type=code");
    });
    expect(mockNavigate).not.toHaveBeenCalledWith("/oauth2/authorize?client_id=demo&response_type=code");
  });

  it("redirect_to with another same-origin absolute path is honored via client-side navigation", async () => {
    mockSearchParams.set("redirect_to", "/dashboard/sessions");
    vi.spyOn(api, "login").mockResolvedValue({ ok: true, mfa_required: false });

    render(React.createElement(LoginPage));
    fireEvent.submit(document.querySelector("form")!);

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith("/dashboard/sessions");
    });
    expect(mockLocation.href).toBe("");
  });

  it("handles standard input typing", () => {
    render(React.createElement(LoginPage));

    const emailInput = screen.getByLabelText(/email/i) as HTMLInputElement;
    const passwordInput = screen.getByLabelText(/password/i) as HTMLInputElement;

    fireEvent.change(emailInput, { target: { value: "user@example.com" } });
    fireEvent.change(passwordInput, { target: { value: "password123" } });

    expect(emailInput.value).toBe("user@example.com");
    expect(passwordInput.value).toBe("password123");
  });

  it("handles successful login without MFA", async () => {
    const loginSpy = vi.spyOn(api, "login").mockResolvedValue({
      ok: true,
      mfa_required: false,
    });

    render(React.createElement(LoginPage));

    fireEvent.change(screen.getByLabelText(/email/i), { target: { value: "test@example.com" } });
    fireEvent.change(screen.getByLabelText(/password/i), { target: { value: "pass" } });
    fireEvent.submit(document.querySelector("form")!);

    await waitFor(() => {
      expect(loginSpy).toHaveBeenCalledWith("test@example.com", "pass");
      expect(scheduleRefresh).toHaveBeenCalled();
    });

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

    render(React.createElement(LoginPage));
    fireEvent.submit(document.querySelector("form")!);

    await waitFor(() => {
      expect(screen.getByText("mfaTitle")).toBeDefined();
    });

    expect(screen.getByText("mfaSetupRequiredDesc")).toBeDefined();
    expect(screen.getByText("secret-xyz")).toBeDefined();
    expect(screen.getByText("code1")).toBeDefined();
    expect(screen.getByText("code2")).toBeDefined();
  });

  it("handles login requiring MFA verification (already setup)", async () => {
    vi.spyOn(api, "login").mockResolvedValue({
      ok: true,
      mfa_required: true,
      mfa_token: "mfa-token-abc",
      totp_enabled: true,
    });

    render(React.createElement(LoginPage));
    fireEvent.submit(document.querySelector("form")!);

    await waitFor(() => {
      expect(screen.getByLabelText(/MFA Code/i)).toBeDefined();
    });

    const mfaCodeInput = screen.getByLabelText(/MFA Code/i);
    fireEvent.change(mfaCodeInput, { target: { value: "123456" } });

    const mfaSpy = vi.spyOn(api, "mfaChallenge").mockResolvedValue({ ok: true } as any);
    fireEvent.submit(document.querySelector("form")!);

    await waitFor(() => {
      expect(mfaSpy).toHaveBeenCalledWith("mfa-token-abc", "123456");
      expect(scheduleRefresh).toHaveBeenCalled();
    });

    vi.mocked(scheduleRefresh).mock.calls[0][0]?.();
    expect(mockNavigate).toHaveBeenCalledWith("/auth/login");
    expect(mockNavigate).toHaveBeenCalledWith("/dashboard");
  });

  it("handles locked account and rate limiting API errors", async () => {
    vi.spyOn(api, "login").mockRejectedValueOnce(
      new ApiError("account_locked", 180, 423)
    );

    render(React.createElement(LoginPage));
    fireEvent.submit(document.querySelector("form")!);

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(expect.stringContaining("errors.account_locked"));
    });

    // Rate Limit Error
    vi.spyOn(api, "login").mockRejectedValueOnce(
      new ApiError("rate_limit_exceeded", 30, 429)
    );
    fireEvent.submit(document.querySelector("form")!);

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "retryButton ({\"seconds\":30})" })).toBeDefined();
    });

    // Test countdown timer in useEffect
    act(() => {
      setIntervalCallback();
    });
    expect(screen.getByRole("button", { name: "retryButton ({\"seconds\":29})" })).toBeDefined();
  });

  it("does not submit credentials or MFA while retry countdown is active", async () => {
    vi.spyOn(api, "login").mockRejectedValueOnce(
      new ApiError("rate_limit_exceeded", 5, 429)
    );
    render(React.createElement(LoginPage));
    fireEvent.submit(document.querySelector("form")!);

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "retryButton ({\"seconds\":5})" })).toBeDefined();
    });

    const loginSpy = vi.spyOn(api, "login").mockResolvedValue({ ok: true, mfa_required: false });
    loginSpy.mockClear();
    fireEvent.submit(document.querySelector("form")!);
    expect(loginSpy).not.toHaveBeenCalled();
  });

  it("handles regular API and generic failures during credentials login", async () => {
    vi.spyOn(api, "login").mockRejectedValueOnce(new ApiError("invalid_credentials", undefined, 401));
    render(React.createElement(LoginPage));
    fireEvent.submit(document.querySelector("form")!);
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(expect.stringContaining("errors.invalid_credentials"));
    });

    vi.spyOn(api, "login").mockRejectedValueOnce(new Error("Net fail"));
    fireEvent.submit(document.querySelector("form")!);
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(expect.stringContaining("errors.internal_error"));
    });
  });

  it("handles MFA challenge failures including rate limiting", async () => {
    vi.spyOn(api, "login").mockResolvedValue({
      ok: true,
      mfa_required: true,
      mfa_token: "mfa-token-xyz",
      totp_enabled: true,
    });
    render(React.createElement(LoginPage));
    fireEvent.submit(document.querySelector("form")!);

    await waitFor(() => {
      expect(screen.getByLabelText(/MFA Code/i)).toBeDefined();
    });

    vi.spyOn(api, "mfaChallenge").mockRejectedValueOnce(
      new ApiError("rate_limit_exceeded", 10, 429)
    );
    fireEvent.change(screen.getByLabelText(/MFA Code/i), { target: { value: "123456" } });
    fireEvent.submit(document.querySelector("form")!);

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "retryButton ({\"seconds\":10})" })).toBeDefined();
    });

    // Other failure
    act(() => {
      for (let i = 0; i < 10; i++) setIntervalCallback();
    });
    vi.spyOn(api, "mfaChallenge").mockRejectedValueOnce(new Error("wrong code"));
    fireEvent.submit(document.querySelector("form")!);
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(expect.stringContaining("errors.invalid_credentials"));
    });
  });

  it("allows backing to credentials stage from MFA layout", async () => {
    vi.spyOn(api, "login").mockResolvedValue({
      ok: true,
      mfa_required: true,
      mfa_token: "mfa-token-123",
      mfa_setup_url: "otpauth://totp/...",
      mfa_setup_secret: "secret-xyz",
      mfa_recovery_codes: ["code1"],
    });

    render(React.createElement(LoginPage));
    fireEvent.submit(document.querySelector("form")!);

    await waitFor(() => {
      expect(screen.getByText("mfaTitle")).toBeDefined();
    });

    const checkbox = screen.getByRole("checkbox") as HTMLInputElement;
    expect(checkbox.checked).toBe(false);
    fireEvent.click(checkbox);
    expect(checkbox.checked).toBe(true);

    fireEvent.click(screen.getByRole("button", { name: "backToLogin" }));

    await waitFor(() => {
      expect(screen.queryByText("mfaTitle")).toBeNull();
      expect(screen.getByRole("button", { name: "loginButton" })).toBeDefined();
    });
  });

  it("handles recovery code (length 14) during MFA verification", async () => {
    vi.spyOn(api, "login").mockResolvedValue({
      ok: true,
      mfa_required: true,
      mfa_token: "mfa-token-abc",
      totp_enabled: true,
    });

    render(React.createElement(LoginPage));
    fireEvent.submit(document.querySelector("form")!);

    await waitFor(() => {
      expect(screen.getByLabelText(/MFA Code/i)).toBeDefined();
    });

    const mfaCodeInput = screen.getByLabelText(/MFA Code/i);
    fireEvent.change(mfaCodeInput, { target: { value: "xxxx-xxxx-xxxx" } });

    const mfaSpy = vi.spyOn(api, "mfaChallenge").mockResolvedValue({ ok: true } as any);
    fireEvent.submit(document.querySelector("form")!);

    await waitFor(() => {
      expect(mfaSpy).toHaveBeenCalledWith("mfa-token-abc", "xxxx-xxxx-xxxx");
    });
  });

  it("handles ApiErrors without retryAfter properties safely", async () => {
    vi.spyOn(api, "login").mockRejectedValueOnce(new ApiError("account_locked", undefined, 423));
    render(React.createElement(LoginPage));
    fireEvent.submit(document.querySelector("form")!);
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(expect.stringContaining("errors.account_locked"));
    });

    vi.spyOn(api, "login").mockRejectedValueOnce(new ApiError("rate_limit_exceeded", undefined, 429));
    fireEvent.submit(document.querySelector("form")!);
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(expect.stringContaining("errors.rate_limit_exceeded"));
    });
  });

  it("renders loading state for both credentials and MFA stages", async () => {
    let resolveLogin: any;
    vi.spyOn(api, "login").mockImplementation(() => new Promise((res) => { resolveLogin = res; }));

    render(React.createElement(LoginPage));
    fireEvent.submit(document.querySelector("form")!);

    expect(screen.getByRole("button", { name: "..." })).toBeDefined();

    resolveLogin({ ok: true, mfa_required: false });
    await act(async () => {
      await Promise.resolve();
    });
  });

  it("renders the rate-limit countdown on the MFA submit button", async () => {
    vi.spyOn(api, "login").mockResolvedValue({
      ok: true,
      mfa_required: true,
      mfa_token: "mfa-token-abc",
      totp_enabled: true,
    });
    render(React.createElement(LoginPage));
    fireEvent.submit(document.querySelector("form")!);

    await waitFor(() => {
      expect(screen.getByLabelText(/MFA Code/i)).toBeDefined();
    });

    vi.spyOn(api, "mfaChallenge").mockRejectedValueOnce(
      new ApiError("rate_limit_exceeded", 10, 429)
    );
    fireEvent.change(screen.getByLabelText(/MFA Code/i), { target: { value: "123456" } });
    fireEvent.submit(document.querySelector("form")!);

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "retryButton ({\"seconds\":10})" })).toBeDefined();
    });
  });

  it("handles logical edge cases for mfa setup conditions", async () => {
    vi.spyOn(api, "login").mockResolvedValue({
      ok: true,
      mfa_required: true,
    });
    render(React.createElement(LoginPage));
    fireEvent.submit(document.querySelector("form")!);
    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith("/dashboard");
    });
  });

  it("renders Sign in with passkey button only when WebAuthn is supported", () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(false);
    const { rerender } = render(React.createElement(LoginPage));
    expect(screen.queryByRole("button", { name: "signInWithPasskey" })).toBeNull();

    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    rerender(React.createElement(LoginPage));
    expect(screen.getByRole("button", { name: "signInWithPasskey" })).toBeDefined();
  });

  it("handlePasswordlessLogin: navigates to dashboard on success", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    const fakeOptions = { publicKey: { timeout: 60000 }, ceremony_id: "c-1" };
    const fakeAssertion = { id: "assert-1" };
    vi.spyOn(api, "webauthnPasswordlessBegin").mockResolvedValue(fakeOptions as any);
    vi.mocked(performAssertion).mockResolvedValue(fakeAssertion as any);
    vi.spyOn(api, "webauthnPasswordlessFinish").mockResolvedValue({ ok: true });

    render(React.createElement(LoginPage));
    fireEvent.click(screen.getByRole("button", { name: "signInWithPasskey" }));

    await waitFor(() => {
      expect(api.webauthnPasswordlessBegin).toHaveBeenCalled();
      expect(performAssertion).toHaveBeenCalledWith(fakeOptions);
      expect(api.webauthnPasswordlessFinish).toHaveBeenCalledWith("c-1", fakeAssertion);
      expect(scheduleRefresh).toHaveBeenCalled();
      expect(mockNavigate).toHaveBeenCalledWith("/dashboard");
    });
  });

  it("handlePasswordlessLogin: caps ceremony timeout to 60 s", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    const fakeOptions = { publicKey: { timeout: 300000 }, ceremony_id: "c-2" };
    vi.spyOn(api, "webauthnPasswordlessBegin").mockResolvedValue(fakeOptions as any);
    vi.mocked(performAssertion).mockResolvedValue({} as any);
    vi.spyOn(api, "webauthnPasswordlessFinish").mockResolvedValue({ ok: true });

    render(React.createElement(LoginPage));
    fireEvent.click(screen.getByRole("button", { name: "signInWithPasskey" }));

    await waitFor(() => {
      expect(fakeOptions.publicKey.timeout).toBe(60000);
    });
  });

  it("handlePasswordlessLogin: shows passkey_unavailable toast on NotAllowedError", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    vi.spyOn(api, "webauthnPasswordlessBegin").mockResolvedValue({ publicKey: { timeout: 60000 }, ceremony_id: "c-3" } as any);
    const notAllowed = Object.assign(new Error("Not allowed"), { name: "NotAllowedError" });
    vi.mocked(performAssertion).mockRejectedValue(notAllowed);

    render(React.createElement(LoginPage));
    fireEvent.click(screen.getByRole("button", { name: "signInWithPasskey" }));

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(expect.stringContaining("errors.passkey_unavailable"));
    });
  });

  it("handlePasswordlessLogin: sets retryAfter on rate_limit_exceeded", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    vi.spyOn(api, "webauthnPasswordlessBegin").mockRejectedValue(
      new ApiError("rate_limit_exceeded", 45, 429)
    );

    render(React.createElement(LoginPage));
    fireEvent.click(screen.getByRole("button", { name: "signInWithPasskey" }));

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "retryButton ({\"seconds\":45})" })).toBeDefined();
      expect(toast.error).toHaveBeenCalledWith(expect.stringContaining("errors.rate_limit_exceeded_countdown"));
    });
  });

  it("handlePasswordlessLogin: shows generic error toast on other failures", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    vi.spyOn(api, "webauthnPasswordlessBegin").mockRejectedValue(new Error("network error"));

    render(React.createElement(LoginPage));
    fireEvent.click(screen.getByRole("button", { name: "signInWithPasskey" }));

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(expect.stringContaining("errors.webauthn_failed"));
    });
  });

  it("renders Use a passkey button on MFA screen when webauthnEnabled is true", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    vi.spyOn(api, "login").mockResolvedValue({
      ok: true,
      mfa_required: true,
      mfa_token: "mfa-token-abc",
      webauthn_enabled: true,
    });

    render(React.createElement(LoginPage));
    fireEvent.submit(document.querySelector("form")!);

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "usePasskey" })).toBeDefined();
    });
  });

  it("handlePasskeyLogin: navigates to dashboard on success during MFA stage", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    vi.spyOn(api, "login").mockResolvedValue({
      ok: true,
      mfa_required: true,
      mfa_token: "mfa-token-abc",
      webauthn_enabled: true,
    });

    const fakeOptions = { publicKey: { challenge: "xyz" } };
    const fakeAssertion = { id: "assert-mfa" };
    vi.spyOn(api, "webauthnLoginBegin").mockResolvedValue(fakeOptions as any);
    vi.mocked(performAssertion).mockResolvedValue(fakeAssertion as any);
    vi.spyOn(api, "webauthnLoginFinish").mockResolvedValue({ ok: true } as any);

    render(React.createElement(LoginPage));
    fireEvent.submit(document.querySelector("form")!);

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "usePasskey" })).toBeDefined();
    });

    fireEvent.click(screen.getByRole("button", { name: "usePasskey" }));

    await waitFor(() => {
      expect(api.webauthnLoginBegin).toHaveBeenCalledWith("mfa-token-abc");
      expect(performAssertion).toHaveBeenCalledWith(fakeOptions);
      expect(api.webauthnLoginFinish).toHaveBeenCalledWith("mfa-token-abc", fakeAssertion);
      expect(scheduleRefresh).toHaveBeenCalled();
      expect(mockNavigate).toHaveBeenCalledWith("/dashboard");
    });
  });

  it("handlePasskeyLogin: sets retryAfter on rate_limit_exceeded during MFA", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    vi.spyOn(api, "login").mockResolvedValue({
      ok: true,
      mfa_required: true,
      mfa_token: "mfa-token-abc",
      webauthn_enabled: true,
      totp_enabled: true,
    });
    vi.spyOn(api, "webauthnLoginBegin").mockRejectedValue(
      new ApiError("rate_limit_exceeded", 20, 429)
    );

    render(React.createElement(LoginPage));
    fireEvent.submit(document.querySelector("form")!);

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "usePasskey" })).toBeDefined();
    });

    fireEvent.click(screen.getByRole("button", { name: "usePasskey" }));

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "retryButton ({\"seconds\":20})" })).toBeDefined();
      expect(toast.error).toHaveBeenCalledWith(expect.stringContaining("errors.rate_limit_exceeded_countdown"));
    });
  });

  it("handlePasskeyLogin: shows generic error toast on failure during MFA", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    vi.spyOn(api, "login").mockResolvedValue({
      ok: true,
      mfa_required: true,
      mfa_token: "mfa-token-abc",
      webauthn_enabled: true,
    });
    vi.spyOn(api, "webauthnLoginBegin").mockRejectedValue(new Error("hw error"));

    render(React.createElement(LoginPage));
    fireEvent.submit(document.querySelector("form")!);

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "usePasskey" })).toBeDefined();
    });

    fireEvent.click(screen.getByRole("button", { name: "usePasskey" }));

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(expect.stringContaining("errors.webauthn_failed"));
    });
  });
});
