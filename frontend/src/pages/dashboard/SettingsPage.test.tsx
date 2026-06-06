import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import SettingsPage from "./SettingsPage";
import { api, ApiError } from "@/lib/api";
import { useMeContext } from "@/contexts/MeContext";
import { toast } from "sonner";
import { renderToString } from "react-dom/server";

vi.mock("sonner", () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}));

// Mock react-router-dom and react-i18next
vi.mock("react-router-dom", () => ({
  useNavigate: () => vi.fn(),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

// Mock MeContext
vi.mock("@/contexts/MeContext", () => ({
  useMeContext: vi.fn(),
}));

// Mock child SessionsPage
vi.mock("./SessionsPage", () => ({
  default: () => React.createElement("div", null, "SessionsPageMock"),
}));

const capturedSubmits: any[] = [];
const capturedButtonClicks: any[] = [];
const capturedInputs: any[] = [];
const capturedTabChanges: any[] = [];
const capturedTextFieldChanges: any[] = [];
const capturedSwitchChanges: any[] = [];

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
      if (callIdx >= 35) {
        callIdx = 0;
      }
      return [stateStore[idx], stateSetters[idx]];
    },
    useEffect: (fn: any) => {
      fn();
    },
  };
});

vi.mock("@mui/material/Paper", () => ({
  default: (props: any) => {
    if (props.onSubmit) {
      capturedSubmits.push(props.onSubmit);
    }
    return React.createElement("div", null, props.children);
  }
}));

vi.mock("@mui/material/Button", () => ({
  default: (props: any) => {
    if (props.onClick) {
      capturedButtonClicks.push(props.onClick);
    }
    // Safely check for input children
    React.Children.forEach(props.children, (child: any) => {
      if (child && child.props && child.props.type === "file" && child.props.onChange) {
        capturedInputs.push(child.props.onChange);
      }
    });
    return React.createElement("button", { onClick: props.onClick, type: props.type }, props.children);
  }
}));

vi.mock("@mui/material/Tabs", () => ({
  default: (props: any) => {
    if (props.onChange) {
      capturedTabChanges.push(props.onChange);
    }
    return React.createElement("div", null, props.children);
  }
}));

vi.mock("@mui/material/Tab", () => ({
  default: (props: any) => {
    return React.createElement("div", { id: props.id }, props.label);
  }
}));

vi.mock("@mui/material/TextField", () => ({
  default: (props: any) => {
    if (props.onChange) {
      capturedTextFieldChanges.push(props.onChange);
    }
    return React.createElement("input", { onChange: props.onChange, value: props.value });
  }
}));

vi.mock("@mui/material/Switch", () => ({
  default: (props: any) => {
    if (props.onChange) {
      capturedSwitchChanges.push(props.onChange);
    }
    return React.createElement("input", { type: "checkbox", onChange: props.onChange, checked: props.checked });
  }
}));

