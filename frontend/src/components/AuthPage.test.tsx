import { describe, expect, it } from "vitest";
import React from "react";
import { AuthPage } from "./AuthPage";
import { renderToString } from "react-dom/server";

describe("AuthPage component", () => {
  it("renders title, subtitle and children correctly", () => {
    const title = "Sign In";
    const subtitle = "Please login to your account";
    const children = React.createElement("div", null, "Form Content");

    const html = renderToString(
      React.createElement(
        AuthPage,
        { title, subtitle },
        children
      )
    );

    expect(html).toContain("Sign In");
    expect(html).toContain("Please login to your account");
    expect(html).toContain("Form Content");
  });

  it("renders without subtitle successfully", () => {
    const html = renderToString(
      React.createElement(
        AuthPage,
        { title: "Only Title" },
        React.createElement("div", null, "Child")
      )
    );
    expect(html).toContain("Only Title");
  });
});
