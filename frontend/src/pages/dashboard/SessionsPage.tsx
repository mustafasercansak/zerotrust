import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { api, ApiError, type AuditEntry, type PageParams, type PagedResult, type Session } from "@/lib/api";
import { cancelRefresh } from "@/lib/tokenManager";
import { ResourceTablePage } from "@/components/ResourceTablePage";
import { formatDateTime } from "@/lib/dateUtils";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import Tab from "@mui/material/Tab";
import Tabs from "@mui/material/Tabs";
import Typography from "@mui/material/Typography";
import type { GridColDef } from "@mui/x-data-grid";

function parseUA(ua: string, deviceInfo?: Session["device_info"]): string {
  if (!ua) return "Unknown";

  let os = deviceInfo?.os ?? "Unknown";
  let osVersion = deviceInfo?.os_version ?? "";
  if (!deviceInfo?.os) {
    if (/iPhone|iPad|iPod/.test(ua)) { os = "iOS"; osVersion = ua.match(/OS ([\d_]+)/)?.[1]?.replaceAll("_", ".") ?? ""; }
    else if (/Android/.test(ua)) { os = "Android"; osVersion = ua.match(/Android ([\d.]+)/)?.[1] ?? ""; }
    else if (/Windows/.test(ua)) { os = "Windows"; const v = ua.match(/Windows NT ([\d.]+)/)?.[1]; osVersion = v === "10.0" ? "10/11" : v ?? ""; }
    else if (/Macintosh|Mac OS X/.test(ua)) { os = "macOS"; osVersion = ua.match(/Mac OS X ([\d_]+)/)?.[1]?.replaceAll("_", ".") ?? ""; }
    else if (/Linux/.test(ua)) os = "Linux";
  }
  const osLabel = osVersion ? `${os} ${osVersion}` : os;

  let browser = deviceInfo?.browser ?? "";
  let bv = deviceInfo?.browser_version ?? "";
  if (!browser) {
    if (/OPR\//.test(ua)) browser = "Opera";
    else if (/Edg\//.test(ua)) browser = "Edge";
    else if (/Chrome\//.test(ua) && !/Chromium\//.test(ua)) browser = "Chrome";
    else if (/Firefox\//.test(ua)) browser = "Firefox";
    else if (/Safari\//.test(ua) && !/Chrome\//.test(ua)) browser = "Safari";
  }
  if (!bv) {
    if (browser === "Opera") bv = ua.match(/OPR\/([\d.]+)/)?.[1] ?? "";
    else if (browser === "Edge") bv = ua.match(/Edg\/([\d.]+)/)?.[1] ?? "";
    else if (browser === "Firefox") bv = ua.match(/Firefox\/([\d.]+)/)?.[1] ?? "";
    else if (browser === "Safari") bv = ua.match(/Version\/([\d.]+)/)?.[1] ?? "";
    else if (browser === "Chrome" || browser === "Brave") bv = ua.match(/Chrome\/([\d.]+)/)?.[1] ?? "";
  }
  const browserLabel = bv ? `${browser} ${bv}` : browser;
  return browser ? `${browserLabel} on ${osLabel}` : osLabel;
}

function sessionDeviceKey(s: Session): string {
  return [s.device_info?.browser ?? "", s.device_info?.browser_version ?? "", s.device_info?.os ?? "",
    s.device_info?.os_version ?? "", s.device_info?.architecture ?? "", s.device_info?.mobile ?? "",
    s.ip_address ?? "", s.user_agent ?? ""].join("|");
}

export default function SessionsPage() {
  const { t } = useTranslation("sessions");
  const { t: tAudit } = useTranslation("audit");
  const { i18n } = useTranslation();
  const navigate = useNavigate();

  const [tab, setTab] = useState(0);
  const [revoking, setRevoking] = useState<string | null>(null);
  const [revokingAll, setRevokingAll] = useState(false);
  const [otherSessionCount, setOtherSessionCount] = useState(0);
  const [refresh, setRefresh] = useState(0);
  const knownSessions = useRef<Map<string, Session> | null>(null);
  const checking = useRef(false);

  const inspectSessions = useCallback(async (notify: boolean) => {
    if (checking.current) return;
    checking.current = true;
    try {
      const sessions = await api.listSessions();
      const others = sessions.filter((s) => !s.is_current);
      const next = new Map(others.map((s) => [sessionDeviceKey(s), s]));
      const prev = knownSessions.current;
      knownSessions.current = next;
      setOtherSessionCount(next.size);

      if (notify && prev) {
        const added = others.filter((s) => !prev.has(sessionDeviceKey(s)));
        const removed = [...prev.entries()].filter(([k]) => !next.has(k)).map(([, s]) => s);
        if (added.length > 0)
          window.dispatchEvent(new CustomEvent("session:new_device", {
            detail: { device: parseUA(added[0].user_agent, added[0].device_info) },
          }));
        else if (removed.length > 0)
          window.dispatchEvent(new CustomEvent("session:ended", {
            detail: { device: parseUA(removed[0].user_agent, removed[0].device_info) },
          }));
      }

      setRefresh((n) => n + 1);
    } catch (err: unknown) {
      const code = err instanceof ApiError ? err.message : "internal_error";
      if (["missing_token", "invalid_token", "token_expired"].includes(code))
        navigate("/auth/login", { replace: true });
    } finally {
      checking.current = false;
    }
  }, [navigate]);

  useEffect(() => {
    void inspectSessions(false);
    const onChanged = () => void inspectSessions(false);
    window.addEventListener("sessions:changed", onChanged);
    const timer = window.setInterval(() => void inspectSessions(true), 15_000);
    return () => { window.removeEventListener("sessions:changed", onChanged); window.clearInterval(timer); };
  }, [inspectSessions]);

  const sessionFetcher = useCallback(
    async (p: PageParams): Promise<PagedResult<Session>> => {
      void refresh;
      const all = await api.listSessions();
      const filtered = all.filter((s) =>
        Object.entries(p.filters).every(([k, v]) =>
          String(s[k as keyof Session] ?? "").toLowerCase().includes(v.toLowerCase()),
        ),
      );
      const sorted = [...filtered].sort((a, b) => {
        if (!p.sortKey) return 0;
        const l = String(a[p.sortKey as keyof Session] ?? "");
        const r = String(b[p.sortKey as keyof Session] ?? "");
        return p.sortDir === "desc" ? r.localeCompare(l) : l.localeCompare(r);
      });
      const start = p.page * p.pageSize;
      return { data: sorted.slice(start, start + p.pageSize), total: sorted.length };
    },
    [refresh],
  );

  const auditFetcher = useCallback(async (p: PageParams): Promise<PagedResult<AuditEntry>> => {
    return api.listMyAudit(p.pageSize, p.page * p.pageSize);
  }, []);

  const sessionColumns = useMemo<GridColDef<Session>[]>(() => [
    {
      field: "user_agent", headerName: t("device"), flex: 1.4, minWidth: 220,
      renderCell: ({ row }) => (
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, height: "100%" }}>
          <Typography variant="body2">{parseUA(row.user_agent, row.device_info)}</Typography>
          {row.is_current && <Chip size="small" color="primary" label={t("thisDevice")} data-testid="chip-current-session" />}
        </Box>
      ),
    },
    {
      field: "ip_address", headerName: t("ip"), flex: 0.8, minWidth: 150,
      renderCell: ({ row }) => (
        <Typography variant="caption" color="text.secondary" sx={{ fontFamily: "monospace" }} data-testid="session-ip">
          {row.ip_address || "—"}
        </Typography>
      ),
    },
    {
      field: "last_used_at", headerName: t("lastActive"), flex: 0.8, minWidth: 160,
      renderCell: ({ row }) => (
        <Typography variant="caption" color="text.secondary">
          {formatDateTime(row.last_used_at, i18n.language, t("never"))}
        </Typography>
      ),
    },
    {
      field: "created_at", headerName: t("signedIn"), flex: 0.8, minWidth: 160,
      renderCell: ({ row }) => (
        <Typography variant="caption" color="text.secondary">
          {formatDateTime(row.created_at, i18n.language)}
        </Typography>
      ),
    },
    {
      field: "actions", headerName: "", sortable: false, filterable: false, width: 150, align: "right",
      renderCell: ({ row }) => (
        <Button color="error" variant="contained" size="small" onClick={() => handleRevoke(row)} disabled={revoking === row.id}>
          {row.is_current ? t("signOutThisDevice") : t("signOut")}
        </Button>
      ),
    },
  // eslint-disable-next-line react-hooks/exhaustive-deps
  ], [t, i18n.language, revoking]);

  const auditColumns = useMemo<GridColDef<AuditEntry>[]>(() => [
    {
      field: "action", headerName: t("historyAction"), flex: 1.2, minWidth: 200,
      renderCell: ({ row }) => {
        const label = tAudit(`actions.${row.action}`, { defaultValue: row.action });
        const isTranslated = label !== row.action;
        const stepUpFor = row.metadata?.step_up_for as string | undefined;
        const stepUpLabel = stepUpFor
          ? tAudit(`step_up_for.${stepUpFor}`, { defaultValue: stepUpFor })
          : undefined;
        const localeFrom = row.metadata?.from as string | undefined;
        const localeTo = row.metadata?.to as string | undefined;
        const localeDetail = row.action === "user.locale_changed" && localeFrom && localeTo
          ? `${localeFrom.toUpperCase()} → ${localeTo.toUpperCase()}`
          : undefined;
        const subtitle = stepUpLabel ?? localeDetail;
        return (
          <Box sx={{ display: "flex", flexDirection: "column", justifyContent: "center", height: "100%" }}>
            <Typography variant="body2">{label}</Typography>
            {subtitle && (
              <Typography variant="caption" color="text.secondary" sx={{ fontSize: 11 }}>
                {stepUpLabel ? "→ " : ""}{subtitle}
              </Typography>
            )}
            {!isTranslated && !subtitle && (
              <Typography variant="caption" color="text.disabled" sx={{ fontFamily: "monospace", fontSize: 10 }}>
                {row.action}
              </Typography>
            )}
          </Box>
        );
      },
    },
    {
      field: "ip_address", headerName: t("ip"), flex: 0.7, minWidth: 140,
      renderCell: ({ row }) => (
        <Typography variant="caption" color="text.secondary" sx={{ fontFamily: "monospace" }}>
          {row.ip_address || "—"}
        </Typography>
      ),
    },
    {
      field: "metadata.outcome", headerName: t("historyOutcome"), flex: 0.5, minWidth: 110,
      sortable: false, filterable: false,
      renderCell: ({ row }) => {
        const outcome = row.metadata?.outcome as string | undefined;
        if (!outcome) return null;
        return (
          <Chip
            size="small"
            color={outcome === "success" ? "success" : outcome === "failure" ? "error" : "default"}
            label={outcome === "success" ? t("historySuccess") : outcome === "failure" ? t("historyFailure") : outcome}
          />
        );
      },
    },
    {
      field: "created_at", headerName: t("historyDate"), flex: 0.8, minWidth: 160,
      renderCell: ({ row }) => (
        <Typography variant="caption" color="text.secondary">
          {formatDateTime(row.created_at, i18n.language)}
        </Typography>
      ),
    },
  ], [t, tAudit, i18n.language]);

  async function handleRevoke(session: Session) {
    if (!confirm(session.is_current ? t("signOutThisConfirm") : t("signOutConfirm"))) return;
    setRevoking(session.id);
    try {
      if (session.is_current) {
        cancelRefresh();
        await api.logout();
        navigate("/auth/login");
        return;
      }
      await api.revokeSession(session.id);
      setRefresh((n) => n + 1);
    } catch { toast.error(t("errors.internal_error")); }
    finally { setRevoking(null); }
  }

  async function handleRevokeOthers() {
    if (!confirm(t("signOutOthersConfirm"))) return;
    setRevokingAll(true);
    try {
      await api.revokeOtherSessions();
      toast.success(t("signedOutOthers"), { duration: 5000 });
      setRefresh((n) => n + 1);
    } catch { toast.error(t("errors.internal_error")); }
    finally { setRevokingAll(false); }
  }

  return (
    <Box sx={{ height: "100%", display: "flex", flexDirection: "column", overflow: "hidden" }}>
      <Box sx={{ borderBottom: 1, borderColor: "divider", px: 2, flexShrink: 0 }}>
        <Tabs value={tab} onChange={(_, v) => setTab(v)}>
          <Tab label={t("tabActiveSessions")} data-testid="tab-active-sessions" />
          <Tab label={t("tabLoginHistory")} data-testid="tab-login-history" />
        </Tabs>
      </Box>

      {tab === 0 && (
        <Box sx={{ flex: 1, overflow: "hidden", display: "flex", flexDirection: "column" }}>
          <ResourceTablePage
            columns={sessionColumns}
            fetcher={sessionFetcher}
            getRowId={(s) => s.id}
            defaultSortKey="last_used_at"
            defaultSortDir="desc"
            emptyMessage={t("noSessions")}
            refreshSignal={refresh}
            action={
              <Button variant="outlined" color="error" size="small" onClick={handleRevokeOthers}
                disabled={revokingAll || otherSessionCount === 0}>
                {t("signOutOthers")}
              </Button>
            }
            pageSizeOptions={[10, 25, 50]}
            defaultPageSize={25}
          />
        </Box>
      )}

      {tab === 1 && (
        <Box sx={{ flex: 1, overflow: "hidden", display: "flex", flexDirection: "column" }}>
          <ResourceTablePage
            columns={auditColumns}
            fetcher={auditFetcher}
            getRowId={(e) => e.id}
            defaultSortKey="created_at"
            defaultSortDir="desc"
            emptyMessage={t("noHistory")}
            pageSizeOptions={[25, 50, 100]}
            defaultPageSize={25}
          />
        </Box>
      )}
    </Box>
  );
}
