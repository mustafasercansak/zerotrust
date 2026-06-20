import { test, expect } from "@playwright/test";

// These tests only load the login page — no backend API calls are made.
// They pass with just the Vite dev server running.

test.describe("Login page", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/auth/login");
  });

  test("shows heading, email/password fields and sign-in button", async ({ page }) => {
    await expect(page.getByRole("heading", { name: "Secure Access" })).toBeVisible();
    await expect(page.getByLabel("Email")).toBeVisible();
    await expect(page.getByLabel("Password")).toBeVisible();
    await expect(page.getByRole("button", { name: "Sign In", exact: true })).toBeVisible();
  });

  test("forgot password link navigates to reset page", async ({ page }) => {
    await page.getByRole("link", { name: "Forgot password?" }).click();
    await page.waitForURL("**/auth/forgot-password");
    await expect(page.getByLabel("Email")).toBeVisible();
    await expect(page.getByRole("button", { name: "Send reset link" })).toBeVisible();
  });

  test("sign-in button is disabled while loading", async ({ page }) => {
    // Hold the login request open for 2s so we can observe the disabled state.
    // Use a CSS selector so the locator stays valid when the button text changes to "...".
    await page.route("**/api/v1/auth/login", async (route) => {
      await new Promise((resolve) => setTimeout(resolve, 2_000));
      await route.fulfill({ status: 401, contentType: "application/json", body: JSON.stringify({ message: "invalid_credentials" }) });
    });

    await page.getByLabel("Email").fill("test@example.com");
    await page.getByLabel("Password").fill("password");

    const submitBtn = page.locator("form button[type='submit']");
    await submitBtn.click();
    // Button text changes to "..." and becomes disabled during the in-flight request.
    await expect(submitBtn).toHaveText("...");
    await expect(submitBtn).toBeDisabled();
  });
});
