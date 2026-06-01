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

  it("handles listAuditLog and listAuditLogTrends successfully", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, []));
    vi.stubGlobal("fetch", fetchMock);

    await api.listAuditLog({ page: 0, pageSize: 10, filters: {} });
    await api.listAuditLogTrends();
    expect(fetchMock).toHaveBeenCalledTimes(2);
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
});
