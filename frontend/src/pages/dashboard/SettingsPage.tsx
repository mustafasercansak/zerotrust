import { useEffect, useState, useRef } from "react";
import { useTranslation } from "react-i18next";
import { api, ApiError, type AuditEntry } from "@/lib/api";
import { useMeContext } from "@/contexts/MeContext";
import { DashboardPage } from "@/components/DashboardPage";
import SessionsPage from "./SessionsPage";
import { useStepUp } from "@/hooks/useStepUp";
import { StepUpMfaDialog } from "@/components/StepUpMfaDialog";
import { PasswordStrengthBar } from "@/components/PasswordStrengthBar";
import { toast } from "sonner";
import Avatar from "@mui/material/Avatar";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import CircularProgress from "@mui/material/CircularProgress";
import Paper from "@mui/material/Paper";
import Tab from "@mui/material/Tab";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
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

const ACTION_LABELS: Record<string, string> = {
  "auth.login": "Login",
  "auth.logout": "Logout",
  "auth.refresh": "Token Refresh",
  "auth.register": "Registration",
  "auth.password_reset": "Password Reset",
  "user.password_changed": "Password Changed",
  "user.locale_changed": "Language Changed",
  "user.profile_updated": "Profile Updated",
  "user.avatar_uploaded": "Avatar Uploaded",
  "user.avatar_deleted": "Avatar Deleted",
  "user.notify_preference_updated": "Notification Preference Updated",
  "mfa.setup": "2FA Setup",
  "mfa.verify": "2FA Verified",
  "mfa.disabled": "2FA Disabled",
  "session.revoked": "Session Revoked",
  "session.revoke_all": "All Sessions Revoked",
  "login.anomaly": "Login Anomaly Detected",
};

function formatAction(action: string): string {
  return ACTION_LABELS[action] ?? action.replace(/[._]/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}

function formatDateTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });
  } catch {
    return iso;
  }
}

