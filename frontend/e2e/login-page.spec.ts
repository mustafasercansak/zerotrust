import { test, expect } from "@playwright/test";

// Login page E2E tests — all API calls are mocked so no backend is required.

// ─── Helpers ──────────────────────────────────────────────────────────────────

function mockLogin(
  page: Parameters<Parameters<typeof test>[1]>[0],
  status: number,
  body: unknown,
) {
  return page.route("**/api/v1/auth/login", (route) =>
    route.fulfill({
      status,
      contentType: "application/json",
      body: JSON.stringify(body),
    }),
  );
}

async function fillAndSubmit(page: Parameters<Parameters<typeof test>[1]>[0]) {
  await page.getByTestId("login-email-input").fill("test@example.com");
  await page.getByTestId("login-password-input").fill("password123");
  await page.getByRole("button", { name: "Sign In", exact: true }).click();
}

// ─── Page structure ───────────────────────────────────────────────────────────

test.describe("Login page", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/auth/login");
  });

  test("shows heading, email/password fields and sign-in button", async ({ page }) => {
    await expect(page.getByRole("heading", { name: "Secure Access" })).toBeVisible();
    await expect(page.getByTestId("login-email-input")).toBeVisible();
    await expect(page.getByTestId("login-password-input")).toBeVisible();
    await expect(page.getByRole("button", { name: "Sign In", exact: true })).toBeVisible();
  });

  test("forgot password link navigates to reset page", async ({ page }) => {
    await page.getByRole("link", { name: "Forgot password?" }).click();
    await page.waitForURL("**/auth/forgot-password");
    await expect(page.getByTestId("forgot-email-input")).toBeVisible();
    await expect(page.getByRole("button", { name: "Send reset link" })).toBeVisible();
  });

  test("sign-in button is disabled while loading", async ({ page }) => {
    await page.route("**/api/v1/auth/login", async (route) => {
      await new Promise((resolve) => setTimeout(resolve, 2_000));
      await route.fulfill({ status: 401, contentType: "application/json", body: JSON.stringify({ error: "invalid_credentials" }) });
    });

    await page.getByTestId("login-email-input").fill("test@example.com");
    await page.getByTestId("login-password-input").fill("password");

    const submitBtn = page.locator("form button[type='submit']");
    await submitBtn.click();
    await expect(submitBtn).toHaveText("...");
    await expect(submitBtn).toBeDisabled();
  });
});

// ─── Login error responses ────────────────────────────────────────────────────

test.describe("Login — credential errors", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/auth/login");
  });

  test("shows toast on invalid credentials", async ({ page }) => {
    await mockLogin(page, 401, { error: "invalid_credentials" });
    await fillAndSubmit(page);

    const toast = page.locator("[data-sonner-toaster] li").first();
    await expect(toast).toContainText("Invalid email or password.", { timeout: 6_000 });
    await expect(page).toHaveURL(/auth\/login/);
  });

  test("shows toast when user account is deactivated", async ({ page }) => {
    await mockLogin(page, 403, { error: "user_inactive" });
    await fillAndSubmit(page);

    const toast = page.locator("[data-sonner-toaster] li").first();
    await expect(toast).toContainText("Your account has been deactivated.", { timeout: 6_000 });
  });

  test("shows toast when sign-in is blocked by IP allowlist", async ({ page }) => {
    await mockLogin(page, 403, { error: "ip_not_allowed" });
    await fillAndSubmit(page);

    const toast = page.locator("[data-sonner-toaster] li").first();
    await expect(toast).toContainText("Sign-in is not allowed from your current IP address.", { timeout: 6_000 });
  });

  test("shows toast when sign-in is blocked by country allowlist", async ({ page }) => {
    await mockLogin(page, 403, { error: "country_not_allowed" });
    await fillAndSubmit(page);

    const toast = page.locator("[data-sonner-toaster] li").first();
    await expect(toast).toContainText("Sign-in is not allowed from your current country or region.", { timeout: 6_000 });
  });

  test("shows account locked toast with minutes remaining", async ({ page }) => {
    // retry_after is in seconds; the UI converts to ceil(minutes).
    await mockLogin(page, 429, { error: "account_locked", retry_after: 300 });
    await fillAndSubmit(page);

    const toast = page.locator("[data-sonner-toaster] li").first();
    await expect(toast).toContainText("Too many failed login attempts.", { timeout: 6_000 });
    await expect(toast).toContainText("5 minutes");
  });

  test("shows rate-limit countdown on button and toast", async ({ page }) => {
    await mockLogin(page, 429, { error: "rate_limit_exceeded", retry_after: 60 });
    await fillAndSubmit(page);

    // Toast appears with seconds
    const toast = page.locator("[data-sonner-toaster] li").first();
    await expect(toast).toContainText("60", { timeout: 6_000 });

    // Button switches to countdown text and stays disabled
    const submitBtn = page.locator("form button[type='submit']");
    await expect(submitBtn).toBeDisabled();
    await expect(submitBtn).toContainText("Wait");
  });
});

