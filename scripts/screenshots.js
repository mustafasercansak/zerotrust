#!/usr/bin/env node
/**
 * scripts/screenshots.js
 *
 * Takes screenshots of every major ZeroTrust UI page and saves them to
 * docs/images/. Then regenerates docs/index.md with embedded images.
 *
 * Usage:
 *   node scripts/screenshots.js [--url http://localhost:3000] [--email admin@company.com] [--password <pass>]
 *
 * Or via Makefile:
 *   make screenshots
 *
 * Requires:
 *   npx playwright install chromium   (first time only)
 */

const path = require("path");
const fs = require("fs");

let chromium;
try {
  chromium = require(require.resolve("playwright", {
    paths: [path.resolve(__dirname, "..", "frontend", "node_modules")],
  })).chromium;
} catch (err) {
  console.error("❌ Playwright module not found.");
  console.error("   Run: cd frontend && npm install playwright --no-save && npx playwright install chromium");
  process.exit(1);
}

// ── CLI args ────────────────────────────────────────────────────────────────
const args = process.argv.slice(2);
function getArg(flag, def) {
  const i = args.indexOf(flag);
  return i !== -1 && args[i + 1] ? args[i + 1] : def;
}

const BASE_URL = getArg("--url", "http://localhost:3000");
const EMAIL    = getArg("--email", process.env.ADMIN_EMAIL || "admin@company.com");
const PASSWORD = getArg("--password", process.env.ADMIN_PASSWORD || "");
const OUT_DIR  = path.resolve(__dirname, "..", "docs", "images");

if (!PASSWORD) {
  console.error("❌  Provide admin password via --password <pass> or ADMIN_PASSWORD env var.");
  process.exit(1);
}

fs.mkdirSync(OUT_DIR, { recursive: true });

// ── Pages to screenshot ─────────────────────────────────────────────────────
const PAGES = [
  { name: "login",               path: "/auth/login",                  file: "login.png",               title: "Login Page",                    waitFor: "text=ZeroTrust" },
  { name: "dashboard",           path: "/dashboard",                   file: "dashboard.png",            title: "Dashboard Overview",            waitFor: "text=ZeroTrust" },
  { name: "mfa_setup",           path: "/dashboard/mfa",               file: "mfa_setup.png",            title: "Two-Factor Authentication",     waitFor: "text=ZeroTrust" },
  { name: "passkey_management",  path: "/dashboard/mfa",               file: "passkey_management.png",   title: "Passkey Management",            waitFor: "text=ZeroTrust" },
  { name: "sessions",            path: "/dashboard/sessions",          file: "sessions.png",             title: "Session Management",            waitFor: "text=ZeroTrust" },
  { name: "users",               path: "/dashboard/users",             file: "users.png",                title: "User Management",               waitFor: "text=ZeroTrust" },
  { name: "security_dashboard",  path: "/dashboard/security",          file: "security_dashboard.png",   title: "Security Dashboard",            waitFor: "text=ZeroTrust" },
  { name: "audit",               path: "/dashboard/audit",             file: "audit.png",                title: "Audit Log",                     waitFor: "text=ZeroTrust" },
  { name: "service_accounts",    path: "/dashboard/service-accounts",  file: "service_accounts.png",     title: "Service Accounts",              waitFor: "text=ZeroTrust" },
  { name: "oidc_clients",        path: "/dashboard/oidc-clients",      file: "oidc_clients.png",         title: "OIDC Identity Provider",        waitFor: "text=ZeroTrust" },
  { name: "settings",            path: "/dashboard/settings",          file: "settings.png",             title: "Settings",                      waitFor: "text=ZeroTrust" },
];

