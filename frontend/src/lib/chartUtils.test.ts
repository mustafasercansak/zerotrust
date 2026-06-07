import { describe, expect, it } from "vitest";
import { getBezierPath, getBezierAreaPath } from "./chartUtils";

describe("chartUtils", () => {
  describe("getBezierPath", () => {
    it("returns empty string for empty array", () => {
      expect(getBezierPath([])).toBe("");
    });

    it("returns move command for a single point", () => {
      expect(getBezierPath([{ x: 10, y: 20 }])).toBe("M 10 20");
    });

    it("creates a bezier curve for multiple points", () => {
      const points = [
        { x: 10, y: 20 },
        { x: 40, y: 80 },
      ];
      const result = getBezierPath(points);
      expect(result).toContain("M 10 20");
      expect(result).toContain("C 20 20, 30 80, 40 80");
    });
  });

  describe("getBezierAreaPath", () => {
    it("returns empty string for empty array", () => {
      expect(getBezierAreaPath([], 100)).toBe("");
    });

    it("creates a closed area path to the fillHeight", () => {
      const points = [
        { x: 10, y: 20 },
        { x: 40, y: 80 },
      ];
      const result = getBezierAreaPath(points, 100);
      expect(result).toContain("M 10 20");
      expect(result).toContain("C 20 20, 30 80, 40 80");
      expect(result).toContain("L 40 100");
      expect(result).toContain("L 10 100 Z");
    });
  });
});
