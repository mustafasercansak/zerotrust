"use client";

import { useCallback, useMemo, useState } from "react";
import { useTranslations, useLocale } from "next-intl";
import { api, type PageParams, type ServiceAccount, type ServiceAccountCreated } from "@/lib/api";
import { formatDate } from "@/lib/dateUtils";
import { useMeContext } from "../context";
import { ResourceTablePage } from "@/components/ResourceTablePage";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Checkbox from "@mui/material/Checkbox";
import Chip from "@mui/material/Chip";
import Dialog from "@mui/material/Dialog";
import DialogActions from "@mui/material/DialogActions";
import DialogContent from "@mui/material/DialogContent";
import DialogContentText from "@mui/material/DialogContentText";
import DialogTitle from "@mui/material/DialogTitle";
import FormControlLabel from "@mui/material/FormControlLabel";
import IconButton from "@mui/material/IconButton";
import Stack from "@mui/material/Stack";
import TextField from "@mui/material/TextField";
import Tooltip from "@mui/material/Tooltip";
import Typography from "@mui/material/Typography";
import Check from "@mui/icons-material/Check";
import ContentCopy from "@mui/icons-material/ContentCopy";
import type { GridColDef } from "@mui/x-data-grid";

const ALL_SCOPES = [
  "users:read",
  "users:write",
  "service_accounts:read",
  "service_accounts:write",
  "service_accounts:delete",
];

