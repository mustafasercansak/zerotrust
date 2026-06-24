import { useState } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { api } from "@/lib/api";
import { AuthPage } from "@/components/AuthPage";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import MuiLink from "@mui/material/Link";
import TextField from "@mui/material/TextField";

export default function ForgotPasswordPage() {
  const { t } = useTranslation("auth");
  const [email, setEmail] = useState("");
  const [submitted, setSubmitted] = useState(false);
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    try {
      await api.forgotPassword(email);
    } finally {
      setLoading(false);
      setSubmitted(true);
    }
  }

  return (
    <AuthPage title={t("forgotTitle")} subtitle={t("forgotSubtitle")}>
      {submitted ? (
        <Box sx={{ display: "grid", gap: 2, textAlign: "center" }}>
          <Alert severity="success">{t("forgotSent")}</Alert>
          <MuiLink component={Link} to="/auth/login" underline="hover">
            {t("backToLogin")}
          </MuiLink>
        </Box>
      ) : (
        <Box component="form" onSubmit={handleSubmit} sx={{ display: "grid", gap: 2 }}>
          <TextField
            label={t("email")}
            type="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            fullWidth
            slotProps={{ htmlInput: { "data-testid": "forgot-email-input" } }}
          />
          <Button type="submit" variant="contained" disabled={loading}>
            {loading ? "..." : t("forgotButton")}
          </Button>
          <MuiLink component={Link} to="/auth/login" underline="hover" sx={{ justifySelf: "center" }}>
            {t("backToLogin")}
          </MuiLink>
        </Box>
      )}
    </AuthPage>
  );
}
