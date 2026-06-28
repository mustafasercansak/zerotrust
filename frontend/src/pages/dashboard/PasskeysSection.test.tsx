import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { render, screen, fireEvent, cleanup, waitFor } from "@testing-library/react";
import PasskeysSection from "./PasskeysSection";
import { api, ApiError } from "@/lib/api";
import { toast } from "sonner";
import { isWebAuthnSupported, performRegistration } from "@/lib/webauthn";

vi.mock("sonner", () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}));

const mockT = (key: string, options?: any) => {
  if (options?.date) return `${key}:${options.date}`;
  return key;
};

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: mockT,
    i18n: { language: "en" },
  }),
}));

vi.mock("@/lib/webauthn", () => ({
  isWebAuthnSupported: vi.fn().mockReturnValue(false),
  performRegistration: vi.fn(),
}));

describe("PasskeysSection", () => {
  let listSpy: any;

  beforeEach(() => {
    vi.useRealTimers();
    vi.mocked(toast.error).mockClear();
    vi.mocked(isWebAuthnSupported).mockReturnValue(false);
    listSpy = vi.spyOn(api, "webauthnList").mockResolvedValue({ credentials: [] });
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    vi.useRealTimers();
  });

  // ── Unsupported state ────────────────────────────────────────────────────────

  it("shows unsupported notice when WebAuthn is unavailable", () => {
    render(<PasskeysSection />);
    expect(screen.getByText("passkeys.title")).toBeDefined();
    expect(screen.getByText("passkeys.unsupported")).toBeDefined();
    expect(screen.queryByText("passkeys.add")).toBeNull();
  });

  it("does not call webauthnList when WebAuthn is unsupported", () => {
    render(<PasskeysSection />);
    expect(listSpy).not.toHaveBeenCalled();
  });

  // ── Supported state ──────────────────────────────────────────────────────────

  it("shows add button and calls webauthnList on mount when WebAuthn is supported", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    
    render(<PasskeysSection />);
    
    await waitFor(() => {
      expect(screen.getByText("passkeys.add")).toBeDefined();
    });
    expect(listSpy).toHaveBeenCalledOnce();
  });

  it("renders credential list with last-used date when credentials are pre-loaded", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    listSpy.mockResolvedValue({
      credentials: [
        { id: "c1", name: "iPhone Touch ID", sign_count: 5, created_at: "2026-01-01T00:00:00Z", last_used_at: "2026-06-01T00:00:00Z" },
        { id: "c2", name: "YubiKey 5", sign_count: 0, created_at: "2026-02-01T00:00:00Z", last_used_at: null },
      ],
    });

    render(<PasskeysSection />);
    await waitFor(() => {
      expect(screen.getByText(/iPhone Touch ID/)).toBeDefined();
      expect(screen.getByText(/YubiKey 5/)).toBeDefined();
      expect(screen.getByText(/passkeys.neverUsed/)).toBeDefined();
    });
  });

  it("shows empty state when credential list is empty and loading is done", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    listSpy.mockResolvedValue({ credentials: [] });

    render(<PasskeysSection />);

    await waitFor(() => {
      expect(screen.getByText("passkeys.empty")).toBeDefined();
    });
    expect(screen.queryByText("passkeys.unsupported")).toBeNull();
  });

  it("shows error toast when credential load fails", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    listSpy.mockRejectedValue(new Error("network error"));

    render(<PasskeysSection />);

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("passkeys.loadError");
    });
  });

  // ── handleAdd ────────────────────────────────────────────────────────────────

  it("handleAdd: does nothing when user cancels the name prompt", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    listSpy.mockResolvedValue({ credentials: [] });
    const beginSpy = vi.spyOn(api, "webauthnRegisterBegin").mockResolvedValue({} as any);
    
    const promptSpy = vi.spyOn(window, "prompt").mockReturnValue(null);

    render(<PasskeysSection />);

    await waitFor(() => {
      expect(screen.getByText("passkeys.add")).toBeDefined();
    });

    fireEvent.click(screen.getByText("passkeys.add"));
    expect(promptSpy).toHaveBeenCalled();
    expect(beginSpy).not.toHaveBeenCalled();
  });

  it("handleAdd: registers passkey and refreshes list on success", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    listSpy.mockResolvedValue({ credentials: [] });
    const promptSpy = vi.spyOn(window, "prompt").mockReturnValue("My Key");
    
    const fakeOptions = { challenge: "abc" };
    const fakeCred = { id: "new-cred" };
    const beginSpy = vi.spyOn(api, "webauthnRegisterBegin").mockResolvedValue(fakeOptions as any);
    const performSpy = vi.mocked(performRegistration).mockResolvedValue(fakeCred as any);
    const finishSpy = vi.spyOn(api, "webauthnRegisterFinish").mockResolvedValue(undefined as any);

    render(<PasskeysSection />);

    await waitFor(() => {
      expect(screen.getByText("passkeys.add")).toBeDefined();
    });

    fireEvent.click(screen.getByText("passkeys.add"));

    await waitFor(() => {
      expect(beginSpy).toHaveBeenCalled();
      expect(performSpy).toHaveBeenCalledWith(fakeOptions);
      expect(finishSpy).toHaveBeenCalledWith("My Key", fakeCred);
      expect(listSpy).toHaveBeenCalledTimes(2); // mount + refresh
    });
  });

  it("handleAdd: uses default name when prompt returns only whitespace", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    listSpy.mockResolvedValue({ credentials: [] });
    vi.spyOn(window, "prompt").mockReturnValue("   ");

    const beginSpy = vi.spyOn(api, "webauthnRegisterBegin").mockResolvedValue({} as any);
    vi.mocked(performRegistration).mockResolvedValue({} as any);
    const finishSpy = vi.spyOn(api, "webauthnRegisterFinish").mockResolvedValue(undefined as any);

    render(<PasskeysSection />);

    await waitFor(() => {
      expect(screen.getByText("passkeys.add")).toBeDefined();
    });

    fireEvent.click(screen.getByText("passkeys.add"));

    await waitFor(() => {
      expect(finishSpy).toHaveBeenCalledWith("passkeys.defaultName", expect.anything());
    });
  });

  it("handleAdd: shows duplicate error toast on credential_already_registered", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    listSpy.mockResolvedValue({ credentials: [] });
    vi.spyOn(window, "prompt").mockReturnValue("Key");
    
    vi.spyOn(api, "webauthnRegisterBegin").mockRejectedValue(new ApiError("credential_already_registered"));

    render(<PasskeysSection />);

    await waitFor(() => {
      expect(screen.getByText("passkeys.add")).toBeDefined();
    });

    fireEvent.click(screen.getByText("passkeys.add"));

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("passkeys.duplicateError");
    });
  });

  it("handleAdd: shows hardware attestation error on hardware_attestation_required", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    listSpy.mockResolvedValue({ credentials: [] });
    vi.spyOn(window, "prompt").mockReturnValue("Key");
    
    vi.spyOn(api, "webauthnRegisterBegin").mockRejectedValue(new ApiError("hardware_attestation_required"));

    render(<PasskeysSection />);

    await waitFor(() => {
      expect(screen.getByText("passkeys.add")).toBeDefined();
    });

    fireEvent.click(screen.getByText("passkeys.add"));

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("passkeys.hardwareAttestationRequiredError");
    });
  });

  it("handleAdd: shows generic error toast on unknown registration failure", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    listSpy.mockResolvedValue({ credentials: [] });
    vi.spyOn(window, "prompt").mockReturnValue("Key");
    
    vi.spyOn(api, "webauthnRegisterBegin").mockRejectedValue(new Error("unexpected"));

    render(<PasskeysSection />);

    await waitFor(() => {
      expect(screen.getByText("passkeys.add")).toBeDefined();
    });

    fireEvent.click(screen.getByText("passkeys.add"));

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("passkeys.registerError");
    });
  });

  // ── handleDelete ─────────────────────────────────────────────────────────────

  it("handleDelete: calls deleteCredential with correct id and refreshes list", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    listSpy.mockResolvedValue({
      credentials: [
        { id: "cred-abc", name: "iPhone", sign_count: 0, created_at: "2026-01-01T00:00:00Z", last_used_at: null },
      ],
    });
    const deleteSpy = vi.spyOn(api, "webauthnDeleteCredential").mockResolvedValue(undefined as any);

    render(<PasskeysSection />);

    await waitFor(() => {
      expect(screen.getByText("iPhone")).toBeDefined();
    });

    const deleteBtn = screen.getByRole("button", { name: "passkeys.remove" });
    fireEvent.click(deleteBtn);

    await waitFor(() => {
      expect(deleteSpy).toHaveBeenCalledWith("cred-abc");
      expect(listSpy).toHaveBeenCalled();
    });
  });

  it("handleDelete: shows error toast on delete failure", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    listSpy.mockResolvedValue({
      credentials: [
        { id: "cred-abc", name: "iPhone", sign_count: 0, created_at: "2026-01-01T00:00:00Z", last_used_at: null },
      ],
    });
    vi.spyOn(api, "webauthnDeleteCredential").mockRejectedValue(new Error("delete failed"));

    render(<PasskeysSection />);

    await waitFor(() => {
      expect(screen.getByText("iPhone")).toBeDefined();
    });

    const deleteBtn = screen.getByRole("button", { name: "passkeys.remove" });
    fireEvent.click(deleteBtn);

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("passkeys.removeError");
    });
  });
});
