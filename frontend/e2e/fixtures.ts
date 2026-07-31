import { test as base, expect, type Cookie, type Page } from "@playwright/test";

// Authenticated-session fixture for tests that need a real logged-in browser.
//
// The old "login once in a setup project, save cookies to e2e/.auth/*.json,
// reuse across tests" pattern is fundamentally incompatible with two backend
// behaviors:
//   1. Refresh-token rotation — a saved refresh token is valid exactly once,
//      so only the first test loading the saved state would survive; every
//      later replay of the rotated token is a correct 401.
//   2. The stale-initial-session janitor (session.RevokeStaleInitialSessions)
//      revokes any session whose access token is never refreshed within 90 s
//      of login — a saved session with no live browser dies before the
//      authenticated project even starts.
//
// Instead, each role keeps ONE live session per worker, the way a real
// browser tab behaves: it is refreshed (rotated) before each test and only
// re-created via a fresh login when refresh fails (e.g. after the UI logout
// test revokes it). This gives every test a fresh, valid access token while
// keeping logins at ~1 per role per run — far below the 10/min/IP login
// rate limit (refreshes have their own 30/min budget, also not approached).

export type Role = "user" | "admin";

const credentials: Record<Role, { email: string | undefined; password: string | undefined }> = {
  user: { email: process.env.E2E_USER_EMAIL, password: process.env.E2E_USER_PASSWORD },
  admin: { email: process.env.E2E_ADMIN_EMAIL, password: process.env.E2E_ADMIN_PASSWORD },
};

const sessions = new Map<Role, Cookie[]>();

// Replay the cached session cookies and rotate them via the refresh endpoint,
// mirroring the SPA's proactive refresh. Returns false when there is no cached
// session or the session is no longer valid (rotated elsewhere, revoked).
async function refreshSession(page: Page, role: Role): Promise<boolean> {
  const cached = sessions.get(role);
  if (!cached) return false;
  await page.context().addCookies(cached);
  const csrf = cached.find((c) => c.name === "csrf_token")?.value ?? "";
  const res = await page.request.post("/api/v1/auth/refresh", {
    headers: { "X-CSRF-Token": csrf },
    data: {},
  });
  if (!res.ok()) return false;
  sessions.set(role, await page.context().cookies());
  return true;
}

async function loginSession(page: Page, role: Role): Promise<void> {
  const { email, password } = credentials[role];
  // The CSRF middleware only exempts login when no session cookies are present.
  await page.context().clearCookies();
  const res = await page.request.post("/api/v1/auth/login", { data: { email, password } });
  expect(res.ok(), `API login as ${role} (${email}) failed with status ${res.status()}`).toBeTruthy();
  sessions.set(role, await page.context().cookies());
}

export const test = base.extend<{ authAs: (role: Role) => Promise<void> }>({
  authAs: async ({ page }, use) => {
    await use(async (role: Role) => {
      if (!(await refreshSession(page, role))) {
        await loginSession(page, role);
      }
    });
  },
});

export { expect };
