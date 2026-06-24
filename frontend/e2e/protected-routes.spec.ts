import { test, expect } from "@playwright/test";

// Tests that unauthenticated requests to protected routes redirect to the login page.
// /api/v1/me returning 401 is what triggers the redirect — mock it so the backend
// does not need to be running for these structural tests.

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
    await page.route("**/api/v1/me", (r) =>
      r.fulfill({ status: 401, contentType: "application/json", body: JSON.stringify({ error: "missing_token" }) }),
    );
    await page.route("**/api/v1/session/policy", (r) =>
      r.fulfill({ status: 401, contentType: "application/json", body: JSON.stringify({ error: "missing_token" }) }),
    );

    await page.goto(route);
    await page.waitForURL("**/auth/login**", { timeout: 8_000 });
    await expect(page.getByRole("heading", { name: "Secure Access" })).toBeVisible();
  });
}
