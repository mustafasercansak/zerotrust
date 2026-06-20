import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { api, type AdminHealthData, type AuditEntry, type SecurityPostureData, type Session } from "@/lib/api";
import { formatDate } from "@/lib/dateUtils";
import { useMeContext } from "@/contexts/MeContext";
import { SessionCard } from "@/components/SessionCard";
import { AuditEntryCard } from "@/components/AuditEntryCard";
import Avatar from "@mui/material/Avatar";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import Divider from "@mui/material/Divider";
import LinearProgress from "@mui/material/LinearProgress";
import Paper from "@mui/material/Paper";
import Skeleton from "@mui/material/Skeleton";
import Typography from "@mui/material/Typography";
import LockIcon from "@mui/icons-material/Lock";
import LockOpenIcon from "@mui/icons-material/LockOpen";
import VerifiedUserIcon from "@mui/icons-material/VerifiedUser";
import CheckCircleOutlineIcon from "@mui/icons-material/CheckCircle";
import WarningAmberIcon from "@mui/icons-material/WarningAmber";
import DevicesIcon from "@mui/icons-material/Devices";
import AdminPanelSettingsIcon from "@mui/icons-material/AdminPanelSettings";
import GroupIcon from "@mui/icons-material/Group";
import StorageIcon from "@mui/icons-material/Storage";
import MemoryIcon from "@mui/icons-material/Memory";

// ── Compact section wrapper ───────────────────────────────────────────────────

interface SectionProps {
  title: string;
  linkLabel?: string;
  onLink?: () => void;
  children: React.ReactNode;
}

function Section({ title, linkLabel, onLink, children }: SectionProps) {
  return (
    <Paper variant="outlined" sx={{ p: 1.5 }}>
      <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 1 }}>
        <Typography variant="subtitle2" sx={{ fontWeight: 700, textTransform: "uppercase", letterSpacing: 0.8, fontSize: 10 }}>
          {title}
        </Typography>
        {linkLabel && onLink && (
          <Button size="small" onClick={onLink} sx={{ fontSize: 10, minWidth: 0, py: 0 }}>
            {linkLabel} →
          </Button>
        )}
      </Box>
      {children}
    </Paper>
  );
}

// ── Security checklist row ────────────────────────────────────────────────────

function SecurityRow({ ok, primary, action }: {
  ok: boolean;
  primary: string;
  action?: { label: string; onClick: () => void };
}) {
  return (
    <Box sx={{ display: "flex", alignItems: "center", gap: 1, py: 0.5 }}>
      {ok
        ? <CheckCircleOutlineIcon color="success" sx={{ fontSize: 16, flexShrink: 0 }} />
        : <WarningAmberIcon color="warning" sx={{ fontSize: 16, flexShrink: 0 }} />
      }
      <Typography variant="caption" sx={{ flex: 1, lineHeight: 1.4 }}>{primary}</Typography>
      {action && (
        <Button size="small" onClick={action.onClick} sx={{ fontSize: 10, minWidth: 0, py: 0, whiteSpace: "nowrap", flexShrink: 0 }}>
          {action.label} →
        </Button>
      )}
    </Box>
  );
}

// ── Admin posture stat tile ───────────────────────────────────────────────────

function PostureStat({ icon, value, label, color }: {
  icon: React.ReactNode;
  value: number;
  label: string;
  color: "default" | "warning" | "success";
}) {
  const palette = color === "warning" ? "warning.main" : color === "success" ? "success.main" : "text.secondary";
  return (
    <Box sx={{ display: "flex", alignItems: "center", gap: 1, p: 1, borderRadius: 1, bgcolor: "background.default", border: "1px solid", borderColor: "divider" }}>
      <Box sx={{ color: palette, display: "flex", flexShrink: 0, "& svg": { fontSize: 18 } }}>{icon}</Box>
      <Box>
        <Typography variant="subtitle1" sx={{ fontWeight: 700, lineHeight: 1, color: palette }}>{value}</Typography>
        <Typography variant="caption" color="text.secondary" sx={{ fontSize: 10 }}>{label}</Typography>
      </Box>
    </Box>
  );
}

