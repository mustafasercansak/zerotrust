import React, { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, ApiError, type AuditEntry, type PageParams, type Session, type UserData, type UserMfaInfo } from "@/lib/api";
import { formatDate, formatDateTime } from "@/lib/dateUtils";
import { formatSessionDevice } from "@/lib/sessionUtils";
import { useMeContext } from "@/contexts/MeContext";
import { ResourceTablePage } from "@/components/ResourceTablePage";
import { SessionCard } from "@/components/SessionCard";
import { AuditEntryCard } from "@/components/AuditEntryCard";
import { requiredValidator } from "@/lib/validation";
import { useStepUp } from "@/hooks/useStepUp";
import { StepUpMfaDialog } from "@/components/StepUpMfaDialog";
import { toast } from "sonner";
import Alert from "@mui/material/Alert";
import Avatar from "@mui/material/Avatar";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Chip, { type ChipProps } from "@mui/material/Chip";
import CircularProgress from "@mui/material/CircularProgress";
import Dialog from "@mui/material/Dialog";
import DialogActions from "@mui/material/DialogActions";
import DialogContent from "@mui/material/DialogContent";
import DialogTitle from "@mui/material/DialogTitle";
import Divider from "@mui/material/Divider";
import Drawer from "@mui/material/Drawer";
import IconButton from "@mui/material/IconButton";
import LinearProgress from "@mui/material/LinearProgress";
import List from "@mui/material/List";
import ListItem from "@mui/material/ListItem";
import ListItemText from "@mui/material/ListItemText";
import Menu from "@mui/material/Menu";
import MenuItem from "@mui/material/MenuItem";
import Paper from "@mui/material/Paper";
import Stack from "@mui/material/Stack";
import TextField from "@mui/material/TextField";
import Tooltip from "@mui/material/Tooltip";
import Typography from "@mui/material/Typography";
import CloseIcon from "@mui/icons-material/Close";
import DevicesIcon from "@mui/icons-material/Devices";
import HistoryIcon from "@mui/icons-material/History";
import LockIcon from "@mui/icons-material/Lock";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import PersonIcon from "@mui/icons-material/Person";
import SecurityIcon from "@mui/icons-material/Security";
import { getGridStringOperators, type GridColDef, type GridRowParams } from "@mui/x-data-grid";
import { PasswordStrengthBar } from "@/components/PasswordStrengthBar";

const AVAILABLE_ROLES = ["admin", "user"];
const DEFAULT_MAX_SESSIONS = 5;

// ── helpers ──────────────────────────────────────────────────────────────────

function sessionCountColor(count: number, maxSessions: number): ChipProps["color"] {
  const ratio = count / Math.max(maxSessions, 1);
  if (ratio <= 0.5) return "primary";
  if (ratio <= 0.75) return "warning";
  return "error";
}

function fullName(row: Pick<UserData, "first_name" | "last_name">): string {
  return [row.first_name, row.last_name].filter(Boolean).join(" ").trim();
}

function userInitials(row: Pick<UserData, "email" | "first_name" | "last_name">): string {
  const parts = [row.first_name, row.last_name].filter(Boolean);
  if (parts.length > 0) return parts.map((p) => p[0]).join("").slice(0, 2).toUpperCase();
  return row.email.slice(0, 2).toUpperCase();
}

// ── Row actions menu ──────────────────────────────────────────────────────────

interface RowActionsProps {
  row: UserData;
  me: { user_id: string } | null;
  onStatusChange: (id: string, active: boolean) => void;
  onViewSessions: (row: UserData) => void;
  onRevokeAll: (row: UserData) => void;
}

