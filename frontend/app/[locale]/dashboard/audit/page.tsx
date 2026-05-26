"use client";

import { useEffect, useState, useCallback } from "react";
import { useTranslations } from "next-intl";
import { api, AuditEntry } from "@/lib/api";
import { useMeContext } from "../context";

const PAGE_SIZE = 50;

function formatDate(iso: string | null | undefined): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;
  return new Intl.DateTimeFormat(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(d);
}

export default function AuditPage() {
  const t = useTranslations("audit");
  const me = useMeContext();
  const isAdmin = me?.roles.includes("admin") ?? false;

  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const [offset, setOffset] = useState(0);
  const [hasMore, setHasMore] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [loading, setLoading] = useState(false);

  const load = useCallback(async (nextOffset: number, replace: boolean) => {
    setLoading(true);
    try {
      const page = await api.listAuditLog(PAGE_SIZE, nextOffset);
      setEntries((prev) => (replace ? page : [...prev, ...page]));
      setOffset(nextOffset + page.length);
      setHasMore(page.length === PAGE_SIZE);
    } catch {
      setLoadError(t("errors.internal_error"));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    if (isAdmin) load(0, true);
  }, [isAdmin, load]);

  if (!isAdmin) {
    return (
      <div className="p-8">
        <p className="text-red-400">{t("accessDenied")}</p>
      </div>
    );
  }

  return (
    <div className="px-8 py-8 space-y-6">
      <h1 className="text-2xl font-bold text-white">{t("title")}</h1>

      {loadError && <p className="text-red-400 text-sm">{loadError}</p>}

      <div className="rounded-xl border border-gray-800 overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-gray-900 text-gray-400 text-xs uppercase tracking-wider">
            <tr>
              <th className="px-4 py-3 text-left">{t("action")}</th>
              <th className="px-4 py-3 text-left">{t("user")}</th>
              <th className="px-4 py-3 text-left">{t("ip")}</th>
              <th className="px-4 py-3 text-left">{t("time")}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-800">
            {entries.length === 0 && !loading && (
              <tr>
                <td colSpan={4} className="px-4 py-6 text-center text-gray-500">
                  {t("noEntries")}
                </td>
              </tr>
            )}
            {entries.map((e) => (
              <tr key={e.id} className="bg-gray-950 hover:bg-gray-900 transition-colors">
                <td className="px-4 py-3">
                  <span className="font-mono text-xs text-indigo-300">{e.action}</span>
                </td>
                <td className="px-4 py-3 text-gray-400 text-xs font-mono">
                  {e.user_id ?? "—"}
                </td>
                <td className="px-4 py-3 text-gray-400 text-xs font-mono">
                  {e.ip_address ?? "—"}
                </td>
                <td className="px-4 py-3 text-gray-400 text-xs">
                  {formatDate(e.created_at)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {hasMore && (
        <div className="text-center">
          <button
            onClick={() => load(offset, false)}
            disabled={loading}
            className="px-4 py-2 text-sm text-gray-400 hover:text-white border border-gray-700 hover:border-gray-500 rounded-lg transition-colors disabled:opacity-40"
          >
            {loading ? "..." : t("loadMore")}
          </button>
        </div>
      )}
    </div>
  );
}
