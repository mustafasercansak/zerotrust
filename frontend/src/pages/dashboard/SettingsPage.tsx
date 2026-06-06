import { useEffect, useState, useRef } from "react";
import { useTranslation } from "react-i18next";
import { api, ApiError } from "@/lib/api";
import { useMeContext } from "@/contexts/MeContext";
import { DashboardPage } from "@/components/DashboardPage";
import SessionsPage from "./SessionsPage";
import { toast } from "sonner";
import Avatar from "@mui/material/Avatar";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import CircularProgress from "@mui/material/CircularProgress";
import Paper from "@mui/material/Paper";
import Tab from "@mui/material/Tab";
import Tabs from "@mui/material/Tabs";
import TextField from "@mui/material/TextField";
import Typography from "@mui/material/Typography";
import MenuItem from "@mui/material/MenuItem";
import Switch from "@mui/material/Switch";
import FormControlLabel from "@mui/material/FormControlLabel";

function initials(me: { email: string; first_name?: string; last_name?: string }): string {
  const parts = [me.first_name, me.last_name].filter((x): x is string => !!x);
  if (parts.length > 0) return parts.map((p) => p.charAt(0)).join("").slice(0, 2).toUpperCase();
  return me.email.slice(0, 2).toUpperCase();
}

export default function SettingsPage() {
  const { t } = useTranslation("settings");
  const { t: tProfile } = useTranslation("profile");
  const me = useMeContext();
  const isAdmin = me?.roles.includes("admin") ?? false;

  const [activeTab, setActiveTab] = useState(0);

  // Profile Settings State
  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");
  const [savingProfile, setSavingProfile] = useState(false);
  const [uploadingAvatar, setUploadingAvatar] = useState(false);
  const [avatarTimestamp, setAvatarTimestamp] = useState(Date.now());
  const fileInputRef = useRef<HTMLInputElement>(null);

  // System Settings State
  const [maxSessions, setMaxSessions] = useState("");
  const [passwordComplexity, setPasswordComplexity] = useState("low");
  const [globalMfaRequired, setGlobalMfaRequired] = useState("false");
  const [maxLoginAttempts, setMaxLoginAttempts] = useState("5");
  const [systemLoading, setSystemLoading] = useState(true);
  const [savingSystem, setSavingSystem] = useState(false);

  // Initialize Profile Settings Form
  useEffect(() => {
    if (me) {
      setFirstName(me.first_name ?? "");
      setLastName(me.last_name ?? "");
    }
  }, [me]);

  // Load System Settings if admin
  useEffect(() => {
    if (!isAdmin) return;
    api.admin.getSettings()
      .then((s) => {
        setMaxSessions(s["max_sessions_per_user"] ?? "5");
        setPasswordComplexity(s["password_complexity"] ?? "low");
        setGlobalMfaRequired(s["global_mfa_required"] ?? "false");
        setMaxLoginAttempts(s["max_login_attempts"] ?? "5");
      })
      .catch(() => toast.error(t("errors.internal_error")))
      .finally(() => setSystemLoading(false));
  }, [isAdmin]);

  async function handleProfileSave(e: React.FormEvent) {
    e.preventDefault();
    setSavingProfile(true);
    try {
      const updated = await api.updateProfile({ first_name: firstName, last_name: lastName });
      window.dispatchEvent(new CustomEvent("me:updated", { detail: updated }));
      toast.success(t("saved"));
    } catch (err) {
      const code = err instanceof ApiError ? err.message : "internal_error";
      toast.error(tProfile(`errors.${code}`, { defaultValue: tProfile("errors.internal_error") }));
    } finally {
      setSavingProfile(false);
    }
  }

  async function handleAvatarChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    if (file.size > 2 * 1024 * 1024) {
      toast.error(tProfile("errors.file_too_large"));
      return;
    }
    setUploadingAvatar(true);
    try {
      const updated = await api.uploadAvatar(file);
      window.dispatchEvent(new CustomEvent("me:updated", { detail: updated }));
      setAvatarTimestamp(Date.now());
    } catch (err) {
      const code = err instanceof ApiError ? err.message : "internal_error";
      toast.error(tProfile(`errors.${code}`, { defaultValue: tProfile("errors.internal_error") }));
    } finally {
      setUploadingAvatar(false);
    }
  }

  async function handleAvatarDelete() {
    setUploadingAvatar(true);
    try {
      const updated = await api.deleteAvatar();
      window.dispatchEvent(new CustomEvent("me:updated", { detail: updated }));
      setAvatarTimestamp(Date.now());
    } catch (err) {
      const code = err instanceof ApiError ? err.message : "internal_error";
      toast.error(tProfile(`errors.${code}`, { defaultValue: tProfile("errors.internal_error") }));
    } finally {
      setUploadingAvatar(false);
    }
  }

  async function runWithStepUp<T>(action: () => Promise<T>): Promise<T> {
    try {
      return await action();
    } catch (err) {
      if (!(err instanceof ApiError) || err.message !== "mfa_required") {
        throw err;
      }
      const code = window.prompt(t("mfaPrompt"))?.trim() ?? "";
      if (!code) {
        throw err;
      }
      await api.mfaStepUp(code);
      return action();
    }
  }

  async function handleSystemSave(e: React.FormEvent) {
    e.preventDefault();
    const n = parseInt(maxSessions, 10);
    if (isNaN(n) || n < 1 || n > 20) { toast.error(t("errors.invalid_value", { defaultValue: t("errors.internal_error") })); return; }
    const m = parseInt(maxLoginAttempts, 10);
    if (isNaN(m) || m < 1 || m > 20) { toast.error(t("errors.invalid_value", { defaultValue: t("errors.internal_error") })); return; }
    setSavingSystem(true);
    try {
      await runWithStepUp(() => api.admin.updateSettings({
        max_sessions_per_user: String(n),
        password_complexity: passwordComplexity,
        global_mfa_required: globalMfaRequired,
        max_login_attempts: String(m),
      }));
      toast.success(t("saved"));
    } catch (err) {
      const code = err instanceof ApiError ? err.message : "internal_error";
      toast.error(t(`errors.${code}`, { defaultValue: t("errors.internal_error") }));
    } finally {
      setSavingSystem(false);
    }
  }

  if (!me) {
    return (
      <Box sx={{ display: "flex", alignItems: "center", justifyContent: "center", height: "100%", py: 8 }}>
        <CircularProgress size={24} />
      </Box>
    );
  }

  return (
    <DashboardPage>
      <Box sx={{ width: "100%", display: "grid", gap: 3 }}>
        <Box sx={{ borderBottom: 1, borderColor: "divider" }}>
          <Tabs value={activeTab} onChange={(_, val: number) => setActiveTab(val)}>
            <Tab label={t("tabProfile")} id="tab-profile" />
            <Tab label={t("tabSecurity")} id="tab-security" />
            {isAdmin && <Tab label={t("tabSystem")} id="tab-system" />}
          </Tabs>
        </Box>

        {activeTab === 0 && (
          <Paper variant="outlined" component="form" onSubmit={handleProfileSave} sx={{ p: 4, display: "grid", gap: 3 }}>
            <Typography variant="h6" sx={{ fontWeight: 700 }}>{tProfile("title")}</Typography>
            <Box sx={{ display: "flex", alignItems: "center", gap: 3 }}>
              <Avatar
                src={me.has_avatar ? `/api/v1/users/${me.user_id}/avatar?t=${avatarTimestamp}` : undefined}
                sx={{ width: 80, height: 80, fontSize: 28 }}
              >
                {initials(me)}
              </Avatar>
              <Box sx={{ display: "flex", gap: 1.5 }}>
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
            <Box>
              <Button type="submit" variant="contained" disabled={savingProfile} sx={{ minWidth: 120 }}>
                {savingProfile ? tProfile("saving") : t("save")}
              </Button>
            </Box>
          </Paper>
        )}

        {activeTab === 1 && (
          <Box>
            <SessionsPage />
          </Box>
        )}

        {activeTab === 2 && isAdmin && (
          <Paper variant="outlined" component="form" onSubmit={handleSystemSave} sx={{ p: 4, display: "grid", gap: 3 }}>
            <Typography variant="h6" sx={{ fontWeight: 700 }}>{t("title")}</Typography>
            {systemLoading ? (
              <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
                <CircularProgress size={18} />
                <Typography variant="body2" color="text.secondary">{t("loading")}</Typography>
              </Box>
            ) : (
              <Box sx={{ display: "grid", gap: 3.5 }}>
                {/* Max Sessions */}
                <Box sx={{ display: "grid", gap: 1 }}>
                  <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>{t("maxSessions")}</Typography>
                  <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>{t("maxSessionsDesc")}</Typography>
                  <TextField
                    label={t("maxSessionsInput")}
                    type="number"
                    value={maxSessions}
                    onChange={(e) => setMaxSessions(e.target.value)}
                    sx={{ width: 160 }}
                    slotProps={{ htmlInput: { min: 1, max: 20 } }}
                  />
                </Box>

                {/* Password Complexity */}
                <Box sx={{ display: "grid", gap: 1 }}>
                  <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>{t("passwordComplexity")}</Typography>
                  <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>{t("passwordComplexityDesc")}</Typography>
                  <TextField
                    select
                    label={t("passwordComplexityInput")}
                    value={passwordComplexity}
                    onChange={(e) => setPasswordComplexity(e.target.value)}
                    sx={{ width: 320 }}
                  >
                    <MenuItem value="low">{t("passwordComplexityLow")}</MenuItem>
                    <MenuItem value="medium">{t("passwordComplexityMedium")}</MenuItem>
                    <MenuItem value="strong">{t("passwordComplexityStrong")}</MenuItem>
                  </TextField>
                </Box>

                {/* Max Login Attempts */}
                <Box sx={{ display: "grid", gap: 1 }}>
                  <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>{t("maxLoginAttempts")}</Typography>
                  <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>{t("maxLoginAttemptsDesc")}</Typography>
                  <TextField
                    label={t("maxLoginAttemptsInput")}
                    type="number"
                    value={maxLoginAttempts}
                    onChange={(e) => setMaxLoginAttempts(e.target.value)}
                    sx={{ width: 160 }}
                    slotProps={{ htmlInput: { min: 1, max: 20 } }}
                  />
                </Box>

                {/* Global MFA */}
                <Box sx={{ display: "grid", gap: 1 }}>
                  <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>{t("globalMfaRequired")}</Typography>
                  <Typography variant="body2" color="text.secondary">{t("globalMfaRequiredDesc")}</Typography>
                  <FormControlLabel
                    control={
                      <Switch
                        checked={globalMfaRequired === "true"}
                        onChange={(e) => setGlobalMfaRequired(e.target.checked ? "true" : "false")}
                        color="primary"
                      />
                    }
                    label={t("globalMfaRequired")}
                    sx={{ mt: 1 }}
                  />
                </Box>

                <Box sx={{ mt: 1 }}>
                  <Button type="submit" variant="contained" disabled={savingSystem} sx={{ minWidth: 120 }}>
                    {savingSystem ? t("saving") : t("save")}
                  </Button>
                </Box>
              </Box>
            )}
          </Paper>
        )}
      </Box>
    </DashboardPage>
  );
}
