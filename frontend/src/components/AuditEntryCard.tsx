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

const countryNameToFlag: Record<string, string> = {
  "united states": "🇺🇸",
  "turkey": "🇹🇷",
  "türkiye": "🇹🇷",
  "united kingdom": "🇬🇧",
  "germany": "🇩🇪",
  "france": "🇫🇷",
  "japan": "🇯🇵",
  "australia": "🇦🇺",
  "canada": "🇨🇦",
  "netherlands": "🇳🇱",
  "switzerland": "🇨🇭",
  "sweden": "🇸🇪",
  "singapore": "🇸🇬",
  "local": "💻",
  "localhost": "💻",
};

function getCountryFlag(country?: string): string {
  if (!country) return "";
  return countryNameToFlag[country.toLowerCase()] || "📍";
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

  const location = entry.metadata?.location as { country?: string; city?: string } | undefined;
  const locationText = location
    ? location.city && location.country
      ? `${location.city}, ${location.country}`
      : location.country || ""
    : "";

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
            <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap", mt: 0.25 }}>
              <Typography
                variant="caption"
                color="text.secondary"
                noWrap
              >
                {entry.ip_address || "—"}
              </Typography>
              {locationText && (
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{
                    display: "inline-flex",
                    alignItems: "center",
                    gap: 0.5,
                    fontSize: "0.675rem",
                    bgcolor: "rgba(99, 102, 241, 0.08)",
                    border: "1px solid rgba(99, 102, 241, 0.15)",
                    px: 0.6,
                    py: 0.05,
                    borderRadius: 0.5,
                    color: "#818cf8",
                    fontWeight: 600,
                  }}
                >
                  {getCountryFlag(location?.country)} {locationText}
                </Typography>
              )}
            </Box>
          )}

          <Box sx={{ display: "flex", alignItems: "center", gap: 0.75, flexWrap: "wrap" }}>
            <Typography variant="caption" color="text.disabled">
              {formatDateTime(entry.created_at, locale)}
            </Typography>
            {compact && locationText && (
              <Typography
                variant="caption"
                sx={{
                  display: "inline-flex",
                  alignItems: "center",
                  gap: 0.25,
                  fontSize: "0.625rem",
                  color: "text.secondary",
                }}
              >
                · {getCountryFlag(location?.country)} {locationText}
              </Typography>
            )}
          </Box>
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
