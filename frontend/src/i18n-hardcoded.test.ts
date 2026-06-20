/**
 * Detects two categories of potential i18n violations in TSX source files:
 *
 * 1. Multi-word JSX text nodes between tags — e.g. <Typography>Some text</Typography>
 * 2. String literal values on user-facing props (placeholder, aria-label, emptyMessage)
 *    that look like natural language.
 *
 * Known-OK strings are listed in ALLOWED_PROP_STRINGS below.
 */

import { describe, expect, it } from "vitest";
import { readFileSync, readdirSync, statSync } from "fs";
import { join, resolve, dirname } from "path";
import { fileURLToPath } from "url";

const SRC_DIR = resolve(dirname(fileURLToPath(import.meta.url)));

// ── File collection ────────────────────────────────────────────────────────────

function gatherTsx(dir: string): string[] {
  const results: string[] = [];
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) {
      results.push(...gatherTsx(full));
    } else if (entry.endsWith(".tsx") && !entry.endsWith(".test.tsx")) {
      results.push(full);
    }
  }
  return results;
}

const TSX_FILES = [
  ...gatherTsx(join(SRC_DIR, "pages")),
  ...gatherTsx(join(SRC_DIR, "components")),
];

// ── Allowlist ──────────────────────────────────────────────────────────────────
// Prop string values that are intentionally hardcoded (technical hints, examples).

// Add strings here only when they are intentionally hardcoded (e.g. pure technical
// identifiers or format tokens that carry no translatable meaning).
const ALLOWED_PROP_STRINGS = new Set<string>([]);

// ── Detection helpers ──────────────────────────────────────────────────────────

interface Finding {
  file: string;
  line: number;
  text: string;
  kind: "jsx-text" | "prop-string";
}

// Matches multi-word text between a closing > and an opening < on the same line.
// Excludes lines that contain { or } (JSX expressions) and comment lines.
const JSX_TEXT_RE = />\s*([A-Za-zÀ-ɏ][A-Za-zÀ-ɏ]*(?:\s+[A-Za-zÀ-ɏ][A-Za-zÀ-ɏ]*)+)\s*</g;

// Matches string literal values on user-facing props.
const PROP_STRING_RE = /(?:placeholder|aria-label|emptyMessage)\s*=\s*"([^"]{4,})"/g;

function scanFile(filePath: string): Finding[] {
  const rel = filePath.replace(SRC_DIR + "/", "");
  const lines = readFileSync(filePath, "utf-8").split("\n");
  const findings: Finding[] = [];

  for (let i = 0; i < lines.length; i++) {
    const raw = lines[i];
    const trimmed = raw.trimStart();

    // Skip comment lines and import/export lines
    if (
      trimmed.startsWith("//") ||
      trimmed.startsWith("*") ||
      trimmed.startsWith("/*") ||
      trimmed.startsWith("import ") ||
      trimmed.startsWith("export ")
    ) {
      continue;
    }

    // 1 — JSX text nodes: only check lines that don't contain { or } so we
    //     don't accidentally match inside JSX expressions.
    if (!raw.includes("{") && !raw.includes("}")) {
      JSX_TEXT_RE.lastIndex = 0;
      let m: RegExpExecArray | null;
      while ((m = JSX_TEXT_RE.exec(raw)) !== null) {
        const text = m[1].trim();
        if (text.length >= 4) {
          findings.push({ file: rel, line: i + 1, text, kind: "jsx-text" });
        }
      }
    }

    // 2 — String literal prop values
    PROP_STRING_RE.lastIndex = 0;
    let m: RegExpExecArray | null;
    while ((m = PROP_STRING_RE.exec(raw)) !== null) {
      const text = m[1].trim();
      if (!ALLOWED_PROP_STRINGS.has(text)) {
        findings.push({ file: rel, line: i + 1, text, kind: "prop-string" });
      }
    }
  }

  return findings;
}

// ── Tests ──────────────────────────────────────────────────────────────────────

describe("Hardcoded text detection", () => {
  const allFindings = TSX_FILES.flatMap(scanFile);
  const jsxFindings = allFindings.filter((f) => f.kind === "jsx-text");
  const propFindings = allFindings.filter((f) => f.kind === "prop-string");

  const fmt = (f: Finding) => `  ${f.file}:${f.line}  →  "${f.text}"`;

  it("no multi-word hardcoded text in JSX nodes", () => {
    expect(
      jsxFindings,
      `Suspected hardcoded JSX text (use t() or add to allowlist):\n${jsxFindings.map(fmt).join("\n")}`,
    ).toHaveLength(0);
  });

  it("no hardcoded strings on user-facing props", () => {
    expect(
      propFindings,
      `Suspected hardcoded prop strings (use t() or add ALLOWED_PROP_STRINGS):\n${propFindings.map(fmt).join("\n")}`,
    ).toHaveLength(0);
  });
});
