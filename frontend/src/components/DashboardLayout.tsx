import { Link, Outlet, useLocation, useNavigate } from "react-router-dom";
import { useState, Suspense, useRef } from "react";
import { useTranslation } from "react-i18next";
import { useAuth } from "@/lib/useAuth";
import { cancelRefresh } from "@/lib/tokenManager";
import { api, ApiError, type MeData } from "@/lib/api";
import { MeContext } from "@/contexts/MeContext";
import Alert from "@mui/material/Alert";
import Avatar from "@mui/material/Avatar";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import CircularProgress from "@mui/material/CircularProgress";
import Divider from "@mui/material/Divider";
import Dialog from "@mui/material/Dialog";
import DialogActions from "@mui/material/DialogActions";
import DialogContent from "@mui/material/DialogContent";
import DialogTitle from "@mui/material/DialogTitle";
import List from "@mui/material/List";
import ListItemButton from "@mui/material/ListItemButton";
import TextField from "@mui/material/TextField";
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
  const { t: tProfile } = useTranslation("profile");
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const { i18n } = useTranslation();
  const { me, setMe, loading, bootstrapError, retry } = useAuth();
  const [profileOpen, setProfileOpen] = useState(false);
  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");
  const [profileError, setProfileError] = useState<string | null>(null);
  const [savingProfile, setSavingProfile] = useState(false);
  const [uploadingAvatar, setUploadingAvatar] = useState(false);
  const [avatarTimestamp, setAvatarTimestamp] = useState(Date.now());
  const fileInputRef = useRef<HTMLInputElement>(null);

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
    { to: "/dashboard/sessions", label: t("sessions") },
    { to: "/dashboard/mfa", label: t("mfa") },
    ...(isAdmin
      ? [
          { to: "/dashboard/users", label: t("users") },
          { to: "/dashboard/audit", label: t("audit") },
          { to: "/dashboard/service-accounts", label: t("serviceAccounts") },
          { to: "/dashboard/settings", label: t("settings") },
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
    if (!me) return;
    setFirstName(me.first_name ?? "");
    setLastName(me.last_name ?? "");
    setProfileError(null);
    setProfileOpen(true);
  }

  async function handleProfileSave(e: React.FormEvent) {
    e.preventDefault();
    setSavingProfile(true);
    setProfileError(null);
    try {
      const updated = await api.updateProfile({ first_name: firstName, last_name: lastName });
      setMe(updated);
      setProfileOpen(false);
    } catch (err) {
      const code = err instanceof ApiError ? err.message : "internal_error";
      setProfileError(tProfile(`errors.${code}`, { defaultValue: tProfile("errors.internal_error") }));
    } finally {
      setSavingProfile(false);
    }
  }

  async function handleAvatarChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    if (file.size > 2 * 1024 * 1024) {
      setProfileError(tProfile("errors.file_too_large"));
      return;
    }
    setUploadingAvatar(true);
    setProfileError(null);
    try {
      const updated = await api.uploadAvatar(file);
      setMe(updated);
      setAvatarTimestamp(Date.now());
    } catch (err) {
      const code = err instanceof ApiError ? err.message : "internal_error";
      setProfileError(tProfile(`errors.${code}`, { defaultValue: tProfile("errors.internal_error") }));
    } finally {
      setUploadingAvatar(false);
    }
  }

  async function handleAvatarDelete() {
    setUploadingAvatar(true);
    setProfileError(null);
    try {
      const updated = await api.deleteAvatar();
      setMe(updated);
      setAvatarTimestamp(Date.now());
    } catch (err) {
      const code = err instanceof ApiError ? err.message : "internal_error";
      setProfileError(tProfile(`errors.${code}`, { defaultValue: tProfile("errors.internal_error") }));
    } finally {
      setUploadingAvatar(false);
    }
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
        <Dialog open={profileOpen} onClose={() => setProfileOpen(false)} fullWidth maxWidth="xs">
          <Box component="form" onSubmit={handleProfileSave}>
            <DialogTitle>{tProfile("title")}</DialogTitle>
            <DialogContent sx={{ display: "grid", gap: 2, pt: 1 }}>
              <Box sx={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 1.5, mb: 1 }}>
                <Avatar
                  src={me.has_avatar ? `/api/v1/users/${me.user_id}/avatar?t=${avatarTimestamp}` : undefined}
                  sx={{ width: 80, height: 80, fontSize: 28 }}
                >
                  {initials(me)}
                </Avatar>
                <Box sx={{ display: "flex", gap: 1 }}>
                  <Button variant="outlined" size="small" component="label" disabled={uploadingAvatar}>
                    {tProfile("uploadButton")}
                    <input type="file" hidden accept="image/png, image/jpeg" onChange={handleAvatarChange} ref={fileInputRef} />
                  </Button>
                  {me.has_avatar && (
                    <Button variant="outlined" size="small" color="error" onClick={handleAvatarDelete} disabled={uploadingAvatar}>
                      {tProfile("deleteButton")}
                    </Button>
                  )}
                </Box>
              </Box>
              <TextField label={tProfile("firstName")} value={firstName} onChange={(e) => setFirstName(e.target.value)} slotProps={{ htmlInput: { maxLength: 80 } }} fullWidth />
              <TextField label={tProfile("lastName")} value={lastName} onChange={(e) => setLastName(e.target.value)} slotProps={{ htmlInput: { maxLength: 80 } }} fullWidth />
              {profileError && <Alert severity="error">{profileError}</Alert>}
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 3 }}>
              <Button onClick={() => setProfileOpen(false)}>{tCommon("cancel")}</Button>
              <Button type="submit" variant="contained" disabled={savingProfile}>
                {savingProfile ? tProfile("saving") : tCommon("save")}
              </Button>
            </DialogActions>
          </Box>
        </Dialog>
      </Box>
    </MeContext.Provider>
  );
}
