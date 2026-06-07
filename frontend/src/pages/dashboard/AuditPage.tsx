import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, type AuditEntry, type AuditTrendPoint, type PageParams } from "@/lib/api";
import { formatDateTime } from "@/lib/dateUtils";
import { useMeContext } from "@/contexts/MeContext";
import { ResourceTablePage } from "@/components/ResourceTablePage";
import { getBezierPath, getBezierAreaPath } from "@/lib/chartUtils";
import Box from "@mui/material/Box";
import Chip from "@mui/material/Chip";
import CircularProgress from "@mui/material/CircularProgress";
import Paper from "@mui/material/Paper";
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

function AuditTrendsChart({ refreshSignal = 0 }: { refreshSignal?: number }) {
  const { t } = useTranslation("audit");
  const [trends, setTrends] = useState<AuditTrendPoint[] | null>(null);
  const [loading, setLoading] = useState(true);
  const hasLoaded = useRef(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(!hasLoaded.current);
    api.listAuditLogTrends()
      .then((data) => {
        if (!cancelled) setTrends(data);
      })
      .catch(() => {})
      .finally(() => {
        if (!cancelled) {
          hasLoaded.current = true;
          setLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [refreshSignal]);

  if (loading) {
    return (
      <Paper variant="outlined" sx={{ p: 3, height: 180, display: "flex", alignItems: "center", justifyContent: "center", bgcolor: "background.paper" }}>
        <CircularProgress size={20} />
      </Paper>
    );
  }

  if (!trends || trends.length === 0) return null;

  const maxVal = Math.max(...trends.map((pt) => Math.max(pt.success, pt.failure)), 1);
  const paddingX = 60;
  const paddingY = 25;
  const width = 800;
  const height = 160;
  const chartWidth = width - paddingX * 2;
  const chartHeight = height - paddingY * 2;

  const points = trends.map((pt, i) => {
    const x = paddingX + i * (chartWidth / (trends.length - 1));
    const ySuccess = height - paddingY - (pt.success / maxVal) * chartHeight;
    const yFailure = height - paddingY - (pt.failure / maxVal) * chartHeight;
    return { x, ySuccess, yFailure, ...pt };
  });

  const successPts = points.map(p => ({ x: p.x, y: p.ySuccess }));
  const successPath = getBezierPath(successPts);
  const successArea = getBezierAreaPath(successPts, height - paddingY);

  const failurePts = points.map(p => ({ x: p.x, y: p.yFailure }));
  const failurePath = getBezierPath(failurePts);
  const failureArea = getBezierAreaPath(failurePts, height - paddingY);

  return (
    <Paper variant="outlined" sx={{ p: 3, mb: 2, bgcolor: "background.paper" }}>
      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 2 }}>
        <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>
          {t("trendsTitle")}
        </Typography>
        <Box sx={{ display: "flex", gap: 2 }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 0.75 }}>
            <Box sx={{ width: 10, height: 10, borderRadius: "50%", bgcolor: "#22c55e" }} />
            <Typography variant="caption" color="text.secondary">{t("success")}</Typography>
          </Box>
          <Box sx={{ display: "flex", alignItems: "center", gap: 0.75 }}>
            <Box sx={{ width: 10, height: 10, borderRadius: "50%", bgcolor: "#f43f5e" }} />
            <Typography variant="caption" color="text.secondary">{t("failure")}</Typography>
          </Box>
        </Box>
      </Box>

      <Box sx={{ overflowX: "auto", width: "100%" }}>
        <svg viewBox={`0 0 ${width} ${height}`} width="100%" height={height} style={{ minWidth: 600, display: "block" }}>
          <defs>
            <linearGradient id="successGrad" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="#22c55e" stopOpacity="0.25" />
              <stop offset="100%" stopColor="#22c55e" stopOpacity="0.0" />
            </linearGradient>
            <linearGradient id="failureGrad" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="#f43f5e" stopOpacity="0.25" />
              <stop offset="100%" stopColor="#f43f5e" stopOpacity="0.0" />
            </linearGradient>
          </defs>

          {/* Horizontal Grid lines */}
          <line x1={paddingX} y1={paddingY} x2={width - paddingX} y2={paddingY} stroke="currentColor" opacity="0.08" strokeDasharray="3 3" />
          <line x1={paddingX} y1={paddingY + chartHeight / 2} x2={width - paddingX} y2={paddingY + chartHeight / 2} stroke="currentColor" opacity="0.08" strokeDasharray="3 3" />
          <line x1={paddingX} y1={height - paddingY} x2={width - paddingX} y2={height - paddingY} stroke="currentColor" opacity="0.08" />

          {/* Area Gradients */}
          {successArea && <path d={successArea} fill="url(#successGrad)" />}
          {failureArea && <path d={failureArea} fill="url(#failureGrad)" />}

          {/* Line paths */}
          {successPath && <path d={successPath} fill="none" stroke="#22c55e" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />}
          {failurePath && <path d={failurePath} fill="none" stroke="#f43f5e" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />}

          {/* Dots and values for Success */}
          {points.map((p, i) => (
            <g key={`succ-${i}`}>
              <circle cx={p.x} cy={p.ySuccess} r="3.5" fill="#22c55e" stroke="#0b1120" strokeWidth="1.5" />
              <text x={p.x} y={p.ySuccess - 8} textAnchor="middle" fill="#22c55e" fontSize="9" fontWeight="700">
                {p.success > 0 ? p.success : ""}
              </text>
            </g>
          ))}

          {/* Dots and values for Failure */}
          {points.map((p, i) => (
            <g key={`fail-${i}`}>
              <circle cx={p.x} cy={p.yFailure} r="3.5" fill="#f43f5e" stroke="#0b1120" strokeWidth="1.5" />
              <text x={p.x} y={p.yFailure - 8} textAnchor="middle" fill="#f43f5e" fontSize="9" fontWeight="700">
                {p.failure > 0 ? p.failure : ""}
              </text>
            </g>
          ))}

          {/* X-Axis labels */}
          {points.map((p, i) => {
            const m = p.date.split("-");
            const label = m.length >= 3 ? `${m[2]}/${m[1]}` : p.date;
            return (
              <text key={`lbl-${i}`} x={p.x} y={height - 6} textAnchor="middle" fill="currentColor" opacity="0.35" fontSize="9">
                {label}
              </text>
            );
          })}
        </svg>
      </Box>
    </Paper>
  );
}

