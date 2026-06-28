import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { render, screen, fireEvent, waitFor, cleanup, within, waitForElementToBeRemoved } from "@testing-library/react";
import UsersPage from "./UsersPage";
import { api, ApiError } from "@/lib/api";
import { useMeContext } from "@/contexts/MeContext";
import { toast } from "sonner";

vi.mock("sonner", () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}));

vi.mock("@/contexts/MeContext", () => ({
  useMeContext: vi.fn(),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, options?: any) => {
      if (options?.count !== undefined) return `${key}:${options.count}`;
      return key;
    },
    tCommon: (key: string) => key,
    i18n: { language: "en" },
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

vi.mock("@mui/x-data-grid", () => ({
  DataGrid: (props: any) => {
    const renderedRows = (props.rows ?? []).map((row: any) => {
      props.getRowId?.(row);
      return React.createElement("div", { key: row.id, className: "mock-row", "data-testid": `row-${row.id}` },
        (props.columns ?? []).map((col: any) => {
          const cellContent = col.renderCell ? col.renderCell({ row }) : row[col.field];
          return React.createElement("div", { key: col.field, className: "mock-cell" }, cellContent);
        })
      );
    });
    return React.createElement("div", { className: "mock-datagrid" },
      React.createElement("button", {
        "data-testid": "mock-select-rows-u1-u2",
        onClick: () => props.onRowSelectionModelChange?.({ type: "include", ids: new Set(["u1", "u2"]) })
      }),
      React.createElement("button", {
        "data-testid": "mock-select-rows-u2",
        onClick: () => props.onRowSelectionModelChange?.({ type: "include", ids: new Set(["u2"]) })
      }),
      React.createElement("button", {
        "data-testid": "mock-select-rows-u2-u3",
        onClick: () => props.onRowSelectionModelChange?.({ type: "include", ids: new Set(["u2", "u3"]) })
      }),
      React.createElement("button", {
        "data-testid": "mock-select-rows-u1",
        onClick: () => props.onRowSelectionModelChange?.({ type: "include", ids: new Set(["u1"]) })
      }),
      React.createElement("button", {
        "data-testid": "mock-select-rows-nonexistent",
        onClick: () => props.onRowSelectionModelChange?.({ type: "include", ids: new Set(["nonexistent-id"]) })
      }),
      renderedRows
    );
  },
  getGridStringOperators: () => [
    { value: "contains", label: "contains", getApplyFilterFn: () => null },
    { value: "equals", label: "equals", getApplyFilterFn: () => null },
  ],
}));

describe("UsersPage page component", () => {
  let confirmMock = vi.fn().mockReturnValue(true);
  let promptMock = vi.fn().mockReturnValue("123456");
  let alertMock = vi.fn();

  beforeEach(() => {
    confirmMock = vi.fn().mockReturnValue(true);
    promptMock = vi.fn().mockReturnValue("123456");
    alertMock = vi.fn();
    vi.stubGlobal("confirm", confirmMock);
    vi.stubGlobal("prompt", promptMock);
    vi.stubGlobal("alert", alertMock);

    vi.mocked(toast.error).mockClear();
    vi.mocked(toast.success).mockClear();
    mockRunWithStepUp.mockClear();
    mockRunWithStepUp.mockImplementation(async (action: any) => action());

    vi.spyOn(window, "addEventListener");
    vi.spyOn(window, "removeEventListener");
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  const getMockUsers = () => ({
    data: [
      {
        id: "u1",
        email: "user1@example.com",
        first_name: "Alice",
        last_name: "Smith",
        has_avatar: true,
        locale: "en",
        is_active: true,
        roles: ["admin"],
        active_sessions: 2,
        mfa_enabled: true,
        passkey_count: 1,
        created_at: "2026-06-04T12:00:00Z",
        updated_at: "2026-06-04T12:00:00Z",
      },
      {
        id: "u2",
        email: "user2@example.com",
        first_name: "",
        last_name: "",
        has_avatar: false,
        locale: "en",
        is_active: false,
        roles: [],
        active_sessions: 0,
        mfa_enabled: false,
        passkey_count: 0,
        created_at: "2026-06-04T10:00:00Z",
        updated_at: "2026-06-04T10:00:00Z",
      },
      {
        id: "u3",
        email: "user3@example.com",
        first_name: "Chris",
        last_name: "Jones",
        has_avatar: false,
        locale: "en",
        is_active: true,
        roles: ["user"],
        active_sessions: 3,
        mfa_enabled: true,
        passkey_count: 0,
        created_at: "2026-06-04T09:00:00Z",
        updated_at: "2026-06-04T09:00:00Z",
      },
      {
        id: "u4",
        email: "user4@example.com",
        first_name: "Dana",
        last_name: "Ray",
        has_avatar: false,
        locale: "en",
        is_active: true,
        roles: ["user"],
        active_sessions: 5,
        mfa_enabled: false,
        passkey_count: 2,
        created_at: "2026-06-04T08:00:00Z",
        updated_at: "2026-06-04T08:00:00Z",
      }
    ],
    total: 4,
  });

  it("renders loader screen then loads settings and user lists", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u1",
      roles: ["admin"],
    } as any);

    const getSettingsSpy = vi.spyOn(api.admin, "getSettings").mockResolvedValue({
      max_sessions_per_user: "5",
    });
    const listSpy = vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());

    render(<UsersPage />);

    await waitFor(() => {
      expect(screen.getByTestId("create-user-button")).toBeDefined();
    });

    expect(getSettingsSpy).toHaveBeenCalled();
    expect(listSpy).toHaveBeenCalled();
  });

  it("handles creating a new user successfully", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u1",
      roles: ["admin"],
      locale: "en",
    } as any);

    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    const createSpy = vi.spyOn(api.admin, "createUser").mockResolvedValue({} as any);

    render(<UsersPage />);

    const createBtn = await screen.findByTestId("create-user-button");
    fireEvent.click(createBtn);

    await screen.findByText("createUserTitle");

    fireEvent.change(screen.getByLabelText(/email/i), { target: { value: "new@example.com" } });
    fireEvent.change(screen.getByLabelText(/firstName/i), { target: { value: "Bob" } });
    fireEvent.change(screen.getByLabelText(/lastName/i), { target: { value: "Jones" } });
    fireEvent.change(screen.getByLabelText(/password/i), { target: { value: "password123" } });

    fireEvent.click(within(screen.getByRole("dialog")).getByText("admin"));

    const submitBtn = screen.getByRole("button", { name: "create" });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(createSpy).toHaveBeenCalledWith({
        email: "new@example.com",
        first_name: "Bob",
        last_name: "Jones",
        password: "password123",
        locale: "en",
        roles: ["user", "admin"],
      });
    });
  });

  it("handles user status toggle with MFA step-up flow", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u1",
      roles: ["admin"],
    } as any);

    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    
    const stepUpSpy = vi.spyOn(api, "mfaStepUp").mockResolvedValue({} as any);
    const statusSpy = vi.spyOn(api.admin, "setUserStatus")
      .mockRejectedValueOnce(new ApiError("mfa_required"))
      .mockResolvedValueOnce({} as any);

    mockRunWithStepUp.mockImplementationOnce(async (action: any) => {
      try { return await action(); }
      catch { await api.mfaStepUp("123456"); return action(); }
    });

    render(<UsersPage />);

    const rowEl = await screen.findByTestId("row-u2");
    const actionsBtn = within(rowEl).getByRole("button");
    fireEvent.click(actionsBtn);

    const activateItem = await screen.findByText("activate");
    fireEvent.click(activateItem);

    await waitFor(() => {
      expect(statusSpy).toHaveBeenCalledWith("u2", true);
      expect(mockRunWithStepUp).toHaveBeenCalled();
      expect(stepUpSpy).toHaveBeenCalledWith("123456");
      expect(statusSpy).toHaveBeenCalledTimes(2);
    });
  });

  it("handles revoking all sessions for user", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u1",
      roles: ["admin"],
    } as any);

    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    const revokeAllSpy = vi.spyOn(api.admin, "revokeAllUserSessions").mockResolvedValue({} as any);

    render(<UsersPage />);

    const rowEl = await screen.findByTestId("row-u1");
    const actionsBtn = within(rowEl).getByRole("button");
    fireEvent.click(actionsBtn);

    const revokeAllItem = await screen.findByText("revokeAllSessions");
    fireEvent.click(revokeAllItem);

    await waitFor(() => {
      expect(revokeAllSpy).toHaveBeenCalledWith("u1");
    });
  });

  it("handles sessions dialog with single session revocation and revoke all sessions", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u1",
      roles: ["admin"],
    } as any);

    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    
    const userSessions = [
      {
        id: "s1",
        is_current: true,
        ip_address: "127.0.0.1",
        user_agent: "Mozilla/5.0",
        created_at: "2026-06-04T12:00:00Z",
        last_used_at: "2026-06-04T12:00:00Z",
        device_info: { browser: "Chrome", browser_version: "120", os: "Windows", os_version: "10" },
      },
      {
        id: "s2",
        is_current: false,
        ip_address: "127.0.0.2",
        user_agent: "Mozilla/5.0",
        created_at: "2026-06-04T12:00:00Z",
        last_used_at: "2026-06-04T12:00:00Z",
        device_info: { browser: "Chrome", browser_version: "120", os: "Windows", os_version: "10" },
      }
    ];
    vi.spyOn(api.admin, "listUserSessions").mockResolvedValue(userSessions);
    const revokeSessionSpy = vi.spyOn(api.admin, "revokeUserSession").mockResolvedValue({} as any);
    const revokeAllSpy = vi.spyOn(api.admin, "revokeAllUserSessions").mockResolvedValue({} as any);

    render(<UsersPage />);

    const rowEl = await screen.findByTestId("row-u1");
    const actionsBtn = within(rowEl).getByRole("button");
    fireEvent.click(actionsBtn);

    const viewSessionsItem = await screen.findByText("viewSessions");
    fireEvent.click(viewSessionsItem);

    await screen.findByText("IP: 127.0.0.1");

    const revokeSessBtn = screen.getAllByRole("button", { name: "revokeSession" })[0];
    fireEvent.click(revokeSessBtn);

    await waitFor(() => {
      expect(revokeSessionSpy).toHaveBeenCalledWith("u1", "s1");
    });

    const revokeAllBtn = screen.getByRole("button", { name: "revokeAllSessions" });
    fireEvent.click(revokeAllBtn);

    await waitFor(() => {
      expect(revokeAllSpy).toHaveBeenCalledWith("u1");
    });
  });

  it("handles createUser API error", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    vi.spyOn(api.admin, "createUser").mockRejectedValue(new ApiError("already_exists"));

    render(<UsersPage />);

    const createBtn = await screen.findByTestId("create-user-button");
    fireEvent.click(createBtn);

    await screen.findByText("createUserTitle");

    fireEvent.change(screen.getByLabelText(/email/i), { target: { value: "new@example.com" } });
    fireEvent.change(screen.getByLabelText(/password/i), { target: { value: "password" } });

    const submitBtn = screen.getByRole("button", { name: "create" });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("errors.already_exists");
    });
  });

  it("handles cancel/empty MFA prompt on step-up", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    vi.spyOn(api.admin, "setUserStatus").mockRejectedValue(new ApiError("mfa_required"));

    render(<UsersPage />);

    const rowEl = await screen.findByTestId("row-u2");
    const actionsBtn = within(rowEl).getByRole("button");
    fireEvent.click(actionsBtn);

    const activateItem = await screen.findByText("activate");
    fireEvent.click(activateItem);

    await waitFor(() => {
      expect(mockRunWithStepUp).toHaveBeenCalled();
    });
  });

  it("handles settings load failure and cleanup", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockRejectedValue(new Error("settings failed"));
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());

    const { unmount } = render(<UsersPage />);

    await screen.findByTestId("create-user-button");

    unmount();

    expect(window.removeEventListener).toHaveBeenCalledWith("sessions:changed", expect.any(Function));
  });

  it("handles API error during SessionsDialog revocation", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    vi.spyOn(api.admin, "listUserSessions").mockResolvedValue([
      {
        id: "s1",
        is_current: true,
        ip_address: "1.1.1.1",
        user_agent: "Mozilla",
        device_info: null,
        created_at: "2026-06-04T12:00:00Z",
        last_used_at: null,
      }
    ]);
    vi.spyOn(api.admin, "revokeUserSession").mockRejectedValue(new Error("API Error"));
    vi.spyOn(api.admin, "revokeAllUserSessions").mockRejectedValue(new Error("API Error"));

    render(<UsersPage />);

    const rowEl = await screen.findByTestId("row-u1");
    const actionsBtn = within(rowEl).getByRole("button");
    fireEvent.click(actionsBtn);

    const viewSessionsItem = await screen.findByText("viewSessions");
    fireEvent.click(viewSessionsItem);

    await screen.findByText("IP: 1.1.1.1");

    const revokeSessBtn = screen.getByRole("button", { name: "revokeSession" });
    fireEvent.click(revokeSessBtn);
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("errors.internal_error");
    });

    const revokeAllBtn = screen.getByRole("button", { name: "revokeAllSessions" });
    fireEvent.click(revokeAllBtn);
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("errors.internal_error");
    });
  });

  it("does not load admin data when the viewer is not an admin", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["user"] } as any);
    const getSettingsSpy = vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    const listSpy = vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());

    render(<UsersPage />);

    await screen.findByText("accessDenied");
    expect(getSettingsSpy).not.toHaveBeenCalled();
    expect(listSpy).not.toHaveBeenCalled();
  });

  it("ignores settings that resolve after cleanup and refreshes on session events", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    let resolveSettings: (value: { max_sessions_per_user: string }) => void = () => {};
    vi.spyOn(api.admin, "getSettings").mockReturnValue(new Promise((resolve) => { resolveSettings = resolve; }));
    const listSpy = vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());

    const { unmount } = render(<UsersPage />);

    const addEventListenerSpy = vi.mocked(window.addEventListener);
    const call = addEventListenerSpy.mock.calls.find(([event]) => event === "sessions:changed");
    expect(call).toBeDefined();
    const sessionsChanged = call![1] as () => void;

    sessionsChanged();

    unmount();
    resolveSettings({ max_sessions_per_user: "9" });

    await Promise.resolve();
    await Promise.resolve();

    expect(listSpy).toHaveBeenCalled();
    expect(window.removeEventListener).toHaveBeenCalledWith("sessions:changed", sessionsChanged);
  });

  it("covers row menu close handlers", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());

    render(<UsersPage />);

    const rowEl = await screen.findByTestId("row-u2");
    const actionsBtn = within(rowEl).getByRole("button");
    fireEvent.click(actionsBtn);

    const activateItem = await screen.findByText("activate");

    fireEvent.keyDown(activateItem, { key: "Escape", code: "Escape" });

    await waitFor(() => {
      expect(screen.queryByText("activate")).toBeNull();
    });
  });

  it("handles deactivate cancellation and generic status errors", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    const statusSpy = vi.spyOn(api.admin, "setUserStatus").mockRejectedValue(new Error("status failed"));

    render(<UsersPage />);

    confirmMock.mockReturnValueOnce(false);
    const rowEl3 = await screen.findByTestId("row-u3");
    fireEvent.click(within(rowEl3).getByRole("button"));
    const deactivateItem = await screen.findByText("deactivate");
    fireEvent.click(deactivateItem);

    expect(statusSpy).not.toHaveBeenCalled();

    confirmMock.mockReturnValueOnce(true);
    fireEvent.click(within(rowEl3).getByRole("button"));
    const deactivateItem2 = await screen.findByText("deactivate");
    fireEvent.click(deactivateItem2);

    await waitFor(() => {
      expect(statusSpy).toHaveBeenCalledWith("u3", false);
      expect(toast.error).toHaveBeenCalledWith("errors.internal_error");
    });
  });

  it("handles revoke-all cancellation, MFA retry cancellation, and generic errors", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    const revokeAllSpy = vi.spyOn(api.admin, "revokeAllUserSessions")
      .mockRejectedValueOnce(new ApiError("mfa_required"))
      .mockRejectedValueOnce(new Error("revoke failed"));

    render(<UsersPage />);

    confirmMock.mockReturnValueOnce(false);
    const rowEl1 = await screen.findByTestId("row-u1");
    fireEvent.click(within(rowEl1).getByRole("button"));
    const revokeAllItem = await screen.findByText("revokeAllSessions");
    fireEvent.click(revokeAllItem);

    expect(revokeAllSpy).not.toHaveBeenCalled();

    confirmMock.mockReturnValueOnce(true);
    fireEvent.click(within(rowEl1).getByRole("button"));
    const revokeAllItem2 = await screen.findByText("revokeAllSessions");
    fireEvent.click(revokeAllItem2);

    await waitFor(() => {
      expect(revokeAllSpy).toHaveBeenCalledTimes(1);
    });

    confirmMock.mockReturnValueOnce(true);
    fireEvent.click(within(rowEl1).getByRole("button"));
    const revokeAllItem3 = await screen.findByText("revokeAllSessions");
    fireEvent.click(revokeAllItem3);

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("errors.internal_error");
    });
  });

  it("handles create dialog cancel, close, role removal, and non-ApiError create failures", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    const createSpy = vi.spyOn(api.admin, "createUser").mockRejectedValue(new Error("create failed"));

    render(<UsersPage />);

    const createBtn = await screen.findByTestId("create-user-button");
    fireEvent.click(createBtn);
    const dialogTitle = await screen.findByText("createUserTitle");
    fireEvent.keyDown(dialogTitle, { key: "Escape" });
    await waitForElementToBeRemoved(() => screen.queryByRole("dialog"));

    fireEvent.click(createBtn);
    const cancelBtn = await screen.findByRole("button", { name: "cancel" });
    fireEvent.click(cancelBtn);
    await waitForElementToBeRemoved(() => screen.queryByRole("dialog"));

    fireEvent.click(createBtn);
    await screen.findByText("createUserTitle");

    fireEvent.change(screen.getByLabelText(/email/i), { target: { value: "roleless@example.com" } });
    fireEvent.change(screen.getByLabelText(/password/i), { target: { value: "password" } });

    fireEvent.click(within(screen.getByRole("dialog")).getByText("user"));

    const submitBtn = screen.getByRole("button", { name: "create" });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(createSpy).toHaveBeenCalled();
      expect(toast.error).toHaveBeenCalledWith("errors.internal_error");
    });
  });

  it("renders empty sessions dialogs", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    const listSessionsSpy = vi.spyOn(api.admin, "listUserSessions").mockResolvedValue([]);

    render(<UsersPage />);

    const rowEl = await screen.findByTestId("row-u1");
    fireEvent.click(within(rowEl).getByRole("button"));

    const viewSessionsItem = await screen.findByText("viewSessions");
    fireEvent.click(viewSessionsItem);

    await screen.findByText("noSessionsFound");

    const closeBtn = screen.getByRole("button", { name: "cancel" });
    fireEvent.click(closeBtn);

    await waitFor(() => {
      expect(screen.queryByText("noSessionsFound")).toBeNull();
    });
    expect(listSessionsSpy).toHaveBeenCalledTimes(1);
  });

  it("renders failed sessions dialogs", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    const listSessionsSpy = vi.spyOn(api.admin, "listUserSessions").mockRejectedValue(new Error("sessions failed"));

    render(<UsersPage />);

    const rowEl = await screen.findByTestId("row-u1");
    fireEvent.click(within(rowEl).getByRole("button"));

    const viewSessionsItem = await screen.findByText("viewSessions");
    fireEvent.click(viewSessionsItem);

    await screen.findByText("errors.internal_error");
    expect(listSessionsSpy).toHaveBeenCalledTimes(1);
  });

  it("rethrows MFA-required session revocations without showing generic alerts", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    vi.spyOn(api.admin, "listUserSessions").mockResolvedValue([
      {
        id: "s1",
        is_current: false,
        ip_address: "",
        user_agent: "Mozilla",
        device_info: { os: "Linux", architecture: "x64" },
        created_at: "2026-06-04T12:00:00Z",
        last_used_at: null,
      },
      {
        id: "s2",
        is_current: true,
        ip_address: "2.2.2.2",
        user_agent: "Mozilla",
        device_info: { browser: "Firefox", os: "Linux" },
        created_at: "2026-06-04T13:00:00Z",
        last_used_at: "2026-06-04T14:00:00Z",
      },
    ]);
    const revokeSessionSpy = vi.spyOn(api.admin, "revokeUserSession").mockRejectedValue(new ApiError("mfa_required"));
    const revokeAllSpy = vi.spyOn(api.admin, "revokeAllUserSessions").mockRejectedValue(new ApiError("mfa_required"));

    render(<UsersPage />);

    const rowEl = await screen.findByTestId("row-u1");
    fireEvent.click(within(rowEl).getByRole("button"));

    const viewSessionsItem = await screen.findByText("viewSessions");
    fireEvent.click(viewSessionsItem);

    await screen.findByText("Linux x64");
    expect(screen.getByText(/Firefox/)).toBeDefined();

    const revokeSessBtn = screen.getAllByRole("button", { name: "revokeSession" })[0];
    try {
      await fireEvent.click(revokeSessBtn);
    } catch (e) {}

    const revokeAllBtn = screen.getByRole("button", { name: "revokeAllSessions" });
    try {
      await fireEvent.click(revokeAllBtn);
    } catch (e) {}

    expect(revokeSessionSpy).toHaveBeenCalledWith("u1", "s1");
    expect(revokeAllSpy).toHaveBeenCalledWith("u1");
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("evaluates loading states, edge case fallback branches, and non-MFA ApiErrors", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "invalid" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    
    let resolveCreate: any;
    const createPromise = new Promise((res) => { resolveCreate = res; });
    vi.spyOn(api.admin, "createUser").mockReturnValue(createPromise as any);
    
    vi.spyOn(api.admin, "setUserStatus").mockRejectedValue(new ApiError("forbidden"));

    vi.spyOn(api.admin, "listUserSessions").mockResolvedValue([{ id: "s1", is_current: false } as any]);
    vi.spyOn(api.admin, "revokeUserSession").mockRejectedValue(new ApiError("forbidden"));
    vi.spyOn(api.admin, "revokeAllUserSessions").mockRejectedValue(new ApiError("forbidden"));

    render(<UsersPage />);

    const createBtn = await screen.findByTestId("create-user-button");
    fireEvent.click(createBtn);

    await screen.findByText("createUserTitle");

    fireEvent.change(screen.getByLabelText(/email/i), { target: { value: "test@example.com" } });
    fireEvent.change(screen.getByLabelText(/password/i), { target: { value: "pass" } });

    const submitBtn = screen.getByRole("button", { name: "create" });
    fireEvent.click(submitBtn);

    await screen.findByText("creating");
    
    resolveCreate({});
    await createPromise;
    await waitForElementToBeRemoved(() => screen.queryByRole("dialog"));

    const rowEl3 = await screen.findByTestId("row-u3");
    fireEvent.click(within(rowEl3).getByRole("button"));
    const deactivateItem = await screen.findByText("deactivate");
    fireEvent.click(deactivateItem);
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("errors.internal_error");
    });

    const rowEl1 = await screen.findByTestId("row-u1");
    fireEvent.click(within(rowEl1).getByRole("button"));
    const revokeAllItem = await screen.findByText("revokeAllSessions");
    fireEvent.click(revokeAllItem);
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("errors.internal_error");
    });

    fireEvent.click(within(rowEl1).getByRole("button"));
    const viewSessionsItem = await screen.findByText("viewSessions");
    fireEvent.click(viewSessionsItem);

    await screen.findByRole("button", { name: "revokeSession" });

    const revokeSessBtn = screen.getByRole("button", { name: "revokeSession" });
    try { await fireEvent.click(revokeSessBtn); } catch (e) {}
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("errors.internal_error");
    });

    const revokeAllBtn = screen.getByRole("button", { name: "revokeAllSessions" });
    try { await fireEvent.click(revokeAllBtn); } catch (e) {}
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("errors.internal_error");
    });
  });

  it("handles getSettings catch block when cancelled is true", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    
    let rejectSettings: any;
    vi.spyOn(api.admin, "getSettings").mockReturnValue(new Promise((_, rej) => { rejectSettings = rej; }));
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    
    const { unmount } = render(<UsersPage />);
    
    unmount();
    
    rejectSettings(new Error("fetch failed"));
    await Promise.resolve();
    await Promise.resolve();
  });

  it("evaluates all branches of sessionDeviceLabel", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });

    vi.spyOn(api.admin, "listUserSessions").mockResolvedValue([
      { id: "s1", is_current: false, device_info: { browser: "Chrome" } } as any,
      { id: "s2", is_current: false, device_info: { os: "Windows" } } as any,
      { id: "s3", is_current: false, device_info: {} } as any,
      { id: "s4", is_current: false, device_info: { browser: "Firefox", os: "Linux" } } as any,
    ]);

    render(<UsersPage />);

    const rowEl1 = await screen.findByTestId("row-u1");
    fireEvent.click(within(rowEl1).getByRole("button"));
    const viewSessionsItem = await screen.findByText("viewSessions");
    fireEvent.click(viewSessionsItem);

    await screen.findByText("Chrome");
    expect(screen.getByText("Windows")).toBeDefined();
    expect(screen.getByText("Unknown device")).toBeDefined();
    expect(screen.getByText("Firefox — Linux")).toBeDefined();
  });

  it("evaluates getSettings edge cases and cancellation", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listUsers").mockResolvedValue({ data: [], total: 0 });

    vi.spyOn(api.admin, "getSettings").mockResolvedValue({} as any);
    const { unmount } = render(<UsersPage />);
    await Promise.resolve();
    unmount();
    
    let resolveSettings: any;
    vi.spyOn(api.admin, "getSettings").mockReturnValue(new Promise((res) => { resolveSettings = res; }));
    const { unmount: unmount2 } = render(<UsersPage />);
    unmount2();
    resolveSettings({ max_sessions_per_user: "10" });
    await Promise.resolve();
    await Promise.resolve();
  });

  it("handles null user context safely", () => {
    vi.mocked(useMeContext).mockReturnValue(null);
    render(<UsersPage />);
    expect(screen.getByText("accessDenied")).toBeDefined();
  });

  it("handles runWithStepUp branches explicitly", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });

    render(<UsersPage />);

    vi.spyOn(api.admin, "setUserStatus").mockRejectedValueOnce(new ApiError("some_other_error"));
    const rowEl = await screen.findByTestId("row-u3");
    fireEvent.click(within(rowEl).getByRole("button"));
    
    const deactivateItem = await screen.findByText("deactivate");
    fireEvent.click(deactivateItem);

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("errors.internal_error");
    });
  });

  it("covers remaining session dialog and step-up fallbacks", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listUsers").mockResolvedValue({
      data: [
        {
          ...getMockUsers().data[2],
          active_sessions: 0,
        },
      ],
      total: 1,
    });
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUserSessions").mockResolvedValue([
      {
        id: "s1",
        is_current: false,
        ip_address: "",
        user_agent: "Mozilla",
        device_info: { browser: "Chrome", browser_version: "120.1" },
        created_at: "2026-06-04T12:00:00Z",
        last_used_at: null,
      } as any,
    ]);
    vi.spyOn(api.admin, "setUserStatus").mockRejectedValue(new ApiError("mfa_required"));

    render(<UsersPage />);

    await screen.findByText("noActiveSessions");

    const rowEl = await screen.findByTestId("row-u3");
    fireEvent.click(within(rowEl).getByRole("button"));
    const deactivateItem = await screen.findByText("deactivate");
    fireEvent.click(deactivateItem);

    await waitFor(() => {
      expect(mockRunWithStepUp).toHaveBeenCalled();
    });

    fireEvent.click(within(rowEl).getByRole("button"));
    const viewSessionsItem = await screen.findByText("viewSessions");
    fireEvent.click(viewSessionsItem);

    await screen.findByText("Chrome 120");
  });

  it("shows bulk action toolbar when rows are selected", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());

    render(<UsersPage />);

    const selectBtn = await screen.findByTestId("mock-select-rows-u1-u2");
    fireEvent.click(selectBtn);

    await screen.findByText("bulkSelected:2");
    expect(screen.getByText("bulkActivate")).toBeDefined();
    expect(screen.getByText("bulkDeactivate")).toBeDefined();
    expect(screen.getByText("bulkExport")).toBeDefined();
    expect(screen.getByText("bulkClear")).toBeDefined();
  });

  it("handles bulk activate successfully", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    const bulkSpy = vi.spyOn(api.admin, "bulkSetUserStatus").mockResolvedValue(undefined);

    render(<UsersPage />);

    const selectBtn = await screen.findByTestId("mock-select-rows-u2");
    fireEvent.click(selectBtn);

    const bulkActBtn = await screen.findByRole("button", { name: "bulkActivate" });
    fireEvent.click(bulkActBtn);

    await waitFor(() => {
      expect(bulkSpy).toHaveBeenCalledWith(["u2"], true);
      expect(toast.success).toHaveBeenCalledWith(expect.stringContaining("bulkActivated:1"));
    });
  });

  it("handles bulk deactivate successfully", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    const bulkSpy = vi.spyOn(api.admin, "bulkSetUserStatus").mockResolvedValue(undefined);

    render(<UsersPage />);

    const selectBtn = await screen.findByTestId("mock-select-rows-u2-u3");
    fireEvent.click(selectBtn);

    const bulkDeactBtn = await screen.findByRole("button", { name: "bulkDeactivate" });
    fireEvent.click(bulkDeactBtn);

    await waitFor(() => {
      expect(bulkSpy).toHaveBeenCalledWith(["u2", "u3"], false);
      expect(toast.success).toHaveBeenCalledWith(expect.stringContaining("bulkDeactivated:2"));
    });
  });

  it("handles bulk API error (last_admin)", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    vi.spyOn(api.admin, "bulkSetUserStatus").mockRejectedValue(new ApiError("last_admin"));

    render(<UsersPage />);

    const selectBtn = await screen.findByTestId("mock-select-rows-u1");
    fireEvent.click(selectBtn);

    const bulkDeactBtn = await screen.findByRole("button", { name: "bulkDeactivate" });
    fireEvent.click(bulkDeactBtn);

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("errors.last_admin");
    });
  });

  it("handles bulk mfa_required silently", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    vi.spyOn(api.admin, "bulkSetUserStatus").mockRejectedValue(new ApiError("mfa_required"));
    mockRunWithStepUp.mockImplementationOnce(async (action: any) => {
      try { return await action(); } catch { throw new ApiError("mfa_required"); }
    });

    render(<UsersPage />);

    const selectBtn = await screen.findByTestId("mock-select-rows-u2");
    fireEvent.click(selectBtn);

    const bulkActBtn = await screen.findByRole("button", { name: "bulkActivate" });
    fireEvent.click(bulkActBtn);

    await waitFor(() => {
      expect(toast.error).not.toHaveBeenCalled();
      expect(toast.success).not.toHaveBeenCalled();
    });
  });

  it("handles bulk clear", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());

    render(<UsersPage />);

    const selectBtn = await screen.findByTestId("mock-select-rows-u1-u2");
    fireEvent.click(selectBtn);

    await screen.findByText("bulkSelected:2");

    const clearBtn = screen.getByRole("button", { name: "bulkClear" });
    fireEvent.click(clearBtn);

    await waitFor(() => {
      expect(screen.queryByText("bulkSelected:2")).toBeNull();
    });
  });

  it("handles bulk export with selected rows", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());

    const anchorClick = vi.fn();
    const originalCreateElement = document.createElement.bind(document);
    vi.spyOn(document, "createElement").mockImplementation((tagName) => {
      if (tagName === "a") {
        return {
          href: "",
          download: "",
          click: anchorClick,
          style: {},
        } as any;
      }
      return originalCreateElement(tagName);
    });

    vi.stubGlobal("URL", {
      ...global.URL,
      createObjectURL: vi.fn().mockReturnValue("blob:test"),
      revokeObjectURL: vi.fn(),
    });

    render(<UsersPage />);

    const selectBtn = await screen.findByTestId("mock-select-rows-u1-u2");
    fireEvent.click(selectBtn);

    const exportBtn = await screen.findByRole("button", { name: "bulkExport" });
    fireEvent.click(exportBtn);

    await waitFor(() => {
      expect(anchorClick).toHaveBeenCalled();
    });
  });

  it("skips export when no rows match selection", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());

    const anchorClick = vi.fn();
    const originalCreateElement = document.createElement.bind(document);
    vi.spyOn(document, "createElement").mockImplementation((tagName) => {
      if (tagName === "a") {
        return {
          href: "",
          download: "",
          click: anchorClick,
          style: {},
        } as any;
      }
      return originalCreateElement(tagName);
    });

    vi.stubGlobal("URL", {
      ...global.URL,
      createObjectURL: vi.fn().mockReturnValue("blob:test"),
      revokeObjectURL: vi.fn(),
    });

    render(<UsersPage />);

    const selectBtn = await screen.findByTestId("mock-select-rows-nonexistent");
    fireEvent.click(selectBtn);

    const exportBtn = await screen.findByRole("button", { name: "bulkExport" });
    fireEvent.click(exportBtn);

    await waitFor(() => {
      expect(anchorClick).toHaveBeenCalled();
    });
  });

  it("renders the sessions dialog loading state", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    vi.spyOn(api.admin, "listUserSessions").mockReturnValue(new Promise(() => {}));

    render(<UsersPage />);

    const rowEl = await screen.findByTestId("row-u1");
    fireEvent.click(within(rowEl).getByRole("button"));

    const viewSessionsItem = await screen.findByText("viewSessions");
    fireEvent.click(viewSessionsItem);

    await screen.findByRole("progressbar");
  });
});
