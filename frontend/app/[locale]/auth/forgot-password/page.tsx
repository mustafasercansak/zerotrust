"use client";

import { useState } from "react";
import { useTranslations, useLocale } from "next-intl";
import Link from "next/link";
import { api } from "@/lib/api";

export default function ForgotPasswordPage() {
  const t = useTranslations("auth");
  const locale = useLocale();
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
    <main className="min-h-screen flex items-center justify-center bg-gray-950 px-4">
      <div className="w-full max-w-sm space-y-6">
        <div className="text-center">
          <h1 className="text-2xl font-bold text-white">{t("forgotTitle")}</h1>
          <p className="text-sm text-gray-400 mt-1">{t("forgotSubtitle")}</p>
        </div>

        {submitted ? (
          <div className="text-center space-y-4">
            <p className="text-sm text-emerald-400 bg-emerald-950/50 border border-emerald-800 rounded-lg px-4 py-3">
              {t("forgotSent")}
            </p>
            <Link
              href={`/${locale}/auth/login`}
              className="block text-sm text-indigo-400 hover:text-indigo-300 transition-colors"
            >
              {t("backToLogin")}
            </Link>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="space-y-4">
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

            <button
              type="submit"
              disabled={loading}
              className="w-full bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white font-medium py-2.5 rounded-lg transition-colors"
            >
              {loading ? "..." : t("forgotButton")}
            </button>

            <div className="text-center">
              <Link
                href={`/${locale}/auth/login`}
                className="text-sm text-gray-400 hover:text-white transition-colors"
              >
                {t("backToLogin")}
              </Link>
            </div>
          </form>
        )}
      </div>
    </main>
  );
}
