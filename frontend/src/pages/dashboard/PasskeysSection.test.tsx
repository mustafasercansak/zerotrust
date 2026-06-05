import { afterEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { renderToString } from "react-dom/server";

// Return the translation key so assertions are locale-independent.
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

import PasskeysSection from "./PasskeysSection";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("PasskeysSection", () => {
  it("renders the heading and an unsupported notice when WebAuthn is unavailable", () => {
    // In the node test environment there is no window.PublicKeyCredential,
    // so the component should render its unsupported state without crashing.
    const html = renderToString(React.createElement(PasskeysSection));
    expect(html).toContain("passkeys.title");
    expect(html).toContain("passkeys.unsupported");
    // The "add" action is hidden when unsupported.
    expect(html).not.toContain("passkeys.add");
  });

  it("shows the add action when WebAuthn is supported", () => {
    vi.stubGlobal("window", { PublicKeyCredential: function () {} });
    vi.stubGlobal("navigator", { credentials: { create: vi.fn(), get: vi.fn() } });
    const html = renderToString(React.createElement(PasskeysSection));
    expect(html).toContain("passkeys.add");
  });
});
