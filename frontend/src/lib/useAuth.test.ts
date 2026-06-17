import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useAuth, authBootstrapFailureAction, classifyAuthBootstrapError, isAuthRedirectError } from "./useAuth";
import { ApiError, api } from "./api";

// Stub variables for mock React state
let stateCalls: any[] = [];
let callIdx = 0;

// Mock router navigation
const mockNavigate = vi.fn();
vi.mock("react-router-dom", () => ({
  useNavigate: () => mockNavigate,
}));

// Mock localization
const mockChangeLanguage = vi.fn();
const mockI18n = {
  language: "en",
  changeLanguage: mockChangeLanguage,
};
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    i18n: mockI18n,
    t: (key: string) => key,
  }),
}));

beforeEach(() => {
  vi.stubGlobal("localStorage", {
    setItem: vi.fn(),
    getItem: vi.fn(),
  });
});

// Mock React
vi.mock("react", async (importOriginal) => {
  const original = await importOriginal<typeof import("react")>();
  return {
    ...original,
    useState: (init: any) => {
      const idx = callIdx;
      callIdx++;
      if (!stateCalls[idx]) {
        stateCalls[idx] = [init, vi.fn()];
      }
      return stateCalls[idx];
    },
    useEffect: (fn: any) => {
      fn();
    },
  };
});

describe("auth bootstrap helper error handling", () => {
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

describe("useAuth React hook", () => {
  beforeEach(() => {
    callIdx = 0;
    mockNavigate.mockClear();
    mockChangeLanguage.mockClear();
    stateCalls = [
      [null, vi.fn()], // me
      [true, vi.fn()], // loading
      [null, vi.fn()], // bootstrapError
    ];
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("loads profile on mount, syncs mismatching locale, and sets state", async () => {
    const mockMe = vi.spyOn(api, "me").mockResolvedValue({
      user_id: "u1",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: false,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      locale: "tr", // mismatching locale
      roles: ["admin"],
    });

    useAuth();

    await vi.waitFor(() => {
      expect(mockMe).toHaveBeenCalled();
      expect(mockChangeLanguage).toHaveBeenCalledWith("tr");
      expect(stateCalls[0][1]).toHaveBeenCalledWith(expect.any(Object)); // setMe
      expect(stateCalls[1][1]).toHaveBeenCalledWith(false); // setLoading
    });
  });

  it("keeps the current language when the profile locale already matches", async () => {
    const mockMe = vi.spyOn(api, "me").mockResolvedValue({
      user_id: "u1",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: false,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      locale: "en",
      roles: ["admin"],
    });

    useAuth();

    await vi.waitFor(() => {
      expect(mockMe).toHaveBeenCalled();
      expect(stateCalls[0][1]).toHaveBeenCalledWith(expect.objectContaining({ locale: "en" }));
      expect(stateCalls[1][1]).toHaveBeenCalledWith(false);
    });
    expect(mockChangeLanguage).not.toHaveBeenCalled();
    expect(localStorage.setItem).not.toHaveBeenCalled();
  });

  it("does not sync language when the profile locale is empty", async () => {
    const mockMe = vi.spyOn(api, "me").mockResolvedValue({
      user_id: "u1",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: false,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      locale: "",
      roles: ["admin"],
    });

    useAuth();

    await vi.waitFor(() => {
      expect(mockMe).toHaveBeenCalled();
      expect(stateCalls[0][1]).toHaveBeenCalledWith(expect.objectContaining({ locale: "" }));
      expect(stateCalls[1][1]).toHaveBeenCalledWith(false);
    });
    expect(mockChangeLanguage).not.toHaveBeenCalled();
    expect(localStorage.setItem).not.toHaveBeenCalled();
  });

  it("redirects to login on auth failure (401/403)", async () => {
    const mockMe = vi.spyOn(api, "me").mockRejectedValue(new ApiError("unauthorized", undefined, 401));

    useAuth();

    await vi.waitFor(() => {
      expect(mockMe).toHaveBeenCalled();
      expect(mockNavigate).toHaveBeenCalledWith("/auth/login", { replace: true });
    });
  });

  it("sets bootstrap error on network/infrastructure failure", async () => {
    const mockMe = vi.spyOn(api, "me").mockRejectedValue(new ApiError("server_error", undefined, 500));

    useAuth();

    await vi.waitFor(() => {
      expect(mockMe).toHaveBeenCalled();
      expect(stateCalls[2][1]).toHaveBeenCalledWith("server"); // setBootstrapError
      expect(stateCalls[1][1]).toHaveBeenCalledWith(false); // setLoading
    });
  });
});
