"use client";

import { useTranslations, useLocale } from "next-intl";
import { useCallback, useMemo } from "react";
import { api, type PageParams } from "@/lib/api";
import { useMeContext } from "../context";
import { DataTable, type Column } from "@/components/DataTable";
import type { AuditEntry } from "@/lib/api";
import { formatDateTime } from "@/lib/dateUtils";

export default function AuditPage() {
  const t = useTranslations("audit");
  const locale = useLocale();
  const me = useMeContext();
  const isAdmin = me?.roles.includes("admin") ?? false;

  const fetcher = useCallback((p: PageParams) => api.listAuditLog(p), []);

  const columns = useMemo<Column<AuditEntry>[]>(() => [
    {
      key: "action",
      label: t("action"),
      sortKey: "action",
      filterKey: "action",
      render: (e) => <span className="font-mono text-xs text-indigo-300">{e.action}</span>,
    },
    {
      key: "resource",
      label: t("resource"),
      filterKey: "resource",
      render: (e) => <span className="text-gray-400 text-xs font-mono">{e.resource}</span>,
    },
    {
      key: "user_id",
      label: t("user"),
      filterKey: "user_id",
      render: (e) => <span className="text-gray-400 text-xs font-mono">{e.user_id ?? "—"}</span>,
    },
    {
      key: "ip_address",
      label: t("ip"),
      render: (e) => <span className="text-gray-400 text-xs font-mono">{e.ip_address ?? "—"}</span>,
    },
    {
      key: "created_at",
      label: t("time"),
      sortKey: "created_at",
      render: (e) => <span className="text-gray-400 text-xs">{formatDateTime(e.created_at, locale)}</span>,
    },
  ], [t, locale]);

  if (!isAdmin) {
    return <div className="p-8"><p className="text-red-400">{t("accessDenied")}</p></div>;
  }

  return (
    <div className="flex flex-col h-full px-8 py-6 gap-4">
      <DataTable
        columns={columns}
        fetcher={fetcher}
        rowKey={(e) => e.id}
        defaultSortKey="created_at"
        defaultSortDir="desc"
        emptyMessage={t("noEntries")}
        pageSizeOptions={[10, 25, 50]}
        defaultPageSize={25}
      />
    </div>
  );
}
