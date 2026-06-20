import { Link, Outlet, useLocation, useNavigate } from "react-router-dom";
import { useState, Suspense, useEffect, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { useAuth } from "@/lib/useAuth";
import { cancelRefresh } from "@/lib/tokenManager";
import { api, type MeData } from "@/lib/api";
import { MeContext } from "@/contexts/MeContext";
import { useIdleTimeout } from "@/hooks/useIdleTimeout";
import { SessionTimeoutDialog } from "@/components/SessionTimeoutDialog";
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
  const { me, setMe, loading, bootstrapError, retry, localeWarning, dismissLocaleWarning, anomalyWarning, dismissAnomalyWarning } = useAuth();
  const [avatarTimestamp, setAvatarTimestamp] = useState(Date.now());
  const [newDeviceWarning, setNewDeviceWarning] = useState<string | null>(null);
  const [sessionEndedWarning, setSessionEndedWarning] = useState<string | null>(null);

  // Defined with useCallback so useIdleTimeout receives a stable reference
  // and can be called before the conditional early-returns (Rules of Hooks).
  const handleLogout = useCallback(() => {
    cancelRefresh();
    void api.logout();
    navigate("/auth/login");
  }, [navigate]);

  const { warningVisible, secondsRemaining, extendSession, dismissWarning } = useIdleTimeout(handleLogout);

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

  useEffect(() => {
    const handleNewDevice = (e: Event) => {
      const customEvent = e as CustomEvent<{ device: string }>;
      setNewDeviceWarning(customEvent.detail.device);
    };
    const handleSessionEnded = (e: Event) => {
      const customEvent = e as CustomEvent<{ device: string }>;
      setSessionEndedWarning(customEvent.detail.device);
    };
    window.addEventListener("session:new_device", handleNewDevice);
    window.addEventListener("session:ended", handleSessionEnded);
    return () => {
      window.removeEventListener("session:new_device", handleNewDevice);
      window.removeEventListener("session:ended", handleSessionEnded);
    };
  }, []);

  if (loading) {
    return (
      <Box
        sx={{
          minHeight: "100vh",
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          justifyContent: "center",
          bgcolor: "background.default",
          gap: 3,
        }}
      >
        <Box
          sx={{
            position: "relative",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
          }}
        >
          <Box
            sx={{
              position: "absolute",
              width: 64,
              height: 64,
              borderRadius: "50%",
              border: "2px solid #6366f1",
              opacity: 0.5,
              animation: "pulseRing 2s cubic-bezier(0.215, 0.610, 0.355, 1) infinite",
              "@keyframes pulseRing": {
                "0%": { transform: "scale(0.8)", opacity: 0.5 },
                "80%, 100%": { transform: "scale(2.2)", opacity: 0 },
              },
            }}
          />
          <CircularProgress size={48} thickness={3.5} sx={{ color: "primary.main" }} />
        </Box>
        <Typography
          variant="body2"
          color="text.secondary"
          sx={{
            fontWeight: 500,
            letterSpacing: 0.5,
            animation: "pulseText 1.5s ease-in-out infinite alternate",
            "@keyframes pulseText": {
              "0%": { opacity: 0.4 },
              "100%": { opacity: 1 },
            },
          }}
        >
          {tCommon("loading")}
        </Typography>
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
    { to: "/dashboard/sessions", label: t("sessions") },
    { to: "/dashboard/settings", label: t("settings") },
    ...(isAdmin
      ? [
          { to: "/dashboard/users", label: t("users") },
          { to: "/dashboard/security", label: t("security") },
          { to: "/dashboard/audit", label: t("audit") },
          { to: "/dashboard/service-accounts", label: t("serviceAccounts") },
          { to: "/dashboard/oidc-clients", label: t("oidcClients") },
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
          <Box sx={{ px: 2.5, py: 2.5, display: "flex", alignItems: "center", gap: 1.5 }}>
            <Box
              component="img"
              src="/logo.png"
              alt="ZeroTrust Logo"
              sx={{
                width: 32,
                height: 32,
                objectFit: "contain",
                borderRadius: "4px",
              }}
            />
            <Typography sx={{ fontWeight: 700, letterSpacing: 0.2, fontSize: "1.1rem" }}>
              {tCommon("appName")}
            </Typography>
          </Box>
          <Divider />

          <List component="nav" sx={{ flex: 1, px: 1.5, py: 2 }}>
            {navLinks.map(({ to, label }) => (
              <ListItemButton
                key={to}
                component={Link}
                to={to}
                selected={pathname === to}
                sx={{
                  borderRadius: 1.25,
                  mb: 0.75,
                  position: "relative",
                  transition: "all 0.2s cubic-bezier(0.4, 0, 0.2, 1)",
                  px: 2,
                  py: 1.25,
                  "&.Mui-selected": {
                    bgcolor: "rgba(99, 102, 241, 0.12)",
                    color: "primary.main",
                    fontWeight: 600,
                    "&::before": {
                      content: '""',
                      position: "absolute",
                      left: 0,
                      top: "20%",
                      height: "60%",
                      width: 3,
                      borderRadius: 1,
                      bgcolor: "primary.main",
                    },
                    "&:hover": {
                      bgcolor: "rgba(99, 102, 241, 0.18)",
                    },
                  },
                  "&:hover": {
                    bgcolor: "rgba(255, 255, 255, 0.04)",
                    transform: "translateX(4px)",
                  },
                }}
              >
                <Typography variant="body2" sx={{ fontWeight: pathname === to ? 600 : 400 }}>
                  {label}
                </Typography>
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
        <Box component="main" sx={{ flex: 1, minWidth: 0, overflowY: "auto", display: "flex", flexDirection: "column" }}>
          {anomalyWarning && (
            <Alert
              severity="error"
              onClose={dismissAnomalyWarning}
              sx={{ borderRadius: 0, flexShrink: 0 }}
              action={
                <Button color="inherit" size="small" onClick={() => { dismissAnomalyWarning(); navigate("/dashboard/sessions"); }}>
                  {tCommon("viewSessions")}
                </Button>
              }
            >
              {tCommon("anomalyWarning")}
            </Alert>
          )}
          {newDeviceWarning && (
            <Alert
              severity="error"
              onClose={() => setNewDeviceWarning(null)}
              sx={{ borderRadius: 0, flexShrink: 0 }}
              action={
                <Button color="inherit" size="small" onClick={() => { setNewDeviceWarning(null); navigate("/dashboard/sessions"); }}>
                  {tCommon("viewSessions")}
                </Button>
              }
            >
              {tCommon("newDeviceWarning", { device: newDeviceWarning })}
            </Alert>
          )}
          {sessionEndedWarning && (
            <Alert
              severity="warning"
              onClose={() => setSessionEndedWarning(null)}
              sx={{ borderRadius: 0, flexShrink: 0 }}
              action={
                <Button color="inherit" size="small" onClick={() => { setSessionEndedWarning(null); navigate("/dashboard/sessions"); }}>
                  {tCommon("viewSessions")}
                </Button>
              }
            >
              {tCommon("sessionEndedWarning", { device: sessionEndedWarning })}
            </Alert>
          )}
          {localeWarning && (
            <Alert severity="warning" onClose={dismissLocaleWarning} sx={{ borderRadius: 0, flexShrink: 0 }}>
              {tCommon("localeChangedWarning")}
            </Alert>
          )}
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
      <SessionTimeoutDialog
        open={warningVisible}
        secondsRemaining={secondsRemaining}
        onExtend={extendSession}
        onLogout={handleLogout}
      />
    </MeContext.Provider>
  );
}
