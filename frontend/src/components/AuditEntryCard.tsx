/**
 * AuditEntryCard — reusable card for a single audit log entry.
 *
 * Used by:
 *  - UserProfileDrawer (admin audit tab)
 *  - AuditPage detail drawer
 *  - HomePage recent-activity feed
 */
import { useTranslation } from "react-i18next";
import { formatDateTime } from "@/lib/dateUtils";
import type { AuditEntry } from "@/lib/api";
import Box from "@mui/material/Box";
import Chip from "@mui/material/Chip";
import Paper from "@mui/material/Paper";
import Typography from "@mui/material/Typography";

interface AuditEntryCardProps {
  entry: AuditEntry;
  /** Locale string forwarded to date formatter */
  locale: string;
  /** When true the card is rendered in a compact single-line style */
  compact?: boolean;
  /** Called when the card is clicked */
  onClick?: (entry: AuditEntry) => void;
}

export function AuditEntryCard({
  entry,
  locale,
  compact = false,
  onClick,
}: AuditEntryCardProps) {
  const { t } = useTranslation("audit");
  const outcome = entry.metadata?.outcome as string | undefined;
  const isSuccess = outcome === "success";
  const isFailure = outcome === "failure";

  const accentColor = isSuccess
    ? "success.main"
    : isFailure
    ? "error.main"
    : "divider";

  return (
    <Paper
      variant="outlined"
      onClick={onClick ? () => onClick(entry) : undefined}
      sx={{
        p: compact ? 1 : 1.5,
        position: "relative",
        overflow: "hidden",
        cursor: onClick ? "pointer" : "default",
        transition: "background-color 0.15s",
        "&:hover": onClick
          ? { bgcolor: "action.hover" }
          : undefined,
      }}
    >
      {/* Left accent bar */}
      <Box
        sx={{
          position: "absolute",
          left: 0,
          top: 0,
          bottom: 0,
          width: 3,
          bgcolor: accentColor,
        }}
      />

      <Box
        sx={{
          display: "flex",
          alignItems: "flex-start",
          justifyContent: "space-between",
          gap: 1,
          pl: 0.5,
        }}
      >
        <Box sx={{ minWidth: 0, flex: 1 }}>
          <Typography
            variant="body2"
            sx={{ fontWeight: 600, fontFamily: "monospace", fontSize: 12 }}
            noWrap
          >
            {entry.action}
          </Typography>

          {!compact && (
            <Typography
              variant="caption"
              color="text.secondary"
              noWrap
              sx={{ display: "block" }}
            >
              {entry.ip_address || "—"}
            </Typography>
          )}

          <Typography variant="caption" color="text.disabled">
            {formatDateTime(entry.created_at, locale)}
          </Typography>
        </Box>

        {outcome && (
          <Chip
            size="small"
            color={isSuccess ? "success" : isFailure ? "error" : "default"}
            label={isSuccess ? t("success") : isFailure ? t("failure") : outcome}
            sx={{ flexShrink: 0 }}
          />
        )}
      </Box>
    </Paper>
  );
}