// ── Main ─────────────────────────────────────────────────────────────────────
(async () => {
  console.log(`📸  ZeroTrust Screenshot Tool`);
  console.log(`    Base URL : ${BASE_URL}`);
  console.log(`    Output   : ${OUT_DIR}`);
  console.log();

  const browser = await chromium.launch({
    executablePath: process.env.CHROME_BIN || "/usr/bin/google-chrome",
    headless: true,
    args: ["--no-sandbox", "--disable-setuid-sandbox", "--disable-dev-shm-usage"],
  });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    colorScheme: "dark",
    deviceScaleFactor: 1.5,
  });
  const page = await context.newPage();

  // ── Login ─────────────────────────────────────────────────────────────────
  console.log("🔑  Logging in…");
  try {
    await page.goto(`${BASE_URL}/auth/login`, { waitUntil: "load" });
  } catch (err) {
    console.error(`\n❌ Failed to connect to ${BASE_URL}.`);
    console.error(`   Make sure the application is running (e.g., 'make dev' in another terminal).`);
    process.exit(1);
  }

  // Take login page screenshot before submitting
  const loginEntry = PAGES.find(p => p.name === "login");
  if (loginEntry) {
    await page.screenshot({ path: path.join(OUT_DIR, loginEntry.file), fullPage: false });
    console.log(`  ✓ ${loginEntry.file}`);
  }

  await page.fill('input[type="email"]', EMAIL);
  await page.fill('input[type="password"]', PASSWORD);
  await page.click('button[type="submit"]');
  try {
    await page.waitForURL(`${BASE_URL}/dashboard`, { timeout: 15000 });
  } catch (err) {
    console.error(`\n❌ Login timed out. Check that the admin password is correct and no forced MFA screen interrupted the flow.`);
    process.exit(1);
  }
  await page.waitForTimeout(1500);

  // ── Screenshot each page ──────────────────────────────────────────────────
  for (const entry of PAGES) {
    if (entry.name === "login") continue; // already captured
    try {
      console.log(`  → ${entry.path}`);
      await page.goto(`${BASE_URL}${entry.path}`, { waitUntil: "load", timeout: 15000 });
      await page.waitForTimeout(2000);

      // If we are capturing MFA setup, click "Enable 2FA" to reveal the QR code and recovery codes
      if (entry.name === "mfa_setup") {
        try {
          const setupBtn = page.locator('button:has-text("Enable 2FA")');
          await setupBtn.waitFor({ state: "visible", timeout: 5000 });
          await setupBtn.click();
          // Wait for the backend to generate codes and the UI to display them
          await page.waitForSelector('text="Backup Recovery Codes"', { timeout: 5000 });
          await page.waitForTimeout(500);
        } catch (e) {}
      }

      const dest = path.join(OUT_DIR, entry.file);
      await page.screenshot({ path: dest, fullPage: false });
      console.log(`  ✓ ${entry.file}`);
    } catch (err) {
      console.warn(`  ⚠ Skipped ${entry.path}: ${err.message}`);
    }
  }

  await browser.close();
  console.log();

  // ── Generate docs/index.md ────────────────────────────────────────────────
  generateDocs();
  console.log("✅  Done. Screenshots saved to docs/images/ and docs/index.md updated.");
})();

// ── Docs generator ────────────────────────────────────────────────────────────
function generateDocs() {
  const sections = [
    {
      heading: "## Login",
      image: "images/login.png",
      caption: "Secure login page with email/password, TOTP MFA, and passwordless passkey support.",
    },
    {
      heading: "## Dashboard Overview",
      image: "images/dashboard.png",
      caption: "Central dashboard with security metrics, active sessions, and quick-access navigation.",
    },
    {
      heading: "## Two-Factor Authentication (MFA)",
      image: "images/mfa_setup.png",
      caption: "TOTP setup with QR code scan and encrypted backup recovery codes.",
    },
    {
      heading: "## Passkey Management",
      image: "images/passkey_management.png",
      caption: "Register and manage FIDO2 passkeys for phishing-resistant second-factor and passwordless login.",
    },
    {
      heading: "## Session Management",
      image: "images/sessions.png",
      caption: "View and revoke active browser sessions across all devices in real-time.",
    },
    {
      heading: "## User Management",
      image: "images/users.png",
      caption: "Admin panel for listing users, assigning roles, and managing account status.",
    },
    {
      heading: "## Security Dashboard",
      image: "images/security_dashboard.png",
      caption: "Aggregated authentication trends, lockouts, anomaly detection, login geography and failed-login sources.",
    },
    {
      heading: "## Audit Log",
      image: "images/audit.png",
      caption: "Immutable, searchable audit log of all security events with metadata.",
    },
    {
      heading: "## Service Accounts (M2M)",
      image: "images/service_accounts.png",
      caption: "Create and manage machine-to-machine OAuth2 service accounts with scoped credentials.",
    },
    {
      heading: "## OIDC Identity Provider",
      image: "images/oidc_clients.png",
      caption: "Register and manage OIDC clients. ZeroTrust acts as a standards-compliant OpenID Connect provider with roles/groups claims.",
    },
    {
      heading: "## Settings",
      image: "images/settings.png",
      caption: "Configure session policies, password complexity, MFA enforcement, and other system-wide security settings.",
    },
  ];

  const docsDir = path.resolve(__dirname, "..", "docs");
  fs.mkdirSync(docsDir, { recursive: true });

  const lines = [
    "# ZeroTrust — Screenshots",
    "",
    "> Auto-generated by `scripts/screenshots.js`. Run `make screenshots` to refresh.",
    "",
    "---",
    "",
  ];

  for (const s of sections) {
    const imgPath = path.join(docsDir, s.image);
    const exists = fs.existsSync(imgPath);
    lines.push(s.heading);
    lines.push("");
    if (exists) {
      lines.push(`![${s.caption}](${s.image})`);
    } else {
      lines.push(`> ⚠ Screenshot not yet captured. Run \`make screenshots\`.`);
    }
    lines.push("");
    lines.push(`*${s.caption}*`);
    lines.push("");
    lines.push("---");
    lines.push("");
  }

  const outPath = path.join(docsDir, "index.md");
  fs.writeFileSync(outPath, lines.join("\n"), "utf8");
  console.log(`📄  docs/index.md written (${lines.length} lines)`);
}
