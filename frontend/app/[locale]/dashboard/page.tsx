"use client";

import { useTranslations, useLocale } from "next-intl";
import { useMeContext } from "./context";
import { Badge } from "@/components/ui/badge";

export default function DashboardPage() {
  const t = useTranslations("dashboard");
  const locale = useLocale();
  const me = useMeContext();

  if (!me) return null;

  return (
    <div className="h-full overflow-auto px-8 py-8 max-w-2xl space-y-6">
      <h1 className="text-2xl font-bold text-white">{t("title")}</h1>

      <div className="rounded-xl bg-gray-900 border border-gray-800 p-6 space-y-3">
        <p className="text-gray-300">{t("welcome", { email: me.email })}</p>
        <div className="flex items-center gap-2">
          <span className="inline-block w-2 h-2 rounded-full bg-green-400" />
          <span className="text-sm text-green-400">{t("verified")}</span>
        </div>
      </div>

      <div className="rounded-xl bg-gray-900 border border-gray-800 p-6 space-y-2">
        <p className="text-xs text-gray-500 uppercase tracking-wider">{t("securityStatus")}</p>
        <div className="grid grid-cols-2 gap-2 text-sm">
          <span className="text-gray-400">User ID</span>
          <span className="text-gray-200 font-mono text-xs truncate">{me.user_id}</span>
          <span className="text-gray-400">{t("locale")}</span>
          <span className="text-gray-200">{locale.toUpperCase()}</span>
          <span className="text-gray-400">{t("roles")}</span>
          <div className="flex flex-wrap gap-1">
            {me.roles.map((r) => <Badge key={r} variant="indigo">{r}</Badge>)}
          </div>
        </div>
      </div>
    </div>
  );
}
