import { describe, expect, it } from "vitest";
import { ApiError } from "./api";
import { authBootstrapFailureAction, classifyAuthBootstrapError, isAuthRedirectError } from "./useAuth";

describe("auth bootstrap error handling", () => {
  it.each([401, 403])("redirects HTTP %s auth failures to login", (status) => {
    expect(isAuthRedirectError(new ApiError("auth_error", undefined, status))).toBe(true);
  });

  it.each([400, 404, 429, 500, 503])("does not redirect HTTP %s non-auth failures", (status) => {
    expect(isAuthRedirectError(new ApiError("request_error", undefined, status))).toBe(false);
  });

  it.each([500, 502, 503])("shows server error state for HTTP %s bootstrap failures", (status) => {
    expect(classifyAuthBootstrapError(new ApiError("internal_error", undefined, status))).toBe("server");
  });

  it("shows network error state for fetch failures without an HTTP response", () => {
    expect(classifyAuthBootstrapError(new TypeError("Failed to fetch"))).toBe("network");
  });

  it("maps auth failures to the redirect action used by useAuth", () => {
    expect(authBootstrapFailureAction(new ApiError("missing_token", undefined, 401))).toEqual({ type: "redirect" });
  });

  it("maps infrastructure failures to the retryable error action used by useAuth", () => {
    expect(authBootstrapFailureAction(new ApiError("internal_error", undefined, 503))).toEqual({
      type: "error",
      error: "server",
    });
    expect(authBootstrapFailureAction(new TypeError("Failed to fetch"))).toEqual({
      type: "error",
      error: "network",
    });
  });
});