function RowActions({ row, me, onStatusChange, onViewSessions, onRevokeAll }: RowActionsProps) {
  const { t } = useTranslation("admin");
  const [anchor, setAnchor] = useState<null | HTMLElement>(null);
  const isSelf = me?.user_id === row.id;

  return (
    <>
      <Tooltip title={t("actions")}>
        <IconButton size="small" onClick={(e) => { e.stopPropagation(); setAnchor(e.currentTarget); }}>
          <MoreVertIcon fontSize="small" />
        </IconButton>
      </Tooltip>
      <Menu anchorEl={anchor} open={Boolean(anchor)} onClose={() => setAnchor(null)}
        onClick={() => setAnchor(null)}>
        <MenuItem onClick={() => onViewSessions(row)}>
          {t("viewSessions")}
        </MenuItem>
        {row.active_sessions > 0 && (
          <MenuItem onClick={() => onRevokeAll(row)} sx={{ color: "warning.main" }}>
            {t("revokeAllSessions")}
          </MenuItem>
        )}
        {!isSelf && (
          <MenuItem
            onClick={() => onStatusChange(row.id, !row.is_active)}
            sx={{ color: row.is_active ? "error.main" : "success.main" }}
          >
            {row.is_active ? t("deactivate") : t("activate")}
          </MenuItem>
        )}
      </Menu>
    </>
  );
}

// ── Sessions dialog ───────────────────────────────────────────────────────────

interface SessionsDialogProps {
  user: UserData;
  onClose: () => void;
  onRevoke: (userId: string, sessionId: string) => Promise<void>;
  onRevokeAll: (userId: string) => Promise<void>;
}

function SessionsDialog({ user, onClose, onRevoke, onRevokeAll }: SessionsDialogProps) {
  const { t } = useTranslation("admin");
  const { i18n } = useTranslation();
  const [sessions, setSessions] = useState<Session[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async (userId: string) => {
    setLoading(true);
    setError(null);
    try {
      const data = await api.admin.listUserSessions(userId);
      setSessions(data);
    } catch {
      setError("internal_error");
    } finally {
      setLoading(false);
    }
  }, []);

  // Load sessions when dialog opens
  const prevUser = useState<string | null>(null);
  if (user && prevUser[0] !== user.id) {
    prevUser[1](user.id);
    void load(user.id);
  }

  return (
    <Dialog open onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle sx={{ pb: 1 }}>
        {t("sessionsDialogTitle", { email: user.email })}
      </DialogTitle>
      <DialogContent sx={{ p: 0 }}>
        {loading && (
          <Box sx={{ display: "flex", justifyContent: "center", py: 4 }}>
            <CircularProgress size={28} />
          </Box>
        )}
        {error && <Alert severity="error" sx={{ m: 2 }}>{t(`errors.${error}`)}</Alert>}
        {!loading && !error && sessions?.length === 0 && (
          <Typography variant="body2" color="text.secondary" sx={{ p: 3 }}>
            {t("noSessionsFound")}
          </Typography>
        )}
        {!loading && sessions && sessions.length > 0 && (
          <List disablePadding>
            {sessions.map((s, idx) => (
              <Box key={s.id}>
                {idx > 0 && <Divider />}
                <ListItem
                   secondaryAction={
                    <Button size="small" color="warning" onClick={async () => {
                      try {
                        await onRevoke(user.id, s.id);
                        setSessions((prev) => prev!.filter((x) => x.id !== s.id));
                      } catch {
                        // handled by caller
                      }
                    }}>
                      {t("revokeSession")}
                    </Button>
                  }
                >
                  <ListItemText
                    primary={
                      <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                        <Typography variant="body2">{formatSessionDevice(s)}</Typography>
                        {s.is_current && <Chip label="current" size="small" color="primary" />}
                      </Box>
                    }
                    secondary={
                      <Stack spacing={0.25}>
                        <Typography variant="caption" color="text.secondary" sx={{ fontFamily: "monospace" }}>
                          IP: {s.ip_address || "—"}
                        </Typography>
                        <Typography variant="caption" color="text.disabled">
                          {formatDateTime(s.last_used_at ?? s.created_at, i18n.language)}
                        </Typography>
                      </Stack>
                    }
                  />
                </ListItem>
              </Box>
            ))}
          </List>
        )}
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2, justifyContent: "space-between" }}>
        <Button
          color="warning"
          disabled={!sessions || sessions.length === 0}
          onClick={async () => {
            try {
              await onRevokeAll(user.id);
              setSessions([]);
            } catch {
              // handled by caller
            }
          }}
        >
          {t("revokeAllSessions")}
        </Button>
        <Button onClick={onClose}>{t("cancel")}</Button>
      </DialogActions>
    </Dialog>
  );
}

// ── User Profile Drawer ───────────────────────────────────────────────────────

interface DrawerSection {
  key: string;
  icon: React.ReactNode;
  label: string;
}