export default function AuditPage() {
  const { t } = useTranslation("audit");
  const { i18n } = useTranslation();
  const me = useMeContext();
  const isAdmin = me?.roles.includes("admin") ?? false;
  const [trendRefresh, setTrendRefresh] = useState(0);

  const fetcher = useCallback(async (p: PageParams) => {
    const result = await api.listAuditLog(p);
    setTrendRefresh((n) => n + 1);
    return result;
  }, []);

  const tabs = useMemo(() => [
    { key: "all", label: t("tabAll") },
    { key: "failures", label: t("tabFailures"), preset: { outcome: "failure" } },
    { key: "auth", label: t("tabAuth"), preset: { action: "auth." } },
  ] as Array<{ key: string; label: string; preset?: Record<string, string> }>, [t]);

  const columns = useMemo<GridColDef<AuditEntry>[]>(() => [
    {
      field: "outcome", headerName: t("outcome"), width: 110, sortable: false,
      renderCell: ({ row }) => {
        const outcome = row.metadata?.outcome;
        if (!outcome) return null;
        return (
          <Chip
            size="small"
            label={outcome === "success" ? t("success") : t("failure")}
            color={outcome === "success" ? "success" : "error"}
            variant="outlined"
          />
        );
      },
    },
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
      field: "details", headerName: t("details"), minWidth: 160, flex: 0.8, sortable: false,
      renderCell: ({ row }) => {
        const status = row.metadata?.status;
        const reason = row.metadata?.reason;
        if (!status && !reason) return null;
        return (
          <Box sx={{ display: "flex", alignItems: "center", gap: 0.75, flexWrap: "wrap" }}>
            {status != null && (
              <Chip
                size="small"
                label={String(status)}
                variant="outlined"
                color={status >= 400 ? "warning" : "default"}
                sx={{ fontFamily: "monospace", fontSize: "0.7rem" }}
              />
            )}
            {reason && (
              <Typography variant="caption" color="text.secondary" sx={{ fontFamily: "monospace" }}>
                {reason}
              </Typography>
            )}
          </Box>
        );
      },
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
      field: "ip_address", headerName: t("ip"), minWidth: 160, flex: 0.8,
      renderCell: ({ row }) => {
        const loc = row.metadata?.location;
        const place = [loc?.city, loc?.country].filter(Boolean).join(", ");
        return (
          <Box sx={{ display: "flex", flexDirection: "column", justifyContent: "center", py: 0.5 }}>
            <Typography variant="caption" color="text.secondary" sx={{ fontFamily: "monospace", lineHeight: 1.4 }}>
              {row.ip_address ?? "—"}
            </Typography>
            {place && (
              <Typography variant="caption" color="text.disabled" sx={{ lineHeight: 1.2 }}>
                {place}
              </Typography>
            )}
          </Box>
        );
      },
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
    <Box sx={{ display: "flex", flexDirection: "column", height: "100%" }}>
      {isAdmin && <AuditTrendsChart refreshSignal={trendRefresh} />}
      <Box sx={{ flex: 1, minHeight: 0 }}>
        <ResourceTablePage
          columns={columns}
          tabs={tabs}
          fetcher={fetcher}
          getRowId={(e) => e.id}
          accessDenied={!isAdmin}
          accessDeniedMessage={t("accessDenied")}
          defaultSortKey="created_at"
          defaultSortDir="desc"
          emptyMessage={t("noEntries")}
          pageSizeOptions={[10, 25, 50]}
          defaultPageSize={25}
          rowHeight={64}
        />
      </Box>
    </Box>
  );
}
