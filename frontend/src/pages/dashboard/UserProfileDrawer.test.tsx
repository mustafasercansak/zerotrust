import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { render, screen, fireEvent, cleanup, waitFor } from "@testing-library/react";
import { UserProfileDrawer } from "./UsersPage";
import { api } from "@/lib/api";
import { toast } from "sonner";

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

vi.mock("@mui/material/Drawer", () => ({
  default: (props: any) => React.createElement("div", { "data-testid": "drawer" }, props.children),
}));

vi.mock("@mui/material/Tooltip", () => ({
  default: (props: any) => props.children,
}));

vi.mock("@/components/SessionCard", () => ({
  SessionCard: ({ session }: any) =>
    React.createElement("div", { "data-testid": "mock-session" }, `session:${session.id}`),
}));

vi.mock("@/components/AuditEntryCard", () => ({
  AuditEntryCard: ({ entry }: any) =>
    React.createElement("div", { "data-testid": "mock-audit" }, `audit:${entry.id}`),
}));

describe("UserProfileDrawer component", () => {
  const onClose = vi.fn();
  const onRevoke = vi.fn().mockResolvedValue(undefined);
  const onRevokeAll = vi.fn().mockResolvedValue(undefined);
  const onStatusChange = vi.fn().mockResolvedValue(undefined);

  let listUserSessionsSpy: any;
  let listAuditLogSpy: any;
  let listUserMfaSpy: any;

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
    vi.useRealTimers();
    onClose.mockClear();
    onRevoke.mockClear();
    onRevokeAll.mockClear();
    onStatusChange.mockClear();
    vi.mocked(toast.error).mockClear();

    listUserSessionsSpy = vi.spyOn(api.admin, "listUserSessions").mockResolvedValue([]);
    listAuditLogSpy = vi.spyOn(api, "listAuditLog").mockResolvedValue({ data: [], total: 0 });
    listUserMfaSpy = vi.spyOn(api.admin, "listUserMfa").mockResolvedValue({ totp_enabled: false, webauthn_credentials: [] });
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  // ── Profile section (default) ─────────────────────────────────────────────

  it("renders profile section with user name, email, and account info", () => {
    render(<UserProfileDrawer {...defaultProps()} />);
    expect(screen.getAllByText(/Alice/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/Smith/).length).toBeGreaterThan(0);
    expect(screen.getAllByText("alice@example.com").length).toBeGreaterThan(0);
    expect(screen.getByText("accountInfo")).toBeDefined();
  });

  it("falls back to email as display name when first_name and last_name are empty", () => {
    render(<UserProfileDrawer {...defaultProps({ first_name: "", last_name: "" })} />);
    expect(screen.getAllByText("alice@example.com").length).toBeGreaterThan(0);
  });

  it("derives initials from email when no name parts are present", () => {
    render(<UserProfileDrawer {...defaultProps({ first_name: "", last_name: "" })} />);
    expect(screen.getByText("AL")).toBeDefined();
  });

  it("shows deactivate button when user is active and isSelf is false", () => {
    render(<UserProfileDrawer {...defaultProps({ is_active: true })} />);
    expect(screen.getByRole("button", { name: "deactivate" })).toBeDefined();
  });

  it("hides action buttons entirely when isSelf is true", () => {
    render(<UserProfileDrawer {...defaultProps({}, { isSelf: true })} />);
    expect(screen.queryByRole("button", { name: "deactivate" })).toBeNull();
    expect(screen.queryByRole("button", { name: "activate" })).toBeNull();
    expect(screen.queryByRole("button", { name: "revokeAllSessions" })).toBeNull();
  });

  it("hides revokeAll button in profile section when effectiveSessionCount is 0", () => {
    render(<UserProfileDrawer {...defaultProps({ active_sessions: 0 })} />);
    expect(screen.queryByRole("button", { name: "revokeAllSessions" })).toBeNull();
  });

  it("shows revokeAll button in profile section when effectiveSessionCount > 0", () => {
    render(<UserProfileDrawer {...defaultProps({ active_sessions: 2 })} />);
    expect(screen.getByRole("button", { name: "revokeAllSessions" })).toBeDefined();
  });

  it("uses sessionCount over active_sessions for effectiveSessionCount when non-null", async () => {
    listUserSessionsSpy.mockResolvedValue([]);

    render(<UserProfileDrawer {...defaultProps({ active_sessions: 2 })} />);
    expect(screen.getByRole("button", { name: "revokeAllSessions" })).toBeDefined();

    fireEvent.click(screen.getByText("drawerSessions"));
    await waitFor(() => {
      expect(screen.getByText("noSessionsFound")).toBeDefined();
    });

    fireEvent.click(screen.getByText("drawerProfile"));
    expect(screen.queryByRole("button", { name: "revokeAllSessions" })).toBeNull();
  });

  it("profile section shows avatar with initials when has_avatar is false", () => {
    render(<UserProfileDrawer {...defaultProps()} />);
    expect(screen.getByText("AS")).toBeDefined();
  });

  it("renders active chip for active user", () => {
    render(<UserProfileDrawer {...defaultProps({ is_active: true })} />);
    expect(screen.getByText("active")).toBeDefined();
  });

  it("renders inactive chip for inactive user", () => {
    render(<UserProfileDrawer {...defaultProps({ is_active: false })} />);
    expect(screen.getByText("inactive")).toBeDefined();
  });

  it("renders role chips", () => {
    render(<UserProfileDrawer {...defaultProps()} />);
    expect(screen.getByText("admin")).toBeDefined();
  });

  it("calls onStatusChange when deactivate button is clicked", async () => {
    render(<UserProfileDrawer {...defaultProps({ is_active: true })} />);
    const deactivateBtn = screen.getByRole("button", { name: "deactivate" });
    fireEvent.click(deactivateBtn);
    expect(onStatusChange).toHaveBeenCalledWith("u1", false);
  });

  it("calls onRevokeAll and resets sessionCount when revokeAll is clicked in profile", async () => {
    render(<UserProfileDrawer {...defaultProps({ active_sessions: 2 })} />);
    const revokeAllBtn = screen.getByRole("button", { name: "revokeAllSessions" });
    fireEvent.click(revokeAllBtn);

    expect(onRevokeAll).toHaveBeenCalledWith("u1");
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "revokeAllSessions" })).toBeNull();
    });
  });

  it("renders four section nav tabs", () => {
    render(<UserProfileDrawer {...defaultProps()} />);
    expect(screen.getByText("drawerProfile")).toBeDefined();
    expect(screen.getByText("drawerSessions")).toBeDefined();
    expect(screen.getByText("drawerAudit")).toBeDefined();
    expect(screen.getByText("drawerMfa")).toBeDefined();
  });

  // ── Sessions section ──────────────────────────────────────────────────────

  it("shows LinearProgress while sessions are loading", () => {
    listUserSessionsSpy.mockReturnValue(new Promise(() => {}));

    render(<UserProfileDrawer {...defaultProps()} />);
    fireEvent.click(screen.getByText("drawerSessions"));
    expect(screen.getByRole("progressbar")).toBeDefined();
  });

  it("shows noSessionsFound when sessions load empty", async () => {
    listUserSessionsSpy.mockResolvedValue([]);

    render(<UserProfileDrawer {...defaultProps()} />);
    fireEvent.click(screen.getByText("drawerSessions"));

    await waitFor(() => {
      expect(screen.getByText("noSessionsFound")).toBeDefined();
    });
  });

  it("renders loaded sessions as SessionCard mocks", async () => {
    listUserSessionsSpy.mockResolvedValue([
      { id: "s1", is_current: true, ip_address: "1.1.1.1", user_agent: "Mozilla", created_at: "2026-06-01T00:00:00Z", last_used_at: null, device_info: null },
      { id: "s2", is_current: false, ip_address: "2.2.2.2", user_agent: "Safari", created_at: "2026-06-02T00:00:00Z", last_used_at: null, device_info: null },
    ]);

    render(<UserProfileDrawer {...defaultProps()} />);
    fireEvent.click(screen.getByText("drawerSessions"));

    await waitFor(() => {
      expect(screen.getByText("session:s1")).toBeDefined();
      expect(screen.getByText("session:s2")).toBeDefined();
    });
  });

  it("does not re-fetch sessions when already loaded", async () => {
    listUserSessionsSpy.mockResolvedValue([{ id: "s1" } as any]);

    render(<UserProfileDrawer {...defaultProps()} />);

    fireEvent.click(screen.getByText("drawerSessions"));
    await waitFor(() => {
      expect(screen.getByText("session:s1")).toBeDefined();
    });
    expect(listUserSessionsSpy).toHaveBeenCalledOnce();

    fireEvent.click(screen.getByText("drawerProfile"));
    fireEvent.click(screen.getByText("drawerSessions"));

    expect(listUserSessionsSpy).toHaveBeenCalledOnce();
  });

  it("shows empty sessions on API error", async () => {
    listUserSessionsSpy.mockRejectedValue(new Error("network error"));

    render(<UserProfileDrawer {...defaultProps()} />);
    fireEvent.click(screen.getByText("drawerSessions"));

    await waitFor(() => {
      expect(screen.getByText("noSessionsFound")).toBeDefined();
    });
  });

  // ── Audit section ─────────────────────────────────────────────────────────

  it("shows LinearProgress while audit is loading", () => {
    listAuditLogSpy.mockReturnValue(new Promise(() => {}));

    render(<UserProfileDrawer {...defaultProps()} />);
    fireEvent.click(screen.getByText("drawerAudit"));
    expect(screen.getByRole("progressbar")).toBeDefined();
  });

  it("shows noAuditEntries when audit loads empty", async () => {
    listAuditLogSpy.mockResolvedValue({ data: [], total: 0 });

    render(<UserProfileDrawer {...defaultProps()} />);
    fireEvent.click(screen.getByText("drawerAudit"));

    await waitFor(() => {
      expect(screen.getByText("noAuditEntries")).toBeDefined();
    });
  });

  it("renders loaded audit entries as AuditEntryCard mocks", async () => {
    listAuditLogSpy.mockResolvedValue({
      data: [
        { id: "a1", action: "auth.login", created_at: "2026-06-01T00:00:00Z" },
        { id: "a2", action: "users.update", created_at: "2026-06-02T00:00:00Z" },
      ],
      total: 2,
    });

    render(<UserProfileDrawer {...defaultProps()} />);
    fireEvent.click(screen.getByText("drawerAudit"));

    await waitFor(() => {
      expect(screen.getByText("audit:a1")).toBeDefined();
      expect(screen.getByText("audit:a2")).toBeDefined();
    });
  });

  it("shows empty audit on API error", async () => {
    listAuditLogSpy.mockRejectedValue(new Error("error"));

    render(<UserProfileDrawer {...defaultProps()} />);
    fireEvent.click(screen.getByText("drawerAudit"));

    await waitFor(() => {
      expect(screen.getByText("noAuditEntries")).toBeDefined();
    });
  });

  // ── MFA section ───────────────────────────────────────────────────────────

  it("shows LinearProgress while MFA is loading", () => {
    listUserMfaSpy.mockReturnValue(new Promise(() => {}));

    render(<UserProfileDrawer {...defaultProps()} />);
    fireEvent.click(screen.getByText("drawerMfa"));
    expect(screen.getByRole("progressbar")).toBeDefined();
  });

  it("renders TOTP enabled state with success chip", async () => {
    listUserMfaSpy.mockResolvedValue({
      totp_enabled: true,
      webauthn_credentials: [],
    });

    render(<UserProfileDrawer {...defaultProps()} />);
    fireEvent.click(screen.getByText("drawerMfa"));

    await waitFor(() => {
      expect(screen.getByText("totpLabel")).toBeDefined();
      expect(screen.getByText("totpEnabled")).toBeDefined();
      expect(screen.getByText("enabled")).toBeDefined();
      expect(screen.getByText("noPasskeys")).toBeDefined();
    });
  });

  it("renders TOTP disabled state with default chip", async () => {
    listUserMfaSpy.mockResolvedValue({
      totp_enabled: false,
      webauthn_credentials: [],
    });

    render(<UserProfileDrawer {...defaultProps()} />);
    fireEvent.click(screen.getByText("drawerMfa"));

    await waitFor(() => {
      expect(screen.getByText("totpDisabled")).toBeDefined();
      expect(screen.getByText("disabled")).toBeDefined();
    });
  });

  it("renders passkeys list with name, sign_count, and created_at", async () => {
    listUserMfaSpy.mockResolvedValue({
      totp_enabled: false,
      webauthn_credentials: [
        { id: "k1", name: "My YubiKey", sign_count: 12, created_at: "2026-01-01T00:00:00Z", last_used_at: "2026-06-01T00:00:00Z" },
        { id: "k2", name: "", sign_count: 0, created_at: "2026-02-01T00:00:00Z", last_used_at: null },
      ],
    });

    render(<UserProfileDrawer {...defaultProps()} />);
    fireEvent.click(screen.getByText("drawerMfa"));

    await waitFor(() => {
      expect(screen.getByText(/My YubiKey/)).toBeDefined();
      expect(screen.getByText(/signCount:\s*12/)).toBeDefined();
      expect(screen.getByText(/lastUsed/)).toBeDefined();
      expect(screen.getByText(/unnamedPasskey/)).toBeDefined();
    });
  });

  it("uses default MFA info on API error", async () => {
    listUserMfaSpy.mockRejectedValue(new Error("mfa error"));

    render(<UserProfileDrawer {...defaultProps()} />);
    fireEvent.click(screen.getByText("drawerMfa"));

    await waitFor(() => {
      expect(screen.getByText("totpDisabled")).toBeDefined();
      expect(screen.getByText("noPasskeys")).toBeDefined();
    });
  });

  it("does not re-fetch MFA when already loaded", async () => {
    listUserMfaSpy.mockResolvedValue({ totp_enabled: true, webauthn_credentials: [] });

    render(<UserProfileDrawer {...defaultProps()} />);
    fireEvent.click(screen.getByText("drawerMfa"));

    await waitFor(() => {
      expect(screen.getByText("totpEnabled")).toBeDefined();
    });
    expect(listUserMfaSpy).toHaveBeenCalledOnce();

    fireEvent.click(screen.getByText("drawerProfile"));
    fireEvent.click(screen.getByText("drawerMfa"));

    expect(listUserMfaSpy).toHaveBeenCalledOnce();
  });
});
