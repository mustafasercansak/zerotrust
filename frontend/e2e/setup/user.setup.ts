import { test as setup, expect } from "@playwright/test";
import { fileURLToPath } from "url";
import path from "path";

const authFile = path.join(fileURLToPath(new URL(".", import.meta.url)), "../.auth/user.json");

setup("authenticate as regular user", async ({ page }) => {
  const email = process.env.E2E_USER_EMAIL;
  const password = process.env.E2E_USER_PASSWORD;

  if (!email || !password) {
    await page.context().storageState({ path: authFile });
    return;
  }

  await page.goto("/auth/login");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Sign In", exact: true }).click();
  await page.waitForURL("**/dashboard**", { timeout: 10_000 });
  await expect(page).toHaveURL(/dashboard/);

  await page.context().storageState({ path: authFile });
});
