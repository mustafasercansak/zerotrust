import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { UserProfileDrawer } from "./UsersPage";
import { api } from "@/lib/api";
import { toast } from "sonner";
import { renderToString } from "react-dom/server";

vi.mock("sonner", () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: "en" },
  }),
}));

vi.mock("@mui/material/Drawer", () => ({
  default: (props: any) => React.createElement("div", null, props.children),
}));

vi.mock("@mui/material/Tooltip", () => ({
  default: (props: any) => props.children,
}));

// Prevent SessionCard and AuditEntryCard from consuming state indices
vi.mock("@/components/SessionCard", () => ({
  SessionCard: ({ session }: any) =>
    React.createElement("div", { className: "mock-session" }, `session:${session.id}`),
}));
vi.mock("@/components/AuditEntryCard", () => ({
  AuditEntryCard: ({ entry }: any) =>
    React.createElement("div", { className: "mock-audit" }, `audit:${entry.id}`),
}));

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
      if (callIdx >= 20) callIdx = 0;
      return [stateStore[idx], stateSetters[idx]];
    },
    useEffect: (fn: any) => {
      const cleanup = fn();
      if (typeof cleanup === "function") effectCleanups.push(cleanup);
    },
  };
});

const capturedButtonClicks: any[] = [];
vi.mock("@mui/material/Button", () => ({
  default: (props: any) => {
    if (props.onClick) capturedButtonClicks.push(props.onClick);
    return React.createElement(
      "button",
      { onClick: props.onClick, disabled: props.disabled },
      props.children,
    );
  },
}));

