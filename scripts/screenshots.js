#!/usr/bin/env node
/**
 * scripts/screenshots.js
 *
 * Takes screenshots of every major ZeroTrust UI page and saves them to
 * docs/images/. Then regenerates docs/index.md with embedded images.
 *
 * Usage:
 *   node scripts/screenshots.js [--url http://localhost:3000] [--email admin@company.com] [--password <pass>]
 *                               [--totp-secret <base32>]   # TOTP secret to auto-generate a code when MFA is required
 *                               [--totp-code   <6digits>]  # or supply the code directly (e.g. from your authenticator app)
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

const BASE_URL    = getArg("--url", "http://localhost:3000");
const EMAIL       = getArg("--email", process.env.ADMIN_EMAIL || "admin@company.com");
const PASSWORD    = getArg("--password", process.env.ADMIN_PASSWORD || "");
const TOTP_SECRET = getArg("--totp-secret", process.env.TOTP_SECRET || "");
const TOTP_CODE   = getArg("--totp-code", "");
const OUT_DIR     = path.resolve(__dirname, "..", "docs", "images");

if (!PASSWORD) {
  console.error("❌  Provide admin password via --password <pass> or ADMIN_PASSWORD env var.");
  process.exit(1);
}

fs.mkdirSync(OUT_DIR, { recursive: true });

// ── TOTP (RFC 6238 / HOTP RFC 4226) ─────────────────────────────────────────
const crypto = require("crypto");

function _base32Decode(input) {
  const CHARS = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
  const clean = input.toUpperCase().replace(/[\s=]+/g, "");
  let bits = 0, val = 0;
  const bytes = [];
  for (const ch of clean) {
    const idx = CHARS.indexOf(ch);
    if (idx < 0) continue;
    val = (val << 5) | idx;
    bits += 5;
    if (bits >= 8) { bytes.push((val >>> (bits - 8)) & 0xff); bits -= 8; }
  }
  return Buffer.from(bytes);
}

function generateTOTP(base32Secret, digits = 6, period = 30) {
  const key = _base32Decode(base32Secret);
  const counter = Math.floor(Date.now() / 1000 / period);
  const buf = Buffer.alloc(8);
  buf.writeBigUInt64BE(BigInt(counter));
  const digest = crypto.createHmac("sha1", key).update(buf).digest();
  const offset = digest[digest.length - 1] & 0x0f;
  const code = (
    ((digest[offset]     & 0x7f) << 24) |
    ((digest[offset + 1] & 0xff) << 16) |
    ((digest[offset + 2] & 0xff) << 8)  |
     (digest[offset + 3] & 0xff)
  ) % (10 ** digits);
  return String(code).padStart(digits, "0");
}

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
  { name: "settings_profile",     path: "/dashboard/settings",          file: "settings_profile.png",     title: "Settings - Profile Settings",   waitFor: "text=ZeroTrust" },
  { name: "settings_security",    path: "/dashboard/settings",          file: "settings_security.png",    title: "Settings - Security & Sessions",waitFor: "text=ZeroTrust" },
  { name: "settings_system",      path: "/dashboard/settings",          file: "settings_system.png",      title: "Settings - System Settings",    waitFor: "text=ZeroTrust" },
  { name: "settings_activity",    path: "/dashboard/settings",          file: "settings_activity.png",    title: "Settings - Login History",      waitFor: "text=ZeroTrust" },
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
  // Force English locale in every page before React initializes
  await context.addInitScript(() => {
    localStorage.setItem("locale", "en");
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

  // Take login page screenshot before submitting (wait for form to render)
  const loginEntry = PAGES.find(p => p.name === "login");
  if (loginEntry) {
    await page.waitForSelector('input[type="email"]', { state: "visible", timeout: 10000 });
    await page.waitForTimeout(500);
    await page.screenshot({ path: path.join(OUT_DIR, loginEntry.file), fullPage: false });
    console.log(`  ✓ ${loginEntry.file}`);
  }

  // Intercept the login API response to surface errors early
  let loginApiStatus = 0;
  let loginApiBody   = "";
  page.on("response", async (res) => {
    if (res.url().includes("/api/v1/auth/login") && res.request().method() === "POST") {
      loginApiStatus = res.status();
      loginApiBody   = await res.text().catch(() => "");
    }
  });

  await page.fill('input[type="email"]', EMAIL);
  await page.fill('input[type="password"]', PASSWORD);
  await page.click('button[type="submit"]');
  // Brief pause to capture any error toast before it fades
  await page.waitForTimeout(1500);
  if (page.url().includes("/auth/login")) {
    await page.screenshot({ path: path.join(OUT_DIR, "_submit_debug.png") });
  }

  // Race between reaching /dashboard (no MFA) and the TOTP input appearing (MFA required).
  // isVisible() is a point-in-time snapshot; waitFor() polls until the element is in the DOM.
  const MFA_SELECTOR = 'input[placeholder="000000 or xxxx-xxxx-xxxx"]';
  let loginStage = "unknown";
  await Promise.race([
    page.waitForURL(`${BASE_URL}/dashboard`, { timeout: 15000 })
      .then(() => { loginStage = "dashboard"; }).catch(() => {}),
    page.waitForSelector(MFA_SELECTOR, { state: "visible", timeout: 15000 })
      .then(() => { loginStage = "mfa"; }).catch(() => {}),
  ]);

  if (loginStage === "dashboard") {
    // no MFA — already there
  } else if (loginStage === "mfa") {
    let code = TOTP_CODE;
    if (!code && TOTP_SECRET) code = generateTOTP(TOTP_SECRET);
    if (!code) {
      console.error(`\n❌ MFA challenge detected. Re-run with --totp-code <6-digit-code> or --totp-secret <base32-secret>.`);
      process.exit(1);
    }
    console.log(`  ↳ MFA challenge — submitting TOTP code`);
    await page.fill(MFA_SELECTOR, code);
    await page.click('button[type="submit"]');
    try {
      await page.waitForURL(`${BASE_URL}/dashboard`, { timeout: 15000 });
      loginStage = "dashboard";
    } catch (_) {
      const debugPath = path.join(OUT_DIR, "_mfa_debug.png");
      await page.screenshot({ path: debugPath });
      console.error(`\n❌ MFA verification failed (wrong code or timeout). Debug screenshot: ${debugPath}`);
      process.exit(1);
    }
  } else {
    const debugPath = path.join(OUT_DIR, "_login_debug.png");
    await page.screenshot({ path: debugPath });
    console.error(`\n❌ Login failed.`);
    if (loginApiStatus) {
      console.error(`   API response: HTTP ${loginApiStatus}  ${loginApiBody.slice(0, 200)}`);
    } else {
      console.error(`   No login API response captured — the form may not have submitted.`);
    }
    console.error(`   Debug screenshot: ${debugPath}`);
    process.exit(1);
  }
  await page.waitForTimeout(1500);

  // ── Screenshot each page ──────────────────────────────────────────────────
  for (const entry of PAGES) {
    if (entry.name === "login") continue; // already captured
    try {
      console.log(`  → ${entry.path}`);
      // SPA-internal navigation: avoids full page reload so React and auth state persist.
      // React Router v6 listens to popstate and re-renders the matching route.
      await page.evaluate((path) => {
        window.history.pushState({}, "", path);
        window.dispatchEvent(new PopStateEvent("popstate", { state: {} }));
      }, entry.path);
      // Wait for the lazy chunk and page data to settle
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

      // Click correct settings tab based on entry name
      if (entry.name === "settings_profile") {
        try {
          const tab = page.locator("#tab-profile");
          await tab.waitFor({ state: "visible", timeout: 5000 });
          await tab.click();
          await page.waitForTimeout(1000);
          // Scroll down slightly to show locale + notification cards
          await page.evaluate(() => window.scrollBy(0, 200));
          await page.waitForTimeout(300);
        } catch (e) {}
      }
      if (entry.name === "settings_security") {
        try {
          const tab = page.locator("#tab-security");
          await tab.waitFor({ state: "visible", timeout: 5000 });
          await tab.click();
          await page.waitForTimeout(1000);
        } catch (e) {}
      }
      if (entry.name === "settings_system") {
        try {
          const tab = page.locator("#tab-system");
          await tab.waitFor({ state: "visible", timeout: 5000 });
          await tab.click();
          await page.waitForTimeout(1000);
        } catch (e) {}
      }
      if (entry.name === "settings_activity") {
        try {
          const tab = page.locator("#tab-activity");
          await tab.waitFor({ state: "visible", timeout: 5000 });
          await tab.click();
          await page.waitForTimeout(1000);
        } catch (e) {}
      }

      const dest = path.join(OUT_DIR, entry.file);
      await page.screenshot({ path: dest, fullPage: false });
      console.log(`  ✓ ${entry.file}`);
    } catch (err) {
      const debugPath = path.join(OUT_DIR, `_debug_${entry.name}.png`);
      await page.screenshot({ path: debugPath }).catch(() => {});
      console.warn(`  ⚠ Skipped ${entry.path}: ${err.message}`);
      console.warn(`     Debug screenshot: ${debugPath}`);
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
      description: "ZeroTrust implements the Authorization Code flow with PKCE (S256, RFC 7636). Registered clients receive an authorization code on user consent, exchange it for an Ed25519-signed ID token and access token, and use the `refresh_token` grant (RFC 6749 §6) with `offline_access` scope to obtain long-lived rotating refresh tokens. Tokens can be introspected via `POST /oauth2/introspect` (RFC 7662) and revoked via `POST /oauth2/revoke` (RFC 7009). The `max_age` parameter (OIDC Core §3.1.2.1) enforces re-authentication when a session exceeds a client-specified age. Live profile claims are available at `/oauth2/userinfo`. All consent decisions, token exchanges, rotations, introspections, and revocations are written to the audit log. The discovery document at `/.well-known/openid-configuration` advertises all supported endpoints, scopes, and signing algorithms.",
    },
    {
      heading: "## Settings — Profile Settings",
      image: "images/settings_profile.png",
      caption: "Manage personal profile information, upload an avatar, select language preference, and update password.",
    },
    {
      heading: "## Settings — Security & Sessions",
      image: "images/settings_security.png",
      caption: "View and manage active browser sessions across all devices for your account.",
    },
    {
      heading: "## Settings — System Settings",
      image: "images/settings_system.png",
      caption: "Configure session policies, password complexity, MFA enforcement, and hardware attestation requirements.",
    },
    {
      heading: "## Settings — Login History",
      image: "images/settings_activity.png",
      caption: "Inspect security events and authentication history recorded for your account.",
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
    if (s.description) {
      lines.push("");
      lines.push(s.description);
    }
    lines.push("");
    lines.push("---");
    lines.push("");
  }

  const outPath = path.join(docsDir, "index.md");
  fs.writeFileSync(outPath, lines.join("\n"), "utf8");
  console.log(`📄  docs/index.md written (${lines.length} lines)`);
}
