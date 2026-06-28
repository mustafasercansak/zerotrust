import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, type AuditEntry, type AuditTrendPoint, type PageParams } from "@/lib/api";
import { formatDateTime } from "@/lib/dateUtils";
import { useMeContext } from "@/contexts/MeContext";
import { ResourceTablePage } from "@/components/ResourceTablePage";
import { getBezierPath, getBezierAreaPath } from "@/lib/chartUtils";
import { toast } from "sonner";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import CircularProgress from "@mui/material/CircularProgress";
import Drawer from "@mui/material/Drawer";
import IconButton from "@mui/material/IconButton";
import Menu from "@mui/material/Menu";
import MenuItem from "@mui/material/MenuItem";
import Paper from "@mui/material/Paper";
import Typography from "@mui/material/Typography";
import CloseIcon from "@mui/icons-material/Close";
import ComputerIcon from "@mui/icons-material/Computer";
import DownloadIcon from "@mui/icons-material/Download";
import InfoIcon from "@mui/icons-material/Info";
import PersonIcon from "@mui/icons-material/Person";
import WarningIcon from "@mui/icons-material/Warning";
import type { GridColDef, GridRowParams } from "@mui/x-data-grid";

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

// ── Audit Trend Chart ─────────────────────────────────────────────────────────

function AuditTrendsChart({ refreshSignal = 0 }: { refreshSignal?: number }) {
  const { t } = useTranslation("audit");
  const [trends, setTrends] = useState<AuditTrendPoint[] | null>(null);
  const [loading, setLoading] = useState(true);
  const hasLoaded = useRef(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(!hasLoaded.current);
    api.listAuditLogTrends()
      .then((data) => { if (!cancelled) setTrends(data); })
      .catch(() => {})
      .finally(() => {
        if (!cancelled) { hasLoaded.current = true; setLoading(false); }
      });
    return () => { cancelled = true; };
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
  const paddingX = 60; const paddingY = 25; const width = 800; const height = 160;
  const chartWidth = width - paddingX * 2; const chartHeight = height - paddingY * 2;

  const points = trends.map((pt, i) => {
    const x = trends.length === 1 ? paddingX + chartWidth / 2 : paddingX + i * (chartWidth / (trends.length - 1));
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
        <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>{t("trendsTitle")}</Typography>
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
          <line x1={paddingX} y1={paddingY} x2={width - paddingX} y2={paddingY} stroke="currentColor" opacity="0.08" strokeDasharray="3 3" />
          <line x1={paddingX} y1={paddingY + chartHeight / 2} x2={width - paddingX} y2={paddingY + chartHeight / 2} stroke="currentColor" opacity="0.08" strokeDasharray="3 3" />
          <line x1={paddingX} y1={height - paddingY} x2={width - paddingX} y2={height - paddingY} stroke="currentColor" opacity="0.08" />
          {successArea && <path d={successArea} fill="url(#successGrad)" />}
          {failureArea && <path d={failureArea} fill="url(#failureGrad)" />}
          {successPath && <path d={successPath} fill="none" stroke="#22c55e" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />}
          {failurePath && <path d={failurePath} fill="none" stroke="#f43f5e" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />}
          {points.map((p, i) => (
            <g key={`succ-${i}`}>
              <circle cx={p.x} cy={p.ySuccess} r="3.5" fill="#22c55e" stroke="#0b1120" strokeWidth="1.5" />
              <text x={p.x} y={p.ySuccess - 8} textAnchor="middle" fill="#22c55e" fontSize="9" fontWeight="700">{p.success > 0 ? p.success : ""}</text>
            </g>
          ))}
          {points.map((p, i) => (
            <g key={`fail-${i}`}>
              <circle cx={p.x} cy={p.yFailure} r="3.5" fill="#f43f5e" stroke="#0b1120" strokeWidth="1.5" />
              <text x={p.x} y={p.yFailure - 8} textAnchor="middle" fill="#f43f5e" fontSize="9" fontWeight="700">{p.failure > 0 ? p.failure : ""}</text>
            </g>
          ))}
          {points.map((p, i) => {
            const m = p.date.split("-");
            const label = m.length >= 3 ? `${m[2]}/${m[1]}` : p.date;
            return (
              <text key={`lbl-${i}`} x={p.x} y={height - 6} textAnchor="middle" fill="currentColor" opacity="0.35" fontSize="9">{label}</text>
            );
          })}
        </svg>
      </Box>
    </Paper>
  );
}

// ── Audit Detail Drawer ───────────────────────────────────────────────────────

interface AuditDetailDrawerProps {
  entry: AuditEntry;
  onClose: () => void;
}

export function AuditDetailDrawer({ entry, onClose }: AuditDetailDrawerProps) {
  const { t } = useTranslation(["audit", "securityDashboard"]);
  const { i18n } = useTranslation();
  const [activeSection, setActiveSection] = useState<"info" | "client" | "user">("info");
  const [showRaw, setShowRaw] = useState(false);

  const loc = entry.metadata?.location;
  const location = [loc?.city, loc?.country].filter(Boolean).join(", ");
  const clientInfo = entry.metadata?.client_info;
  const outcome = entry.metadata?.outcome as string | undefined;
  const isSuccess = outcome === "success";
  const isFailure = outcome === "failure";

  const sections = [
    { key: "info" as const, icon: <InfoIcon fontSize="small" />, label: t("drawerInfo") },
    { key: "client" as const, icon: <ComputerIcon fontSize="small" />, label: t("drawerClient") },
    { key: "user" as const, icon: <PersonIcon fontSize="small" />, label: t("drawerUser") },
  ];

  return (
    <Drawer
      anchor="right"
      open
      onClose={onClose}
      slotProps={{ paper: { sx: { width: { xs: "100vw", sm: 460 }, display: "flex", flexDirection: "column" } } }}
    >
      {/* Header */}
      <Box sx={{ p: 2.5, display: "flex", alignItems: "flex-start", gap: 2, borderBottom: 1, borderColor: "divider" }}>
        <Box sx={{ flex: 1, minWidth: 0 }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 0.5, flexWrap: "wrap" }}>
            {outcome && (
              <Chip size="small" color={isSuccess ? "success" : isFailure ? "error" : "default"} label={outcome} />
            )}
          </Box>
          <Typography variant="body2" sx={{ fontWeight: 700, fontFamily: "monospace", wordBreak: "break-all" }}>
            {entry.action}
          </Typography>
          <Typography variant="caption" color="text.disabled">
            {formatDateTime(entry.created_at, i18n.language)}
          </Typography>
        </Box>
        <IconButton onClick={onClose} size="small" sx={{ mt: -0.5, flexShrink: 0 }}>
          <CloseIcon fontSize="small" />
        </IconButton>
      </Box>

      {/* Section nav */}
      <Box role="tablist" sx={{ display: "flex", borderBottom: 1, borderColor: "divider", flexShrink: 0 }}>
        {sections.map((sec) => (
          <Box
            component="button"
            type="button"
            role="tab"
            aria-selected={activeSection === sec.key}
            key={sec.key}
            onClick={() => setActiveSection(sec.key)}
            sx={{
              alignItems: "center", cursor: "pointer", display: "flex", flex: 1,
              flexDirection: "column", gap: 0.5, py: 1.25, borderBottom: 2,
              borderLeft: 0, borderRight: 0, borderTop: 0, bgcolor: "transparent",
              borderColor: activeSection === sec.key ? "primary.main" : "transparent",
              color: activeSection === sec.key ? "primary.main" : "text.secondary",
              font: "inherit",
              transition: "all 0.15s",
              "&:hover": { color: "primary.main", bgcolor: "action.hover" },
            }}
          >
            {sec.icon}
            <Typography variant="caption" sx={{ lineHeight: 1, fontSize: 10, fontWeight: activeSection === sec.key ? 700 : 400 }}>
              {sec.label}
            </Typography>
          </Box>
        ))}
      </Box>

      {/* Content */}
      <Box sx={{ flex: 1, overflow: "auto", p: 2 }}>

        {activeSection === "info" && (() => {
          const score = typeof entry.metadata?.risk_score === "number" ? entry.metadata.risk_score : undefined;
          const anomalyType = typeof entry.metadata?.anomaly_type === "string" ? entry.metadata.anomaly_type : undefined;
          const details = typeof entry.metadata?.details === "string" ? entry.metadata.details : undefined;
          
          return (
            <Box sx={{ display: "grid", gap: 2 }}>
              {score !== undefined && (
                <Paper
                  variant="outlined"
                  sx={{
                    p: 2.5,
                    display: "grid",
                    gap: 1.5,
                    border: "1px solid",
                    borderColor: (theme) => {
                      if (score >= 80) return theme.palette.error.main;
                      if (score >= 40) return theme.palette.warning.main;
                      return theme.palette.success.main;
                    },
                    bgcolor: "action.hover",
                    position: "relative",
                    overflow: "hidden",
                  }}
                >
                  <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                    <Typography variant="caption" sx={{ fontWeight: 700, color: "text.secondary", textTransform: "uppercase", letterSpacing: 0.8 }}>
                      {t("riskSeverity", { defaultValue: "Risk Level" })}
                    </Typography>
                    <Chip
                      size="small"
                      label={(() => {
                        if (score >= 80) return t("riskSeverityHigh", { defaultValue: "High Risk" });
                        if (score >= 40) return t("riskSeverityMedium", { defaultValue: "Medium Risk" });
                        return t("riskSeverityLow", { defaultValue: "Low Risk" });
                      })()}
                      color={(() => {
                        if (score >= 80) return "error";
                        if (score >= 40) return "warning";
                        return "success";
                      })()}
                      sx={{ fontWeight: "bold" }}
                    />
                  </Box>
                  
                  <Box sx={{ display: "flex", alignItems: "baseline", gap: 0.5 }}>
                    <Typography variant="h3" sx={{ fontWeight: 800, letterSpacing: -1, lineHeight: 1 }}>
                      {score}
                    </Typography>
                    <Typography variant="body2" color="text.secondary">/ 100</Typography>
                  </Box>

                  <Box sx={{ height: 6, borderRadius: 3, bgcolor: "action.selected", overflow: "hidden", width: "100%" }}>
                    <Box
                      sx={{
                        width: `${score}%`,
                        height: "100%",
                        bgcolor: (theme) => {
                          if (score >= 80) return theme.palette.error.main;
                          if (score >= 40) return theme.palette.warning.main;
                          return theme.palette.success.main;
                        },
                        borderRadius: 3,
                      }}
                    />
                  </Box>

                  {anomalyType && (
                    <Box sx={{ mt: 0.5, p: 1.5, borderRadius: 1, bgcolor: "background.paper", borderLeft: 3, borderColor: "warning.main" }}>
                      <Typography variant="subtitle2" sx={{ fontWeight: 700, fontSize: "0.8rem", color: "warning.main", mb: 0.25 }}>
                        {t("anomalousSignals", { defaultValue: "Anomalous Signals" })}: {t(`securityDashboard:anomalyTypes.${anomalyType}`, { defaultValue: anomalyType })}
                      </Typography>
                      <Typography variant="caption" color="text.secondary">
                        {details || t("anomalousEventWarning", { defaultValue: "Anomalous event flagged during authentication." })}
                      </Typography>
                    </Box>
                  )}
                </Paper>
              )}

              <Paper variant="outlined" sx={{ p: 2 }}>
                <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 700, textTransform: "uppercase", letterSpacing: 0.8, display: "block", mb: 1.5 }}>
                  {t("eventDetails")}
                </Typography>
                <Box sx={{ display: "grid", gridTemplateColumns: "110px 1fr", gap: 1 }}>
                  {([
                    { label: t("action"), value: entry.action },
                    { label: t("resource"), value: entry.resource || "—" },
                    { label: t("ip"), value: entry.ip_address || "—" },
                    ...(location ? [{ label: t("location"), value: location }] : []),
                    ...(entry.metadata?.status != null ? [{ label: t("status"), value: String(entry.metadata.status) }] : []),
                    ...(entry.metadata?.reason ? [{ label: t("reason"), value: String(entry.metadata.reason) }] : []),
                  ] as { label: string; value: string }[]).map(({ label, value }) => (
                    <React.Fragment key={label}>
                      <Typography variant="body2" color="text.secondary">{label}</Typography>
                      <Typography variant="body2" sx={{ fontFamily: "monospace", wordBreak: "break-all", fontSize: 12 }}>{value}</Typography>
                    </React.Fragment>
                  ))}
                </Box>
              </Paper>

              {/* Raw metadata toggle */}
              {entry.metadata && (
                <Box>
                  <Box onClick={() => setShowRaw((v) => !v)} sx={{ display: "flex", alignItems: "center", gap: 0.75, cursor: "pointer", mb: 1 }}>
                    <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 700, textTransform: "uppercase", letterSpacing: 0.8 }}>
                      {t("rawMetadata")}
                    </Typography>
                    <Typography variant="caption" color="primary">{showRaw ? "▲" : "▼"}</Typography>
                  </Box>
                  {showRaw && (
                    <Paper variant="outlined" sx={{ p: 1.5, bgcolor: "background.default" }}>
                      <Typography component="pre" variant="caption" sx={{ fontFamily: "monospace", whiteSpace: "pre-wrap", wordBreak: "break-all", m: 0 }}>
                        {JSON.stringify(entry.metadata, null, 2)}
                      </Typography>
                    </Paper>
                  )}
                </Box>
              )}
            </Box>
          );
        })()}

        {/* Client section */}
        {activeSection === "client" && (
          <Box sx={{ display: "grid", gap: 2 }}>
            <Paper variant="outlined" sx={{ p: 2 }}>
              <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 700, textTransform: "uppercase", letterSpacing: 0.8, display: "block", mb: 1.5 }}>
                {t("clientDetails")}
              </Typography>
              <Box sx={{ display: "grid", gridTemplateColumns: "110px 1fr", gap: 1 }}>
                {([
                  { label: t("browserLabel"), value: clientLabel(clientInfo, entry.user_agent) },
                  { label: t("osLabel"), value: osLabel(clientInfo, entry.user_agent) || "—" },
                  ...(clientInfo?.architecture ? [{ label: t("arch"), value: clientInfo.architecture }] : []),
                  ...(clientInfo?.mobile ? [{ label: t("mobile"), value: clientInfo.mobile }] : []),
                ] as { label: string; value: string }[]).map(({ label, value }) => (
                  <React.Fragment key={label}>
                    <Typography variant="body2" color="text.secondary">{label}</Typography>
                    <Typography variant="body2" sx={{ wordBreak: "break-all" }}>{value}</Typography>
                  </React.Fragment>
                ))}
              </Box>
            </Paper>
            {entry.user_agent && (
              <Paper variant="outlined" sx={{ p: 2 }}>
                <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 700, textTransform: "uppercase", letterSpacing: 0.8, display: "block", mb: 1 }}>
                  {t("userAgent")}
                </Typography>
                <Typography variant="caption" sx={{ fontFamily: "monospace", wordBreak: "break-all" }}>
                  {entry.user_agent}
                </Typography>
              </Paper>
            )}
          </Box>
        )}

        {/* User section */}
        {activeSection === "user" && (
          <Box sx={{ display: "grid", gap: 2 }}>
            {(entry.user_email || entry.user_id) ? (
              <Paper variant="outlined" sx={{ p: 2 }}>
                <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 700, textTransform: "uppercase", letterSpacing: 0.8, display: "block", mb: 1.5 }}>
                  {t("actorDetails")}
                </Typography>
                <Box sx={{ display: "grid", gridTemplateColumns: "110px 1fr", gap: 1 }}>
                  {entry.user_email && (
                    <>
                      <Typography variant="body2" color="text.secondary">{t("email")}</Typography>
                      <Typography variant="body2">{entry.user_email}</Typography>
                    </>
                  )}
                  {entry.user_id && (
                    <>
                      <Typography variant="body2" color="text.secondary">{t("userId")}</Typography>
                      <Typography variant="caption" sx={{ fontFamily: "monospace", wordBreak: "break-all" }}>{entry.user_id}</Typography>
                    </>
                  )}
                </Box>
              </Paper>
            ) : (
              <Typography variant="body2" color="text.secondary">{t("anonymousActor")}</Typography>
            )}
          </Box>
        )}
      </Box>
    </Drawer>
  );
}

