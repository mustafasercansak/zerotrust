"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { api, ServiceAccount, ServiceAccountCreated } from "@/lib/api";
import { useMeContext } from "../context";

const ALL_SCOPES = [
  "users:read",
  "users:write",
  "service_accounts:read",
  "service_accounts:write",
  "service_accounts:delete",
];

function expiryLabel(expiresAt: string | null, tNever: string, tExpired: string): {
  text: string;
  expired: boolean;
} {
  if (!expiresAt) return { text: tNever, expired: false };
  const expired = new Date(expiresAt) < new Date();
  return { text: expiresAt.slice(0, 10), expired };
}

export default function ServiceAccountsPage() {
  const t = useTranslations("serviceAccounts");
  const tCommon = useTranslations("common");
  const me = useMeContext();
  const isAdmin = me?.roles.includes("admin") ?? false;

  const [accounts, setAccounts] = useState<ServiceAccount[]>([]);
  const [loadError, setLoadError] = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const [newSecret, setNewSecret] = useState<ServiceAccountCreated | null>(null);

  const [name, setName] = useState("");
  const [selectedScopes, setSelectedScopes] = useState<string[]>([]);
  const [expiresAt, setExpiresAt] = useState("");
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState("");

  const fetchAccounts = () => {
    api.admin
      .listServiceAccounts()
      .then(setAccounts)
      .catch(() => setLoadError(t("errors.internal_error")));
  };

  // Initial load
  useEffect(() => {
    if (!isAdmin) return;
    fetchAccounts();
  }, [isAdmin]); // eslint-disable-line react-hooks/exhaustive-deps

  // SSE: refetch whenever the database signals a change.
  // Same-origin EventSource sends the httpOnly access_token cookie automatically.
  useEffect(() => {
    if (!isAdmin) return;
    const es = new EventSource("/api/v1/admin/service-accounts/events");
    es.onmessage = (e) => {
      if (e.data === "change") fetchAccounts();
    };
    es.onerror = () => es.close();
    return () => es.close();
  }, [isAdmin]); // eslint-disable-line react-hooks/exhaustive-deps

  function toggleScope(scope: string) {
    setSelectedScopes((prev) =>
      prev.includes(scope) ? prev.filter((s) => s !== scope) : [...prev, scope]
    );
  }

  function resetForm() {
    setName("");
    setSelectedScopes([]);
    setExpiresAt("");
    setCreateError("");
  }

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setCreateError("");
    if (!name.trim()) return;
    setCreating(true);
    try {
      const created = await api.admin.createServiceAccount({
        name: name.trim(),
        scopes: selectedScopes,
        expires_at: expiresAt || undefined,
      });
      setAccounts((prev) => [created, ...prev]);
      setNewSecret(created);
      setShowCreate(false);
      resetForm();
    } catch (err: unknown) {
      const code = err instanceof Error ? err.message : "internal_error";
      setCreateError(
        t(`errors.${code}` as Parameters<typeof t>[0], {
          defaultValue: t("errors.internal_error"),
        })
      );
    } finally {
      setCreating(false);
    }
  }

  async function handleToggleStatus(sa: ServiceAccount) {
    try {
      await api.admin.setServiceAccountStatus(sa.id, !sa.is_active);
      setAccounts((prev) =>
        prev.map((a) => (a.id === sa.id ? { ...a, is_active: !a.is_active } : a))
      );
    } catch {
      alert(t("errors.internal_error"));
    }
  }

  async function handleRevoke(sa: ServiceAccount) {
    if (!confirm(t("revokeConfirm", { name: sa.name }))) return;
    try {
      await api.admin.revokeServiceAccount(sa.id);
      setAccounts((prev) => prev.filter((a) => a.id !== sa.id));
    } catch {
      alert(t("errors.internal_error"));
    }
  }

  if (!isAdmin) {
    return (
      <div className="p-8">
        <p className="text-red-400">{t("accessDenied")}</p>
      </div>
    );
  }

  // Minimum selectable date is tomorrow
  const tomorrow = new Date();
  tomorrow.setDate(tomorrow.getDate() + 1);
  const minDate = tomorrow.toISOString().slice(0, 10);

  return (
    <div className="px-8 py-8 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-white">{t("title")}</h1>
        <button
          onClick={() => {
            resetForm();
            setShowCreate(true);
          }}
          className="bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-medium px-4 py-2 rounded-lg transition-colors"
        >
          + {t("create")}
        </button>
      </div>

      {loadError && <p className="text-red-400">{loadError}</p>}

      {/* Table */}
      <div className="rounded-xl border border-gray-800 overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-gray-900 text-gray-400 text-xs uppercase tracking-wider">
            <tr>
              <th className="px-4 py-3 text-left">{t("name")}</th>
              <th className="px-4 py-3 text-left">{t("clientId")}</th>
              <th className="px-4 py-3 text-left">{t("scopes")}</th>
              <th className="px-4 py-3 text-left">{t("status")}</th>
              <th className="px-4 py-3 text-left">{t("createdAt")}</th>
              <th className="px-4 py-3 text-left">{t("expiresAt")}</th>
              <th className="px-4 py-3" />
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-800">
            {accounts.length === 0 && (
              <tr>
                <td colSpan={7} className="px-4 py-6 text-center text-gray-500">
                  —
                </td>
              </tr>
            )}
            {accounts.map((sa) => {
              const expiry = expiryLabel(sa.expires_at, t("noExpiry"), t("expired"));
              return (
                <tr key={sa.id} className="bg-gray-950 hover:bg-gray-900 transition-colors">
                  <td className="px-4 py-3 text-white font-medium">{sa.name}</td>
                  <td className="px-4 py-3 text-gray-300 font-mono text-xs">{sa.client_id}</td>
                  <td className="px-4 py-3">
                    {sa.scopes.length === 0 ? (
                      <span className="text-gray-500 text-xs">{t("noScopes")}</span>
                    ) : (
                      <div className="flex flex-wrap gap-1">
                        {sa.scopes.map((s) => (
                          <span
                            key={s}
                            className="text-xs px-2 py-0.5 rounded-full bg-sky-900/50 text-sky-300 border border-sky-800"
                          >
                            {s}
                          </span>
                        ))}
                      </div>
                    )}
                  </td>
                  <td className="px-4 py-3">
                    <button
                      onClick={() => handleToggleStatus(sa)}
                      title={sa.is_active ? t("deactivateHint") : t("activateHint")}
                      className={`text-xs px-2 py-0.5 rounded-full border transition-colors cursor-pointer ${
                        sa.is_active
                          ? "bg-emerald-900/50 text-emerald-300 border-emerald-700 hover:bg-red-900/50 hover:text-red-300 hover:border-red-700"
                          : "bg-gray-800 text-gray-400 border-gray-700 hover:bg-emerald-900/50 hover:text-emerald-300 hover:border-emerald-700"
                      }`}
                    >
                      {sa.is_active ? t("active") : t("inactive")}
                    </button>
                  </td>
                  <td className="px-4 py-3 text-gray-400 text-xs">{sa.created_at.slice(0, 10)}</td>
                  <td className="px-4 py-3 text-xs">
                    {expiry.expired ? (
                      <span className="px-2 py-0.5 rounded-full bg-red-900/50 text-red-300 border border-red-800">
                        {t("expired")}
                      </span>
                    ) : (
                      <span className={expiry.text === t("noExpiry") ? "text-gray-500" : "text-gray-300"}>
                        {expiry.text}
                      </span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <button
                      onClick={() => handleRevoke(sa)}
                      className="text-xs text-red-400 hover:text-red-300 transition-colors"
                    >
                      {t("revoke")}
                    </button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {/* Create modal */}
      {showCreate && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm px-4">
          <div className="w-full max-w-md bg-gray-900 border border-gray-700 rounded-2xl p-6 space-y-5">
            <h2 className="text-white font-semibold">{t("createTitle")}</h2>
            <form onSubmit={handleCreate} className="space-y-4">
              <div>
                <label className="block text-xs text-gray-400 mb-1">{t("name")}</label>
                <input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  className="w-full bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-indigo-500"
                  placeholder={t("name")}
                  required
                />
              </div>

              <div>
                <label className="block text-xs text-gray-400 mb-2">{t("scopes")}</label>
                <div className="space-y-2">
                  {ALL_SCOPES.map((scope) => (
                    <label key={scope} className="flex items-center gap-2 cursor-pointer">
                      <input
                        type="checkbox"
                        checked={selectedScopes.includes(scope)}
                        onChange={() => toggleScope(scope)}
                        className="accent-indigo-500"
                      />
                      <span className="text-sm text-gray-300 font-mono">{scope}</span>
                    </label>
                  ))}
                </div>
              </div>

              <div>
                <label className="block text-xs text-gray-400 mb-1">
                  {t("expiresAt")}
                  <span className="ml-1 text-gray-600">({t("noExpiry")})</span>
                </label>
                <input
                  type="date"
                  value={expiresAt}
                  min={minDate}
                  onChange={(e) => setExpiresAt(e.target.value)}
                  className="w-full bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-indigo-500 [color-scheme:dark]"
                />
              </div>

              {createError && <p className="text-red-400 text-xs">{createError}</p>}

              <div className="flex gap-3 justify-end pt-2">
                <button
                  type="button"
                  onClick={() => setShowCreate(false)}
                  className="px-4 py-2 text-sm text-gray-400 hover:text-white transition-colors"
                >
                  {tCommon("cancel")}
                </button>
                <button
                  type="submit"
                  disabled={creating}
                  className="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-sm rounded-lg transition-colors"
                >
                  {creating ? t("creating") : t("create")}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Secret reveal modal */}
      {newSecret && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm px-4">
          <div className="w-full max-w-md bg-gray-900 border border-gray-700 rounded-2xl p-6 space-y-4">
            <h2 className="text-white font-semibold">{t("secretTitle")}</h2>
            <p className="text-yellow-400 text-xs">{t("secretWarning")}</p>
            <div>
              <label className="block text-xs text-gray-400 mb-1">{t("secretLabel")}</label>
              <div className="bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 font-mono text-sm text-emerald-300 break-all select-all">
                {newSecret.client_secret}
              </div>
            </div>
            <button
              onClick={() => setNewSecret(null)}
              className="w-full px-4 py-2 bg-indigo-600 hover:bg-indigo-500 text-white text-sm rounded-lg transition-colors"
            >
              {t("done")}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
