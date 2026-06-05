import { describe, expect, it } from "vitest";
import React from "react";
import { DashboardPage } from "./DashboardPage";
import { renderToString } from "react-dom/server";

describe("DashboardPage component", () => {
  it("renders children and action correctly when access is not denied", () => {
    const html = renderToString(
      React.createElement(
        DashboardPage,
        {
          action: React.createElement("button", null, "Page Action"),
          children: React.createElement("div", null, "Dashboard Content"),
        }
      )
    );

    expect(html).toContain("Dashboard Content");
    expect(html).toContain("Page Action");
    expect(html).not.toContain("error");
  });

  it("renders access denied message when access is denied", () => {
    const html = renderToString(
      React.createElement(
        DashboardPage,
        {
          accessDenied: true,
          accessDeniedMessage: "You do not have permission to view this page",
          children: React.createElement("div", null, "Secret Dashboard Content"),
        }
      )
    );

    expect(html).toContain("You do not have permission to view this page");
    expect(html).not.toContain("Secret Dashboard Content");
  });
});
