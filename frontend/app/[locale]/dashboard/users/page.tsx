"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { api, ApiError, UserData } from "@/lib/api";
import { useMeContext } from "../context";

const AVAILABLE_ROLES = ["admin", "user"];

export default function UsersPage() {
  const t = useTranslations("admin");
  const tCommon = useTranslations("common");
  const me = useMeContext();

  const [users, setUsers] = useState<UserData[]>([]);
  const [loadingUsers, setLoadingUsers] = useState(true);
  const [showModal, setShowModal] = useState(false);

  // Create form state
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [selectedRoles, setSelectedRoles] = useState<string[]>(["user"]);
  const [formError, setFormError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);

  const isAdmin = me?.roles.includes("admin") ?? false;

  useEffect(() => {
    if (!isAdmin) return;
    api.admin
      .listUsers()
      .then(setUsers)
      .catch(() => {})
      .finally(() => setLoadingUsers(false));
  }, [isAdmin]);

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setFormError(null);
    setCreating(true);
    try {
      const user = await api.admin.createUser({
        email,
        password,
        locale: me?.locale ?? "tr",
        roles: selectedRoles,
      });
      setUsers((prev) => [...prev, user]);
      setShowModal(false);
      setEmail("");
      setPassword("");
      setSelectedRoles(["user"]);
    } catch (err: unknown) {
      const code = err instanceof ApiError ? err.message : "internal_error";
      const key = `errors.${code}` as Parameters<typeof t>[0];
      setFormError(t(key, { defaultValue: t("errors.internal_error") }));
    } finally {
      setCreating(false);
    }
  }

  function toggleRole(role: string) {
    setSelectedRoles((prev) =>
      prev.includes(role) ? prev.filter((r) => r !== role) : [...prev, role]
    );
  }

  if (!isAdmin) {
    return (
      <div className="px-8 py-8">
        <p className="text-red-400">{t("accessDenied")}</p>
      </div>
    );
  }

  return (
    <div className="px-8 py-8 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-white">{t("usersTitle")}</h1>
        <button
          onClick={() => setShowModal(true)}
          className="bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-medium px-4 py-2 rounded-lg transition-colors"
        >
          + {t("createUser")}
        </button>
      </div>

      {/* User table */}
      <div className="rounded-xl border border-gray-800 overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-gray-900 text-gray-400 text-xs uppercase tracking-wider">
            <tr>
              <th className="px-4 py-3 text-left">{t("email")}</th>
              <th className="px-4 py-3 text-left">{t("roles")}</th>
              <th className="px-4 py-3 text-left">{t("status")}</th>
              <th className="px-4 py-3 text-left">{t("createdAt")}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-800">
            {loadingUsers ? (
              <tr>
                <td colSpan={4} className="px-4 py-6 text-center text-gray-500">
                  {tCommon("loading")}
                </td>
              </tr>
            ) : users.length === 0 ? (
              <tr>
                <td colSpan={4} className="px-4 py-6 text-center text-gray-500">
                  —
                </td>
              </tr>
            ) : (
              users.map((u) => (
                <tr key={u.id} className="bg-gray-950 hover:bg-gray-900 transition-colors">
                  <td className="px-4 py-3 text-gray-200">{u.email}</td>
                  <td className="px-4 py-3">
                    <div className="flex flex-wrap gap-1">
                      {u.roles.length === 0 ? (
                        <span className="text-gray-500 text-xs">{t("noRoles")}</span>
                      ) : (
                        u.roles.map((r) => (
                          <span
                            key={r}
                            className="text-xs px-2 py-0.5 rounded-full bg-indigo-900/60 text-indigo-300 border border-indigo-700"
                          >
                            {r}
                          </span>
                        ))
                      )}
                    </div>
                  </td>
                  <td className="px-4 py-3">
                    <span
                      className={`text-xs px-2 py-0.5 rounded-full ${
                        u.is_active
                          ? "bg-green-900/40 text-green-400 border border-green-700"
                          : "bg-red-900/40 text-red-400 border border-red-700"
                      }`}
                    >
                      {u.is_active ? t("active") : t("inactive")}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-gray-400 text-xs">
                    {new Date(u.created_at).toLocaleDateString()}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Create user modal */}
      {showModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm px-4">
          <div className="w-full max-w-md bg-gray-900 border border-gray-700 rounded-2xl p-6 space-y-5">
            <h2 className="text-lg font-semibold text-white">{t("createUserTitle")}</h2>

            <form onSubmit={handleCreate} className="space-y-4">
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
                <label className="block text-sm text-gray-400 mb-1">{t("password")}</label>
                <input
                  type="password"
                  required
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  className="w-full rounded-lg bg-gray-800 border border-gray-700 text-white px-4 py-2.5 focus:outline-none focus:ring-2 focus:ring-indigo-500"
                />
              </div>

              <div>
                <label className="block text-sm text-gray-400 mb-2">{t("roles")}</label>
                <div className="flex gap-2">
                  {AVAILABLE_ROLES.map((role) => (
                    <button
                      key={role}
                      type="button"
                      onClick={() => toggleRole(role)}
                      className={`px-3 py-1.5 rounded-lg text-sm border transition-colors ${
                        selectedRoles.includes(role)
                          ? "bg-indigo-600 border-indigo-500 text-white"
                          : "bg-gray-800 border-gray-700 text-gray-400 hover:border-gray-500"
                      }`}
                    >
                      {role}
                    </button>
                  ))}
                </div>
              </div>

              {formError && (
                <p className="text-sm text-red-400 bg-red-950/50 border border-red-800 rounded-lg px-4 py-2">
                  {formError}
                </p>
              )}

              <div className="flex gap-3 pt-1">
                <button
                  type="submit"
                  disabled={creating}
                  className="flex-1 bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white font-medium py-2.5 rounded-lg transition-colors"
                >
                  {creating ? t("creating") : t("create")}
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setShowModal(false);
                    setFormError(null);
                  }}
                  className="flex-1 bg-gray-800 hover:bg-gray-700 text-gray-300 font-medium py-2.5 rounded-lg transition-colors"
                >
                  {t("cancel")}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
