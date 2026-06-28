import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { render, screen, fireEvent, cleanup, waitFor } from "@testing-library/react";
import MfaPage from "./MfaPage";
import { api } from "@/lib/api";
import { toast } from "sonner";

vi.mock("sonner", () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

vi.mock("qrcode.react", () => ({
  QRCodeSVG: () => React.createElement("div", null, "QRCode"),
}));

vi.mock("./PasskeysSection", () => ({
  default: () => React.createElement("div", { "data-testid": "passkeys-section" }, "PasskeysSection"),
}));

describe("MfaPage page component", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("renders loader screen then loading completed", async () => {
    const statusSpy = vi.spyOn(api, "mfaStatus").mockResolvedValue({
      enabled: true,
      supported: true,
    });

    render(<MfaPage />);

    expect(screen.getByText("loading")).toBeDefined();

    await waitFor(() => {
      expect(screen.getByText("statusEnabled")).toBeDefined();
    });
    expect(statusSpy).toHaveBeenCalled();
  });

  it("handles the complete setup and verify MFA flow", async () => {
    // 1. Initial status is disabled
    const statusSpy = vi.spyOn(api, "mfaStatus").mockResolvedValue({
      enabled: false,
      supported: true,
    });

    // Mock setup API response
    const setupSpy = vi.spyOn(api, "mfaSetup").mockResolvedValue({
      otp_auth_url: "otpauth://totp/Test",
      secret: "SECRET123",
      recovery_codes: ["code1", "code2"],
    });

    // Mock verify API response
    const verifySpy = vi.spyOn(api, "mfaVerify").mockResolvedValue({} as any);

    render(<MfaPage />);

    await waitFor(() => {
      expect(screen.getByText("statusDisabled")).toBeDefined();
    });

    const setupBtn = screen.getByTestId("mfa-enable-button");
    fireEvent.click(setupBtn);

    expect(setupSpy).toHaveBeenCalled();

    await waitFor(() => {
      expect(screen.getByText("Two-Factor Authentication Setup")).toBeDefined();
    });

    // Check checkbox indicating codes are saved
    const checkbox = screen.getByRole("checkbox");
    fireEvent.click(checkbox);

    // Type the verification code
    const codeInput = screen.getByPlaceholderText("totpPlaceholder");
    fireEvent.change(codeInput, { target: { value: "123456" } });

    // Submit verification
    const submitBtn = screen.getByRole("button", { name: "Continue" });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(verifySpy).toHaveBeenCalledWith("123456");
    });
  });

  it("handles disabling MFA when it is enabled", async () => {
    vi.spyOn(api, "mfaStatus").mockResolvedValue({
      enabled: true,
      supported: true,
    });

    const disableSpy = vi.spyOn(api, "mfaDisable").mockResolvedValue({} as any);

    render(<MfaPage />);

    await waitFor(() => {
      expect(screen.getByText("statusEnabled")).toBeDefined();
    });

    const codeInput = screen.getByPlaceholderText("totpPlaceholder");
    fireEvent.change(codeInput, { target: { value: "654321" } });

    const disableBtn = screen.getByTestId("mfa-disable-button");
    fireEvent.click(disableBtn);

    await waitFor(() => {
      expect(disableSpy).toHaveBeenCalledWith("654321");
    });
  });

  it("handles MFA status unsupported and API error on load", async () => {
    vi.spyOn(api, "mfaStatus").mockResolvedValue({ enabled: false, supported: false });
    
    const { unmount } = render(<MfaPage />);
    await waitFor(() => {
      expect(screen.getByText("unsupportedDesc")).toBeDefined();
    });
    unmount();
    cleanup();

    vi.spyOn(api, "mfaStatus").mockRejectedValue(new Error("API error"));
    render(<MfaPage />);
    await waitFor(() => {
      expect(screen.getByText("unsupportedDesc")).toBeDefined();
    });
  });

  it("handles error during MFA setup", async () => {
    vi.spyOn(api, "mfaStatus").mockResolvedValue({ enabled: false, supported: true });
    vi.spyOn(api, "mfaSetup").mockRejectedValue(new Error("Setup error"));

    render(<MfaPage />);
    await waitFor(() => {
      expect(screen.getByText("statusDisabled")).toBeDefined();
    });

    const setupBtn = screen.getByTestId("mfa-enable-button");
    fireEvent.click(setupBtn);

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("errors.internal_error");
    });
  });

  it("handles error during MFA verification", async () => {
    vi.spyOn(api, "mfaStatus").mockResolvedValue({ enabled: false, supported: true });
    vi.spyOn(api, "mfaSetup").mockResolvedValue({
      otp_auth_url: "otpauth://totp/Test",
      secret: "SECRET123",
      recovery_codes: ["code1"],
    });
    vi.spyOn(api, "mfaVerify").mockRejectedValue(new Error("Verify error"));

    render(<MfaPage />);
    await waitFor(() => {
      expect(screen.getByText("statusDisabled")).toBeDefined();
    });

    const setupBtn = screen.getByTestId("mfa-enable-button");
    fireEvent.click(setupBtn);

    await waitFor(() => {
      expect(screen.getByText("Two-Factor Authentication Setup")).toBeDefined();
    });

    const checkbox = screen.getByRole("checkbox");
    fireEvent.click(checkbox);

    const codeInput = screen.getByPlaceholderText("totpPlaceholder");
    fireEvent.change(codeInput, { target: { value: "000000" } });

    const submitBtn = screen.getByRole("button", { name: "Continue" });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("errors.invalid_code");
    });
  });

  it("handles error during MFA disabling", async () => {
    vi.spyOn(api, "mfaStatus").mockResolvedValue({ enabled: true, supported: true });
    vi.spyOn(api, "mfaDisable").mockRejectedValue(new Error("Disable error"));

    render(<MfaPage />);
    await waitFor(() => {
      expect(screen.getByText("statusEnabled")).toBeDefined();
    });

    const codeInput = screen.getByPlaceholderText("totpPlaceholder");
    fireEvent.change(codeInput, { target: { value: "000000" } });

    const disableBtn = screen.getByTestId("mfa-disable-button");
    fireEvent.click(disableBtn);

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("errors.invalid_code");
    });
  });

  it("renders pending setup submitting state", async () => {
    vi.spyOn(api, "mfaStatus").mockResolvedValue({ enabled: false, supported: true });
    vi.spyOn(api, "mfaSetup").mockResolvedValue({
      otp_auth_url: "otpauth://totp/Test",
      secret: "SECRET123",
      recovery_codes: ["code1", "code2"],
    });
    vi.spyOn(api, "mfaVerify").mockReturnValue(new Promise(() => {}));

    render(<MfaPage />);
    await waitFor(() => {
      expect(screen.getByText("statusDisabled")).toBeDefined();
    });

    const setupBtn = screen.getByTestId("mfa-enable-button");
    fireEvent.click(setupBtn);

    await waitFor(() => {
      expect(screen.getByText("Two-Factor Authentication Setup")).toBeDefined();
    });

    const checkbox = screen.getByRole("checkbox");
    fireEvent.click(checkbox);

    const codeInput = screen.getByPlaceholderText("totpPlaceholder");
    fireEvent.change(codeInput, { target: { value: "123456" } });

    const submitBtn = screen.getByRole("button", { name: "Continue" });
    fireEvent.click(submitBtn);

    expect(screen.getByRole("button", { name: "..." })).toBeDefined();
  });
});