// ── Main page ─────────────────────────────────────────────────────────────────

export default function AuditPage() {
  const { t } = useTranslation(["audit", "securityDashboard"]);
  const { i18n } = useTranslation();
  const me = useMeContext();
  const isAdmin = me?.roles.includes("admin") ?? false;
  const [trendRefresh, setTrendRefresh] = useState(0);
  const [detailEntry, setDetailEntry] = useState<AuditEntry | null>(null);
  const [exportAnchor, setExportAnchor] = useState<null | HTMLElement>(null);
  const [exporting, setExporting] = useState(false);
  const [exportFilters, setExportFilters] = useState<Record<string, string>>({});

  const fetcher = useCallback(async (p: PageParams) => {
    const result = await api.listAuditLog(p);
    setTrendRefresh((n) => n + 1);
    return result;
  }, []);

  const handleRowClick = useCallback((params: GridRowParams<AuditEntry>) => {
    setDetailEntry(params.row);
  }, []);

  const handleExport = useCallback(async (format: "csv" | "json") => {
    setExportAnchor(null);
    setExporting(true);
    try {
      const res = await api.admin.auditExport({ format, ...exportFilters });
      if (!res.ok) {
        toast.error(t("exportFailed", { defaultValue: t("errors.internal_error", { defaultValue: "Export failed" }) }));
        return;
      }
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `audit-log.${format}`;
      a.click();
      URL.revokeObjectURL(url);
    } catch {
      toast.error(t("exportFailed", { defaultValue: t("errors.internal_error", { defaultValue: "Export failed" }) }));
    } finally {
      setExporting(false);
    }
  }, [exportFilters, t]);

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
            label={outcome === "success" ? t("success") : outcome === "failure" ? t("failure") : String(outcome)}
            color={outcome === "success" ? "success" : outcome === "failure" ? "error" : "default"}
            variant="outlined"
          />
        );
      },
    },
    {
      field: "risk_score", headerName: t("riskScore"), width: 100, sortable: false,
      renderCell: ({ row }) => {
        const score = typeof row.metadata?.risk_score === "number" ? row.metadata.risk_score : undefined;
        if (score === undefined) return null;
        let color: "success" | "warning" | "error" = "success";
        if (score >= 80) color = "error";
        else if (score >= 40) color = "warning";
        return (
          <Chip
            size="small"
            label={`${score}`}
            color={color}
            sx={{ fontWeight: 800, width: 44, "& .MuiChip-label": { px: 0 } }}
          />
        );
      },
    },
    {
      field: "action", headerName: t("action"), minWidth: 180, flex: 1,
      renderCell: ({ row }) => (
        <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
          {row.action === "login.anomaly" && <WarningIcon sx={{ color: "error.main", fontSize: 16 }} />}
          <Typography variant="caption" color="primary" sx={{ fontFamily: "monospace" }}>{row.action}</Typography>
        </Box>
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
            {status != null && <Chip size="small" label={String(status)} variant="outlined" color={status >= 400 ? "warning" : "default"} sx={{ fontFamily: "monospace", fontSize: "0.7rem" }} />}
            {reason && <Typography variant="caption" color="text.secondary" sx={{ fontFamily: "monospace" }}>{reason}</Typography>}
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
            <Typography variant="caption" color="text.disabled" sx={{ fontFamily: "monospace", lineHeight: 1.3 }}>{row.user_id}</Typography>
          </Box>
        ) : (
          <Typography variant="caption" color="text.secondary" sx={{ fontFamily: "monospace" }}>{row.user_id ?? "—"}</Typography>
        ),
    },
    {
      field: "ip_address", headerName: t("ip"), minWidth: 160, flex: 0.8,
      renderCell: ({ row }) => {
        const loc = row.metadata?.location;
        const place = [loc?.city, loc?.country].filter(Boolean).join(", ");
        return (
          <Box sx={{ display: "flex", flexDirection: "column", justifyContent: "center", py: 0.5 }}>
            <Typography variant="caption" color="text.secondary" sx={{ fontFamily: "monospace", lineHeight: 1.4 }}>{row.ip_address ?? "—"}</Typography>
            {place && <Typography variant="caption" color="text.disabled" sx={{ lineHeight: 1.2 }}>{place}</Typography>}
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

  const exportAction = isAdmin ? (
    <>
      <Button
        size="small"
        variant="outlined"
        startIcon={<DownloadIcon />}
        disabled={exporting}
        onClick={(e) => setExportAnchor(e.currentTarget)}
        data-testid="export-button"
      >
        {t("export")}
      </Button>
      <Menu anchorEl={exportAnchor} open={Boolean(exportAnchor)} onClose={() => setExportAnchor(null)}>
        <MenuItem onClick={() => handleExport("csv")}>{t("exportCsv")}</MenuItem>
        <MenuItem onClick={() => handleExport("json")}>{t("exportJson")}</MenuItem>
      </Menu>
    </>
  ) : undefined;

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
          onRowClick={handleRowClick}
          action={exportAction}
          onFiltersChange={setExportFilters}
        />
      </Box>

      {detailEntry && (
        <AuditDetailDrawer
          entry={detailEntry}
          onClose={() => setDetailEntry(null)}
        />
      )}
    </Box>
  );
}
