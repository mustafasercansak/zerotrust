"use client";

import { useEffect, useState } from "react";
import { useTranslations, useLocale } from "next-intl";
import { useRouter } from "next/navigation";
import { api, Session } from "@/lib/api";
import { cancelRefresh } from "@/lib/tokenManager";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { formatDateTime } from "@/lib/dateUtils";

function parseUA(ua: string): string {
  if (!ua) return "Unknown";

  let os = "Unknown";
  if (/iPhone|iPad|iPod/.test(ua)) os = "iOS";
  else if (/Android/.test(ua)) os = "Android";
  else if (/Windows/.test(ua)) os = "Windows";
  else if (/Macintosh|Mac OS X/.test(ua)) os = "macOS";
  else if (/Linux/.test(ua)) os = "Linux";

  let browser = "";
  if (/Edg\//.test(ua)) browser = "Edge";
  else if (/Chrome\//.test(ua) && !/Chromium\//.test(ua)) browser = "Chrome";
  else if (/Firefox\//.test(ua)) browser = "Firefox";
  else if (/Safari\//.test(ua) && !/Chrome\//.test(ua)) browser = "Safari";

  return browser ? `${browser} on ${os}` : os;
}


export default function SessionsPage() {
  const t = useTranslations("sessions");
  const locale = useLocale();
  const router = useRouter();

  const [sessions, setSessions] = useState<Session[]>([]);
  const [loadError, setLoadError] = useState("");
  const [revoking, setRevoking] = useState<string | null>(null);

  useEffect(() => {
    api
      .listSessions()
      .then(setSessions)
      .catch(() => setLoadError(t("errors.internal_error")));
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  async function handleRevoke(session: Session) {
    const msg = session.is_current ? t("signOutThisConfirm") : t("signOutConfirm");
    if (!confirm(msg)) return;

    setRevoking(session.id);
    try {
      if (session.is_current) {
        cancelRefresh();
        await api.logout();
        router.push(`/${locale}/auth/login`);
        return;
      }
      await api.revokeSession(session.id);
      setSessions((prev) => prev.filter((s) => s.id !== session.id));
    } catch {
      setLoadError(t("errors.internal_error"));
    } finally {
      setRevoking(null);
    }
  }

  return (
    <div className="flex flex-col h-full px-8 py-6 gap-4">
      {loadError && <p className="shrink-0 text-red-400 text-sm">{loadError}</p>}

      <div className="flex-1 min-h-0 rounded-xl border border-gray-800 overflow-hidden">
        <div className="overflow-auto h-full">
          <table className="w-full text-sm">
            <thead className="sticky top-0 z-10">
              <tr className="border-b border-gray-800 bg-gray-900 text-gray-400 text-xs uppercase tracking-wider">
                <th className="px-4 py-3 text-left">{t("device")}</th>
                <th className="px-4 py-3 text-left">{t("ip")}</th>
                <th className="px-4 py-3 text-left">{t("lastActive")}</th>
                <th className="px-4 py-3 text-left">{t("signedIn")}</th>
                <th className="px-4 py-3" />
              </tr>
            </thead>
            <tbody>
              {sessions.length === 0 && !loadError && (
                <tr>
                  <td colSpan={5} className="px-4 py-10 text-center text-gray-500">
                    {t("noSessions")}
                  </td>
                </tr>
              )}
              {sessions.map((s) => (
                <tr
                  key={s.id}
                  className="border-b border-gray-800/50 last:border-0 bg-gray-950 hover:bg-gray-900/60 transition-colors"
                >
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-2">
                      <span className="text-white">{parseUA(s.user_agent)}</span>
                      {s.is_current && <Badge variant="indigo">{t("thisDevice")}</Badge>}
                    </div>
                  </td>
                  <td className="px-4 py-3 text-gray-400 font-mono text-xs">
                    {s.ip_address || "—"}
                  </td>
                  <td className="px-4 py-3 text-gray-400 text-xs">
                    {formatDateTime(s.last_used_at, locale, t("never"))}
                  </td>
                  <td className="px-4 py-3 text-gray-400 text-xs">
                    {formatDateTime(s.created_at, locale)}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <Button
                      variant="danger"
                      size="sm"
                      onClick={() => handleRevoke(s)}
                      disabled={revoking === s.id}
                    >
                      {s.is_current ? t("signOutThisDevice") : t("signOut")}
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
