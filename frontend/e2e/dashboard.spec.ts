import { test, expect, type Page } from "@playwright/test";

// Dashboard page E2E tests — all API calls are mocked so no backend is required.
// We mock /api/v1/me to return a valid user, which bypasses the auth redirect.

// ─── Shared mock data ─────────────────────────────────────────────────────────

const MOCK_USER = {
  user_id: "user-111",
  email: "alice@example.com",
  first_name: "Alice",
  last_name: "Smith",
  has_avatar: false,
  locale: "en",
  notify_security_emails: true,
  roles: ["user"],
  permissions: [],
  created_at: "2024-01-15T10:00:00Z",
  updated_at: "2024-06-01T08:00:00Z",
};

const MOCK_ADMIN = { ...MOCK_USER, user_id: "admin-222", email: "admin@example.com", roles: ["admin", "user"] };

const MOCK_SESSION = {
  id: "sess-abc",
  ip_address: "192.168.1.10",
  user_agent: "Mozilla/5.0 (X11; Linux x86_64) Chrome/126",
  device_info: { browser: "Chrome", browser_version: "126.0.0.0", os: "Linux", os_version: "", architecture: "x86_64", mobile: "" },
  created_at: "2024-06-20T09:00:00Z",
  last_used_at: "2024-06-20T12:00:00Z",
  is_current: true,
};

const EMPTY_PAGED = { data: [], total: 0, page: 0, page_size: 10 };

// ─── Mock helpers ─────────────────────────────────────────────────────────────

async function mockAuthenticatedUser(page: Page, me = MOCK_USER) {
  await page.route("**/api/v1/me", (r) =>
    r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(me) }),
  );
  // Idle timeout policy — needed by useIdleTimeout in DashboardLayout
  await page.route("**/api/v1/session/policy", (r) =>
    r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ idle_timeout_seconds: 300 }) }),
  );
  // Silence SSE — no cookie so TokenRefreshProvider won't open it, but mock defensively
  await page.route("**/api/v1/sessions/events", (r) => r.fulfill({ status: 200, body: "" }));
  // Audit anomaly check on dashboard load
  await page.route("**/api/v1/me/audit**", (r) =>
    r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(EMPTY_PAGED) }),
  );
}

async function mockHomePageData(page: Page, isAdmin = false) {
  await page.route("**/api/v1/mfa/status", (r) =>
    r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ enabled: false }) }),
  );
  await page.route("**/api/v1/sessions", (r) =>
    r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify([MOCK_SESSION]) }),
  );
  // listAuditLog calls /api/v1/admin/audit for all users (including regular)
  await page.route("**/api/v1/admin/audit**", (r) =>
    r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(EMPTY_PAGED) }),
  );
  if (isAdmin) {
    await page.route("**/api/v1/admin/security-posture", (r) =>
      r.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ total_users: 5, mfa_rate: 0.8, inactive_users: 1, admin_count: 1 }),
      }),
    );
    await page.route("**/api/v1/admin/health", (r) =>
      r.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          status: "ok",
          database: { status: "ok", pool: { total: 10, idle: 5, max: 20 } },
          redis: { status: "ok", pool: { total: 5, idle: 3, max: 10 } },
        }),
      }),
    );
  }
}

// ─── Nav / layout ────────────────────────────────────────────────────────────

test.describe("Dashboard layout — regular user", () => {
  test.beforeEach(async ({ page }) => {
    await mockAuthenticatedUser(page);
    await mockHomePageData(page);
    await page.goto("/dashboard");
    await expect(page).toHaveURL(/dashboard/, { timeout: 8_000 });
  });

  test("renders nav links for standard pages", async ({ page }) => {
    await expect(page.getByTestId("nav-sessions")).toBeVisible();
    await expect(page.getByTestId("nav-mfa")).toBeVisible();
    await expect(page.getByTestId("nav-settings")).toBeVisible();
  });

  test("does not show admin-only nav links", async ({ page }) => {
    await expect(page.getByTestId("nav-users")).not.toBeVisible();
    await expect(page.getByTestId("nav-security")).not.toBeVisible();
    await expect(page.getByTestId("nav-audit")).not.toBeVisible();
  });

  test("shows user email in sidebar", async ({ page }) => {
    await expect(page.getByTestId("sidebar-user-email")).toBeVisible();
  });

  test("Log Out button is present", async ({ page }) => {
    await expect(page.getByTestId("logout-button")).toBeVisible();
  });
});

