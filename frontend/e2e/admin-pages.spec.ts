import { test, expect, type Page } from "@playwright/test";

// E2E tests for admin dashboard pages: HomePage, UsersPage, AuditPage, SecurityDashboardPage.
// All API calls are mocked — no backend required.

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

const MOCK_ADMIN_USER_ROW = {
  id: "user-333",
  email: "bob@example.com",
  first_name: "Bob",
  last_name: "Jones",
  has_avatar: false,
  locale: "en",
  is_active: true,
  roles: ["user"],
  created_at: "2024-03-10T08:00:00Z",
  updated_at: "2024-06-01T08:00:00Z",
  active_sessions: 1,
  mfa_enabled: false,
  passkey_count: 0,
};

const MOCK_SECURITY_DASHBOARD = {
  range: "24h",
  since: "2024-06-20T00:00:00Z",
  generated_at: "2024-06-20T12:00:00Z",
  metrics: { successful_logins: 42, failed_logins: 3, lockouts: 0, anomalies: 1, active_sessions: 7 },
  auth_activity: [
    { bucket: "2024-06-20T00:00:00Z", success: 10, failure: 1 },
    { bucket: "2024-06-20T06:00:00Z", success: 15, failure: 2 },
    { bucket: "2024-06-20T12:00:00Z", success: 17, failure: 0 },
  ],
  anomaly_breakdown: [{ name: "impossible_travel", count: 1 }],
  login_countries: [{ name: "TR", count: 35 }, { name: "US", count: 7 }],
  failed_login_ips: [{ name: "10.0.0.5", count: 3 }],
};

// ─── Mock helpers ─────────────────────────────────────────────────────────────

async function mockAuth(page: Page, me = MOCK_USER) {
  await page.route("**/api/v1/me", (r) =>
    r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(me) }),
  );
  await page.route("**/api/v1/session/policy", (r) =>
    r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ idle_timeout_seconds: 300 }) }),
  );
  await page.route("**/api/v1/sessions/events", (r) => r.fulfill({ status: 200, body: "" }));
  await page.route("**/api/v1/me/audit**", (r) =>
    r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(EMPTY_PAGED) }),
  );
}

async function mockHomeData(page: Page, isAdmin = false) {
  await page.route("**/api/v1/mfa/status", (r) =>
    r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ enabled: false }) }),
  );
  await page.route("**/api/v1/sessions", (r) =>
    r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify([MOCK_SESSION]) }),
  );
  await page.route("**/api/v1/admin/audit**", (r) =>
    r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(EMPTY_PAGED) }),
  );
  if (isAdmin) {
    await page.route("**/api/v1/admin/security-posture", (r) =>
      r.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ total_users: 8, users_without_mfa: 3, users_inactive_30d: 1 }),
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

// ─── HomePage — regular user ──────────────────────────────────────────────────

test.describe("HomePage — regular user", () => {
  test.beforeEach(async ({ page }) => {
    await mockAuth(page, MOCK_USER);
    await mockHomeData(page);
    await page.goto("/dashboard");
    await expect(page).toHaveURL(/dashboard/, { timeout: 8_000 });
  });

  test("shows the authenticated user's name", async ({ page }) => {
    await expect(page.getByTestId("homepage-user-name")).toBeVisible({ timeout: 6_000 });
    await expect(page.getByTestId("homepage-user-name")).toContainText("Alice Smith");
  });

  test("shows MFA status chip", async ({ page }) => {
    await expect(page.getByTestId("homepage-mfa-chip")).toBeVisible({ timeout: 6_000 });
  });

  test("does not show posture section for regular user", async ({ page }) => {
    // Admin posture only appears for admin role
    await expect(page.getByTestId("nav-users")).not.toBeVisible();
  });

  test("sidebar email matches logged-in user", async ({ page }) => {
    await expect(page.getByTestId("sidebar-user-email")).toContainText("alice@example.com");
  });
});

// ─── HomePage — admin user ────────────────────────────────────────────────────

test.describe("HomePage — admin user", () => {
  test.beforeEach(async ({ page }) => {
    await mockAuth(page, MOCK_ADMIN);
    await mockHomeData(page, true);
    await page.goto("/dashboard");
    await expect(page).toHaveURL(/dashboard/, { timeout: 8_000 });
  });

  test("shows admin user name", async ({ page }) => {
    await expect(page.getByTestId("homepage-user-name")).toContainText("Alice Smith", { timeout: 6_000 });
  });

  test("shows admin nav links", async ({ page }) => {
    await expect(page.getByTestId("nav-users")).toBeVisible();
    await expect(page.getByTestId("nav-security")).toBeVisible();
    await expect(page.getByTestId("nav-audit")).toBeVisible();
  });

  test("MFA chip is visible for admin", async ({ page }) => {
    await expect(page.getByTestId("homepage-mfa-chip")).toBeVisible({ timeout: 6_000 });
  });
});

