/**
 * SessionCard — reusable card for a single session.
 *
 * Used by:
 *  - UserProfileDrawer (admin sessions tab)
 *  - HomePage (current user's recent sessions)
 */
import { useTranslation } from "react-i18next";
import { formatSessionDevice } from "@/lib/sessionUtils";
import { formatDateTime } from "@/lib/dateUtils";
import type { Session } from "@/lib/api";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import Paper from "@mui/material/Paper";
import Typography from "@mui/material/Typography";

interface SessionCardProps {
  session: Session;
  /** Called when the revoke button is clicked. Omit to hide the button. */
  onRevoke?: (session: Session) => void;
  /** Locale string forwarded to date formatter */
  locale: string;
}

function getFlagEmoji(countryCode?: string): string {
  if (!countryCode || countryCode.length !== 2) return "";
  const codePoints = countryCode
    .toUpperCase()
    .split("")
    .map((char) => 127397 + char.charCodeAt(0));
  try {
    return String.fromCodePoint(...codePoints);
  } catch {
    return "";
  }
}

export function SessionCard({ session: s, onRevoke, locale }: SessionCardProps) {
  const { t } = useTranslation("admin");

  return (
    <Paper
      variant="outlined"
      sx={{ p: 1.5, position: "relative", overflow: "hidden" }}
    >
      {/* Left accent bar for current session */}
      {s.is_current && (
        <Box
          sx={{
            position: "absolute",
            left: 0,
            top: 0,
            bottom: 0,
            width: 3,
            bgcolor: "primary.main",
          }}
        />
      )}

      <Box
        sx={{
          display: "flex",
          justifyContent: "space-between",
          alignItems: "flex-start",
          gap: 1,
        }}
      >
        <Box sx={{ minWidth: 0 }}>
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              gap: 0.75,
              flexWrap: "wrap",
            }}
          >
            <Typography
              variant="body2"
              noWrap
              sx={{ fontWeight: 600 }}
            >
              {formatSessionDevice(s)}
            </Typography>
            {s.is_current && (
              <Chip label="current" size="small" color="primary" />
            )}
          </Box>

          <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ fontFamily: "monospace" }}
            >
              {s.ip_address || "—"}
            </Typography>
            {s.location && (
              <Typography
                variant="caption"
                sx={{
                  display: "inline-flex",
                  alignItems: "center",
                  gap: 0.5,
                  bgcolor: "rgba(99, 102, 241, 0.1)",
                  color: "#818cf8",
                  px: 0.8,
                  py: 0.1,
                  borderRadius: 0.5,
                  fontWeight: 600,
                  fontSize: "0.675rem",
                  border: "1px solid rgba(99, 102, 241, 0.2)"
                }}
              >
                {s.country_code ? `${getFlagEmoji(s.country_code)} ` : ""}{s.location}
              </Typography>
            )}
          </Box>

          <Typography
            variant="caption"
            color="text.disabled"
            sx={{ display: "block" }}
          >
            {formatDateTime(s.last_used_at ?? s.created_at, locale)}
          </Typography>
        </Box>

        {onRevoke && (
          <Button
            size="small"
            color="warning"
            sx={{ flexShrink: 0 }}
            onClick={() => onRevoke(s)}
          >
            {t("revokeSession")}
          </Button>
        )}
      </Box>
    </Paper>
  );
}
