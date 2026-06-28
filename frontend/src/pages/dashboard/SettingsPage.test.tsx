import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { render, screen, fireEvent, waitFor, cleanup } from "@testing-library/react";
import SettingsPage from "./SettingsPage";
import { api, ApiError } from "@/lib/api";
import { useMeContext } from "@/contexts/MeContext";
import { toast } from "sonner";

vi.mock("sonner", () => ({
  toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
}));

// Mock react-router-dom
vi.mock("react-router-dom", () => ({
  useNavigate: () => vi.fn(),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: "en", changeLanguage: vi.fn() },
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

// Conditionally mock MUI Button to allow clicking the webhook test button when disabled
vi.mock("@mui/material/Button", async (importOriginal) => {
  const original = await importOriginal<typeof import("@mui/material/Button")>();
  return {
    ...original,
    default: (props: any) => {
      if (props["data-testid"] === "settings-webhook-test") {
        return React.createElement("button", {
          "data-testid": props["data-testid"],
          onClick: props.onClick,
          type: props.type,
        }, props.children);
      }
      return React.createElement(original.default, props);
    }
  };
});

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

describe("SettingsPage page component", () => {
  beforeEach(() => {
    vi.mocked(toast.error).mockClear();
    vi.mocked(toast.success).mockClear();
    vi.mocked(toast.info).mockClear();
    vi.stubGlobal("confirm", vi.fn().mockReturnValue(true));
    mockRunWithStepUp.mockClear();
    mockRunWithStepUp.mockImplementation(async (action: any) => action());
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("renders loader or profiles correctly", async () => {
    // 1st: me is null
    vi.mocked(useMeContext).mockReturnValue(null);
    const { unmount } = render(<SettingsPage />);
    expect(screen.getByRole("progressbar")).toBeDefined();
    unmount();

    // 2nd: me is present
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: true,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      roles: ["admin"],
      locale: "en",
    });

    const getSettingsSpy = vi.spyOn(api.admin, "getSettings").mockResolvedValue({
      max_sessions_per_user: "5",
      password_complexity: "strong",
      global_mfa_required: "true",
      max_login_attempts: "5",
    });

    render(<SettingsPage />);
    expect(screen.getByDisplayValue("John")).toBeDefined();
    expect(screen.getByDisplayValue("Doe")).toBeDefined();

    await waitFor(() => {
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
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      roles: ["admin"],
      locale: "en",
    });

    vi.spyOn(api.admin, "getSettings").mockResolvedValue({});
    const updateSpy = vi.spyOn(api, "updateProfile").mockResolvedValue({
      id: "u123",
      first_name: "JohnUpdated",
      last_name: "DoeUpdated",
    } as any);

    render(<SettingsPage />);

    // Trigger form submit
    const saveBtn = screen.getByTestId("settings-profile-save");
    fireEvent.click(saveBtn);

    await waitFor(() => {
      expect(updateSpy).toHaveBeenCalledWith({ first_name: "John", last_name: "Doe" });
    });

    // Trigger update profile failure
    const updateFailSpy = vi.spyOn(api, "updateProfile").mockRejectedValue(new ApiError("invalid_value", undefined, 400));
    fireEvent.click(saveBtn);

    await waitFor(() => {
      expect(updateFailSpy).toHaveBeenCalled();
      expect(toast.error).toHaveBeenCalled();
    });
  });

  it("handles avatar change and delete paths", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: true,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      roles: ["admin"],
      locale: "en",
    });

    vi.spyOn(api.admin, "getSettings").mockResolvedValue({});
    const uploadSpy = vi.spyOn(api, "uploadAvatar").mockResolvedValue({} as any);
    const deleteSpy = vi.spyOn(api, "deleteAvatar").mockResolvedValue({} as any);

    render(<SettingsPage />);

    const fileInput = document.querySelector('input[type="file"]');
    expect(fileInput).toBeDefined();

    // Test Avatar Change Too Large
    fireEvent.change(fileInput!, { target: { files: [{ size: 3 * 1024 * 1024 }] } });
    expect(toast.error).toHaveBeenCalledWith("errors.file_too_large");

    // Test Avatar Change Success
    const file = new File(["avatar"], "avatar.png", { type: "image/png" });
    Object.defineProperty(file, "size", { value: 1024 });
    fireEvent.change(fileInput!, { target: { files: [file] } });
    await waitFor(() => {
      expect(uploadSpy).toHaveBeenCalledWith(file);
    });

    // Test Avatar Delete Success
    const deleteBtn = screen.getByText("deleteButton");
    fireEvent.click(deleteBtn);
    await waitFor(() => {
      expect(deleteSpy).toHaveBeenCalled();
    });
  });

  it("ignores avatar input changes when no file is selected", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: true,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      roles: ["admin"],
      locale: "en",
    });

    vi.spyOn(api.admin, "getSettings").mockResolvedValue({});
    const uploadSpy = vi.spyOn(api, "uploadAvatar").mockResolvedValue({} as any);

    render(<SettingsPage />);
    const fileInput = document.querySelector('input[type="file"]');
    expect(fileInput).toBeDefined();
    fireEvent.change(fileInput!, { target: { files: [] } });

    expect(uploadSpy).not.toHaveBeenCalled();
  });

  it("handles system settings save including step up mfa", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: true,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      roles: ["admin"],
      locale: "en",
    });

    const getSettingsSpy = vi.spyOn(api.admin, "getSettings").mockResolvedValue({
      max_sessions_per_user: "5",
      max_login_attempts: "5",
    });
    const updateSettingsSpy = vi.spyOn(api.admin, "updateSettings").mockResolvedValue({} as any);

    render(<SettingsPage />);
    await waitFor(() => {
      expect(getSettingsSpy).toHaveBeenCalled();
    });

    fireEvent.click(screen.getByTestId("tab-system-settings"));

    await waitFor(() => {
      expect(screen.queryByText("loading")).toBeNull();
    });

    const maxSessionsInput = screen.getByTestId("settings-max-sessions") as HTMLInputElement;
    expect(maxSessionsInput.value).toBe("5");

    const saveBtn = screen.getByTestId("settings-system-save");
    fireEvent.click(saveBtn);
    await waitFor(() => {
      expect(updateSettingsSpy).toHaveBeenCalled();
    });

    // Test invalid values (sessions out of range) - submit form directly to bypass native HTML5 validation constraints
    fireEvent.change(maxSessionsInput, { target: { value: "99" } });
    fireEvent.submit(saveBtn.closest("form")!);
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("errors.invalid_value");
    });

    // Test MFA Step-up: simulate useStepUp catching mfa_required and retrying
    fireEvent.change(maxSessionsInput, { target: { value: "5" } });
    vi.spyOn(api.admin, "updateSettings").mockRejectedValueOnce(new ApiError("mfa_required", undefined, 403));
    const stepUpSpy = vi.spyOn(api, "mfaStepUp").mockResolvedValue({} as any);
    mockRunWithStepUp.mockImplementationOnce(async (action: any) => {
      try { return await action(); }
      catch { await api.mfaStepUp("123456"); return action(); }
    });

    fireEvent.click(saveBtn);
    await waitFor(() => {
      expect(stepUpSpy).toHaveBeenCalledWith("123456");
    });
  });

  it("handles system settings load and save failure branches", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: true,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      roles: ["admin"],
      locale: "en",
    });

    vi.spyOn(api.admin, "getSettings").mockRejectedValue(new Error("settings unavailable"));
    render(<SettingsPage />);

    await waitFor(() => {
      expect(toast.error).toHaveBeenLastCalledWith("errors.internal_error");
    });

    cleanup();
    vi.mocked(toast.error).mockClear();

    vi.spyOn(api.admin, "getSettings").mockResolvedValue({
      max_sessions_per_user: "5",
      max_login_attempts: "5",
    });
    render(<SettingsPage />);
    await waitFor(() => {
      expect(screen.getByTestId("tab-system-settings")).toBeDefined();
    });

    fireEvent.click(screen.getByTestId("tab-system-settings"));
    
    await waitFor(() => {
      expect(screen.queryByText("loading")).toBeNull();
    });

    const maxLoginAttemptsInput = screen.getByTestId("settings-max-login-attempts");

    // Invalid max login attempts - submit form directly to bypass native HTML5 validation constraints
    fireEvent.change(maxLoginAttemptsInput, { target: { value: "99" } });
    const saveBtn = screen.getByTestId("settings-system-save");
    fireEvent.submit(saveBtn.closest("form")!);
    await waitFor(() => {
      expect(toast.error).toHaveBeenLastCalledWith("errors.invalid_value");
    });

    // Reset and mock updateSettings failure
    vi.mocked(toast.error).mockClear();
    fireEvent.change(maxLoginAttemptsInput, { target: { value: "5" } });
    vi.spyOn(api.admin, "updateSettings").mockRejectedValueOnce(new Error("save failed"));
    fireEvent.click(saveBtn);
    await waitFor(() => {
      expect(toast.error).toHaveBeenLastCalledWith("errors.internal_error");
    });
  });

  it("does not complete system save when step-up MFA prompt is empty", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: true,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      roles: ["admin"],
      locale: "en",
    });

    vi.spyOn(api.admin, "getSettings").mockResolvedValue({
      max_sessions_per_user: "5",
      max_login_attempts: "5",
    });
    const updateSpy = vi.spyOn(api.admin, "updateSettings").mockRejectedValue(new ApiError("mfa_required", undefined, 403));
    const stepUpSpy = vi.spyOn(api, "mfaStepUp").mockResolvedValue({} as any);
    mockRunWithStepUp.mockImplementationOnce(async (action: any) => {
      return action();
    });

    render(<SettingsPage />);
    await waitFor(() => {
      expect(screen.getByTestId("tab-system-settings")).toBeDefined();
    });

    fireEvent.click(screen.getByTestId("tab-system-settings"));
    
    await waitFor(() => {
      expect(screen.queryByText("loading")).toBeNull();
    });

    const saveBtn = screen.getByTestId("settings-system-save");

    fireEvent.click(saveBtn);
    await waitFor(() => {
      expect(updateSpy).toHaveBeenCalledTimes(1);
      expect(stepUpSpy).not.toHaveBeenCalled();
    });
  });

  it("triggers interactive form and tab controls", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: true,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      roles: ["admin"],
      locale: "en",
    });

    vi.spyOn(api.admin, "getSettings").mockResolvedValue({
      max_sessions_per_user: "5",
      max_login_attempts: "5",
      global_mfa_required: "false",
    });

    render(<SettingsPage />);

    // Click tab security
    fireEvent.click(screen.getByTestId("tab-security-sessions"));
    expect(screen.getByText("SessionsPageMock")).toBeDefined();

    // Click tab system
    fireEvent.click(screen.getByTestId("tab-system-settings"));

    await waitFor(() => {
      expect(screen.queryByText("loading")).toBeNull();
    });

    // Click tab profile
    fireEvent.click(screen.getByTestId("tab-profile-settings"));

    const firstNameInput = screen.getByTestId("settings-first-name") as HTMLInputElement;
    const lastNameInput = screen.getByTestId("settings-last-name") as HTMLInputElement;
    fireEvent.change(firstNameInput, { target: { value: "JohnNew" } });
    fireEvent.change(lastNameInput, { target: { value: "DoeNew" } });

    expect(firstNameInput.value).toBe("JohnNew");
    expect(lastNameInput.value).toBe("DoeNew");

    // Click tab system again
    fireEvent.click(screen.getByTestId("tab-system-settings"));

    await waitFor(() => {
      expect(screen.queryByText("loading")).toBeNull();
    });

    const maxSessionsInput = screen.getByTestId("settings-max-sessions") as HTMLInputElement;
    fireEvent.change(maxSessionsInput, { target: { value: "8" } });
    expect(maxSessionsInput.value).toBe("8");

    const switches = screen.getAllByRole("switch");
    const mfaSwitch = switches[0];
    fireEvent.click(mfaSwitch);
    fireEvent.click(mfaSwitch);
  });

  it("renders email initials, no-avatar profile, and the security tab", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "fallback@example.com",
      first_name: undefined,
      last_name: undefined,
      has_avatar: false,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      roles: ["user"],
      locale: "en",
    } as any);
    const getSettingsSpy = vi.spyOn(api.admin, "getSettings").mockResolvedValue({});

    render(<SettingsPage />);
    expect(screen.getByText("FA")).toBeDefined();

    const avatarImg = document.querySelector('img[src*="/avatar"]');
    expect(avatarImg).toBeNull();
    expect(getSettingsSpy).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTestId("tab-security-sessions"));
    expect(screen.getByText("SessionsPageMock")).toBeDefined();
  });

  it("renders saving labels and shows success toasts on save", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: true,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      roles: ["admin"],
      locale: "en",
    });
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({
      max_sessions_per_user: "5",
      max_login_attempts: "5",
    });

    let resolveProfile: any;
    const profilePromise = new Promise((resolve) => {
      resolveProfile = resolve;
    });
    vi.spyOn(api, "updateProfile").mockReturnValue(profilePromise as any);

    render(<SettingsPage />);

    const profileSaveBtn = screen.getByTestId("settings-profile-save");
    fireEvent.click(profileSaveBtn);

    expect(screen.getByText("saving")).toBeDefined();

    resolveProfile({});
    await waitFor(() => {
      expect(toast.success).toHaveBeenCalledWith("saved");
    });

    fireEvent.click(screen.getByTestId("tab-system-settings"));
    
    await waitFor(() => {
      expect(screen.queryByText("loading")).toBeNull();
    });

    const systemSaveBtn = screen.getByTestId("settings-system-save");

    let resolveSystem: any;
    const systemPromise = new Promise((resolve) => {
      resolveSystem = resolve;
    });
    vi.spyOn(api.admin, "updateSettings").mockReturnValue(systemPromise as any);

    fireEvent.click(systemSaveBtn);
    expect(screen.getByText("saving")).toBeDefined();

    resolveSystem({});
    await waitFor(() => {
      expect(toast.success).toHaveBeenCalledWith("saved");
    });
  });

  it("handles avatar ApiError failures", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: true,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      roles: ["admin"],
      locale: "en",
    });
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({});
    vi.spyOn(api, "uploadAvatar").mockRejectedValue(new ApiError("invalid_value", undefined, 400));
    vi.spyOn(api, "deleteAvatar").mockRejectedValue(new ApiError("invalid_value", undefined, 400));

    render(<SettingsPage />);

    const fileInput = document.querySelector('input[type="file"]');
    expect(fileInput).toBeDefined();

    fireEvent.change(fileInput!, { target: { files: [{ size: 1024 }] } });
    await waitFor(() => {
      expect(toast.error).toHaveBeenLastCalledWith("errors.invalid_value");
    });

    const deleteBtn = screen.getByText("deleteButton");
    fireEvent.click(deleteBtn);
    await waitFor(() => {
      expect(toast.error).toHaveBeenLastCalledWith("errors.invalid_value");
    });
  });

  it("handles nonnumeric login attempts and undefined MFA prompt returns", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: true,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      roles: ["admin"],
      locale: "en",
    });
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({
      max_sessions_per_user: "5",
      max_login_attempts: "5",
    });
    const updateSpy = vi.spyOn(api.admin, "updateSettings").mockRejectedValue(new ApiError("mfa_required", undefined, 403));
    const stepUpSpy = vi.spyOn(api, "mfaStepUp").mockResolvedValue({} as any);
    mockRunWithStepUp.mockImplementation(async (action: any) => action());

    render(<SettingsPage />);
    await waitFor(() => {
      expect(screen.getByTestId("tab-system-settings")).toBeDefined();
    });

    fireEvent.click(screen.getByTestId("tab-system-settings"));

    await waitFor(() => {
      expect(screen.queryByText("loading")).toBeNull();
    });

    const maxLoginAttemptsInput = screen.getByTestId("settings-max-login-attempts");
    fireEvent.change(maxLoginAttemptsInput, { target: { value: "nope" } });

    const saveBtn = screen.getByTestId("settings-system-save");
    fireEvent.submit(saveBtn.closest("form")!);
    await waitFor(() => {
      expect(toast.error).toHaveBeenLastCalledWith("errors.invalid_value");
    });

    fireEvent.change(maxLoginAttemptsInput, { target: { value: "5" } });
    fireEvent.click(saveBtn);
    await waitFor(() => {
      expect(updateSpy).toHaveBeenCalledTimes(1);
      expect(stepUpSpy).not.toHaveBeenCalled();
    });
  });

  it("shows a success toast after a profile save", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: true,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      roles: ["admin"],
      locale: "en",
    });
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({});
    const updateProfileSpy = vi.spyOn(api, "updateProfile").mockResolvedValue({} as any);

    render(<SettingsPage />);
    const profileSaveBtn = screen.getByTestId("settings-profile-save");
    fireEvent.click(profileSaveBtn);

    await waitFor(() => {
      expect(updateProfileSpy).toHaveBeenCalled();
      expect(toast.success).toHaveBeenCalledWith("saved");
    });
  });

  it("shows error when new passwords do not match", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: false,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      roles: ["user"],
      locale: "en",
    } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({});

    render(<SettingsPage />);

    fireEvent.change(screen.getByTestId("settings-new-password"), { target: { value: "NewPass1!" } });
    fireEvent.change(screen.getByTestId("settings-confirm-password"), { target: { value: "DifferentPass1!" } });

    fireEvent.click(screen.getByRole("button", { name: "changePassword.submit" }));

    await waitFor(() => {
      expect(toast.error).toHaveBeenLastCalledWith("changePassword.errors.passwords_do_not_match");
    });
  });

  it("handles password change success", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: false,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      roles: ["user"],
      locale: "en",
    } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({});
    const changePasswordSpy = vi.spyOn(api, "changePassword").mockResolvedValue(undefined);

    render(<SettingsPage />);

    fireEvent.change(screen.getByTestId("settings-current-password"), { target: { value: "Current1!" } });
    fireEvent.change(screen.getByTestId("settings-new-password"), { target: { value: "NewPass1!" } });
    fireEvent.change(screen.getByTestId("settings-confirm-password"), { target: { value: "NewPass1!" } });

    fireEvent.click(screen.getByRole("button", { name: "changePassword.submit" }));

    await waitFor(() => {
      expect(changePasswordSpy).toHaveBeenCalledWith("Current1!", "NewPass1!");
      expect(toast.success).toHaveBeenCalledWith("changePassword.success");
    });
  });

  it("handles password change api errors", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: false,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      roles: ["user"],
      locale: "en",
    } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({});

    render(<SettingsPage />);

    fireEvent.change(screen.getByTestId("settings-current-password"), { target: { value: "WrongCurrent!" } });
    fireEvent.change(screen.getByTestId("settings-new-password"), { target: { value: "NewPass1!" } });
    fireEvent.change(screen.getByTestId("settings-confirm-password"), { target: { value: "NewPass1!" } });

    // wrong_password error
    vi.spyOn(api, "changePassword").mockRejectedValueOnce(new ApiError("wrong_password", undefined, 401));
    fireEvent.click(screen.getByRole("button", { name: "changePassword.submit" }));

    await waitFor(() => {
      expect(toast.error).toHaveBeenLastCalledWith("changePassword.errors.wrong_password");
    });

    // generic error
    vi.spyOn(api, "changePassword").mockRejectedValueOnce(new Error("network"));
    fireEvent.click(screen.getByRole("button", { name: "changePassword.submit" }));

    await waitFor(() => {
      expect(toast.error).toHaveBeenLastCalledWith("changePassword.errors.internal_error");
    });
  });

  it("renders changing label while password change is in progress", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: false,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      roles: ["user"],
      locale: "en",
    } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({});

    vi.spyOn(api, "changePassword").mockReturnValue(new Promise(() => {}));

    render(<SettingsPage />);

    fireEvent.change(screen.getByTestId("settings-current-password"), { target: { value: "Current1!" } });
    fireEvent.change(screen.getByTestId("settings-new-password"), { target: { value: "NewPass1!" } });
    fireEvent.change(screen.getByTestId("settings-confirm-password"), { target: { value: "NewPass1!" } });

    fireEvent.click(screen.getByRole("button", { name: "changePassword.submit" }));

    await waitFor(() => {
      expect(screen.getByText("changePassword.submitting")).toBeDefined();
    });
  });

  it("renders activity tab for admin at index 3 and calls listMyAudit", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123", email: "test@example.com", first_name: "John", last_name: "Doe",
      has_avatar: false, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
      roles: ["admin"], locale: "en",
    } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({});
    const auditSpy = vi.spyOn(api, "listMyAudit").mockResolvedValue({ data: [], total: 0 });

    render(<SettingsPage />);
    fireEvent.click(screen.getByTestId("tab-login-activity"));

    await waitFor(() => {
      expect(auditSpy).toHaveBeenCalledWith(25, 0);
    });
  });

  it("renders activity tab for non-admin at index 2 and calls listMyAudit", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u456", email: "user@example.com", first_name: "", last_name: "",
      has_avatar: false, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
      roles: ["user"], locale: "en",
    } as any);
    const auditSpy = vi.spyOn(api, "listMyAudit").mockResolvedValue({ data: [], total: 0 });

    render(<SettingsPage />);
    fireEvent.click(screen.getByTestId("tab-login-activity"));

    await waitFor(() => {
      expect(auditSpy).toHaveBeenCalledWith(25, 0);
    });
  });

  it("renders empty state when no activity entries", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u456", email: "user@example.com", first_name: "", last_name: "",
      has_avatar: false, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
      roles: ["user"], locale: "en",
    } as any);
    vi.spyOn(api, "listMyAudit").mockResolvedValue({ data: [], total: 0 });

    render(<SettingsPage />);
    fireEvent.click(screen.getByTestId("tab-login-activity"));

    await waitFor(() => {
      expect(screen.getByText("activityTitle")).toBeDefined();
      expect(screen.getByText("activityEmpty")).toBeDefined();
    });
  });

  it("renders loading spinner on activity tab while fetching", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u456", email: "user@example.com", first_name: "", last_name: "",
      has_avatar: false, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
      roles: ["user"], locale: "en",
    } as any);
    vi.spyOn(api, "listMyAudit").mockReturnValue(new Promise(() => {}));

    render(<SettingsPage />);
    fireEvent.click(screen.getByTestId("tab-login-activity"));

    await waitFor(() => {
      expect(screen.getByText("activityTitle")).toBeDefined();
      expect(screen.getByRole("progressbar")).toBeDefined();
    });
  });

  it("renders history entries with action, outcome, and location", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u456", email: "user@example.com", first_name: "", last_name: "",
      has_avatar: false, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
      roles: ["user"], locale: "en",
    } as any);

    const entries = [
      {
        id: "e1", user_id: "u456", user_email: "user@example.com",
        action: "auth.login", resource: "session", ip_address: "1.2.3.4",
        user_agent: "Chrome/120",
        metadata: { outcome: "success", location: { city: "Istanbul", country: "TR" }, client_info: { browser: "Chrome", os: "Windows" } },
        created_at: "2026-06-20T10:00:00Z",
      },
      {
        id: "e2", user_id: "u456", user_email: null,
        action: "user.password_changed", resource: "user", ip_address: null,
        user_agent: null,
        metadata: { outcome: "failure" },
        created_at: "2026-06-20T09:00:00Z",
      },
    ];

    vi.spyOn(api, "listMyAudit").mockResolvedValue({ data: entries, total: 2 });

    render(<SettingsPage />);
    fireEvent.click(screen.getByTestId("tab-login-activity"));

    await waitFor(() => {
      expect(screen.getByText("Login")).toBeDefined();
      expect(screen.getByText("Password Changed")).toBeDefined();
      expect(screen.getByText("Istanbul, TR")).toBeDefined();
      expect(screen.getByText("success")).toBeDefined();
      expect(screen.getByText("failure")).toBeDefined();
    });
  });

  it("shows pagination controls when historyTotal > 25", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u456", email: "user@example.com", first_name: "", last_name: "",
      has_avatar: false, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
      roles: ["user"], locale: "en",
    } as any);
    vi.spyOn(api, "listMyAudit").mockResolvedValue({
      data: [{ id: "x", action: "auth.login", resource: "", user_id: null, user_email: null, ip_address: null, user_agent: null, metadata: null, created_at: "2026-06-20T00:00:00Z" }],
      total: 50
    });

    render(<SettingsPage />);
    fireEvent.click(screen.getByTestId("tab-login-activity"));

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "activityPrev" })).toBeDefined();
      expect(screen.getByRole("button", { name: "activityNext" })).toBeDefined();
    });
  });

  it("does not call listMyAudit when not on activity tab", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u456", email: "user@example.com", first_name: "", last_name: "",
      has_avatar: false, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
      roles: ["user"], locale: "en",
    } as any);
    const auditSpy = vi.spyOn(api, "listMyAudit").mockResolvedValue({ data: [], total: 0 });

    render(<SettingsPage />);
    expect(auditSpy).not.toHaveBeenCalled();
  });

  it("calls testWebhook and shows success toast on delivery", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123", email: "admin@example.com", first_name: "Admin", last_name: "User",
      has_avatar: false, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
      roles: ["admin"], locale: "en",
    } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({
      webhook_enabled: "true",
      webhook_url: "https://hooks.slack.com/services/test",
    });
    const testSpy = vi.spyOn(api.admin, "testWebhook").mockResolvedValue(undefined);

    render(<SettingsPage />);
    await waitFor(() => {
      expect(screen.getByTestId("tab-system-settings")).toBeDefined();
    });

    fireEvent.click(screen.getByTestId("tab-system-settings"));
    
    await waitFor(() => {
      expect(screen.queryByText("loading")).toBeNull();
    });

    const testBtn = screen.getByTestId("settings-webhook-test");
    fireEvent.click(testBtn);

    await waitFor(() => {
      expect(testSpy).toHaveBeenCalledWith("https://hooks.slack.com/services/test");
      expect(toast.success).toHaveBeenCalledWith("webhookTestSuccess");
    });
  });

  it("shows error toast when testWebhook delivery fails", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123", email: "admin@example.com", first_name: "Admin", last_name: "User",
      has_avatar: false, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
      roles: ["admin"], locale: "en",
    } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({
      webhook_enabled: "true",
      webhook_url: "https://hooks.slack.com/services/test",
    });
    vi.spyOn(api.admin, "testWebhook").mockRejectedValue(new ApiError("webhook_delivery_failed"));

    render(<SettingsPage />);
    await waitFor(() => {
      expect(screen.getByTestId("tab-system-settings")).toBeDefined();
    });

    fireEvent.click(screen.getByTestId("tab-system-settings"));
    
    await waitFor(() => {
      expect(screen.queryByText("loading")).toBeNull();
    });

    const testBtn = screen.getByTestId("settings-webhook-test");
    fireEvent.click(testBtn);

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("webhookTestFailed");
    });
  });

  it("shows info toast and skips API call when testWebhook URL is empty", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123", email: "admin@example.com", first_name: "Admin", last_name: "User",
      has_avatar: false, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
      roles: ["admin"], locale: "en",
    } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({
      webhook_enabled: "true",
      webhook_url: "",
    });
    const testSpy = vi.spyOn(api.admin, "testWebhook");

    render(<SettingsPage />);
    await waitFor(() => {
      expect(screen.getByTestId("tab-system-settings")).toBeDefined();
    });

    fireEvent.click(screen.getByTestId("tab-system-settings"));
    
    await waitFor(() => {
      expect(screen.queryByText("loading")).toBeNull();
    });

    const testBtn = screen.getByTestId("settings-webhook-test") as HTMLButtonElement;
    
    // Fire a click directly on the conditionally mocked button (which ignores disabled state)
    fireEvent.click(testBtn);

    await waitFor(() => {
      expect(testSpy).not.toHaveBeenCalled();
      expect(toast.info).toHaveBeenCalledWith("webhookTestNoUrl");
    });
  });

  it("initializes ip_allowlist and country_allowlist from settings API response", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123", email: "admin@example.com", first_name: "Admin", last_name: "User",
      has_avatar: false, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
      roles: ["admin"], locale: "en",
    } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({
      ip_allowlist: "10.0.0.0/8,192.168.1.1",
      country_allowlist: "TR,US",
      webhook_url: "https://hooks.slack.com/services/xyz",
      webhook_enabled: "true",
    });

    render(<SettingsPage />);
    await waitFor(() => {
      expect(screen.getByTestId("tab-system-settings")).toBeDefined();
    });

    fireEvent.click(screen.getByTestId("tab-system-settings"));
    
    await waitFor(() => {
      expect(screen.queryByText("loading")).toBeNull();
    });

    expect((screen.getByTestId("settings-ip-allowlist") as HTMLInputElement).value).toBe("10.0.0.0/8,192.168.1.1");
    expect((screen.getByTestId("settings-country-allowlist") as HTMLInputElement).value).toBe("TR,US");
    expect((screen.getByTestId("settings-webhook-url") as HTMLInputElement).value).toBe("https://hooks.slack.com/services/xyz");
  });

  it("includes ip_allowlist and country_allowlist in system settings save payload", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123", email: "admin@example.com", first_name: "Admin", last_name: "User",
      has_avatar: false, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
      roles: ["admin"], locale: "en",
    });
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({
      max_sessions_per_user: "5",
      max_login_attempts: "5",
    });
    const updateSpy = vi.spyOn(api.admin, "updateSettings").mockResolvedValue({} as any);

    render(<SettingsPage />);
    await waitFor(() => {
      expect(screen.getByTestId("tab-system-settings")).toBeDefined();
    });

    fireEvent.click(screen.getByTestId("tab-system-settings"));
    
    await waitFor(() => {
      expect(screen.queryByText("loading")).toBeNull();
    });

    fireEvent.change(screen.getByTestId("settings-ip-allowlist"), { target: { value: "10.0.0.1,172.16.0.0/12" } });
    fireEvent.change(screen.getByTestId("settings-country-allowlist"), { target: { value: "TR,DE" } });

    const saveBtn = screen.getByTestId("settings-system-save");
    fireEvent.click(saveBtn);

    await waitFor(() => {
      expect(updateSpy).toHaveBeenCalledWith(expect.objectContaining({
        ip_allowlist: "10.0.0.1,172.16.0.0/12",
        country_allowlist: "TR,DE",
      }));
    });
  });

  it("initializes risk-based auth settings and includes them in the save payload", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123", email: "admin@example.com", first_name: "Admin", last_name: "User",
      has_avatar: false, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
      roles: ["admin"], locale: "en",
    } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({
      max_sessions_per_user: "5",
      max_login_attempts: "5",
      risk_based_auth_enabled: "true",
      risk_threshold_mfa: "45",
      risk_threshold_block: "85",
    });
    const updateSpy = vi.spyOn(api.admin, "updateSettings").mockResolvedValue({} as any);

    render(<SettingsPage />);
    await waitFor(() => {
      expect(screen.getByTestId("tab-system-settings")).toBeDefined();
    });

    fireEvent.click(screen.getByTestId("tab-system-settings"));
    
    await waitFor(() => {
      expect(screen.queryByText("loading")).toBeNull();
    });

    expect((screen.getByTestId("settings-risk-threshold-mfa") as HTMLInputElement).value).toBe("45");
    expect((screen.getByTestId("settings-risk-threshold-block") as HTMLInputElement).value).toBe("85");

    const saveBtn = screen.getByTestId("settings-system-save");
    fireEvent.click(saveBtn);

    await waitFor(() => {
      expect(updateSpy).toHaveBeenCalledWith(expect.objectContaining({
        risk_based_auth_enabled: "true",
        risk_threshold_mfa: "45",
        risk_threshold_block: "85",
      }));
    });
  });

  it("initializes advanced risk-based tuning settings and saves updated values", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123", email: "admin@example.com", first_name: "Admin", last_name: "User",
      has_avatar: false, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
      roles: ["admin"], locale: "en",
    } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({
      max_sessions_per_user: "5",
      max_login_attempts: "5",
      risk_based_auth_enabled: "true",
      risk_threshold_mfa: "45",
      risk_threshold_block: "85",
      risk_score_impossible_travel: "75",
      risk_score_new_device: "25",
      risk_score_suspicious_hours: "15",
      risk_score_failed_attempt: "10",
      risk_failed_attempt_max_score: "35",
      risk_suspicious_hour_start: "22",
      risk_suspicious_hour_end: "6",
      risk_impossible_travel_velocity_kmh: "700",
      risk_impossible_travel_window_hours: "12",
      risk_impossible_travel_min_distance_km: "8",
    });
    const updateSpy = vi.spyOn(api.admin, "updateSettings").mockResolvedValue({} as any);

    render(<SettingsPage />);
    await waitFor(() => {
      expect(screen.getByTestId("tab-system-settings")).toBeDefined();
    });

    fireEvent.click(screen.getByTestId("tab-system-settings"));
    
    await waitFor(() => {
      expect(screen.queryByText("loading")).toBeNull();
    });

    // Check initialized values
    expect((screen.getByTestId("settings-risk-score-impossible-travel") as HTMLInputElement).value).toBe("75");
    expect((screen.getByTestId("settings-risk-score-new-device") as HTMLInputElement).value).toBe("25");
    expect((screen.getByTestId("settings-risk-score-suspicious-hours") as HTMLInputElement).value).toBe("15");
    expect((screen.getByTestId("settings-risk-score-failed-attempt") as HTMLInputElement).value).toBe("10");
    expect((screen.getByTestId("settings-risk-failed-attempt-max-score") as HTMLInputElement).value).toBe("35");
    expect((screen.getByTestId("settings-risk-suspicious-hour-start") as HTMLInputElement).value).toBe("22");
    expect((screen.getByTestId("settings-risk-suspicious-hour-end") as HTMLInputElement).value).toBe("6");
    expect((screen.getByTestId("settings-risk-impossible-travel-velocity-kmh") as HTMLInputElement).value).toBe("700");
    expect((screen.getByTestId("settings-risk-impossible-travel-window-hours") as HTMLInputElement).value).toBe("12");
    expect((screen.getByTestId("settings-risk-impossible-travel-min-distance-km") as HTMLInputElement).value).toBe("8");

    // Modify values
    fireEvent.change(screen.getByTestId("settings-risk-score-impossible-travel"), { target: { value: "90" } });
    fireEvent.change(screen.getByTestId("settings-risk-score-new-device"), { target: { value: "35" } });
    fireEvent.change(screen.getByTestId("settings-risk-score-suspicious-hours"), { target: { value: "25" } });
    fireEvent.change(screen.getByTestId("settings-risk-score-failed-attempt"), { target: { value: "20" } });
    fireEvent.change(screen.getByTestId("settings-risk-failed-attempt-max-score"), { target: { value: "50" } });
    fireEvent.change(screen.getByTestId("settings-risk-suspicious-hour-start"), { target: { value: "20" } });
    fireEvent.change(screen.getByTestId("settings-risk-suspicious-hour-end"), { target: { value: "4" } });
    fireEvent.change(screen.getByTestId("settings-risk-impossible-travel-velocity-kmh"), { target: { value: "900" } });
    fireEvent.change(screen.getByTestId("settings-risk-impossible-travel-window-hours"), { target: { value: "18" } });
    fireEvent.change(screen.getByTestId("settings-risk-impossible-travel-min-distance-km"), { target: { value: "15" } });

    const saveBtn = screen.getByTestId("settings-system-save");
    fireEvent.click(saveBtn);

    await waitFor(() => {
      expect(updateSpy).toHaveBeenCalledWith(expect.objectContaining({
        risk_score_impossible_travel: "90",
        risk_score_new_device: "35",
        risk_score_suspicious_hours: "25",
        risk_score_failed_attempt: "20",
        risk_failed_attempt_max_score: "50",
        risk_suspicious_hour_start: "20",
        risk_suspicious_hour_end: "4",
        risk_impossible_travel_velocity_kmh: "900",
        risk_impossible_travel_window_hours: "18",
        risk_impossible_travel_min_distance_km: "15",
      }));
    });
  });

  it("shows error toast when new risk settings are out of valid bounds", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u123", email: "admin@example.com", first_name: "Admin", last_name: "User",
      has_avatar: false, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
      roles: ["admin"], locale: "en",
    } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({
      max_sessions_per_user: "5",
      max_login_attempts: "5",
      risk_based_auth_enabled: "true",
    });

    render(<SettingsPage />);
    await waitFor(() => {
      expect(screen.getByTestId("tab-system-settings")).toBeDefined();
    });

    fireEvent.click(screen.getByTestId("tab-system-settings"));
    
    await waitFor(() => {
      expect(screen.queryByText("loading")).toBeNull();
    });

    const saveBtn = screen.getByTestId("settings-system-save");

    // Invalid impossible travel score (> 100)
    fireEvent.change(screen.getByTestId("settings-risk-score-impossible-travel"), { target: { value: "101" } });
    fireEvent.submit(saveBtn.closest("form")!);
    await waitFor(() => {
      expect(toast.error).toHaveBeenLastCalledWith("errors.invalid_value");
    });
    fireEvent.change(screen.getByTestId("settings-risk-score-impossible-travel"), { target: { value: "80" } });

    // Invalid velocity (< 100)
    fireEvent.change(screen.getByTestId("settings-risk-impossible-travel-velocity-kmh"), { target: { value: "50" } });
    fireEvent.submit(saveBtn.closest("form")!);
    await waitFor(() => {
      expect(toast.error).toHaveBeenLastCalledWith("errors.invalid_value");
    });
  });
});
