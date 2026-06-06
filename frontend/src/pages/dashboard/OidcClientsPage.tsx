import OidcClientsSection from "./OidcClientsSection";
import Box from "@mui/material/Box";

export default function OidcClientsPage() {
  return (
    <Box sx={{ display: "flex", flexDirection: "column", height: "100%", overflow: "hidden", p: 4, gap: 0 }}>
      <OidcClientsSection />
    </Box>
  );
}