describe("SettingsPage page component", () => {
  beforeEach(() => {
    stateStore = {};
    stateSetters = {};
    callIdx = 0;
    capturedSubmits.length = 0;
    capturedButtonClicks.length = 0;
    capturedInputs.length = 0;
    capturedTabChanges.length = 0;
    capturedTextFieldChanges.length = 0;
    capturedSwitchChanges.length = 0;
    vi.mocked(toast.error).mockClear();
    vi.mocked(toast.success).mockClear();
    vi.stubGlobal("confirm", vi.fn().mockReturnValue(true));
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  const runRender = () => {
    callIdx = 0;
    return renderToString(React.createElement(SettingsPage));
  };

  it("renders loader or profiles correctly", async () => {
    // 1st: me is null
    vi.mocked(useMeContext).mockReturnValue(null);
    let html = runRender();
    expect(html).toContain("CircularProgress");

    // 2nd: me is present
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: true,
      roles: ["admin"],
      locale: "en",
    });

    const getSettingsSpy = vi.spyOn(api.admin, "getSettings").mockResolvedValue({
      max_sessions_per_user: "5",
      password_complexity: "strong",
      global_mfa_required: "true",
      max_login_attempts: "5",
    });

    runRender(); // run once to trigger useEffect and update stateStore
    html = runRender(); // run again to capture the updated HTML
    expect(html).toContain("John");
    expect(html).toContain("Doe");

    await vi.waitFor(() => {
      expect(getSettingsSpy).toHaveBeenCalled();
    });
  });

  it("handles profile save success and failure paths", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: true,
      roles: ["admin"],
      locale: "en",
    });

    vi.spyOn(api.admin, "getSettings").mockResolvedValue({});
    const updateSpy = vi.spyOn(api, "updateProfile").mockResolvedValue({
      id: "u123",
      first_name: "JohnUpdated",
      last_name: "DoeUpdated",
    } as any);

    runRender();

    expect(capturedSubmits[0]).toBeDefined();

    // Trigger form submit
    await capturedSubmits[0]({ preventDefault: vi.fn() });
    expect(updateSpy).toHaveBeenCalled();

    // Trigger update profile failure
    vi.spyOn(api, "updateProfile").mockRejectedValue(new ApiError("invalid_value", undefined, 400));
    await capturedSubmits[0]({ preventDefault: vi.fn() });
  });

  it("handles avatar change and delete paths", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: true,
      roles: ["admin"],
      locale: "en",
    });

    vi.spyOn(api.admin, "getSettings").mockResolvedValue({});
    const uploadSpy = vi.spyOn(api, "uploadAvatar").mockResolvedValue({} as any);
    const deleteSpy = vi.spyOn(api, "deleteAvatar").mockResolvedValue({} as any);

    runRender();

    // Test Avatar Change Too Large
    expect(capturedInputs[0]).toBeDefined();
    capturedInputs[0]({ target: { files: [{ size: 3 * 1024 * 1024 }] } });

    // Test Avatar Change Success
    const file = { size: 1024 };
    await capturedInputs[0]({ target: { files: [file] } });
    expect(uploadSpy).toHaveBeenCalledWith(file);

    // Test Avatar Delete Success
    expect(capturedButtonClicks[0]).toBeDefined();
    await capturedButtonClicks[0]();
    expect(deleteSpy).toHaveBeenCalled();
  });

  it("ignores avatar input changes when no file is selected", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: true,
      roles: ["admin"],
      locale: "en",
    });

    vi.spyOn(api.admin, "getSettings").mockResolvedValue({});
    const uploadSpy = vi.spyOn(api, "uploadAvatar").mockResolvedValue({} as any);

    runRender();
    expect(capturedInputs[0]).toBeDefined();
    await capturedInputs[0]({ target: { files: [] } });

    expect(uploadSpy).not.toHaveBeenCalled();
  });

  it("handles system settings save including step up mfa", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: true,
      roles: ["admin"],
      locale: "en",
    });

    vi.spyOn(api.admin, "getSettings").mockResolvedValue({});
    const updateSettingsSpy = vi.spyOn(api.admin, "updateSettings").mockResolvedValue({} as any);

    // Render with activeTab = 2 (System)
    stateStore[0] = 2; // set activeTab state to 2
    runRender();

    // Await getSettings API resolution to update maxSessions/maxLoginAttempts to "5"
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender(); // re-render to populate closures

    expect(capturedSubmits[capturedSubmits.length - 1]).toBeDefined();

    // Save settings success
    await capturedSubmits[capturedSubmits.length - 1]({ preventDefault: vi.fn() });
    expect(updateSettingsSpy).toHaveBeenCalled();

    // Test invalid values (sessions out of range)
    stateStore[6] = "99";
    runRender(); // re-render with new state
    await capturedSubmits[capturedSubmits.length - 1]({ preventDefault: vi.fn() });

    // Test MFA Step-up prompt
    stateStore[6] = "5";
    runRender(); // re-render with new state
    vi.spyOn(api.admin, "updateSettings").mockRejectedValueOnce(new ApiError("mfa_required", undefined, 403));
    vi.stubGlobal("window", {
      prompt: vi.fn().mockReturnValue("123456"),
    });
    const stepUpSpy = vi.spyOn(api, "mfaStepUp").mockResolvedValue({} as any);

    await capturedSubmits[capturedSubmits.length - 1]({ preventDefault: vi.fn() });
    expect(stepUpSpy).toHaveBeenCalledWith("123456");
  });

  it("handles system settings load and save failure branches", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: true,
      roles: ["admin"],
      locale: "en",
    });

    vi.spyOn(api.admin, "getSettings").mockRejectedValue(new Error("settings unavailable"));
    stateStore[0] = 2;
    runRender();
    await Promise.resolve();
    await Promise.resolve();
    runRender();
    expect(toast.error).toHaveBeenLastCalledWith("errors.internal_error");

    stateStore[6] = "5";
    stateStore[9] = "99";
    await capturedSubmits[capturedSubmits.length - 1]({ preventDefault: vi.fn() });
    expect(toast.error).toHaveBeenLastCalledWith("errors.invalid_value");

    stateStore[9] = "5";
    vi.spyOn(api.admin, "updateSettings").mockRejectedValueOnce(new Error("save failed"));
    runRender();
    await capturedSubmits[capturedSubmits.length - 1]({ preventDefault: vi.fn() });
    expect(toast.error).toHaveBeenLastCalledWith("errors.internal_error");
  });

  it("does not complete system save when step-up MFA prompt is empty", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: true,
      roles: ["admin"],
      locale: "en",
    });

    vi.spyOn(api.admin, "getSettings").mockResolvedValue({});
    const updateSpy = vi.spyOn(api.admin, "updateSettings").mockRejectedValue(new ApiError("mfa_required", undefined, 403));
    const stepUpSpy = vi.spyOn(api, "mfaStepUp").mockResolvedValue({} as any);
    vi.stubGlobal("window", { prompt: vi.fn().mockReturnValue("") });
    stateStore[0] = 2;
    stateStore[6] = "5";
    stateStore[9] = "5";
    runRender();

    await capturedSubmits[capturedSubmits.length - 1]({ preventDefault: vi.fn() });

    expect(updateSpy).toHaveBeenCalledTimes(1);
    expect(stepUpSpy).not.toHaveBeenCalled();
  });

  it("triggers interactive form and tab controls", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: true,
      roles: ["admin"],
      locale: "en",
    });

    vi.spyOn(api.admin, "getSettings").mockResolvedValue({});
    
    // 1st render (profile tab)
    runRender();

    // Call tab onChange (switch to tab 1, then tab 2)
    expect(capturedTabChanges[0]).toBeDefined();
    capturedTabChanges[0](null, 1);
    capturedTabChanges[0](null, 2);

    // Call firstName/lastName textfields onChange
    expect(capturedTextFieldChanges[0]).toBeDefined();
    capturedTextFieldChanges[0]({ target: { value: "JohnNew" } });
    capturedTextFieldChanges[1]({ target: { value: "DoeNew" } });

    // Await getSettings
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();

    // Render on activeTab = 2 (System)
    stateStore[0] = 2;
    runRender();

    // Now under system tab, textfields should be populated/captured
    capturedTextFieldChanges.forEach((tfChange) => {
      tfChange({ target: { value: "8" } });
    });

    // Call Switch onChange
    expect(capturedSwitchChanges[0]).toBeDefined();
    capturedSwitchChanges[0]({ target: { checked: true } });
    capturedSwitchChanges[0]({ target: { checked: false } });
  });

  it("renders email initials, no-avatar profile, and the security tab", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "fallback@example.com",
      first_name: undefined,
      last_name: undefined,
      has_avatar: false,
      roles: ["user"],
      locale: "en",
    } as any);
    const getSettingsSpy = vi.spyOn(api.admin, "getSettings").mockResolvedValue({});

    let html = runRender();
    expect(html).toContain("FA");
    expect(html).not.toContain("/api/v1/users/u123/avatar");
    expect(getSettingsSpy).not.toHaveBeenCalled();

    stateStore[0] = 1;
    html = runRender();
    expect(html).toContain("SessionsPageMock");
  });

  it("renders saving labels and shows success toasts on save", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: true,
      roles: ["admin"],
      locale: "en",
    });
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({});
    vi.spyOn(api, "updateProfile").mockResolvedValue({} as any);
    vi.spyOn(api.admin, "updateSettings").mockResolvedValue({} as any);
    vi.stubGlobal("window", { dispatchEvent: vi.fn() });
    vi.stubGlobal("CustomEvent", class {
      type: string;
      detail: unknown;
      constructor(type: string, init?: { detail?: unknown }) {
        this.type = type;
        this.detail = init?.detail;
      }
    });

    stateStore[3] = true; // savingProfile → renders the "saving" label
    expect(runRender()).toContain("saving");
    await capturedSubmits[0]({ preventDefault: vi.fn() });
    expect(toast.success).toHaveBeenCalledWith("saved");

    stateStore[0] = 2;
    stateStore[6] = "5";
    stateStore[9] = "5";
    stateStore[10] = false; // systemLoading
    stateStore[11] = true; // savingSystem → renders the "saving" label
    expect(runRender()).toContain("saving");
    await capturedSubmits[capturedSubmits.length - 1]({ preventDefault: vi.fn() });
    expect(toast.success).toHaveBeenCalledWith("saved");
  });

  it("handles avatar ApiError failures", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: true,
      roles: ["admin"],
      locale: "en",
    });
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({});
    vi.spyOn(api, "uploadAvatar").mockRejectedValue(new ApiError("invalid_value", undefined, 400));
    vi.spyOn(api, "deleteAvatar").mockRejectedValue(new ApiError("invalid_value", undefined, 400));

    runRender();
    await capturedInputs[0]({ target: { files: [{ size: 1024 }] } });
    expect(toast.error).toHaveBeenLastCalledWith("errors.invalid_value");

    await capturedButtonClicks[0]();
    expect(toast.error).toHaveBeenLastCalledWith("errors.invalid_value");
  });

  it("handles nonnumeric login attempts and undefined MFA prompt returns", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: true,
      roles: ["admin"],
      locale: "en",
    });
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({});
    const updateSpy = vi.spyOn(api.admin, "updateSettings").mockRejectedValue(new ApiError("mfa_required", undefined, 403));
    const stepUpSpy = vi.spyOn(api, "mfaStepUp").mockResolvedValue({} as any);
    vi.stubGlobal("window", { prompt: vi.fn().mockReturnValue(undefined) });

    stateStore[0] = 2;
    stateStore[6] = "5";
    stateStore[9] = "nope";
    stateStore[10] = false;
    runRender();
    await capturedSubmits[capturedSubmits.length - 1]({ preventDefault: vi.fn() });
    expect(toast.error).toHaveBeenLastCalledWith("errors.invalid_value");

    stateStore[9] = "5";
    runRender();
    await capturedSubmits[capturedSubmits.length - 1]({ preventDefault: vi.fn() });
    expect(updateSpy).toHaveBeenCalledTimes(1);
    expect(stepUpSpy).not.toHaveBeenCalled();
  });

  it("shows a success toast after a profile save", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: true,
      roles: ["admin"],
      locale: "en",
    });
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({});
    const updateProfileSpy = vi.spyOn(api, "updateProfile").mockResolvedValue({} as any);
    vi.stubGlobal("window", { dispatchEvent: vi.fn() });
    vi.stubGlobal("CustomEvent", class {
      type: string;
      detail: unknown;

      constructor(type: string, init?: { detail?: unknown }) {
        this.type = type;
        this.detail = init?.detail;
      }
    });

    runRender();
    await capturedSubmits[0]({ preventDefault: vi.fn() });
    expect(updateProfileSpy).toHaveBeenCalled();
    expect(toast.success).toHaveBeenCalledWith("saved");
  });
});
