import { useCallback, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, ApiError, type PageParams, type ServiceAccount, type ServiceAccountCreated, type ServiceProbeResult, type ServiceTokenResponse } from "@/lib/api";
import { formatDate } from "@/lib/dateUtils";
import { useMeContext } from "@/contexts/MeContext";
import { ResourceTablePage } from "@/components/ResourceTablePage";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import Divider from "@mui/material/Divider";
import Dialog from "@mui/material/Dialog";
import DialogActions from "@mui/material/DialogActions";
import DialogContent from "@mui/material/DialogContent";
import DialogContentText from "@mui/material/DialogContentText";
import DialogTitle from "@mui/material/DialogTitle";
import FormControlLabel from "@mui/material/FormControlLabel";
import IconButton from "@mui/material/IconButton";
import LinearProgress from "@mui/material/LinearProgress";
import MenuItem from "@mui/material/MenuItem";
import Stack from "@mui/material/Stack";
import Switch from "@mui/material/Switch";
import TextField from "@mui/material/TextField";
import Tooltip from "@mui/material/Tooltip";
import Typography from "@mui/material/Typography";
import Check from "@mui/icons-material/Check";
import ContentCopy from "@mui/icons-material/ContentCopy";
import DeleteOutlined from "@mui/icons-material/DeleteOutlined";
import Edit from "@mui/icons-material/Edit";
import Key from "@mui/icons-material/Key";
import PlayArrow from "@mui/icons-material/PlayArrow";
import Science from "@mui/icons-material/Science";
import VerifiedUser from "@mui/icons-material/VerifiedUser";
import type { GridColDef } from "@mui/x-data-grid";

const PERMISSION_GROUPS = [
  { key: "users", labelKey: "scopeGroups.users", scopes: ["users:read", "users:create", "users:update", "users:delete"] },
  { key: "serviceAccounts", labelKey: "scopeGroups.serviceAccounts", scopes: ["service_accounts:read", "service_accounts:create", "service_accounts:update", "service_accounts:delete"] },
  { key: "audit", labelKey: "scopeGroups.audit", scopes: ["audit:read"] },
] as const;

const PROBE_TARGETS = [
  { key: "serviceAccounts", label: "Service Accounts", method: "GET", path: "/api/v1/admin/service-accounts?limit=1&offset=0", scope: "service_accounts:read" },
  { key: "users", label: "Users", method: "GET", path: "/api/v1/admin/users?limit=1&offset=0", scope: "users:read" },
  { key: "audit", label: "Audit Log", method: "GET", path: "/api/v1/admin/audit?limit=1&offset=0", scope: "audit:read" },
] as const;

type ProbeKey = typeof PROBE_TARGETS[number]["key"];

interface ServiceJwtPayload {
  sub?: string;
  cid?: string;
  scopes?: string[];
  sub_type?: string;
  exp?: number;
  iat?: number;
  jti?: string;
}

function decodeServiceJwt(token: string): ServiceJwtPayload | null {
  const part = token.split(".")[1];
  if (!part) return null;
  try {
    const base64 = part.replace(/-/g, "+").replace(/_/g, "/").padEnd(Math.ceil(part.length / 4) * 4, "=");
    return JSON.parse(atob(base64)) as ServiceJwtPayload;
  } catch {
    return null;
  }
}

function prettyJson(value: unknown): string {
  return JSON.stringify(value, null, 2);
}

function scopeAction(scope: string): string {
  return scope.split(":")[1] ?? scope;
}

interface ScopeSelectorProps {
  selected: string[];
  onToggle: (scope: string) => void;
}

function ScopeSelector({ selected, onToggle }: ScopeSelectorProps) {
  const { t } = useTranslation("serviceAccounts");

  return (
    <Box sx={{ display: "grid", gap: 1 }}>
      <Typography variant="caption" color="text.secondary">{t("scopes")}</Typography>
      {PERMISSION_GROUPS.map((group) => (
        <Box
          key={group.key}
          sx={{
            border: 1,
            borderColor: "divider",
            borderRadius: 1,
            display: "grid",
            gap: 1,
            p: 1.25,
          }}
        >
          <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>
            {t(group.labelKey)}
          </Typography>
          <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.75 }}>
            {group.scopes.map((scope) => (
              <Chip
                key={scope}
                clickable
                color={selected.includes(scope) ? "primary" : "default"}
                label={scopeAction(scope)}
                onClick={() => onToggle(scope)}
                size="small"
                variant={selected.includes(scope) ? "filled" : "outlined"}
              />
            ))}
          </Box>
        </Box>
      ))}
    </Box>
  );
}

