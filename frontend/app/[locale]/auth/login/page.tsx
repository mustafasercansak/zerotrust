"use client";

import { useState } from "react";
import { useTranslations, useLocale } from "next-intl";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { api, ApiError } from "@/lib/api";
import { scheduleRefresh } from "@/lib/tokenManager";
import { AuthPage } from "@/components/AuthPage";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import MuiLink from "@mui/material/Link";
import TextField from "@mui/material/TextField";

type Stage = "credentials" | "mfa";

export default function LoginPage() {
  const t = useTranslations("auth");
  const locale = useLocale();
  const router = useRouter();

  const [stage, setStage] = useState<Stage>("credentials");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [mfaToken, setMfaToken] = useState("");
  const [totpCode, setTotpCode] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function handleCredentials(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setLoading(true);
    try {
      const result = await api.login(email, password);
      if (result.mfa_required && result.mfa_token) {
        setMfaToken(result.mfa_token);
        setStage("mfa");
        return;
      }
      scheduleRefresh(() => router.push(`/${locale}/auth/login`));
      router.push(`/${locale}/dashboard`);
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        if (err.message === "account_locked" && err.retryAfter) {
          const minutes = Math.ceil(err.retryAfter / 60);
          setError(t("errors.account_locked", { minutes }));
        } else {
          const key = `errors.${err.message}` as Parameters<typeof t>[0];
          setError(t(key, { defaultValue: t("errors.internal_error") }));
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
    setLoading(true);
    try {
      await api.mfaChallenge(mfaToken, totpCode);
      scheduleRefresh(() => router.push(`/${locale}/auth/login`));
      router.push(`/${locale}/dashboard`);
    } catch {
      setError(t("errors.invalid_credentials"));
    } finally {
      setLoading(false);
    }
  }

  if (stage === "mfa") {
    return (
      <AuthPage title={t("mfaTitle")} subtitle={t("mfaSubtitle")}>
          <Box component="form" onSubmit={handleMFA} sx={{ display: "grid", gap: 2 }}>
              <TextField
                label={t("mfaCode")}
                type="text"
                inputMode="numeric"
                autoFocus
                required
                value={totpCode}
                onChange={(e) => setTotpCode(e.target.value)}
                placeholder="000000"
                slotProps={{ htmlInput: { maxLength: 6, style: { textAlign: "center", fontFamily: "monospace" } } }}
              />

            {error && (
              <Alert severity="error">
                {error}
              </Alert>
            )}

            <Button
              type="submit"
              variant="contained"
              disabled={loading || totpCode.length !== 6}
            >
              {loading ? "..." : t("mfaButton")}
            </Button>

            <Button
              type="button"
              onClick={() => { setStage("credentials"); setError(null); setTotpCode(""); }}
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
              helperText={(
                <MuiLink
                  component={Link}
                  href={`/${locale}/auth/forgot-password`}
                  underline="hover"
                  variant="caption"
                >
                  {t("forgotLink")}
                </MuiLink>
              )}
            />

          {error && (
            <Alert severity="error">
              {error}
            </Alert>
          )}

          <Button
            type="submit"
            variant="contained"
            disabled={loading}
          >
            {loading ? "..." : t("loginButton")}
          </Button>
        </Box>
    </AuthPage>
  );
}
