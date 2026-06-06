import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, ApiError, OidcClient } from "@/lib/api";
import { toast } from "sonner";
import {
  DataGrid,
  GridColDef,
  GridRenderCellParams,
  GridToolbar,
} from "@mui/x-data-grid";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import CircularProgress from "@mui/material/CircularProgress";
import Dialog from "@mui/material/Dialog";
import DialogActions from "@mui/material/DialogActions";
import DialogContent from "@mui/material/DialogContent";
import DialogTitle from "@mui/material/DialogTitle";
import IconButton from "@mui/material/IconButton";
import Paper from "@mui/material/Paper";
import TextField from "@mui/material/TextField";
import Tooltip from "@mui/material/Tooltip";
import Typography from "@mui/material/Typography";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import DeleteOutlineIcon from "@mui/icons-material/DeleteOutlined";
import EditIcon from "@mui/icons-material/Edit";
import LinkIcon from "@mui/icons-material/Link";

export default function OidcClientsSection() {
  const { t } = useTranslation("settings");
  const { t: tCommon } = useTranslation("common");
  const [clients, setClients] = useState<OidcClient[]>([]);
  const [loading, setLoading] = useState(true);

  // Dialog State
  const [openCreate, setOpenCreate] = useState(false);
  const [name, setName] = useState("");
  const [clientIdInput, setClientIdInput] = useState("");
  const [redirectUris, setRedirectUris] = useState("");
  const [allowedScopes, setAllowedScopes] = useState("openid profile email");
  const [submitting, setSubmitting] = useState(false);

  // Edit State
  const [editingClient, setEditingClient] = useState<OidcClient | null>(null);
  const [editName, setEditName] = useState("");
  const [editRedirectUris, setEditRedirectUris] = useState("");
  const [editAllowedScopes, setEditAllowedScopes] = useState("");

  // Secret Display State
  const [createdClient, setCreatedClient] = useState<OidcClient | null>(null);

  useEffect(() => { loadClients(); }, []);

  async function loadClients() {
    setLoading(true);
    try {
      const data = await api.admin.listOidcClients();
      setClients(data || []);
    } catch {
      toast.error(t("oidc.errors.internal_error"));
    } finally {
      setLoading(false);
    }
  }

  async function runWithStepUp<T>(action: () => Promise<T>): Promise<T> {
    try {
      return await action();
    } catch (err) {
      if (!(err instanceof ApiError) || err.message !== "mfa_required") throw err;
      const code = window.prompt(t("mfaPrompt"))?.trim() ?? "";
      if (!code) throw err;
      await api.mfaStepUp(code);
      return action();
    }
  }

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    try {
      const uris = redirectUris.split(",").map((x) => x.trim()).filter(Boolean);
      const scopes = allowedScopes.split(" ").map((x) => x.trim()).filter(Boolean);
      const client = await runWithStepUp(() =>
        api.admin.createOidcClient({ client_id: clientIdInput.trim(), name: name.trim(), redirect_uris: uris, allowed_scopes: scopes })
      );
      setCreatedClient(client);
      setOpenCreate(false);
      setName(""); setClientIdInput(""); setRedirectUris(""); setAllowedScopes("openid profile email");
      loadClients();
    } catch (err) {
      const code = err instanceof ApiError ? err.message : "internal_error";
      toast.error(t(`oidc.errors.${code}`, { defaultValue: t("oidc.errors.internal_error") }));
    } finally {
      setSubmitting(false);
    }
  }

  async function handleDelete(client: OidcClient) {
    if (!window.confirm(t("oidc.deleteConfirm", { name: client.name }))) return;
    try {
      await runWithStepUp(() => api.admin.deleteOidcClient(client.id));
      toast.success(t("saved"));
      loadClients();
    } catch (err) {
      const code = err instanceof ApiError ? err.message : "internal_error";
      toast.error(t(`oidc.errors.${code}`, { defaultValue: t("oidc.errors.internal_error") }));
    }
  }

  function handleEditClick(client: OidcClient) {
    setEditingClient(client);
    setEditName(client.name);
    setEditRedirectUris(client.redirect_uris.join(", "));
    setEditAllowedScopes(client.allowed_scopes.join(" "));
  }

  async function handleUpdate(e: React.FormEvent) {
    e.preventDefault();
    if (!editingClient) return;
    setSubmitting(true);
    try {
      const uris = editRedirectUris.split(",").map((x) => x.trim()).filter(Boolean);
      const scopes = editAllowedScopes.split(" ").map((x) => x.trim()).filter(Boolean);
      await runWithStepUp(() =>
        api.admin.updateOidcClient(editingClient.id, { name: editName.trim(), redirect_uris: uris, allowed_scopes: scopes })
      );
      setEditingClient(null);
      toast.success(t("saved"));
      loadClients();
    } catch (err) {
      const code = err instanceof ApiError ? err.message : "internal_error";
      toast.error(t(`oidc.errors.${code}`, { defaultValue: t("oidc.errors.internal_error") }));
    } finally {
      setSubmitting(false);
    }
  }

  function handleCopy(text: string) {
    navigator.clipboard.writeText(text);
    toast.success(t("oidc.copied", { defaultValue: "Copied" }));
  }

  const columns: GridColDef<OidcClient>[] = [
    {
      field: "name",
      headerName: t("oidc.name"),
      flex: 1,
      minWidth: 160,
      renderCell: (params: GridRenderCellParams<OidcClient>) => (
        <Typography variant="body2" sx={{ fontWeight: 700, lineHeight: "52px" }}>
          {params.value}
        </Typography>
      ),
    },
    {
      field: "client_id",
      headerName: t("oidc.clientId"),
      flex: 1,
      minWidth: 160,
      renderCell: (params: GridRenderCellParams<OidcClient>) => (
        <Box sx={{ display: "flex", alignItems: "center", gap: 0.5, height: "100%" }}>
          <Typography
            variant="caption"
            sx={{
              fontFamily: "monospace",
              color: "primary.main",
              bgcolor: "rgba(99,102,241,0.12)",
              px: 0.75,
              py: 0.25,
              borderRadius: 1,
              fontWeight: 700,
            }}
          >
            {params.value}
          </Typography>
          <Tooltip title="Copy">
            <IconButton size="small" onClick={() => handleCopy(params.value as string)} sx={{ opacity: 0.5, "&:hover": { opacity: 1 }, p: 0.25 }}>
              <ContentCopyIcon sx={{ fontSize: 13 }} />
            </IconButton>
          </Tooltip>
        </Box>
      ),
    },
    {
      field: "redirect_uris",
      headerName: t("oidc.redirectUrisHeader", { defaultValue: "Redirect URIs" }),
      flex: 1.5,
      minWidth: 200,
      sortable: false,
      renderCell: (params: GridRenderCellParams<OidcClient>) => {
        const uris: string[] = params.value || [];
        return (
          <Box sx={{ display: "flex", flexDirection: "column", gap: 0.4, justifyContent: "center", height: "100%", py: 0.5 }}>
            {uris.map((uri) => (
              <Box key={uri} sx={{ display: "flex", alignItems: "center", gap: 0.4 }}>
                <LinkIcon sx={{ fontSize: 11, color: "text.disabled", flexShrink: 0 }} />
                <Typography
                  variant="caption"
                  sx={{
                    fontFamily: "monospace",
                    color: "text.secondary",
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                    whiteSpace: "nowrap",
                  }}
                >
                  {uri}
                </Typography>
              </Box>
            ))}
          </Box>
        );
      },
    },
    {
      field: "allowed_scopes",
      headerName: t("oidc.scopes", { defaultValue: "Scopes" }),
      flex: 1,
      minWidth: 160,
      sortable: false,
      renderCell: (params: GridRenderCellParams<OidcClient>) => {
        const scopes: string[] = params.value || [];
        return (
          <Box sx={{ display: "flex", gap: 0.5, flexWrap: "wrap", alignItems: "center", height: "100%" }}>
            {scopes.map((s) => (
              <Chip
                key={s}
                label={s}
                size="small"
                variant="outlined"
                sx={{
                  fontSize: "0.68rem",
                  height: 20,
                  borderColor: "rgba(99,102,241,0.4)",
                  color: "primary.light",
                }}
              />
            ))}
          </Box>
        );
      },
    },
    {
      field: "actions",
      headerName: t("oidc.actions", { defaultValue: "Actions" }),
      width: 110,
      sortable: false,
      filterable: false,
      align: "right",
      headerAlign: "right",
      renderCell: (params: GridRenderCellParams<OidcClient>) => (
        <Box sx={{ display: "flex", alignItems: "center", gap: 0.5, height: "100%", justifyContent: "flex-end" }}>
          <Tooltip title={t("oidc.editAction", { defaultValue: "Edit" })}>
            <IconButton
              size="small"
              onClick={() => handleEditClick(params.row)}
              sx={{
                border: "1px solid",
                borderColor: "rgba(255,255,255,0.12)",
                color: "text.secondary",
                "&:hover": { borderColor: "primary.main", color: "primary.main" },
              }}
            >
              <EditIcon sx={{ fontSize: 15 }} />
            </IconButton>
          </Tooltip>
          <Tooltip title={t("oidc.delete", { defaultValue: "Delete" })}>
            <IconButton
              size="small"
              color="error"
              onClick={() => handleDelete(params.row)}
              sx={{
                border: "1px solid",
                borderColor: "rgba(239,68,68,0.3)",
                "&:hover": { borderColor: "error.main", bgcolor: "rgba(239,68,68,0.08)" },
              }}
            >
              <DeleteOutlineIcon sx={{ fontSize: 15 }} />
            </IconButton>
          </Tooltip>
        </Box>
      ),
    },
  ];

  if (loading) {
    return (
      <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, py: 4 }}>
        <CircularProgress size={18} />
        <Typography variant="body2" color="text.secondary">{t("loading")}</Typography>
      </Box>
    );
  }

  return (
    <Box sx={{ display: "flex", flexDirection: "column", height: "100%", gap: 3 }}>
      {/* Header */}
      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <Typography variant="h6" sx={{ fontWeight: 700 }}>{t("oidc.title")}</Typography>
        <Button
          variant="contained"
          size="small"
          onClick={() => setOpenCreate(true)}
          sx={{
            background: "linear-gradient(135deg, #4f46e5 0%, #6366f1 100%)",
            "&:hover": { background: "linear-gradient(135deg, #4338ca 0%, #4f46e5 100%)" },
          }}
        >
          {t("oidc.create")}
        </Button>
      </Box>

      {/* DataGrid */}
      <Paper
        variant="outlined"
        sx={{ flex: 1, minHeight: 0, bgcolor: "#0b1120", borderColor: "rgba(255,255,255,0.08)", borderRadius: 3, overflow: "hidden", display: "flex", flexDirection: "column" }}
      >
        <DataGrid
          rows={clients}
          columns={columns}
          getRowId={(r) => r.id}
          rowHeight={60}
          pageSizeOptions={[10, 25, 50]}
          initialState={{ pagination: { paginationModel: { pageSize: 10 } } }}
          slots={{ toolbar: GridToolbar }}
          slotProps={{
            toolbar: {
              showQuickFilter: true,
              quickFilterProps: { debounceMs: 300 },
            },
          }}
          disableRowSelectionOnClick
          sx={{
            border: "none",
            flex: 1,
            height: "100%",
            "& .MuiDataGrid-columnHeaders": {
              bgcolor: "rgba(255,255,255,0.03)",
              borderBottom: "1px solid rgba(255,255,255,0.08)",
              "& .MuiDataGrid-columnHeaderTitle": {
                fontWeight: 700,
                fontSize: "0.75rem",
                textTransform: "uppercase",
                letterSpacing: "0.06em",
                color: "text.disabled",
              },
            },
            "& .MuiDataGrid-row": {
              borderBottom: "1px solid rgba(255,255,255,0.04)",
              "&:hover": {
                bgcolor: "rgba(99,102,241,0.06)",
              },
              "&:last-child": { borderBottom: "none" },
            },
            "& .MuiDataGrid-cell": {
              border: "none",
              alignItems: "center",
            },
            "& .MuiDataGrid-footerContainer": {
              borderTop: "1px solid rgba(255,255,255,0.08)",
              bgcolor: "rgba(255,255,255,0.02)",
              flexShrink: 0,
            },
            "& .MuiDataGrid-toolbarContainer": {
              px: 2,
              py: 1,
              borderBottom: "1px solid rgba(255,255,255,0.06)",
              gap: 1,
              flexShrink: 0,
              "& .MuiInputBase-root": {
                fontSize: "0.85rem",
              },
            },
            "& .MuiDataGrid-main": {
              flex: 1,
              minHeight: 0,
            },
            "& .MuiToolbar-root": {
              color: "text.secondary",
              fontSize: "0.8rem",
            },
          }}
        />
      </Paper>

      {/* Create Dialog */}
      <Dialog open={openCreate} onClose={() => setOpenCreate(false)} maxWidth="sm" fullWidth>
        <Box component="form" onSubmit={handleCreate}>
          <DialogTitle>{t("oidc.createTitle")}</DialogTitle>
          <DialogContent sx={{ display: "grid", gap: 2.5, pt: 1 }}>
            <TextField label={t("oidc.name")} value={name} onChange={(e) => setName(e.target.value)} placeholder="My Web Application" required fullWidth />
            <TextField label={t("oidc.clientId")} value={clientIdInput} onChange={(e) => setClientIdInput(e.target.value)} placeholder="my-web-app" required fullWidth />
            <TextField label={t("oidc.redirectUris")} value={redirectUris} onChange={(e) => setRedirectUris(e.target.value)} placeholder="http://localhost:3000/callback, https://myapp.com/callback" required fullWidth />
            <TextField label={t("oidc.allowedScopes")} value={allowedScopes} onChange={(e) => setAllowedScopes(e.target.value)} placeholder="openid profile email" required fullWidth />
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setOpenCreate(false)} color="inherit">{tCommon("cancel")}</Button>
            <Button type="submit" variant="contained" disabled={submitting}>
              {submitting ? "..." : t("oidc.create")}
            </Button>
          </DialogActions>
        </Box>
      </Dialog>

      {/* Secret Display Dialog */}
      <Dialog open={!!createdClient} onClose={() => setCreatedClient(null)} maxWidth="sm" fullWidth>
        <DialogTitle>{t("oidc.secretTitle")}</DialogTitle>
        <DialogContent sx={{ display: "grid", gap: 2.5, pt: 1 }}>
          <Typography variant="body2" color="warning.main" sx={{ fontWeight: 600 }}>{t("oidc.secretWarning")}</Typography>
          <Box sx={{ bgcolor: "background.default", border: 1, borderColor: "divider", borderRadius: 1, p: 2 }}>
            <Typography variant="caption" color="text.secondary" sx={{ display: "block" }}>{t("oidc.clientId")}</Typography>
            <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <Typography sx={{ fontFamily: "monospace", fontWeight: 700 }}>{createdClient?.client_id}</Typography>
              <Button size="small" onClick={() => handleCopy(createdClient?.client_id || "")}><ContentCopyIcon fontSize="small" /></Button>
            </Box>
          </Box>
          <Box sx={{ bgcolor: "background.default", border: 1, borderColor: "divider", borderRadius: 1, p: 2 }}>
            <Typography variant="caption" color="text.secondary" sx={{ display: "block" }}>{t("oidc.clientSecret")}</Typography>
            <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <Typography sx={{ fontFamily: "monospace", fontWeight: 700, overflowWrap: "anywhere" }}>{createdClient?.client_secret}</Typography>
              <Button size="small" onClick={() => handleCopy(createdClient?.client_secret || "")}><ContentCopyIcon fontSize="small" /></Button>
            </Box>
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setCreatedClient(null)} variant="contained">{t("oidc.done")}</Button>
        </DialogActions>
      </Dialog>

      {/* Edit Dialog */}
      <Dialog open={!!editingClient} onClose={() => setEditingClient(null)} maxWidth="sm" fullWidth>
        <Box component="form" onSubmit={handleUpdate}>
          <DialogTitle>{t("oidc.editTitle", { defaultValue: "Edit OIDC Client" })}</DialogTitle>
          <DialogContent sx={{ display: "grid", gap: 2.5, pt: 1 }}>
            <TextField label={t("oidc.name")} value={editName} onChange={(e) => setEditName(e.target.value)} required fullWidth />
            <TextField label={t("oidc.redirectUris")} value={editRedirectUris} onChange={(e) => setEditRedirectUris(e.target.value)} required fullWidth />
            <TextField label={t("oidc.allowedScopes")} value={editAllowedScopes} onChange={(e) => setEditAllowedScopes(e.target.value)} required fullWidth />
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setEditingClient(null)} color="inherit">{tCommon("cancel")}</Button>
            <Button type="submit" variant="contained" disabled={submitting}>
              {submitting ? "..." : t("save")}
            </Button>
          </DialogActions>
        </Box>
      </Dialog>
    </Box>
  );
}
