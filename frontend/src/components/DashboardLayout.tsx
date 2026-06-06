import { Link, Outlet, useLocation, useNavigate } from "react-router-dom";
import { useState, Suspense, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { useAuth } from "@/lib/useAuth";
import { cancelRefresh } from "@/lib/tokenManager";
import { api, type MeData } from "@/lib/api";
import { MeContext } from "@/contexts/MeContext";
import Alert from "@mui/material/Alert";
import Avatar from "@mui/material/Avatar";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import CircularProgress from "@mui/material/CircularProgress";
import Divider from "@mui/material/Divider";
import List from "@mui/material/List";
import ListItemButton from "@mui/material/ListItemButton";
import Typography from "@mui/material/Typography";

function displayName(me: Pick<MeData, "email" | "first_name" | "last_name">): string {
  const name = [me.first_name, me.last_name].filter(Boolean).join(" ").trim();
  return name || me.email;
}

function initials(me: Pick<MeData, "email" | "first_name" | "last_name">): string {
  const parts = [me.first_name, me.last_name].filter(Boolean);
  if (parts.length > 0) return parts.map((p) => p[0]).join("").slice(0, 2).toUpperCase();
  return me.email.slice(0, 2).toUpperCase();
}

export default function DashboardLayout() {
  const { t } = useTranslation("nav");
  const { t: tCommon } = useTranslation("common");
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const { i18n } = useTranslation();
  const { me, setMe, loading, bootstrapError, retry } = useAuth();
  const [avatarTimestamp, setAvatarTimestamp] = useState(Date.now());

  useEffect(() => {
    const handleMeUpdated = (e: Event) => {
      const customEvent = e as CustomEvent<MeData>;
      setMe(customEvent.detail);
      setAvatarTimestamp(Date.now());
    };
    window.addEventListener("me:updated", handleMeUpdated);
    return () => {
      window.removeEventListener("me:updated", handleMeUpdated);
    };
  }, [setMe]);

  if (loading) {
    return (
      <Box sx={{ minHeight: "100vh", display: "grid", placeItems: "center", bgcolor: "background.default" }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
          <CircularProgress size={18} />
          <Typography color="text.secondary">{tCommon("loading")}</Typography>
        </Box>
      </Box>
    );
  }

  if (bootstrapError || !me) {
    return (
      <Box sx={{ minHeight: "100vh", display: "grid", placeItems: "center", bgcolor: "background.default", p: 2 }}>
        <Box sx={{ display: "grid", gap: 2, width: "min(100%, 420px)" }}>
          <Alert severity="error">
            {tCommon(`authBootstrap.${bootstrapError ?? "network"}`)}
          </Alert>
          <Button variant="contained" onClick={retry}>
            {tCommon("retry")}
          </Button>
        </Box>
      </Box>
    );
  }

  const isAdmin = me.roles.includes("admin");

  const navLinks = [
    { to: "/dashboard", label: t("dashboard") },
    { to: "/dashboard/mfa", label: t("mfa") },
    { to: "/dashboard/settings", label: t("settings") },
    ...(isAdmin
      ? [
          { to: "/dashboard/users", label: t("users") },
          { to: "/dashboard/audit", label: t("audit") },
          { to: "/dashboard/service-accounts", label: t("serviceAccounts") },
        ]
      : []),
  ];

  async function handleLocaleChange(locale: "tr" | "en") {
    if (locale === i18n.language) return;
    // Locale changes are audited server-side as a security signal.
    await api.updateLocale(locale).catch(() => {});
    i18n.changeLanguage(locale);
    localStorage.setItem("locale", locale);
  }

  function openProfile() {
    navigate("/dashboard/settings");
  }

  function handleLogout() {
    cancelRefresh();
    void api.logout();
    navigate("/auth/login");
  }

  return (
    <MeContext.Provider value={me}>
      <Box sx={{ display: "flex", height: "100vh", bgcolor: "background.default" }}>
        {/* ── Sidebar ───────────────────────────────────────────────── */}
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
            <Typography sx={{ fontWeight: 700, letterSpacing: 0.2 }}>{tCommon("appName")}</Typography>
          </Box>
          <Divider />

          <List component="nav" sx={{ flex: 1, px: 1.5, py: 2 }}>
            {navLinks.map(({ to, label }) => (
              <ListItemButton
                key={to}
                component={Link}
                to={to}
                selected={pathname === to}
                sx={{ borderRadius: 1, mb: 0.5 }}
              >
                <Typography variant="body2">{label}</Typography>
              </ListItemButton>
            ))}
          </List>

          <Divider />
          <Box sx={{ display: "grid", gap: 1.5, px: 2, py: 2 }}>
            <Button
              color="inherit"
              onClick={openProfile}
              sx={{ justifyContent: "flex-start", gap: 1, p: 0, textTransform: "none" }}
            >
              <Avatar
                src={me.has_avatar ? `/api/v1/users/${me.user_id}/avatar?t=${avatarTimestamp}` : undefined}
                sx={{ width: 32, height: 32, fontSize: 13 }}
              >
                {initials(me)}
              </Avatar>
              <Box sx={{ minWidth: 0, textAlign: "left" }}>
                <Typography variant="body2" noWrap sx={{ fontWeight: 700 }}>{displayName(me)}</Typography>
                <Typography variant="caption" color="text.secondary" noWrap>{me.email}</Typography>
              </Box>
            </Button>
            <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
              {me.roles.map((r) => <Chip key={r} size="small" color="primary" label={r} />)}
            </Box>
            <Box sx={{ display: "flex", gap: 0.5 }}>
              {(["tr", "en"] as const).map((l) => (
                <Button
                  key={l}
                  size="small"
                  variant={l === i18n.language ? "contained" : "outlined"}
                  onClick={() => handleLocaleChange(l)}
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

        {/* ── Page content ──────────────────────────────────────────── */}
        <Box component="main" sx={{ flex: 1, minWidth: 0, overflow: "hidden" }}>
          <Suspense
            fallback={
              <Box sx={{ display: "flex", alignItems: "center", justifyContent: "center", height: "100%" }}>
                <CircularProgress size={24} />
              </Box>
            }
          >
            <Outlet />
          </Suspense>
        </Box>
      </Box>
    </MeContext.Provider>
  );
}
