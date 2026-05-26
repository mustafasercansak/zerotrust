"use client";

import { useEffect, useState } from "react";
import { useTranslations, useLocale } from "next-intl";
import { useRouter } from "next/navigation";
import { api, Session } from "@/lib/api";
import { cancelRefresh } from "@/lib/tokenManager";

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

function formatDate(iso: string | null, never: string): string {
  if (!iso) return never;
  return new Intl.DateTimeFormat(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(iso));
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
        await api.logout(); // revokes session, blocks JTI, clears all cookies
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
    <div className="px-8 py-8 space-y-6">
      <h1 className="text-2xl font-bold text-white">{t("title")}</h1>

      {loadError && <p className="text-red-400 text-sm">{loadError}</p>}

      <div className="rounded-xl border border-gray-800 overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-gray-900 text-gray-400 text-xs uppercase tracking-wider">
            <tr>
              <th className="px-4 py-3 text-left">{t("device")}</th>
              <th className="px-4 py-3 text-left">{t("ip")}</th>
              <th className="px-4 py-3 text-left">{t("lastActive")}</th>
              <th className="px-4 py-3 text-left">{t("signedIn")}</th>
              <th className="px-4 py-3" />
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-800">
            {sessions.length === 0 && !loadError && (
              <tr>
                <td colSpan={5} className="px-4 py-6 text-center text-gray-500">
                  {t("noSessions")}
                </td>
              </tr>
            )}
            {sessions.map((s) => (
              <tr key={s.id} className="bg-gray-950 hover:bg-gray-900 transition-colors">
                <td className="px-4 py-3">
                  <div className="flex items-center gap-2">
                    <span className="text-white">{parseUA(s.user_agent)}</span>
                    {s.is_current && (
                      <span className="text-xs px-2 py-0.5 rounded-full bg-indigo-900/60 text-indigo-300 border border-indigo-700">
                        {t("thisDevice")}
                      </span>
                    )}
                  </div>
                </td>
                <td className="px-4 py-3 text-gray-400 font-mono text-xs">
                  {s.ip_address || "—"}
                </td>
                <td className="px-4 py-3 text-gray-400 text-xs">
                  {formatDate(s.last_used_at, t("never"))}
                </td>
                <td className="px-4 py-3 text-gray-400 text-xs">
                  {formatDate(s.created_at, "—")}
                </td>
                <td className="px-4 py-3 text-right">
                  <button
                    onClick={() => handleRevoke(s)}
                    disabled={revoking === s.id}
                    className="text-xs text-red-400 hover:text-red-300 disabled:opacity-40 transition-colors"
                  >
                    {s.is_current ? t("signOutThisDevice") : t("signOut")}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
