import { test, expect } from "@playwright/test";

// Tests that unauthenticated requests to protected routes redirect to the login page.
// Requires the backend to be running at :8080 — the /api/v1/me 401 triggers the redirect.

const protectedRoutes = [
  "/dashboard",
  "/dashboard/sessions",
  "/dashboard/mfa",
  "/dashboard/users",
  "/dashboard/audit",
  "/dashboard/settings",
];

for (const route of protectedRoutes) {
  test(`${route} redirects to login when unauthenticated`, async ({ page }) => {
    await page.goto(route);
    await page.waitForURL("**/auth/login**", { timeout: 8_000 });
    await expect(page.getByRole("heading", { name: "Secure Access" })).toBeVisible();
  });
}
