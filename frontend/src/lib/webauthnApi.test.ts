import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "./api";

function jsonResponse(status: number, body: Record<string, unknown> | unknown[]): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("webauthn api methods", () => {
  beforeEach(() => {
    vi.stubGlobal("document", { cookie: "csrf_token=csrf123" });
  });
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("webauthnLoginBegin posts the mfa token and returns options", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, { publicKey: { challenge: "AQID" } }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await api.webauthnLoginBegin("tok");
    expect(res.publicKey.challenge).toBe("AQID");
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/auth/webauthn/login/begin");
    expect(JSON.parse(init.body)).toEqual({ mfa_token: "tok" });
  });

  it("webauthnLoginFinish posts the token and credential", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, { ok: true }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await api.webauthnLoginFinish("tok", { id: "abc" });
    expect(res).toEqual({ ok: true });
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/auth/webauthn/login/finish");
    expect(JSON.parse(init.body)).toEqual({ mfa_token: "tok", credential: { id: "abc" } });
  });

  it("webauthnPasswordlessBegin posts with no body and returns options + ceremony_id", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(
      jsonResponse(200, { publicKey: { challenge: "AQID" }, ceremony_id: "cer-1" }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const res = await api.webauthnPasswordlessBegin();
    expect(res.ceremony_id).toBe("cer-1");
    expect(res.publicKey.challenge).toBe("AQID");
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/auth/webauthn/passwordless/begin");
    expect(init.method).toBe("POST");
    expect(init.body).toBeUndefined();
  });

  it("webauthnPasswordlessFinish posts the ceremony id and credential", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, { ok: true }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await api.webauthnPasswordlessFinish("cer-1", { id: "abc" });
    expect(res).toEqual({ ok: true });
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/auth/webauthn/passwordless/finish");
    expect(JSON.parse(init.body)).toEqual({ ceremony_id: "cer-1", credential: { id: "abc" } });
  });

  it("webauthnList fetches the credentials", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(
      jsonResponse(200, { credentials: [{ id: "c1", name: "YubiKey", sign_count: 3, created_at: "2026-01-01", last_used_at: null }] }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const res = await api.webauthnList();
    expect(res.credentials).toHaveLength(1);
    expect(res.credentials[0].name).toBe("YubiKey");
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/webauthn/credentials");
  });

  it("webauthnRegisterBegin posts and returns creation options", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, { publicKey: { challenge: "AQID", user: { id: "AQID" } } }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await api.webauthnRegisterBegin();
    expect(res.publicKey.challenge).toBe("AQID");
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/webauthn/register/begin");
    expect(init.method).toBe("POST");
  });

  it("webauthnRegisterFinish posts name and credential", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, { ok: true }));
    vi.stubGlobal("fetch", fetchMock);

    await api.webauthnRegisterFinish("My Key", { id: "abc" });
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/webauthn/register/finish");
    expect(JSON.parse(init.body)).toEqual({ name: "My Key", credential: { id: "abc" } });
  });

  it("webauthnDeleteCredential issues a DELETE", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await api.webauthnDeleteCredential("cred-1");
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/webauthn/credentials/cred-1");
    expect(init.method).toBe("DELETE");
  });
});
