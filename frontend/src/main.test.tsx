import { afterEach, describe, expect, it, vi } from "vitest";
import React from "react";

const renderMock = vi.fn();
const createRootMock = vi.fn(() => ({ render: renderMock }));

vi.mock("react-dom/client", () => ({
  default: { createRoot: createRootMock },
  createRoot: createRootMock,
}));

vi.mock("./App", () => ({
  default: () => React.createElement("div", null, "App"),
}));

vi.mock("./i18n", () => ({}));

describe("main bootstrap", () => {
  afterEach(() => {
    vi.resetModules();
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("mounts the React application into the root element", async () => {
    const root = { id: "root" };
    vi.stubGlobal("document", {
      getElementById: vi.fn().mockReturnValue(root),
    });

    await import("./main");

    expect(document.getElementById).toHaveBeenCalledWith("root");
    expect(createRootMock).toHaveBeenCalledWith(root);
    expect(renderMock).toHaveBeenCalled();
  });
});
