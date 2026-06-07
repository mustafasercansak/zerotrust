import { useState } from "react";
import Box from "@mui/material/Box";
import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";
import Typography from "@mui/material/Typography";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import CheckIcon from "@mui/icons-material/Check";
import { toast } from "sonner";

interface SecretDisplayCardProps {
  label: string;
  value: string;
  hasGradient?: boolean;
  icon?: React.ReactNode;
  successMessage?: string;
}

export function SecretDisplayCard({ label, value, hasGradient, icon, successMessage }: SecretDisplayCardProps) {
  const [copied, setCopied] = useState(false);

  const handleCopy = () => {
    navigator.clipboard.writeText(value);
    setCopied(true);
    toast.success(successMessage || "Copied to clipboard");
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <Box
      sx={{
        border: "1px solid",
        borderColor: hasGradient ? "rgba(99, 102, 241, 0.25)" : "rgba(255, 255, 255, 0.08)",
        borderRadius: 2.5,
        p: 2.5,
        bgcolor: hasGradient ? "rgba(99, 102, 241, 0.04)" : "rgba(255, 255, 255, 0.02)",
        boxShadow: hasGradient ? "0 8px 32px 0 rgba(99, 102, 241, 0.06)" : "none",
        position: "relative",
        overflow: "hidden",
      }}
    >
      {hasGradient && <Box sx={{ position: "absolute", top: 0, left: 0, right: 0, height: 3, background: "linear-gradient(90deg, #4f46e5, #818cf8)" }} />}
      <Typography variant="caption" color="text.secondary" sx={{ display: "flex", alignItems: "center", gap: 0.75, mb: 1, fontWeight: 700, letterSpacing: 0.5, textTransform: "uppercase" }}>
        {icon}
        {label}
      </Typography>
      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", bgcolor: "#030712", border: "1px solid rgba(255,255,255,0.06)", borderRadius: 1.5, p: 1.5 }}>
        <Typography sx={{ fontFamily: "monospace", fontWeight: 700, overflowWrap: "anywhere", fontSize: "0.9rem" }}>{value}</Typography>
        <Tooltip title="Copy">
          <IconButton
            size="small"
            onClick={handleCopy}
            sx={{
              bgcolor: "rgba(255, 255, 255, 0.03)",
              border: "1px solid rgba(255, 255, 255, 0.08)",
              transition: "all 0.2s",
              "&:hover": {
                bgcolor: "rgba(255, 255, 255, 0.08)",
                borderColor: "rgba(255, 255, 255, 0.2)",
              }
            }}
          >
            {copied ? <CheckIcon sx={{ fontSize: 13 }} color="success" /> : <ContentCopyIcon sx={{ fontSize: 13 }} />}
          </IconButton>
        </Tooltip>
      </Box>
    </Box>
  );
}
