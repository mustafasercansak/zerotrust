import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { renderToString } from "react-dom/server";
import { api, ApiError } from "@/lib/api";
import { toast } from "sonner";

vi.mock("sonner", () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("@/lib/webauthn", () => ({
  isWebAuthnSupported: vi.fn().mockReturnValue(false),
  performRegistration: vi.fn(),
}));

// Index-based useState mock — PasskeysSection has 3 state slots:
//   0: credentials, 1: loading, 2: busy
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
      stateSetters[idx] = (newVal: any) => {
        stateStore[idx] = typeof newVal === "function" ? newVal(stateStore[idx]) : newVal;
      };
      if (callIdx >= 10) callIdx = 0;
      return [stateStore[idx], stateSetters[idx]];
    },
    useEffect: (fn: any) => { fn(); },
  };
});

const capturedButtonClicks: any[] = [];

vi.mock("@mui/material/Button", () => ({
  default: (props: any) => {
    if (props.onClick) capturedButtonClicks.push(props.onClick);
    return React.createElement("button", { disabled: props.disabled }, props.children);
  },
}));
vi.mock("@mui/material/IconButton", () => ({
  default: (props: any) => {
    if (props.onClick) capturedButtonClicks.push(props.onClick);
    return React.createElement("button", { "aria-label": props["aria-label"] }, props.children);
  },
}));
vi.mock("@mui/material/Paper", () => ({ default: (p: any) => React.createElement("div", null, p.children) }));
vi.mock("@mui/material/Box", () => ({ default: (p: any) => React.createElement("div", null, p.children) }));
vi.mock("@mui/material/Typography", () => ({ default: (p: any) => React.createElement("span", null, p.children) }));
vi.mock("@mui/material/Divider", () => ({ default: () => null }));
vi.mock("@mui/material/Alert", () => ({ default: (p: any) => React.createElement("div", { role: "alert" }, p.children) }));
vi.mock("@mui/material/Tooltip", () => ({ default: (p: any) => React.createElement("span", null, p.children) }));
vi.mock("@mui/icons-material/Fingerprint", () => ({ default: () => null }));
vi.mock("@mui/icons-material/DeleteOutlined", () => ({ default: () => null }));

import PasskeysSection from "./PasskeysSection";
import { isWebAuthnSupported, performRegistration } from "@/lib/webauthn";

