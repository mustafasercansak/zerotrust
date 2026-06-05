import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import UsersPage from "./UsersPage";
import { api, ApiError } from "@/lib/api";
import { useMeContext } from "@/contexts/MeContext";
import { renderToString } from "react-dom/server";

// State Mocking System
let stateStore: any = {};
let stateSetters: any = {};
let callIdx = 0;
let effectCleanups: Array<() => void> = [];

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
      if (callIdx >= 50) {
        callIdx = 0;
      }
      return [stateStore[idx], stateSetters[idx]];
    },
    useEffect: (fn: any) => {
      const cleanup = fn();
      if (typeof cleanup === "function") effectCleanups.push(cleanup);
    },
  };
});

vi.mock("@/contexts/MeContext", () => ({
  useMeContext: vi.fn(),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: "en" },
  }),
}));

const capturedButtonClicks: any[] = [];
const capturedMenuItemClicks: any[] = [];
const capturedChipClicks: any[] = [];
const capturedIconButtonClicks: any[] = [];
const capturedSubmits: any[] = [];
const capturedInputs: any[] = [];
const capturedDialogCloses: any[] = [];
const capturedMenuCloses: any[] = [];
const capturedMenuClicks: any[] = [];

vi.mock("@mui/material/Button", () => ({
  default: (props: any) => {
    if (props.onClick) capturedButtonClicks.push(props.onClick);
    return React.createElement("button", { onClick: props.onClick, type: props.type, disabled: props.disabled }, props.children);
  }
}));

vi.mock("@mui/material/MenuItem", () => ({
  default: (props: any) => {
    if (props.onClick) capturedMenuItemClicks.push(props.onClick);
    return React.createElement("li", { onClick: props.onClick }, props.children);
  }
}));

vi.mock("@mui/material/Chip", () => ({
  default: (props: any) => {
    if (props.onClick) capturedChipClicks.push(props.onClick);
    return React.createElement("div", { onClick: props.onClick }, props.children ?? props.label);
  }
}));

vi.mock("@mui/material/IconButton", () => ({
  default: (props: any) => {
    if (props.onClick) capturedIconButtonClicks.push(props.onClick);
    return React.createElement("button", { onClick: props.onClick }, props.children);
  }
}));

vi.mock("@mui/material/Box", () => ({
  default: (props: any) => {
    if (props.onSubmit) capturedSubmits.push(props.onSubmit);
    return React.createElement("div", { onSubmit: props.onSubmit }, props.children);
  }
}));

vi.mock("@mui/material/TextField", () => ({
  default: (props: any) => {
    if (props.onChange) capturedInputs.push(props.onChange);
    return React.createElement("input", { type: props.type, value: props.value, onChange: props.onChange });
  }
}));

vi.mock("@mui/material/Dialog", () => ({
  default: (props: any) => {
    if (props.onClose) capturedDialogCloses.push(props.onClose);
    return props.open ? React.createElement("div", null, props.children) : null;
  }
}));

vi.mock("@mui/material/Menu", () => ({
  default: (props: any) => {
    if (props.onClose) capturedMenuCloses.push(props.onClose);
    if (props.onClick) capturedMenuClicks.push(props.onClick);
    return props.open ? React.createElement("div", null, props.children) : null;
  }
}));

vi.mock("@mui/material/Tooltip", () => ({
  default: (props: any) => props.children
}));

vi.mock("@mui/x-data-grid", () => ({
  DataGrid: (props: any) => {
    const renderedRows = (props.rows ?? []).map((row: any) => {
      props.getRowId?.(row);
      return React.createElement("div", { key: row.id, className: "mock-row" },
        (props.columns ?? []).map((col: any) => {
          const cellContent = col.renderCell ? col.renderCell({ row }) : row[col.field];
          return React.createElement("div", { key: col.field, className: "mock-cell" }, cellContent);
        })
      );
    });
    return React.createElement("div", { className: "mock-datagrid" }, renderedRows);
  },
}));

