import { describe, expect, it } from "vitest";
import type { Session } from "./api";
import { formatSessionDevice } from "./sessionUtils";

const base: Session = {
  id: "s1",
  ip_address: "10.0.0.1",
  user_agent: "",
  device_info: null,
  created_at: "2026-01-01T00:00:00Z",
  last_used_at: null,
  is_current: false,
};

describe("formatSessionDevice", () => {
  describe("structured device_info path", () => {
    it("returns browser+major-version — os+version+arch when all fields present", () => {
      const s: Session = {
        ...base,
        device_info: { browser: "Chrome", browser_version: "120.0.1", os: "macOS", os_version: "14.0", architecture: "arm64" },
      };
      expect(formatSessionDevice(s)).toBe("Chrome 120 — macOS 14.0 arm64");
    });

    it("omits architecture when absent", () => {
      const s: Session = {
        ...base,
        device_info: { browser: "Firefox", browser_version: "121.0", os: "Windows", os_version: "11" },
      };
      expect(formatSessionDevice(s)).toBe("Firefox 121 — Windows 11");
    });

    it("returns browser label without version when browser_version absent", () => {
      const s: Session = { ...base, device_info: { browser: "Edge", os: "Windows" } };
      expect(formatSessionDevice(s)).toBe("Edge — Windows");
    });

    it("returns browser only when os fields absent", () => {
      const s: Session = { ...base, device_info: { browser: "Safari", browser_version: "17.0" } };
      expect(formatSessionDevice(s)).toBe("Safari 17");
    });

    it("returns os only when browser absent", () => {
      const s: Session = { ...base, device_info: { os: "Linux" } };
      expect(formatSessionDevice(s)).toBe("Linux");
    });

    it("returns Unknown device when device_info is empty object", () => {
      const s: Session = { ...base, device_info: {} };
      expect(formatSessionDevice(s)).toBe("Unknown device");
    });
  });

  describe("UA-string fallback (device_info is null)", () => {
    const ua = (str: string): Session => ({ ...base, user_agent: str });

    it("identifies Chrome on macOS", () => {
      const result = formatSessionDevice(
        ua("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
      );
      expect(result).toBe("Chrome — macOS");
    });

    it("identifies Firefox on Windows", () => {
      const result = formatSessionDevice(
        ua("Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0")
      );
      expect(result).toBe("Firefox — Windows");
    });

    it("identifies Edge on Windows (not Chrome)", () => {
      const result = formatSessionDevice(
        ua("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0")
      );
      expect(result).toBe("Edge — Windows");
    });

    it("identifies Opera on Windows", () => {
      const result = formatSessionDevice(
        ua("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 OPR/104.0.0.0")
      );
      expect(result).toBe("Opera — Windows");
    });

    it("identifies Safari on iOS", () => {
      const result = formatSessionDevice(
        ua("Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1")
      );
      expect(result).toBe("Safari — iOS");
    });

    it("identifies Android with no recognized browser", () => {
      const result = formatSessionDevice(
        ua("Dalvik/2.1.0 (Linux; U; Android 13; Pixel 7 Build/TQ3A)")
      );
      expect(result).toBe("Android");
    });

    it("identifies Linux with no recognized browser", () => {
      const result = formatSessionDevice(
        ua("Mozilla/5.0 (X11; Linux x86_64) Gecko/20100101")
      );
      expect(result).toBe("Linux");
    });

    it("returns Unknown device for empty user_agent", () => {
      expect(formatSessionDevice({ ...base, user_agent: "" })).toBe("Unknown device");
    });
  });
});
