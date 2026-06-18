import { useState } from "react";
import { useTranslation } from "react-i18next";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Dialog from "@mui/material/Dialog";
import DialogActions from "@mui/material/DialogActions";
import DialogContent from "@mui/material/DialogContent";
import DialogTitle from "@mui/material/DialogTitle";
import TextField from "@mui/material/TextField";
import Typography from "@mui/material/Typography";
import LockIcon from "@mui/icons-material/Lock";

interface Props {
  open: boolean;
  error?: string;
  loading?: boolean;
  onSubmit: (code: string) => void;
  onClose: () => void;
}

export function StepUpMfaDialog({ open, error, loading, onSubmit, onClose }: Props) {
  const { t } = useTranslation("common");
  const [code, setCode] = useState("");

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!code.trim()) return;
    onSubmit(code.trim());
    setCode("");
  }

  function handleClose() {
    setCode("");
    onClose();
  }

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="xs" fullWidth>
      <Box component="form" onSubmit={handleSubmit}>
        <DialogTitle sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <LockIcon fontSize="small" color="primary" />
          {t("stepUpMfa.title")}
        </DialogTitle>
        <DialogContent sx={{ display: "grid", gap: 2, pt: "8px !important" }}>
          <Typography variant="body2" color="text.secondary">
            {t("stepUpMfa.description")}
          </Typography>
          <TextField
            label={t("stepUpMfa.codeLabel")}
            value={code}
            onChange={(e) => setCode(e.target.value)}
            autoFocus
            fullWidth
            slotProps={{ htmlInput: { maxLength: 6, inputMode: "numeric" } }}
            error={!!error}
            helperText={error ? t(`errors.${error}`, { defaultValue: t("stepUpMfa.invalidCode") }) : undefined}
            disabled={loading}
          />
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 2 }}>
          <Button onClick={handleClose} color="inherit" disabled={loading}>
            {t("cancel")}
          </Button>
          <Button type="submit" variant="contained" disabled={!code.trim() || loading}>
            {t("stepUpMfa.verify")}
          </Button>
        </DialogActions>
      </Box>
    </Dialog>
  );
}
