import type { ReactNode } from "react";
import Box from "@mui/material/Box";
import Paper from "@mui/material/Paper";
import Typography from "@mui/material/Typography";

interface AuthPageProps {
  title: ReactNode;
  subtitle?: ReactNode;
  children: ReactNode;
}

export function AuthPage({ title, subtitle, children }: AuthPageProps) {
  return (
    <Box
      component="main"
      sx={{
        alignItems: "center",
        bgcolor: "background.default",
        display: "flex",
        justifyContent: "center",
        minHeight: "100vh",
        px: 2,
      }}
    >
      <Box sx={{ maxWidth: 400, width: "100%" }}>
        <Box sx={{ mb: 3, textAlign: "center", display: "flex", flexDirection: "column", alignItems: "center" }}>
          <Box
            component="img"
            src="/logo.png"
            alt="ZeroTrust Logo"
            sx={{
              width: 72,
              height: 72,
              objectFit: "contain",
              mb: 2,
              borderRadius: "8px",
            }}
          />
          <Typography variant="h5" sx={{ fontWeight: 700 }}>{title}</Typography>
          {subtitle && (
            <Typography variant="body2" color="text.secondary" sx={{ mt: 0.75 }}>
              {subtitle}
            </Typography>
          )}
        </Box>
        <Paper variant="outlined" sx={{ p: 3 }}>
          {children}
        </Paper>
      </Box>
    </Box>
  );
}
