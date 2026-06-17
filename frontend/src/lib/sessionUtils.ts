import type { Session } from "./api";

/**
 * Returns a human-readable label for the device that created a session.
 * Prefers structured `device_info` from the server, falls back to UA-string
 * parsing so the function works even without enriched data.
 */
export function formatSessionDevice(s: Session): string {
  const di = s.device_info;

  // ── Structured path (preferred) ────────────────────────────────────────────
  if (di?.browser) {
    const major = di.browser_version?.split(".")?.[0];
    const browserStr = major ? `${di.browser} ${major}` : di.browser;
    const osStr = [di.os_version ? `${di.os} ${di.os_version}` : di.os, di.architecture]
      .filter(Boolean)
      .join(" ");
    if (browserStr && osStr) return `${browserStr} — ${osStr}`;
    return browserStr || osStr || "Unknown device";
  }

  // ── UA-string fallback ──────────────────────────────────────────────────────
  const ua = s.user_agent ?? "";
  if (!ua) return "Unknown device";

  let os = "Unknown";
  if (/iPhone|iPad|iPod/.test(ua)) os = "iOS";
  else if (/Android/.test(ua)) os = "Android";
  else if (/Windows/.test(ua)) os = "Windows";
  else if (/Macintosh|Mac OS X/.test(ua)) os = "macOS";
  else if (/Linux/.test(ua)) os = "Linux";

  let browser = "";
  if (/OPR\//.test(ua)) browser = "Opera";
  else if (/Edg\//.test(ua)) browser = "Edge";
  else if (/Chrome\//.test(ua) && !/Chromium\//.test(ua)) browser = "Chrome";
  else if (/Firefox\//.test(ua)) browser = "Firefox";
  else if (/Safari\//.test(ua) && !/Chrome\//.test(ua)) browser = "Safari";

  return browser ? `${browser} — ${os}` : os;
}
