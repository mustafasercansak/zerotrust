import { describe, expect, it } from "vitest";
import { formatDate, formatDateTime } from "./dateUtils";

describe("dateUtils", () => {
  describe("formatDate", () => {
    it("returns fallback when iso string is empty, null, or undefined", () => {
      expect(formatDate(undefined, "en-US")).toBe("—");
      expect(formatDate(null, "en-US")).toBe("—");
      expect(formatDate("", "en-US")).toBe("—");
      expect(formatDate(undefined, "en-US", "N/A")).toBe("N/A");
    });

    it("returns fallback when date is invalid", () => {
      expect(formatDate("invalid-date", "en-US")).toBe("—");
      expect(formatDate("invalid-date", "en-US", "custom")).toBe("custom");
    });

    it("formats valid date correctly according to locale", () => {
      const iso = "2026-06-04T12:00:00Z";
      // Format should yield MM/DD/YYYY for en-US
      const formattedEn = formatDate(iso, "en-US");
      expect(formattedEn).toContain("06");
      expect(formattedEn).toContain("04");
      expect(formattedEn).toContain("2026");
    });
  });

  describe("formatDateTime", () => {
    it("returns fallback when iso string is empty, null, or undefined", () => {
      expect(formatDateTime(undefined, "en-US")).toBe("—");
      expect(formatDateTime(null, "en-US")).toBe("—");
      expect(formatDateTime("", "en-US")).toBe("—");
      expect(formatDateTime(undefined, "en-US", "N/A")).toBe("N/A");
    });

    it("returns fallback when date is invalid", () => {
      expect(formatDateTime("invalid-date", "en-US")).toBe("—");
      expect(formatDateTime("invalid-date", "en-US", "custom")).toBe("custom");
    });

    it("formats valid date and time correctly according to locale", () => {
      const iso = "2026-06-04T12:00:00Z";
      const formattedEn = formatDateTime(iso, "en-US");
      expect(formattedEn).toContain("06");
      expect(formattedEn).toContain("04");
      expect(formattedEn).toContain("2026");
    });
  });
});
