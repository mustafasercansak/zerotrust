import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, ApiError, type PageParams, type Session, type UserData } from "@/lib/api";
import { formatDate, formatDateTime } from "@/lib/dateUtils";
import { useMeContext } from "@/contexts/MeContext";
import { ResourceTablePage } from "@/components/ResourceTablePage";
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
import IconButton from "@mui/material/IconButton";
import List from "@mui/material/List";
import ListItem from "@mui/material/ListItem";
import ListItemText from "@mui/material/ListItemText";
import Menu from "@mui/material/Menu";
import MenuItem from "@mui/material/MenuItem";
import Stack from "@mui/material/Stack";
import TextField from "@mui/material/TextField";
import Tooltip from "@mui/material/Tooltip";
import Typography from "@mui/material/Typography";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import type { GridColDef } from "@mui/x-data-grid";

const AVAILABLE_ROLES = ["admin", "user"];
const DEFAULT_MAX_SESSIONS = 5;

// ── helpers ──────────────────────────────────────────────────────────────────

function sessionDeviceLabel(s: Session): string {
  const { browser, browser_version: bv, os, os_version: ov, architecture: arch } = s.device_info ?? {};
  const major = bv?.split(".")?.[0];
  const browserStr = major ? `${browser} ${major}` : browser;
  const osStr = [ov ? `${os} ${ov}` : os, arch].filter(Boolean).join(" ");
  if (browserStr && osStr) return `${browserStr} — ${osStr}`;
  return browserStr || osStr || "Unknown device";
}

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
                        <Typography variant="body2">{sessionDeviceLabel(s)}</Typography>
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

// ── Main page ─────────────────────────────────────────────────────────────────

export default function UsersPage() {
  const { t } = useTranslation("admin");
  const { t: tCommon } = useTranslation("common");
  const { i18n } = useTranslation();
  const me = useMeContext();
  const isAdmin = me?.roles.includes("admin") ?? false;

  const [showCreate, setShowCreate] = useState(false);
  const [email, setEmail] = useState("");
  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");
  const [password, setPassword] = useState("");
  const [selectedRoles, setSelectedRoles] = useState<string[]>(["user"]);
  const [formError, setFormError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [refresh, setRefresh] = useState(0);
  const [maxSessions, setMaxSessions] = useState(DEFAULT_MAX_SESSIONS);

  // Sessions dialog
  const [sessionsUser, setSessionsUser] = useState<UserData | null>(null);

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
  async function runWithStepUp<T>(action: () => Promise<T>): Promise<T> {
    try {
      return await action();
    } catch (err) {
      if (!(err instanceof ApiError) || err.message !== "mfa_required") {
        throw err;
      }

      const code = window.prompt(t("mfaPrompt"))?.trim() ?? "";
      if (!code) {
        throw err;
      }

      await api.mfaStepUp(code);
      return action();
    }
  }

  async function handleStatusChange(userId: string, active: boolean) {
    if (!active && !window.confirm(t("deactivateConfirm"))) return;
    try {
      await runWithStepUp(() => api.admin.setUserStatus(userId, active));
      setRefresh((n) => n + 1);
    } catch (err) {
      if (err instanceof ApiError && err.message === "mfa_required") {
        return;
      }
      alert(t("errors.internal_error"));
    }
  }

  async function handleRevokeAll(row: UserData) {
    if (!window.confirm(t("revokeAllConfirm"))) return;
    try {
      await runWithStepUp(() => api.admin.revokeAllUserSessions(row.id));
      setRefresh((n) => n + 1);
    } catch (err) {
      if (err instanceof ApiError && err.message === "mfa_required") {
        return;
      }
      alert(t("errors.internal_error"));
    }
  }
  const columns = useMemo<GridColDef<UserData>[]>(() => [
    {
      field: "email", headerName: t("user"), minWidth: 280, flex: 1.45,
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
      field: "roles", headerName: t("roles"), minWidth: 160, flex: 0.9, sortable: false, filterable: false,
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
    setEmail(""); setFirstName(""); setLastName(""); setPassword(""); setSelectedRoles(["user"]); setFormError(null); setShowCreate(true);
  }

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setFormError(null);
    setCreating(true);
    try {
      await api.admin.createUser({ email, first_name: firstName, last_name: lastName, password, locale: me?.locale ?? "tr", roles: selectedRoles });
      setShowCreate(false);
      setRefresh((n) => n + 1);
    } catch (err) {
      const code = err instanceof ApiError ? err.message : "internal_error";
      setFormError(t(`errors.${code}`, { defaultValue: t("errors.internal_error") }));
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
      />

      {/* ── Create user dialog ── */}
      <Dialog open={showCreate} onClose={() => setShowCreate(false)} fullWidth maxWidth="xs">
        <Box component="form" onSubmit={handleCreate}>
          <DialogTitle>{t("createUserTitle")}</DialogTitle>
          <DialogContent sx={{ display: "grid", gap: 2, pt: 1 }}>
            <TextField label={t("email")} type="email" required value={email} onChange={(e) => setEmail(e.target.value)} fullWidth />
            <Stack direction={{ xs: "column", sm: "row" }} spacing={1.5}>
              <TextField label={t("firstName")} value={firstName} onChange={(e) => setFirstName(e.target.value)} slotProps={{ htmlInput: { maxLength: 80 } }} fullWidth />
              <TextField label={t("lastName")} value={lastName} onChange={(e) => setLastName(e.target.value)} slotProps={{ htmlInput: { maxLength: 80 } }} fullWidth />
            </Stack>
            <TextField label={t("password")} type="password" required value={password} onChange={(e) => setPassword(e.target.value)} fullWidth />
            <Stack direction="row" spacing={1}>
              {AVAILABLE_ROLES.map((role) => (
                <Chip key={role} label={role}
                  color={selectedRoles.includes(role) ? "primary" : "default"}
                  variant={selectedRoles.includes(role) ? "filled" : "outlined"}
                  onClick={() => toggleRole(role)} />
              ))}
            </Stack>
            {formError && <Alert severity="error">{formError}</Alert>}
          </DialogContent>
          <DialogActions sx={{ px: 3, pb: 3 }}>
            <Button onClick={() => setShowCreate(false)}>{tCommon("cancel")}</Button>
            <Button type="submit" variant="contained" disabled={creating}>
              {creating ? t("creating") : t("create")}
            </Button>
          </DialogActions>
        </Box>
      </Dialog>

      {/* ── Sessions dialog ── */}
      {sessionsUser && (
        <SessionsDialog
          user={sessionsUser}
          onClose={() => setSessionsUser(null)}
          onRevoke={async (userId, sessionId) => {
            try {
              await runWithStepUp(() => api.admin.revokeUserSession(userId, sessionId));
            } catch (err) {
              if (err instanceof ApiError && err.message === "mfa_required") {
                throw err;
              }
              alert(t("errors.internal_error"));
              throw err;
            }
          }}
          onRevokeAll={async (userId) => {
            try {
              await runWithStepUp(() => api.admin.revokeAllUserSessions(userId));
              setRefresh((n) => n + 1);
            } catch (err) {
              if (err instanceof ApiError && err.message === "mfa_required") {
                throw err;
              }
              alert(t("errors.internal_error"));
              throw err;
            }
          }}
        />
      )}
    </>
  );
}
