import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, ApiError, type WebAuthnCredential } from "@/lib/api";
import { performRegistration, isWebAuthnSupported } from "@/lib/webauthn";
import { toast } from "sonner";
import Paper from "@mui/material/Paper";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Typography from "@mui/material/Typography";
import Divider from "@mui/material/Divider";
import Alert from "@mui/material/Alert";
import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";
import Fingerprint from "@mui/icons-material/Fingerprint";
import DeleteOutline from "@mui/icons-material/DeleteOutlined";
import Edit from "@mui/icons-material/Edit";

function getAuthenticatorName(aaguid?: string): string | null {
  if (!aaguid) return null;
  const common: Record<string, string> = {
    "ad9a0119-7427-47d6-841c-f26440049db8": "YubiKey 5 Series",
    "2fc0579f-818a-4990-843e-055d288d6896": "YubiKey 5C Series",
    "f8a011f3-8c53-41c5-b46b-bd34fd7c1400": "YubiKey 5 Series",
    "39a11974-2747-d684-1c72-2440049db8a7": "YubiKey 5 Series",
    "4b2e6167-73d7-4632-ac78-5db7d2983b63": "Google Titan Key",
    "089870a3-e20c-4ff6-997c-9b168673a985": "Google Titan Key",
    "77997799-7799-7799-7799-779977997799": "Windows Hello",
    "d063a51f-506a-4d2c-8067-ff70f1a92e10": "Apple Touch ID",
    "ad201824-34dd-42a1-a477-8c310c85c5df": "Apple Face ID / Touch ID",
    "9f88c7fb-d392-4dcf-86dc-ef2375806c9a": "Apple iCloud Keychain",
    "143b0bd0-34dd-42a1-a477-8c310c85c5df": "Apple Device",
    "6028c7fb-d392-4dcf-86dc-ef2375806c9a": "Apple Device",
  };
  return common[aaguid.toLowerCase()] || null;
}

/**
 * PasskeysSection lets an authenticated user register, list, and remove FIDO2
 * passkeys used as a phishing-resistant second factor.
 */
