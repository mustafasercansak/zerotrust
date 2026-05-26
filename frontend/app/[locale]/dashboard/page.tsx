"use client";

import { useTranslations, useLocale } from "next-intl";
import { useMeContext } from "./context";
import Box from "@mui/material/Box";
import Chip from "@mui/material/Chip";
import Paper from "@mui/material/Paper";
import Typography from "@mui/material/Typography";

export default function DashboardPage() {
  const t = useTranslations("dashboard");
  const locale = useLocale();
  const me = useMeContext();

  if (!me) return null;

  return (
    <Box sx={{ height: "100%", maxWidth: 720, overflow: "auto", p: 4 }}>
      <Typography variant="h5" fontWeight={700} sx={{ mb: 3 }}>{t("title")}</Typography>

      <Paper variant="outlined" sx={{ p: 3, mb: 3 }}>
        <Typography color="text.secondary">{t("welcome", { email: me.email })}</Typography>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, mt: 1.5 }}>
          <Box sx={{ width: 8, height: 8, borderRadius: "50%", bgcolor: "success.main" }} />
          <Typography variant="body2" color="success.main">{t("verified")}</Typography>
        </Box>
      </Paper>

      <Paper variant="outlined" sx={{ p: 3 }}>
        <Typography variant="overline" color="text.secondary">{t("securityStatus")}</Typography>
        <Box sx={{ display: "grid", gridTemplateColumns: "140px 1fr", gap: 1.25, mt: 1 }}>
          <Typography variant="body2" color="text.secondary">User ID</Typography>
          <Typography variant="caption" noWrap sx={{ fontFamily: "monospace" }}>{me.user_id}</Typography>
          <Typography variant="body2" color="text.secondary">{t("locale")}</Typography>
          <Typography variant="body2">{locale.toUpperCase()}</Typography>
          <Typography variant="body2" color="text.secondary">{t("roles")}</Typography>
          <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
            {me.roles.map((r) => <Chip key={r} size="small" color="primary" label={r} />)}
          </Box>
        </Box>
      </Paper>
    </Box>
  );
}
