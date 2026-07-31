import { test, expect } from "./fixtures";

// Settings page E2E tests. Requires:
//   - Backend running at :8080
//   - E2E_USER_EMAIL + E2E_USER_PASSWORD  →  a regular user (no MFA)
//   - E2E_ADMIN_EMAIL + E2E_ADMIN_PASSWORD →  an admin user  (no MFA)
//
// These tests verify the UI structure of the settings page and that
// interactive controls work correctly. They do not test backend persistence.
// Authentication uses the `authAs` fixture (e2e/fixtures.ts) — see auth-flow.spec.ts.

const hasUserCreds = !!process.env.E2E_USER_EMAIL && !!process.env.E2E_USER_PASSWORD;
const hasAdminCreds = !!process.env.E2E_ADMIN_EMAIL && !!process.env.E2E_ADMIN_PASSWORD;

async function forceEnglish(page: Parameters<Parameters<typeof test>[1]>[0]) {
  await page.route("**/api/v1/me", async (route) => {
    try {
      const response = await route.fetch();
      const body = await response.body();
      const json = JSON.parse(body.toString()) as Record<string, unknown>;
      await route.fulfill({ response, json: { ...json, locale: "en" } });
    } catch {
      await route.continue().catch(() => {});
    }
  });
}

// ─── Regular user — settings page ────────────────────────────────────────────

test.describe("Settings page — regular user", () => {
  test.skip(!hasUserCreds, "Set E2E_USER_EMAIL and E2E_USER_PASSWORD to run");

  test.beforeEach(async ({ page, authAs }) => {
    await authAs("user");
    await forceEnglish(page);
    await page.goto("/dashboard/settings");
    await expect(page).toHaveURL(/dashboard\/settings/, { timeout: 8_000 });
  });

  test("profile tab is selected by default and shows name fields", async ({ page }) => {
    await expect(page.getByRole("tab", { name: "Profile Settings" })).toBeVisible();
    await expect(page.getByLabel("First name")).toBeVisible();
    await expect(page.getByLabel("Last name")).toBeVisible();
  });

  test("locale selector is visible and shows current language", async ({ page }) => {
    const localeField = page.locator("text=Language & Region").first();
    await expect(localeField).toBeVisible();
    // The select should contain English or Türkçe
    const localeSelect = page.getByLabel("Language");
    await expect(localeSelect).toBeVisible();
  });

  test("email notification toggle is visible", async ({ page }) => {
    await expect(page.locator("text=Email Notifications").first()).toBeVisible();
    const toggle = page.getByRole("checkbox", { name: /security alert/i });
    await expect(toggle).toBeVisible();
  });

  test("password change form is visible", async ({ page }) => {
    await expect(page.getByLabel("Current password")).toBeVisible();
    await expect(page.getByLabel("New password")).toBeVisible();
    await expect(page.getByLabel("Confirm new password")).toBeVisible();
  });

  test("security and sessions tab switches to sessions view", async ({ page }) => {
    await page.getByRole("tab", { name: "Security & Sessions" }).click();
    await expect(page.getByText("Active Sessions")).toBeVisible({ timeout: 8_000 });
  });

  test("locale dropdown can be changed to Turkish and back", async ({ page }) => {
    // Intercept the PATCH /me/locale call so we don't actually change the server state
    await page.route("**/api/v1/me/locale", async (route) => {
      await route.fulfill({ status: 204, body: "" });
    });

    const localeSelect = page.getByLabel("Language");
    await localeSelect.click();
    await page.getByRole("option", { name: "Türkçe" }).click();
    // Switch back to English (the /me mock keeps returning en so no UI shift)
    await localeSelect.click();
    await page.getByRole("option", { name: "English" }).click();
  });
});

// ─── Admin — settings page shows system tab ───────────────────────────────────

test.describe("Settings page — admin", () => {
  test.skip(!hasAdminCreds, "Set E2E_ADMIN_EMAIL and E2E_ADMIN_PASSWORD to run");

  test.beforeEach(async ({ page, authAs }) => {
    await authAs("admin");
    await forceEnglish(page);
    await page.goto("/dashboard/settings");
    await expect(page).toHaveURL(/dashboard\/settings/, { timeout: 8_000 });
  });

  test("admin sees System Settings tab", async ({ page }) => {
    await expect(page.getByRole("tab", { name: "System Settings" })).toBeVisible();
  });

  test("system settings tab shows security policy fields", async ({ page }) => {
    await page.getByRole("tab", { name: "System Settings" }).click();
    await expect(page.getByLabel("Limit")).toBeVisible({ timeout: 8_000 });
    await expect(page.getByText("Max Concurrent Sessions")).toBeVisible();
  });
});
