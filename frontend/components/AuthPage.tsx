"use client";

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
        <Box sx={{ mb: 3, textAlign: "center" }}>
          <Typography variant="h5" fontWeight={700}>{title}</Typography>
          {subtitle ? (
            <Typography variant="body2" color="text.secondary" sx={{ mt: 0.75 }}>
              {subtitle}
            </Typography>
          ) : null}
        </Box>
        <Paper variant="outlined" sx={{ p: 3 }}>
          {children}
        </Paper>
      </Box>
    </Box>
  );
}