export default function ServiceAccountsPage() {
  const t = useTranslations("serviceAccounts");
  const tCommon = useTranslations("common");
  const locale = useLocale();
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
  const [refresh, setRefresh] = useState(0);

  const fetcher = useCallback(
    (p: PageParams) => api.admin.listServiceAccounts(p),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [refresh],
  );

  const tabs = useMemo(() => [
    { key: "all",      label: tCommon("filterAll") },
    { key: "active",   label: tCommon("filterActive"),   preset: { status: "active" } },
    { key: "inactive", label: tCommon("filterInactive"), preset: { status: "inactive" } },
    { key: "expired",  label: tCommon("filterExpired"),  preset: { status: "expired" } },
  ], [tCommon]);

  const columns = useMemo<GridColDef<ServiceAccount>[]>(() => [
    {
      field: "name",
      headerName: t("name"),
      minWidth: 180,
      flex: 1,
      renderCell: ({ row }) => <Typography variant="body2" fontWeight={600}>{row.name}</Typography>,
    },
    {
      field: "client_id",
      headerName: t("clientId"),
      minWidth: 260,
      flex: 1.2,
      sortable: false,
      renderCell: ({ row }) => (
        <Typography variant="caption" color="text.secondary" sx={{ fontFamily: "monospace" }}>
          {row.client_id}
        </Typography>
      ),
    },
    {
      field: "scopes",
      headerName: t("scopes"),
      minWidth: 240,
      flex: 1.2,
      sortable: false,
      filterable: false,
      renderCell: ({ row }) =>
        row.scopes.length === 0 ? (
          <Chip size="small" variant="outlined" label={t("noScopes")} />
        ) : (
          <Stack direction="row" spacing={0.5} sx={{ alignItems: "center", height: "100%" }}>
            {row.scopes.map((s) => <Chip key={s} size="small" color="info" label={s} />)}
          </Stack>
        ),
    },
    {
      field: "is_active",
      headerName: t("status"),
      minWidth: 130,
      flex: 0.6,
      renderCell: ({ row }) => (
        <Chip
          size="small"
          color={row.is_active ? "success" : "default"}
          label={row.is_active ? t("active") : t("inactive")}
          onClick={() => handleToggleStatus(row)}
          clickable
          title={row.is_active ? t("deactivateHint") : t("activateHint")}
        />
      ),
    },
    {
      field: "created_at",
      headerName: t("createdAt"),
      minWidth: 150,
      flex: 0.7,
      renderCell: ({ row }) => (
        <Typography variant="caption" color="text.secondary">
          {formatDate(row.created_at, locale)}
        </Typography>
      ),
    },
    {
      field: "expires_at",
      headerName: t("expiresAt"),
      minWidth: 150,
      flex: 0.7,
      renderCell: ({ row }) => {
        if (!row.expires_at) return <Chip size="small" variant="outlined" label={t("noExpiry")} />;
        const expired = new Date(row.expires_at) < new Date();
        return expired
          ? <Chip size="small" color="error" label={t("expired")} />
          : (
            <Typography variant="caption" color="text.secondary">
              {formatDate(row.expires_at, locale)}
            </Typography>
          );
      },
    },
    {
      field: "actions",
      headerName: "",
      sortable: false,
      filterable: false,
      align: "right",
      width: 120,
      renderCell: ({ row }) => (
        <Button color="error" size="small" onClick={() => handleRevoke(row)}>
          {t("revoke")}
        </Button>
      ),
    },
  // eslint-disable-next-line react-hooks/exhaustive-deps
  ], [t, locale]);

  async function handleToggleStatus(sa: ServiceAccount) {
    try {
      await api.admin.setServiceAccountStatus(sa.id, !sa.is_active);
      setRefresh((n) => n + 1);
    } catch {
      alert(t("errors.internal_error"));
    }
  }

  async function handleRevoke(sa: ServiceAccount) {
    if (!confirm(t("revokeConfirm", { name: sa.name }))) return;
    try {
      await api.admin.revokeServiceAccount(sa.id);
      setRefresh((n) => n + 1);
    } catch {
      alert(t("errors.internal_error"));
    }
  }

  function resetForm() {
    setName(""); setSelectedScopes([]); setExpiresAt(""); setCreateError("");
  }

  function toggleScope(scope: string) {
    setSelectedScopes((prev) =>
      prev.includes(scope) ? prev.filter((s) => s !== scope) : [...prev, scope]
    );
  }

  async function handleCreate(e: { preventDefault(): void }) {
    e.preventDefault();
    setCreateError("");
    if (!name.trim()) return;
    setCreating(true);
    try {
      const created = await api.admin.createServiceAccount({
        name: name.trim(), scopes: selectedScopes, expires_at: expiresAt || undefined,
      });
      setNewSecret(created);
      setShowCreate(false);
      resetForm();
      setRefresh((n) => n + 1);
    } catch (err) {
      const code = err instanceof Error ? err.message : "internal_error";
      setCreateError(t(`errors.${code}` as Parameters<typeof t>[0], { defaultValue: t("errors.internal_error") }));
    } finally {
      setCreating(false);
    }
  }

  function handleCopy() {
    if (!newSecret) return;
    navigator.clipboard.writeText(newSecret.client_secret).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }

  const tomorrow = new Date();
  tomorrow.setDate(tomorrow.getDate() + 1);
  const minDate = tomorrow.toISOString().slice(0, 10);

  return (
    <>
      <ResourceTablePage
        columns={columns}
        tabs={tabs}
        fetcher={fetcher}
        getRowId={(sa) => sa.id}
        accessDenied={!isAdmin}
        accessDeniedMessage={t("accessDenied")}
        action={(
          <Button variant="contained" onClick={() => { resetForm(); setShowCreate(true); }}>
            + {t("create")}
          </Button>
        )}
        defaultSortKey="created_at"
        defaultSortDir="desc"
        pageSizeOptions={[10, 25, 50]}
        defaultPageSize={25}
      />

      <Dialog open={showCreate} onClose={() => setShowCreate(false)} fullWidth maxWidth="sm">
        <Box component="form" onSubmit={handleCreate}>
          <DialogTitle>{t("createTitle")}</DialogTitle>
          <DialogContent sx={{ display: "grid", gap: 2, pt: 1 }}>
              <TextField
                id="sa-name"
                label={t("name")}
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
                placeholder={t("name")}
                fullWidth
              />

              <Box>
                <Typography variant="caption" color="text.secondary">{t("scopes")}</Typography>
                <Box sx={{ display: "grid", gap: 0.5, mt: 0.5 }}>
                {ALL_SCOPES.map((scope) => (
                  <FormControlLabel
                    key={scope}
                    control={(
                      <Checkbox
                        size="small"
                      checked={selectedScopes.includes(scope)}
                      onChange={() => toggleScope(scope)}
                      />
                    )}
                    label={<Typography variant="caption" sx={{ fontFamily: "monospace" }}>{scope}</Typography>}
                  />
                ))}
                </Box>
              </Box>

              <TextField
                id="sa-expires"
                label={`${t("expiresAt")} (${t("noExpiry")})`}
                type="date"
                value={expiresAt}
                onChange={(e) => setExpiresAt(e.target.value)}
                InputLabelProps={{ shrink: true }}
                slotProps={{ htmlInput: { min: minDate } }}
              />

            {createError && (
              <Alert severity="error">
                {createError}
              </Alert>
            )}
          </DialogContent>
          <DialogActions sx={{ px: 3, pb: 3 }}>
            <Button onClick={() => setShowCreate(false)}>{tCommon("cancel")}</Button>
            <Button type="submit" variant="contained" disabled={creating}>
              {creating ? t("creating") : t("create")}
            </Button>
          </DialogActions>
        </Box>
      </Dialog>

      <Dialog open={!!newSecret} onClose={() => setNewSecret(null)} fullWidth maxWidth="sm">
        <DialogTitle>{t("secretTitle")}</DialogTitle>
        <DialogContent sx={{ display: "grid", gap: 2 }}>
          <DialogContentText color="warning.main">
            {t("secretWarning")}
          </DialogContentText>

          <Box>
            <Typography variant="caption" color="text.secondary">{t("secretLabel")}</Typography>
            <Box
              sx={{
                alignItems: "center",
                bgcolor: "background.default",
                border: 1,
                borderColor: "divider",
                borderRadius: 1,
                color: "success.main",
                display: "flex",
                fontFamily: "monospace",
                gap: 1,
                mt: 0.5,
                p: 1.5,
              }}
            >
              <Typography variant="body2" sx={{ flex: 1, overflowWrap: "anywhere", fontFamily: "monospace" }}>
                {newSecret?.client_secret}
              </Typography>
              <Tooltip title="Copy">
                <IconButton
                  size="small"
                  onClick={handleCopy}
                >
                  {copied ? <Check color="success" fontSize="small" /> : <ContentCopy fontSize="small" />}
                </IconButton>
              </Tooltip>
            </Box>
          </Box>
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 3 }}>
          <Button variant="contained" fullWidth onClick={() => setNewSecret(null)}>
            {t("done")}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
}
