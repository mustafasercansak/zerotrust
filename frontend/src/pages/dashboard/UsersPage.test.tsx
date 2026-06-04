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
      fn();
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
    return React.createElement("div", { onClick: props.onClick }, props.children);
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
  default: (props: any) => props.open ? React.createElement("div", null, props.children) : null
}));

vi.mock("@mui/material/Menu", () => ({
  default: (props: any) => props.open ? React.createElement("div", null, props.children) : null
}));

vi.mock("@mui/material/Tooltip", () => ({
  default: (props: any) => props.children
}));

vi.mock("@mui/x-data-grid", () => ({
  DataGrid: (props: any) => {
    const renderedRows = (props.rows ?? []).map((row: any) => {
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
    capturedButtonClicks.length = 0;
    capturedMenuItemClicks.length = 0;
    capturedChipClicks.length = 0;
    capturedIconButtonClicks.length = 0;
    capturedSubmits.length = 0;
    capturedInputs.length = 0;
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
        is_active: false,
        roles: [],
        active_sessions: 0,
        created_at: "2026-06-04T10:00:00Z",
      }
    ],
    total: 2,
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

  it("handles API error during SessionsDialog revocation", async () => {
    vi.mocked(useMeContext).mockReturnValue({ user_id: "u1", roles: ["admin"] } as any);
    vi.spyOn(api.admin, "getSettings").mockResolvedValue({ max_sessions_per_user: "5" });
    vi.spyOn(api.admin, "listUsers").mockResolvedValue(getMockUsers());
    vi.spyOn(api.admin, "listUserSessions").mockResolvedValue([
      { id: "s1", is_current: true, ip_address: "1.1.1.1", user_agent: "Mozilla", created_at: "2026-06-04T12:00:00Z" }
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
});