export default function SettingsPage() {
  const { t } = useTranslation("settings");
  const { t: tProfile } = useTranslation("profile");
  const { i18n } = useTranslation();
  const me = useMeContext();
  const isAdmin = me?.roles.includes("admin") ?? false;

  const { runWithStepUp, stepUpOpen, stepUpError, stepUpSubmitting, handleStepUpSubmit, handleStepUpClose } = useStepUp();

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

  // Change Password State (appended after system settings to preserve test state indices)
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [changingPassword, setChangingPassword] = useState(false);

  // Session Timeout State (appended last to preserve test state indices)
  const [idleTimeout, setIdleTimeout] = useState("300");
  const [adminIdleTimeout, setAdminIdleTimeout] = useState("180");
  const [absoluteTimeout, setAbsoluteTimeout] = useState("28800");

  // Notification preferences — initialised from the /me response
  const [notifySecurityEmails, setNotifySecurityEmails] = useState(true);
  const [savingNotify, setSavingNotify] = useState(false);

  // Activity / login history (appended last to preserve test state indices)
  const [historyPage, setHistoryPage] = useState(0);
  const [historyEntries, setHistoryEntries] = useState<AuditEntry[]>([]);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyTotal, setHistoryTotal] = useState(0);

  // Hardware attestation state (appended last to preserve test state indices)
  const [requireHardwareAttestation, setRequireHardwareAttestation] = useState("false");

  // Webhook alerts state (appended last to preserve test state indices)
  const [webhookEnabled, setWebhookEnabled] = useState("false");
  const [webhookUrl, setWebhookUrl] = useState("");
  const [testingWebhook, setTestingWebhook] = useState(false);

  // IP allowlist (appended last to preserve test state indices)
  const [ipAllowlist, setIpAllowlist] = useState("");

  // Country allowlist (appended last to preserve test state indices)
  const [countryAllowlist, setCountryAllowlist] = useState("");

  useEffect(() => {
    if (me) setNotifySecurityEmails(me.notify_security_emails ?? true);
  }, [me]);

  const activityTabIndex = isAdmin ? 3 : 2;

  useEffect(() => {
    if (activeTab !== activityTabIndex) return;
    let cancelled = false;
    setHistoryLoading(true);
    api.listMyAudit(25, historyPage * 25)
      .then(({ data, total }) => {
        if (cancelled) return;
        setHistoryEntries(data);
        setHistoryTotal(total);
      })
      .catch(() => {})
      .finally(() => { if (!cancelled) setHistoryLoading(false); });
    return () => { cancelled = true; };
  }, [activeTab, activityTabIndex, historyPage]);

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
        setRequireHardwareAttestation(s["require_hardware_attestation"] ?? "false");
        setWebhookEnabled(s["webhook_enabled"] ?? "false");
        setWebhookUrl(s["webhook_url"] ?? "");
        setIpAllowlist(s["ip_allowlist"] ?? "");
        setCountryAllowlist(s["country_allowlist"] ?? "");
        setMaxLoginAttempts(s["max_login_attempts"] ?? "5");
        setIdleTimeout(s["session_idle_timeout_seconds"] ?? "300");
        setAdminIdleTimeout(s["session_idle_timeout_seconds_admin"] ?? "180");
        setAbsoluteTimeout(s["session_absolute_timeout_seconds"] ?? "28800");
      })
      .catch(() => toast.error(t("errors.internal_error")))
      .finally(() => setSystemLoading(false));
  }, [isAdmin, t]);

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



  async function handlePasswordChange(e: React.FormEvent) {
    e.preventDefault();
    const tPw = (key: string) => tProfile(`changePassword.errors.${key}`, { defaultValue: tProfile("changePassword.errors.internal_error") });
    if (newPassword !== confirmPassword) {
      toast.error(tProfile("changePassword.errors.passwords_do_not_match"));
      return;
    }
    setChangingPassword(true);
    try {
      await api.changePassword(currentPassword, newPassword);
      toast.success(tProfile("changePassword.success"));
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
    } catch (err) {
      const code = err instanceof ApiError ? err.message : "internal_error";
      toast.error(tPw(code));
    } finally {
      setChangingPassword(false);
    }
  }

  async function handleLocaleChange(locale: "tr" | "en") {
    if (i18n.language.startsWith(locale)) return;
    await api.updateLocale(locale).catch(() => {});
    i18n.changeLanguage(locale);
    globalThis.localStorage?.setItem("locale", locale);
    toast.success(t("localeSaved"));
  }

  async function handleNotifyToggle(enabled: boolean) {
    setNotifySecurityEmails(enabled);
    setSavingNotify(true);
    try {
      await api.updateNotifications(enabled);
      toast.success(t("notifySecurityEmailsSaved"));
    } catch {
      setNotifySecurityEmails(!enabled);
      toast.error(t("errors.internal_error"));
    } finally {
      setSavingNotify(false);
    }
  }

  const numericProps = (min: number, max: number) => ({
    min, max,
    onInvalid: (e: React.FormEvent<HTMLInputElement>) => {
      (e.target as HTMLInputElement).setCustomValidity(t("errors.invalid_value"));
    },
    onInput: (e: React.FormEvent<HTMLInputElement>) => {
      (e.target as HTMLInputElement).setCustomValidity("");
    },
  });

  async function handleTestWebhook() {
    if (!webhookUrl) {
      toast.info(t("webhookTestNoUrl"));
      return;
    }
    setTestingWebhook(true);
    try {
      await api.admin.testWebhook(webhookUrl);
      toast.success(t("webhookTestSuccess"));
    } catch {
      toast.error(t("webhookTestFailed"));
    } finally {
      setTestingWebhook(false);
    }
  }

  async function handleSystemSave(e: React.FormEvent) {
    e.preventDefault();
    const n = parseInt(maxSessions, 10);
    if (isNaN(n) || n < 1 || n > 20) { toast.error(t("errors.invalid_value", { defaultValue: t("errors.internal_error") })); return; }
    const m = parseInt(maxLoginAttempts, 10);
    if (isNaN(m) || m < 1 || m > 20) { toast.error(t("errors.invalid_value", { defaultValue: t("errors.internal_error") })); return; }
    const idle = parseInt(idleTimeout, 10);
    if (isNaN(idle) || idle < 60 || idle > 3600) { toast.error(t("errors.invalid_value", { defaultValue: t("errors.internal_error") })); return; }
    const adminIdle = parseInt(adminIdleTimeout, 10);
    if (isNaN(adminIdle) || adminIdle < 60 || adminIdle > 1800) { toast.error(t("errors.invalid_value", { defaultValue: t("errors.internal_error") })); return; }
    const absolute = parseInt(absoluteTimeout, 10);
    if (isNaN(absolute) || absolute < 1800 || absolute > 172800) { toast.error(t("errors.invalid_value", { defaultValue: t("errors.internal_error") })); return; }
    setSavingSystem(true);
    try {
      await runWithStepUp(() => api.admin.updateSettings({
        max_sessions_per_user: String(n),
        password_complexity: passwordComplexity,
        global_mfa_required: globalMfaRequired,
        require_hardware_attestation: requireHardwareAttestation,
        webhook_enabled: webhookEnabled,
        webhook_url: webhookUrl,
        ip_allowlist: ipAllowlist,
        country_allowlist: countryAllowlist,
        max_login_attempts: String(m),
        session_idle_timeout_seconds: String(idle),
        session_idle_timeout_seconds_admin: String(adminIdle),
        session_absolute_timeout_seconds: String(absolute),
      }), "settings_update");
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
      <Box sx={{ width: "100%", height: "100%", display: "flex", flexDirection: "column", minHeight: 0, gap: 2 }}>
        <Box sx={{ borderBottom: 1, borderColor: "divider" }}>
          <Tabs value={activeTab} onChange={(_, val: number) => setActiveTab(val)}>
            <Tab label={t("tabProfile")} id="tab-profile" data-testid="tab-profile-settings" />
            <Tab label={t("tabSecurity")} id="tab-security" data-testid="tab-security-sessions" />
            {isAdmin && <Tab label={t("tabSystem")} id="tab-system" data-testid="tab-system-settings" />}
            <Tab label={t("tabActivity")} id="tab-activity" data-testid="tab-login-activity" />
          </Tabs>
        </Box>

        {activeTab === 0 && (
          <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "1fr 320px" }, gap: 2, alignItems: "start" }}>
            {/* Left column: profile info + password */}
            <Box sx={{ display: "grid", gap: 2 }}>
              <Paper variant="outlined" component="form" onSubmit={handleProfileSave} sx={{ p: 3, display: "grid", gap: 2 }}>
                <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
                  <Avatar
                    src={me.has_avatar ? `/api/v1/users/${me.user_id}/avatar?t=${avatarTimestamp}` : undefined}
                    sx={{ width: 64, height: 64, fontSize: 22, flexShrink: 0 }}
                  >
                    {initials(me)}
                  </Avatar>
                  <Box>
                    <Typography variant="h6" sx={{ fontWeight: 700, lineHeight: 1.2 }}>{tProfile("title")}</Typography>
                    <Box sx={{ display: "flex", gap: 1, mt: 0.75 }}>
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
                </Box>
                <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 2 }}>
                  <TextField label={tProfile("firstName")} value={firstName} onChange={(e) => setFirstName(e.target.value)} slotProps={{ htmlInput: { maxLength: 80, "data-testid": "settings-first-name" } }} />
                  <TextField label={tProfile("lastName")} value={lastName} onChange={(e) => setLastName(e.target.value)} slotProps={{ htmlInput: { maxLength: 80, "data-testid": "settings-last-name" } }} />
                </Box>
                <Box>
                  <Button type="submit" variant="contained" disabled={savingProfile} sx={{ minWidth: 120 }} data-testid="settings-profile-save">
                    {savingProfile ? tProfile("saving") : t("save")}
                  </Button>
                </Box>
              </Paper>

              <Paper variant="outlined" component="form" onSubmit={handlePasswordChange} sx={{ p: 3, display: "grid", gap: 2 }}>
                <Typography variant="h6" sx={{ fontWeight: 700 }}>{tProfile("changePassword.title")}</Typography>
                <TextField
                  label={tProfile("changePassword.currentPassword")}
                  type="password"
                  value={currentPassword}
                  onChange={(e) => setCurrentPassword(e.target.value)}
                  fullWidth
                  slotProps={{ htmlInput: { "data-testid": "settings-current-password" } }}
                />
                <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 2 }}>
                  <Box>
                    <TextField
                      label={tProfile("changePassword.newPassword")}
                      type="password"
                      value={newPassword}
                      onChange={(e) => setNewPassword(e.target.value)}
                      fullWidth
                      slotProps={{ htmlInput: { "data-testid": "settings-new-password" } }}
                    />
                    <PasswordStrengthBar password={newPassword} />
                  </Box>
                  <TextField
                    label={tProfile("changePassword.confirmPassword")}
                    type="password"
                    value={confirmPassword}
                    onChange={(e) => setConfirmPassword(e.target.value)}
                    fullWidth
                    slotProps={{ htmlInput: { "data-testid": "settings-confirm-password" } }}
                  />
                </Box>
                <Box>
                  <Button type="submit" variant="outlined" disabled={changingPassword} sx={{ minWidth: 160 }}>
                    {changingPassword ? tProfile("changePassword.submitting") : tProfile("changePassword.submit")}
                  </Button>
                </Box>
              </Paper>
            </Box>

            {/* Right column: preferences */}
            <Box sx={{ display: "grid", gap: 2 }}>
              <Paper variant="outlined" sx={{ p: 3, display: "grid", gap: 1.5 }}>
                <Typography variant="subtitle1" sx={{ fontWeight: 700 }}>{t("localeSection")}</Typography>
                <Typography variant="body2" color="text.secondary">{t("localeDesc")}</Typography>
                <TextField
                  select
                  label={t("localeLabel")}
                  value={i18n.language.startsWith("tr") ? "tr" : "en"}
                  onChange={(e) => handleLocaleChange(e.target.value as "tr" | "en")}
                  fullWidth
                  size="small"
                >
                  <MenuItem value="en">English</MenuItem>
                  <MenuItem value="tr">Türkçe</MenuItem>
                </TextField>
              </Paper>

              <Paper variant="outlined" sx={{ p: 3, display: "grid", gap: 1.5 }}>
                <Typography variant="subtitle1" sx={{ fontWeight: 700 }}>{t("notificationsSection")}</Typography>
                <Typography variant="body2" color="text.secondary">{t("notificationsDesc")}</Typography>
                <FormControlLabel
                  control={
                    <Switch
                      checked={notifySecurityEmails}
                      onChange={(e) => handleNotifyToggle(e.target.checked)}
                      disabled={savingNotify}
                      color="primary"
                    />
                  }
                  label={t("notifySecurityEmails")}
                />
              </Paper>
            </Box>
          </Box>
        )}

        {activeTab === 1 && (
          <Box sx={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "column" }}>
            <SessionsPage />
          </Box>
        )}

        {activeTab === activityTabIndex && (
          <Paper variant="outlined" sx={{ overflow: "hidden" }} data-testid="activity-section">
            <Box sx={{ px: 3, pt: 2.5, pb: 1.5, display: "flex", alignItems: "center", justifyContent: "space-between" }}>
              <Box>
                <Typography variant="h6" sx={{ fontWeight: 700 }}>{t("activityTitle")}</Typography>
                <Typography variant="body2" color="text.secondary">{t("activityDesc")}</Typography>
              </Box>
              {historyTotal > 0 && (
                <Typography variant="caption" color="text.secondary">
                  {t("activityCount", { from: historyPage * 25 + 1, to: Math.min((historyPage + 1) * 25, historyTotal), total: historyTotal })}
                </Typography>
              )}
            </Box>
            {historyLoading ? (
              <Box sx={{ display: "flex", justifyContent: "center", py: 4 }}>
                <CircularProgress size={24} />
              </Box>
            ) : historyEntries.length === 0 ? (
              <Box sx={{ py: 4, textAlign: "center" }} data-testid="activity-empty-state">
                <Typography variant="body2" color="text.secondary">{t("activityEmpty")}</Typography>
              </Box>
            ) : (
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <TableCell sx={{ fontWeight: 700, whiteSpace: "nowrap" }}>{t("activityColTime")}</TableCell>
                    <TableCell sx={{ fontWeight: 700 }}>{t("activityColEvent")}</TableCell>
                    <TableCell sx={{ fontWeight: 700 }}>{t("activityColLocation")}</TableCell>
                    <TableCell sx={{ fontWeight: 700 }}>{t("activityColDevice")}</TableCell>
                    <TableCell sx={{ fontWeight: 700 }}>{t("activityColOutcome")}</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {historyEntries.map((entry) => {
                    const outcome = entry.metadata?.outcome;
                    const loc = entry.metadata?.location;
                    const ci = entry.metadata?.client_info;
                    const locationStr = [loc?.city, loc?.country].filter(Boolean).join(", ") || entry.ip_address || "—";
                    const deviceStr = ci ? [ci.browser, ci.os].filter(Boolean).join(" / ") : (entry.user_agent?.slice(0, 40) || "—");
                    return (
                      <TableRow key={entry.id} hover>
                        <TableCell sx={{ whiteSpace: "nowrap", color: "text.secondary", fontSize: 12 }}>
                          {formatDateTime(entry.created_at)}
                        </TableCell>
                        <TableCell sx={{ fontWeight: 500 }}>{formatAction(entry.action)}</TableCell>
                        <TableCell sx={{ color: "text.secondary", fontSize: 12 }}>{locationStr}</TableCell>
                        <TableCell sx={{ color: "text.secondary", fontSize: 12, maxWidth: 200, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{deviceStr}</TableCell>
                        <TableCell>
                          {outcome ? (
                            <Chip
                              size="small"
                              label={outcome}
                              color={outcome === "success" ? "success" : "error"}
                              sx={{ height: 20, fontSize: 11, "& .MuiChip-label": { px: 0.75 } }}
                            />
                          ) : <Typography variant="caption" color="text.disabled">—</Typography>}
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            )}
            {historyTotal > 25 && (
              <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1, px: 2, py: 1.5, borderTop: 1, borderColor: "divider" }}>
                <Button size="small" disabled={historyPage === 0} onClick={() => setHistoryPage((p) => p - 1)}>
                  {t("activityPrev")}
                </Button>
                <Button size="small" disabled={(historyPage + 1) * 25 >= historyTotal} onClick={() => setHistoryPage((p) => p + 1)}>
                  {t("activityNext")}
                </Button>
              </Box>
            )}
          </Paper>
        )}

        {activeTab === 2 && isAdmin && (
          <Paper variant="outlined" component="form" onSubmit={handleSystemSave} sx={{ p: 3, display: "grid", gap: 2.5 }}>
            <Typography variant="h6" sx={{ fontWeight: 700 }}>{t("title")}</Typography>
            {systemLoading ? (
              <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
                <CircularProgress size={18} />
                <Typography variant="body2" color="text.secondary">{t("loading")}</Typography>
              </Box>
            ) : (
              <Box sx={{ display: "grid", gap: 2.5 }}>
                {/* Row 1: Max Sessions | Max Login Attempts */}
                <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 2 }}>
                  <Box sx={{ display: "grid", gap: 0.75 }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>{t("maxSessions")}</Typography>
                    <Typography variant="body2" color="text.secondary">{t("maxSessionsDesc")}</Typography>
                    <TextField
                      label={t("maxSessionsInput")}
                      type="number"
                      value={maxSessions}
                      onChange={(e) => setMaxSessions(e.target.value)}
                      sx={{ width: 140, mt: 0.5 }}
                      size="small"
                      slotProps={{ htmlInput: { ...numericProps(1, 20), "data-testid": "settings-max-sessions" } }}
                    />
                  </Box>
                  <Box sx={{ display: "grid", gap: 0.75 }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>{t("maxLoginAttempts")}</Typography>
                    <Typography variant="body2" color="text.secondary">{t("maxLoginAttemptsDesc")}</Typography>
                    <TextField
                      label={t("maxLoginAttemptsInput")}
                      type="number"
                      value={maxLoginAttempts}
                      onChange={(e) => setMaxLoginAttempts(e.target.value)}
                      sx={{ width: 140, mt: 0.5 }}
                      size="small"
                      slotProps={{ htmlInput: { ...numericProps(1, 20), "data-testid": "settings-max-login-attempts" } }}
                    />
                  </Box>
                </Box>

                {/* Row 2: Session Timeouts (3 cols) */}
                <Box sx={{ display: "grid", gap: 0.75 }}>
                  <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>{t("sessionTimeouts")}</Typography>
                  <Typography variant="body2" color="text.secondary">{t("sessionTimeoutsDesc")}</Typography>
                  <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "repeat(3, 1fr)" }, gap: 2, mt: 0.5 }}>
                    <TextField
                      label={t("idleTimeoutInput")}
                      type="number"
                      value={idleTimeout}
                      onChange={(e) => setIdleTimeout(e.target.value)}
                      slotProps={{ htmlInput: numericProps(60, 3600) }}
                      helperText={t("idleTimeoutHint")}
                      size="small"
                    />
                    <TextField
                      label={t("adminIdleTimeoutInput")}
                      type="number"
                      value={adminIdleTimeout}
                      onChange={(e) => setAdminIdleTimeout(e.target.value)}
                      slotProps={{ htmlInput: numericProps(60, 1800) }}
                      helperText={t("adminIdleTimeoutHint")}
                      size="small"
                    />
                    <TextField
                      label={t("absoluteTimeoutInput")}
                      type="number"
                      value={absoluteTimeout}
                      onChange={(e) => setAbsoluteTimeout(e.target.value)}
                      slotProps={{ htmlInput: numericProps(1800, 172800) }}
                      helperText={t("absoluteTimeoutHint")}
                      size="small"
                    />
                  </Box>
                </Box>

                {/* Row 3: Password Complexity | Global MFA */}
                <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 2 }}>
                  <Box sx={{ display: "grid", gap: 0.75 }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>{t("passwordComplexity")}</Typography>
                    <Typography variant="body2" color="text.secondary">{t("passwordComplexityDesc")}</Typography>
                    <TextField
                      select
                      label={t("passwordComplexityInput")}
                      value={passwordComplexity}
                      onChange={(e) => setPasswordComplexity(e.target.value)}
                      size="small"
                      sx={{ mt: 0.5 }}
                      fullWidth
                    >
                      <MenuItem value="low">{t("passwordComplexityLow")}</MenuItem>
                      <MenuItem value="medium">{t("passwordComplexityMedium")}</MenuItem>
                      <MenuItem value="strong">{t("passwordComplexityStrong")}</MenuItem>
                    </TextField>
                  </Box>
                  <Box sx={{ display: "grid", gap: 0.75 }}>
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
                      sx={{ mt: 0.5 }}
                    />
                  </Box>
                </Box>

                {/* Row 4: Require Hardware Attestation */}
                <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 2 }}>
                  <Box sx={{ display: "grid", gap: 0.75 }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>{t("requireHardwareAttestation")}</Typography>
                    <Typography variant="body2" color="text.secondary">{t("requireHardwareAttestationDesc")}</Typography>
                    <FormControlLabel
                      control={
                        <Switch
                          checked={requireHardwareAttestation === "true"}
                          onChange={(e) => setRequireHardwareAttestation(e.target.checked ? "true" : "false")}
                          color="primary"
                        />
                      }
                      label={t("requireHardwareAttestation")}
                      sx={{ mt: 0.5 }}
                    />
                  </Box>
                </Box>

                {/* Row 5: IP Allowlist */}
                <Box sx={{ display: "grid", gap: 0.75 }}>
                  <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>{t("ipAllowlist")}</Typography>
                  <Typography variant="body2" color="text.secondary">{t("ipAllowlistDesc")}</Typography>
                  <TextField
                    multiline
                    minRows={3}
                    maxRows={8}
                    label={t("ipAllowlistInput")}
                    value={ipAllowlist}
                    onChange={(e) => setIpAllowlist(e.target.value)}
                    placeholder={t("ipAllowlistPlaceholder")}
                    size="small"
                    fullWidth
                    sx={{ mt: 0.5, fontFamily: "monospace" }}
                    slotProps={{ htmlInput: { style: { fontFamily: "monospace", fontSize: 13 }, "data-testid": "settings-ip-allowlist" } }}
                    helperText={t("ipAllowlistHint")}
                  />
                </Box>

                {/* Row 6: Country Allowlist */}
                <Box sx={{ display: "grid", gap: 0.75 }}>
                  <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>{t("countryAllowlist")}</Typography>
                  <Typography variant="body2" color="text.secondary">{t("countryAllowlistDesc")}</Typography>
                  <TextField
                    label={t("countryAllowlistInput")}
                    value={countryAllowlist}
                    onChange={(e) => setCountryAllowlist(e.target.value)}
                    placeholder={t("countryAllowlistPlaceholder")}
                    size="small"
                    fullWidth
                    sx={{ mt: 0.5, fontFamily: "monospace" }}
                    slotProps={{ htmlInput: { style: { fontFamily: "monospace", fontSize: 13 }, "data-testid": "settings-country-allowlist" } }}
                    helperText={t("countryAllowlistHint")}
                  />
                </Box>

                {/* Row 7: Webhook Alerts */}
                <Box sx={{ display: "grid", gap: 0.75 }}>
                  <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>{t("webhookAlerts")}</Typography>
                  <Typography variant="body2" color="text.secondary">{t("webhookAlertsDesc")}</Typography>
                  <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "auto 1fr" }, gap: 3, alignItems: "center", mt: 0.5 }}>
                    <FormControlLabel
                      control={
                        <Switch
                          checked={webhookEnabled === "true"}
                          onChange={(e) => setWebhookEnabled(e.target.checked ? "true" : "false")}
                          color="primary"
                        />
                      }
                      label={t("webhookEnabled")}
                    />
                    <Box sx={{ display: "flex", gap: 1, alignItems: "flex-start" }}>
                      <TextField
                        label={t("webhookUrlInput")}
                        type="url"
                        value={webhookUrl}
                        onChange={(e) => setWebhookUrl(e.target.value)}
                        disabled={webhookEnabled !== "true"}
                        placeholder={t("webhookUrlPlaceholder")}
                        size="small"
                        fullWidth
                        slotProps={{ htmlInput: { "data-testid": "settings-webhook-url" } }}
                      />
                      <Button
                        variant="outlined"
                        size="small"
                        onClick={handleTestWebhook}
                        disabled={testingWebhook || webhookEnabled !== "true" || !webhookUrl}
                        sx={{ whiteSpace: "nowrap", mt: "1px", height: 40 }}
                        data-testid="settings-webhook-test"
                      >
                        {testingWebhook ? t("webhookTesting") : t("webhookTest")}
                      </Button>
                    </Box>
                  </Box>
                </Box>

                <Box>
                  <Button type="submit" variant="contained" disabled={savingSystem} sx={{ minWidth: 120 }} data-testid="settings-system-save">
                    {savingSystem ? t("saving") : t("save")}
                  </Button>
                </Box>
              </Box>
            )}
          </Paper>
        )}
      </Box>
      <StepUpMfaDialog
        open={stepUpOpen}
        error={stepUpError}
        loading={stepUpSubmitting}
        onSubmit={handleStepUpSubmit}
        onClose={handleStepUpClose}
      />
    </DashboardPage>
  );
}