// ─── MFA stage ───────────────────────────────────────────────────────────────

test.describe("Login — MFA stage", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/auth/login");
  });

  test("transitions to MFA code entry when TOTP is enabled", async ({ page }) => {
    await mockLogin(page, 200, {
      mfa_required: true,
      mfa_token: "test-mfa-token",
      totp_enabled: true,
      webauthn_enabled: false,
    });
    await fillAndSubmit(page);

    await expect(page.getByRole("heading", { name: "Two-Factor Authentication" })).toBeVisible({ timeout: 6_000 });
    await expect(page.getByTestId("mfa-code-input")).toBeVisible();
    await expect(page.getByRole("button", { name: "Verify" })).toBeVisible();
  });

  test("shows error toast when MFA code is wrong", async ({ page }) => {
    await mockLogin(page, 200, {
      mfa_required: true,
      mfa_token: "test-mfa-token",
      totp_enabled: true,
      webauthn_enabled: false,
    });
    await fillAndSubmit(page);

    // MFA stage loaded — now submit a wrong code
    await page.route("**/api/v1/auth/mfa", (route) =>
      route.fulfill({ status: 401, contentType: "application/json", body: JSON.stringify({ error: "invalid_credentials" }) }),
    );

    await page.getByTestId("mfa-code-input").fill("000000");
    await page.getByRole("button", { name: "Verify" }).click();

    const toast = page.locator("[data-sonner-toaster] li").first();
    await expect(toast).toContainText("Invalid email or password.", { timeout: 6_000 });
  });

  test("shows QR code and secret when MFA setup is required on first login", async ({ page }) => {
    await mockLogin(page, 200, {
      mfa_required: true,
      mfa_token: "test-mfa-token",
      totp_enabled: false,
      webauthn_enabled: false,
      mfa_setup_url: "otpauth://totp/ZeroTrust%3Atest%40example.com?secret=JBSWY3DPEHPK3PXP&issuer=ZeroTrust",
      mfa_setup_secret: "JBSWY3DPEHPK3PXP",
      mfa_recovery_codes: ["abcd-1234", "efgh-5678"],
    });
    await fillAndSubmit(page);

    // QR code SVG must be rendered
    await expect(page.locator("svg").first()).toBeVisible({ timeout: 6_000 });
    // Secret is shown in monospace
    await expect(page.getByText("JBSWY3DPEHPK3PXP")).toBeVisible();
    // Recovery codes listed
    await expect(page.getByText("abcd-1234")).toBeVisible();
    await expect(page.getByText("efgh-5678")).toBeVisible();
  });

  test("back button on MFA stage returns to credentials form", async ({ page }) => {
    await mockLogin(page, 200, {
      mfa_required: true,
      mfa_token: "test-mfa-token",
      totp_enabled: true,
      webauthn_enabled: false,
    });
    await fillAndSubmit(page);
    await expect(page.getByRole("heading", { name: "Two-Factor Authentication" })).toBeVisible({ timeout: 6_000 });

    await page.getByRole("button", { name: /back/i }).click();
    await expect(page.getByRole("heading", { name: "Secure Access" })).toBeVisible();
    await expect(page.getByTestId("login-email-input")).toBeVisible();
  });
});
