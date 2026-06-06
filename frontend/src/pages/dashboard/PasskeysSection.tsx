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
      } else {
        toast.error(t("passkeys.registerError"));
      }
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
        <Fingerprint fontSize="small" color="primary" />
        <Typography variant="subtitle2" sx={{ fontWeight: 700, flex: 1 }}>
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
                    <Typography variant="body2" sx={{ fontWeight: 600 }} noWrap>{c.name}</Typography>
                    <Typography variant="caption" color="text.secondary">
                      {t("passkeys.added", { date: new Date(c.created_at).toLocaleDateString() })}
                      {c.last_used_at
                        ? ` · ${t("passkeys.lastUsed", { date: new Date(c.last_used_at).toLocaleDateString() })}`
                        : ` · ${t("passkeys.neverUsed")}`}
                    </Typography>
                  </Box>
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
