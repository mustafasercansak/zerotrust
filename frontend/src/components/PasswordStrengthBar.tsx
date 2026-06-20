import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";

function score(password: string): number {
  if (!password) return 0;
  let s = 0;
  if (password.length >= 8) s++;
  if (password.length >= 14) s++;
  if (/[A-Z]/.test(password)) s++;
  if (/[0-9]/.test(password)) s++;
  if (/[^A-Za-z0-9]/.test(password)) s++;
  return Math.min(s, 4);
}

const COLORS = ["#ef4444", "#f97316", "#eab308", "#22c55e"] as const;
const LABEL_KEYS = ["weak", "fair", "strong", "veryStrong"] as const;

interface PasswordStrengthBarProps {
  password: string;
}

export function PasswordStrengthBar({ password }: PasswordStrengthBarProps) {
  const { t } = useTranslation("common");
  const s = useMemo(() => score(password), [password]);

  if (!password) return null;

  const color = COLORS[s - 1] ?? COLORS[0];
  const label = t(`passwordStrength.${LABEL_KEYS[s - 1] ?? LABEL_KEYS[0]}`);

  return (
    <Box sx={{ mt: 0.5 }}>
      <Box sx={{ display: "flex", gap: 0.5, mb: 0.5 }}>
        {COLORS.map((c, i) => (
          <Box
            key={i}
            sx={{
              flex: 1,
              height: 4,
              borderRadius: 2,
              bgcolor: i < s ? color : "action.hover",
              transition: "background-color 0.2s",
            }}
          />
        ))}
      </Box>
      <Typography variant="caption" sx={{ color, fontWeight: 500 }}>
        {label}
      </Typography>
    </Box>
  );
}
