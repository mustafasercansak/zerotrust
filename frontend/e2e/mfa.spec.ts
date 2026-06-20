import { test, expect } from "@playwright/test";

// MFA page E2E tests. Requires:
//   - Backend running at :8080
//   - E2E_USER_EMAIL + E2E_USER_PASSWORD  →  a regular user (no MFA set up)
//
// These tests verify the MFA page structure. They do not scan QR codes or
// submit TOTP codes — that requires a real authenticator and would be
// non-deterministic in CI.

const hasUserCreds = !!process.env.E2E_USER_EMAIL && !!process.env.E2E_USER_PASSWORD;

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

test.describe("MFA page", () => {
  test.skip(!hasUserCreds, "Set E2E_USER_EMAIL and E2E_USER_PASSWORD to run");

  test.beforeEach(async ({ page }) => {
    await forceEnglish(page);
    await page.goto("/dashboard/mfa");
    await expect(page).toHaveURL(/dashboard\/mfa/, { timeout: 8_000 });
  });

  test("shows TOTP status section", async ({ page }) => {
    await expect(page.getByText("Two-Factor Authentication")).toBeVisible({ timeout: 8_000 });
    // Status is either Enabled or Disabled
    const status = page.locator("text=Enabled, text=Disabled").first();
    // More robustly: one of the status labels must be present
    await expect(
      page.getByText("Enabled").or(page.getByText("Disabled")),
    ).toBeVisible({ timeout: 8_000 });
  });

  test("shows passkeys section", async ({ page }) => {
    await expect(page.getByText("Passkeys")).toBeVisible({ timeout: 8_000 });
  });

  test("enable 2FA button or manage button is present depending on status", async ({ page }) => {
    // Either "Enable 2FA" (disabled state) or the setup form is already visible
    const enableBtn = page.getByRole("button", { name: "Enable 2FA" });
    const disableBtn = page.getByRole("button", { name: /Disable/i });
    await expect(enableBtn.or(disableBtn)).toBeVisible({ timeout: 8_000 });
  });

  test("add passkey button is present", async ({ page }) => {
    await expect(page.getByRole("button", { name: /Add a passkey/i })).toBeVisible({
      timeout: 8_000,
    });
  });
});