test.describe("Dashboard layout — admin user", () => {
  test.beforeEach(async ({ page }) => {
    await mockAuthenticatedUser(page, MOCK_ADMIN);
    await mockHomePageData(page, true);
    await page.goto("/dashboard");
    await expect(page).toHaveURL(/dashboard/, { timeout: 8_000 });
  });

  test("shows admin-only nav links", async ({ page }) => {
    await expect(page.getByTestId("nav-users")).toBeVisible();
    await expect(page.getByTestId("nav-security")).toBeVisible();
    await expect(page.getByTestId("nav-audit")).toBeVisible();
  });

  test("shows Service Accounts and OIDC Clients links", async ({ page }) => {
    await expect(page.getByTestId("nav-service-accounts")).toBeVisible();
    await expect(page.getByTestId("nav-oidc-clients")).toBeVisible();
  });
});

// ─── Sessions page ────────────────────────────────────────────────────────────

test.describe("Sessions page", () => {
  test.beforeEach(async ({ page }) => {
    await mockAuthenticatedUser(page);
    await page.route("**/api/v1/sessions", (r) =>
      r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify([MOCK_SESSION]) }),
    );
    await page.route("**/api/v1/me/audit**", (r) =>
      r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(EMPTY_PAGED) }),
    );
    await page.goto("/dashboard/sessions");
    await expect(page).toHaveURL(/dashboard\/sessions/, { timeout: 8_000 });
  });

  test("shows active sessions tab", async ({ page }) => {
    await expect(page.getByTestId("tab-active-sessions")).toBeVisible({ timeout: 6_000 });
  });

  test("shows This device label for current session", async ({ page }) => {
    await expect(page.getByTestId("chip-current-session")).toBeVisible({ timeout: 6_000 });
  });

  test("shows session IP address", async ({ page }) => {
    await expect(page.getByTestId("session-ip").filter({ hasText: "192.168.1.10" })).toBeVisible({ timeout: 6_000 });
  });

  test("shows multiple sessions when present", async ({ page }) => {
    const otherSession = {
      ...MOCK_SESSION,
      id: "sess-other",
      ip_address: "10.0.0.5",
      is_current: false,
      device_info: { browser: "Firefox", browser_version: "127.0", os: "Windows", os_version: "11", architecture: "x64", mobile: "" },
    };
    await page.route("**/api/v1/sessions", (r) =>
      r.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify([MOCK_SESSION, otherSession]),
      }),
    );
    await page.reload();
    await expect(page.getByTestId("session-ip").filter({ hasText: "192.168.1.10" })).toBeVisible({ timeout: 6_000 });
    await expect(page.getByTestId("session-ip").filter({ hasText: "10.0.0.5" })).toBeVisible({ timeout: 6_000 });
  });
});

// ─── MFA page ────────────────────────────────────────────────────────────────

test.describe("MFA page", () => {
  test.beforeEach(async ({ page }) => {
    await mockAuthenticatedUser(page);
  });

  test("shows Two-Factor Authentication section when MFA is disabled", async ({ page }) => {
    await page.route("**/api/v1/mfa/status", (r) =>
      r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ enabled: false }) }),
    );
    await page.goto("/dashboard/mfa");

    await expect(page.getByTestId("mfa-section-title")).toBeVisible({ timeout: 8_000 });
    await expect(page.getByTestId("mfa-status-chip")).toBeVisible();
    await expect(page.getByTestId("mfa-enable-button")).toBeVisible();
  });

  test("shows Enabled status and Disable button when MFA is active", async ({ page }) => {
    await page.route("**/api/v1/mfa/status", (r) =>
      r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ enabled: true }) }),
    );
    await page.goto("/dashboard/mfa");

    await expect(page.getByTestId("mfa-status-chip")).toBeVisible({ timeout: 8_000 });
    await expect(page.getByTestId("mfa-disable-button")).toBeVisible();
  });

  test("shows Passkeys section", async ({ page }) => {
    await page.route("**/api/v1/mfa/status", (r) =>
      r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ enabled: false }) }),
    );
    await page.route("**/api/v1/me/webauthn**", (r) =>
      r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify([]) }),
    );
    await page.goto("/dashboard/mfa");

    await expect(page.getByTestId("passkeys-section-title")).toBeVisible({ timeout: 8_000 });
  });
});

// ─── Settings page ───────────────────────────────────────────────────────────

