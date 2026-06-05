import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import MfaPage from "./MfaPage";
import { api } from "@/lib/api";
import { renderToString } from "react-dom/server";

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
      if (callIdx >= 20) {
        callIdx = 0;
      }
      return [stateStore[idx], stateSetters[idx]];
    },
    useEffect: (fn: any) => {
      fn();
    },
  };
});

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

vi.mock("qrcode.react", () => ({
  QRCodeSVG: () => React.createElement("div", null, "QRCode"),
}));

const capturedSubmits: any[] = [];
const capturedButtonClicks: any[] = [];
const capturedInputs: any[] = [];

vi.mock("@mui/material/Button", () => ({
  default: (props: any) => {
    if (props.onClick) {
      capturedButtonClicks.push(props.onClick);
    }
    return React.createElement("button", { onClick: props.onClick, type: props.type, disabled: props.disabled }, props.children);
  }
}));

vi.mock("@mui/material/Box", () => ({
  default: (props: any) => {
    if (props.onSubmit) {
      capturedSubmits.push(props.onSubmit);
    }
    return React.createElement("div", null, props.children);
  }
}));

vi.mock("@mui/material/Checkbox", () => ({
  default: (props: any) => {
    if (props.onChange) {
      capturedInputs.push(props.onChange);
    }
    return React.createElement("input", { type: "checkbox", checked: props.checked, onChange: props.onChange });
  }
}));

vi.mock("@mui/material/TextField", () => ({
  default: (props: any) => {
    if (props.onChange) {
      capturedInputs.push(props.onChange);
    }
    return React.createElement("input", { type: "text", value: props.value, onChange: props.onChange });
  }
}));

describe("MfaPage page component", () => {
  beforeEach(() => {
    stateStore = {};
    stateSetters = {};
    callIdx = 0;
    capturedSubmits.length = 0;
    capturedButtonClicks.length = 0;
    capturedInputs.length = 0;
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  const runRender = () => {
    callIdx = 0;
    return renderToString(React.createElement(MfaPage));
  };

  it("renders loader screen then loading completed", async () => {
    const statusSpy = vi.spyOn(api, "mfaStatus").mockResolvedValue({
      enabled: true,
      supported: true,
    });

    let html = runRender();
    expect(html).toContain("loading");

    await Promise.resolve(); // resolve mfaStatus promise
    await Promise.resolve(); // resolve .then callback
    await Promise.resolve(); // resolve .finally callback

    html = runRender();
    expect(html).toContain("statusEnabled");

    await vi.waitFor(() => {
      expect(statusSpy).toHaveBeenCalled();
    });
  });

  it("handles the complete setup and verify MFA flow", async () => {
    // 1. Initial status is disabled
    const statusSpy = vi.spyOn(api, "mfaStatus").mockResolvedValue({
      enabled: false,
      supported: true,
    });

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender(); // render as disabled

    expect(capturedButtonClicks[0]).toBeDefined(); // setup button

    // Mock setup API response
    const setupSpy = vi.spyOn(api, "mfaSetup").mockResolvedValue({
      otp_auth_url: "otpauth://totp/Test",
      secret: "SECRET123",
      recovery_codes: ["code1", "code2"],
    });

    // Trigger handleSetup
    await capturedButtonClicks[0]();
    expect(setupSpy).toHaveBeenCalled();

    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender(); // render as pending setup stage

    // Trigger checkbox state change for backup codes saved
    expect(capturedInputs[0]).toBeDefined(); // checkbox is the first input rendered in pending stage
    capturedInputs[0]({ target: { checked: true } });

    // Trigger typing code
    expect(capturedInputs[1]).toBeDefined(); // code input is the second input
    capturedInputs[1]({ target: { value: "123456" } });

    runRender(); // re-render to propagate state updates

    // Mock verify API response
    const verifySpy = vi.spyOn(api, "mfaVerify").mockResolvedValue({} as any);
    expect(capturedSubmits[capturedSubmits.length - 1]).toBeDefined(); // verify form submit
    await capturedSubmits[capturedSubmits.length - 1]({ preventDefault: vi.fn() });

    expect(verifySpy).toHaveBeenCalledWith("123456");
  });

  it("handles disabling MFA when it is enabled", async () => {
    vi.spyOn(api, "mfaStatus").mockResolvedValue({
      enabled: true,
      supported: true,
    });

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender(); // render as enabled

    // Trigger typing code
    expect(capturedInputs[0]).toBeDefined();
    capturedInputs[0]({ target: { value: "654321" } });

    runRender(); // re-render to propagate state updates

    // Mock disable API response
    const disableSpy = vi.spyOn(api, "mfaDisable").mockResolvedValue({} as any);
    expect(capturedSubmits[capturedSubmits.length - 1]).toBeDefined(); // disable form submit
    await capturedSubmits[capturedSubmits.length - 1]({ preventDefault: vi.fn() });

    expect(disableSpy).toHaveBeenCalledWith("654321");
  });

  it("handles MFA status unsupported and API error on load", async () => {
    vi.spyOn(api, "mfaStatus").mockResolvedValue({ enabled: false, supported: false });
    let html = runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    html = runRender();
    expect(html).toContain("unsupported");

    // Catch condition
    vi.spyOn(api, "mfaStatus").mockRejectedValue(new Error("API error"));
    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    html = runRender();
    expect(html).toContain("unsupported");
  });

  it("handles errors during MFA setup, verification, and disabling", async () => {
    // Setup failure
    vi.spyOn(api, "mfaStatus").mockResolvedValue({ enabled: false, supported: true });
    vi.spyOn(api, "mfaSetup").mockRejectedValue(new Error("Setup error"));
    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    await capturedButtonClicks[0](); // Click setup
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    let html = runRender();
    expect(html).toContain("errors.internal_error");

    // Verification failure
    vi.spyOn(api, "mfaSetup").mockResolvedValue({
      otp_auth_url: "otpauth://totp/Test",
      secret: "SECRET123",
      recovery_codes: ["code1"],
    });
    vi.spyOn(api, "mfaVerify").mockRejectedValue(new Error("Verify error"));
    await capturedButtonClicks[0](); // click setup again
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    capturedInputs[0]({ target: { checked: true } }); // Check codes saved
    capturedInputs[1]({ target: { value: "000000" } }); // type code
    runRender();
    await capturedSubmits[capturedSubmits.length - 1]({ preventDefault: vi.fn() }); // verify
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    html = runRender();
    expect(html).toContain("errors.invalid_code");

    // Disable failure
    vi.spyOn(api, "mfaStatus").mockResolvedValue({ enabled: true, supported: true });
    vi.spyOn(api, "mfaDisable").mockRejectedValue(new Error("Disable error"));
    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    capturedInputs[0]({ target: { value: "000000" } });
    runRender();
    await capturedSubmits[capturedSubmits.length - 1]({ preventDefault: vi.fn() });
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    html = runRender();
    expect(html).toContain("errors.invalid_code");
  });

  it("renders pending setup submitting and error states", () => {
    vi.spyOn(api, "mfaStatus").mockResolvedValue({ enabled: false, supported: true });
    stateStore[0] = "pending";
    stateStore[1] = {
      otp_auth_url: "otpauth://totp/Test",
      secret: "SECRET123",
      recovery_codes: ["code1", "code2"],
    };
    stateStore[3] = "errors.invalid_code";
    stateStore[4] = true;
    stateStore[5] = true;

    const html = runRender();

    expect(html).toContain("...");
    expect(html).toContain("errors.invalid_code");
  });
});
