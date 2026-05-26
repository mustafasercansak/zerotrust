"use client";

import { useState, useCallback, useMemo } from "react";
import { useTranslations, useLocale } from "next-intl";
import { api, ApiError, type PageParams, type UserData } from "@/lib/api";
import { formatDate } from "@/lib/dateUtils";
import { useMeContext } from "../context";
import { DataTable, type Column, type Tab } from "@/components/DataTable";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input, Label } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";

const AVAILABLE_ROLES = ["admin", "user"];

export default function UsersPage() {
  const t = useTranslations("admin");
  const tCommon = useTranslations("common");
  const locale = useLocale();
  const me = useMeContext();
  const isAdmin = me?.roles.includes("admin") ?? false;

  const [showModal, setShowModal] = useState(false);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [selectedRoles, setSelectedRoles] = useState<string[]>(["user"]);
  const [formError, setFormError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [refresh, setRefresh] = useState(0);

  const fetcher = useCallback(
    (p: PageParams) => api.admin.listUsers(p),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [refresh],
  );

  const tabs = useMemo<Tab[]>(() => [
    { key: "all",      label: tCommon("filterAll") },
    { key: "active",   label: tCommon("filterActive"),   preset: { status: "active" } },
    { key: "inactive", label: tCommon("filterInactive"), preset: { status: "inactive" } },
  ], [tCommon]);

  const columns = useMemo<Column<UserData>[]>(() => [
    {
      key: "email",
      label: t("email"),
      sortKey: "email",
      filterKey: "email",
      render: (u) => <span className="text-gray-200">{u.email}</span>,
    },
    {
      key: "roles",
      label: t("roles"),
      render: (u) =>
        u.roles.length === 0 ? (
          <Badge variant="muted">{t("noRoles")}</Badge>
        ) : (
          <div className="flex flex-wrap gap-1">
            {u.roles.map((r) => <Badge key={r} variant="indigo">{r}</Badge>)}
          </div>
        ),
    },
    {
      key: "status",
      label: t("status"),
      sortKey: "is_active",
      render: (u) => (
        <Badge variant={u.is_active ? "success" : "danger"}>
          {u.is_active ? t("active") : t("inactive")}
        </Badge>
      ),
    },
    {
      key: "created_at",
      label: t("createdAt"),
      sortKey: "created_at",
      render: (u) => <span className="text-gray-400 text-xs">{formatDate(u.created_at, locale)}</span>,
    },
  ], [t, locale]);

  function toggleRole(role: string) {
    setSelectedRoles((prev) =>
      prev.includes(role) ? prev.filter((r) => r !== role) : [...prev, role]
    );
  }

  function openModal() {
    setEmail("");
    setPassword("");
    setSelectedRoles(["user"]);
    setFormError(null);
    setShowModal(true);
  }

  async function handleCreate(e: { preventDefault(): void }) {
    e.preventDefault();
    setFormError(null);
    setCreating(true);
    try {
      await api.admin.createUser({ email, password, locale: me?.locale ?? "tr", roles: selectedRoles });
      setShowModal(false);
      setRefresh((n) => n + 1);
    } catch (err) {
      const code = err instanceof ApiError ? err.message : "internal_error";
      setFormError(t(`errors.${code}` as Parameters<typeof t>[0], { defaultValue: t("errors.internal_error") }));
    } finally {
      setCreating(false);
    }
  }

  if (!isAdmin) {
    return <div className="px-8 py-8"><p className="text-red-400">{t("accessDenied")}</p></div>;
  }

  return (
    <div className="flex flex-col h-full px-8 py-6 gap-4">
      <div className="shrink-0 flex justify-end">
        <Button onClick={openModal}>+ {t("createUser")}</Button>
      </div>

      <DataTable
        columns={columns}
        tabs={tabs}
        fetcher={fetcher}
        rowKey={(u) => u.id}
        defaultSortKey="created_at"
        defaultSortDir="desc"
        pageSizeOptions={[10, 25, 50]}
        defaultPageSize={25}
      />

      <Dialog open={showModal} onOpenChange={(open) => { if (!open) setShowModal(false); }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("createUserTitle")}</DialogTitle>
          </DialogHeader>

          <form onSubmit={handleCreate} className="space-y-4">
            <div>
              <Label htmlFor="email">{t("email")}</Label>
              <Input
                id="email"
                type="email"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
            </div>

            <div>
              <Label htmlFor="password">{t("password")}</Label>
              <Input
                id="password"
                type="password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </div>

            <div>
              <Label>{t("roles")}</Label>
              <div className="flex gap-2 mt-1">
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

            <DialogFooter>
              <Button type="submit" disabled={creating} className="flex-1">
                {creating ? t("creating") : t("create")}
              </Button>
              <Button
                type="button"
                variant="secondary"
                className="flex-1"
                onClick={() => setShowModal(false)}
              >
                {tCommon("cancel")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
