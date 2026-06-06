import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { ThemeProvider } from "@mui/material/styles";
import CssBaseline from "@mui/material/CssBaseline";
import { Toaster } from "sonner";
import theme from "./theme";
import TokenRefreshProvider from "./components/TokenRefreshProvider";
import { lazy, Suspense } from "react";

const DashboardLayout = lazy(() => import("./components/DashboardLayout"));
const LoginPage = lazy(() => import("./pages/auth/LoginPage"));
const ForgotPasswordPage = lazy(() => import("./pages/auth/ForgotPasswordPage"));
const ResetPasswordPage = lazy(() => import("./pages/auth/ResetPasswordPage"));
const HomePage = lazy(() => import("./pages/dashboard/HomePage"));
const SessionsPage = lazy(() => import("./pages/dashboard/SessionsPage"));
const UsersPage = lazy(() => import("./pages/dashboard/UsersPage"));
const AuditPage = lazy(() => import("./pages/dashboard/AuditPage"));
const SecurityDashboardPage = lazy(() => import("./pages/dashboard/SecurityDashboardPage"));
const ServiceAccountsPage = lazy(() => import("./pages/dashboard/ServiceAccountsPage"));
const SettingsPage = lazy(() => import("./pages/dashboard/SettingsPage"));
const OidcClientsPage = lazy(() => import("./pages/dashboard/OidcClientsPage"));
const MfaPage = lazy(() => import("./pages/dashboard/MfaPage"));
const ConsentPage = lazy(() => import("./pages/auth/ConsentPage"));

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
          <Suspense fallback={<div role="status" aria-label="Loading" />}>
            <Routes>
              <Route path="/" element={<Navigate to="/dashboard" replace />} />

              {/* ── Auth ──────────────────────────────────────────────────────── */}
              <Route path="/auth/login" element={<LoginPage />} />
              <Route path="/auth/forgot-password" element={<ForgotPasswordPage />} />
              <Route path="/auth/reset-password" element={<ResetPasswordPage />} />
              <Route path="/oauth2/consent" element={<ConsentPage />} />

              {/* ── Dashboard (protected, nested) ─────────────────────────────── */}
              <Route path="/dashboard" element={<DashboardLayout />}>
                <Route index element={<HomePage />} />
                <Route path="sessions" element={<SessionsPage />} />
                <Route path="mfa" element={<MfaPage />} />
                <Route path="users" element={<UsersPage />} />
                <Route path="security" element={<SecurityDashboardPage />} />
                <Route path="audit" element={<AuditPage />} />
                <Route path="service-accounts" element={<ServiceAccountsPage />} />
                <Route path="settings" element={<SettingsPage />} />
                <Route path="oidc-clients" element={<OidcClientsPage />} />
              </Route>

              <Route path="*" element={<Navigate to="/dashboard" replace />} />
            </Routes>
          </Suspense>
        </TokenRefreshProvider>
      </BrowserRouter>
    </ThemeProvider>
  );
}