export default function PasskeysSection() {
  const { t } = useTranslation("mfa");
  const [credentials, setCredentials] = useState<WebAuthnCredential[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const supported = isWebAuthnSupported();

  async function refresh() {
    try {
      const res = await api.webauthnList();
      setCredentials(res.credentials ?? []);
    } catch {
      toast.error(t("passkeys.loadError"));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    if (!supported) {
      setLoading(false);
      return;
    }
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function handleAdd() {
    const defaultName = t("passkeys.defaultName");
    const name = window.prompt(t("passkeys.namePrompt"), defaultName);
    if (name === null) return; // cancelled
    setBusy(true);
    try {
      const options = await api.webauthnRegisterBegin();
      const credential = await performRegistration(options);
      await api.webauthnRegisterFinish(name.trim() || defaultName, credential);
      await refresh();
    } catch (err: unknown) {
      if (err instanceof ApiError && err.message === "credential_already_registered") {
        toast.error(t("passkeys.duplicateError"));
      } else if (err instanceof ApiError && err.message === "hardware_attestation_required") {
        toast.error(t("passkeys.hardwareAttestationRequiredError"));
      } else {
        toast.error(t("passkeys.registerError"));
      }
    } finally {
      setBusy(false);
    }
  }

  async function handleRename(id: string, currentName: string) {
    const name = window.prompt(t("passkeys.renamePrompt"), currentName);
    if (name === null) return; // cancelled
    const trimmed = name.trim();
    if (!trimmed) {
      toast.error(t("passkeys.renameError"));
      return;
    }
    setBusy(true);
    try {
      await api.webauthnRenameCredential(id, trimmed);
      toast.success(t("passkeys.renameSuccess"));
      await refresh();
    } catch {
      toast.error(t("passkeys.renameError"));
    } finally {
      setBusy(false);
    }
  }

  async function handleDelete(id: string) {
    setBusy(true);
    try {
      await api.webauthnDeleteCredential(id);
      await refresh();
    } catch {
      toast.error(t("passkeys.removeError"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Paper variant="outlined" sx={{ display: "grid", gap: 2, px: 3, py: 2.75, width: "100%", bgcolor: "#0b1120", borderColor: "divider" }}>
      <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
        <Box sx={{ position: "relative", display: "inline-flex", mr: 0.5 }}>
          {busy && (
            <Box
              sx={{
                position: "absolute",
                inset: -6,
                borderRadius: "50%",
                border: "2px solid #6366f1",
                animation: "spinPulse 1.5s linear infinite",
                "@keyframes spinPulse": {
                  "0%": { transform: "rotate(0deg) scale(0.9)", opacity: 0.2 },
                  "50%": { transform: "rotate(180deg) scale(1.1)", opacity: 0.8 },
                  "100%": { transform: "rotate(360deg) scale(0.9)", opacity: 0.2 },
                }
              }}
            />
          )}
          <Fingerprint
            fontSize="small"
            color={busy ? "disabled" : "primary"}
            sx={{
              animation: busy ? "pulseScale 1s ease-in-out infinite alternate" : "none",
              "@keyframes pulseScale": {
                "0%": { transform: "scale(0.95)" },
                "100%": { transform: "scale(1.15)" },
              }
            }}
          />
        </Box>
        <Typography variant="subtitle2" sx={{ fontWeight: 700, flex: 1 }} data-testid="passkeys-section-title">
          {t("passkeys.title")}
        </Typography>
        {supported && (
          <Button size="small" variant="contained" startIcon={<Fingerprint />} onClick={handleAdd} disabled={busy}>
            {t("passkeys.add")}
          </Button>
        )}
      </Box>
      <Typography variant="body2" color="text.secondary">
        {t("passkeys.description")}
      </Typography>

      {!supported && <Alert severity="info">{t("passkeys.unsupported")}</Alert>}

      {supported && !loading && (
        <>
          <Divider />
          {credentials.length === 0 ? (
            <Typography variant="body2" color="text.secondary">{t("passkeys.empty")}</Typography>
          ) : (
            <Box sx={{ display: "grid", gap: 1 }}>
              {credentials.map((c) => (
                <Box key={c.id} sx={{ display: "flex", alignItems: "center", gap: 1.5, border: 1, borderColor: "divider", borderRadius: 1, px: 1.5, py: 1 }}>
                  <Fingerprint fontSize="small" color="action" />
                  <Box sx={{ flex: 1, minWidth: 0 }}>
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
                      <Typography variant="body2" sx={{ fontWeight: 600 }} noWrap>{c.name}</Typography>
                      {getAuthenticatorName(c.aaguid) && (
                        <Typography
                          variant="caption"
                          sx={{
                            display: "inline-block",
                            px: 0.8,
                            py: 0.1,
                            borderRadius: 0.5,
                            bgcolor: "rgba(99, 102, 241, 0.15)",
                            color: "#818cf8",
                            fontWeight: 600,
                            fontSize: "0.675rem",
                            border: "1px solid rgba(99, 102, 241, 0.3)"
                          }}
                        >
                          {getAuthenticatorName(c.aaguid)}
                        </Typography>
                      )}
                    </Box>
                    <Typography variant="caption" color="text.secondary">
                      {t("passkeys.added", { date: new Date(c.created_at).toLocaleDateString() })}
                      {c.last_used_at
                        ? ` · ${t("passkeys.lastUsed", { date: new Date(c.last_used_at).toLocaleDateString() })}`
                        : ` · ${t("passkeys.neverUsed")}`}
                    </Typography>
                  </Box>
                  <Tooltip title={t("passkeys.rename")}>
                    <span>
                      <IconButton size="small" onClick={() => handleRename(c.id, c.name)} disabled={busy} aria-label={t("passkeys.rename")}>
                        <Edit fontSize="small" />
                      </IconButton>
                    </span>
                  </Tooltip>
                  <Tooltip title={t("passkeys.remove")}>
                    <span>
                      <IconButton size="small" onClick={() => handleDelete(c.id)} disabled={busy} aria-label={t("passkeys.remove")}>
                        <DeleteOutline fontSize="small" />
                      </IconButton>
                    </span>
                  </Tooltip>
                </Box>
              ))}
            </Box>
          )}
        </>
      )}
    </Paper>
  );
}
