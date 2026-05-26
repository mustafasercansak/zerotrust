import { scheduleRefresh } from "./tokenManager";

export interface MeData {
  user_id: string;
  email: string;
  locale: string;
  roles: string[];
}

export interface UserData {
  id: string;
  email: string;
  locale: string;
  is_active: boolean;
  roles: string[];
  created_at: string;
}

export class ApiError extends Error {
  retryAfter?: number;
  constructor(code: string, retryAfter?: number) {
    super(code);
    this.name = "ApiError";
    this.retryAfter = retryAfter;
  }
}

export interface ServiceAccount {
  id: string;
  name: string;
  client_id: string;
  is_active: boolean;
  scopes: string[];
  created_at: string;
  expires_at: string | null;
}

export interface ServiceAccountCreated extends ServiceAccount {
  client_secret: string;
}

export interface Session {
  id: string;
  ip_address: string;
  user_agent: string;
  created_at: string;
  last_used_at: string | null;
  is_current: boolean;
}

function getCSRFToken(): string {
  if (typeof document === "undefined") return "";
  const m = document.cookie.match(/(?:^|;\s*)csrf_token=([^;]*)/);
  return m ? decodeURIComponent(m[1]) : "";
}

let refreshing: Promise<void> | null = null;

async function refreshTokens(): Promise<void> {
  if (refreshing) return refreshing;

  refreshing = (async () => {
    const res = await fetch("/api/v1/auth/refresh", {
      method: "POST",
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": getCSRFToken(),
      },
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) throw new ApiError(data.error ?? "invalid_token");
    scheduleRefresh();
  })().finally(() => {
    refreshing = null;
  });

  return refreshing;
}

async function request<T>(path: string, init?: RequestInit, retry = true): Promise<T> {
  const method = (init?.method ?? "GET").toUpperCase();
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(init?.headers as Record<string, string>),
  };

  if (["POST", "PUT", "PATCH", "DELETE"].includes(method)) {
    const csrf = getCSRFToken();
    if (csrf) headers["X-CSRF-Token"] = csrf;
  }

  const res = await fetch(path, {
    ...init,
    credentials: "include",
    headers,
  });

  if (res.status === 204) {
    return undefined as T;
  }

  const data = await res.json();

  if (!res.ok) {
    const code: string = data.error ?? "internal_error";
    const retryAfter: number | undefined = data.retry_after;

    if (res.status === 401 && code === "token_expired" && retry) {
      try {
        await refreshTokens();
        return request<T>(path, init, false);
      } catch {
        throw new ApiError("missing_token");
      }
    }

    throw new ApiError(code, retryAfter);
  }

  return data as T;
}

export const api = {
  login: (email: string, password: string) =>
    request<{ ok: boolean }>("/api/v1/auth/login", {
      method: "POST",
      body: JSON.stringify({ email, password }),
    }),

  logout: () =>
    fetch("/api/v1/auth/logout", {
      method: "POST",
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": getCSRFToken(),
      },
    }),

  me: () => request<MeData>("/api/v1/me"),

  listSessions: () => request<Session[]>("/api/v1/sessions"),

  revokeSession: (id: string) =>
    request<void>(`/api/v1/sessions/${id}`, { method: "DELETE" }),

  admin: {
    listUsers: () => request<UserData[]>("/api/v1/admin/users"),

    createUser: (payload: { email: string; password: string; locale: string; roles: string[] }) =>
      request<UserData>("/api/v1/admin/users", {
        method: "POST",
        body: JSON.stringify(payload),
      }),

    updateRoles: (userId: string, roles: string[]) =>
      request<void>(`/api/v1/admin/users/${userId}/roles`, {
        method: "PATCH",
        body: JSON.stringify({ roles }),
      }),

    listServiceAccounts: () =>
      request<ServiceAccount[]>("/api/v1/admin/service-accounts"),

    createServiceAccount: (payload: { name: string; scopes: string[]; expires_at?: string }) =>
      request<ServiceAccountCreated>("/api/v1/admin/service-accounts", {
        method: "POST",
        body: JSON.stringify(payload),
      }),

    setServiceAccountStatus: (id: string, isActive: boolean) =>
      request<void>(`/api/v1/admin/service-accounts/${id}/status`, {
        method: "PATCH",
        body: JSON.stringify({ is_active: isActive }),
      }),

    revokeServiceAccount: (id: string) =>
      request<void>(`/api/v1/admin/service-accounts/${id}`, {
        method: "DELETE",
      }),
  },
};
