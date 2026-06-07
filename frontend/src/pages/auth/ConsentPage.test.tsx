import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import ConsentPage from "./ConsentPage";
import { api, ApiError } from "@/lib/api";
import { toast } from "sonner";
import { renderToString } from "react-dom/server";

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

const capturedButtonClicks: any[] = [];
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
      if (callIdx >= 5) {
        callIdx = 0;
      }
      return [stateStore[idx], stateSetters[idx]];
    },
  };
});

vi.mock("@mui/material/Button", () => ({
  default: (props: any) => {
    if (props.onClick) {
      capturedButtonClicks.push(props.onClick);
    }
    return React.createElement("button", { onClick: props.onClick }, props.children);
  }
}));

describe("ConsentPage", () => {
  beforeEach(() => {
    stateStore = {};
    stateSetters = {};
    callIdx = 0;
    capturedButtonClicks.length = 0;
    mockSearchParams.delete("client_id");
    mockSearchParams.delete("redirect_uri");
    mockSearchParams.delete("scope");
    mockSearchParams.delete("state");
    mockSearchParams.delete("code_challenge");
    mockSearchParams.delete("code_challenge_method");
    mockSearchParams.delete("nonce");
    vi.mocked(toast.error).mockClear();
    vi.stubGlobal("window", { location: { href: "" } });
    vi.spyOn(api, "getOidcClientInfo").mockResolvedValue({ name: "Test App", allowed_scopes: ["openid"] });
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  const runRender = () => {
    callIdx = 0;
    return renderToString(React.createElement(ConsentPage));
  };

  it("renders requested client info and scopes", () => {
    mockSearchParams.set("client_id", "test-client");
    mockSearchParams.set("scope", "openid profile email custom_scope");

    const html = runRender();
    expect(html).toContain("test-client");
    expect(html).toContain("consent.scopes.openid");
    expect(html).toContain("consent.scopes.profile");
    expect(html).toContain("consent.scopes.email");
    expect(html).toContain("consent.scopes.custom_scope");
  });

  it("shows resolved client display name when clientName state is populated", () => {
    mockSearchParams.set("client_id", "test-client");
    // Pre-populate clientName state (idx 1) so the component renders the name
    stateStore[1] = "My Web App";
    const html = runRender();
    expect(html).toContain("My Web App");
    // Still shows the raw client_id in smaller text as the identifier
    expect(html).toContain("test-client");
  });

  it("falls back to client_id when clientName is null", () => {
    mockSearchParams.set("client_id", "fallback-client");
    // clientName state (idx 1) defaults to null — not pre-populated
    const html = runRender();
    expect(html).toContain("fallback-client");
  });

  it("handles approve authorization flow", async () => {
    mockSearchParams.set("client_id", "test-client");
    mockSearchParams.set("redirect_uri", "http://localhost:3000/callback");
    mockSearchParams.set("scope", "openid");

    const submitConsentSpy = vi.spyOn(api, "submitConsent").mockResolvedValue({
      redirect_url: "http://localhost:3000/callback?code=foo",
    });

    runRender();

    // Trigger Authorize button (index 0)
    expect(capturedButtonClicks[0]).toBeDefined();
    await capturedButtonClicks[0]();

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

  it("handles reject/cancel flow", async () => {
    mockSearchParams.set("client_id", "test-client");
    mockSearchParams.set("redirect_uri", "http://localhost:3000/callback");

    const submitConsentSpy = vi.spyOn(api, "submitConsent").mockResolvedValue({
      redirect_url: "http://localhost:3000/callback?error=access_denied",
    });

    runRender();

    // Trigger Cancel button (index 1)
    expect(capturedButtonClicks[1]).toBeDefined();
    await capturedButtonClicks[1]();

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

  it("handles error during submission", async () => {
    mockSearchParams.set("client_id", "test-client");
    vi.spyOn(api, "submitConsent").mockRejectedValue(new ApiError("invalid_request", undefined, 400));

    runRender();

    await capturedButtonClicks[0]();
    expect(toast.error).toHaveBeenCalledWith("errors.invalid_request");
  });

  it("handles generic error during submission", async () => {
    mockSearchParams.set("client_id", "test-client");
    vi.spyOn(api, "submitConsent").mockRejectedValue(new Error("generic error"));

    runRender();

    await capturedButtonClicks[0]();
    expect(toast.error).toHaveBeenCalledWith("consent.errorInternal");
  });

  it("performs MFA step-up when consent returns 403 mfa_required", async () => {
    mockSearchParams.set("client_id", "test-client");
    mockSearchParams.set("redirect_uri", "http://localhost/cb");
    mockSearchParams.set("scope", "openid");

    vi.stubGlobal("window", {
      location: { href: "" },
      prompt: vi.fn().mockReturnValue("123456"),
    });

    const submitConsentSpy = vi.spyOn(api, "submitConsent")
      .mockRejectedValueOnce(new ApiError("mfa_required", undefined, 403))
      .mockResolvedValueOnce({ redirect_url: "http://localhost/cb?code=stepped" });

    const mfaStepUpSpy = vi.spyOn(api, "mfaStepUp").mockResolvedValue({ ok: true });

    runRender();

    await capturedButtonClicks[0]();

    expect(window.prompt).toHaveBeenCalled();
    expect(mfaStepUpSpy).toHaveBeenCalledWith("123456");
    expect(submitConsentSpy).toHaveBeenCalledTimes(2);
    expect(window.location.href).toBe("http://localhost/cb?code=stepped");
  });
});
