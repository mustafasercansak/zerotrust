"use client";

import { createContext, useContext } from "react";
import type { MeData } from "@/lib/useAuth";

export const MeContext = createContext<MeData | null>(null);

export function useMeContext(): MeData | null {
  return useContext(MeContext);
}
