"use client";

import { useState, useCallback, useMemo } from "react";
import { useTranslations, useLocale } from "next-intl";
import { api, ApiError, type PageParams, type UserData } from "@/lib/api";
import { formatDate } from "@/lib/dateUtils";
import { useMeContext } from "../context";
import { ResourceTablePage } from "@/components/ResourceTablePage";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import Dialog from "@mui/material/Dialog";
import DialogActions from "@mui/material/DialogActions";
import DialogContent from "@mui/material/DialogContent";
import DialogTitle from "@mui/material/DialogTitle";
import Stack from "@mui/material/Stack";
import TextField from "@mui/material/TextField";
import Typography from "@mui/material/Typography";
import type { GridColDef } from "@mui/x-data-grid";

const AVAILABLE_ROLES = ["admin", "user"];

export default function UsersPage() {
  const t = useTranslations("admin");
  const tCommon = useTranslations("common");
  const locale = useLocale();
  const me = useMeContext();
  const isAdmin = me?.roles.includes("admin") ?? false;

  const [showModal, setShowModal] = useState(false);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [selectedRoles, setSelectedRoles] = useState<string[]>(["user"]);
  const [formError, setFormError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [refresh, setRefresh] = useState(0);

  const fetcher = useCallback(
    (p: PageParams) => api.admin.listUsers(p),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [refresh],
  );

  const tabs = useMemo(() => [
    { key: "all",      label: tCommon("filterAll") },
    { key: "active",   label: tCommon("filterActive"),   preset: { status: "active" } },
    { key: "inactive", label: tCommon("filterInactive"), preset: { status: "inactive" } },
  ], [tCommon]);

  const columns = useMemo<GridColDef<UserData>[]>(() => [
    {
      field: "email",
      headerName: t("email"),
      minWidth: 260,
      flex: 1.3,
      renderCell: ({ row }) => <Typography variant="body2">{row.email}</Typography>,
    },
    {
      field: "roles",
      headerName: t("roles"),
      minWidth: 180,
      flex: 0.9,
      sortable: false,
      filterable: false,
      renderCell: ({ row }) =>
        row.roles.length === 0 ? (
          <Chip size="small" variant="outlined" label={t("noRoles")} />
        ) : (
          <Stack direction="row" spacing={0.5} sx={{ alignItems: "center", height: "100%" }}>
            {row.roles.map((r) => <Chip key={r} size="small" color="primary" label={r} />)}
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
          color={row.is_active ? "success" : "error"}
          label={row.is_active ? t("active") : t("inactive")}
        />
      ),
    },
    {
      field: "created_at",
      headerName: t("createdAt"),
      minWidth: 160,
      flex: 0.8,
      renderCell: ({ row }) => (
        <Typography variant="caption" color="text.secondary">
          {formatDate(row.created_at, locale)}
        </Typography>
      ),
    },
  ], [t, locale]);

  function toggleRole(role: string) {
    setSelectedRoles((prev) =>
      prev.includes(role) ? prev.filter((r) => r !== role) : [...prev, role]
    );
  }

  function openModal() {
    setEmail("");
    setPassword("");
    setSelectedRoles(["user"]);
    setFormError(null);
    setShowModal(true);
  }

  async function handleCreate(e: { preventDefault(): void }) {
    e.preventDefault();
    setFormError(null);
    setCreating(true);
    try {
      await api.admin.createUser({ email, password, locale: me?.locale ?? "tr", roles: selectedRoles });
      setShowModal(false);
      setRefresh((n) => n + 1);
    } catch (err) {
      const code = err instanceof ApiError ? err.message : "internal_error";
      setFormError(t(`errors.${code}` as Parameters<typeof t>[0], { defaultValue: t("errors.internal_error") }));
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
        action={<Button variant="contained" onClick={openModal}>+ {t("createUser")}</Button>}
        defaultSortKey="created_at"
        defaultSortDir="desc"
        pageSizeOptions={[10, 25, 50]}
        defaultPageSize={25}
      />

      <Dialog open={showModal} onClose={() => setShowModal(false)} fullWidth maxWidth="xs">
        <Box component="form" onSubmit={handleCreate}>
          <DialogTitle>{t("createUserTitle")}</DialogTitle>
          <DialogContent sx={{ display: "grid", gap: 2, pt: 1 }}>
              <TextField
                id="email"
                label={t("email")}
                type="email"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                fullWidth
              />

              <TextField
                id="password"
                label={t("password")}
                type="password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                fullWidth
              />

              <Stack direction="row" spacing={1}>
                {AVAILABLE_ROLES.map((role) => (
                  <Chip
                    key={role}
                    label={role}
                    color={selectedRoles.includes(role) ? "primary" : "default"}
                    variant={selectedRoles.includes(role) ? "filled" : "outlined"}
                    onClick={() => toggleRole(role)}
                  />
                ))}
              </Stack>

            {formError && (
              <Alert severity="error">
                {formError}
              </Alert>
            )}
          </DialogContent>
          <DialogActions sx={{ px: 3, pb: 3 }}>
            <Button onClick={() => setShowModal(false)}>{tCommon("cancel")}</Button>
            <Button type="submit" variant="contained" disabled={creating}>
              {creating ? t("creating") : t("create")}
            </Button>
          </DialogActions>
        </Box>
      </Dialog>
    </>
  );
}