export default function ServiceAccountsPage() {
  const { t } = useTranslation("serviceAccounts");
  const { t: tCommon } = useTranslation("common");
  const { i18n } = useTranslation();
  const me = useMeContext();
  const isAdmin = me?.roles.includes("admin") ?? false;

  const [showCreate, setShowCreate] = useState(false);
  const [newSecret, setNewSecret] = useState<ServiceAccountCreated | null>(null);
  const [copied, setCopied] = useState(false);
  const [name, setName] = useState("");
  const [selectedScopes, setSelectedScopes] = useState<string[]>([]);
  const [expiresAt, setExpiresAt] = useState("");
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState("");
  const [editing, setEditing] = useState<ServiceAccount | null>(null);
  const [editName, setEditName] = useState("");
  const [editScopes, setEditScopes] = useState<string[]>([]);
  const [editExpiresAt, setEditExpiresAt] = useState("");
  const [editActive, setEditActive] = useState(true);
  const [updating, setUpdating] = useState(false);
  const [editError, setEditError] = useState("");
  const [testing, setTesting] = useState<ServiceAccount | null>(null);
  const [testSecret, setTestSecret] = useState("");
  const [probeKey, setProbeKey] = useState<ProbeKey>("serviceAccounts");
  const [tokenResult, setTokenResult] = useState<ServiceTokenResponse | null>(null);
  const [tokenPayload, setTokenPayload] = useState<ServiceJwtPayload | null>(null);
  const [probeResult, setProbeResult] = useState<ServiceProbeResult | null>(null);
  const [testError, setTestError] = useState("");
  const [tokenLoading, setTokenLoading] = useState(false);
  const [probeLoading, setProbeLoading] = useState(false);
  const [refresh, setRefresh] = useState(0);

  // eslint-disable-next-line react-hooks/exhaustive-deps
  const fetcher = useCallback((p: PageParams) => api.admin.listServiceAccounts(p), [refresh]);

  const tabs = useMemo(() => [
    { key: "all",      label: tCommon("filterAll") },
    { key: "active",   label: tCommon("filterActive"),   preset: { status: "active" } },
    { key: "inactive", label: tCommon("filterInactive"), preset: { status: "inactive" } },
    { key: "expired",  label: tCommon("filterExpired"),  preset: { status: "expired" } },
  ], [tCommon]);

  const columns = useMemo<GridColDef<ServiceAccount>[]>(() => [
    {
      field: "name", headerName: t("name"), minWidth: 180, flex: 1,
      renderCell: ({ row }) => <Typography variant="body2" sx={{ fontWeight: 600 }}>{row.name}</Typography>,
    },
    {
      field: "client_id", headerName: t("clientId"), minWidth: 260, flex: 1.2, sortable: false,
      renderCell: ({ row }) => (
        <Typography variant="caption" color="text.secondary" sx={{ fontFamily: "monospace" }}>{row.client_id}</Typography>
      ),
    },
    {
      field: "scopes", headerName: t("scopes"), minWidth: 240, flex: 1.2, sortable: false, filterable: false,
      renderCell: ({ row }) =>
        row.scopes.length === 0
          ? <Chip size="small" variant="outlined" label={t("noScopes")} />
          : <Stack direction="row" spacing={0.5} sx={{ alignItems: "center", height: "100%" }}>
              {row.scopes.map((s) => <Chip key={s} size="small" color="info" label={s} />)}
            </Stack>,
    },
    {
      field: "is_active", headerName: t("status"), minWidth: 130, flex: 0.6,
      renderCell: ({ row }) => (
        <Chip size="small" color={row.is_active ? "success" : "default"}
          label={row.is_active ? t("active") : t("inactive")}
          onClick={() => handleToggleStatus(row)} clickable
          title={row.is_active ? t("deactivateHint") : t("activateHint")} />
      ),
    },
    {
      field: "created_at", headerName: t("createdAt"), minWidth: 150, flex: 0.7,
      renderCell: ({ row }) => (
        <Typography variant="caption" color="text.secondary">{formatDate(row.created_at, i18n.language)}</Typography>
      ),
    },
    {
      field: "expires_at", headerName: t("expiresAt"), minWidth: 150, flex: 0.7,
      renderCell: ({ row }) => {
        if (!row.expires_at) return <Chip size="small" variant="outlined" label={t("noExpiry")} />;
        const expired = new Date(row.expires_at) < new Date();
        return expired
          ? <Chip size="small" color="error" label={t("expired")} />
          : <Typography variant="caption" color="text.secondary">{formatDate(row.expires_at, i18n.language)}</Typography>;
      },
    },
    {
      field: "actions", headerName: "", sortable: false, filterable: false, align: "right", width: 132,
      // eslint-disable-next-line react-hooks/exhaustive-deps
      renderCell: ({ row }) => (
        <Box sx={{ display: "flex", justifyContent: "flex-end", width: "100%" }}>
          <Tooltip title={t("test")}>
            <IconButton size="small" onClick={() => openTest(row)}>
              <Science fontSize="small" />
            </IconButton>
          </Tooltip>
          <Tooltip title={t("edit")}>
            <IconButton size="small" onClick={() => openEdit(row)}>
              <Edit fontSize="small" />
            </IconButton>
          </Tooltip>
          <Tooltip title={t("revoke")}>
            <IconButton color="error" size="small" onClick={() => handleRevoke(row)}>
              <DeleteOutlined fontSize="small" />
            </IconButton>
          </Tooltip>
        </Box>
      ),
    },
  // eslint-disable-next-line react-hooks/exhaustive-deps
  ], [t, i18n.language]);
  async function handleToggleStatus(sa: ServiceAccount) {
    try {
      await runWithStepUp(() => api.admin.setServiceAccountStatus(sa.id, !sa.is_active));
      setRefresh((n) => n + 1);
    } catch (err) {
      if (err instanceof ApiError && err.message === "mfa_required") {
        return;
      }
      alert(t("errors.internal_error"));
    }
  }

  async function handleRevoke(sa: ServiceAccount) {
    if (!confirm(t("revokeConfirm", { name: sa.name }))) return;
    try {
      await runWithStepUp(() => api.admin.revokeServiceAccount(sa.id));
      setRefresh((n) => n + 1);
    } catch (err) {
      if (err instanceof ApiError && err.message === "mfa_required") {
        return;
      }
      alert(t("errors.internal_error"));
    }
  }
  function reset() { setName(""); setSelectedScopes([]); setExpiresAt(""); setCreateError(""); }

  function toggleScope(scope: string) {
    setSelectedScopes((prev) => prev.includes(scope) ? prev.filter((s) => s !== scope) : [...prev, scope]);
  }

  function toggleEditScope(scope: string) {
    setEditScopes((prev) => prev.includes(scope) ? prev.filter((s) => s !== scope) : [...prev, scope]);
  }

  function openEdit(sa: ServiceAccount) {
    setEditing(sa);
    setEditName(sa.name);
    setEditScopes(sa.scopes);
    setEditExpiresAt(sa.expires_at ? sa.expires_at.slice(0, 10) : "");
    setEditActive(sa.is_active);
    setEditError("");
  }

  function openTest(sa: ServiceAccount) {
    setTesting(sa);
    setTestSecret("");
    setProbeKey("serviceAccounts");
    setTokenResult(null);
    setTokenPayload(null);
    setProbeResult(null);
    setTestError("");
  }

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setCreateError("");
    if (!name.trim()) return;
    setCreating(true);
    try {
      const run = () => api.admin.createServiceAccount({ name: name.trim(), scopes: selectedScopes, expires_at: expiresAt || undefined });
      const created = await runWithStepUp(run);
      setNewSecret(created);
      setShowCreate(false);
      reset();
      setRefresh((n) => n + 1);
    } catch (err) {
      const code = err instanceof Error ? err.message : "internal_error";
      setCreateError(t(`errors.${code}`, { defaultValue: t("errors.internal_error") }));
    } finally {
      setCreating(false);
    }
  }

  async function handleUpdate(e: React.FormEvent) {
    e.preventDefault();
    if (!editing || !editName.trim()) return;
    setEditError("");
    setUpdating(true);
    try {
      const run = () => api.admin.updateServiceAccount(editing.id, {
        name: editName.trim(),
        scopes: editScopes,
        expires_at: editExpiresAt || null,
        is_active: editActive,
      });
      await runWithStepUp(run);
      setEditing(null);
      setRefresh((n) => n + 1);
    } catch (err) {
      const code = err instanceof Error ? err.message : "internal_error";
      setEditError(t(`errors.${code}`, { defaultValue: t("errors.internal_error") }));
    } finally {
      setUpdating(false);
    }
  }

  function handleCopy() {
    if (!newSecret) return;
    navigator.clipboard.writeText(newSecret.client_secret).then(() => { setCopied(true); setTimeout(() => setCopied(false), 2000); });
  }

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

  async function handleTokenTest() {
    if (!testing || !testSecret) return;
    setTokenLoading(true);
    setTestError("");
    setProbeResult(null);
    try {
      const result = await api.admin.createServiceToken({ client_id: testing.client_id, client_secret: testSecret });
      setTokenResult(result);
      setTokenPayload(decodeServiceJwt(result.access_token));
    } catch (err) {
      const code = err instanceof Error ? err.message : "invalid_client";
      setTokenResult(null);
      setTokenPayload(null);
      setTestError(t(`errors.${code}`, { defaultValue: t("errors.invalid_client") }));
    } finally {
      setTokenLoading(false);
    }
  }

  async function handleProbe() {
    if (!tokenResult) return;
    const target = PROBE_TARGETS.find((item) => item.key === probeKey) ?? PROBE_TARGETS[0];
    setProbeLoading(true);
    setTestError("");
    try {
      const result = await api.admin.probeWithServiceToken(target.path, tokenResult.access_token);
      setProbeResult(result);
    } catch {
      setProbeResult(null);
      setTestError(t("errors.internal_error"));
    } finally {
      setProbeLoading(false);
    }
  }

  const minDate = new Date(Date.now() + 86400000).toISOString().slice(0, 10);
  const editMinDate = editExpiresAt && editExpiresAt < minDate ? editExpiresAt : minDate;
  const selectedProbe = PROBE_TARGETS.find((item) => item.key === probeKey) ?? PROBE_TARGETS[0];
  const tokenScopes = tokenPayload?.scopes ?? [];
  const selectedScopeAllowed = tokenScopes.includes(selectedProbe.scope);
  const tokenExpiresAt = tokenPayload?.exp ? new Date(tokenPayload.exp * 1000) : null;

  return (
    <>
      <ResourceTablePage
        columns={columns} tabs={tabs} fetcher={fetcher} getRowId={(sa) => sa.id}
        accessDenied={!isAdmin} accessDeniedMessage={t("accessDenied")}
        action={<Button variant="contained" onClick={() => { reset(); setShowCreate(true); }}>+ {t("create")}</Button>}
        eventSourceUrl="/api/v1/admin/service-accounts/events"
        defaultSortKey="created_at" defaultSortDir="desc"
        pageSizeOptions={[10, 25, 50]} defaultPageSize={25}
      />

      <Dialog open={showCreate} onClose={() => setShowCreate(false)} fullWidth maxWidth="sm">
        <Box component="form" onSubmit={handleCreate}>
          <DialogTitle>{t("createTitle")}</DialogTitle>
          <DialogContent sx={{ display: "grid", gap: 2, pt: 1 }}>
            <TextField label={t("name")} value={name} onChange={(e) => setName(e.target.value)} required fullWidth />
            <ScopeSelector selected={selectedScopes} onToggle={toggleScope} />
            <TextField label={`${t("expiresAt")} (${t("noExpiry")})`} type="date" value={expiresAt}
              onChange={(e) => setExpiresAt(e.target.value)}
              slotProps={{ inputLabel: { shrink: true }, htmlInput: { min: minDate } }} />
            {createError && <Alert severity="error">{createError}</Alert>}
          </DialogContent>
          <DialogActions sx={{ px: 3, pb: 3 }}>
            <Button onClick={() => setShowCreate(false)}>{tCommon("cancel")}</Button>
            <Button type="submit" variant="contained" disabled={creating}>{creating ? t("creating") : t("create")}</Button>
          </DialogActions>
        </Box>
      </Dialog>

      <Dialog open={!!editing} onClose={() => setEditing(null)} fullWidth maxWidth="sm">
        <Box component="form" onSubmit={handleUpdate}>
          <DialogTitle>{t("editTitle")}</DialogTitle>
          <DialogContent sx={{ display: "grid", gap: 2, pt: 1 }}>
            <TextField label={t("name")} value={editName} onChange={(e) => setEditName(e.target.value)} required fullWidth />
            <TextField
              label={t("clientId")}
              value={editing?.client_id ?? ""}
              disabled
              fullWidth
              helperText={t("clientIdLocked")}
            />
            <FormControlLabel
              control={<Switch checked={editActive} onChange={(e) => setEditActive(e.target.checked)} />}
              label={editActive ? t("active") : t("inactive")}
            />
            <ScopeSelector selected={editScopes} onToggle={toggleEditScope} />
            <TextField label={`${t("expiresAt")} (${t("noExpiry")})`} type="date" value={editExpiresAt}
              onChange={(e) => setEditExpiresAt(e.target.value)}
              slotProps={{ inputLabel: { shrink: true }, htmlInput: { min: editMinDate } }} />
            {editError && <Alert severity="error">{editError}</Alert>}
          </DialogContent>
          <DialogActions sx={{ px: 3, pb: 3 }}>
            <Button onClick={() => setEditing(null)}>{tCommon("cancel")}</Button>
            <Button type="submit" variant="contained" disabled={updating}>{updating ? t("saving") : t("save")}</Button>
          </DialogActions>
        </Box>
      </Dialog>

      <Dialog open={!!testing} onClose={() => setTesting(null)} fullWidth maxWidth="md">
        <DialogTitle sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <Science color="primary" fontSize="small" />
          {t("testTitle")}
        </DialogTitle>
        <DialogContent sx={{ display: "grid", gap: 2.5 }}>
          {(tokenLoading || probeLoading) && <LinearProgress />}

          <Box sx={{ display: "grid", gap: 1 }}>
            <Typography variant="body2" color="text.secondary">{t("testSubtitle")}</Typography>
            <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
              <Chip size="small" color={testing?.is_active ? "success" : "default"} label={testing?.is_active ? t("active") : t("inactive")} />
              <Chip size="small" variant="outlined" label={testing?.name ?? ""} />
              <Chip size="small" variant="outlined" label={testing?.client_id ?? ""} sx={{ fontFamily: "monospace" }} />
            </Box>
          </Box>

          <Box sx={{ display: "grid", gap: 2, gridTemplateColumns: { xs: "1fr", md: "1fr 1fr" } }}>
            <Box sx={{ border: 1, borderColor: "divider", borderRadius: 1, display: "grid", gap: 2, p: 2 }}>
              <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                <Key color="primary" fontSize="small" />
                <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>{t("tokenStep")}</Typography>
              </Box>
              <TextField
                label={t("secretLabel")}
                type="password"
                value={testSecret}
                onChange={(e) => setTestSecret(e.target.value)}
                fullWidth
                autoComplete="off"
                helperText={t("testSecretHint")}
              />
              <Button
                variant="contained"
                startIcon={<Key />}
                onClick={handleTokenTest}
                disabled={!testSecret || tokenLoading}
              >
                {t("getToken")}
              </Button>
            </Box>

            <Box sx={{ border: 1, borderColor: "divider", borderRadius: 1, display: "grid", gap: 2, p: 2 }}>
              <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                <PlayArrow color="primary" fontSize="small" />
                <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>{t("probeStep")}</Typography>
              </Box>
              <TextField select label={t("probeTarget")} value={probeKey} onChange={(e) => setProbeKey(e.target.value as ProbeKey)} fullWidth>
                {PROBE_TARGETS.map((target) => (
                  <MenuItem key={target.key} value={target.key}>
                    {target.method} {target.label}
                  </MenuItem>
                ))}
              </TextField>
              <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                <Chip
                  size="small"
                  color={tokenResult ? (selectedScopeAllowed ? "success" : "warning") : "default"}
                  label={selectedProbe.scope}
                />
                <Typography variant="caption" color="text.secondary">{selectedProbe.path}</Typography>
              </Box>
              <Button
                variant="outlined"
                startIcon={<PlayArrow />}
                onClick={handleProbe}
                disabled={!tokenResult || probeLoading}
              >
                {t("runProbe")}
              </Button>
            </Box>
          </Box>

          {testError && <Alert severity="error">{testError}</Alert>}

          {tokenResult && tokenPayload && (
            <Box sx={{ border: 1, borderColor: "divider", borderRadius: 1, overflow: "hidden" }}>
              <Box sx={{ alignItems: "center", display: "flex", gap: 1, px: 2, py: 1.5 }}>
                <VerifiedUser color="success" fontSize="small" />
                <Typography variant="subtitle2" sx={{ fontWeight: 700, flex: 1 }}>{t("tokenReady")}</Typography>
                <Chip size="small" color="success" label={`${tokenResult.expires_in}s`} />
              </Box>
              <Divider />
              <Box sx={{ display: "grid", gap: 1.25, p: 2 }}>
                <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.75 }}>
                  {tokenScopes.length > 0 ? tokenScopes.map((scope) => (
                    <Chip key={scope} size="small" color={scope === selectedProbe.scope ? "success" : "info"} label={scope} />
                  )) : <Chip size="small" variant="outlined" label={t("noScopes")} />}
                </Box>
                <Typography variant="caption" color="text.secondary">
                  {t("tokenMeta", {
                    subject: tokenPayload.sub ?? "-",
                    expires: tokenExpiresAt ? tokenExpiresAt.toLocaleTimeString() : "-",
                  })}
                </Typography>
              </Box>
            </Box>
          )}

          {probeResult && (
            <Box sx={{ border: 1, borderColor: probeResult.ok ? "success.main" : "error.main", borderRadius: 1, overflow: "hidden" }}>
              <Box sx={{ alignItems: "center", display: "flex", gap: 1, px: 2, py: 1.5 }}>
                <Chip size="small" color={probeResult.ok ? "success" : "error"} label={`${probeResult.status} ${probeResult.statusText}`} />
                <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>{probeResult.ok ? t("probePassed") : t("probeBlocked")}</Typography>
              </Box>
              <Divider />
              <Box
                component="pre"
                sx={{
                  bgcolor: "background.default",
                  color: "text.secondary",
                  fontFamily: "monospace",
                  fontSize: 12,
                  m: 0,
                  maxHeight: 260,
                  overflow: "auto",
                  p: 2,
                  whiteSpace: "pre-wrap",
                }}
              >
                {prettyJson(probeResult.body)}
              </Box>
            </Box>
          )}
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 3 }}>
          <Button onClick={() => setTesting(null)}>{t("done")}</Button>
        </DialogActions>
      </Dialog>

      <Dialog open={!!newSecret} onClose={() => setNewSecret(null)} fullWidth maxWidth="sm">
        <DialogTitle>{t("secretTitle")}</DialogTitle>
        <DialogContent sx={{ display: "grid", gap: 2 }}>
          <DialogContentText color="warning.main">{t("secretWarning")}</DialogContentText>
          <Box>
            <Typography variant="caption" color="text.secondary">{t("secretLabel")}</Typography>
            <Box sx={{ alignItems: "center", bgcolor: "background.default", border: 1, borderColor: "divider", borderRadius: 1,
              color: "success.main", display: "flex", fontFamily: "monospace", gap: 1, mt: 0.5, p: 1.5 }}>
              <Typography variant="body2" sx={{ flex: 1, overflowWrap: "anywhere", fontFamily: "monospace" }}>
                {newSecret?.client_secret}
              </Typography>
              <Tooltip title="Copy">
                <IconButton size="small" onClick={handleCopy}>
                  {copied ? <Check color="success" fontSize="small" /> : <ContentCopy fontSize="small" />}
                </IconButton>
              </Tooltip>
            </Box>
          </Box>
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 3 }}>
          <Button variant="contained" fullWidth onClick={() => setNewSecret(null)}>{t("done")}</Button>
        </DialogActions>
      </Dialog>
    </>
  );
}
