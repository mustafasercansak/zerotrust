import { test, expect } from "./fixtures";

// Full auth flow tests. Requires:
//   - Backend running at :8080
//   - E2E_USER_EMAIL + E2E_USER_PASSWORD  →  a regular user (no MFA)
//   - E2E_ADMIN_EMAIL + E2E_ADMIN_PASSWORD →  an admin user  (no MFA)
//
// Authentication is provided by the `authAs` fixture (e2e/fixtures.ts): each
// role keeps one live session that is refreshed (rotated) before every test,
// like a real browser tab — saved-cookie reuse across tests is impossible
// with refresh-token rotation and the stale-initial-session janitor.

const hasUserCreds = !!process.env.E2E_USER_EMAIL && !!process.env.E2E_USER_PASSWORD;
const hasAdminCreds = !!process.env.E2E_ADMIN_EMAIL && !!process.env.E2E_ADMIN_PASSWORD;

// Force the /me response to return locale:"en" so selectors stay in English
// regardless of what locale the user's server-side profile has set.
async function forceEnglish(page: Parameters<Parameters<typeof test>[1]>[0]) {
  await page.route("**/api/v1/me", async (route) => {
    try {
      const response = await route.fetch();
      const body = await response.body();
      const json = JSON.parse(body.toString()) as Record<string, unknown>;
      await route.fulfill({ response, json: { ...json, locale: "en" } });
    } catch {
      // Response disposed (page navigating away during the intercept) — let it through.
      await route.continue().catch(() => {});
    }
  });
}

// ─── Wrong credentials (no auth needed) ──────────────────────────────────────

test.describe("Login with wrong credentials", () => {
  test("shows error toast on invalid password", async ({ page }) => {
    await page.goto("/auth/login");
    await page.getByLabel("Email").fill("nobody@example.com");
    await page.getByLabel("Password").fill("wrongpassword");
    await page.getByRole("button", { name: "Sign In", exact: true }).click();

    const toast = page.locator("[data-sonner-toaster] li").first();
    await expect(toast).toBeVisible({ timeout: 6_000 });
    await expect(page).toHaveURL(/auth\/login/);
  });
});

// ─── Regular user flow (live session via authAs fixture) ────────────────────

test.describe("Regular user flow", () => {
  test.skip(!hasUserCreds, "Set E2E_USER_EMAIL and E2E_USER_PASSWORD to run");

  test.beforeEach(async ({ page, authAs }) => {
    await authAs("user");
    await forceEnglish(page);
    await page.goto("/dashboard");
    await expect(page).toHaveURL(/dashboard/, { timeout: 8_000 });
  });

  test("dashboard shows navigation sidebar", async ({ page }) => {
    await expect(page.getByRole("link", { name: "Sessions" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Settings" })).toBeVisible();
  });

  test("sessions page lists current session", async ({ page }) => {
    await page.goto("/dashboard/sessions");
    await expect(page.getByText("This device")).toBeVisible({ timeout: 8_000 });
  });

  test("regular user cannot access admin-only navigation items", async ({ page }) => {
    await expect(page.getByRole("link", { name: "Users" })).not.toBeVisible();
    await expect(page.getByRole("link", { name: "Security Dashboard" })).not.toBeVisible();
    await expect(page.getByRole("link", { name: "Audit Log" })).not.toBeVisible();
  });

  test("logout redirects to login page", async ({ page }) => {
    await page.getByRole("button", { name: "Log Out" }).click();
    await page.waitForURL("**/auth/login**", { timeout: 8_000 });
    await expect(page.getByRole("heading", { name: "Secure Access" })).toBeVisible();
  });
});

// ─── Admin flow (live session via authAs fixture) ───────────────────────────

test.describe("Admin flow", () => {
  test.skip(!hasAdminCreds, "Set E2E_ADMIN_EMAIL and E2E_ADMIN_PASSWORD to run");

  test.beforeEach(async ({ page, authAs }) => {
    await authAs("admin");
    await forceEnglish(page);
    await page.goto("/dashboard");
    await expect(page).toHaveURL(/dashboard/, { timeout: 8_000 });
  });

  test("admin can see admin-only navigation items", async ({ page }) => {
    await expect(page.getByRole("link", { name: "Users" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Security Dashboard" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Audit Log" })).toBeVisible();
  });

  test("users page lists at least one user", async ({ page }) => {
    // Wait for the data to actually arrive before asserting on grid rows —
    // index-based row checks race the fetch on slow runners (CI).
    const usersLoaded = page.waitForResponse(
      (r) => r.url().includes("/api/v1/admin/users") && r.status() === 200,
    );
    await page.goto("/dashboard/users");
    await usersLoaded;
    // DataGrid row 0 is the header; row 1 is the first data row.
    await expect(page.getByRole("row").nth(1)).toBeVisible({ timeout: 8_000 });
  });

  test("audit log page loads events", async ({ page }) => {
    const auditLoaded = page.waitForResponse(
      (r) => r.url().includes("/api/v1/admin/audit?") && r.status() === 200,
    );
    await page.goto("/dashboard/audit");
    await auditLoaded;
    await expect(page.getByRole("row").nth(1)).toBeVisible({ timeout: 8_000 });
  });

  test("security dashboard renders charts", async ({ page }) => {
    await page.goto("/dashboard/security");
    await expect(page.locator("svg").first()).toBeVisible({ timeout: 8_000 });
  });
});
