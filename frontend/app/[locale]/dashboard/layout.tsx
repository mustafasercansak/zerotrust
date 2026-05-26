"use client";

import { useTranslations, useLocale } from "next-intl";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useAuth } from "@/lib/useAuth";
import { cancelRefresh } from "@/lib/tokenManager";
import { api } from "@/lib/api";
import { MeContext } from "./context";

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
    ...(isAdmin
      ? [
          { href: `/${locale}/dashboard/users`, label: t("users") },
          { href: `/${locale}/dashboard/service-accounts`, label: t("serviceAccounts") },
        ]
      : []),
  ];

  return (
    <MeContext.Provider value={me}>
      <div className="min-h-screen bg-gray-950 flex">
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

          <div className="px-4 py-4 border-t border-gray-800">
            <p className="text-xs text-gray-500 truncate mb-2">{me.email}</p>
            <div className="flex flex-wrap gap-1 mb-3">
              {me.roles.map((r) => (
                <span
                  key={r}
                  className="text-xs px-2 py-0.5 rounded-full bg-indigo-900/60 text-indigo-300 border border-indigo-700"
                >
                  {r}
                </span>
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
        <main className="flex-1 overflow-auto">{children}</main>
      </div>
    </MeContext.Provider>
  );
}