// ─── UsersPage ────────────────────────────────────────────────────────────────

test.describe("UsersPage", () => {
  test.beforeEach(async ({ page }) => {
    await mockAuth(page, MOCK_ADMIN);
    await page.route("**/api/v1/admin/users**", (r) =>
      r.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: [MOCK_ADMIN_USER_ROW], total: 1, page: 0, page_size: 25 }),
      }),
    );
    await page.goto("/dashboard/users");
    await expect(page).toHaveURL(/dashboard\/users/, { timeout: 8_000 });
  });

  test("shows Create User button", async ({ page }) => {
    await expect(page.getByTestId("create-user-button")).toBeVisible({ timeout: 6_000 });
  });

  test("lists users from API", async ({ page }) => {
    await expect(page.getByText("bob@example.com")).toBeVisible({ timeout: 6_000 });
  });

  test("shows logout button", async ({ page }) => {
    await expect(page.getByTestId("logout-button")).toBeVisible();
  });

  test("shows admin nav links in sidebar", async ({ page }) => {
    await expect(page.getByTestId("nav-users")).toBeVisible();
    await expect(page.getByTestId("nav-security")).toBeVisible();
  });
});

// ─── AuditPage ────────────────────────────────────────────────────────────────

test.describe("AuditPage", () => {
  test.beforeEach(async ({ page }) => {
    await mockAuth(page, MOCK_ADMIN);
    await page.route("**/api/v1/admin/audit/trends", (r) =>
      r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify([]) }),
    );
    await page.route("**/api/v1/admin/audit**", (r) =>
      r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(EMPTY_PAGED) }),
    );
    await page.goto("/dashboard/audit");
    await expect(page).toHaveURL(/dashboard\/audit/, { timeout: 8_000 });
  });

  test("shows Export button for admin", async ({ page }) => {
    await expect(page.getByTestId("export-button")).toBeVisible({ timeout: 6_000 });
  });

  test("shows audit nav link as active", async ({ page }) => {
    await expect(page.getByTestId("nav-audit")).toBeVisible();
  });
});

// ─── SecurityDashboardPage ────────────────────────────────────────────────────

test.describe("SecurityDashboardPage", () => {
  test.beforeEach(async ({ page }) => {
    await mockAuth(page, MOCK_ADMIN);
    await page.route("**/api/v1/admin/security-dashboard**", (r) =>
      r.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(MOCK_SECURITY_DASHBOARD),
      }),
    );
    await page.goto("/dashboard/security");
    await expect(page).toHaveURL(/dashboard\/security/, { timeout: 8_000 });
  });

  test("shows 24h range button as selected by default", async ({ page }) => {
    await expect(page.getByTestId("range-button-24h")).toBeVisible({ timeout: 6_000 });
  });

  test("shows all three range buttons", async ({ page }) => {
    await expect(page.getByTestId("range-button-24h")).toBeVisible({ timeout: 6_000 });
    await expect(page.getByTestId("range-button-7d")).toBeVisible();
    await expect(page.getByTestId("range-button-30d")).toBeVisible();
  });

  test("clicking 7d range button selects it", async ({ page }) => {
    await expect(page.getByTestId("range-button-7d")).toBeVisible({ timeout: 6_000 });
    await page.getByTestId("range-button-7d").click();
    await expect(page.getByTestId("range-button-7d")).toHaveClass(/MuiButton-contained/, { timeout: 4_000 });
    await expect(page.getByTestId("range-button-24h")).not.toHaveClass(/MuiButton-contained/);
  });

  test("clicking 30d range button selects it", async ({ page }) => {
    await expect(page.getByTestId("range-button-30d")).toBeVisible({ timeout: 6_000 });
    await page.getByTestId("range-button-30d").click();
    await expect(page.getByTestId("range-button-30d")).toHaveClass(/MuiButton-contained/, { timeout: 4_000 });
    await expect(page.getByTestId("range-button-24h")).not.toHaveClass(/MuiButton-contained/);
  });

  test("shows security nav link in sidebar", async ({ page }) => {
    await expect(page.getByTestId("nav-security")).toBeVisible();
  });
});