interface UserProfileDrawerProps {
  user: UserData;
  onClose: () => void;
  onRevoke: (userId: string, sessionId: string) => Promise<void>;
  onRevokeAll: (userId: string) => Promise<void>;
  onStatusChange: (userId: string, active: boolean) => Promise<void>;
  isSelf: boolean;
}

export function UserProfileDrawer({
  user, onClose, onRevoke, onRevokeAll, onStatusChange, isSelf,
}: UserProfileDrawerProps) {
  const { t } = useTranslation("admin");
  const { i18n } = useTranslation();
  const [activeSection, setActiveSection] = useState<string>("profile");

  const [sessions, setSessions] = useState<Session[] | null>(null);
  const [sessionsLoading, setSessionsLoading] = useState(false);
  const [sessionCount, setSessionCount] = useState<number | null>(null);
  const effectiveSessionCount = sessionCount ?? user.active_sessions;

  const [audit, setAudit] = useState<AuditEntry[] | null>(null);
  const [auditLoading, setAuditLoading] = useState(false);

  const [mfa, setMfa] = useState<UserMfaInfo | null>(null);
  const [mfaLoading, setMfaLoading] = useState(false);

  // Load data for each section on demand
  useEffect(() => {
    if (activeSection === "sessions" && sessions === null && !sessionsLoading) {
      setSessionsLoading(true);
      api.admin.listUserSessions(user.id)
        .then((data) => { setSessions(data); setSessionCount(data.length); })
        .catch(() => setSessions([]))
        .finally(() => setSessionsLoading(false));
    }
  }, [activeSection, user.id, sessions, sessionsLoading]);

  useEffect(() => {
    if (activeSection === "audit" && audit === null && !auditLoading) {
      setAuditLoading(true);
      api.listAuditLog({ page: 0, pageSize: 10, filters: { user_id: user.id } })
        .then((res) => setAudit(res.data))
        .catch(() => setAudit([]))
        .finally(() => setAuditLoading(false));
    }
  }, [activeSection, user.id, audit, auditLoading]);

  useEffect(() => {
    if (activeSection === "mfa" && mfa === null && !mfaLoading) {
      setMfaLoading(true);
      api.admin.listUserMfa(user.id)
        .then(setMfa)
        .catch(() => setMfa({ totp_enabled: false, webauthn_credentials: [] }))
        .finally(() => setMfaLoading(false));
    }
  }, [activeSection, user.id, mfa, mfaLoading]);

  const sections: DrawerSection[] = [
    { key: "profile", icon: <PersonIcon fontSize="small" />, label: t("drawerProfile") },
    { key: "sessions", icon: <DevicesIcon fontSize="small" />, label: t("drawerSessions") },
    { key: "audit", icon: <HistoryIcon fontSize="small" />, label: t("drawerAudit") },
    { key: "mfa", icon: <SecurityIcon fontSize="small" />, label: t("drawerMfa") },
  ];

  const name = [user.first_name, user.last_name].filter(Boolean).join(" ") || user.email;
  const initials = user.first_name?.[0]
    ? `${user.first_name[0]}${user.last_name?.[0] ?? ""}`.toUpperCase()
    : user.email.slice(0, 2).toUpperCase();

  return (
    <Drawer
      anchor="right"
      open
      onClose={onClose}
      slotProps={{ paper: { sx: { width: { xs: "100vw", sm: 460 }, display: "flex", flexDirection: "column" } } }}
    >
      {/* Header */}
      <Box sx={{ p: 2.5, display: "flex", alignItems: "flex-start", gap: 2, borderBottom: 1, borderColor: "divider" }}>
        <Avatar
          src={user.has_avatar ? `/api/v1/users/${user.id}/avatar` : undefined}
          sx={{ width: 52, height: 52, fontSize: 18, fontWeight: 700 }}
        >
          {initials}
        </Avatar>
        <Box sx={{ flex: 1, minWidth: 0 }}>
          <Typography variant="subtitle1" sx={{ fontWeight: 700 }} noWrap>{name}</Typography>
          <Typography variant="caption" color="text.secondary" noWrap sx={{ display: "block" }}>{user.email}</Typography>
          <Box sx={{ display: "flex", gap: 0.75, mt: 0.75, flexWrap: "wrap" }}>
            <Chip
              size="small"
              color={user.is_active ? "success" : "error"}
              label={user.is_active ? t("active") : t("inactive")}
            />
            {user.roles.map((r) => (
              <Chip key={r} size="small" color="primary" label={r} />
            ))}
          </Box>
        </Box>
        <IconButton onClick={onClose} size="small" sx={{ mt: -0.5 }}>
          <CloseIcon fontSize="small" />
        </IconButton>
      </Box>

      {/* Section nav tabs */}
      <Box sx={{ display: "flex", borderBottom: 1, borderColor: "divider", flexShrink: 0 }}>
        {sections.map((sec) => (
          <Tooltip key={sec.key} title={sec.label}>
            <Box
              onClick={() => setActiveSection(sec.key)}
              sx={{
                alignItems: "center",
                cursor: "pointer",
                display: "flex",
                flex: 1,
                flexDirection: "column",
                gap: 0.5,
                py: 1.25,
                borderBottom: 2,
                borderColor: activeSection === sec.key ? "primary.main" : "transparent",
                color: activeSection === sec.key ? "primary.main" : "text.secondary",
                transition: "all 0.15s",
                "&:hover": { color: "primary.main", bgcolor: "action.hover" },
              }}
            >
              {sec.icon}
              <Typography variant="caption" sx={{ lineHeight: 1, fontSize: 10, fontWeight: activeSection === sec.key ? 700 : 400 }}>
                {sec.label}
              </Typography>
            </Box>
          </Tooltip>
        ))}
      </Box>

      {/* Content */}
      <Box sx={{ flex: 1, overflow: "auto", p: 2 }}>
        {/* Profile */}
        {activeSection === "profile" && (
          <Box sx={{ display: "grid", gap: 2 }}>
            <Paper variant="outlined" sx={{ p: 2 }}>
              <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 700, textTransform: "uppercase", letterSpacing: 0.8 }}>
                {t("accountInfo")}
              </Typography>
              <Box sx={{ display: "grid", gridTemplateColumns: "120px 1fr", gap: 1, mt: 1.5 }}>
                {[
                  { label: t("user"), value: name },
                  { label: t("email"), value: user.email },
                  { label: t("createdAt"), value: formatDate(user.created_at, i18n.language) },
                  { label: t("updatedAt"), value: formatDate(user.updated_at, i18n.language) },
                  { label: t("locale"), value: user.locale?.toUpperCase() || "—" },
                ].map(({ label, value }) => (
                  <React.Fragment key={label}>
                    <Typography variant="body2" color="text.secondary">{label}</Typography>
                    <Typography variant="body2" noWrap title={value}>{value}</Typography>
                  </React.Fragment>
                ))}
              </Box>
            </Paper>
            {!isSelf && (
              <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }}>
                <Button
                  size="small"
                  variant="outlined"
                  color={user.is_active ? "error" : "success"}
                  onClick={() => onStatusChange(user.id, !user.is_active)}
                >
                  {user.is_active ? t("deactivate") : t("activate")}
                </Button>
                {effectiveSessionCount > 0 && (
                  <Button size="small" variant="outlined" color="warning" onClick={async () => {
                    await onRevokeAll(user.id);
                    setSessionCount(0);
                    setSessions([]);
                  }}>
                    {t("revokeAllSessions")}
                  </Button>
                )}
              </Box>
            )}
          </Box>
        )}

        {/* Sessions */}
        {activeSection === "sessions" && (
          <Box>
            {sessionsLoading && <LinearProgress sx={{ mb: 2 }} />}
            {!sessionsLoading && sessions?.length === 0 && (
              <Typography variant="body2" color="text.secondary">{t("noSessionsFound")}</Typography>
            )}
            {sessions && sessions.length > 0 && (
              <Box sx={{ display: "grid", gap: 1.5 }}>
                {sessions.map((s) => (
                  <SessionCard
                    key={s.id}
                    session={s}
                    locale={i18n.language}
                    onRevoke={async (sess) => {
                      try {
                        await onRevoke(user.id, sess.id);
                        setSessions((prev) => prev!.filter((x) => x.id !== sess.id));
                        setSessionCount((n) => Math.max(0, (n ?? user.active_sessions) - 1));
                      } catch { /* handled by caller */ }
                    }}
                  />
                ))}
              </Box>
            )}
          </Box>
        )}

        {/* Audit log */}
        {activeSection === "audit" && (
          <Box>
            {auditLoading && <LinearProgress sx={{ mb: 2 }} />}
            {!auditLoading && audit?.length === 0 && (
              <Typography variant="body2" color="text.secondary">{t("noAuditEntries")}</Typography>
            )}
            {audit && audit.length > 0 && (
              <Box sx={{ display: "grid", gap: 1 }}>
                {audit.map((entry) => (
                  <AuditEntryCard
                    key={entry.id}
                    entry={entry}
                    locale={i18n.language}
                  />
                ))}
              </Box>
            )}
          </Box>
        )}

        {/* MFA */}
        {activeSection === "mfa" && (
          <Box sx={{ display: "grid", gap: 2 }}>
            {mfaLoading && <LinearProgress />}
            {mfa && (
              <>
                <Paper variant="outlined" sx={{ p: 2 }}>
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
                    <LockIcon color={mfa.totp_enabled ? "success" : "disabled"} />
                    <Box>
                      <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>{t("totpLabel")}</Typography>
                      <Typography variant="caption" color={mfa.totp_enabled ? "success.main" : "text.secondary"}>
                        {mfa.totp_enabled ? t("totpEnabled") : t("totpDisabled")}
                      </Typography>
                    </Box>
                    <Chip
                      size="small"
                      sx={{ ml: "auto" }}
                      color={mfa.totp_enabled ? "success" : "default"}
                      label={mfa.totp_enabled ? t("enabled") : t("disabled")}
                    />
                  </Box>
                </Paper>
                <Box>
                  <Typography variant="subtitle2" sx={{ fontWeight: 700, mb: 1 }}>
                    {t("passkeys")} ({mfa.webauthn_credentials.length})
                  </Typography>
                  {mfa.webauthn_credentials.length === 0 ? (
                    <Typography variant="body2" color="text.secondary">{t("noPasskeys")}</Typography>
                  ) : (
                    <Box sx={{ display: "grid", gap: 1 }}>
                      {mfa.webauthn_credentials.map((cred) => (
                        <Paper key={cred.id} variant="outlined" sx={{ p: 1.5 }}>
                          <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
                            <SecurityIcon color="primary" fontSize="small" />
                            <Box sx={{ flex: 1, minWidth: 0 }}>
                              <Typography variant="body2" sx={{ fontWeight: 600 }} noWrap>
                                {cred.name || t("unnamedPasskey")}
                              </Typography>
                              <Typography variant="caption" color="text.secondary">
                                {t("signCount")}: {cred.sign_count} · {t("added")}: {formatDate(cred.created_at, i18n.language)}
                              </Typography>
                              {cred.last_used_at && (
                                <Typography variant="caption" color="text.disabled" sx={{ display: "block" }}>
                                  {t("lastUsed")}: {formatDateTime(cred.last_used_at, i18n.language)}
                                </Typography>
                              )}
                            </Box>
                          </Box>
                        </Paper>
                      ))}
                    </Box>
                  )}
                </Box>
              </>
            )}
          </Box>
        )}
      </Box>
    </Drawer>
  );
}

