import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { ThemeProvider } from "@mui/material/styles";
import CssBaseline from "@mui/material/CssBaseline";
import { Toaster } from "sonner";
import theme from "./theme";
import TokenRefreshProvider from "./components/TokenRefreshProvider";
import DashboardLayout from "./components/DashboardLayout";
import LoginPage from "./pages/auth/LoginPage";
import ForgotPasswordPage from "./pages/auth/ForgotPasswordPage";
import ResetPasswordPage from "./pages/auth/ResetPasswordPage";
import HomePage from "./pages/dashboard/HomePage";
import SessionsPage from "./pages/dashboard/SessionsPage";
import UsersPage from "./pages/dashboard/UsersPage";
import AuditPage from "./pages/dashboard/AuditPage";
import ServiceAccountsPage from "./pages/dashboard/ServiceAccountsPage";
import SettingsPage from "./pages/dashboard/SettingsPage";
import MfaPage from "./pages/dashboard/MfaPage";

export default function App() {
  return (
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <Toaster
        theme="dark"
        position="top-right"
        richColors
        closeButton
        toastOptions={{ style: { fontFamily: "inherit" } }}
      />
      <BrowserRouter>
        {/* TokenRefreshProvider must be inside BrowserRouter (uses useNavigate). */}
        <TokenRefreshProvider>
          <Routes>
            <Route path="/" element={<Navigate to="/dashboard" replace />} />

            {/* ── Auth ──────────────────────────────────────────────────────── */}
            <Route path="/auth/login" element={<LoginPage />} />
            <Route path="/auth/forgot-password" element={<ForgotPasswordPage />} />
            <Route path="/auth/reset-password" element={<ResetPasswordPage />} />

            {/* ── Dashboard (protected, nested) ─────────────────────────────── */}
            <Route path="/dashboard" element={<DashboardLayout />}>
              <Route index element={<HomePage />} />
              <Route path="sessions" element={<SessionsPage />} />
              <Route path="mfa" element={<MfaPage />} />
              <Route path="users" element={<UsersPage />} />
              <Route path="audit" element={<AuditPage />} />
              <Route path="service-accounts" element={<ServiceAccountsPage />} />
              <Route path="settings" element={<SettingsPage />} />
            </Route>

            <Route path="*" element={<Navigate to="/dashboard" replace />} />
          </Routes>
        </TokenRefreshProvider>
      </BrowserRouter>
    </ThemeProvider>
  );
}
