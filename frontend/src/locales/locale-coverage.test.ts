import { describe, expect, it } from "vitest";
import en from "./en.json";
import tr from "./tr.json";

function collectLeafKeys(obj: unknown, prefix = ""): string[] {
  if (typeof obj !== "object" || obj === null || Array.isArray(obj)) {
    return prefix ? [prefix] : [];
  }
  return Object.entries(obj as Record<string, unknown>).flatMap(([key, val]) => {
    const path = prefix ? `${prefix}.${key}` : key;
    return typeof val === "object" && val !== null && !Array.isArray(val)
      ? collectLeafKeys(val, path)
      : [path];
  });
}

function getValue(obj: unknown, path: string): unknown {
  return path.split(".").reduce((o: unknown, k) => (o as Record<string, unknown>)?.[k], obj);
}

describe("Locale parity: en ↔ tr", () => {
  const enKeys = new Set(collectLeafKeys(en));
  const trKeys = new Set(collectLeafKeys(tr));

  it("every key in en.json exists in tr.json", () => {
    const missing = [...enKeys].filter((k) => !trKeys.has(k));
    expect(missing, `Keys in en.json missing from tr.json:\n${missing.join("\n")}`).toHaveLength(0);
  });

  it("every key in tr.json exists in en.json", () => {
    const missing = [...trKeys].filter((k) => !enKeys.has(k));
    expect(missing, `Keys in tr.json missing from en.json:\n${missing.join("\n")}`).toHaveLength(0);
  });

  it("no empty string values in en.json", () => {
    const empty = [...enKeys].filter((k) => getValue(en, k) === "");
    expect(empty, `Empty values in en.json:\n${empty.join("\n")}`).toHaveLength(0);
  });

  it("no empty string values in tr.json", () => {
    const empty = [...trKeys].filter((k) => getValue(tr, k) === "");
    expect(empty, `Empty values in tr.json:\n${empty.join("\n")}`).toHaveLength(0);
  });
});
