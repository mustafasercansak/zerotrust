import { scheduleRefresh } from "./tokenManager";
import { getClientInfo } from "./clientInfo";
import type { CredentialCreationOptionsJSON, CredentialRequestOptionsJSON } from "./webauthn";

export interface WebAuthnCredential {
  id: string;
  name: string;
  sign_count: number;
  created_at: string;
  last_used_at: string | null;
}

export interface UserMfaInfo {
  totp_enabled: boolean;
  webauthn_credentials: WebAuthnCredential[];
}

export interface MeData {
  user_id: string;
  email: string;
  first_name: string;
  last_name: string;
  has_avatar: boolean;
  locale: string;
  notify_security_emails?: boolean;
  roles: string[];
  permissions?: string[];
  created_at: string;
  updated_at: string;
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
  updated_at: string;
  active_sessions: number;
  mfa_enabled: boolean;
  passkey_count: number;
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

export interface OidcClient {
  id: string;
  client_id: string;
  client_secret?: string;
  name: string;
  redirect_uris: string[];
  allowed_scopes: string[];
  created_at: string;
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
    outcome?: string;
    status?: number;
    reason?: string;
    location?: { country?: string; city?: string };
    client_info?: {
      architecture?: string;
      browser?: string;
      browser_version?: string;
      mobile?: string;
      os?: string;
      os_version?: string;
    };
    [key: string]: unknown;
  } | null;
  created_at: string;
}

export interface AuditTrendPoint {
  date: string;
  success: number;
  failure: number;
}

export interface SecurityDashboardCount {
  name: string;
  count: number;
}

export interface SecurityDashboardData {
  range: "24h" | "7d" | "30d";
  since: string;
  generated_at: string;
  metrics: {
    successful_logins: number;
    failed_logins: number;
    lockouts: number;
    anomalies: number;
    active_sessions: number;
    average_risk_score: number;
  };
  auth_activity: Array<{
    bucket: string;
    success: number;
    failure: number;
    average_risk_score: number;
  }>;
  anomaly_breakdown: SecurityDashboardCount[];
  login_countries: SecurityDashboardCount[];
  failed_login_ips: SecurityDashboardCount[];
  blocked_countries: SecurityDashboardCount[];
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
    request<{ ok?: boolean; mfa_required?: boolean; mfa_token?: string; totp_enabled?: boolean; webauthn_enabled?: boolean; mfa_setup_secret?: string; mfa_setup_url?: string; mfa_recovery_codes?: string[] }>("/api/v1/auth/login", {
      method: "POST",
      body: JSON.stringify({ email, password, client_info: await getClientInfo() }),
    }),
  mfaChallenge: (mfaToken: string, totpCode: string) =>
    request<{ ok: boolean }>("/api/v1/auth/mfa/challenge", {
      method: "POST",
      body: JSON.stringify({ mfa_token: mfaToken, totp_code: totpCode }),
    }),

  // WebAuthn passkey login (second factor). Begin returns the assertion options
  // ({ publicKey: ... }); finish submits the signed assertion.
  webauthnLoginBegin: (mfaToken: string) =>
    request<CredentialRequestOptionsJSON>("/api/v1/auth/webauthn/login/begin", {
      method: "POST",
      body: JSON.stringify({ mfa_token: mfaToken }),
    }),
  webauthnLoginFinish: (mfaToken: string, credential: unknown) =>
    request<{ ok: boolean }>("/api/v1/auth/webauthn/login/finish", {
      method: "POST",
      body: JSON.stringify({ mfa_token: mfaToken, credential }),
    }),

