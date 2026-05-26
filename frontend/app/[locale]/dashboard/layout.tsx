"use client";

import { useTranslations, useLocale } from "next-intl";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useAuth } from "@/lib/useAuth";
import { cancelRefresh } from "@/lib/tokenManager";
import { api } from "@/lib/api";
import { MeContext } from "./context";
import { Badge } from "@/components/ui/badge";

export default function DashboardLayout({ children }: { children: React.ReactNode }) {
  const t = useTranslations("nav");
  const tCommon = useTranslations("common");
  const locale = useLocale();
  const router = useRouter();
  const pathname = usePathname();
  const { me, loading } = useAuth();

  function handleLogout() {
    cancelRefresh();
    api.logout();
    router.push(`/${locale}/auth/login`);
  }

  if (loading || !me) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-950">
        <p className="text-gray-400">{tCommon("loading")}</p>
      </div>
    );
  }

  const isAdmin = me.roles.includes("admin");

  const navLinks = [
    { href: `/${locale}/dashboard`, label: t("dashboard") },
    { href: `/${locale}/dashboard/sessions`, label: t("sessions") },
    { href: `/${locale}/dashboard/mfa`, label: t("mfa") },
    ...(isAdmin
      ? [
          { href: `/${locale}/dashboard/users`, label: t("users") },
          { href: `/${locale}/dashboard/audit`, label: t("audit") },
          { href: `/${locale}/dashboard/service-accounts`, label: t("serviceAccounts") },
        ]
      : []),
  ];

  return (
    <MeContext.Provider value={me}>
      <div className="h-screen bg-gray-950 flex">
        {/* Sidebar */}
        <aside className="w-56 shrink-0 border-r border-gray-800 flex flex-col">
          <div className="px-5 py-5 border-b border-gray-800">
            <span className="text-white font-bold tracking-wide">{tCommon("appName")}</span>
          </div>

          <nav className="flex-1 px-3 py-4 space-y-1">
            {navLinks.map(({ href, label }) => {
              const active = pathname === href;
              return (
                <Link
                  key={href}
                  href={href}
                  className={`flex items-center gap-2 px-3 py-2 rounded-lg text-sm transition-colors ${
                    active
                      ? "bg-indigo-600 text-white"
                      : "text-gray-400 hover:text-white hover:bg-gray-800"
                  }`}
                >
                  {label}
                </Link>
              );
            })}
          </nav>

          <div className="px-4 py-4 border-t border-gray-800 space-y-3">
            <p className="text-xs text-gray-500 truncate">{me.email}</p>
            <div className="flex flex-wrap gap-1">
              {me.roles.map((r) => <Badge key={r} variant="indigo">{r}</Badge>)}
            </div>
            <div className="flex gap-1">
              {(["tr", "en"] as const).map((l) => (
                <button
                  key={l}
                  onClick={async () => {
                    if (l === locale) return;
                    await api.updateLocale(l).catch(() => {});
                    router.push(pathname.replace(new RegExp(`^/${locale}`), `/${l}`));
                  }}
                  className={`text-xs px-2 py-1 rounded border transition-colors ${
                    l === locale
                      ? "border-indigo-600 text-indigo-400 bg-indigo-900/30"
                      : "border-gray-700 text-gray-500 hover:text-white hover:border-gray-500"
                  }`}
                >
                  {l.toUpperCase()}
                </button>
              ))}
            </div>
            <button
              onClick={handleLogout}
              className="w-full text-xs text-gray-400 hover:text-white transition-colors text-left"
            >
              {tCommon("logout")}
            </button>
          </div>
        </aside>

        {/* Main content */}
        <main className="flex-1 overflow-hidden">{children}</main>
      </div>
    </MeContext.Provider>
  );
}