describe("PasskeysSection", () => {
  beforeEach(() => {
    stateStore = {};
    stateSetters = {};
    callIdx = 0;
    capturedButtonClicks.length = 0;
    vi.mocked(toast.error).mockClear();
    vi.mocked(isWebAuthnSupported).mockReturnValue(false);
    vi.spyOn(api, "webauthnList").mockResolvedValue({ credentials: [] });
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  const runRender = () => {
    callIdx = 0;
    return renderToString(React.createElement(PasskeysSection));
  };

  // ── Unsupported state ────────────────────────────────────────────────────────

  it("shows unsupported notice when WebAuthn is unavailable", () => {
    const html = runRender();
    expect(html).toContain("passkeys.title");
    expect(html).toContain("passkeys.unsupported");
    expect(html).not.toContain("passkeys.add");
  });

  it("does not call webauthnList when WebAuthn is unsupported", () => {
    runRender();
    expect(api.webauthnList).not.toHaveBeenCalled();
  });

  // ── Supported state ──────────────────────────────────────────────────────────

  it("shows add button and calls webauthnList on mount when WebAuthn is supported", () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    const html = runRender();
    expect(html).toContain("passkeys.add");
    expect(api.webauthnList).toHaveBeenCalledOnce();
  });

  it("renders credential list with last-used date when credentials are pre-loaded", () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    stateStore[0] = [
      { id: "c1", name: "iPhone Touch ID", sign_count: 5, created_at: "2026-01-01T00:00:00Z", last_used_at: "2026-06-01T00:00:00Z" },
      { id: "c2", name: "YubiKey 5", sign_count: 0, created_at: "2026-02-01T00:00:00Z", last_used_at: null },
    ];
    stateStore[1] = false; // loading = false
    const html = runRender();
    expect(html).toContain("iPhone Touch ID");
    expect(html).toContain("YubiKey 5");
    expect(html).toContain("passkeys.neverUsed");
  });

  it("shows empty state when credential list is empty and loading is done", () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    stateStore[0] = [];
    stateStore[1] = false;
    const html = runRender();
    expect(html).toContain("passkeys.empty");
    expect(html).not.toContain("passkeys.unsupported");
  });

  it("shows error toast when credential load fails", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    vi.spyOn(api, "webauthnList").mockRejectedValue(new Error("network error"));
    runRender();
    await vi.waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("passkeys.loadError");
    });
  });

  // ── handleAdd ────────────────────────────────────────────────────────────────

  it("handleAdd: does nothing when user cancels the name prompt", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    stateStore[1] = false;
    vi.stubGlobal("window", { prompt: vi.fn().mockReturnValue(null) });
    const beginSpy = vi.spyOn(api, "webauthnRegisterBegin").mockResolvedValue({} as any);
    runRender();
    await capturedButtonClicks[0](); // Add button
    expect(beginSpy).not.toHaveBeenCalled();
  });

  it("handleAdd: registers passkey and refreshes list on success", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    stateStore[1] = false;
    vi.stubGlobal("window", { prompt: vi.fn().mockReturnValue("My Key") });
    const fakeOptions = { challenge: "abc" };
    const fakeCred = { id: "new-cred" };
    vi.spyOn(api, "webauthnRegisterBegin").mockResolvedValue(fakeOptions as any);
    vi.mocked(performRegistration).mockResolvedValue(fakeCred as any);
    vi.spyOn(api, "webauthnRegisterFinish").mockResolvedValue(undefined as any);
    vi.spyOn(api, "webauthnList").mockResolvedValue({ credentials: [
      { id: "new-cred", name: "My Key", sign_count: 0, created_at: "2026-06-01T00:00:00Z", last_used_at: null },
    ]});
    runRender();
    await capturedButtonClicks[0]();
    expect(api.webauthnRegisterBegin).toHaveBeenCalled();
    expect(performRegistration).toHaveBeenCalledWith(fakeOptions);
    expect(api.webauthnRegisterFinish).toHaveBeenCalledWith("My Key", fakeCred);
    expect(api.webauthnList).toHaveBeenCalledTimes(2); // mount + post-register refresh
  });

  it("handleAdd: uses default name when prompt returns only whitespace", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    stateStore[1] = false;
    vi.stubGlobal("window", { prompt: vi.fn().mockReturnValue("   ") });
    vi.spyOn(api, "webauthnRegisterBegin").mockResolvedValue({} as any);
    vi.mocked(performRegistration).mockResolvedValue({} as any);
    const finishSpy = vi.spyOn(api, "webauthnRegisterFinish").mockResolvedValue(undefined as any);
    runRender();
    await capturedButtonClicks[0]();
    expect(finishSpy).toHaveBeenCalledWith("passkeys.defaultName", expect.anything());
  });

  it("handleAdd: shows duplicate error toast on credential_already_registered", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    stateStore[1] = false;
    vi.stubGlobal("window", { prompt: vi.fn().mockReturnValue("Key") });
    vi.spyOn(api, "webauthnRegisterBegin").mockRejectedValue(new ApiError("credential_already_registered"));
    runRender();
    await capturedButtonClicks[0]();
    expect(toast.error).toHaveBeenCalledWith("passkeys.duplicateError");
  });

  it("handleAdd: shows hardware attestation error on hardware_attestation_required", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    stateStore[1] = false;
    vi.stubGlobal("window", { prompt: vi.fn().mockReturnValue("Key") });
    vi.spyOn(api, "webauthnRegisterBegin").mockRejectedValue(new ApiError("hardware_attestation_required"));
    runRender();
    await capturedButtonClicks[0]();
    expect(toast.error).toHaveBeenCalledWith("passkeys.hardwareAttestationRequiredError");
  });

  it("handleAdd: shows generic error toast on unknown registration failure", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    stateStore[1] = false;
    vi.stubGlobal("window", { prompt: vi.fn().mockReturnValue("Key") });
    vi.spyOn(api, "webauthnRegisterBegin").mockRejectedValue(new Error("unexpected"));
    runRender();
    await capturedButtonClicks[0]();
    expect(toast.error).toHaveBeenCalledWith("passkeys.registerError");
  });

  // ── handleDelete ─────────────────────────────────────────────────────────────

  it("handleDelete: calls deleteCredential with correct id and refreshes list", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    stateStore[0] = [
      { id: "cred-abc", name: "iPhone", sign_count: 0, created_at: "2026-01-01T00:00:00Z", last_used_at: null },
    ];
    stateStore[1] = false;
    const deleteSpy = vi.spyOn(api, "webauthnDeleteCredential").mockResolvedValue(undefined as any);
    runRender();
    // capturedButtonClicks[0] = Add, [1] = Delete for first credential
    await capturedButtonClicks[1]();
    expect(deleteSpy).toHaveBeenCalledWith("cred-abc");
    expect(api.webauthnList).toHaveBeenCalled();
  });

  it("handleDelete: shows error toast on delete failure", async () => {
    vi.mocked(isWebAuthnSupported).mockReturnValue(true);
    stateStore[0] = [
      { id: "cred-abc", name: "iPhone", sign_count: 0, created_at: "2026-01-01T00:00:00Z", last_used_at: null },
    ];
    stateStore[1] = false;
    vi.spyOn(api, "webauthnDeleteCredential").mockRejectedValue(new Error("delete failed"));
    runRender();
    await capturedButtonClicks[1]();
    expect(toast.error).toHaveBeenCalledWith("passkeys.removeError");
  });
});
