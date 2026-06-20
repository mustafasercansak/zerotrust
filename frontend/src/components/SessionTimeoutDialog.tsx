import { useTranslation } from "react-i18next";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Dialog from "@mui/material/Dialog";
import DialogActions from "@mui/material/DialogActions";
import DialogContent from "@mui/material/DialogContent";
import DialogTitle from "@mui/material/DialogTitle";
import LinearProgress from "@mui/material/LinearProgress";
import Typography from "@mui/material/Typography";
import TimerOutlinedIcon from "@mui/icons-material/TimerOutlined";

const WARN_BEFORE_SECONDS = 60;

interface Props {
  open: boolean;
  secondsRemaining: number;
  onExtend: () => void;
  onLogout: () => void;
}

export function SessionTimeoutDialog({ open, secondsRemaining, onExtend, onLogout }: Props) {
  const { t } = useTranslation("sessionTimeout");
  const progress = (secondsRemaining / WARN_BEFORE_SECONDS) * 100;

  return (
    <Dialog open={open} maxWidth="xs" fullWidth onClose={() => { /* intentionally non-closable */ }}>
      <DialogTitle sx={{ display: "flex", alignItems: "center", gap: 1 }}>
        <TimerOutlinedIcon fontSize="small" color="warning" />
        {t("title")}
      </DialogTitle>
      <DialogContent sx={{ display: "grid", gap: 2, pt: "8px !important" }}>
        <Typography variant="body2" color="text.secondary">
          {t("description")}
        </Typography>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
          <LinearProgress
            variant="determinate"
            value={progress}
            color={secondsRemaining <= 15 ? "error" : "warning"}
            sx={{ flex: 1, height: 6, borderRadius: 3 }}
          />
          <Typography variant="body2" sx={{ fontWeight: 700, minWidth: 32, textAlign: "right" }}>
            {secondsRemaining}s
          </Typography>
        </Box>
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2, gap: 1 }}>
        <Button onClick={onLogout} color="inherit">
          {t("logoutNow")}
        </Button>
        <Button onClick={onExtend} variant="contained" color="primary" autoFocus>
          {t("extend")}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
