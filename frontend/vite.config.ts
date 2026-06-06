import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "path";

// Support both the Vite-prefixed var and the legacy docker-compose var.
const backendUrl =
  process.env.VITE_BACKEND_URL ??
  process.env.BACKEND_INTERNAL_URL ??
  "http://localhost:8080";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "src"),
    },
  },
  server: {
    port: 3000,
    proxy: {
      // Vite proxy properly supports SSE streaming — no buffering issues.
      "/api": { target: backendUrl, changeOrigin: true },
      "/.well-known": { target: backendUrl, changeOrigin: true },
      "/health": { target: backendUrl, changeOrigin: true },
      "/oauth2/authorize": { target: backendUrl, changeOrigin: true },
      "/oauth2/token": { target: backendUrl, changeOrigin: true },
      "/oauth2/userinfo": { target: backendUrl, changeOrigin: true },
    },
  },
  build: {
    outDir: "dist",
    sourcemap: false,
  },
});
