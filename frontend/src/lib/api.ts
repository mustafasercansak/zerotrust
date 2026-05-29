import { scheduleRefresh } from "./tokenManager";
import { getClientInfo } from "./clientInfo";

export interface MeData {
  user_id: string;
  email: string;
  first_name: string;
  last_name: string;
  has_avatar: boolean;
  locale: string;
  roles: string[];
  permissions?: string[];
}

export interface UserData {
  id: string;
  email: string;
  first_name: string;
  last_name: string;
  has_avatar: boolean;
  locale: string;
  is_active: boolean;
  roles: string[];
  created_at: string;
  active_sessions: number;
}

export class ApiError extends Error {
  status?: number;
  retryAfter?: number;
  constructor(code: string, retryAfter?: number, status?: number) {
    super(code);
    this.name = "ApiError";
    this.retryAfter = retryAfter;
    this.status = status;
  }
}

function isAuthStatus(status?: number): boolean {
  return status === 401 || status === 403;
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

export interface ServiceTokenResponse {
  access_token: string;
  token_type: string;
  expires_in: number;
}

export interface ServiceProbeResult {
  ok: boolean;
  status: number;
  statusText: string;
  body: unknown;
}

export interface Session {
  id: string;
  ip_address: string;
  user_agent: string;
  device_info: {
    architecture?: string;
    browser?: string;
    browser_version?: string;
    mobile?: string;
    os?: string;
    os_version?: string;
  } | null;
  created_at: string;
  last_used_at: string | null;
  is_current: boolean;
}

export interface AuditEntry {
  id: string;
  user_id: string | null;
  user_email: string | null;
  action: string;
  resource: string;
  ip_address: string | null;
  user_agent: string | null;
  metadata: {
    client_info?: {
      architecture?: string;
      browser?: string;
      browser_version?: string;
      mobile?: string;
      os?: string;
      os_version?: string;
    };
  } | null;
  created_at: string;
}

// Shared pagination / sort / filter params used by resource table fetchers.
export interface PageParams {
  page: number;
  pageSize: number;
  sortKey?: string;
  sortDir?: "asc" | "desc";
  filters: Record<string, string>;
}

export interface PagedResult<T> {
  data: T[];
  total: number;
}

function buildQuery(p: PageParams): string {
  const params = new URLSearchParams();
  params.set("limit", String(p.pageSize));
  params.set("offset", String(p.page * p.pageSize));
  if (p.sortKey) params.set("sort_by", p.sortKey);
  if (p.sortDir) params.set("sort_dir", p.sortDir);
  for (const [k, v] of Object.entries(p.filters)) {
    if (v) params.set(k, v);
  }
  return `?${params.toString()}`;
}

function getCSRFToken(): string {
  if (typeof document === "undefined") return "";
  const m = document.cookie.match(/(?:^|;\s*)csrf_token=([^;]*)/);
  return m ? decodeURIComponent(m[1]) : "";
}

let refreshing: Promise<void> | null = null;
let clientInfoPromise: Promise<Record<string, string | undefined>> | null = null;

function cachedClientInfo(): Promise<Record<string, string | undefined>> {
  if (!clientInfoPromise) {
    clientInfoPromise = getClientInfo();
  }
  return clientInfoPromise;
}

async function refreshTokens(): Promise<void> {
  if (refreshing) return refreshing;

  refreshing = (async () => {
    const clientInfo = await getClientInfo();
    const res = await fetch("/api/v1/auth/refresh", {
      method: "POST",
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": getCSRFToken(),
      },
      body: JSON.stringify({ client_info: clientInfo }),
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) throw new ApiError(data.error ?? "invalid_token", undefined, res.status);
    scheduleRefresh();
  })().finally(() => {
    refreshing = null;
  });

  return refreshing;
}

async function request<T>(path: string, init?: RequestInit, retry = true): Promise<T> {
  const method = (init?.method ?? "GET").toUpperCase();
  const clientInfo = await cachedClientInfo();
  const isFormData = init?.body instanceof FormData;
  const headers: Record<string, string> = {
    "X-Client-Info": JSON.stringify(clientInfo),
    ...(init?.headers as Record<string, string>),
  };

  if (!isFormData) {
    headers["Content-Type"] = "application/json";
  }

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

  const data = await res.json().catch(() => ({}));

  if (!res.ok) {
    const code: string = data.error ?? "internal_error";
    const retryAfterHeader = res.headers.get("Retry-After");
    const retryAfter: number | undefined = data.retry_after ?? (retryAfterHeader ? Number.parseInt(retryAfterHeader, 10) : undefined);

    if (res.status === 401 && code === "token_expired" && retry) {
      try {
        await refreshTokens();
        return request<T>(path, init, false);
      } catch (err) {
        if (err instanceof ApiError && !isAuthStatus(err.status)) {
          throw err;
        }
        if (!(err instanceof ApiError)) {
          throw err;
        }
        throw new ApiError("missing_token", undefined, 401);
      }
    }

    throw new ApiError(code, retryAfter, res.status);
  }

  return data as T;
}

export const api = {
  login: async (email: string, password: string) =>
    request<{ ok?: boolean; mfa_required?: boolean; mfa_token?: string }>("/api/v1/auth/login", {
      method: "POST",
      body: JSON.stringify({ email, password, client_info: await getClientInfo() }),
    }),

  mfaChallenge: (mfaToken: string, totpCode: string) =>
    request<{ ok: boolean }>("/api/v1/auth/mfa/challenge", {
      method: "POST",
      body: JSON.stringify({ mfa_token: mfaToken, totp_code: totpCode }),
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

  updateLocale: (locale: string) =>
    request<void>("/api/v1/me/locale", {
      method: "PATCH",
      body: JSON.stringify({ locale }),
    }),

  updateProfile: (payload: { first_name: string; last_name: string }) =>
    request<MeData>("/api/v1/me/profile", {
      method: "PATCH",
      body: JSON.stringify(payload),
    }),

  uploadAvatar: (file: File) => {
    const formData = new FormData();
    formData.append("avatar", file);
    return request<MeData>("/api/v1/me/avatar", {
      method: "POST",
      body: formData,
    });
  },

  deleteAvatar: () =>
    request<MeData>("/api/v1/me/avatar", { method: "DELETE" }),

  forgotPassword: (email: string) =>
    request<{ ok: boolean }>("/api/v1/auth/forgot-password", {
      method: "POST",
      body: JSON.stringify({ email }),
    }),

  resetPassword: (token: string, password: string) =>
    request<{ ok: boolean }>("/api/v1/auth/reset-password", {
      method: "POST",
      body: JSON.stringify({ token, password }),
    }),

  listSessions: () => request<Session[]>("/api/v1/sessions"),

  revokeSession: (id: string) =>
    request<void>(`/api/v1/sessions/${id}`, { method: "DELETE" }),

  revokeOtherSessions: () =>
    request<void>("/api/v1/sessions", { method: "DELETE" }),

  listAuditLog: (p: PageParams) =>
    request<PagedResult<AuditEntry>>(`/api/v1/admin/audit${buildQuery(p)}`),

  mfaStatus: () => request<{ enabled: boolean; supported?: boolean }>("/api/v1/mfa/status"),

  mfaSetup: () =>
    request<{ otp_auth_url: string; secret: string }>("/api/v1/mfa/setup", { method: "POST" }),

  mfaVerify: (code: string) =>
    request<{ ok: boolean }>("/api/v1/mfa/verify", {
      method: "POST",
      body: JSON.stringify({ code }),
    }),

  mfaDisable: (code: string) =>
    request<{ ok: boolean }>("/api/v1/mfa/disable", {
      method: "POST",
      body: JSON.stringify({ code }),
    }),

  mfaStepUp: (code: string) =>
    request<{ ok: boolean }>("/api/v1/mfa/step-up", {
      method: "POST",
      body: JSON.stringify({ code }),
    }),

  admin: {
    listUsers: (p: PageParams) =>
      request<PagedResult<UserData>>(`/api/v1/admin/users${buildQuery(p)}`),

    createUser: (payload: { email: string; first_name?: string; last_name?: string; password: string; locale: string; roles: string[] }) =>
      request<UserData>("/api/v1/admin/users", {
        method: "POST",
        body: JSON.stringify(payload),
      }),

    updateRoles: (userId: string, roles: string[]) =>
      request<void>(`/api/v1/admin/users/${userId}/roles`, {
        method: "PATCH",
        body: JSON.stringify({ roles }),
      }),

    setUserStatus: (userId: string, isActive: boolean) =>
      request<void>(`/api/v1/admin/users/${userId}/status`, {
        method: "PATCH",
        body: JSON.stringify({ is_active: isActive }),
      }),

    listUserSessions: (userId: string) =>
      request<Session[]>(`/api/v1/admin/users/${userId}/sessions`),

    revokeAllUserSessions: (userId: string) =>
      request<void>(`/api/v1/admin/users/${userId}/sessions`, { method: "DELETE" }),

    revokeUserSession: (userId: string, sessionId: string) =>
      request<void>(`/api/v1/admin/users/${userId}/sessions/${sessionId}`, { method: "DELETE" }),

    listServiceAccounts: (p: PageParams) =>
      request<PagedResult<ServiceAccount>>(`/api/v1/admin/service-accounts${buildQuery(p)}`),

    createServiceAccount: (payload: { name: string; scopes: string[]; expires_at?: string }) =>
      request<ServiceAccountCreated>("/api/v1/admin/service-accounts", {
        method: "POST",
        body: JSON.stringify(payload),
      }),

    updateServiceAccount: (id: string, payload: { name: string; scopes: string[]; expires_at?: string | null; is_active: boolean }) =>
      request<ServiceAccount>(`/api/v1/admin/service-accounts/${id}`, {
        method: "PATCH",
        body: JSON.stringify(payload),
      }),

    createServiceToken: async (payload: { client_id: string; client_secret: string }) => {
      const res = await fetch("/api/v1/auth/token", {
        method: "POST",
        credentials: "omit",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ grant_type: "client_credentials", ...payload }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new ApiError(data.error ?? "invalid_client");
      return data as ServiceTokenResponse;
    },

    probeWithServiceToken: async (path: string, token: string) => {
      const res = await fetch(path, {
        credentials: "omit",
        headers: { Authorization: `Bearer ${token}` },
      });
      const text = await res.text();
      let body: unknown = text;
      try {
        body = text ? JSON.parse(text) : null;
      } catch {
        body = text;
      }
      return {
        ok: res.ok,
        status: res.status,
        statusText: res.statusText,
        body,
      } satisfies ServiceProbeResult;
    },

    setServiceAccountStatus: (id: string, isActive: boolean) =>
      request<void>(`/api/v1/admin/service-accounts/${id}/status`, {
        method: "PATCH",
        body: JSON.stringify({ is_active: isActive }),
      }),

    revokeServiceAccount: (id: string) =>
      request<void>(`/api/v1/admin/service-accounts/${id}`, {
        method: "DELETE",
      }),

    getSettings: () =>
      request<Record<string, string>>("/api/v1/admin/settings"),

    updateSettings: (patch: Record<string, string>) =>
      request<void>("/api/v1/admin/settings", {
        method: "PATCH",
        body: JSON.stringify(patch),
      }),
  },
};
