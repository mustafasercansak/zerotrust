import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { ApiError, api, type MeData } from "./api";

export type { MeData };

export type AuthBootstrapError = "network" | "server";
export type AuthBootstrapFailureAction =
  | { type: "redirect" }
  | { type: "error"; error: AuthBootstrapError };

export function isAuthRedirectError(err: unknown): boolean {
  return err instanceof ApiError && (err.status === 401 || err.status === 403);
}

export function classifyAuthBootstrapError(err: unknown): AuthBootstrapError {
  if (err instanceof ApiError && err.status !== undefined && err.status >= 500) {
    return "server";
  }
  return "network";
}

export function authBootstrapFailureAction(err: unknown): AuthBootstrapFailureAction {
  if (isAuthRedirectError(err)) {
    return { type: "redirect" };
  }
  return { type: "error", error: classifyAuthBootstrapError(err) };
}

/**
 * Fetches the current user on mount. Only auth errors redirect to /auth/login;
 * infrastructure failures stay on the protected route with a retryable error.
 * Also syncs the UI locale with the server-stored locale (security signal:
 * unexpected locale changes are audited server-side).
 */
export function useAuth() {
  const navigate = useNavigate();
  const { i18n } = useTranslation();
  const [me, setMe] = useState<MeData | null>(null);
  const [loading, setLoading] = useState(true);
  const [bootstrapError, setBootstrapError] = useState<AuthBootstrapError | null>(null);

  function loadMe() {
    setLoading(true);
    setBootstrapError(null);
    api
      .me()
      .then((data) => {
        // Locale is the source of truth from the server profile.
        // Changing locale goes through PATCH /api/v1/me/locale (audited).
        if (data.locale && data.locale !== i18n.language) {
          i18n.changeLanguage(data.locale);
          localStorage.setItem("locale", data.locale);
        }
        setMe(data);
        setBootstrapError(null);
        setLoading(false);
      })
      .catch((err) => {
        const action = authBootstrapFailureAction(err);
        if (action.type === "redirect") {
          navigate("/auth/login", { replace: true });
          return;
        }
        setMe(null);
        setBootstrapError(action.error);
        setLoading(false);
      });
  }

  useEffect(() => {
    loadMe();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return { me, setMe, loading, bootstrapError, retry: loadMe };
}
