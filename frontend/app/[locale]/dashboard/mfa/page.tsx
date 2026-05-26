"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import CircularProgress from "@mui/material/CircularProgress";
import Link from "@mui/material/Link";
import Paper from "@mui/material/Paper";
import TextField from "@mui/material/TextField";
import Typography from "@mui/material/Typography";

type Status = "loading" | "disabled" | "pending" | "enabled";

export default function MFAPage() {
  const t = useTranslations("mfa");

  const [status, setStatus] = useState<Status>("loading");
  const [setupData, setSetupData] = useState<{ otp_auth_url: string; secret: string } | null>(null);
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    api.mfaStatus().then((d) => setStatus(d.enabled ? "enabled" : "disabled")).catch(() => setStatus("disabled"));
  }, []);

  async function handleSetup() {
    setError("");
    setSubmitting(true);
    try {
      const data = await api.mfaSetup();
      setSetupData(data);
      setStatus("pending");
    } catch {
      setError(t("errors.internal_error"));
    } finally {
      setSubmitting(false);
    }
  }

  async function handleVerify(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      await api.mfaVerify(code);
      setStatus("enabled");
      setSetupData(null);
      setCode("");
    } catch {
      setError(t("errors.invalid_code"));
    } finally {
      setSubmitting(false);
    }
  }

  async function handleDisable(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      await api.mfaDisable(code);
      setStatus("disabled");
      setCode("");
    } catch {
      setError(t("errors.invalid_code"));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Box sx={{ height: "100%", maxWidth: 560, overflow: "auto", p: 4 }}>
      {status === "loading" && <CircularProgress size={20} />}

      {status === "enabled" && (
        <Paper variant="outlined" sx={{ display: "grid", gap: 2, p: 3 }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <Chip size="small" color="success" label={t("statusEnabled")} />
            <Typography variant="body2" color="text.secondary">{t("enabledDesc")}</Typography>
          </Box>
          <Box component="form" onSubmit={handleDisable} sx={{ display: "grid", gap: 1.5, justifyItems: "start" }}>
            <Typography variant="body2" color="text.secondary">{t("disablePrompt")}</Typography>
            <TextField
              type="text"
              inputMode="numeric"
              value={code}
              onChange={(e) => setCode(e.target.value)}
              placeholder="000000"
              sx={{ width: 160 }}
              slotProps={{ htmlInput: { maxLength: 6, style: { textAlign: "center", fontFamily: "monospace" } } }}
            />
            {error && <Alert severity="error">{error}</Alert>}
            <Button
              type="submit"
              color="error"
              variant="contained"
              disabled={submitting || code.length !== 6}
            >
              {t("disableButton")}
            </Button>
          </Box>
        </Paper>
      )}

      {status === "disabled" && (
        <Paper variant="outlined" sx={{ display: "grid", gap: 2, justifyItems: "start", p: 3 }}>
          <Chip size="small" variant="outlined" label={t("statusDisabled")} />
          <Typography variant="body2" color="text.secondary">{t("disabledDesc")}</Typography>
          {error && <Alert severity="error">{error}</Alert>}
          <Button
            onClick={handleSetup}
            variant="contained"
            disabled={submitting}
          >
            {t("setupButton")}
          </Button>
        </Paper>
      )}

      {status === "pending" && setupData && (
        <Paper variant="outlined" sx={{ display: "grid", gap: 2.5, p: 3 }}>
          <Typography variant="body2" color="text.secondary">{t("setupInstructions")}</Typography>

          <Box sx={{ bgcolor: "background.default", border: 1, borderColor: "divider", borderRadius: 1, p: 2 }}>
            <Typography variant="overline" color="text.secondary">{t("secretLabel")}</Typography>
            <Typography variant="body2" color="success.main" sx={{ fontFamily: "monospace", overflowWrap: "anywhere" }}>
              {setupData.secret}
            </Typography>
            <Link
              href={setupData.otp_auth_url}
              sx={{ display: "block", mt: 1, overflowWrap: "anywhere" }}
            >
              {t("openInApp")}
            </Link>
          </Box>

          <Box component="form" onSubmit={handleVerify} sx={{ display: "grid", gap: 1.5, justifyItems: "start" }}>
            <Typography variant="body2" color="text.secondary">{t("verifyPrompt")}</Typography>
            <TextField
              type="text"
              inputMode="numeric"
              value={code}
              onChange={(e) => setCode(e.target.value)}
              placeholder="000000"
              autoFocus
              sx={{ width: 160 }}
              slotProps={{ htmlInput: { maxLength: 6, style: { textAlign: "center", fontFamily: "monospace" } } }}
            />
            {error && <Alert severity="error">{error}</Alert>}
            <Button
              type="submit"
              variant="contained"
              disabled={submitting || code.length !== 6}
            >
              {submitting ? "..." : t("verifyButton")}
            </Button>
          </Box>
        </Paper>
      )}
    </Box>
  );
}
