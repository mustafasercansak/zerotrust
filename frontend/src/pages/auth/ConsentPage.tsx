import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { api, ApiError } from "@/lib/api";
import { AuthPage } from "@/components/AuthPage";
import { toast } from "sonner";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Typography from "@mui/material/Typography";
import Paper from "@mui/material/Paper";
import CheckIcon from "@mui/icons-material/Check";
import List from "@mui/material/List";
import ListItem from "@mui/material/ListItem";
import ListItemIcon from "@mui/material/ListItemIcon";
import ListItemText from "@mui/material/ListItemText";

export default function ConsentPage() {
  const { t } = useTranslation("auth");
  const [searchParams] = useSearchParams();
  const [loading, setLoading] = useState(false);
  const [clientName, setClientName] = useState<string | null>(null);

  const clientID = searchParams.get("client_id") || "";
  const redirectURI = searchParams.get("redirect_uri") || "";
  const scopeStr = searchParams.get("scope") || "";
  const state = searchParams.get("state") || "";
  const codeChallenge = searchParams.get("code_challenge") || "";
  const codeChallengeMethod = searchParams.get("code_challenge_method") || "";
  const nonce = searchParams.get("nonce") || "";

  const scopes = scopeStr ? scopeStr.split(" ") : [];

  useEffect(() => {
    if (!clientID) return;
    api.getOidcClientInfo(clientID)
      .then((info) => setClientName(info.name))
      .catch(() => { /* fall back to showing client_id */ });
  }, [clientID]);

  async function submitConsent(approved: boolean) {
    return api.submitConsent({
      client_id: clientID,
      redirect_uri: redirectURI,
      scopes,
      code_challenge: codeChallenge,
      code_challenge_method: codeChallengeMethod,
      nonce,
      state,
      approved,
    });
  }

  async function handleResponse(approved: boolean) {
    setLoading(true);
    try {
      const resp = await submitConsent(approved).catch(async (err: unknown) => {
        if (err instanceof ApiError && err.message === "mfa_required" && err.status === 403) {
          const code = window.prompt(t("consent.mfaPrompt"))?.trim() ?? "";
          if (!code) throw err;
          await api.mfaStepUp(code);
          return submitConsent(approved);
        }
        throw err;
      });
      window.location.href = resp.redirect_url;
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        toast.error(t(`errors.${err.message}`, { defaultValue: err.message }));
      } else {
        toast.error(t("consent.errorInternal"));
      }
    } finally {
      setLoading(false);
    }
  }

  return (
    <AuthPage title={t("consent.title")} subtitle={t("consent.subtitle")}>
      <Box sx={{ display: "grid", gap: 3 }}>
        <Paper variant="outlined" sx={{ p: 2.5, bgcolor: "background.default", border: 1, borderColor: "divider" }}>
          <Typography variant="body1" sx={{ fontWeight: 700, mb: 0.5, color: "primary.main" }}>
            {clientName ?? clientID}
          </Typography>
          {clientName && (
            <Typography variant="caption" color="text.disabled" sx={{ display: "block", fontFamily: "monospace", mb: 1 }}>
              {clientID}
            </Typography>
          )}
          <Typography variant="body2" color="text.secondary">
            {t("consent.requestingAccess")}
          </Typography>
        </Paper>

        {scopes.length > 0 && (
          <Box>
            <Typography variant="body2" sx={{ fontWeight: 700, mb: 1 }}>
              {t("consent.permissionsTitle")}
            </Typography>
            <List dense disablePadding>
              {scopes.map((s) => (
                <ListItem key={s} disableGutters sx={{ py: 0.5 }}>
                  <ListItemIcon sx={{ minWidth: 32 }}>
                    <CheckIcon color="success" fontSize="small" />
                  </ListItemIcon>
                  <ListItemText
                    primary={
                      <Typography variant="body2" sx={{ fontWeight: 500 }}>
                        {t(`consent.scopes.${s}`, { defaultValue: s })}
                      </Typography>
                    }
                  />
                </ListItem>
              ))}
            </List>
          </Box>
        )}

        <Box sx={{ display: "flex", gap: 2, mt: 1 }}>
          <Button
            fullWidth
            variant="contained"
            color="primary"
            onClick={() => handleResponse(true)}
            disabled={loading}
          >
            {t("consent.authorize")}
          </Button>
          <Button
            fullWidth
            variant="outlined"
            color="error"
            onClick={() => handleResponse(false)}
            disabled={loading}
          >
            {t("consent.cancel")}
          </Button>
        </Box>
      </Box>
    </AuthPage>
  );
}
