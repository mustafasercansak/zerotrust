import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { scheduleRefresh, cancelRefresh } from "@/lib/tokenManager";
import { api, ApiError, type Session } from "@/lib/api";

// How often to poll as an SSE fallback (ms).
const SESSION_POLL_INTERVAL_MS = 5_000;

function browserLabel(session: Session): string {
  const { browser, browser_version: bv } = session.device_info ?? {};
  if (!browser) return "";
  // Only show major version number (e.g. "Brave 148", not "Brave 148.0.0.0")
  const major = bv?.split(".")?.[0];
  return major ? `${browser} ${major}` : browser;
}

function osLabel(session: Session): string {
  const { os, os_version: ov, architecture: arch } = session.device_info ?? {};
  if (!os) return "";
  const parts = [ov ? `${os} ${ov}` : os, arch].filter(Boolean);
  return parts.join(" ");
}

function deviceLabel(session: Session): string {
  const br = browserLabel(session);
  const os = osLabel(session);
  if (br && os) return `${br} — ${os}`;
  return br || os || "Unknown device";
}

function sessionDeviceKey(s: Session): string {
  return [
    s.device_info?.browser ?? "",
    s.device_info?.browser_version ?? "",
    s.device_info?.os ?? "",
    s.device_info?.os_version ?? "",
    s.device_info?.architecture ?? "",
    s.device_info?.mobile ?? "",
    s.ip_address ?? "",
    s.user_agent ?? "",
  ].join("|");
}

export default function TokenRefreshProvider({ children }: { children: React.ReactNode }) {
  const { t } = useTranslation("securityEvents");
  const navigate = useNavigate();
  const { pathname } = useLocation();

  const previousSessions = useRef<Map<string, Session> | null>(null);
  const syncInProgress = useRef(false);

  useEffect(() => {
    const isAuthPage = pathname.includes("/auth/");
    let events: EventSource | null = null;
    let pollTimer: ReturnType<typeof setInterval> | null = null;
    let cancelled = false;

    async function syncSessions({ notifyChanges }: { notifyChanges: boolean }) {
      if (syncInProgress.current) return;
      syncInProgress.current = true;

      try {
        const sessions = await api.listSessions();
        if (cancelled) return;

        const current = sessions.find((s) => s.is_current);
        if (!current) {
          toast.warning(t("currentSessionRevoked"), { duration: 5000 });
          cancelRefresh();
          window.setTimeout(() => navigate("/auth/login"), 1200);
          return;
        }

        const others = sessions.filter((s) => !s.is_current);
        const next = new Map(others.map((s) => [sessionDeviceKey(s), s]));
        const prev = previousSessions.current;
        previousSessions.current = next;

        if (!notifyChanges || !prev) return;

        const added = others.filter((s) => !prev.has(sessionDeviceKey(s)));
        const removed = [...prev.entries()].filter(([k]) => !next.has(k)).map(([, s]) => s);

        if (added.length > 0) {
          const s = added[0];
          // Persistent toast — stays until the user manually closes it.
          toast.warning(t("newSession"), {
            duration: Infinity,
            description: [
              deviceLabel(s),
              s.ip_address ? `IP: ${s.ip_address}` : "",
            ]
              .filter(Boolean)
              .join("\n"),
          });
          window.dispatchEvent(new Event("sessions:changed"));
        } else if (removed.length > 0) {
          const s = removed[0];
          toast.info(t("sessionEnded"), {
            duration: 5000,
            description: [
              deviceLabel(s),
              s.ip_address ? `IP: ${s.ip_address}` : "",
            ]
              .filter(Boolean)
              .join("\n"),
          });
          window.dispatchEvent(new Event("sessions:changed"));
        }
      } catch (err: unknown) {
        if (cancelled) return;
        const code = err instanceof ApiError ? err.message : "internal_error";
        if (["missing_token", "invalid_token", "token_expired"].includes(code)) {
          toast.warning(t("currentSessionRevoked"), { duration: 5000 });
          cancelRefresh();
          window.setTimeout(() => navigate("/auth/login"), 1200);
        }
      } finally {
        syncInProgress.current = false;
      }
    }

    if (!isAuthPage) {
      const hasSession = document.cookie.includes("at_exp=");
      if (hasSession) {
        scheduleRefresh(() => navigate("/auth/login"));

        void syncSessions({ notifyChanges: previousSessions.current !== null });

        // SSE: instant fast-path (proxied through Vite dev server, which
        // supports streaming natively — no buffering issues).
        events = new EventSource("/api/v1/sessions/events");

        events.onmessage = (event) => {
          if (event.data === "revoked") {
            toast.warning(t("currentSessionRevoked"), { duration: 5000 });
            cancelRefresh();
            navigate("/auth/login", { replace: true });
            return;
          }
          if (event.data === "change") {
            void syncSessions({ notifyChanges: true });
          }
        };

        // Polling: guaranteed fallback if SSE misses an event.
        pollTimer = setInterval(() => void syncSessions({ notifyChanges: true }), SESSION_POLL_INTERVAL_MS);
      }
    } else {
      previousSessions.current = null;
    }

    return () => {
      cancelled = true;
      events?.close();
      if (pollTimer !== null) clearInterval(pollTimer);
      cancelRefresh();
    };
  }, [pathname, navigate, t]);

  return <>{children}</>;
}