// ── Main page ─────────────────────────────────────────────────────────────────

export default function HomePage() {
  const { t } = useTranslation("homepage");
  const { i18n } = useTranslation();
  const me = useMeContext();
  const navigate = useNavigate();

  const [mfaEnabled, setMfaEnabled] = useState<boolean | null>(null);
  const [mfaLoading, setMfaLoading] = useState(true);
  const [sessions, setSessions] = useState<Session[] | null>(null);
  const [sessionsLoading, setSessionsLoading] = useState(true);
  const [audit, setAudit] = useState<AuditEntry[] | null>(null);
  const [auditLoading, setAuditLoading] = useState(true);

  const isAdmin = me?.roles.includes("admin") ?? false;
  const [posture, setPosture] = useState<SecurityPostureData | null>(null);
  const [postureLoading, setPostureLoading] = useState(isAdmin);
  const [health, setHealth] = useState<AdminHealthData | null>(null);
  const [healthLoading, setHealthLoading] = useState(isAdmin);

  useEffect(() => {
    if (!me) return;

    api.mfaStatus()
      .then((d) => setMfaEnabled(d.supported !== false ? d.enabled : null))
      .catch(() => setMfaEnabled(null))
      .finally(() => setMfaLoading(false));

    api.listSessions()
      .then((all) => setSessions(all.slice(0, 3)))
      .catch(() => setSessions([]))
      .finally(() => setSessionsLoading(false));

    api.listAuditLog({ page: 0, pageSize: 3, sortKey: "created_at", sortDir: "desc", filters: { user_id: me.user_id } })
      .then((res) => setAudit(res.data))
      .catch(() => setAudit([]))
      .finally(() => setAuditLoading(false));

    if (me.roles.includes("admin")) {
      api.admin.securityPosture()
        .then(setPosture)
        .catch(() => setPosture(null))
        .finally(() => setPostureLoading(false));

      api.admin.health()
        .then(setHealth)
        .catch(() => setHealth(null))
        .finally(() => setHealthLoading(false));
    }
  }, [me]);

  if (!me) return null;

  const name = [me.first_name, me.last_name].filter(Boolean).join(" ") || me.email;
  const initials = me.first_name?.[0]
    ? `${me.first_name[0]}${me.last_name?.[0] ?? ""}`.toUpperCase()
    : me.email.slice(0, 2).toUpperCase();

  const lastSeenEntry = !auditLoading && audit ? (audit.find((e) => !!e.ip_address) ?? null) : null;

  return (
    <Box sx={{ p: { xs: 1, sm: 1.5 }, display: "flex", flexDirection: "column", gap: 1.5 }}>

      {/* ── Row 1: Compact identity card ── */}
      <Paper
        variant="outlined"
        sx={{
          p: 1.5,
          display: "flex",
          alignItems: "center",
          gap: 2,
          background: "linear-gradient(135deg, rgba(99,102,241,0.08) 0%, transparent 100%)",
          borderColor: "rgba(99,102,241,0.25)",
        }}
      >
        <Avatar
          src={me.has_avatar ? `/api/v1/me/avatar` : undefined}
          sx={{ width: 44, height: 44, fontSize: 16, fontWeight: 700, bgcolor: "primary.dark", flexShrink: 0 }}
        >
          {initials}
        </Avatar>
        <Box sx={{ flex: 1, minWidth: 0 }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 0.75, flexWrap: "wrap" }}>
            <Typography variant="subtitle1" sx={{ fontWeight: 700, lineHeight: 1.3 }} noWrap>{name}</Typography>
            <VerifiedUserIcon color="success" sx={{ fontSize: 15 }} />
          </Box>
          {name !== me.email && (
            <Typography variant="caption" color="text.secondary" noWrap sx={{ display: "block" }}>{me.email}</Typography>
          )}
          <Box sx={{ display: "flex", gap: 0.5, mt: 0.5, flexWrap: "wrap" }}>
            {me.roles.map((r) => (
              <Chip key={r} size="small" color="primary" label={r} sx={{ height: 18, fontSize: 10, "& .MuiChip-label": { px: 0.75 } }} />
            ))}
          </Box>
        </Box>
        <Box sx={{ display: { xs: "none", sm: "grid" }, gridTemplateColumns: "auto 1fr", columnGap: 1.5, rowGap: 0.125 }}>
          <Typography variant="caption" color="text.secondary">{t("userId")}</Typography>
          <Typography variant="caption" sx={{ fontFamily: "monospace", fontSize: 10 }} noWrap>{me.user_id}</Typography>
          <Typography variant="caption" color="text.secondary">{t("locale")}</Typography>
          <Typography variant="caption">{i18n.language.toUpperCase()}</Typography>
          <Typography variant="caption" color="text.secondary">{t("joined")}</Typography>
          <Typography variant="caption">{formatDate(me.created_at, i18n.language)}</Typography>
          <Typography variant="caption" color="text.secondary">{t("modified")}</Typography>
          <Typography variant="caption">{formatDate(me.updated_at, i18n.language)}</Typography>
        </Box>
      </Paper>

      {/* ── Row 2: 3-column grid ── */}
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "1fr 1fr", lg: "1fr 1fr 1fr" }, gap: 1.5, alignItems: "start" }}>

        {/* Account security: MFA status + checklist merged */}
        <Section
          title={t("securityTitle")}
          linkLabel={mfaEnabled === false ? t("mfaSetup") : t("mfaManage")}
          onLink={() => navigate("/dashboard/mfa")}
        >
          {mfaLoading ? (
            <Skeleton variant="rounded" height={36} />
          ) : (
            <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1 }}>
              {mfaEnabled ? <LockIcon color="success" sx={{ fontSize: 20 }} /> : <LockOpenIcon color="warning" sx={{ fontSize: 20 }} />}
              <Box>
                <Chip
                  size="small"
                  color={mfaEnabled ? "success" : "warning"}
                  label={mfaEnabled ? t("enabled") : t("disabled")}
                  sx={{ height: 18, fontSize: 10, "& .MuiChip-label": { px: 0.75 } }}
                />
                <Typography variant="caption" color="text.secondary" sx={{ display: "block", fontSize: 10 }}>
                  {mfaEnabled ? t("mfaEnabled") : t("mfaDisabled")}
                </Typography>
              </Box>
            </Box>
          )}
          {!mfaLoading && !sessionsLoading && (
            <>
              <Divider sx={{ my: 0.75 }} />
              <Box>
                {mfaEnabled !== null && (
                  <SecurityRow
                    ok={mfaEnabled}
                    primary={mfaEnabled ? t("securityMfaOn") : t("securityMfaOff")}
                    action={!mfaEnabled ? { label: t("mfaSetup"), onClick: () => navigate("/dashboard/mfa") } : undefined}
                  />
                )}
                {sessions && sessions.length > 0 && (
                  <SecurityRow
                    ok={sessions.length === 1}
                    primary={sessions.length === 1 ? t("securityOneSession") : t("securityManySessions", { count: sessions.length })}
                    action={sessions.length > 1 ? { label: t("sessionsViewAll"), onClick: () => navigate("/dashboard/sessions") } : undefined}
                  />
                )}
                {lastSeenEntry && (
                  <SecurityRow
                    ok
                    primary={t("securityLastSeen", { ip: lastSeenEntry.ip_address, date: formatDate(lastSeenEntry.created_at, i18n.language) })}
                  />
                )}
              </Box>
            </>
          )}
          {(mfaLoading || sessionsLoading) && !mfaLoading && (
            <Skeleton variant="rounded" height={60} sx={{ mt: 1 }} />
          )}
        </Section>

        {/* Active sessions snapshot */}
        <Section
          title={t("sessionsTitle")}
          linkLabel={t("sessionsViewAll")}
          onLink={() => navigate("/dashboard/sessions")}
        >
          {sessionsLoading && <LinearProgress />}
          {!sessionsLoading && sessions?.length === 0 && (
            <Box sx={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 0.5, py: 1.5, color: "text.disabled" }}>
              <DevicesIcon sx={{ fontSize: 28 }} />
              <Typography variant="caption" align="center">{t("noSessions")}</Typography>
            </Box>
          )}
          {sessions && sessions.length > 0 && (
            <Box sx={{ display: "grid", gap: 0.75 }}>
              {sessions.map((s) => (
                <SessionCard key={s.id} session={s} locale={i18n.language} />
              ))}
            </Box>
          )}
        </Section>

        {/* Recent activity feed */}
        <Section
          title={t("activityTitle")}
          linkLabel={t("activityViewAll")}
          onLink={() => navigate("/dashboard/sessions")}
        >
          {auditLoading && <LinearProgress />}
          {!auditLoading && audit?.length === 0 && (
            <Typography variant="caption" color="text.secondary">{t("noActivity")}</Typography>
          )}
          {audit && audit.length > 0 && (
            <Box sx={{ display: "grid", gap: 0.75 }}>
              {audit.map((entry) => (
                <AuditEntryCard
                  key={entry.id}
                  entry={entry}
                  locale={i18n.language}
                  compact
                />
              ))}
            </Box>
          )}
        </Section>
      </Box>

      {/* ── Row 3: Admin cards (admin-only) ── */}
      {isAdmin && (
        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", lg: "1fr auto" }, gap: 1.5, alignItems: "start" }}>

          {/* Platform security posture */}
          <Section
            title={t("adminPostureTitle")}
            linkLabel={t("adminPostureViewAll")}
            onLink={() => navigate("/dashboard/users")}
          >
            {postureLoading ? (
              <Box sx={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 1 }}>
                <Skeleton variant="rounded" height={52} />
                <Skeleton variant="rounded" height={52} />
                <Skeleton variant="rounded" height={52} />
              </Box>
            ) : posture ? (
              <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr 1fr", sm: "1fr 1fr 1fr" }, gap: 1 }}>
                <PostureStat icon={<GroupIcon />} value={posture.total_users} label={t("adminPostureTotal")} color="default" />
                <PostureStat
                  icon={<LockOpenIcon />}
                  value={posture.users_without_mfa}
                  label={t("adminPostureNoMfa")}
                  color={posture.users_without_mfa > 0 ? "warning" : "success"}
                />
                <PostureStat
                  icon={<AdminPanelSettingsIcon />}
                  value={posture.users_inactive_30d}
                  label={t("adminPostureInactive")}
                  color={posture.users_inactive_30d > 0 ? "warning" : "success"}
                />
              </Box>
            ) : null}
          </Section>

          {/* System health */}
          <Section title={t("healthTitle")}>
            {healthLoading ? (
              <Box sx={{ display: "grid", gap: 0.75 }}>
                <Skeleton variant="rounded" height={36} width={200} />
                <Skeleton variant="rounded" height={36} width={200} />
              </Box>
            ) : health ? (
              <Box sx={{ display: "grid", gap: 0.75 }}>
                {([
                  { key: "database", icon: <StorageIcon />, label: t("healthDb") },
                  { key: "redis",    icon: <MemoryIcon />,  label: t("healthRedis") },
                ] as const).map(({ key, icon, label }) => {
                  const svc = health[key];
                  const ok = svc.status === "ok";
                  return (
                    <Box key={key} sx={{ display: "flex", alignItems: "center", gap: 1, p: 0.75, borderRadius: 1, bgcolor: "background.default", border: "1px solid", borderColor: "divider" }}>
                      <Box sx={{ color: ok ? "success.main" : "error.main", display: "flex", "& svg": { fontSize: 16 } }}>{icon}</Box>
                      <Box sx={{ flex: 1, minWidth: 0 }}>
                        <Typography variant="caption" sx={{ fontWeight: 600, lineHeight: 1 }}>{label}</Typography>
                        <Typography variant="caption" color="text.disabled" sx={{ display: "block", fontSize: 10 }}>
                          {t("healthPool", { active: svc.pool.total - svc.pool.idle, idle: svc.pool.idle, max: svc.pool.max })}
                        </Typography>
                      </Box>
                      <Box sx={{ width: 8, height: 8, borderRadius: "50%", bgcolor: ok ? "success.main" : "error.main", flexShrink: 0 }} />
                    </Box>
                  );
                })}
              </Box>
            ) : null}
          </Section>

        </Box>
      )}
    </Box>
  );
}