// ── Main page ─────────────────────────────────────────────────────────────────

export default function UsersPage() {
  const { t } = useTranslation("admin");
  const { t: tCommon } = useTranslation("common");
  const { i18n } = useTranslation();
  const me = useMeContext();
  const isAdmin = me?.roles.includes("admin") ?? false;

  const { runWithStepUp, stepUpOpen, stepUpError, stepUpSubmitting, handleStepUpSubmit, handleStepUpClose } = useStepUp();

  const [showCreate, setShowCreate] = useState(false);
  const [email, setEmail] = useState("");
  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");
  const [password, setPassword] = useState("");
  const [selectedRoles, setSelectedRoles] = useState<string[]>(["user"]);
  const [creating, setCreating] = useState(false);
  const [refresh, setRefresh] = useState(0);
  const [maxSessions, setMaxSessions] = useState(DEFAULT_MAX_SESSIONS);

  // Sessions dialog
  const [sessionsUser, setSessionsUser] = useState<UserData | null>(null);

  // Profile drawer
  const [drawerUser, setDrawerUser] = useState<UserData | null>(null);

  const handleRowClick = useCallback((params: GridRowParams<UserData>) => {
    setDrawerUser(params.row);
  }, []);

  // eslint-disable-next-line react-hooks/exhaustive-deps
  const fetcher = useCallback((p: PageParams) => api.admin.listUsers(p), [refresh]);

  useEffect(() => {
    if (!isAdmin) return;
    let cancelled = false;

    api.admin.getSettings()
      .then((settings) => {
        if (cancelled) return;
        const parsed = Number.parseInt(settings.max_sessions_per_user ?? "", 10);
        setMaxSessions(Number.isFinite(parsed) && parsed > 0 ? parsed : DEFAULT_MAX_SESSIONS);
      })
      .catch(() => {
        if (!cancelled) setMaxSessions(DEFAULT_MAX_SESSIONS);
      });

    const refreshUsers = () => setRefresh((n) => n + 1);
    window.addEventListener("sessions:changed", refreshUsers);

    return () => {
      cancelled = true;
      window.removeEventListener("sessions:changed", refreshUsers);
    };
  }, [isAdmin]);

  const tabs = useMemo(() => [
    { key: "all",      label: tCommon("filterAll") },
    { key: "active",   label: tCommon("filterActive"),   preset: { status: "active" } },
    { key: "inactive", label: tCommon("filterInactive"), preset: { status: "inactive" } },
  ], [tCommon]);

  async function handleStatusChange(userId: string, active: boolean) {
    if (!active && !window.confirm(t("deactivateConfirm"))) return;
    try {
      await runWithStepUp(() => api.admin.setUserStatus(userId, active), "user_status_update");
      setRefresh((n) => n + 1);
    } catch (err) {
      if (err instanceof ApiError && err.message === "mfa_required") {
        return;
      }
      toast.error(t("errors.internal_error"));
    }
  }

  async function handleRevokeAll(row: UserData) {
    if (!window.confirm(t("revokeAllConfirm"))) return;
    try {
      await runWithStepUp(() => api.admin.revokeAllUserSessions(row.id), "user_sessions_revoke_all");
      setRefresh((n) => n + 1);
    } catch (err) {
      if (err instanceof ApiError && err.message === "mfa_required") {
        return;
      }
      toast.error(t("errors.internal_error"));
    }
  }
  const columns = useMemo<GridColDef<UserData>[]>(() => [
    {
      field: "email", headerName: t("user"), minWidth: 280, flex: 1.45,
      filterable: true,
      filterOperators: getGridStringOperators().filter((op) => op.value === "equals"),
      renderCell: ({ row }) => (
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.25, minWidth: 0, height: "100%" }}>
          <Avatar
            src={row.has_avatar ? `/api/v1/users/${row.id}/avatar` : undefined}
            sx={{ width: 32, height: 32, fontSize: 13 }}
          >
            {userInitials(row)}
          </Avatar>
          <Box sx={{ minWidth: 0, display: "flex", flexDirection: "column", justifyContent: "center" }}>
            <Typography variant="body2" noWrap sx={{ fontWeight: fullName(row) ? 700 : 500, lineHeight: 1.2 }}>
              {fullName(row) || row.email}
            </Typography>
            {fullName(row) && (
              <Typography variant="caption" color="text.secondary" noWrap sx={{ lineHeight: 1.2, mt: 0.25 }}>
                {row.email}
              </Typography>
            )}
          </Box>
          {row.active_sessions > 0 && (
            <Chip
              size="small"
              color={sessionCountColor(row.active_sessions, maxSessions)}
              label={t("onlineNow")}
              sx={{ height: 18, fontSize: 10, "& .MuiChip-label": { px: 0.75 } }}
            />
          )}
        </Box>
      ),
    },
    {
      field: "role", headerName: t("roles"), minWidth: 160, flex: 0.9, sortable: false,
      type: "singleSelect", valueOptions: AVAILABLE_ROLES,
      renderCell: ({ row }) =>
        row.roles.length === 0
          ? <Chip size="small" variant="outlined" label={t("noRoles")} />
          : <Stack direction="row" spacing={0.5} sx={{ alignItems: "center", height: "100%" }}>
              {row.roles.map((r) => <Chip key={r} size="small" color="primary" label={r} />)}
            </Stack>,
    },
    {
      field: "is_active", headerName: t("status"), minWidth: 120, flex: 0.6,
      renderCell: ({ row }) => (
        <Chip size="small" color={row.is_active ? "success" : "error"} label={row.is_active ? t("active") : t("inactive")} />
      ),
    },
    {
      field: "active_sessions", headerName: t("sessions"), minWidth: 150, flex: 0.75,
      renderCell: ({ row }) =>
        row.active_sessions > 0 ? (
          <Chip
            size="small"
            color={sessionCountColor(row.active_sessions, maxSessions)}
            variant="outlined"
            label={t("activeSessions", { count: row.active_sessions, defaultValue: `${row.active_sessions} aktif oturum` })}
          />
        ) : (
          <Typography variant="caption" color="text.disabled">
            {t("noActiveSessions")}
          </Typography>
        ),
    },
    {
      field: "mfa_enabled", headerName: t("security"), minWidth: 140, flex: 0.75,
      sortable: false, filterable: false,
      renderCell: ({ row }) => (
        <Box sx={{ display: "flex", alignItems: "center", gap: 0.75, height: "100%" }}>
          <Tooltip title={row.mfa_enabled ? t("totpEnabled") : t("totpDisabled")}>
            <Chip
              size="small"
              color={row.mfa_enabled ? "success" : "default"}
              label="TOTP"
              sx={{ height: 18, fontSize: 10, "& .MuiChip-label": { px: 0.75 } }}
            />
          </Tooltip>
          {row.passkey_count > 0 && (
            <Tooltip title={t("passkeyCount", { count: row.passkey_count })}>
              <Chip
                size="small"
                color="primary"
                icon={<SecurityIcon sx={{ fontSize: "12px !important" }} />}
                label={row.passkey_count}
                sx={{ height: 18, fontSize: 10, "& .MuiChip-label": { px: 0.5 } }}
              />
            </Tooltip>
          )}
        </Box>
      ),
    },
    {
      field: "created_at", headerName: t("createdAt"), minWidth: 140, flex: 0.7,
      renderCell: ({ row }) => (
        <Typography variant="caption" color="text.secondary">{formatDate(row.created_at, i18n.language)}</Typography>
      ),
    },
    {
      field: "__actions", headerName: "", width: 52, sortable: false, filterable: false,
      renderCell: ({ row }) => (
        <RowActions
          row={row}
          me={me}
          onStatusChange={handleStatusChange}
          onViewSessions={setSessionsUser}
          onRevokeAll={handleRevokeAll}
        />
      ),
    },
  // eslint-disable-next-line react-hooks/exhaustive-deps
  ], [t, i18n.language, me, maxSessions]);

  function toggleRole(role: string) {
    setSelectedRoles((prev) => prev.includes(role) ? prev.filter((r) => r !== role) : [...prev, role]);
  }

  function openCreate() {
    setEmail(""); setFirstName(""); setLastName(""); setPassword(""); setSelectedRoles(["user"]); setShowCreate(true);
  }

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setCreating(true);
    try {
      await api.admin.createUser({ email, first_name: firstName, last_name: lastName, password, locale: me?.locale ?? "tr", roles: selectedRoles });
      setShowCreate(false);
      setRefresh((n) => n + 1);
    } catch (err) {
      const code = err instanceof ApiError ? err.message : "internal_error";
      toast.error(t(`errors.${code}`, { defaultValue: t("errors.internal_error") }));
    } finally {
      setCreating(false);
    }
  }

  return (
    <>
      <ResourceTablePage
        columns={columns}
        tabs={tabs}
        fetcher={fetcher}
        getRowId={(u) => u.id}
        accessDenied={!isAdmin}
        accessDeniedMessage={t("accessDenied")}
        action={<Button variant="contained" onClick={openCreate}>+ {t("createUser")}</Button>}
        refreshSignal={refresh}
        defaultSortKey="created_at"
        defaultSortDir="desc"
        pageSizeOptions={[10, 25, 50]}
        defaultPageSize={25}
        rowHeight={64}
        onRowClick={handleRowClick}
      />

      {/* ── Create user dialog ── */}
      <Dialog open={showCreate} onClose={() => setShowCreate(false)} fullWidth maxWidth="xs">
        <Box component="form" onSubmit={handleCreate}>
          <DialogTitle>{t("createUserTitle")}</DialogTitle>
          <DialogContent sx={{ display: "grid", gap: 2, pt: 1 }}>
            <TextField
              label={t("email")}
              type="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              fullWidth
              inputRef={requiredValidator(email, tCommon("validation.required"))}
            />
            <Stack direction={{ xs: "column", sm: "row" }} spacing={1.5}>
              <TextField label={t("firstName")} value={firstName} onChange={(e) => setFirstName(e.target.value)} slotProps={{ htmlInput: { maxLength: 80 } }} fullWidth />
              <TextField label={t("lastName")} value={lastName} onChange={(e) => setLastName(e.target.value)} slotProps={{ htmlInput: { maxLength: 80 } }} fullWidth />
            </Stack>
            <Box>
              <TextField
                label={t("password")}
                type="password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                fullWidth
                inputRef={requiredValidator(password, tCommon("validation.required"))}
              />
              <PasswordStrengthBar password={password} />
            </Box>
            <Stack direction="row" spacing={1}>
              {AVAILABLE_ROLES.map((role) => (
                <Chip key={role} label={role}
                  color={selectedRoles.includes(role) ? "primary" : "default"}
                  variant={selectedRoles.includes(role) ? "filled" : "outlined"}
                  onClick={() => toggleRole(role)} />
              ))}
            </Stack>
          </DialogContent>
          <DialogActions sx={{ px: 3, pb: 3 }}>
            <Button onClick={() => setShowCreate(false)}>{tCommon("cancel")}</Button>
            <Button type="submit" variant="contained" disabled={creating}>
              {creating ? t("creating") : t("create")}
            </Button>
          </DialogActions>
        </Box>
      </Dialog>

      {/* ── User profile drawer ── */}
      {drawerUser && (
        <UserProfileDrawer
          user={drawerUser}
          onClose={() => setDrawerUser(null)}
          isSelf={drawerUser.id === me?.user_id}
          onStatusChange={async (userId, active) => {
            await handleStatusChange(userId, active);
            setRefresh((n) => n + 1);
            setDrawerUser((prev) => prev ? { ...prev, is_active: active } : null);
          }}
          onRevoke={async (userId, sessionId) => {
            try {
              await runWithStepUp(() => api.admin.revokeUserSession(userId, sessionId), "user_session_revoke");
            } catch (err) {
              if (err instanceof ApiError && err.message === "mfa_required") throw err;
              toast.error(t("errors.internal_error"));
              throw err;
            }
          }}
          onRevokeAll={async (userId) => {
            if (!window.confirm(t("revokeAllConfirm"))) return;
            try {
              await runWithStepUp(() => api.admin.revokeAllUserSessions(userId), "user_sessions_revoke_all");
              setRefresh((n) => n + 1);
            } catch (err) {
              if (err instanceof ApiError && err.message === "mfa_required") throw err;
              toast.error(t("errors.internal_error"));
              throw err;
            }
          }}
        />
      )}

      {/* ── Sessions dialog ── */}
      {sessionsUser && (
        <SessionsDialog
          user={sessionsUser}
          onClose={() => setSessionsUser(null)}
          onRevoke={async (userId, sessionId) => {
            try {
              await runWithStepUp(() => api.admin.revokeUserSession(userId, sessionId), "user_session_revoke");
            } catch (err) {
              if (err instanceof ApiError && err.message === "mfa_required") {
                throw err;
              }
              toast.error(t("errors.internal_error"));
              throw err;
            }
          }}
          onRevokeAll={async (userId) => {
            try {
              await runWithStepUp(() => api.admin.revokeAllUserSessions(userId), "user_sessions_revoke_all");
              setRefresh((n) => n + 1);
            } catch (err) {
              if (err instanceof ApiError && err.message === "mfa_required") {
                throw err;
              }
              toast.error(t("errors.internal_error"));
              throw err;
            }
          }}
        />
      )}

      <StepUpMfaDialog
        open={stepUpOpen}
        error={stepUpError}
        loading={stepUpSubmitting}
        onSubmit={handleStepUpSubmit}
        onClose={handleStepUpClose}
      />
    </>
  );
}
