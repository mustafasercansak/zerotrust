import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api } from "@/lib/api";
import { DashboardPage } from "@/components/DashboardPage";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import CircularProgress from "@mui/material/CircularProgress";
import Divider from "@mui/material/Divider";
import Link from "@mui/material/Link";
import Paper from "@mui/material/Paper";
import TextField from "@mui/material/TextField";
import Typography from "@mui/material/Typography";

type Status = "loading" | "disabled" | "pending" | "enabled";

export default function MfaPage() {
  const { t } = useTranslation("mfa");

  const [status, setStatus] = useState<Status>("loading");
  const [setupData, setSetupData] = useState<{ otp_auth_url: string; secret: string } | null>(null);
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    api.mfaStatus().then((d) => setStatus(d.enabled ? "enabled" : "disabled")).catch(() => setStatus("disabled"));
  }, []);

  async function handleSetup() {
    setError(""); setSubmitting(true);
    try { const d = await api.mfaSetup(); setSetupData(d); setStatus("pending"); }
    catch { setError(t("errors.internal_error")); }
    finally { setSubmitting(false); }
  }

  async function handleVerify(e: React.FormEvent) {
    e.preventDefault(); setError(""); setSubmitting(true);
    try { await api.mfaVerify(code); setStatus("enabled"); setSetupData(null); setCode(""); }
    catch { setError(t("errors.invalid_code")); }
    finally { setSubmitting(false); }
  }

  async function handleDisable(e: React.FormEvent) {
    e.preventDefault(); setError(""); setSubmitting(true);
    try { await api.mfaDisable(code); setStatus("disabled"); setCode(""); }
    catch { setError(t("errors.invalid_code")); }
    finally { setSubmitting(false); }
  }

  const codeField = (
    <TextField type="text" inputMode="numeric" value={code} onChange={(e) => setCode(e.target.value)}
      placeholder="000000" autoFocus sx={{ width: 160 }}
      slotProps={{ htmlInput: { maxLength: 6, style: { textAlign: "center", fontFamily: "monospace" } } }} />
  );

  return (
    <DashboardPage>
      <Paper variant="outlined" sx={{ display: "grid", overflow: "hidden", width: "100%" }}>
        <Box sx={{ px: 3, py: 2.75 }}>
          <Typography variant="h6" sx={{ fontWeight: 700 }}>
            {t("accountSecurityTitle")}
          </Typography>
        </Box>

        <Divider />

        {status === "loading" && (
          <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, px: 3, py: 3 }}>
            <CircularProgress size={18} />
            <Typography variant="body2" color="text.secondary">{t("loading")}</Typography>
          </Box>
        )}

        {status === "enabled" && (
          <Box sx={{ display: "grid", gap: 2.5, px: 3, py: 2.75 }}>
            <Box sx={{ alignItems: { xs: "stretch", md: "center" }, display: "grid", gap: 2, gridTemplateColumns: { xs: "1fr", md: "minmax(0, 1fr) auto" } }}>
              <Box>
                <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 0.75 }}>
                  <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>{t("sectionTitle")}</Typography>
                  <Chip size="small" color="success" label={t("statusEnabled")} />
                </Box>
                <Typography variant="body2" color="text.secondary">{t("enabledDesc")}</Typography>
              </Box>
            </Box>
            <Divider />
            <Box component="form" onSubmit={handleDisable} sx={{ alignItems: { xs: "stretch", sm: "center" }, display: "flex", flexWrap: "wrap", gap: 1.5 }}>
              <Typography variant="body2" color="text.secondary" sx={{ flex: "1 1 280px" }}>{t("disablePrompt")}</Typography>
              {codeField}
              <Button type="submit" color="error" variant="contained" disabled={submitting || code.length !== 6}>
                {t("disableButton")}
              </Button>
            </Box>
            {error && <Alert severity="error">{error}</Alert>}
          </Box>
        )}

        {status === "disabled" && (
          <Box sx={{ alignItems: { xs: "stretch", md: "center" }, display: "grid", gap: 2, gridTemplateColumns: { xs: "1fr", md: "minmax(0, 1fr) auto" }, px: 3, py: 2.75 }}>
            <Box>
              <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 0.75 }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>{t("sectionTitle")}</Typography>
                <Chip size="small" variant="outlined" label={t("statusDisabled")} />
              </Box>
              <Typography variant="body2" color="text.secondary">{t("disabledDesc")}</Typography>
              {error && <Alert severity="error" sx={{ mt: 2 }}>{error}</Alert>}
            </Box>
            <Button onClick={handleSetup} variant="contained" disabled={submitting} sx={{ justifySelf: { md: "end" }, minWidth: 160 }}>
              {t("setupButton")}
            </Button>
          </Box>
        )}

        {status === "pending" && setupData && (
          <Box sx={{ display: "grid", gap: 2.5, px: 3, py: 2.75 }}>
            <Box>
              <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 0.75 }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>{t("sectionTitle")}</Typography>
                <Chip size="small" color="warning" label={t("statusPending")} />
              </Box>
              <Typography variant="body2" color="text.secondary">{t("setupInstructions")}</Typography>
            </Box>
            <Box sx={{ bgcolor: "background.default", border: 1, borderColor: "divider", borderRadius: 1, p: 2 }}>
              <Typography variant="overline" color="text.secondary">{t("secretLabel")}</Typography>
              <Typography variant="body2" color="success.main" sx={{ fontFamily: "monospace", overflowWrap: "anywhere" }}>
                {setupData.secret}
              </Typography>
              <Link href={setupData.otp_auth_url} sx={{ display: "block", mt: 1, overflowWrap: "anywhere" }}>
                {t("openInApp")}
              </Link>
            </Box>
            <Box component="form" onSubmit={handleVerify} sx={{ alignItems: { xs: "stretch", sm: "center" }, display: "flex", flexWrap: "wrap", gap: 1.5 }}>
              <Typography variant="body2" color="text.secondary" sx={{ flex: "1 1 320px" }}>{t("verifyPrompt")}</Typography>
              {codeField}
              <Button type="submit" variant="contained" disabled={submitting || code.length !== 6}>
                {submitting ? "..." : t("verifyButton")}
              </Button>
            </Box>
            {error && <Alert severity="error">{error}</Alert>}
          </Box>
        )}
      </Paper>
    </DashboardPage>
  );
}