describe("UsersPage page component", () => {
  let confirmMock = vi.fn().mockReturnValue(true);
  let promptMock = vi.fn().mockReturnValue("123456");
  let alertMock = vi.fn();

  beforeEach(() => {
    stateStore = {};
    stateSetters = {};
    callIdx = 0;
    effectCleanups = [];
    capturedButtonClicks.length = 0;
    capturedMenuItemClicks.length = 0;
    capturedChipClicks.length = 0;
    capturedIconButtonClicks.length = 0;
    capturedSubmits.length = 0;
    capturedInputs.length = 0;
    capturedDialogCloses.length = 0;
    capturedMenuCloses.length = 0;
    capturedMenuClicks.length = 0;
    confirmMock = vi.fn().mockReturnValue(true);
    promptMock = vi.fn().mockReturnValue("123456");
    alertMock = vi.fn();

    vi.stubGlobal("document", {
      visibilityState: "visible",
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    });
    vi.stubGlobal("window", {
      setInterval: vi.fn((fn: any, delay: number) => {
        fn();
        return 123;
      }),
      clearInterval: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      confirm: confirmMock,
      prompt: promptMock,
      alert: alertMock,
    });
    vi.stubGlobal("confirm", confirmMock);
    vi.stubGlobal("prompt", promptMock);
    vi.stubGlobal("alert", alertMock);
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  const runRender = () => {
    callIdx = 0;
    capturedButtonClicks.length = 0;
    capturedMenuItemClicks.length = 0;
    capturedChipClicks.length = 0;
    capturedIconButtonClicks.length = 0;
    capturedSubmits.length = 0;
    capturedInputs.length = 0;
    capturedDialogCloses.length = 0;
    capturedMenuCloses.length = 0;
    capturedMenuClicks.length = 0;
    return renderToString(React.createElement(UsersPage));
  };

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
        created_at: "2026-06-04T12:00:00Z",
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
        created_at: "2026-06-04T10:00:00Z",
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
        created_at: "2026-06-04T09:00:00Z",
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
        created_at: "2026-06-04T08:00:00Z",
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

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();

    const html = runRender();
    expect(html).toContain("createUser");
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

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    // Trigger openCreate (createUser button is the first action button)
    expect(capturedButtonClicks[0]).toBeDefined();
    capturedButtonClicks[0]();
    runRender(); // re-render to open dialog

    // Inputs:
    // [0] -> email
    // [1] -> firstName
    // [2] -> lastName
    // [3] -> password
    expect(capturedInputs[0]).toBeDefined();
    capturedInputs[0]({ target: { value: "new@example.com" } });
    capturedInputs[1]({ target: { value: "Bob" } });
    capturedInputs[2]({ target: { value: "Jones" } });
    capturedInputs[3]({ target: { value: "password123" } });

    // Roles Chip click: [0] -> admin, [1] -> user
    expect(capturedChipClicks[0]).toBeDefined();
    capturedChipClicks[0](); // toggle admin

    runRender();

    // Submit form
    expect(capturedSubmits[0]).toBeDefined();
    await capturedSubmits[0]({ preventDefault: vi.fn() });

    expect(createSpy).toHaveBeenCalledWith({
      email: "new@example.com",
      first_name: "Bob",
      last_name: "Jones",
      password: "password123",
      locale: "en",
      roles: ["user", "admin"],
    });
  });

  it("handles user status toggle with MFA step-up flow", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u1",
      roles: ["admin"],
    } as any);

    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    
    // Mock user status change throwing MFA error first, then succeeding
    const stepUpSpy = vi.spyOn(api, "mfaStepUp").mockResolvedValue({} as any);
    const statusSpy = vi.spyOn(api.admin, "setUserStatus")
      .mockRejectedValueOnce(new ApiError("mfa_required"))
      .mockResolvedValueOnce({} as any);

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    // capturedIconButtonClicks[1] is the RowActions icon button for row 2 (id u2, is_active: false)
    expect(capturedIconButtonClicks[1]).toBeDefined();
    capturedIconButtonClicks[1]({ stopPropagation: vi.fn(), currentTarget: {} });
    runRender(); // render menu

    // MenuItem:
    // [0] -> viewSessions for Bob (u2)
    // [1] -> activate/deactivate for Bob (u2 is not self, status is inactive)
    expect(capturedMenuItemClicks[1]).toBeDefined();
    await capturedMenuItemClicks[1]();

    expect(statusSpy).toHaveBeenCalledWith("u2", true);
    expect(promptMock).toHaveBeenCalled();
    expect(stepUpSpy).toHaveBeenCalledWith("123456");
    expect(statusSpy).toHaveBeenCalledTimes(2);
  });

  it("handles revoking all sessions for user", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u1",
      roles: ["admin"],
    } as any);

    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    const revokeAllSpy = vi.spyOn(api.admin, "revokeAllUserSessions").mockResolvedValue({} as any);

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    // capturedIconButtonClicks[0] is RowActions for Alice (u1)
    capturedIconButtonClicks[0]({ stopPropagation: vi.fn(), currentTarget: {} });
    runRender();

    // MenuItem [1] -> revokeAllSessions for Alice
    expect(capturedMenuItemClicks[1]).toBeDefined();
    await capturedMenuItemClicks[1]();

    expect(revokeAllSpy).toHaveBeenCalledWith("u1");
  });

  it("handles sessions dialog with single session revocation and revoke all sessions", async () => {
    vi.mocked(useMeContext).mockReturnValue({
      user_id: "u1",
      roles: ["admin"],
    } as any);

    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    
    // Mock user sessions
    const userSessions = [
      {
        id: "s1",
        is_current: true,
        ip_address: "127.0.0.1",
        user_agent: "Mozilla/5.0",
        created_at: "2026-06-04T12:00:00Z",
        last_used_at: "2026-06-04T12:00:00Z",
        device_info: { browser: "Chrome", browser_version: "120", os: "Windows", os_version: "10" },
      }
    ];
    vi.spyOn(api.admin, "listUserSessions").mockResolvedValue(userSessions);
    const revokeSessionSpy = vi.spyOn(api.admin, "revokeUserSession").mockResolvedValue({} as any);
    const revokeAllSpy = vi.spyOn(api.admin, "revokeAllUserSessions").mockResolvedValue({} as any);

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    // Open sessions dialog: Alice (u1) -> RowActions
    capturedIconButtonClicks[0]({ stopPropagation: vi.fn(), currentTarget: {} });
    runRender();

    // MenuItem [0] -> viewSessions
    capturedMenuItemClicks[0]();
    
    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    // Inside Dialog, we have buttons:
    // [1] -> revokeSession button in ListItem
    // [2] -> revokeAllSessions in DialogActions
    expect(capturedButtonClicks[1]).toBeDefined(); // revokeSession button
    await capturedButtonClicks[1]();
    expect(revokeSessionSpy).toHaveBeenCalledWith("u1", "s1");

    expect(capturedButtonClicks[2]).toBeDefined(); // revokeAllSessions button
    await capturedButtonClicks[2]();
    expect(revokeAllSpy).toHaveBeenCalledWith("u1");
  });

  it("handles createUser API error", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    vi.spyOn(api.admin, "createUser").mockRejectedValue(new ApiError("already_exists"));

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    capturedButtonClicks[0](); // open create
    runRender();

    capturedInputs[0]({ target: { value: "new@example.com" } });
    capturedInputs[3]({ target: { value: "password" } });
    await capturedSubmits[0]({ preventDefault: vi.fn() });

    const html = runRender();
    expect(html).toContain("errors.already_exists");
  });

  it("handles cancel/empty MFA prompt on step-up", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    vi.spyOn(api.admin, "setUserStatus").mockRejectedValue(new ApiError("mfa_required"));
    promptMock.mockReturnValue(""); // empty prompt return

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    capturedIconButtonClicks[1]({ stopPropagation: vi.fn(), currentTarget: {} });
    runRender();
    await capturedMenuItemClicks[1](); // Bob status toggle (index 1 when Alice menu is closed)

    expect(promptMock).toHaveBeenCalled();
  });

  it("handles settings load failure and cleanup", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockRejectedValue(new Error("settings failed"));
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    expect(effectCleanups[0]).toBeDefined();
    effectCleanups[0]();
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
    const revokeSessionSpy = vi.spyOn(api.admin, "revokeUserSession").mockRejectedValue(new Error("API Error"));
    const revokeAllSpy = vi.spyOn(api.admin, "revokeAllUserSessions").mockRejectedValue(new Error("API Error"));

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    capturedIconButtonClicks[0]({ stopPropagation: vi.fn(), currentTarget: {} });
    runRender();
    capturedMenuItemClicks[0](); // viewSessions
    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    // Revoke single
    try {
      await capturedButtonClicks[1]();
    } catch (e) {}
    expect(alertMock).toHaveBeenCalledWith("errors.internal_error");

    // Revoke all
    try {
      await capturedButtonClicks[2]();
    } catch (e) {}
    expect(alertMock).toHaveBeenCalledWith("errors.internal_error");
  });

  it("does not load admin data when the viewer is not an admin", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["user"] } as any);
    const getSettingsSpy = vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    const listSpy = vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());

    const html = runRender();
    await Promise.resolve();
    await Promise.resolve();

    expect(html).toContain("accessDenied");
    expect(getSettingsSpy).not.toHaveBeenCalled();
    expect(listSpy).not.toHaveBeenCalled();
  });

  it("ignores settings that resolve after cleanup and refreshes on session events", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    let resolveSettings: (value: { max_sessions_per_user: string }) => void = () => {};
    vi.spyOn(api.admin, "getSettings").mockReturnValue(new Promise((resolve) => { resolveSettings = resolve; }));
    const listSpy = vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());

    runRender();
    const sessionsChanged = vi.mocked(window.addEventListener).mock.calls.find(([event]) => event === "sessions:changed")?.[1] as () => void;
    expect(sessionsChanged).toBeDefined();
    sessionsChanged();
    effectCleanups[0]();
    resolveSettings({ max_sessions_per_user: "9" });
    await Promise.resolve();
    await Promise.resolve();
    runRender();
    await Promise.resolve();
    await Promise.resolve();

    expect(listSpy).toHaveBeenCalled();
    expect(window.removeEventListener).toHaveBeenCalledWith("sessions:changed", sessionsChanged);
  });

  it("covers row menu close handlers", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    capturedIconButtonClicks[1]({ stopPropagation: vi.fn(), currentTarget: {} });
    runRender();

    expect(capturedMenuCloses[0]).toBeDefined();
    capturedMenuCloses[0]();
    expect(capturedMenuClicks[0]).toBeDefined();
    capturedMenuClicks[0]();
  });

  it("handles deactivate cancellation and generic status errors", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    const statusSpy = vi.spyOn(api.admin, "setUserStatus").mockRejectedValue(new Error("status failed"));

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    confirmMock.mockReturnValueOnce(false);
    capturedIconButtonClicks[2]({ stopPropagation: vi.fn(), currentTarget: {} });
    runRender();
    await capturedMenuItemClicks[2]();
    expect(statusSpy).not.toHaveBeenCalled();

    confirmMock.mockReturnValueOnce(true);
    capturedIconButtonClicks[2]({ stopPropagation: vi.fn(), currentTarget: {} });
    runRender();
    await capturedMenuItemClicks[2]();
    expect(statusSpy).toHaveBeenCalledWith("u3", false);
    expect(alertMock).toHaveBeenCalledWith("errors.internal_error");
  });

  it("handles revoke-all cancellation, MFA retry cancellation, and generic errors", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    const revokeAllSpy = vi.spyOn(api.admin, "revokeAllUserSessions")
      .mockRejectedValueOnce(new ApiError("mfa_required"))
      .mockRejectedValueOnce(new Error("revoke failed"));

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    confirmMock.mockReturnValueOnce(false);
    capturedIconButtonClicks[0]({ stopPropagation: vi.fn(), currentTarget: {} });
    runRender();
    await capturedMenuItemClicks[1]();
    expect(revokeAllSpy).not.toHaveBeenCalled();

    promptMock.mockReturnValueOnce("");
    capturedIconButtonClicks[0]({ stopPropagation: vi.fn(), currentTarget: {} });
    runRender();
    await capturedMenuItemClicks[1]();
    expect(revokeAllSpy).toHaveBeenCalledTimes(1);

    capturedIconButtonClicks[0]({ stopPropagation: vi.fn(), currentTarget: {} });
    runRender();
    await capturedMenuItemClicks[1]();
    expect(alertMock).toHaveBeenCalledWith("errors.internal_error");
  });

  it("handles create dialog cancel, close, role removal, and non-ApiError create failures", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    const createSpy = vi.spyOn(api.admin, "createUser").mockRejectedValue(new Error("create failed"));

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    capturedButtonClicks[0]();
    runRender();
    expect(capturedDialogCloses[0]).toBeDefined();
    capturedDialogCloses[0]();
    runRender();

    capturedButtonClicks[0]();
    runRender();
    expect(capturedButtonClicks[1]).toBeDefined();
    capturedButtonClicks[1]();
    runRender();

    capturedButtonClicks[0]();
    runRender();
    capturedInputs[0]({ target: { value: "roleless@example.com" } });
    capturedInputs[3]({ target: { value: "password" } });
    capturedChipClicks[1]();
    await capturedSubmits[0]({ preventDefault: vi.fn() });

    expect(createSpy).toHaveBeenCalled();
    expect(runRender()).toContain("errors.internal_error");
  });

  it("renders empty sessions dialogs", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    const listSessionsSpy = vi.spyOn(api.admin, "listUserSessions").mockResolvedValue([]);

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    capturedIconButtonClicks[0]({ stopPropagation: vi.fn(), currentTarget: {} });
    runRender();
    capturedMenuItemClicks[0]();
    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    expect(runRender()).toContain("noSessionsFound");
    const closeEmptyDialog = capturedDialogCloses[capturedDialogCloses.length - 1];
    expect(closeEmptyDialog).toBeDefined();
    closeEmptyDialog();
    expect(listSessionsSpy).toHaveBeenCalledTimes(1);
  });

  it("renders failed sessions dialogs", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    const listSessionsSpy = vi.spyOn(api.admin, "listUserSessions").mockRejectedValue(new Error("sessions failed"));

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    capturedIconButtonClicks[0]({ stopPropagation: vi.fn(), currentTarget: {} });
    runRender();
    capturedMenuItemClicks[0]();
    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    expect(runRender()).toContain("errors.internal_error");
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
    promptMock.mockReturnValue("");

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    capturedIconButtonClicks[0]({ stopPropagation: vi.fn(), currentTarget: {} });
    runRender();
    capturedMenuItemClicks[0]();
    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    const html = runRender();
    expect(html).toContain("Linux x64");
    expect(html).toContain("Firefox");

    await capturedButtonClicks[1]();
    await capturedButtonClicks[3]();

    expect(revokeSessionSpy).toHaveBeenCalledWith("u1", "s1");
    expect(revokeAllSpy).toHaveBeenCalledWith("u1");
    expect(alertMock).not.toHaveBeenCalled();
  });

  it("evaluates loading states, edge case fallback branches, and non-MFA ApiErrors", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    
    // 1. Invalid max_sessions_per_user to hit the false branch in getSettings parsing
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "invalid" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    
    // 2. Mock createUser to freeze on creating = true to hit the creating ? "creating" : "create" branch
    let resolveCreate: any;
    vi.spyOn(api.admin, "createUser").mockReturnValue(new Promise((res) => { resolveCreate = res; }));
    
    // 3. Mock setUserStatus to throw a non-MFA ApiError to hit the err.message !== "mfa_required" branch in runWithStepUp
    vi.spyOn(api.admin, "setUserStatus").mockRejectedValue(new ApiError("forbidden"));

    // 4. Mock sessions for revocation testing with non-MFA ApiError
    vi.spyOn(api.admin, "listUserSessions").mockResolvedValue([{ id: "s1", is_current: false } as any]);
    vi.spyOn(api.admin, "revokeUserSession").mockRejectedValue(new ApiError("forbidden"));
    vi.spyOn(api.admin, "revokeAllUserSessions").mockRejectedValue(new ApiError("forbidden"));

    runRender();
    await Promise.resolve(); await Promise.resolve(); await Promise.resolve();
    runRender();

    // Open create dialog
    capturedButtonClicks[0]();
    runRender();
    
    // Fill create form and submit (will block)
    capturedInputs[0]({ target: { value: "test@example.com" } });
    capturedInputs[3]({ target: { value: "pass" } });
    capturedSubmits[0]({ preventDefault: vi.fn() });
    
    // Render while creating === true
    const html = runRender();
    expect(html).toContain("creating");
    
    // Resolve create
    resolveCreate({});
    await Promise.resolve();
    
    // Trigger status change to hit the ApiError("forbidden") branch in runWithStepUp
    capturedIconButtonClicks[2]({ stopPropagation: vi.fn(), currentTarget: {} });
    runRender();
    await capturedMenuItemClicks[2](); // toggle status
    expect(alertMock).toHaveBeenCalledWith("errors.internal_error");

    // Trigger revokeAll from row menu to hit ApiError("forbidden") in handleRevokeAll
    capturedIconButtonClicks[2]({ stopPropagation: vi.fn(), currentTarget: {} });
    runRender();
    await capturedMenuItemClicks[1](); // revokeAllSessions (index 1)
    expect(alertMock).toHaveBeenCalledWith("errors.internal_error");

    // Open sessions dialog and trigger revocations to hit ApiError("forbidden") branches in SessionsDialog
    capturedIconButtonClicks[0]({ stopPropagation: vi.fn(), currentTarget: {} });
    runRender();
    capturedMenuItemClicks[0](); // viewSessions
    runRender();
    await Promise.resolve(); await Promise.resolve(); await Promise.resolve();
    runRender();

    try { await capturedButtonClicks[1](); } catch (e) {} // Revoke single
    expect(alertMock).toHaveBeenCalledWith("errors.internal_error");

    try { await capturedButtonClicks[2](); } catch (e) {} // Revoke all
    expect(alertMock).toHaveBeenCalledWith("errors.internal_error");
  });

  it("handles getSettings catch block when cancelled is true", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    
    let rejectSettings: any;
    vi.spyOn(api.admin, "getSettings").mockReturnValue(new Promise((_, rej) => { rejectSettings = rej; }));
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    
    runRender();
    
    // Unmount component to set cancelled = true
    effectCleanups[0]();
    
    // Now reject the promise
    rejectSettings(new Error("fetch failed"));
    await Promise.resolve();
    await Promise.resolve();
  });

  it("evaluates all branches of sessionDeviceLabel", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });

    // Provide all 4 device info variants
    vi.spyOn(api.admin, "listUserSessions").mockResolvedValue([
      { id: "s1", is_current: false, device_info: { browser: "Chrome" } } as any, // browserStr true, osStr false
      { id: "s2", is_current: false, device_info: { os: "Windows" } } as any, // browserStr false, osStr true
      { id: "s3", is_current: false, device_info: {} } as any, // both false
      { id: "s4", is_current: false, device_info: { browser: "Firefox", os: "Linux" } } as any, // both true
    ]);

    runRender();
    await Promise.resolve(); await Promise.resolve(); await Promise.resolve();
    runRender();

    // Open SessionsDialog for u1
    capturedIconButtonClicks[0]({ stopPropagation: vi.fn(), currentTarget: {} });
    runRender();
    capturedMenuItemClicks[0](); // viewSessions
    runRender();

    await Promise.resolve(); await Promise.resolve(); await Promise.resolve();
    const html = runRender();

    expect(html).toContain("Chrome");
    expect(html).toContain("Windows");
    expect(html).toContain("Unknown device");
    expect(html).toContain("Firefox — Linux");
  });

  it("evaluates getSettings edge cases and cancellation", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listUsers").mockResolvedValue({ data: [], total: 0 });

    // Case 1: max_sessions_per_user is undefined to hit `?? ""`
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({} as any);
    runRender();
    await Promise.resolve(); await Promise.resolve(); await Promise.resolve();
    
    // Case 2: cancelled is true inside the .then() block
    let resolveSettings: any;
    vi.spyOn(api.admin, "getSettings").mockReturnValue(new Promise((res) => { resolveSettings = res; }));
    runRender();
    effectCleanups[effectCleanups.length - 1](); // run cleanup to set cancelled = true
    resolveSettings({ max_sessions_per_user: "10" });
    await Promise.resolve(); await Promise.resolve(); await Promise.resolve();
  });

  it("handles null user context safely", () => {
    vi.mocked(useMeContext).mockReturnValue(null);
    const html = runRender();
    expect(html).toContain("accessDenied");
  });

  it("handles runWithStepUp branches explicitly", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });

    runRender();
    await Promise.resolve(); await Promise.resolve(); await Promise.resolve();
    runRender();

    // Cause an ApiError that is not mfa_required
    vi.spyOn(api.admin, "setUserStatus").mockRejectedValueOnce(new ApiError("some_other_error"));
    capturedIconButtonClicks[2]({ stopPropagation: vi.fn(), currentTarget: {} }); // u3 icon
    runRender();
    
    // Find deactivate item (last item in the menu) and trigger
    await capturedMenuItemClicks[capturedMenuItemClicks.length - 1]();
    expect(alertMock).toHaveBeenCalledWith("errors.internal_error");
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
    promptMock.mockReturnValue(undefined);

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();

    expect(runRender()).toContain("noActiveSessions");

    capturedIconButtonClicks[0]({ stopPropagation: vi.fn(), currentTarget: {} });
    runRender();
    await capturedMenuItemClicks[capturedMenuItemClicks.length - 1]();
    expect(promptMock).toHaveBeenCalled();

    capturedMenuItemClicks[0]();
    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    const html = runRender();
    expect(html).toContain("Chrome 120");

    expect(capturedButtonClicks[1]).toBeDefined();
  });

  it("renders the sessions dialog loading state", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    const user = getMockUsers().data[2];
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue({ data: [user], total: 1 });
    vi.spyOn(api.admin, "listUserSessions").mockReturnValue(new Promise(() => {}));

    stateStore[10] = user;
    stateStore[21] = true;

    const html = runRender();

    expect(html).toContain("CircularProgress");
  });
});
