"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { api, ApiError } from "@/lib/api";
import { useMeContext } from "../context";
import { DashboardPage } from "@/components/DashboardPage";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import CircularProgress from "@mui/material/CircularProgress";
import Paper from "@mui/material/Paper";
import TextField from "@mui/material/TextField";
import Typography from "@mui/material/Typography";

export default function SettingsPage() {
  const t = useTranslations("settings");
  const me = useMeContext();

  const [maxSessions, setMaxSessions] = useState<string>("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const isAdmin = me?.roles.includes("admin") ?? false;

  useEffect(() => {
    if (!isAdmin) return;
    api.admin.getSettings()
      .then((s) => setMaxSessions(s["max_sessions_per_user"] ?? "5"))
      .catch(() => setError("internal_error"))
      .finally(() => setLoading(false));
  }, [isAdmin]);

  async function handleSave(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSaved(false);

    const n = parseInt(maxSessions, 10);
    if (isNaN(n) || n < 1 || n > 20) {
      setError("invalid_value");
      return;
    }

    setSaving(true);
    try {
      await api.admin.updateSettings({ max_sessions_per_user: String(n) });
      setSaved(true);
      setTimeout(() => setSaved(false), 3000);
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError("internal_error");
      }
    } finally {
      setSaving(false);
    }
  }

  return (
    <DashboardPage accessDenied={!isAdmin} accessDeniedMessage={t("accessDenied")}>
      <Box sx={{ maxWidth: 560 }}>
      <Paper component="form" onSubmit={handleSave} variant="outlined" sx={{ display: "grid", gap: 3, p: 3 }}>
        {loading ? (
          <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
            <CircularProgress size={18} />
            <Typography variant="body2" color="text.secondary">Loading...</Typography>
          </Box>
        ) : (
          <Box>
            <TextField
              id="max-sessions"
              label={t("maxSessions")}
              type="number"
              value={maxSessions}
              onChange={(e) => setMaxSessions(e.target.value)}
              sx={{ width: 140 }}
              slotProps={{ htmlInput: { min: 1, max: 20 } }}
            />
            <Typography variant="caption" color="text.secondary" sx={{ display: "block", mt: 1 }}>
              {t("maxSessionsDesc")}
            </Typography>
          </Box>
        )}

        {error && (
          <Alert severity="error">
            {t(`errors.${error}` as Parameters<typeof t>[0], { defaultValue: t("errors.internal_error") })}
          </Alert>
        )}
        {saved && <Alert severity="success">{t("saved")}</Alert>}

        <Button type="submit" variant="contained" disabled={saving || loading} sx={{ justifySelf: "start" }}>
          {saving ? t("saving") : t("save")}
        </Button>
      </Paper>
      </Box>
    </DashboardPage>
  );
}
