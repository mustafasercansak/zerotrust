import { test, expect } from "@playwright/test";

// OIDC consent page E2E tests.
// No backend required — all API calls are intercepted via page.route().
// The ConsentPage reads OAuth2 parameters from the URL query string.
// Route: /oauth2/consent (not /auth/consent — consent is a separate route tree)

const BASE_PARAMS =
  "client_id=demo-client" +
  "&redirect_uri=http%3A%2F%2Flocalhost%3A3000%2Fcallback" +
  "&scope=openid%20profile%20email" +
  "&state=abc123" +
  "&code_challenge=challenge" +
  "&code_challenge_method=S256";

function mockClientInfo(page: Parameters<Parameters<typeof test>[1]>[0], name = "My App") {
  return page.route("**/oauth2/clients/**", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ name, allowed_scopes: ["openid", "profile", "email"] }),
    }),
  );
}

// ─── Page structure ───────────────────────────────────────────────────────────

test.describe("Consent page — structure", () => {
  test("shows application name and requested scopes", async ({ page }) => {
    await mockClientInfo(page, "My Test App");
    await page.goto(`/oauth2/consent?${BASE_PARAMS}`);

    await expect(page.getByRole("heading", { name: "Authorize Application" })).toBeVisible();
    await expect(page.getByText("My Test App")).toBeVisible({ timeout: 6_000 });
    await expect(page.getByText("Requested Permissions:")).toBeVisible();
    await expect(page.getByText("Verify your identity (OpenID)")).toBeVisible();
    await expect(page.getByText("Access your profile info (name, locale)")).toBeVisible();
    await expect(page.getByText("View your email address")).toBeVisible();
  });

  test("shows client_id as fallback when client info fetch fails", async ({ page }) => {
    await page.route("**/oauth2/clients/**", (route) =>
      route.fulfill({ status: 404, body: "" }),
    );
    await page.goto(`/oauth2/consent?${BASE_PARAMS}`);

    // Falls back to rendering the raw client_id
    await expect(page.getByText("demo-client")).toBeVisible({ timeout: 6_000 });
  });

  test("shows Authorize and Cancel buttons", async ({ page }) => {
    await mockClientInfo(page);
    await page.goto(`/oauth2/consent?${BASE_PARAMS}`);

    await expect(page.getByRole("button", { name: "Authorize" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Cancel" })).toBeVisible();
  });

  test("shows no scope list when scope param is absent", async ({ page }) => {
    await mockClientInfo(page);
    await page.goto(
      "/oauth2/consent?client_id=demo-client&redirect_uri=http%3A%2F%2Flocalhost%3A3000%2Fcallback",
    );

    await expect(page.getByText("Requested Permissions:")).not.toBeVisible();
  });
});

// ─── Authorize flow ───────────────────────────────────────────────────────────

test.describe("Consent page — authorize", () => {
  test("clicking Authorize calls consent API with approved:true", async ({ page }) => {
    await mockClientInfo(page);

    const consentRequests: unknown[] = [];
    await page.route("**/api/v1/oauth2/consent", async (route) => {
      consentRequests.push(JSON.parse((await route.request().postData()) ?? "{}"));
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          redirect_url: "http://localhost:3000/callback?code=authcode&state=abc123",
        }),
      });
    });

    await page.goto(`/oauth2/consent?${BASE_PARAMS}`);

    const responsePromise = page.waitForResponse("**/api/v1/oauth2/consent");
    await page.getByRole("button", { name: "Authorize" }).click();
    await responsePromise;

    expect(consentRequests).toHaveLength(1);
    const req = consentRequests[0] as Record<string, unknown>;
    expect(req.approved).toBe(true);
    expect(req.client_id).toBe("demo-client");
    expect(req.scopes).toEqual(["openid", "profile", "email"]);
  });

  test("clicking Cancel calls consent API with approved:false", async ({ page }) => {
    await mockClientInfo(page);

    const consentRequests: unknown[] = [];
    await page.route("**/api/v1/oauth2/consent", async (route) => {
      consentRequests.push(JSON.parse((await route.request().postData()) ?? "{}"));
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ redirect_url: "http://localhost:3000/callback?error=access_denied" }),
      });
    });

    await page.goto(`/oauth2/consent?${BASE_PARAMS}`);

    const responsePromise = page.waitForResponse("**/api/v1/oauth2/consent");
    await page.getByRole("button", { name: "Cancel" }).click();
    await responsePromise;

    const req = consentRequests[0] as Record<string, unknown>;
    expect(req.approved).toBe(false);
  });
});

// ─── Error handling ───────────────────────────────────────────────────────────

test.describe("Consent page — errors", () => {
  test("shows error toast when consent API returns an error", async ({ page }) => {
    await mockClientInfo(page);

    await page.route("**/api/v1/oauth2/consent", (route) =>
      route.fulfill({
        status: 400,
        contentType: "application/json",
        body: JSON.stringify({ error: "invalid_request" }),
      }),
    );

    await page.goto(`/oauth2/consent?${BASE_PARAMS}`);
    await page.getByRole("button", { name: "Authorize" }).click();

    const toast = page.locator("[data-sonner-toaster] li").first();
    await expect(toast).toBeVisible({ timeout: 6_000 });
  });

  test("buttons are disabled during in-flight consent request", async ({ page }) => {
    await mockClientInfo(page);

    await page.route("**/api/v1/oauth2/consent", async (route) => {
      await new Promise((resolve) => setTimeout(resolve, 2_000));
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ redirect_url: "http://localhost:3000/callback?code=x" }),
      });
    });

    await page.goto(`/oauth2/consent?${BASE_PARAMS}`);
    await page.getByRole("button", { name: "Authorize" }).click();

    await expect(page.getByRole("button", { name: "Authorize" })).toBeDisabled();
    await expect(page.getByRole("button", { name: "Cancel" })).toBeDisabled();
  });
});
