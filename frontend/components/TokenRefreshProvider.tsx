"use client";

import { useEffect } from "react";
import { useRouter, usePathname } from "next/navigation";
import { scheduleRefresh, cancelRefresh } from "@/lib/tokenManager";

export default function TokenRefreshProvider({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();

  useEffect(() => {
    const isAuthPage = pathname.includes("/auth/");

    if (!isAuthPage) {
      const hasSession = document.cookie.includes("at_exp=");
      if (hasSession) {
        scheduleRefresh(() => {
          const locale = pathname.split("/")[1] ?? "tr";
          router.push(`/${locale}/auth/login`);
        });
      }
    }

    return () => cancelRefresh();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  return <>{children}</>;
}