test.describe("Settings page — regular user", () => {
  test.beforeEach(async ({ page }) => {
    await mockAuthenticatedUser(page);
    await page.goto("/dashboard/settings");
    await expect(page).toHaveURL(/dashboard\/settings/, { timeout: 8_000 });
  });

  test("profile tab is selected by default and shows name fields", async ({ page }) => {
    await expect(page.getByTestId("tab-profile-settings")).toBeVisible();
    await expect(page.getByTestId("settings-first-name")).toBeVisible({ timeout: 6_000 });
    await expect(page.getByTestId("settings-last-name")).toBeVisible();
  });

  test("Security and Sessions tab is visible", async ({ page }) => {
    await expect(page.getByTestId("tab-security-sessions")).toBeVisible();
  });

  test("does not show System Settings tab for regular user", async ({ page }) => {
    await expect(page.getByTestId("tab-system-settings")).not.toBeVisible();
  });

  test("password change fields are visible on profile tab", async ({ page }) => {
    await expect(page.getByTestId("settings-current-password")).toBeVisible({ timeout: 6_000 });
    await expect(page.getByTestId("settings-new-password")).toBeVisible();
    await expect(page.getByTestId("settings-confirm-password")).toBeVisible();
  });
});

test.describe("Settings page — admin user", () => {
  test.beforeEach(async ({ page }) => {
    await mockAuthenticatedUser(page, MOCK_ADMIN);
    await page.goto("/dashboard/settings");
    await expect(page).toHaveURL(/dashboard\/settings/, { timeout: 8_000 });
  });

  test("admin sees System Settings tab", async ({ page }) => {
    await expect(page.getByTestId("tab-system-settings")).toBeVisible({ timeout: 6_000 });
  });
});

// ─── Settings page — admin System Settings tab content ───────────────────────

const MOCK_ADMIN_SETTINGS = {
  max_sessions_per_user: "5",
  password_complexity: "strong",
  global_mfa_required: "false",
  require_hardware_attestation: "false",
  webhook_enabled: "true",
  webhook_url: "https://hooks.slack.com/services/test",
  ip_allowlist: "10.0.0.0/8,192.168.1.1",
  country_allowlist: "TR,US",
  max_login_attempts: "5",
  session_idle_timeout_seconds: "300",
  session_idle_timeout_seconds_admin: "180",
  session_absolute_timeout_seconds: "28800",
};

test.describe("Settings page — System Settings tab content", () => {
  test.beforeEach(async ({ page }) => {
    await mockAuthenticatedUser(page, MOCK_ADMIN);
    await page.route("**/api/v1/admin/settings", (r) =>
      r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(MOCK_ADMIN_SETTINGS) }),
    );
    await page.goto("/dashboard/settings");
    await expect(page).toHaveURL(/dashboard\/settings/, { timeout: 8_000 });
    await page.getByTestId("tab-system-settings").click();
  });

  test("shows max sessions field with loaded value", async ({ page }) => {
    await expect(page.getByTestId("settings-max-sessions")).toBeVisible({ timeout: 6_000 });
    await expect(page.getByTestId("settings-max-sessions")).toHaveValue("5");
  });

  test("shows max login attempts field with loaded value", async ({ page }) => {
    await expect(page.getByTestId("settings-max-login-attempts")).toBeVisible({ timeout: 6_000 });
    await expect(page.getByTestId("settings-max-login-attempts")).toHaveValue("5");
  });

  test("shows IP allowlist field with loaded value", async ({ page }) => {
    await expect(page.getByTestId("settings-ip-allowlist")).toBeVisible({ timeout: 6_000 });
    await expect(page.getByTestId("settings-ip-allowlist")).toHaveValue("10.0.0.0/8,192.168.1.1");
  });

  test("shows country allowlist field with loaded value", async ({ page }) => {
    await expect(page.getByTestId("settings-country-allowlist")).toBeVisible({ timeout: 6_000 });
    await expect(page.getByTestId("settings-country-allowlist")).toHaveValue("TR,US");
  });

  test("shows webhook URL field with loaded value", async ({ page }) => {
    await expect(page.getByTestId("settings-webhook-url")).toBeVisible({ timeout: 6_000 });
    await expect(page.getByTestId("settings-webhook-url")).toHaveValue("https://hooks.slack.com/services/test");
  });

  test("webhook test button is enabled when URL is set and webhook is enabled", async ({ page }) => {
    await expect(page.getByTestId("settings-webhook-test")).toBeVisible({ timeout: 6_000 });
    await expect(page.getByTestId("settings-webhook-test")).toBeEnabled();
  });

  test("system save button is visible and enabled", async ({ page }) => {
    await expect(page.getByTestId("settings-system-save")).toBeVisible({ timeout: 6_000 });
    await expect(page.getByTestId("settings-system-save")).toBeEnabled();
  });

  test("saves system settings and calls API on submit", async ({ page }) => {
    let patchCalled = false;
    await page.route("**/api/v1/admin/settings", async (r) => {
      if (r.request().method() !== "GET") {
        patchCalled = true;
        await r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(MOCK_ADMIN_SETTINGS) });
      } else {
        await r.continue();
      }
    });
    await expect(page.getByTestId("settings-system-save")).toBeVisible({ timeout: 6_000 });
    await page.getByTestId("settings-system-save").click();
    await expect(async () => {
      expect(patchCalled).toBe(true);
    }).toPass({ timeout: 4_000 });
  });

  test("IP allowlist field accepts new input", async ({ page }) => {
    const field = page.getByTestId("settings-ip-allowlist");
    await expect(field).toBeVisible({ timeout: 6_000 });
    await field.fill("172.16.0.0/12");
    await expect(field).toHaveValue("172.16.0.0/12");
  });

  test("country allowlist field accepts new input", async ({ page }) => {
    const field = page.getByTestId("settings-country-allowlist");
    await expect(field).toBeVisible({ timeout: 6_000 });
    await field.fill("DE,FR");
    await expect(field).toHaveValue("DE,FR");
  });
});

