import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import ConsentPage from "./ConsentPage";
import { api, ApiError } from "@/lib/api";
import { toast } from "sonner";
import { render, screen, fireEvent, cleanup, waitFor } from "@testing-library/react";

vi.mock("sonner", () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}));

const mockSearchParams = new URLSearchParams();

vi.mock("react-router-dom", () => ({
  useSearchParams: () => [mockSearchParams],
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

const mockRunWithStepUp = vi.fn().mockImplementation(async (action: any) => action());

vi.mock("@/hooks/useStepUp", () => ({
  useStepUp: () => ({
    runWithStepUp: mockRunWithStepUp,
    stepUpOpen: false,
    stepUpError: "",
    stepUpSubmitting: false,
    handleStepUpSubmit: vi.fn(),
    handleStepUpClose: vi.fn(),
  }),
}));

vi.mock("@/components/StepUpMfaDialog", () => ({
  StepUpMfaDialog: () => null,
}));

describe("ConsentPage", () => {
  let originalLocation: any;

  beforeEach(() => {
    mockSearchParams.delete("client_id");
    mockSearchParams.delete("redirect_uri");
    mockSearchParams.delete("scope");
    mockSearchParams.delete("state");
    mockSearchParams.delete("code_challenge");
    mockSearchParams.delete("code_challenge_method");
    mockSearchParams.delete("nonce");
    vi.mocked(toast.error).mockClear();
    mockRunWithStepUp.mockClear();
    mockRunWithStepUp.mockImplementation(async (action: any) => action());
    
    originalLocation = window.location;
    delete (window as any).location;
    window.location = { href: "" } as any;

    vi.spyOn(api, "getOidcClientInfo").mockResolvedValue({ name: "Test App", allowed_scopes: ["openid"] });
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    window.location = originalLocation;
  });

  it("renders requested client info and scopes", async () => {
    mockSearchParams.set("client_id", "test-client");
    mockSearchParams.set("scope", "openid profile email custom_scope");

    render(React.createElement(ConsentPage));

    await waitFor(() => {
      expect(screen.getByText("test-client")).toBeDefined();
    });

    expect(screen.getByText("consent.scopes.openid")).toBeDefined();
    expect(screen.getByText("consent.scopes.profile")).toBeDefined();
    expect(screen.getByText("consent.scopes.email")).toBeDefined();
    expect(screen.getByText("consent.scopes.custom_scope")).toBeDefined();
  });

  it("shows resolved client display name when clientName state is populated", async () => {
    mockSearchParams.set("client_id", "test-client");
    vi.spyOn(api, "getOidcClientInfo").mockResolvedValue({ name: "My Web App", allowed_scopes: ["openid"] });

    render(React.createElement(ConsentPage));

    await waitFor(() => {
      expect(screen.getByText("My Web App")).toBeDefined();
    });
    expect(screen.getByText("test-client")).toBeDefined();
  });

  it("falls back to client_id when clientName is null", async () => {
    mockSearchParams.set("client_id", "fallback-client");
    vi.spyOn(api, "getOidcClientInfo").mockRejectedValue(new Error("failed"));

    render(React.createElement(ConsentPage));

    await waitFor(() => {
      expect(screen.getByText("fallback-client")).toBeDefined();
    });
  });

  it("handles approve authorization flow", async () => {
    mockSearchParams.set("client_id", "test-client");
    mockSearchParams.set("redirect_uri", "http://localhost:3000/callback");
    mockSearchParams.set("scope", "openid");

    const submitConsentSpy = vi.spyOn(api, "submitConsent").mockResolvedValue({
      redirect_url: "http://localhost:3000/callback?code=foo",
    });

    render(React.createElement(ConsentPage));

    await waitFor(() => {
      expect(screen.getByText("consent.authorize")).toBeDefined();
    });

    fireEvent.click(screen.getByText("consent.authorize"));

    await waitFor(() => {
      expect(submitConsentSpy).toHaveBeenCalledWith({
        client_id: "test-client",
        redirect_uri: "http://localhost:3000/callback",
        scopes: ["openid"],
        code_challenge: "",
        code_challenge_method: "",
        nonce: "",
        state: "",
        approved: true,
      });
      expect(window.location.href).toBe("http://localhost:3000/callback?code=foo");
    });
  });

  it("handles reject/cancel flow", async () => {
    mockSearchParams.set("client_id", "test-client");
    mockSearchParams.set("redirect_uri", "http://localhost:3000/callback");

    const submitConsentSpy = vi.spyOn(api, "submitConsent").mockResolvedValue({
      redirect_url: "http://localhost:3000/callback?error=access_denied",
    });

    render(React.createElement(ConsentPage));

    await waitFor(() => {
      expect(screen.getByText("consent.cancel")).toBeDefined();
    });

    fireEvent.click(screen.getByText("consent.cancel"));

    await waitFor(() => {
      expect(submitConsentSpy).toHaveBeenCalledWith({
        client_id: "test-client",
        redirect_uri: "http://localhost:3000/callback",
        scopes: [],
        code_challenge: "",
        code_challenge_method: "",
        nonce: "",
        state: "",
        approved: false,
      });
      expect(window.location.href).toBe("http://localhost:3000/callback?error=access_denied");
    });
  });

  it("handles error during submission", async () => {
    mockSearchParams.set("client_id", "test-client");
    vi.spyOn(api, "submitConsent").mockRejectedValue(new ApiError("invalid_request", undefined, 400));

    render(React.createElement(ConsentPage));

    await waitFor(() => {
      expect(screen.getByText("consent.authorize")).toBeDefined();
    });

    fireEvent.click(screen.getByText("consent.authorize"));

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("errors.invalid_request");
    });
  });

  it("handles generic error during submission", async () => {
    mockSearchParams.set("client_id", "test-client");
    vi.spyOn(api, "submitConsent").mockRejectedValue(new Error("generic error"));

    render(React.createElement(ConsentPage));

    await waitFor(() => {
      expect(screen.getByText("consent.authorize")).toBeDefined();
    });

    fireEvent.click(screen.getByText("consent.authorize"));

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("consent.errorInternal");
    });
  });

  it("performs MFA step-up when consent returns 403 mfa_required", async () => {
    mockSearchParams.set("client_id", "test-client");
    mockSearchParams.set("redirect_uri", "http://localhost/cb");
    mockSearchParams.set("scope", "openid");

    const submitConsentSpy = vi.spyOn(api, "submitConsent")
      .mockRejectedValueOnce(new ApiError("mfa_required", undefined, 403))
      .mockResolvedValueOnce({ redirect_url: "http://localhost/cb?code=stepped" });

    const mfaStepUpSpy = vi.spyOn(api, "mfaStepUp").mockResolvedValue({ ok: true });

    mockRunWithStepUp.mockImplementationOnce(async (action: any) => {
      try { return await action(); }
      catch { await api.mfaStepUp("123456"); return action(); }
    });

    render(React.createElement(ConsentPage));

    await waitFor(() => {
      expect(screen.getByText("consent.authorize")).toBeDefined();
    });

    fireEvent.click(screen.getByText("consent.authorize"));

    await waitFor(() => {
      expect(mfaStepUpSpy).toHaveBeenCalledWith("123456");
      expect(submitConsentSpy).toHaveBeenCalledTimes(2);
      expect(window.location.href).toBe("http://localhost/cb?code=stepped");
    });
  });
});
