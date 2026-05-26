"use client";

import { useCallback, useMemo, useState } from "react";
import { useTranslations, useLocale } from "next-intl";
import { api, type PageParams, type ServiceAccount, type ServiceAccountCreated } from "@/lib/api";
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
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { Copy, Check } from "lucide-react";

const ALL_SCOPES = [
  "users:read",
  "users:write",
  "service_accounts:read",
  "service_accounts:write",
  "service_accounts:delete",
];

export default function ServiceAccountsPage() {
  const t = useTranslations("serviceAccounts");
  const tCommon = useTranslations("common");
  const locale = useLocale();
  const me = useMeContext();
  const isAdmin = me?.roles.includes("admin") ?? false;

  const [showCreate, setShowCreate] = useState(false);
  const [newSecret, setNewSecret] = useState<ServiceAccountCreated | null>(null);
  const [copied, setCopied] = useState(false);
  const [name, setName] = useState("");
  const [selectedScopes, setSelectedScopes] = useState<string[]>([]);
  const [expiresAt, setExpiresAt] = useState("");
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState("");
  const [refresh, setRefresh] = useState(0);

  const fetcher = useCallback(
    (p: PageParams) => api.admin.listServiceAccounts(p),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [refresh],
  );

  const tabs = useMemo<Tab[]>(() => [
    { key: "all",      label: tCommon("filterAll") },
    { key: "active",   label: tCommon("filterActive"),   preset: { status: "active" } },
    { key: "inactive", label: tCommon("filterInactive"), preset: { status: "inactive" } },
    { key: "expired",  label: tCommon("filterExpired"),  preset: { status: "expired" } },
  ], [tCommon]);

  const columns = useMemo<Column<ServiceAccount>[]>(() => [
    {
      key: "name",
      label: t("name"),
      sortKey: "name",
      filterKey: "name",
      render: (sa) => <span className="text-white font-medium">{sa.name}</span>,
    },
    {
      key: "client_id",
      label: t("clientId"),
      render: (sa) => <span className="text-gray-300 font-mono text-xs">{sa.client_id}</span>,
    },
    {
      key: "scopes",
      label: t("scopes"),
      render: (sa) =>
        sa.scopes.length === 0 ? (
          <Badge variant="muted">{t("noScopes")}</Badge>
        ) : (
          <div className="flex flex-wrap gap-1">
            {sa.scopes.map((s) => <Badge key={s} variant="sky">{s}</Badge>)}
          </div>
        ),
    },
    {
      key: "status",
      label: t("status"),
      sortKey: "is_active",
      render: (sa) => (
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
      ),
    },
    {
      key: "created_at",
      label: t("createdAt"),
      sortKey: "created_at",
      render: (sa) => <span className="text-gray-400 text-xs">{formatDate(sa.created_at, locale)}</span>,
    },
    {
      key: "expires_at",
      label: t("expiresAt"),
      sortKey: "expires_at",
      render: (sa) => {
        if (!sa.expires_at) return <Badge variant="muted">{t("noExpiry")}</Badge>;
        const expired = new Date(sa.expires_at) < new Date();
        return expired
          ? <Badge variant="danger">{t("expired")}</Badge>
          : <span className="text-gray-300 text-xs">{formatDate(sa.expires_at, locale)}</span>;
      },
    },
    {
      key: "actions",
      label: "",
      className: "text-right",
      render: (sa) => (
        <Button variant="danger" size="sm" onClick={() => handleRevoke(sa)}>
          {t("revoke")}
        </Button>
      ),
    },
  // eslint-disable-next-line react-hooks/exhaustive-deps
  ], [t, locale]);

  async function handleToggleStatus(sa: ServiceAccount) {
    try {
      await api.admin.setServiceAccountStatus(sa.id, !sa.is_active);
      setRefresh((n) => n + 1);
    } catch {
      alert(t("errors.internal_error"));
    }
  }

  async function handleRevoke(sa: ServiceAccount) {
    if (!confirm(t("revokeConfirm", { name: sa.name }))) return;
    try {
      await api.admin.revokeServiceAccount(sa.id);
      setRefresh((n) => n + 1);
    } catch {
      alert(t("errors.internal_error"));
    }
  }

  function resetForm() {
    setName(""); setSelectedScopes([]); setExpiresAt(""); setCreateError("");
  }

  function toggleScope(scope: string) {
    setSelectedScopes((prev) =>
      prev.includes(scope) ? prev.filter((s) => s !== scope) : [...prev, scope]
    );
  }

  async function handleCreate(e: { preventDefault(): void }) {
    e.preventDefault();
    setCreateError("");
    if (!name.trim()) return;
    setCreating(true);
    try {
      const created = await api.admin.createServiceAccount({
        name: name.trim(), scopes: selectedScopes, expires_at: expiresAt || undefined,
      });
      setNewSecret(created);
      setShowCreate(false);
      resetForm();
      setRefresh((n) => n + 1);
    } catch (err) {
      const code = err instanceof Error ? err.message : "internal_error";
      setCreateError(t(`errors.${code}` as Parameters<typeof t>[0], { defaultValue: t("errors.internal_error") }));
    } finally {
      setCreating(false);
    }
  }

  function handleCopy() {
    if (!newSecret) return;
    navigator.clipboard.writeText(newSecret.client_secret).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }

  if (!isAdmin) {
    return <div className="p-8"><p className="text-red-400">{t("accessDenied")}</p></div>;
  }

  const tomorrow = new Date();
  tomorrow.setDate(tomorrow.getDate() + 1);
  const minDate = tomorrow.toISOString().slice(0, 10);

  return (
    <div className="flex flex-col h-full px-8 py-6 gap-4">
      <div className="shrink-0 flex justify-end">
        <Button onClick={() => { resetForm(); setShowCreate(true); }}>
          + {t("create")}
        </Button>
      </div>

      <DataTable
        columns={columns}
        tabs={tabs}
        fetcher={fetcher}
        rowKey={(sa) => sa.id}
        defaultSortKey="created_at"
        defaultSortDir="desc"
        pageSizeOptions={[10, 25, 50]}
        defaultPageSize={25}
      />

      {/* Create modal */}
      <Dialog open={showCreate} onOpenChange={(open) => { if (!open) setShowCreate(false); }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("createTitle")}</DialogTitle>
          </DialogHeader>

          <form onSubmit={handleCreate} className="space-y-4">
            <div>
              <Label htmlFor="sa-name">{t("name")}</Label>
              <Input
                id="sa-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
                placeholder={t("name")}
              />
            </div>

            <div>
              <Label>{t("scopes")}</Label>
              <div className="mt-1.5 space-y-2 bg-gray-800/50 border border-gray-700/60 rounded-lg p-3">
                {ALL_SCOPES.map((scope) => (
                  <label key={scope} className="flex items-center gap-2.5 cursor-pointer group">
                    <input
                      type="checkbox"
                      checked={selectedScopes.includes(scope)}
                      onChange={() => toggleScope(scope)}
                      className="w-3.5 h-3.5 accent-indigo-500 shrink-0"
                    />
                    <span className="text-xs text-gray-300 font-mono group-hover:text-white transition-colors">
                      {scope}
                    </span>
                  </label>
                ))}
              </div>
            </div>

            <div>
              <Label htmlFor="sa-expires">
                {t("expiresAt")} <span className="text-gray-600">({t("noExpiry")})</span>
              </Label>
              <Input
                id="sa-expires"
                type="date"
                value={expiresAt}
                min={minDate}
                onChange={(e) => setExpiresAt(e.target.value)}
                className="[color-scheme:dark]"
              />
            </div>

            {createError && (
              <p className="text-sm text-red-400 bg-red-950/50 border border-red-800 rounded-lg px-4 py-2">
                {createError}
              </p>
            )}

            <DialogFooter className="justify-end">
              <Button type="button" variant="ghost" onClick={() => setShowCreate(false)}>
                {tCommon("cancel")}
              </Button>
              <Button type="submit" disabled={creating}>
                {creating ? t("creating") : t("create")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Secret reveal modal */}
      <Dialog open={!!newSecret} onOpenChange={(open) => { if (!open) setNewSecret(null); }}>
        <DialogContent showClose={false}>
          <DialogHeader>
            <DialogTitle>{t("secretTitle")}</DialogTitle>
            <DialogDescription className="text-yellow-400/90">
              {t("secretWarning")}
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-3">
            <Label>{t("secretLabel")}</Label>
            <div className="relative">
              <div className="bg-gray-800 border border-gray-700 rounded-lg px-3 py-2.5 font-mono text-sm text-emerald-300 break-all select-all pr-10">
                {newSecret?.client_secret}
              </div>
              <button
                onClick={handleCopy}
                title="Copy"
                className="absolute right-2 top-1/2 -translate-y-1/2 p-1.5 rounded text-gray-400 hover:text-white hover:bg-gray-700 transition-colors"
              >
                {copied ? <Check size={14} className="text-green-400" /> : <Copy size={14} />}
              </button>
            </div>
          </div>

          <DialogFooter className="mt-2">
            <Button className="w-full" onClick={() => setNewSecret(null)}>
              {t("done")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
