"use client";

import { useCallback, useMemo, useState } from "react";
import { useTranslations, useLocale } from "next-intl";
import { useRouter } from "next/navigation";
import { api, type PageParams, type PagedResult, type Session } from "@/lib/api";
import { cancelRefresh } from "@/lib/tokenManager";
import { ResourceTablePage } from "@/components/ResourceTablePage";
import { formatDateTime } from "@/lib/dateUtils";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import Typography from "@mui/material/Typography";
import type { GridColDef } from "@mui/x-data-grid";

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

  const [revoking, setRevoking] = useState<string | null>(null);
  const [refresh, setRefresh] = useState(0);

  const fetcher = useCallback(
    async (p: PageParams): Promise<PagedResult<Session>> => {
      void refresh;
      const all = await api.listSessions();

      const filtered = all.filter((session) =>
        Object.entries(p.filters).every(([key, value]) => {
          const haystack = String(session[key as keyof Session] ?? "").toLowerCase();
          return haystack.includes(value.toLowerCase());
        }),
      );

      const sorted = [...filtered].sort((a, b) => {
        if (!p.sortKey) return 0;
        const left = String(a[p.sortKey as keyof Session] ?? "");
        const right = String(b[p.sortKey as keyof Session] ?? "");
        const result = left.localeCompare(right);
        return p.sortDir === "desc" ? -result : result;
      });

      const start = p.page * p.pageSize;
      return {
        data: sorted.slice(start, start + p.pageSize),
        total: sorted.length,
      };
    },
    [refresh],
  );

  const columns = useMemo<GridColDef<Session>[]>(() => [
    {
      field: "user_agent",
      headerName: t("device"),
      flex: 1.4,
      minWidth: 220,
      renderCell: ({ row }) => (
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, height: "100%" }}>
          <Typography variant="body2">{parseUA(row.user_agent)}</Typography>
          {row.is_current ? <Chip size="small" color="primary" label={t("thisDevice")} /> : null}
        </Box>
      ),
    },
    {
      field: "ip_address",
      headerName: t("ip"),
      flex: 0.8,
      minWidth: 150,
      renderCell: ({ row }) => (
        <Typography variant="caption" color="text.secondary" sx={{ fontFamily: "monospace" }}>
          {row.ip_address || "—"}
        </Typography>
      ),
    },
    {
      field: "last_used_at",
      headerName: t("lastActive"),
      flex: 0.8,
      minWidth: 160,
      renderCell: ({ row }) => (
        <Typography variant="caption" color="text.secondary">
          {formatDateTime(row.last_used_at, locale, t("never"))}
        </Typography>
      ),
    },
    {
      field: "created_at",
      headerName: t("signedIn"),
      flex: 0.8,
      minWidth: 160,
      renderCell: ({ row }) => (
        <Typography variant="caption" color="text.secondary">
          {formatDateTime(row.created_at, locale)}
        </Typography>
      ),
    },
    {
      field: "actions",
      headerName: "",
      sortable: false,
      filterable: false,
      width: 150,
      align: "right",
      renderCell: ({ row }) => (
        <Button
          color="error"
          variant="contained"
          size="small"
          onClick={() => handleRevoke(row)}
          disabled={revoking === row.id}
        >
          {row.is_current ? t("signOutThisDevice") : t("signOut")}
        </Button>
      ),
    },
  // eslint-disable-next-line react-hooks/exhaustive-deps
  ], [t, locale, revoking]);

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
      setRefresh((n) => n + 1);
    } catch {
      alert(t("errors.internal_error"));
    } finally {
      setRevoking(null);
    }
  }

  return (
    <ResourceTablePage
      columns={columns}
      fetcher={fetcher}
      getRowId={(s) => s.id}
      defaultSortKey="last_used_at"
      defaultSortDir="desc"
      emptyMessage={t("noSessions")}
      pageSizeOptions={[10, 25, 50]}
      defaultPageSize={25}
    />
  );
}
