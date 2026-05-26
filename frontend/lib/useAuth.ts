"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { useLocale } from "next-intl";
import { api, ApiError, MeData } from "./api";

export type { MeData };

const AUTH_ERRORS = new Set(["token_expired", "invalid_token", "missing_token"]);

export function useAuth() {
  const router = useRouter();
  const locale = useLocale();
  const [me, setMe] = useState<MeData | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api
      .me()
      .then((data) => {
        setMe(data);
        setLoading(false);
      })
      .catch((err: unknown) => {
        const code = err instanceof ApiError ? err.message : "internal_error";
        if (AUTH_ERRORS.has(code)) {
          router.replace(`/${locale}/auth/login`);
        } else {
          setLoading(false);
        }
      });
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return { me, loading };
}
