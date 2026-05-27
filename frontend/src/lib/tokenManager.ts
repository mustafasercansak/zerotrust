import { getClientInfo } from "./clientInfo";

// Proactive refresh — fires at 80% of the access token's remaining TTL.
const REFRESH_THRESHOLD = 0.8;

let timer: ReturnType<typeof setTimeout> | null = null;

function getCookie(name: string): string | null {
  if (typeof document === "undefined") return null;
  const m = document.cookie.match(new RegExp("(?:^|;\\s*)" + name + "=([^;]*)"));
  return m ? decodeURIComponent(m[1]) : null;
}

function getCSRFToken(): string {
  return getCookie("csrf_token") ?? "";
}

export function scheduleRefresh(onExpired?: () => void): void {
  cancelRefresh();

  const expStr = getCookie("at_exp");
  if (!expStr) {
    onExpired?.();
    return;
  }

  const exp = parseInt(expStr, 10);
  if (isNaN(exp)) return;

  const nowSec = Date.now() / 1000;
  const ttl = exp - nowSec;

  if (ttl <= 0) {
    doRefresh(onExpired);
    return;
  }

  const delayMs = ttl * REFRESH_THRESHOLD * 1000;
  timer = setTimeout(() => doRefresh(onExpired), delayMs);
}

export function cancelRefresh(): void {
  if (timer !== null) {
    clearTimeout(timer);
    timer = null;
  }
}

async function doRefresh(onExpired?: () => void): Promise<void> {
  try {
    const clientInfo = await getClientInfo();
    const res = await fetch("/api/v1/auth/refresh", {
      method: "POST",
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": getCSRFToken(),
      },
      body: JSON.stringify({ client_info: clientInfo }),
    });

    if (!res.ok) throw new Error("refresh_failed");

    scheduleRefresh(onExpired);
  } catch {
    cancelRefresh();
    onExpired?.();
  }
}
