import { describe, expect, it, vi } from "vitest";
import React from "react";
import { renderToString } from "react-dom/server";

vi.mock("./OidcClientsSection", () => ({
  default: () => React.createElement("div", null, "OidcClientsSectionMock")
}));

import OidcClientsPage from "./OidcClientsPage";

describe("OidcClientsPage", () => {
  it("renders correctly", () => {
    const html = renderToString(React.createElement(OidcClientsPage));
    expect(html).toContain("OidcClientsSectionMock");
  });
});
