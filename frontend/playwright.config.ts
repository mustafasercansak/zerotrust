import { defineConfig } from "@playwright/test";

// E2E tests require:
//   - Vite dev server on :3000 (started automatically if not running)
//   - Backend on :8080 for any test that makes API calls
//   - E2E_USER_EMAIL + E2E_USER_PASSWORD (no MFA) for authenticated user tests
//   - E2E_ADMIN_EMAIL + E2E_ADMIN_PASSWORD (no MFA) for admin tests
export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: "list",
  use: {
    baseURL: process.env.E2E_BASE_URL ?? "http://localhost:3000",
    // Use the system Chrome already on the machine — avoids Playwright
    // browser download and the Ubuntu system-lib dependency chain.
    channel: "chrome",
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    // The app reads `locale` from localStorage (default "tr"). Pre-seed it so
    // all tests run in English regardless of what the server profile says.
    storageState: {
      cookies: [],
      origins: [
        {
          origin: "http://localhost:3000",
          localStorage: [{ name: "locale", value: "en" }],
        },
      ],
    },
  },
  projects: [
    // Unauthenticated tests (fully mocked or public pages)
    {
      name: "unauthenticated",
      testMatch: [
        "login-page.spec.ts",
        "protected-routes.spec.ts",
        "password-reset.spec.ts",
        "consent.spec.ts",
        "dashboard.spec.ts",
        "admin-pages.spec.ts",
      ],
    },

    // Authenticated tests get a fresh, valid session per test via the
    // `authAs` fixture in e2e/fixtures.ts (one live session per role,
    // refreshed before each test — saved-cookie reuse is incompatible with
    // refresh-token rotation and the stale-initial-session janitor).
    {
      name: "user-auth",
      testMatch: "auth-flow.spec.ts",
    },
  ],
  webServer: {
    command: "npm run dev",
    url: "http://localhost:3000",
    // Reuse whatever is already listening on :3000 so `make dev` users
    // don't spin up a second Vite process.
    reuseExistingServer: true,
    timeout: 60_000,
  },
});
