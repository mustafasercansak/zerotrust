import { test, expect } from "@playwright/test";

// Password reset flow E2E tests.
// No backend required — all API calls are intercepted via page.route().
// Tests cover the forgot-password page and the reset-password page separately.

// ─── Forgot Password ──────────────────────────────────────────────────────────

test.describe("Forgot password page", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/auth/forgot-password");
  });

  test("shows email field and send reset link button", async ({ page }) => {
    await expect(page.getByRole("heading", { name: "Reset your password" })).toBeVisible();
    await expect(page.getByTestId("forgot-email-input")).toBeVisible();
    await expect(page.getByRole("button", { name: "Send reset link" })).toBeVisible();
  });

  test("back to sign in link navigates to login", async ({ page }) => {
    await page.getByRole("link", { name: "Back to sign in" }).click();
    await page.waitForURL("**/auth/login**");
    await expect(page.getByRole("heading", { name: "Secure Access" })).toBeVisible();
  });

  test("shows success message after submit regardless of whether email exists", async ({ page }) => {
    await page.route("**/api/v1/auth/forgot-password", (route) =>
      route.fulfill({ status: 204, body: "" }),
    );

    await page.getByTestId("forgot-email-input").fill("anybody@example.com");
    await page.getByRole("button", { name: "Send reset link" }).click();

    await expect(
      page.getByText("If that email is registered, a reset link has been sent."),
    ).toBeVisible({ timeout: 6_000 });
    // Button is gone — replaced by success state
    await expect(page.getByRole("button", { name: "Send reset link" })).not.toBeVisible();
  });

  test("submit button shows loading state and is disabled during request", async ({ page }) => {
    await page.route("**/api/v1/auth/forgot-password", async (route) => {
      await new Promise((resolve) => setTimeout(resolve, 2_000));
      await route.fulfill({ status: 204, body: "" });
    });

    await page.getByTestId("forgot-email-input").fill("test@example.com");
    const submitBtn = page.getByRole("button", { name: "Send reset link" });
    await submitBtn.click();

    await expect(page.getByRole("button", { name: "..." })).toBeDisabled();
  });

  test("shows success even when server returns an error (prevents email enumeration)", async ({ page }) => {
    await page.route("**/api/v1/auth/forgot-password", (route) =>
      route.fulfill({
        status: 404,
        contentType: "application/json",
        body: JSON.stringify({ error: "not_found" }),
      }),
    );

    await page.getByTestId("forgot-email-input").fill("nobody@example.com");
    await page.getByRole("button", { name: "Send reset link" }).click();

    // Success message must appear even on 404 — hiding user existence
    await expect(
      page.getByText("If that email is registered, a reset link has been sent."),
    ).toBeVisible({ timeout: 6_000 });
  });
});

// ─── Reset Password ───────────────────────────────────────────────────────────

test.describe("Reset password page — no token", () => {
  test("shows invalid token error immediately when token is absent", async ({ page }) => {
    await page.goto("/auth/reset-password");
    await expect(page.getByRole("alert")).toContainText("Invalid or expired link.");
    // Submit button is disabled without a token
    await expect(page.getByRole("button", { name: "Set password" })).toBeDisabled();
  });
});

test.describe("Reset password page — with token", () => {
  const RESET_URL = "/auth/reset-password?token=valid-test-token";

  test.beforeEach(async ({ page }) => {
    await page.goto(RESET_URL);
  });

  test("shows new password and confirm fields", async ({ page }) => {
    await expect(page.getByRole("heading", { name: "Set new password" })).toBeVisible();
    await expect(page.getByTestId("new-password-input")).toBeVisible();
    await expect(page.getByTestId("confirm-password-input")).toBeVisible();
    await expect(page.getByRole("button", { name: "Set password" })).toBeVisible();
  });

  test("shows mismatch error when passwords do not match", async ({ page }) => {
    await page.getByTestId("new-password-input").fill("StrongPass1!");
    await page.getByTestId("confirm-password-input").fill("DifferentPass1!");
    await page.getByRole("button", { name: "Set password" }).click();

    await expect(page.getByRole("alert")).toContainText("Passwords do not match.");
    // Should not have called the API
  });

  test("successful reset shows success message and redirects to login", async ({ page }) => {
    await page.route("**/api/v1/auth/reset-password", (route) =>
      route.fulfill({ status: 204, body: "" }),
    );

    await page.getByTestId("new-password-input").fill("StrongPass1!");
    await page.getByTestId("confirm-password-input").fill("StrongPass1!");
    await page.getByRole("button", { name: "Set password" }).click();

    await expect(page.getByText("Password updated. Redirecting to login...")).toBeVisible({
      timeout: 6_000,
    });
    // The page auto-redirects after 2 s; wait for navigation
    await page.waitForURL("**/auth/login**", { timeout: 5_000 });
  });

  test("shows expired token error when server returns invalid_token", async ({ page }) => {
    await page.route("**/api/v1/auth/reset-password", (route) =>
      route.fulfill({
        status: 400,
        contentType: "application/json",
        body: JSON.stringify({ error: "invalid_token" }),
      }),
    );

    await page.getByTestId("new-password-input").fill("StrongPass1!");
    await page.getByTestId("confirm-password-input").fill("StrongPass1!");
    await page.getByRole("button", { name: "Set password" }).click();

    await expect(page.getByRole("alert")).toContainText("Invalid or expired link.");
  });

  test("shows generic error for unexpected server failures", async ({ page }) => {
    await page.route("**/api/v1/auth/reset-password", (route) =>
      route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ error: "internal_error" }),
      }),
    );

    await page.getByTestId("new-password-input").fill("StrongPass1!");
    await page.getByTestId("confirm-password-input").fill("StrongPass1!");
    await page.getByRole("button", { name: "Set password" }).click();

    await expect(page.getByRole("alert")).toContainText("Server error. Please try again.");
  });

  test("back to sign in link is present", async ({ page }) => {
    await expect(page.getByRole("link", { name: "Back to sign in" })).toBeVisible();
  });
});
