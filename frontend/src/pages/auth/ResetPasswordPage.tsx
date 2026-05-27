import { useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { api, ApiError } from "@/lib/api";
import { AuthPage } from "@/components/AuthPage";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import MuiLink from "@mui/material/Link";
import TextField from "@mui/material/TextField";

export default function ResetPasswordPage() {
  const { t } = useTranslation("auth");
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const token = searchParams.get("token") ?? "";

  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [done, setDone] = useState(false);

  const visibleError = error ?? (!token ? t("errors.invalid_token") : null);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (password !== confirm) { setError(t("errors.passwords_mismatch")); return; }
    setError(null);
    setLoading(true);
    try {
      await api.resetPassword(token, password);
      setDone(true);
      setTimeout(() => navigate("/auth/login"), 2000);
    } catch (err) {
      if (err instanceof ApiError) {
        setError(t(`errors.${err.message}`, { defaultValue: t("errors.invalid_token") }));
      } else {
        setError(t("errors.internal_error"));
      }
    } finally {
      setLoading(false);
    }
  }

  return (
    <AuthPage title={t("resetTitle")}>
      {done ? (
        <Alert severity="success">{t("resetDone")}</Alert>
      ) : (
        <Box component="form" onSubmit={handleSubmit} sx={{ display: "grid", gap: 2 }}>
          <TextField
            label={t("newPassword")}
            type="password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            fullWidth
          />
          <TextField
            label={t("confirmPassword")}
            type="password"
            required
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            fullWidth
          />
          {visibleError && <Alert severity="error">{visibleError}</Alert>}
          <Button type="submit" variant="contained" disabled={loading || !token}>
            {loading ? "..." : t("resetButton")}
          </Button>
          <MuiLink component={Link} to="/auth/login" underline="hover" sx={{ justifySelf: "center" }}>
            {t("backToLogin")}
          </MuiLink>
        </Box>
      )}
    </AuthPage>
  );
}
