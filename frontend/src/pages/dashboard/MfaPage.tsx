import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import PasskeysSection from "./PasskeysSection";
import { api } from "@/lib/api";
import { DashboardPage } from "@/components/DashboardPage";
import { QRCodeSVG } from "qrcode.react";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import CircularProgress from "@mui/material/CircularProgress";
import Divider from "@mui/material/Divider";
import Paper from "@mui/material/Paper";
import TextField from "@mui/material/TextField";
import Typography from "@mui/material/Typography";
import Checkbox from "@mui/material/Checkbox";
import FormControlLabel from "@mui/material/FormControlLabel";

type Status = "loading" | "disabled" | "pending" | "enabled" | "unsupported";

export default function MfaPage() {
  const { t } = useTranslation("mfa");

  const [status, setStatus] = useState<Status>("loading");
  const [setupData, setSetupData] = useState<{ otp_auth_url: string; secret: string; recovery_codes: string[] } | null>(null);
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [codesSaved, setCodesSaved] = useState(false);

  useEffect(() => {
    api.mfaStatus()
      .then((d) => {
        if (d.supported === false) {
          setStatus("unsupported");
        } else {
          setStatus(d.enabled ? "enabled" : "disabled");
        }
      })
      .catch(() => setStatus("unsupported"));
  }, []);

  async function handleSetup() {
    setError(""); setSubmitting(true); setCodesSaved(false);
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
    <TextField
      type="text"
      inputMode="numeric"
      value={code}
      onChange={(e) => setCode(e.target.value)}
      placeholder="000000"
      autoFocus
      sx={{
        width: 140,
        "& .MuiOutlinedInput-root": {
          bgcolor: "#030712",
          borderRadius: "8px",
          borderColor: "rgba(255, 255, 255, 0.08)",
          "& fieldset": {
            borderColor: "rgba(255, 255, 255, 0.08)",
          },
          "&:hover fieldset": {
            borderColor: "rgba(99, 102, 241, 0.5)",
          },
          "&.Mui-focused fieldset": {
            borderColor: "#6366f1",
          },
        },
      }}
      slotProps={{ htmlInput: { maxLength: 6, style: { textAlign: "center", fontFamily: "monospace", fontWeight: 700 } } }}
    />
  );

  return (
    <DashboardPage>
      {/* Import outfit font to match the mockup's premium geometric typography */}
      <link href="https://fonts.googleapis.com/css2?family=Outfit:wght@400;600;700;800&display=swap" rel="stylesheet" />

      {status === "loading" && (
        <Paper variant="outlined" sx={{ display: "flex", alignItems: "center", gap: 1.5, px: 3, py: 3, width: "100%", bgcolor: "#0b1120" }}>
          <CircularProgress size={18} />
          <Typography variant="body2" color="text.secondary">{t("loading")}</Typography>
        </Paper>
      )}

      {status === "enabled" && (
        <Paper variant="outlined" sx={{ display: "grid", gap: 2.5, px: 3, py: 2.75, width: "100%", bgcolor: "#0b1120", borderColor: "divider" }}>
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
        </Paper>
      )}

      {status === "disabled" && (
        <Paper variant="outlined" sx={{ alignItems: { xs: "stretch", md: "center" }, display: "grid", gap: 2, gridTemplateColumns: { xs: "1fr", md: "minmax(0, 1fr) auto" }, px: 3, py: 2.75, width: "100%", bgcolor: "#0b1120", borderColor: "divider" }}>
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
        </Paper>
      )}

      {status === "unsupported" && (
        <Paper variant="outlined" sx={{ px: 3, py: 2.75, width: "100%", bgcolor: "#0b1120", borderColor: "divider" }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 0.75 }}>
            <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>{t("sectionTitle")}</Typography>
            <Chip size="small" variant="outlined" color="warning" label={t("statusDisabled")} />
          </Box>
          <Typography variant="body2" color="text.secondary">{t("unsupportedDesc")}</Typography>
        </Paper>
      )}

      {status === "pending" && setupData && (
        <Box sx={{ display: "flex", flexDirection: "column", gap: 3, width: "100%", height: "100%", fontFamily: "'Outfit', sans-serif" }}>
          <Typography variant="h5" sx={{ fontWeight: 800, letterSpacing: "-0.02em", color: "#ffffff", fontFamily: "'Outfit', sans-serif" }}>
            Two-Factor Authentication Setup
          </Typography>

          <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "1.1fr 1.2fr" }, gap: 4, flexGrow: 1, height: "100%" }}>
            {/* Left Column: Scan QR Code */}
            <Paper
              variant="outlined"
              sx={{
                p: 4,
                display: "flex",
                flexDirection: "column",
                alignItems: "center",
                justifyContent: "center",
                borderColor: "#4f46e5",
                borderWidth: 1.5,
                bgcolor: "#0b1120",
                borderRadius: 4,
                boxShadow: "0 8px 32px 0 rgba(79, 70, 229, 0.15)",
                height: "100%",
              }}
            >
              <Typography variant="subtitle1" sx={{ fontWeight: 700, mb: 0.5, color: "#ffffff", fontFamily: "'Outfit', sans-serif" }}>
                Scan QR Code
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 3.5, fontFamily: "'Outfit', sans-serif" }}>
                Use Authenticator App
              </Typography>
              <Box sx={{ display: "flex", alignItems: "center", justifyContent: "center", bgcolor: "#ffffff", p: 2.5, borderRadius: 4, width: 220, height: 220, mb: 3.5, boxShadow: "0 4px 12px rgba(0,0,0,0.15)" }}>
                <QRCodeSVG
                  value={setupData.otp_auth_url}
                  size={180}
                  includeMargin={false}
                  imageSettings={{
                    src: "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCIgZmlsbD0iIzYzNjZmMSI+PHBhdGggZD0iTTEyIDJMMyA1djZjMCA1LjUgMy44IDEwLjcgOSAxMiA1LjItMS4zIDktNi41IDktMTJWNWwtOS0zeiIvPjwvc3ZnPg==",
                    height: 36,
                    width: 36,
                    excavate: true,
                  }}
                />
              </Box>
              <Typography variant="caption" color="text.secondary" sx={{ textAlign: "center", mb: 3.5, maxWidth: 280, lineHeight: 1.4, fontFamily: "'Outfit', sans-serif" }}>
                Scan this code with Google Authenticator, Authy, etc.
              </Typography>

              <Box sx={{ bgcolor: "#030712", border: 1, borderColor: "rgba(255, 255, 255, 0.08)", borderRadius: 2, p: 2, width: "100%", textAlign: "center" }}>
                <Typography variant="caption" color="text.secondary" sx={{ display: "block", mb: 0.5, fontWeight: 600, fontFamily: "'Outfit', sans-serif" }}>
                  Setup Key
                </Typography>
                <Typography variant="body2" color="success.main" sx={{ fontFamily: "monospace", fontWeight: 700, letterSpacing: "0.08em", fontSize: "0.95rem" }}>
                  {setupData.secret}
                </Typography>
              </Box>
            </Paper>

            {/* Right Column: Backup Recovery Codes */}
            <Paper
              variant="outlined"
              sx={{
                p: 4,
                display: "flex",
                flexDirection: "column",
                borderColor: "rgba(255, 255, 255, 0.08)",
                bgcolor: "#0b1120",
                borderRadius: 4,
                height: "100%",
              }}
            >
              <Typography variant="subtitle1" sx={{ fontWeight: 700, mb: 0.5, color: "#ffffff", fontFamily: "'Outfit', sans-serif" }}>
                Backup Recovery Codes
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 3.5, fontFamily: "'Outfit', sans-serif" }}>
                Store these single-use codes securely. They will not be shown again.
              </Typography>

              <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 2, mb: 3.5 }}>
                {setupData.recovery_codes.map((c, idx) => (
                  <Box
                    key={idx}
                    sx={{
                      p: 1.5,
                      border: 1,
                      borderColor: "rgba(99, 102, 241, 0.35)",
                      borderRadius: 2.5,
                      bgcolor: "#030712",
                      position: "relative",
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "center",
                      minHeight: 52,
                    }}
                  >
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      sx={{
                        position: "absolute",
                        left: 10,
                        top: 4,
                        fontSize: "0.65rem",
                        fontWeight: 700,
                      }}
                    >
                      {idx + 1}
                    </Typography>
                    <Typography
                      variant="body2"
                      sx={{
                        fontFamily: "monospace",
                        fontWeight: 700,
                        color: "text.primary",
                        fontSize: "0.85rem",
                        letterSpacing: "0.02em",
                      }}
                    >
                      {c}
                    </Typography>
                  </Box>
                ))}
              </Box>

              <FormControlLabel
                control={
                  <Checkbox
                    checked={codesSaved}
                    onChange={(e) => setCodesSaved(e.target.checked)}
                    color="primary"
                    size="small"
                  />
                }
                label={
                  <Typography variant="body2" sx={{ fontWeight: 600, color: "text.primary", fontFamily: "'Outfit', sans-serif" }}>
                    I have safely stored my backup recovery codes
                  </Typography>
                }
                sx={{ mb: 3.5 }}
              />

              <Box component="form" onSubmit={handleVerify} sx={{ display: "flex", flexDirection: "column", gap: 1.5, mt: "auto" }}>
                <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 600, fontFamily: "'Outfit', sans-serif" }}>
                  Verify Code
                </Typography>
                <Box sx={{ display: "flex", gap: 2 }}>
                  {codeField}
                  <Button
                    type="submit"
                    variant="contained"
                    disabled={submitting || code.length !== 6 || !codesSaved}
                    sx={{
                      background: "linear-gradient(135deg, #4f46e5 0%, #6366f1 100%)",
                      boxShadow: "0 4px 14px 0 rgba(79, 70, 229, 0.4)",
                      "&:hover": {
                        background: "linear-gradient(135deg, #4338ca 0%, #4f46e5 100%)",
                      },
                      flexGrow: 1,
                      borderRadius: 2,
                      fontWeight: 700,
                      fontSize: "0.95rem",
                    }}
                  >
                    {submitting ? "..." : "Continue"}
                  </Button>
                </Box>
              </Box>
            </Paper>
          </Box>
          {error && <Alert severity="error" sx={{ mt: 2 }}>{error}</Alert>}
        </Box>
      )}

      {status !== "loading" && (
        <Box sx={{ mt: 3, width: "100%" }}>
          <PasskeysSection />
        </Box>
      )}
    </DashboardPage>
  );
}
