import { beforeEach, describe, expect, it, vi } from "vitest";

describe("i18n configuration", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  it("initializes with the correct language from localStorage", async () => {
    vi.stubGlobal("localStorage", {
      getItem: vi.fn().mockReturnValue("en"),
      setItem: vi.fn(),
    });
    const i18n = (await import("./i18n")).default;
    expect(i18n.language).toBe("en");
  });

  it("initializes with fallback language when localStorage has no saved locale", async () => {
    vi.stubGlobal("localStorage", {
      getItem: vi.fn().mockReturnValue(null),
      setItem: vi.fn(),
    });
    const i18n = (await import("./i18n")).default;
    expect(i18n.language).toBe("tr");
  });
});
