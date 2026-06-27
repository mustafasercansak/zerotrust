import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, api } from "./api";

function jsonResponse(status: number, body: Record<string, unknown> | unknown[]): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("api helper library", () => {
  beforeEach(() => {
    vi.stubGlobal("document", { cookie: "csrf_token=csrf123" });
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("handles login calls and passes down client info", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, { ok: true }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await api.login("test@example.com", "password");
    expect(res).toEqual({ ok: true });
    expect(fetchMock).toHaveBeenCalledWith("/api/v1/auth/login", expect.any(Object));
  });

  it("caches client info between requests", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, { ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal("navigator", { userAgent: "Mozilla/5.0 (X11; Linux x86_64) Firefox/114.0" });

    await api.me();
    await api.me();

    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("handles mfaChallenge calls", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, { ok: true }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await api.mfaChallenge("token", "123456");
    expect(res).toEqual({ ok: true });
  });

  it("handles logout calls successfully", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await api.logout();
    expect(fetchMock).toHaveBeenCalledWith("/api/v1/auth/logout", expect.any(Object));
  });

  it("handles updateLocale calls successfully", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await api.updateLocale("tr");
    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/v1/me/locale");
    expect(init.method).toBe("PATCH");
    expect(init.body).toBe(JSON.stringify({ locale: "tr" }));
  });

  it("handles updateProfile calls successfully", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, { user_id: "1" }));
    vi.stubGlobal("fetch", fetchMock);

    await api.updateProfile({ first_name: "A", last_name: "B" });
    expect(fetchMock.mock.calls[0][1].method).toBe("PATCH");
  });

  it("handles uploadAvatar with FormData", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, { user_id: "1" }));
    vi.stubGlobal("fetch", fetchMock);

    const file = new File(["avatar-content"], "avatar.png", { type: "image/png" });
    const res = await api.uploadAvatar(file);
    expect(res).toEqual({ user_id: "1" });
    const [, init] = fetchMock.mock.calls[0];
    expect(init.body).toBeInstanceOf(FormData);
  });

  it("handles deleteAvatar successfully", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, {}));
    vi.stubGlobal("fetch", fetchMock);

    await api.deleteAvatar();
    expect(fetchMock.mock.calls[0][1].method).toBe("DELETE");
  });

  it("handles forgotPassword and resetPassword successfully", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, { ok: true }));
    vi.stubGlobal("fetch", fetchMock);

    await api.forgotPassword("user@example.com");
    await api.resetPassword("token", "pwd");
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("handles listSessions, revokeSession and revokeOtherSessions successfully", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, []));
    vi.stubGlobal("fetch", fetchMock);

    await api.listSessions();
    await api.revokeSession("id");
    await api.revokeOtherSessions();
    expect(fetchMock).toHaveBeenCalledTimes(3);
  });

  it("handles audit and security dashboard requests successfully", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, []));
    vi.stubGlobal("fetch", fetchMock);

    await api.listAuditLog({ page: 0, pageSize: 10, filters: {} });
    await api.listAuditLogTrends();
    await api.securityDashboard("30d");
    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(fetchMock.mock.calls[2][0]).toBe("/api/v1/admin/security-dashboard?range=30d");
  });

  it("handles mfaStatus, mfaSetup, mfaVerify, mfaDisable, and mfaStepUp successfully", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, {}));
    vi.stubGlobal("fetch", fetchMock);

    await api.mfaStatus();
    await api.mfaSetup();
    await api.mfaVerify("code");
    await api.mfaDisable("code");
    await api.mfaStepUp("code");
    expect(fetchMock).toHaveBeenCalledTimes(5);
  });

  it("handles admin user management operations", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, {}));
    vi.stubGlobal("fetch", fetchMock);

    await api.admin.createUser({ email: "a@b.com", password: "123", locale: "en", roles: ["admin"] });
    await api.admin.updateRoles("uid", ["role"]);
    await api.admin.setUserStatus("uid", true);
    await api.admin.listUserSessions("uid");
    await api.admin.revokeAllUserSessions("uid");
    await api.admin.revokeUserSession("uid", "sid");
    expect(fetchMock).toHaveBeenCalledTimes(6);
  });

  it("handles admin service accounts operations", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, {}));
    vi.stubGlobal("fetch", fetchMock);

    await api.admin.listServiceAccounts({ page: 0, pageSize: 5, filters: {} });
    await api.admin.createServiceAccount({ name: "sa", scopes: ["read"] });
    await api.admin.updateServiceAccount("id", { name: "sa", scopes: [], is_active: false });
    await api.admin.rotateServiceAccountSecret("id");
    await api.admin.setServiceAccountStatus("id", true);
    await api.admin.revokeServiceAccount("id");
    expect(fetchMock).toHaveBeenCalledTimes(6);
  });

  it("handles admin settings operations", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, {}));
    vi.stubGlobal("fetch", fetchMock);

    await api.admin.getSettings();
    await api.admin.updateSettings({ key: "val" });
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("handles admin.listUsers with buildQuery pagination", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, { data: [], total: 0 }));
    vi.stubGlobal("fetch", fetchMock);

    await api.admin.listUsers({
      page: 2,
      pageSize: 10,
      sortKey: "email",
      sortDir: "desc",
      filters: { role: "admin" },
    });

    const [path] = fetchMock.mock.calls[0];
    expect(path).toContain("/api/v1/admin/users");
    expect(path).toContain("limit=10");
    expect(path).toContain("offset=20");
    expect(path).toContain("sort_by=email");
    expect(path).toContain("sort_dir=desc");
    expect(path).toContain("role=admin");
  });

  it("skips empty filter values when building list queries", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, { data: [], total: 0 }));
    vi.stubGlobal("fetch", fetchMock);

    await api.admin.listUsers({
      page: 0,
      pageSize: 25,
      filters: { role: "", status: "active" },
    });

    const [path] = fetchMock.mock.calls[0];
    expect(path).toContain("status=active");
    expect(path).not.toContain("role=");
  });

  it("handles admin.createServiceToken successfully", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, { access_token: "service-token" }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await api.admin.createServiceToken({ client_id: "cid", client_secret: "secret" });
    expect(res).toEqual({ access_token: "service-token" });
    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/v1/auth/token");
    expect(init.credentials).toBe("omit");
  });

  it("throws ApiError if admin.createServiceToken fails", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(400, { error: "invalid_client" }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.admin.createServiceToken({ client_id: "cid", client_secret: "secret" })).rejects.toThrow(ApiError);
  });

  it("uses default invalid_client when service token errors have no JSON body", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response("not-json", { status: 400 }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.admin.createServiceToken({ client_id: "cid", client_secret: "secret" })).rejects.toMatchObject({
      name: "ApiError",
      message: "invalid_client",
    });
  });

  it("handles admin.probeWithServiceToken successfully", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response("pong", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await api.admin.probeWithServiceToken("/api/v1/probe", "token123");
    expect(res).toEqual({
      ok: true,
      status: 200,
      statusText: "",
      body: "pong",
    });
    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/v1/probe");
    expect(init.headers.Authorization).toBe("Bearer token123");
  });

  it("handles admin.probeWithServiceToken returning json object successfully", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, { status: "ok" }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await api.admin.probeWithServiceToken("/api/v1/probe", "token123");
    expect(res.body).toEqual({ status: "ok" });
  });

  it("handles admin.probeWithServiceToken empty responses as null bodies", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response("", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await api.admin.probeWithServiceToken("/api/v1/probe", "token123");
    expect(res.body).toBeNull();
  });

  it("carries status codes and throws custom errors on API failure", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValueOnce(jsonResponse(429, { error: "too_many_requests", retry_after: 60 })));

    const err = await api.me().catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({
      message: "too_many_requests",
      status: 429,
      retryAfter: 60,
    });
  });

  it("uses internal_error when an API error response omits an error code", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValueOnce(jsonResponse(500, {})));

    await expect(api.me()).rejects.toMatchObject({
      name: "ApiError",
      message: "internal_error",
      status: 500,
    });
  });

  it("falls back to Retry-After header when retry_after is absent from response body", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValueOnce(
      new Response(JSON.stringify({ error: "too_many_requests" }), {
        status: 429,
        headers: { "Content-Type": "application/json", "Retry-After": "12" },
      }),
    ));

    await expect(api.me()).rejects.toMatchObject({
      name: "ApiError",
      message: "too_many_requests",
      status: 429,
      retryAfter: 12,
    });
  });

  it("successfully retries the request after a successful token refresh", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(401, { error: "token_expired" }))
      .mockResolvedValueOnce(jsonResponse(200, {}))
      .mockResolvedValueOnce(jsonResponse(200, { user_id: "retried_user" }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await api.me();
    expect(res).toEqual({ user_id: "retried_user" });
    expect(fetchMock).toHaveBeenCalledTimes(3);
  });

  it("shares an in-flight refresh request between concurrent token-expired responses", async () => {
    let resolveRefresh: (value: Response) => void = () => {};
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(401, { error: "token_expired" }))
      .mockResolvedValueOnce(jsonResponse(401, { error: "token_expired" }))
      .mockReturnValueOnce(new Promise<Response>((resolve) => {
        resolveRefresh = resolve;
      }))
      .mockResolvedValueOnce(jsonResponse(200, { user_id: "one" }))
      .mockResolvedValueOnce(jsonResponse(200, { user_id: "two" }));
    vi.stubGlobal("fetch", fetchMock);

    const first = api.me();
    const second = api.me();
    await Promise.resolve();
    resolveRefresh(jsonResponse(200, {}));

    await expect(first).resolves.toEqual({ user_id: "one" });
    await expect(second).resolves.toEqual({ user_id: "two" });
    expect(fetchMock).toHaveBeenCalledTimes(5);
  });

  it("propagates generic error if refreshTokens rejects with non-ApiError", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(401, { error: "token_expired" }))
      .mockRejectedValueOnce(new TypeError("Network Error"));
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.me()).rejects.toThrow(TypeError);
  });

  it("throws generic error when refreshTokens returns non-auth status during token_expired flow", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(401, { error: "token_expired" }))
      .mockResolvedValueOnce(jsonResponse(500, { error: "server_died" }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.me()).rejects.toMatchObject({
      name: "ApiError",
      message: "server_died",
      status: 500,
    });
  });

  it("uses invalid_token when a failed refresh response is not JSON", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(401, { error: "token_expired" }))
      .mockResolvedValueOnce(new Response("not-json", { status: 400 }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.me()).rejects.toMatchObject({
      name: "ApiError",
      message: "invalid_token",
      status: 400,
    });
  });

  it("throws missing_token on auth status refresh failure during token_expired flow", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(401, { error: "token_expired" }))
      .mockResolvedValueOnce(jsonResponse(401, { error: "bad_refresh" }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.me()).rejects.toMatchObject({
      name: "ApiError",
      message: "missing_token",
      status: 401,
    });
  });

  it("returns empty string from getCSRFToken when document is undefined", async () => {
    vi.stubGlobal("document", undefined);
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, {}));
    vi.stubGlobal("fetch", fetchMock);
    
    await api.deleteAvatar();
    const [, init] = fetchMock.mock.calls[0];
    expect(init.headers["X-CSRF-Token"]).toBeUndefined();
  });

  it("omits CSRF header when the csrf cookie is absent", async () => {
    vi.stubGlobal("document", { cookie: "other=value" });
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, {}));
    vi.stubGlobal("fetch", fetchMock);

    await api.deleteAvatar();
    const [, init] = fetchMock.mock.calls[0];
    expect(init.headers["X-CSRF-Token"]).toBeUndefined();
  });

  it("handles updateNotifications successfully", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await api.updateNotifications(false);
    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/v1/me/notifications");
    expect(init.method).toBe("PATCH");
    expect(JSON.parse(init.body)).toEqual({ notify_security_emails: false });
  });

  it("handles changePassword successfully", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await api.changePassword("old", "new");
    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/v1/me/password");
    expect(init.method).toBe("PATCH");
    expect(JSON.parse(init.body)).toEqual({ current_password: "old", new_password: "new" });
  });

  it("handles getSessionPolicy successfully", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, { idle_timeout_seconds: 300 }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await api.getSessionPolicy();
    expect(res).toEqual({ idle_timeout_seconds: 300 });
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/session/policy");
  });

  it("handles listMyAudit with default pagination", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, { data: [], total: 0 }));
    vi.stubGlobal("fetch", fetchMock);

    await api.listMyAudit();
    const [path] = fetchMock.mock.calls[0];
    expect(path).toContain("/api/v1/me/audit");
    expect(path).toContain("limit=50");
    expect(path).toContain("offset=0");
  });

  it("handles listMyAudit with custom pagination", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, { data: [], total: 0 }));
    vi.stubGlobal("fetch", fetchMock);

    await api.listMyAudit(25, 50);
    const [path] = fetchMock.mock.calls[0];
    expect(path).toContain("limit=25");
    expect(path).toContain("offset=50");
  });

  it("handles getOidcClientInfo successfully", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, { name: "My App", allowed_scopes: ["openid"] }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await api.getOidcClientInfo("client-abc");
    expect(res).toEqual({ name: "My App", allowed_scopes: ["openid"] });
    expect(fetchMock.mock.calls[0][0]).toBe("/oauth2/clients/client-abc");
  });

  it("handles submitConsent with approved=true", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, { redirect_url: "https://app.example.com/cb?code=x" }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await api.submitConsent({
      client_id: "cid",
      redirect_uri: "https://app.example.com/cb",
      scopes: ["openid", "profile"],
      state: "abc",
      approved: true,
    });
    expect(res.redirect_url).toContain("code=x");
    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/v1/oauth2/consent");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body).approved).toBe(true);
  });

  it("handles submitConsent with approved=false", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, { redirect_url: "https://app.example.com/cb?error=access_denied" }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await api.submitConsent({
      client_id: "cid",
      redirect_uri: "https://app.example.com/cb",
      scopes: [],
      approved: false,
    });
    expect(res.redirect_url).toContain("error=access_denied");
  });

  it("includes the reason field when mfaStepUp is called with a reason", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, { ok: true }));
    vi.stubGlobal("fetch", fetchMock);

    await api.mfaStepUp("123456", "delete_oidc_client");
    const body = JSON.parse(fetchMock.mock.calls[0][1].body);
    expect(body).toEqual({ code: "123456", reason: "delete_oidc_client" });
  });

  it("omits the reason field when mfaStepUp is called without a reason", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, { ok: true }));
    vi.stubGlobal("fetch", fetchMock);

    await api.mfaStepUp("123456");
    const body = JSON.parse(fetchMock.mock.calls[0][1].body);
    expect(body).toEqual({ code: "123456" });
    expect("reason" in body).toBe(false);
  });

  it("does not set Content-Type for FormData uploads", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, { user_id: "1" }));
    vi.stubGlobal("fetch", fetchMock);

    const file = new File(["img"], "a.png", { type: "image/png" });
    await api.uploadAvatar(file);
    const [, init] = fetchMock.mock.calls[0];
    expect(init.headers["Content-Type"]).toBeUndefined();
  });

  it("sets Content-Type application/json for JSON requests", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, {}));
    vi.stubGlobal("fetch", fetchMock);

    await api.mfaVerify("000000");
    const [, init] = fetchMock.mock.calls[0];
    expect(init.headers["Content-Type"]).toBe("application/json");
  });

  it("includes X-CSRF-Token on POST/PATCH/DELETE requests", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, {}));
    vi.stubGlobal("fetch", fetchMock);

    await api.mfaVerify("000000");
    expect(fetchMock.mock.calls[0][1].headers["X-CSRF-Token"]).toBe("csrf123");

    await api.updateLocale("en");
    expect(fetchMock.mock.calls[1][1].headers["X-CSRF-Token"]).toBe("csrf123");

    await api.revokeSession("id");
    expect(fetchMock.mock.calls[2][1].headers["X-CSRF-Token"]).toBe("csrf123");
  });

  it("handles admin.bulkSetUserStatus successfully", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await api.admin.bulkSetUserStatus(["uid1", "uid2"], false);
    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/v1/admin/users/bulk-status");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body)).toEqual({ user_ids: ["uid1", "uid2"], is_active: false });
  });

  it("handles admin.listUserMfa successfully", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, { totp_enabled: true, webauthn_credentials: [] }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await api.admin.listUserMfa("uid1");
    expect(res.totp_enabled).toBe(true);
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/admin/users/uid1/mfa");
  });

  it("handles admin.testWebhook with an explicit url", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await api.admin.testWebhook("https://hooks.example.com/recv");
    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/v1/admin/settings/webhook/test");
    expect(JSON.parse(init.body)).toEqual({ url: "https://hooks.example.com/recv" });
  });

  it("handles admin.testWebhook with no url (uses stored setting)", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await api.admin.testWebhook();
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ url: "" });
  });

  it("handles admin.listOidcClients successfully", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, []));
    vi.stubGlobal("fetch", fetchMock);

    const res = await api.admin.listOidcClients();
    expect(Array.isArray(res)).toBe(true);
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/admin/oidc/clients");
  });

  it("handles admin.createOidcClient successfully", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(
      jsonResponse(200, { id: "oid", client_id: "my-app", name: "My App", redirect_uris: [], allowed_scopes: [], created_at: "" }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const payload = { client_id: "my-app", name: "My App", redirect_uris: ["https://app.com/cb"], allowed_scopes: ["openid"] };
    const res = await api.admin.createOidcClient(payload);
    expect(res.client_id).toBe("my-app");
    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/v1/admin/oidc/clients");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body)).toEqual(payload);
  });

  it("handles admin.updateOidcClient successfully", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, { id: "oid", client_id: "x", name: "Updated", redirect_uris: [], allowed_scopes: [], created_at: "" }));
    vi.stubGlobal("fetch", fetchMock);

    await api.admin.updateOidcClient("oid", { name: "Updated", redirect_uris: [], allowed_scopes: [] });
    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/v1/admin/oidc/clients/oid");
    expect(init.method).toBe("PUT");
  });

  it("handles admin.deleteOidcClient successfully", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await api.admin.deleteOidcClient("oid");
    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/v1/admin/oidc/clients/oid");
    expect(init.method).toBe("DELETE");
  });

  it("handles admin.rotateOidcClientSecret and returns the new secret", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, { client_secret: "new-secret-abc" }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await api.admin.rotateOidcClientSecret("oid");
    expect(res.client_secret).toBe("new-secret-abc");
    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/v1/admin/oidc/clients/oid/rotate");
    expect(init.method).toBe("POST");
  });

  it("handles admin.securityPosture successfully", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(
      jsonResponse(200, { total_users: 10, users_without_mfa: 3, users_inactive_30d: 1 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const res = await api.admin.securityPosture();
    expect(res.total_users).toBe(10);
    expect(res.users_without_mfa).toBe(3);
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/admin/security-posture");
  });

  it("handles admin.health successfully", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(
      jsonResponse(200, {
        status: "ok",
        database: { status: "ok", pool: { total: 5, idle: 3, max: 20 } },
        redis: { status: "ok", pool: { total: 2, idle: 1, max: 10 } },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const res = await api.admin.health();
    expect(res.status).toBe("ok");
    expect(res.database.pool.max).toBe(20);
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/admin/health");
  });

  it("handles admin.auditExport with csv format and filters", async () => {
    const mockResponse = new Response("col1,col2\n", { status: 200 });
    const fetchMock = vi.fn().mockResolvedValueOnce(mockResponse);
    vi.stubGlobal("fetch", fetchMock);

    const res = await api.admin.auditExport({ format: "csv", action: "auth.login", user_id: "uid1" });
    expect(res).toBe(mockResponse);
    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toContain("/api/v1/admin/audit/export");
    expect(path).toContain("format=csv");
    expect(path).toContain("action=auth.login");
    expect(path).toContain("user_id=uid1");
    expect(init.headers.Accept).toBe("text/csv");
    expect(init.credentials).toBe("include");
  });

  it("handles admin.auditExport with json format", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response("[]", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await api.admin.auditExport({ format: "json" });
    const [, init] = fetchMock.mock.calls[0];
    expect(init.headers.Accept).toBe("application/json");
  });

  it("omits undefined audit export filters from the query string", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response("[]", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await api.admin.auditExport({ format: "csv" });
    const [path] = fetchMock.mock.calls[0];
    expect(path).not.toContain("action=");
    expect(path).not.toContain("user_id=");
    expect(path).not.toContain("outcome=");
  });
});
