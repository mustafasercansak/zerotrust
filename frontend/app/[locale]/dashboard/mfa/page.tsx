"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";

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
    <div className="px-8 py-8 max-w-lg space-y-6">
      <h1 className="text-2xl font-bold text-white">{t("title")}</h1>

      {status === "loading" && <p className="text-gray-400 text-sm">...</p>}

      {status === "enabled" && (
        <div className="space-y-4">
          <div className="flex items-center gap-2">
            <span className="text-xs px-2 py-0.5 rounded-full bg-emerald-900/50 text-emerald-300 border border-emerald-700">
              {t("statusEnabled")}
            </span>
            <span className="text-sm text-gray-400">{t("enabledDesc")}</span>
          </div>
          <form onSubmit={handleDisable} className="space-y-3">
            <p className="text-sm text-gray-400">{t("disablePrompt")}</p>
            <input
              type="text"
              inputMode="numeric"
              maxLength={6}
              value={code}
              onChange={(e) => setCode(e.target.value)}
              placeholder="000000"
              className="w-40 rounded-lg bg-gray-800 border border-gray-700 text-white px-4 py-2 font-mono text-center focus:outline-none focus:ring-2 focus:ring-indigo-500"
            />
            {error && <p className="text-red-400 text-xs">{error}</p>}
            <button
              type="submit"
              disabled={submitting || code.length !== 6}
              className="px-4 py-2 bg-red-700 hover:bg-red-600 disabled:opacity-50 text-white text-sm rounded-lg transition-colors"
            >
              {t("disableButton")}
            </button>
          </form>
        </div>
      )}

      {status === "disabled" && (
        <div className="space-y-4">
          <div className="flex items-center gap-2">
            <span className="text-xs px-2 py-0.5 rounded-full bg-gray-800 text-gray-400 border border-gray-700">
              {t("statusDisabled")}
            </span>
          </div>
          <p className="text-sm text-gray-400">{t("disabledDesc")}</p>
          {error && <p className="text-red-400 text-xs">{error}</p>}
          <button
            onClick={handleSetup}
            disabled={submitting}
            className="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-sm rounded-lg transition-colors"
          >
            {t("setupButton")}
          </button>
        </div>
      )}

      {status === "pending" && setupData && (
        <div className="space-y-5">
          <p className="text-sm text-gray-400">{t("setupInstructions")}</p>

          <div className="rounded-xl border border-gray-700 p-4 space-y-3 bg-gray-900">
            <p className="text-xs text-gray-400 uppercase tracking-wider">{t("secretLabel")}</p>
            <p className="font-mono text-sm text-emerald-300 break-all select-all">{setupData.secret}</p>
            <a
              href={setupData.otp_auth_url}
              className="block text-xs text-indigo-400 hover:text-indigo-300 break-all"
            >
              {t("openInApp")}
            </a>
          </div>

          <form onSubmit={handleVerify} className="space-y-3">
            <p className="text-sm text-gray-400">{t("verifyPrompt")}</p>
            <input
              type="text"
              inputMode="numeric"
              maxLength={6}
              value={code}
              onChange={(e) => setCode(e.target.value)}
              placeholder="000000"
              autoFocus
              className="w-40 rounded-lg bg-gray-800 border border-gray-700 text-white px-4 py-2 font-mono text-center focus:outline-none focus:ring-2 focus:ring-indigo-500"
            />
            {error && <p className="text-red-400 text-xs">{error}</p>}
            <button
              type="submit"
              disabled={submitting || code.length !== 6}
              className="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-sm rounded-lg transition-colors"
            >
              {submitting ? "..." : t("verifyButton")}
            </button>
          </form>
        </div>
      )}
    </div>
  );
}
