import { useCallback, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { api, type AuditEntry, type PageParams } from "@/lib/api";
import { formatDateTime } from "@/lib/dateUtils";
import { useMeContext } from "@/contexts/MeContext";
import { ResourceTablePage } from "@/components/ResourceTablePage";
import Box from "@mui/material/Box";
import Chip from "@mui/material/Chip";
import Typography from "@mui/material/Typography";
import type { GridColDef } from "@mui/x-data-grid";

type AuditClientInfo = NonNullable<AuditEntry["metadata"]>["client_info"];

function browserVersion(version?: string): string {
  return version ? ` ${version.split(".")[0]}` : "";
}

function clientLabel(info?: AuditClientInfo, ua?: string | null): string {
  if (info?.browser) return `${info.browser}${browserVersion(info.browser_version)}`;
  if (!ua) return "Unknown";
  if (/Brave/i.test(ua)) return "Brave";
  if (/Edg\//.test(ua)) return "Edge";
  if (/OPR\//.test(ua)) return "Opera";
  if (/Firefox\//.test(ua)) return "Firefox";
  if (/Chrome\//.test(ua) && !/Chromium\//.test(ua)) return "Chrome";
  if (/Safari\//.test(ua) && !/Chrome\//.test(ua)) return "Safari";
  if (/curl\//i.test(ua)) return "curl";
  if (/PostmanRuntime/i.test(ua)) return "Postman";
  if (/Go-http-client/i.test(ua)) return "Go client";
  if (/python-requests/i.test(ua)) return "Python requests";
  return "API client";
}

function osLabel(info?: AuditClientInfo, ua?: string | null): string {
  if (info?.os) return [info.os, info.os_version].filter(Boolean).join(" ");
  if (!ua) return "";
  if (/Windows/.test(ua)) return "Windows";
  if (/Macintosh|Mac OS X/.test(ua)) return "macOS";
  if (/Android/.test(ua)) return "Android";
  if (/iPhone|iPad|iPod/.test(ua)) return "iOS";
  if (/Linux/.test(ua)) return "Linux";
  return "";
}

export default function AuditPage() {
  const { t } = useTranslation("audit");
  const { i18n } = useTranslation();
  const me = useMeContext();
  const isAdmin = me?.roles.includes("admin") ?? false;

  const fetcher = useCallback((p: PageParams) => api.listAuditLog(p), []);

  const columns = useMemo<GridColDef<AuditEntry>[]>(() => [
    {
      field: "action", headerName: t("action"), minWidth: 180, flex: 1,
      renderCell: ({ row }) => (
        <Typography variant="caption" color="primary" sx={{ fontFamily: "monospace" }}>{row.action}</Typography>
      ),
    },
    {
      field: "resource", headerName: t("resource"), minWidth: 180, flex: 1,
      renderCell: ({ row }) => (
        <Typography variant="caption" color="text.secondary" sx={{ fontFamily: "monospace" }}>{row.resource}</Typography>
      ),
    },
    {
      field: "user_id", headerName: t("user"), minWidth: 240, flex: 1.2,
      renderCell: ({ row }) =>
        row.user_email ? (
          <Box sx={{ display: "flex", flexDirection: "column", justifyContent: "center", py: 0.5 }}>
            <Typography variant="body2" sx={{ lineHeight: 1.4 }}>{row.user_email}</Typography>
            <Typography variant="caption" color="text.disabled" sx={{ fontFamily: "monospace", lineHeight: 1.3 }}>
              {row.user_id}
            </Typography>
          </Box>
        ) : (
          <Typography variant="caption" color="text.secondary" sx={{ fontFamily: "monospace" }}>
            {row.user_id ?? "—"}
          </Typography>
        ),
    },
    {
      field: "ip_address", headerName: t("ip"), minWidth: 140, flex: 0.7,
      renderCell: ({ row }) => (
        <Typography variant="caption" color="text.secondary" sx={{ fontFamily: "monospace" }}>{row.ip_address ?? "—"}</Typography>
      ),
    },
    {
      field: "user_agent", headerName: t("client"), minWidth: 210, flex: 0.9, sortable: false,
      renderCell: ({ row }) => {
        const info = row.metadata?.client_info;
        const os = osLabel(info, row.user_agent);
        return (
          <Box sx={{ display: "flex", alignItems: "center", gap: 0.75, minWidth: 0 }}>
            <Chip size="small" color="info" label={clientLabel(info, row.user_agent)} />
            {os && <Typography variant="caption" color="text.secondary">{os}</Typography>}
          </Box>
        );
      },
    },
    {
      field: "created_at", headerName: t("time"), minWidth: 180, flex: 0.8,
      renderCell: ({ row }) => (
        <Typography variant="caption" color="text.secondary">{formatDateTime(row.created_at, i18n.language)}</Typography>
      ),
    },
  ], [t, i18n.language]);

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