// ─── Settings page — Login Activity tab ──────────────────────────────────────

test.describe("Settings page — Login Activity tab", () => {
  test("shows empty state when no activity entries", async ({ page }) => {
    await mockAuthenticatedUser(page, MOCK_USER);
    await page.route("**/api/v1/me/audit**", (r) =>
      r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: [], total: 0 }) }),
    );
    await page.goto("/dashboard/settings");
    await expect(page).toHaveURL(/dashboard\/settings/, { timeout: 8_000 });
    await page.getByTestId("tab-login-activity").click();
    await expect(page.getByTestId("activity-section")).toBeVisible({ timeout: 6_000 });
    await expect(page.getByTestId("activity-empty-state")).toBeVisible({ timeout: 6_000 });
    await expect(page.getByText("No recent security events found.")).toBeVisible();
  });

  test("shows activity entries when audit returns data", async ({ page }) => {
    const MOCK_AUDIT_ENTRY = {
      id: "e1",
      user_id: MOCK_USER.user_id,
      user_email: MOCK_USER.email,
      action: "auth.login",
      resource: "session",
      ip_address: "1.2.3.4",
      user_agent: "Chrome/120",
      metadata: { outcome: "success", location: { city: "Istanbul", country: "TR" }, client_info: { browser: "Chrome", os: "Windows" } },
      created_at: "2026-06-20T10:00:00Z",
    };
    await mockAuthenticatedUser(page, MOCK_USER);
    await page.route("**/api/v1/me/audit**", (r) =>
      r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: [MOCK_AUDIT_ENTRY], total: 1 }) }),
    );
    await page.goto("/dashboard/settings");
    await expect(page).toHaveURL(/dashboard\/settings/, { timeout: 8_000 });
    await page.getByTestId("tab-login-activity").click();
    await expect(page.getByTestId("activity-section")).toBeVisible({ timeout: 6_000 });
    await expect(page.getByRole("cell", { name: "Login" })).toBeVisible({ timeout: 6_000 });
    await expect(page.getByRole("cell", { name: /Istanbul/ })).toBeVisible();
  });

  test("admin sees Login Activity at tab index 3", async ({ page }) => {
    await mockAuthenticatedUser(page, MOCK_ADMIN);
    await page.route("**/api/v1/admin/settings", (r) =>
      r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({}) }),
    );
    await page.route("**/api/v1/me/audit**", (r) =>
      r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: [], total: 0 }) }),
    );
    await page.goto("/dashboard/settings");
    await expect(page).toHaveURL(/dashboard\/settings/, { timeout: 8_000 });
    await page.getByTestId("tab-login-activity").click();
    await expect(page.getByTestId("activity-section")).toBeVisible({ timeout: 6_000 });
  });
});
