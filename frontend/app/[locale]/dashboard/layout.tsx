"use client";

import { useTranslations, useLocale } from "next-intl";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useAuth } from "@/lib/useAuth";
import { cancelRefresh } from "@/lib/tokenManager";
import { api } from "@/lib/api";
import { MeContext } from "./context";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import CircularProgress from "@mui/material/CircularProgress";
import Divider from "@mui/material/Divider";
import List from "@mui/material/List";
import ListItemButton from "@mui/material/ListItemButton";
import Typography from "@mui/material/Typography";

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
      <Box sx={{ minHeight: "100vh", display: "grid", placeItems: "center", bgcolor: "background.default" }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
          <CircularProgress size={18} />
          <Typography color="text.secondary">{tCommon("loading")}</Typography>
        </Box>
      </Box>
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
          { href: `/${locale}/dashboard/settings`, label: t("settings") },
        ]
      : []),
  ];

  return (
    <MeContext.Provider value={me}>
      <Box sx={{ display: "flex", height: "100vh", bgcolor: "background.default" }}>
        <Box
          component="aside"
          sx={{
            borderRight: 1,
            borderColor: "divider",
            display: "flex",
            flexDirection: "column",
            flexShrink: 0,
            width: 228,
          }}
        >
          <Box sx={{ px: 2.5, py: 2.5 }}>
            <Typography fontWeight={700} letterSpacing={0.2}>{tCommon("appName")}</Typography>
          </Box>
          <Divider />

          <List component="nav" sx={{ flex: 1, px: 1.5, py: 2 }}>
            {navLinks.map(({ href, label }) => {
              const active = pathname === href;
              return (
                <ListItemButton
                  key={href}
                  component={Link}
                  href={href}
                  selected={active}
                  sx={{ borderRadius: 1, mb: 0.5 }}
                >
                  <Typography variant="body2">{label}</Typography>
                </ListItemButton>
              );
            })}
          </List>

          <Divider />
          <Box sx={{ display: "grid", gap: 1.5, px: 2, py: 2 }}>
            <Typography variant="caption" color="text.secondary" noWrap>{me.email}</Typography>
            <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
              {me.roles.map((r) => <Chip key={r} size="small" color="primary" label={r} />)}
            </Box>
            <Box sx={{ display: "flex", gap: 0.5 }}>
              {(["tr", "en"] as const).map((l) => (
                <Button
                  key={l}
                  onClick={async () => {
                    if (l === locale) return;
                    await api.updateLocale(l).catch(() => {});
                    router.push(pathname.replace(new RegExp(`^/${locale}`), `/${l}`));
                  }}
                  size="small"
                  variant={l === locale ? "contained" : "outlined"}
                >
                  {l.toUpperCase()}
                </Button>
              ))}
            </Box>
            <Button
              onClick={handleLogout}
              color="inherit"
              size="small"
              sx={{ justifyContent: "flex-start", px: 0 }}
            >
              {tCommon("logout")}
            </Button>
          </Box>
        </Box>

        <Box component="main" sx={{ flex: 1, minWidth: 0, overflow: "hidden" }}>{children}</Box>
      </Box>
    </MeContext.Provider>
  );
}
