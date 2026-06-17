import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { api, type AuditEntry, type Session } from "@/lib/api";
import { useMeContext } from "@/contexts/MeContext";
import { SessionCard } from "@/components/SessionCard";
import { AuditEntryCard } from "@/components/AuditEntryCard";
import Avatar from "@mui/material/Avatar";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import LinearProgress from "@mui/material/LinearProgress";
import Paper from "@mui/material/Paper";
import Skeleton from "@mui/material/Skeleton";
import Typography from "@mui/material/Typography";
import LockIcon from "@mui/icons-material/Lock";
import LockOpenIcon from "@mui/icons-material/LockOpen";
import VerifiedUserIcon from "@mui/icons-material/VerifiedUser";

// ── Small section wrapper ─────────────────────────────────────────────────────

interface SectionProps {
  title: string;
  linkLabel?: string;
  onLink?: () => void;
  children: React.ReactNode;
}

function Section({ title, linkLabel, onLink, children }: SectionProps) {
  return (
    <Paper variant="outlined" sx={{ p: 2.5 }}>
      <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 2 }}>
        <Typography variant="subtitle2" sx={{ fontWeight: 700, textTransform: "uppercase", letterSpacing: 0.8, fontSize: 11 }}>
          {title}
        </Typography>
        {linkLabel && onLink && (
          <Button size="small" onClick={onLink} sx={{ fontSize: 11 }}>
            {linkLabel} →
          </Button>
        )}
      </Box>
      {children}
    </Paper>
  );
}

// ── Main page ─────────────────────────────────────────────────────────────────

export default function HomePage() {
  const { t } = useTranslation("homepage");
  const { i18n } = useTranslation();
  const me = useMeContext();
  const navigate = useNavigate();

  // MFA status
  const [mfaEnabled, setMfaEnabled] = useState<boolean | null>(null);
  const [mfaLoading, setMfaLoading] = useState(true);

  // Sessions
  const [sessions, setSessions] = useState<Session[] | null>(null);
  const [sessionsLoading, setSessionsLoading] = useState(true);

  // Recent audit
  const [audit, setAudit] = useState<AuditEntry[] | null>(null);
  const [auditLoading, setAuditLoading] = useState(true);

  useEffect(() => {
    if (!me) return;

    // MFA status
    api.mfaStatus()
      .then((d) => setMfaEnabled(d.supported !== false ? d.enabled : null))
      .catch(() => setMfaEnabled(null))
      .finally(() => setMfaLoading(false));

    // Active sessions (show max 3)
    api.listSessions()
      .then((all) => setSessions(all.slice(0, 3)))
      .catch(() => setSessions([]))
      .finally(() => setSessionsLoading(false));

    // Recent audit (last 5 of current user)
    api.listAuditLog({ page: 0, pageSize: 5, sortKey: "created_at", sortDir: "desc", filters: { user_id: me.user_id } })
      .then((res) => setAudit(res.data))
      .catch(() => setAudit([]))
      .finally(() => setAuditLoading(false));
  }, [me]);

  if (!me) return null;

  const name = [me.first_name, me.last_name].filter(Boolean).join(" ") || me.email;
  const initials = me.first_name?.[0]
    ? `${me.first_name[0]}${me.last_name?.[0] ?? ""}`.toUpperCase()
    : me.email.slice(0, 2).toUpperCase();

  return (
    <Box
      sx={{
        height: "100%",
        overflow: "auto",
        p: { xs: 2, sm: 4 },
        display: "flex",
        flexDirection: "column",
        gap: 2.5,
        maxWidth: 780,
      }}
    >
      {/* ── Identity card ── */}
      <Paper
        variant="outlined"
        sx={{
          p: 3,
          display: "flex",
          alignItems: "center",
          gap: 2.5,
          background: "linear-gradient(135deg, rgba(99,102,241,0.08) 0%, transparent 100%)",
          borderColor: "rgba(99,102,241,0.25)",
        }}
      >
        <Avatar
          src={me.has_avatar ? `/api/v1/me/avatar` : undefined}
          sx={{ width: 60, height: 60, fontSize: 22, fontWeight: 700, bgcolor: "primary.dark" }}
        >
          {initials}
        </Avatar>
        <Box sx={{ flex: 1, minWidth: 0 }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap", mb: 0.5 }}>
            <Typography variant="h6" sx={{ fontWeight: 700 }} noWrap>{name}</Typography>
            <VerifiedUserIcon color="success" fontSize="small" />
          </Box>
          <Typography variant="body2" color="text.secondary" noWrap>{me.email}</Typography>
          <Box sx={{ display: "flex", gap: 0.75, mt: 1, flexWrap: "wrap" }}>
            {me.roles.map((r) => (
              <Chip key={r} size="small" color="primary" label={r} />
            ))}
          </Box>
        </Box>
        <Box sx={{ display: { xs: "none", sm: "grid" }, gridTemplateColumns: "auto 1fr", gap: 0.5, columnGap: 1.5 }}>
          <Typography variant="caption" color="text.secondary">{t("userId")}</Typography>
          <Typography variant="caption" sx={{ fontFamily: "monospace", fontSize: 11 }} noWrap>{me.user_id}</Typography>
          <Typography variant="caption" color="text.secondary">{t("locale")}</Typography>
          <Typography variant="caption">{i18n.language.toUpperCase()}</Typography>
        </Box>
      </Paper>

      {/* ── Two-column grid: MFA + Sessions ── */}
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "1fr 1fr" }, gap: 2.5 }}>

        {/* MFA status card */}
        <Section
          title={t("mfaTitle")}
          linkLabel={mfaEnabled === false ? t("mfaSetup") : t("mfaManage")}
          onLink={() => navigate("/mfa")}
        >
          {mfaLoading ? (
            <Skeleton variant="rounded" height={48} />
          ) : (
            <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
              {mfaEnabled
                ? <LockIcon color="success" />
                : <LockOpenIcon color="warning" />
              }
              <Box>
                <Chip
                  size="small"
                  color={mfaEnabled ? "success" : "warning"}
                  label={mfaEnabled ? t("enabled") : t("disabled")}
                  sx={{ mb: 0.5 }}
                />
                <Typography variant="caption" color="text.secondary" sx={{ display: "block" }}>
                  {mfaEnabled ? t("mfaEnabled") : t("mfaDisabled")}
                </Typography>
              </Box>
            </Box>
          )}
        </Section>

        {/* Active sessions snapshot */}
        <Section
          title={t("sessionsTitle")}
          linkLabel={t("sessionsViewAll")}
          onLink={() => navigate("/sessions")}
        >
          {sessionsLoading && <LinearProgress />}
          {!sessionsLoading && sessions?.length === 0 && (
            <Typography variant="body2" color="text.secondary">{t("noSessions")}</Typography>
          )}
          {sessions && sessions.length > 0 && (
            <Box sx={{ display: "grid", gap: 1 }}>
              {sessions.map((s) => (
                <SessionCard key={s.id} session={s} locale={i18n.language} />
              ))}
            </Box>
          )}
        </Section>
      </Box>

      {/* ── Recent activity feed ── */}
      <Section
        title={t("activityTitle")}
        linkLabel={t("activityViewAll")}
        onLink={() => navigate("/audit")}
      >
        {auditLoading && <LinearProgress />}
        {!auditLoading && audit?.length === 0 && (
          <Typography variant="body2" color="text.secondary">{t("noActivity")}</Typography>
        )}
        {audit && audit.length > 0 && (
          <Box sx={{ display: "grid", gap: 1 }}>
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
  );
}
