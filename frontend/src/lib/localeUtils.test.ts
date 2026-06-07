import { describe, expect, it } from "vitest";
import { getMuiDataGridLocale } from "./localeUtils";
import { trTR, enUS } from "@mui/x-data-grid/locales";

describe("localeUtils", () => {
  it("defaults to enUS if language is undefined", () => {
    expect(getMuiDataGridLocale()).toBe(enUS);
  });

  it("returns trTR for Turkish locale code", () => {
    expect(getMuiDataGridLocale("tr")).toBe(trTR);
    expect(getMuiDataGridLocale("tr-TR")).toBe(trTR);
  });

  it("returns enUS for fallback/unsupported locales", () => {
    expect(getMuiDataGridLocale("fr")).toBe(enUS);
    expect(getMuiDataGridLocale("en")).toBe(enUS);
    expect(getMuiDataGridLocale("en-US")).toBe(enUS);
  });
});
