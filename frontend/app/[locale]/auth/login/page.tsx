"use client";

import { useState } from "react";
import { useTranslations, useLocale } from "next-intl";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { api, ApiError } from "@/lib/api";
import { scheduleRefresh } from "@/lib/tokenManager";

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
      <main className="min-h-screen flex items-center justify-center bg-gray-950 px-4">
        <div className="w-full max-w-sm space-y-6">
          <div className="text-center">
            <h1 className="text-2xl font-bold text-white">{t("mfaTitle")}</h1>
            <p className="text-sm text-gray-400 mt-1">{t("mfaSubtitle")}</p>
          </div>

          <form onSubmit={handleMFA} className="space-y-4">
            <div>
              <label className="block text-sm text-gray-400 mb-1">{t("mfaCode")}</label>
              <input
                type="text"
                inputMode="numeric"
                maxLength={6}
                autoFocus
                required
                value={totpCode}
                onChange={(e) => setTotpCode(e.target.value)}
                placeholder="000000"
                className="w-full rounded-lg bg-gray-800 border border-gray-700 text-white px-4 py-2.5 font-mono text-center tracking-widest focus:outline-none focus:ring-2 focus:ring-indigo-500"
              />
            </div>

            {error && (
              <p className="text-sm text-red-400 bg-red-950/50 border border-red-800 rounded-lg px-4 py-2">
                {error}
              </p>
            )}

            <button
              type="submit"
              disabled={loading || totpCode.length !== 6}
              className="w-full bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white font-medium py-2.5 rounded-lg transition-colors"
            >
              {loading ? "..." : t("mfaButton")}
            </button>

            <button
              type="button"
              onClick={() => { setStage("credentials"); setError(null); setTotpCode(""); }}
              className="w-full text-sm text-gray-400 hover:text-white transition-colors"
            >
              {t("backToLogin")}
            </button>
          </form>
        </div>
      </main>
    );
  }

  return (
    <main className="min-h-screen flex items-center justify-center bg-gray-950 px-4">
      <div className="w-full max-w-sm space-y-6">
        <div className="text-center">
          <h1 className="text-2xl font-bold text-white">{t("loginTitle")}</h1>
        </div>

        <form onSubmit={handleCredentials} className="space-y-4">
          <div>
            <label className="block text-sm text-gray-400 mb-1">{t("email")}</label>
            <input
              type="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="w-full rounded-lg bg-gray-800 border border-gray-700 text-white px-4 py-2.5 focus:outline-none focus:ring-2 focus:ring-indigo-500"
            />
          </div>

          <div>
            <div className="flex items-center justify-between mb-1">
              <label className="block text-sm text-gray-400">{t("password")}</label>
              <Link
                href={`/${locale}/auth/forgot-password`}
                className="text-xs text-indigo-400 hover:text-indigo-300 transition-colors"
              >
                {t("forgotLink")}
              </Link>
            </div>
            <input
              type="password"
              required
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full rounded-lg bg-gray-800 border border-gray-700 text-white px-4 py-2.5 focus:outline-none focus:ring-2 focus:ring-indigo-500"
            />
          </div>

          {error && (
            <p className="text-sm text-red-400 bg-red-950/50 border border-red-800 rounded-lg px-4 py-2">
              {error}
            </p>
          )}

          <button
            type="submit"
            disabled={loading}
            className="w-full bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white font-medium py-2.5 rounded-lg transition-colors"
          >
            {loading ? "..." : t("loginButton")}
          </button>
        </form>
      </div>
    </main>
  );
}
