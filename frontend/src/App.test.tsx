import { describe, expect, it, vi } from "vitest";
import React from "react";
import App from "./App";
import { renderToString } from "react-dom/server";

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

describe("App main component", () => {
  it("renders without crashing", () => {
    const html = renderToString(React.createElement(App));
    expect(html).toContain("Toaster");
  });
});