  // Passwordless (usernameless) passkey login via discoverable credentials.
  // Begin returns assertion options plus an opaque ceremony_id that finish must
  // echo back; the authenticator reveals the user, so no password step is needed.
  webauthnPasswordlessBegin: () =>
    request<CredentialRequestOptionsJSON & { ceremony_id: string }>("/api/v1/auth/webauthn/passwordless/begin", {
      method: "POST",
    }),
  webauthnPasswordlessFinish: (ceremonyId: string, credential: unknown) =>
    request<{ ok: boolean }>("/api/v1/auth/webauthn/passwordless/finish", {
      method: "POST",
      body: JSON.stringify({ ceremony_id: ceremonyId, credential }),
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
  getOidcClientInfo: (clientId: string) =>
    request<{ name: string; allowed_scopes: string[] }>(`/oauth2/clients/${encodeURIComponent(clientId)}`),

  submitConsent: (payload: {
    client_id: string;
    redirect_uri: string;
    scopes: string[];
    code_challenge?: string;
    code_challenge_method?: string;
    nonce?: string;
    state?: string;
    approved: boolean;
  }) =>
    request<{ redirect_url: string }>("/api/v1/oauth2/consent", {
      method: "POST",
      body: JSON.stringify(payload),
    }),

  updateLocale: (locale: string) =>
    request<void>("/api/v1/me/locale", {
      method: "PATCH",
      body: JSON.stringify({ locale }),
    }),

  updateNotifications: (notifySecurityEmails: boolean) =>
    request<void>("/api/v1/me/notifications", {
      method: "PATCH",
      body: JSON.stringify({ notify_security_emails: notifySecurityEmails }),
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

  changePassword: (currentPassword: string, newPassword: string) =>
    request<void>("/api/v1/me/password", {
      method: "PATCH",
      body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
    }),

  getSessionPolicy: () =>
    request<{ idle_timeout_seconds: number }>("/api/v1/session/policy"),

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

  listMyAudit: (limit = 50, offset = 0) =>
    request<PagedResult<AuditEntry>>(`/api/v1/me/audit?limit=${limit}&offset=${offset}&sort_by=created_at&sort_dir=desc`),

  listAuditLog: (p: PageParams) =>
    request<PagedResult<AuditEntry>>(`/api/v1/admin/audit${buildQuery(p)}`),

  listAuditLogTrends: () =>
    request<AuditTrendPoint[]>("/api/v1/admin/audit/trends"),

  securityDashboard: (range: "24h" | "7d" | "30d") =>
    request<SecurityDashboardData>(`/api/v1/admin/security-dashboard?range=${range}`),

  mfaStatus: () => request<{ enabled: boolean; supported?: boolean }>("/api/v1/mfa/status"),

  mfaSetup: () =>
    request<{ otp_auth_url: string; secret: string; recovery_codes: string[] }>("/api/v1/mfa/setup", { method: "POST" }),

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

  mfaStepUp: (code: string, reason?: string) =>
    request<{ ok: boolean }>("/api/v1/mfa/step-up", {
      method: "POST",
      body: JSON.stringify({ code, ...(reason ? { reason } : {}) }),
    }),

  // WebAuthn passkey management (authenticated user).
  webauthnList: () =>
    request<{ credentials: WebAuthnCredential[] }>("/api/v1/webauthn/credentials"),

  webauthnRegisterBegin: () =>
    request<CredentialCreationOptionsJSON>("/api/v1/webauthn/register/begin", { method: "POST" }),

  webauthnRegisterFinish: (name: string, credential: unknown) =>
    request<{ ok: boolean }>("/api/v1/webauthn/register/finish", {
      method: "POST",
      body: JSON.stringify({ name, credential }),
    }),

  webauthnDeleteCredential: (id: string) =>
    request<void>(`/api/v1/webauthn/credentials/${id}`, { method: "DELETE" }),

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

    bulkSetUserStatus: (userIds: string[], isActive: boolean) =>
      request<void>("/api/v1/admin/users/bulk-status", {
        method: "POST",
        body: JSON.stringify({ user_ids: userIds, is_active: isActive }),
      }),

    listUserSessions: (userId: string) =>
      request<Session[]>(`/api/v1/admin/users/${userId}/sessions`),

    revokeAllUserSessions: (userId: string) =>
      request<void>(`/api/v1/admin/users/${userId}/sessions`, { method: "DELETE" }),

    revokeUserSession: (userId: string, sessionId: string) =>
      request<void>(`/api/v1/admin/users/${userId}/sessions/${sessionId}`, { method: "DELETE" }),

    listUserMfa: (userId: string) =>
      request<UserMfaInfo>(`/api/v1/admin/users/${userId}/mfa`),

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

    rotateServiceAccountSecret: (id: string) =>
      request<ServiceAccountCreated>(`/api/v1/admin/service-accounts/${id}/rotate`, {
        method: "POST",
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
      const body: unknown = (() => {
        try {
          return text ? JSON.parse(text) : null;
        } catch {
          return text;
        }
      })();
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

    testWebhook: (url?: string) =>
      request<void>("/api/v1/admin/settings/webhook/test", {
        method: "POST",
        body: JSON.stringify({ url: url ?? "" }),
      }),

    listOidcClients: () =>
      request<OidcClient[]>("/api/v1/admin/oidc/clients"),

    createOidcClient: (payload: { client_id: string; name: string; redirect_uris: string[]; allowed_scopes: string[] }) =>
      request<OidcClient>("/api/v1/admin/oidc/clients", {
        method: "POST",
        body: JSON.stringify(payload),
      }),

    deleteOidcClient: (id: string) =>
      request<void>(`/api/v1/admin/oidc/clients/${id}`, {
        method: "DELETE",
      }),

    updateOidcClient: (id: string, payload: { name: string; redirect_uris: string[]; allowed_scopes: string[] }) =>
      request<OidcClient>(`/api/v1/admin/oidc/clients/${id}`, {
        method: "PUT",
        body: JSON.stringify(payload),
      }),

    rotateOidcClientSecret: (id: string) =>
      request<{ client_secret: string }>(`/api/v1/admin/oidc/clients/${id}/rotate`, {
        method: "POST",
      }),

    securityPosture: () =>
      request<SecurityPostureData>("/api/v1/admin/security-posture"),

    health: () =>
      request<AdminHealthData>("/api/v1/admin/health"),

    auditExport: (params: { format: "csv" | "json"; action?: string; user_id?: string; outcome?: string }) => {
      const q = new URLSearchParams({ format: params.format });
      if (params.action) q.set("action", params.action);
      if (params.user_id) q.set("user_id", params.user_id);
      if (params.outcome) q.set("outcome", params.outcome);
      return fetch(`/api/v1/admin/audit/export?${q}`, {
        headers: { Accept: params.format === "json" ? "application/json" : "text/csv" },
        credentials: "include",
      });
    },
  },
};

export interface SecurityPostureData {
  total_users: number;
  users_without_mfa: number;
  users_inactive_30d: number;
}

export interface AdminHealthData {
  status: "ok" | "degraded";
  database: { status: string; pool: { total: number; idle: number; max: number } };
  redis: { status: string; pool: { total: number; idle: number; max: number } };
}
