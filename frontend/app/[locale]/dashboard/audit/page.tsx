"use client";

import { useTranslations, useLocale } from "next-intl";
import { useCallback, useMemo } from "react";
import { api, type PageParams } from "@/lib/api";
import { useMeContext } from "../context";
import { ResourceTablePage } from "@/components/ResourceTablePage";
import type { AuditEntry } from "@/lib/api";
import { formatDateTime } from "@/lib/dateUtils";
import Typography from "@mui/material/Typography";
import type { GridColDef } from "@mui/x-data-grid";

export default function AuditPage() {
  const t = useTranslations("audit");
  const locale = useLocale();
  const me = useMeContext();
  const isAdmin = me?.roles.includes("admin") ?? false;

  const fetcher = useCallback((p: PageParams) => api.listAuditLog(p), []);

  const columns = useMemo<GridColDef<AuditEntry>[]>(() => [
    {
      field: "action",
      headerName: t("action"),
      minWidth: 180,
      flex: 1,
      renderCell: ({ row }) => (
        <Typography variant="caption" color="primary" sx={{ fontFamily: "monospace" }}>
          {row.action}
        </Typography>
      ),
    },
    {
      field: "resource",
      headerName: t("resource"),
      minWidth: 180,
      flex: 1,
      renderCell: ({ row }) => (
        <Typography variant="caption" color="text.secondary" sx={{ fontFamily: "monospace" }}>
          {row.resource}
        </Typography>
      ),
    },
    {
      field: "user_id",
      headerName: t("user"),
      minWidth: 220,
      flex: 1,
      renderCell: ({ row }) => (
        <Typography variant="caption" color="text.secondary" sx={{ fontFamily: "monospace" }}>
          {row.user_id ?? "—"}
        </Typography>
      ),
    },
    {
      field: "ip_address",
      headerName: t("ip"),
      minWidth: 140,
      flex: 0.7,
      renderCell: ({ row }) => (
        <Typography variant="caption" color="text.secondary" sx={{ fontFamily: "monospace" }}>
          {row.ip_address ?? "—"}
        </Typography>
      ),
    },
    {
      field: "created_at",
      headerName: t("time"),
      minWidth: 180,
      flex: 0.8,
      renderCell: ({ row }) => (
        <Typography variant="caption" color="text.secondary">
          {formatDateTime(row.created_at, locale)}
        </Typography>
      ),
    },
  ], [t, locale]);

  return (
    <ResourceTablePage
      columns={columns}
      fetcher={fetcher}
      getRowId={(e) => e.id}
      accessDenied={!isAdmin}
      accessDeniedMessage={t("accessDenied")}
      defaultSortKey="created_at"
      defaultSortDir="desc"
      emptyMessage={t("noEntries")}
      pageSizeOptions={[10, 25, 50]}
      defaultPageSize={25}
    />
  );
}
