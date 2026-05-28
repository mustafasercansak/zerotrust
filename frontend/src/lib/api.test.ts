import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, api } from "./api";

function jsonResponse(status: number, body: Record<string, unknown>): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("api auth refresh error handling", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("preserves network failures during token refresh", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(401, { error: "token_expired" }))
      .mockRejectedValueOnce(new TypeError("Failed to fetch"));
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.me()).rejects.toThrow(TypeError);
  });

  it("preserves server failures during token refresh", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(401, { error: "token_expired" }))
      .mockResolvedValueOnce(jsonResponse(503, { error: "internal_error" }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.me()).rejects.toMatchObject({ name: "ApiError", message: "internal_error", status: 503 });
  });

  it("converts refresh auth failures into a login redirectable error", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(401, { error: "token_expired" }))
      .mockResolvedValueOnce(jsonResponse(401, { error: "missing_token" }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.me()).rejects.toMatchObject({ name: "ApiError", message: "missing_token", status: 401 });
  });

  it("carries HTTP status on direct API errors", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValueOnce(jsonResponse(403, { error: "forbidden" })));

    const err = await api.me().catch((caught: unknown) => caught);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ message: "forbidden", status: 403 });
  });
});
