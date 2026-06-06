import { describe, expect, it, vi } from "vitest";
import React from "react";
import App from "./App";
import { renderToString } from "react-dom/server";

const lazyLoaders = vi.hoisted(() => [] as Array<() => Promise<unknown>>);

vi.mock("react", async (importOriginal) => {
  const original = await importOriginal<typeof import("react")>();
  return {
    ...original,
    lazy: (loader: () => Promise<unknown>) => {
      lazyLoaders.push(loader);
      return () => original.createElement("div", null, "LazyRoute");
    },
  };
});

vi.mock("react-router-dom", () => ({
  BrowserRouter: (props: any) => React.createElement("div", null, props.children),
  Routes: (props: any) => React.createElement("div", null, props.children),
  Route: (props: any) => React.createElement("div", null, props.element),
  Navigate: (props: any) => React.createElement("div", null, `Navigate: ${props.to}`),
}));

vi.mock("sonner", () => ({
  Toaster: () => React.createElement("div", null, "Toaster"),
}));

vi.mock("./components/TokenRefreshProvider", () => ({
  default: (props: any) => React.createElement("div", null, props.children),
}));

vi.mock("./components/DashboardLayout", () => ({
  default: () => React.createElement("div", null, "DashboardLayout"),
}));

vi.mock("./pages/auth/LoginPage", () => ({
  default: () => React.createElement("div", null, "LoginPage"),
}));

vi.mock("./pages/auth/ForgotPasswordPage", () => ({
  default: () => React.createElement("div", null, "ForgotPasswordPage"),
}));

vi.mock("./pages/auth/ResetPasswordPage", () => ({
  default: () => React.createElement("div", null, "ResetPasswordPage"),
}));

vi.mock("./pages/dashboard/HomePage", () => ({
  default: () => React.createElement("div", null, "HomePage"),
}));

vi.mock("./pages/dashboard/SessionsPage", () => ({
  default: () => React.createElement("div", null, "SessionsPage"),
}));

vi.mock("./pages/dashboard/UsersPage", () => ({
  default: () => React.createElement("div", null, "UsersPage"),
}));

vi.mock("./pages/dashboard/AuditPage", () => ({
  default: () => React.createElement("div", null, "AuditPage"),
}));

vi.mock("./pages/dashboard/ServiceAccountsPage", () => ({
  default: () => React.createElement("div", null, "ServiceAccountsPage"),
}));

vi.mock("./pages/dashboard/SettingsPage", () => ({
  default: () => React.createElement("div", null, "SettingsPage"),
}));

vi.mock("./pages/dashboard/MfaPage", () => ({
  default: () => React.createElement("div", null, "MfaPage"),
}));

describe("App main component", () => {
  it("renders without crashing", () => {
    const html = renderToString(React.createElement(App));
    expect(html).toContain("Toaster");
  });

  it("registers and resolves all lazy routes", async () => {
    expect(lazyLoaders).toHaveLength(11);

    const modules = await Promise.all(lazyLoaders.map((load) => load()));

    expect(modules).toHaveLength(11);
    for (const mod of modules) {
      expect(mod).toHaveProperty("default");
    }
  });
});
