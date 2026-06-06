import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import DashboardLayout from "./DashboardLayout";
import { api } from "@/lib/api";
import { renderToString } from "react-dom/server";

// State Mocking System
let stateStore: any = {};
let stateSetters: any = {};
let callIdx = 0;
let effectCleanups: Array<() => void> = [];

vi.mock("react", async (importOriginal) => {
  const original = await importOriginal<typeof import("react")>();
  return {
    ...original,
    useState: (init: any) => {
      const idx = callIdx;
      callIdx++;
      if (!(idx in stateStore)) {
        stateStore[idx] = init;
      }
      stateSetters[idx] = (newVal: any) => {
        if (typeof newVal === "function") {
          stateStore[idx] = newVal(stateStore[idx]);
        } else {
          stateStore[idx] = newVal;
        }
      };
      if (callIdx >= 20) {
        callIdx = 0;
      }
      return [stateStore[idx], stateSetters[idx]];
    },
    useEffect: (fn: any) => {
      const cleanup = fn();
      if (typeof cleanup === "function") effectCleanups.push(cleanup);
    },
  };
});

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
  }),
}));

let capturedClickLogout: any = null;
const capturedClickLocales: Record<string, any> = {};
let capturedClickProfile: any = null;

vi.mock("@mui/material/Button", () => ({
  default: (props: any) => {
    if (props.onClick) {
      if (props.children === "logout") {
        capturedClickLogout = props.onClick;
      } else if (props.children === "EN" || props.children === "TR") {
        capturedClickLocales[props.children] = props.onClick;
      } else {
        capturedClickProfile = props.onClick;
      }
    }
    return React.createElement("button", { onClick: props.onClick }, props.children);
  },
}));

describe("DashboardLayout component", () => {
  let capturedMeUpdatedListener: any = null;

  beforeEach(() => {
    stateStore = {};
    stateSetters = {};
    callIdx = 0;
    effectCleanups = [];
    capturedMeUpdatedListener = null;

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
    capturedClickLogout = null;
    for (const key in capturedClickLocales) {
      delete capturedClickLocales[key];
    }
    capturedClickProfile = null;
    vi.clearAllMocks();

    vi.stubGlobal("window", {
      addEventListener: vi.fn((event, cb) => {
        if (event === "me:updated") capturedMeUpdatedListener = cb;
      }),
      removeEventListener: vi.fn(),
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  const runRender = () => {
    callIdx = 0;
    return renderToString(React.createElement(DashboardLayout));
  };

  it("renders side navigation, Outlet, and user details", () => {
    const html = runRender();
    expect(html).toContain("Outlet");
    expect(html).toContain("John Doe");
    expect(html).toContain("test@example.com");
    expect(html).toContain("security");
  });

  it("renders user initials when first/last name are empty", () => {
    mockMeData.first_name = "";
    mockMeData.last_name = "";
    mockMeData.has_avatar = false;
    const html = runRender();
    expect(html).toContain("TE"); // TE for test@example.com initials
  });

  it("renders loader screen when loading is true", () => {
    mockLoading = true;
    const html = runRender();
    expect(html).toContain("loading");
  });

  it("renders error retry screen when bootstrapError is present", () => {
    mockBootstrapError = "network";
    const html = runRender();
    expect(html).toContain("retry");
  });

  it("renders auth bootstrap fallback when the user is missing", () => {
    mockMeData = null;
    const html = runRender();
    expect(html).toContain("authBootstrap.network");
  });

  it("renders non-admin navigation without admin-only links", () => {
    mockMeData.roles = ["user"];
    mockPathname = "/dashboard/settings";
    const html = runRender();

    expect(html).toContain("settings");
    expect(html).not.toContain("security");
    expect(html).not.toContain("serviceAccounts");
  });

  it("handles profile, locale, and logout buttons correctly", async () => {
    const updateLocaleSpy = vi.spyOn(api, "updateLocale").mockResolvedValue({} as any);
    const logoutSpy = vi.spyOn(api, "logout").mockResolvedValue({} as any);

    runRender();

    expect(capturedClickProfile).toBeDefined();
    capturedClickProfile();
    expect(mockNavigate).toHaveBeenCalledWith("/dashboard/settings");

    expect(capturedClickLogout).toBeDefined();
    capturedClickLogout();
    expect(logoutSpy).toHaveBeenCalled();
    expect(mockNavigate).toHaveBeenCalledWith("/auth/login");

    expect(capturedClickLocales["TR"]).toBeDefined();
    await capturedClickLocales["TR"]();
    expect(updateLocaleSpy).toHaveBeenCalledWith("tr");
  });

  it("skips same-locale changes and ignores locale update failures", async () => {
    const updateLocaleSpy = vi.spyOn(api, "updateLocale").mockRejectedValue(new Error("locale failed"));

    runRender();

    await capturedClickLocales["EN"]();
    expect(updateLocaleSpy).not.toHaveBeenCalled();

    await capturedClickLocales["TR"]();
    expect(updateLocaleSpy).toHaveBeenCalledWith("tr");
    expect(localStorage.setItem).toHaveBeenCalledWith("locale", "tr");
  });

  it("handles me:updated custom window event", () => {
    runRender();
    expect(capturedMeUpdatedListener).toBeDefined();

    const updatedData = { ...mockMeData, first_name: "Jane" };
    capturedMeUpdatedListener({ detail: updatedData });

    expect(mockSetMe).toHaveBeenCalledWith(updatedData);
  });

  it("removes the me:updated listener on cleanup", () => {
    runRender();
    expect(effectCleanups[0]).toBeDefined();

    effectCleanups[0]();

    expect(window.removeEventListener).toHaveBeenCalledWith("me:updated", expect.any(Function));
  });
});
