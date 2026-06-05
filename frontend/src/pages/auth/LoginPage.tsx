import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { api, ApiError } from "@/lib/api";
import { scheduleRefresh } from "@/lib/tokenManager";
import { performAssertion, isWebAuthnSupported } from "@/lib/webauthn";
import { AuthPage } from "@/components/AuthPage";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Divider from "@mui/material/Divider";
import MuiLink from "@mui/material/Link";
import TextField from "@mui/material/TextField";
import Typography from "@mui/material/Typography";
import Checkbox from "@mui/material/Checkbox";
import FormControlLabel from "@mui/material/FormControlLabel";
import Fingerprint from "@mui/icons-material/Fingerprint";
import { QRCodeSVG } from "qrcode.react";

type Stage = "credentials" | "mfa";

export default function LoginPage() {
  const { t } = useTranslation("auth");
  const navigate = useNavigate();

  const [stage, setStage] = useState<Stage>("credentials");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [mfaToken, setMfaToken] = useState("");
  const [mfaSetupSecret, setMfaSetupSecret] = useState("");
  const [mfaSetupURL, setMfaSetupURL] = useState("");
  const [mfaRecoveryCodes, setMfaRecoveryCodes] = useState<string[]>([]);
  const [codesSaved, setCodesSaved] = useState(false);
  const [totpCode, setTotpCode] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [retryAfter, setRetryAfter] = useState(0);
  // Which second factors the account can use (set from the login response).
  const [totpEnabled, setTotpEnabled] = useState(false);
  const [webauthnEnabled, setWebauthnEnabled] = useState(false);

  useEffect(() => {
    if (retryAfter <= 0) return;
    const timer = window.setInterval(() => setRetryAfter((seconds) => Math.max(0, seconds - 1)), 1000);
    return () => window.clearInterval(timer);
  }, [retryAfter]);

  async function handleCredentials(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    if (retryAfter > 0) return;
    setLoading(true);
    try {
      const result = await api.login(email, password);
      if (result.mfa_required && result.mfa_token) {
        setMfaToken(result.mfa_token);
        setTotpEnabled(!!result.totp_enabled);
        setWebauthnEnabled(!!result.webauthn_enabled);
        if (result.mfa_setup_url && result.mfa_setup_secret) {
          setMfaSetupURL(result.mfa_setup_url);
          setMfaSetupSecret(result.mfa_setup_secret);
          setMfaRecoveryCodes(result.mfa_recovery_codes ?? []);
          setCodesSaved(false);
        }
        setStage("mfa");
        return;
      }
      scheduleRefresh(() => navigate("/auth/login"));
      navigate("/dashboard");
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        if (err.message === "account_locked" && err.retryAfter) {
          const minutes = Math.ceil(err.retryAfter / 60);
          setError(t("errors.account_locked", { minutes }));
        } else if (err.message === "rate_limit_exceeded" && err.retryAfter) {
          setRetryAfter(err.retryAfter);
          setError(t("errors.rate_limit_exceeded_countdown", { seconds: err.retryAfter }));
        } else {
          setError(t(`errors.${err.message}`, { defaultValue: t("errors.internal_error") }));
        }
      } else {
        setError(t("errors.internal_error"));
      }
    } finally {
      setLoading(false);
    }
  }

  async function handleMFA(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    if (retryAfter > 0) return;
    setLoading(true);
    try {
      await api.mfaChallenge(mfaToken, totpCode);
      scheduleRefresh(() => navigate("/auth/login"));
      navigate("/dashboard");
    } catch (err: unknown) {
      if (err instanceof ApiError && err.message === "rate_limit_exceeded" && err.retryAfter) {
        setRetryAfter(err.retryAfter);
        setError(t("errors.rate_limit_exceeded_countdown", { seconds: err.retryAfter }));
      } else {
        setError(t("errors.invalid_credentials"));
      }
    } finally {
      setLoading(false);
    }
  }

  async function handlePasskeyLogin() {
    setError(null);
    if (retryAfter > 0) return;
    setLoading(true);
    try {
      const options = await api.webauthnLoginBegin(mfaToken);
      const assertion = await performAssertion(options);
      await api.webauthnLoginFinish(mfaToken, assertion);
      scheduleRefresh(() => navigate("/auth/login"));
      navigate("/dashboard");
    } catch (err: unknown) {
      if (err instanceof ApiError && err.message === "rate_limit_exceeded" && err.retryAfter) {
        setRetryAfter(err.retryAfter);
        setError(t("errors.rate_limit_exceeded_countdown", { seconds: err.retryAfter }));
      } else {
        setError(t("errors.webauthn_failed"));
      }
    } finally {
      setLoading(false);
    }
  }

  if (stage === "mfa") {
    const isSetup = !!mfaSetupURL;
    const showTotp = isSetup || totpEnabled;
    return (
      <AuthPage title={t("mfaTitle")} subtitle={t("mfaSubtitle")}>
        <Box component="form" onSubmit={handleMFA} sx={{ display: "grid", gap: 2 }}>
          {isSetup && (
            <Box sx={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 2, mb: 1 }}>
              <Typography variant="body2" color="warning.main" sx={{ fontWeight: 600, textAlign: "center" }}>
                {t("mfaSetupRequiredDesc")}
              </Typography>
              <Box sx={{ p: 1.5, bgcolor: "#ffffff", borderRadius: 2, border: 1, borderColor: "divider", display: "flex" }}>
                <QRCodeSVG value={mfaSetupURL} size={156} includeMargin={false} />
              </Box>
              <Box sx={{ bgcolor: "background.default", border: 1, borderColor: "divider", borderRadius: 1, p: 1.5, width: "100%" }}>
                <Typography variant="caption" color="text.secondary" sx={{ display: "block" }}>
                  {t("secretLabel")}
                </Typography>
                <Typography variant="body2" color="success.main" sx={{ fontFamily: "monospace", fontWeight: 700, overflowWrap: "anywhere" }}>
                  {mfaSetupSecret}
                </Typography>
              </Box>

              {/* Recovery Codes for Setup */}
              {mfaRecoveryCodes.length > 0 && (
                <Box sx={{ bgcolor: "background.default", border: 1, borderColor: "divider", borderRadius: 1, p: 1.5, width: "100%" }}>
                  <Typography variant="caption" color="text.secondary" sx={{ display: "block", mb: 1, fontWeight: 700 }}>
                    Backup Recovery Codes
                  </Typography>
                  <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1, fontFamily: "monospace", mb: 1.5 }}>
                    {mfaRecoveryCodes.map((c, idx) => (
                      <Box key={idx} sx={{ p: 0.5, border: 1, borderColor: "divider", borderRadius: 0.5, bgcolor: "background.paper", textAlign: "center", fontSize: "0.75rem", fontWeight: 600 }}>
                        {c}
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
                      <Typography variant="caption" sx={{ fontWeight: 600 }}>
                        I have safely stored my backup recovery codes
                      </Typography>
                    }
                  />
                </Box>
              )}
            </Box>
          )}
          {webauthnEnabled && isWebAuthnSupported() && (
            <>
              <Button
                type="button"
                variant="contained"
                startIcon={<Fingerprint />}
                onClick={handlePasskeyLogin}
                disabled={loading || retryAfter > 0}
              >
                {t("usePasskey")}
              </Button>
              {showTotp && <Divider sx={{ fontSize: "0.8rem" }}>{t("or")}</Divider>}
            </>
          )}
          {showTotp && (
            <TextField
              label={isSetup ? t("mfaCode") : "MFA Code / Recovery Code"}
              type="text"
              autoFocus
              required
              value={totpCode}
              onChange={(e) => setTotpCode(e.target.value)}
              placeholder={isSetup ? "000000" : "000000 or xxxx-xxxx-xxxx"}
              slotProps={{ htmlInput: { maxLength: 14, style: { textAlign: "center", fontFamily: "monospace" } } }}
            />
          )}
          {(error || retryAfter > 0) && (
            <Alert severity="error">
              {retryAfter > 0 ? t("errors.rate_limit_exceeded_countdown", { seconds: retryAfter }) : error}
            </Alert>
          )}
          {showTotp && (
            <Button
              type="submit"
              variant="contained"
              disabled={loading || (totpCode.length !== 6 && totpCode.length !== 14) || retryAfter > 0 || (isSetup && !codesSaved)}
            >
              {loading ? "..." : retryAfter > 0 ? t("retryButton", { seconds: retryAfter }) : t("mfaButton")}
            </Button>
          )}
          <Button
            type="button"
            onClick={() => { setStage("credentials"); setError(null); setTotpCode(""); setMfaSetupURL(""); setMfaSetupSecret(""); setMfaRecoveryCodes([]); setCodesSaved(false); }}
            color="inherit"
          >
            {t("backToLogin")}
          </Button>
        </Box>
      </AuthPage>
    );
  }

  return (
    <AuthPage title={t("loginTitle")}>
      <Box component="form" onSubmit={handleCredentials} sx={{ display: "grid", gap: 2 }}>
        <TextField
          label={t("email")}
          type="email"
          required
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          fullWidth
        />
        <TextField
          label={t("password")}
          type="password"
          required
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          fullWidth
          helperText={
            <MuiLink component={Link} to="/auth/forgot-password" underline="hover" variant="caption">
              {t("forgotLink")}
            </MuiLink>
          }
        />
        {(error || retryAfter > 0) && (
          <Alert severity="error">
            {retryAfter > 0 ? t("errors.rate_limit_exceeded_countdown", { seconds: retryAfter }) : error}
          </Alert>
        )}
        <Button type="submit" variant="contained" disabled={loading || retryAfter > 0}>
          {loading ? "..." : retryAfter > 0 ? t("retryButton", { seconds: retryAfter }) : t("loginButton")}
        </Button>
      </Box>
    </AuthPage>
  );
}
