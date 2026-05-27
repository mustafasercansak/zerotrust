import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, ApiError } from "@/lib/api";
import { useMeContext } from "@/contexts/MeContext";
import { DashboardPage } from "@/components/DashboardPage";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import CircularProgress from "@mui/material/CircularProgress";
import Divider from "@mui/material/Divider";
import Paper from "@mui/material/Paper";
import TextField from "@mui/material/TextField";
import Typography from "@mui/material/Typography";

export default function SettingsPage() {
  const { t } = useTranslation("settings");
  const me = useMeContext();
  const isAdmin = me?.roles.includes("admin") ?? false;

  const [maxSessions, setMaxSessions] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState<string | null>(null);

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
    if (isNaN(n) || n < 1 || n > 20) { setError("invalid_value"); return; }
    setSaving(true);
    try {
      await api.admin.updateSettings({ max_sessions_per_user: String(n) });
      setSaved(true);
      setTimeout(() => setSaved(false), 3000);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "internal_error");
    } finally {
      setSaving(false);
    }
  }

  return (
    <DashboardPage accessDenied={!isAdmin} accessDeniedMessage={t("accessDenied")}>
      <Box sx={{ width: "100%" }}>
        <Paper
          component="form"
          onSubmit={handleSave}
          variant="outlined"
          sx={{
            display: "grid",
            overflow: "hidden",
          }}
        >
          <Box sx={{ px: 3, py: 2.75 }}>
            <Typography variant="h6" sx={{ fontWeight: 700 }}>
              {t("title")}
            </Typography>
          </Box>

          <Divider />

          {loading ? (
            <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, px: 3, py: 3 }}>
              <CircularProgress size={18} />
              <Typography variant="body2" color="text.secondary">{t("loading")}</Typography>
            </Box>
          ) : (
            <Box
              sx={{
                alignItems: { xs: "stretch", md: "center" },
                display: "grid",
                gap: { xs: 2, md: 3 },
                gridTemplateColumns: { xs: "1fr", md: "minmax(0, 1fr) auto" },
                px: 3,
                py: 2.75,
              }}
            >
              <Box>
                <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>
                  {t("maxSessions")}
                </Typography>
                <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                  {t("maxSessionsDesc")}
                </Typography>
              </Box>
              <Box
                sx={{
                  alignItems: "center",
                  display: "flex",
                  gap: 1.5,
                  justifyContent: { xs: "stretch", md: "flex-end" },
                }}
              >
                <TextField
                  label={t("maxSessionsInput")}
                  type="number"
                  value={maxSessions}
                  onChange={(e) => setMaxSessions(e.target.value)}
                  sx={{ width: { xs: "100%", sm: 160 } }}
                  slotProps={{ htmlInput: { min: 1, max: 20 } }}
                />
                <Button
                  type="submit"
                  variant="contained"
                  disabled={saving || loading}
                  sx={{ minWidth: 132, whiteSpace: "nowrap" }}
                >
                  {saving ? t("saving") : t("save")}
                </Button>
              </Box>
            </Box>
          )}

          {(error || saved) && (
            <Box sx={{ px: 3, pb: 2 }}>
              {error && <Alert severity="error">{t(`errors.${error}`, { defaultValue: t("errors.internal_error") })}</Alert>}
              {saved && <Alert severity="success">{t("saved")}</Alert>}
            </Box>
          )}

        </Paper>
      </Box>
    </DashboardPage>
  );
}