describe("UserProfileDrawer component", () => {
  const onClose = vi.fn();
  const onRevoke = vi.fn().mockResolvedValue(undefined);
  const onRevokeAll = vi.fn().mockResolvedValue(undefined);
  const onStatusChange = vi.fn().mockResolvedValue(undefined);

  const getMockUser = (overrides: any = {}): any => ({
    id: "u1",
    email: "alice@example.com",
    first_name: "Alice",
    last_name: "Smith",
    has_avatar: false,
    locale: "en",
    is_active: true,
    roles: ["admin"],
    active_sessions: 2,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-06-01T00:00:00Z",
    ...overrides,
  });

  const defaultProps = (userOverrides: any = {}, extra: any = {}) => ({
    user: getMockUser(userOverrides),
    onClose,
    onRevoke,
    onRevokeAll,
    onStatusChange,
    isSelf: false,
    ...extra,
  });

  beforeEach(() => {
    stateStore = {};
    stateSetters = {};
    callIdx = 0;
    effectCleanups = [];
    capturedButtonClicks.length = 0;
    onClose.mockClear();
    onRevoke.mockClear();
    onRevokeAll.mockClear();
    onStatusChange.mockClear();
    vi.mocked(toast.error).mockClear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  const runRender = (props?: any) => {
    callIdx = 0;
    capturedButtonClicks.length = 0;
    return renderToString(
      React.createElement(UserProfileDrawer, props ?? defaultProps()),
    );
  };

  // ── Profile section (default) ─────────────────────────────────────────────

  it("renders profile section with user name, email, and account info", () => {
    const html = runRender();
    expect(html).toContain("Alice");
    expect(html).toContain("Smith");
    expect(html).toContain("alice@example.com");
    expect(html).toContain("accountInfo");
  });

  it("falls back to email as display name when first_name and last_name are empty", () => {
    const html = runRender(defaultProps({ first_name: "", last_name: "" }));
    expect(html).toContain("alice@example.com");
  });

  it("derives initials from email when no name parts are present", () => {
    const html = runRender(defaultProps({ first_name: "", last_name: "" }));
    expect(html).toContain("AL"); // email.slice(0,2).toUpperCase()
  });

  it("shows deactivate button when user is active and isSelf is false", () => {
    runRender(defaultProps({ is_active: true }));
    const hasDeactivateBtn = capturedButtonClicks.length > 0;
    expect(hasDeactivateBtn).toBe(true);
  });

  it("hides action buttons entirely when isSelf is true", () => {
    runRender(defaultProps({}, { isSelf: true }));
    // No status-change or revoke buttons should appear
    expect(capturedButtonClicks.length).toBe(0);
  });

  it("hides revokeAll button in profile section when effectiveSessionCount is 0", () => {
    // active_sessions=0 and sessionCount=null → effectiveSessionCount=0
    runRender(defaultProps({ active_sessions: 0 }));
    // Only 1 button: deactivate (no revokeAll)
    expect(capturedButtonClicks.length).toBe(1);
  });

  it("shows revokeAll button in profile section when effectiveSessionCount > 0", () => {
    // active_sessions=2 and sessionCount=null → effectiveSessionCount=2
    runRender(defaultProps({ active_sessions: 2 }));
    expect(capturedButtonClicks.length).toBe(2);
  });

  it("uses sessionCount over active_sessions for effectiveSessionCount when non-null", () => {
    stateStore[3] = 0; // sessionCount = 0 (overrides active_sessions=2)
    runRender(defaultProps({ active_sessions: 2 }));
    // effectiveSessionCount = 0 → revokeAll button hidden
    expect(capturedButtonClicks.length).toBe(1); // only deactivate
  });

  it("profile section shows avatar with initials when has_avatar is false", () => {
    const html = runRender();
    expect(html).toContain("AS"); // Alice Smith initials
  });

  it("renders active chip for active user", () => {
    const html = runRender();
    expect(html).toContain("active");
  });

  it("renders inactive chip for inactive user", () => {
    const html = runRender(defaultProps({ is_active: false }));
    expect(html).toContain("inactive");
  });

  it("renders role chips", () => {
    const html = runRender();
    expect(html).toContain("admin");
  });

  it("calls onStatusChange when deactivate button is clicked", async () => {
    runRender();
    expect(capturedButtonClicks[0]).toBeDefined();
    await capturedButtonClicks[0]();
    expect(onStatusChange).toHaveBeenCalledWith("u1", false);
  });

  it("calls onRevokeAll and resets sessionCount when revokeAll is clicked in profile", async () => {
    stateStore[3] = 3; // sessionCount = 3
    runRender(defaultProps({ active_sessions: 2 }));
    expect(capturedButtonClicks[1]).toBeDefined(); // revokeAll button
    await capturedButtonClicks[1]();
    expect(onRevokeAll).toHaveBeenCalledWith("u1");
    expect(stateStore[3]).toBe(0);  // sessionCount reset
    expect(stateStore[1]).toEqual([]); // sessions cleared
  });

  it("renders four section nav tabs", () => {
    const html = runRender();
    expect(html).toContain("drawerProfile");
    expect(html).toContain("drawerSessions");
    expect(html).toContain("drawerAudit");
    expect(html).toContain("drawerMfa");
  });

  // ── Sessions section ──────────────────────────────────────────────────────

  it("shows LinearProgress while sessions are loading", () => {
    stateStore[0] = "sessions"; // jump to sessions section
    vi.spyOn(api.admin, "listUserSessions").mockResolvedValue([]);

    runRender(); // effect fires → sessionsLoading becomes true
    const html = runRender(); // second render sees sessionsLoading = true
    expect(html).toContain("LinearProgress");
  });

  it("shows noSessionsFound when sessions load empty", async () => {
    stateStore[0] = "sessions";
    vi.spyOn(api.admin, "listUserSessions").mockResolvedValue([]);

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    const html = runRender();
    expect(html).toContain("noSessionsFound");
  });

  it("renders loaded sessions as SessionCard mocks", async () => {
    stateStore[0] = "sessions";
    vi.spyOn(api.admin, "listUserSessions").mockResolvedValue([
      { id: "s1", is_current: true, ip_address: "1.1.1.1", user_agent: "Mozilla", created_at: "2026-06-01T00:00:00Z", last_used_at: null, device_info: null },
      { id: "s2", is_current: false, ip_address: "2.2.2.2", user_agent: "Safari", created_at: "2026-06-02T00:00:00Z", last_used_at: null, device_info: null },
    ] as any[]);

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    const html = runRender();
    expect(html).toContain("session:s1");
    expect(html).toContain("session:s2");
  });

  it("updates sessionCount after sessions load", async () => {
    stateStore[0] = "sessions";
    vi.spyOn(api.admin, "listUserSessions").mockResolvedValue([
      { id: "s1" } as any,
      { id: "s2" } as any,
    ]);

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    runRender();
    expect(stateStore[3]).toBe(2); // sessionCount set to data.length
  });

  it("does not re-fetch sessions when already loaded", async () => {
    stateStore[0] = "sessions";
    stateStore[1] = [{ id: "s1" }]; // sessions already loaded
    const spy = vi.spyOn(api.admin, "listUserSessions").mockResolvedValue([]);

    runRender();
    expect(spy).not.toHaveBeenCalled();
  });

  it("shows empty sessions on API error", async () => {
    stateStore[0] = "sessions";
    vi.spyOn(api.admin, "listUserSessions").mockRejectedValue(new Error("network error"));

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    const html = runRender();
    expect(html).toContain("noSessionsFound");
  });

  // ── Audit section ─────────────────────────────────────────────────────────

  it("shows LinearProgress while audit is loading", () => {
    stateStore[0] = "audit";
    vi.spyOn(api, "listAuditLog").mockResolvedValue({ data: [], total: 0 });

    runRender(); // effect fires → auditLoading = true
    const html = runRender();
    expect(html).toContain("LinearProgress");
  });

  it("shows noAuditEntries when audit loads empty", async () => {
    stateStore[0] = "audit";
    vi.spyOn(api, "listAuditLog").mockResolvedValue({ data: [], total: 0 });

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    const html = runRender();
    expect(html).toContain("noAuditEntries");
  });

  it("renders loaded audit entries as AuditEntryCard mocks", async () => {
    stateStore[0] = "audit";
    vi.spyOn(api, "listAuditLog").mockResolvedValue({
      data: [
        { id: "a1", action: "auth.login", created_at: "2026-06-01T00:00:00Z" },
        { id: "a2", action: "users.update", created_at: "2026-06-02T00:00:00Z" },
      ] as any[],
      total: 2,
    });

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    const html = runRender();
    expect(html).toContain("audit:a1");
    expect(html).toContain("audit:a2");
  });

  it("shows empty audit on API error", async () => {
    stateStore[0] = "audit";
    vi.spyOn(api, "listAuditLog").mockRejectedValue(new Error("error"));

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    const html = runRender();
    expect(html).toContain("noAuditEntries");
  });

  // ── MFA section ───────────────────────────────────────────────────────────

  it("shows LinearProgress while MFA is loading", () => {
    stateStore[0] = "mfa";
    vi.spyOn(api.admin, "listUserMfa").mockResolvedValue({ totp_enabled: false, webauthn_credentials: [] });

    runRender(); // effect fires → mfaLoading = true
    const html = runRender();
    expect(html).toContain("LinearProgress");
  });

  it("renders TOTP enabled state with success chip", async () => {
    stateStore[0] = "mfa";
    vi.spyOn(api.admin, "listUserMfa").mockResolvedValue({
      totp_enabled: true,
      webauthn_credentials: [],
    });

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    const html = runRender();
    expect(html).toContain("totpLabel");
    expect(html).toContain("totpEnabled");
    expect(html).toContain("enabled");
    expect(html).toContain("noPasskeys");
  });

  it("renders TOTP disabled state with default chip", async () => {
    stateStore[0] = "mfa";
    vi.spyOn(api.admin, "listUserMfa").mockResolvedValue({
      totp_enabled: false,
      webauthn_credentials: [],
    });

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    const html = runRender();
    expect(html).toContain("totpDisabled");
    expect(html).toContain("disabled");
  });

  it("renders passkeys list with name, sign_count, and created_at", async () => {
    stateStore[0] = "mfa";
    vi.spyOn(api.admin, "listUserMfa").mockResolvedValue({
      totp_enabled: false,
      webauthn_credentials: [
        { id: "k1", name: "My YubiKey", sign_count: 12, created_at: "2026-01-01T00:00:00Z", last_used_at: "2026-06-01T00:00:00Z" },
        { id: "k2", name: "", sign_count: 0, created_at: "2026-02-01T00:00:00Z", last_used_at: null },
      ],
    });

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    const html = runRender();
    expect(html).toContain("My YubiKey");
    expect(html).toContain("12");
    expect(html).toContain("lastUsed");
    expect(html).toContain("unnamedPasskey"); // fallback for empty name
  });

  it("uses default MFA info on API error", async () => {
    stateStore[0] = "mfa";
    vi.spyOn(api.admin, "listUserMfa").mockRejectedValue(new Error("mfa error"));

    runRender();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    const html = runRender();
    expect(html).toContain("totpDisabled");
    expect(html).toContain("noPasskeys");
  });

  it("does not re-fetch MFA when already loaded", async () => {
    stateStore[0] = "mfa";
    stateStore[6] = { totp_enabled: true, webauthn_credentials: [] }; // mfa already set
    const spy = vi.spyOn(api.admin, "listUserMfa").mockResolvedValue({ totp_enabled: false, webauthn_credentials: [] });

    runRender();
    expect(spy).not.toHaveBeenCalled();
  });
});
