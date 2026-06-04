import { describe, expect, it } from "vitest";
import theme from "./theme";

describe("theme configuration", () => {
  it("has a dark palette mode", () => {
    expect(theme.palette.mode).toBe("dark");
  });

  it("defines the primary main color", () => {
    expect(theme.palette.primary.main).toBe("#6366f1");
  });

  it("defines expected custom shapes and typography", () => {
    expect(theme.shape.borderRadius).toBe(8);
    expect(theme.typography.fontFamily).toContain("Arial");
  });
});
