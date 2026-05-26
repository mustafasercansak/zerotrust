"use client";

import type { ReactNode } from "react";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";

interface DashboardPageProps {
  children: ReactNode;
  action?: ReactNode;
  error?: ReactNode;
  accessDenied?: boolean;
  accessDeniedMessage?: ReactNode;
}

export function DashboardPage({
  children,
  action,
  error,
  accessDenied = false,
  accessDeniedMessage,
}: DashboardPageProps) {
  if (accessDenied) {
    return (
      <Box sx={{ p: 4 }}>
        <Alert severity="error">{accessDeniedMessage}</Alert>
      </Box>
    );
  }

  return (
    <Box
      sx={{
        display: "flex",
        flexDirection: "column",
        height: "100%",
        minHeight: 0,
        px: 4,
        py: 3,
        gap: 2,
      }}
    >
      {action ? (
        <Box sx={{ display: "flex", justifyContent: "flex-end", flexShrink: 0 }}>
          {action}
        </Box>
      ) : null}
      {error ? (
        <Alert severity="error" sx={{ flexShrink: 0 }}>
          {error}
        </Alert>
      ) : null}
      {children}
    </Box>
  );
}
