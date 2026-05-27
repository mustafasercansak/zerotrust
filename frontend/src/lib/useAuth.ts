import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { api, type MeData } from "./api";

export type { MeData };

/**
 * Fetches the current user on mount. On auth error, redirects to /auth/login.
 * Also syncs the UI locale with the server-stored locale (security signal:
 * unexpected locale changes are audited server-side).
 */
export function useAuth() {
  const navigate = useNavigate();
  const { i18n } = useTranslation();
  const [me, setMe] = useState<MeData | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
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
        setLoading(false);
      })
      .catch(() => {
        // Any failure on a protected route (auth error, network error, 5xx) →
        // redirect to login. If we can't verify identity, gate access.
        navigate("/auth/login", { replace: true });
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return { me, setMe, loading };
}
