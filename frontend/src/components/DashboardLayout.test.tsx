import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import DashboardLayout from "./DashboardLayout";
import { api } from "@/lib/api";
import { render, screen, fireEvent, waitFor, cleanup, act } from "@testing-library/react";

const mockNavigate = vi.fn();
let mockPathname = "/dashboard";
vi.mock("react-router-dom", () => ({
  Link: ({ children, to }: any) => React.createElement("a", { href: to }, children),
  useNavigate: () => mockNavigate,
  useLocation: () => ({ pathname: mockPathname }),
  Outlet: () => React.createElement("div", null, "Outlet"),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: {
      language: "en",
      changeLanguage: vi.fn(),
    },
  }),
}));

vi.mock("@/lib/tokenManager", () => ({
  cancelRefresh: vi.fn(),
}));

vi.mock("@/hooks/useIdleTimeout", () => ({
  useIdleTimeout: () => ({
    warningVisible: false,
    secondsRemaining: 60,
    extendSession: vi.fn(),
    dismissWarning: vi.fn(),
  }),
}));

vi.mock("@/components/SessionTimeoutDialog", () => ({
  SessionTimeoutDialog: () => null,
}));

// Mock useAuth
let mockLoading = false;
let mockBootstrapError: any = null;
let mockMeData: any = {
  user_id: "u123",
  email: "test@example.com",
  first_name: "John",
  last_name: "Doe",
  has_avatar: true,
  roles: ["admin"],
  locale: "en",
};
const mockSetMe = vi.fn();
const mockRetry = vi.fn();

vi.mock("@/lib/useAuth", () => ({
  useAuth: () => ({
    me: mockMeData,
    setMe: mockSetMe,
    loading: mockLoading,
    bootstrapError: mockBootstrapError,
    retry: mockRetry,
    localeWarning: false,
    dismissLocaleWarning: vi.fn(),
    anomalyWarning: false,
    dismissAnomalyWarning: vi.fn(),
  }),
}));

describe("DashboardLayout component", () => {
  beforeEach(() => {
    global.localStorage = {
      getItem: vi.fn(),
      setItem: vi.fn(),
      removeItem: vi.fn(),
      clear: vi.fn(),
      length: 0,
      key: vi.fn(),
    } as any;
    mockLoading = false;
    mockBootstrapError = null;
    mockMeData = {
      user_id: "u123",
      email: "test@example.com",
      first_name: "John",
      last_name: "Doe",
      has_avatar: true,
      roles: ["admin"],
      locale: "en",
    };
    mockNavigate.mockClear();
    mockSetMe.mockClear();
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("renders side navigation, Outlet, and user details", () => {
    render(React.createElement(DashboardLayout));
    expect(screen.getByText("Outlet")).toBeDefined();
    expect(screen.getByText("John Doe")).toBeDefined();
    expect(screen.getByText("test@example.com")).toBeDefined();
    expect(screen.getByText("security")).toBeDefined();
  });

  it("renders user initials when first/last name are empty", () => {
    mockMeData.first_name = "";
    mockMeData.last_name = "";
    mockMeData.has_avatar = false;
    render(React.createElement(DashboardLayout));
    expect(screen.getByText("TE")).toBeDefined(); // TE for test@example.com initials
  });

  it("renders loader screen when loading is true", () => {
    mockLoading = true;
    render(React.createElement(DashboardLayout));
    expect(screen.getByText("loading")).toBeDefined();
  });

  it("renders error retry screen when bootstrapError is present", () => {
    mockBootstrapError = "network";
    render(React.createElement(DashboardLayout));
    expect(screen.getByText("retry")).toBeDefined();
  });

  it("renders auth bootstrap fallback when the user is missing", () => {
    mockMeData = null;
    render(React.createElement(DashboardLayout));
    expect(screen.getByText("authBootstrap.network")).toBeDefined();
  });

  it("renders non-admin navigation without admin-only links", () => {
    mockMeData.roles = ["user"];
    mockPathname = "/dashboard/settings";
    render(React.createElement(DashboardLayout));

    expect(screen.getByText("settings")).toBeDefined();
    expect(screen.queryByText("security")).toBeNull();
    expect(screen.queryByText("serviceAccounts")).toBeNull();
  });

  it("handles profile, locale, and logout buttons correctly", async () => {
    const updateLocaleSpy = vi.spyOn(api, "updateLocale").mockResolvedValue({} as any);
    const logoutSpy = vi.spyOn(api, "logout").mockResolvedValue({} as any);

    render(React.createElement(DashboardLayout));

    // Profile Avatar button
    fireEvent.click(screen.getByText("John Doe"));
    expect(mockNavigate).toHaveBeenCalledWith("/dashboard/settings");

    // Logout button
    fireEvent.click(screen.getByText("logout"));
    expect(logoutSpy).toHaveBeenCalled();
    expect(mockNavigate).toHaveBeenCalledWith("/auth/login");

    // TR Locale button
    fireEvent.click(screen.getByText("TR"));
    expect(updateLocaleSpy).toHaveBeenCalledWith("tr");
  });

  it("skips same-locale changes and ignores locale update failures", async () => {
    const updateLocaleSpy = vi.spyOn(api, "updateLocale").mockRejectedValue(new Error("locale failed"));

    render(React.createElement(DashboardLayout));

    // EN is current, so clicking EN should skip updateLocale call
    fireEvent.click(screen.getByText("EN"));
    expect(updateLocaleSpy).not.toHaveBeenCalled();

    // TR is different, so clicking TR should trigger updateLocale call
    fireEvent.click(screen.getByText("TR"));
    expect(updateLocaleSpy).toHaveBeenCalledWith("tr");
    await waitFor(() => {
      expect(localStorage.setItem).toHaveBeenCalledWith("locale", "tr");
    });
  });

  it("handles me:updated custom window event", () => {
    render(React.createElement(DashboardLayout));

    const updatedData = { ...mockMeData, first_name: "Jane" };
    act(() => {
      window.dispatchEvent(new CustomEvent("me:updated", { detail: updatedData }));
    });

    expect(mockSetMe).toHaveBeenCalledWith(updatedData);
  });
});
