import { afterEach, describe, expect, it, vi } from "vitest";
import { getClientInfo } from "./clientInfo";

describe("getClientInfo client info utility", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("returns empty object when navigator is undefined (SSR safety)", async () => {
    vi.stubGlobal("navigator", undefined);
    const info = await getClientInfo();
    expect(info).toEqual({});
  });

  it("correctly parses iOS User Agent and Safari browser", async () => {
    const ua = "Mozilla/5.0 (iPhone; CPU iPhone OS 16_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.5 Mobile/15E148 Safari/604.1";
    vi.stubGlobal("navigator", { userAgent: ua });

    const info = await getClientInfo();
    expect(info.os).toBe("iOS");
    expect(info.os_version).toBe("16.5");
    expect(info.browser).toBe("Safari");
    expect(info.browser_version).toBe("16.5");
  });

  it("correctly parses Android User Agent and Chrome browser", async () => {
    const ua = "Mozilla/5.0 (Linux; Android 13; Pixel 6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Mobile Safari/537.36";
    vi.stubGlobal("navigator", { userAgent: ua });

    const info = await getClientInfo();
    expect(info.os).toBe("Android");
    expect(info.os_version).toBe("13");
    expect(info.browser).toBe("Chrome");
    expect(info.browser_version).toBe("114.0.0.0");
  });

  it("correctly parses Windows NT and Edge browser", async () => {
    const ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36 Edg/114.0.1823.43";
    vi.stubGlobal("navigator", { userAgent: ua });

    const info = await getClientInfo();
    expect(info.os).toBe("Windows");
    expect(info.os_version).toBe("10/11");
    expect(info.browser).toBe("Edge");
    expect(info.browser_version).toBe("114.0.1823.43");
  });

  it("correctly parses macOS and Opera browser", async () => {
    const ua = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36 OPR/98.0.4759.15";
    vi.stubGlobal("navigator", { userAgent: ua });

    const info = await getClientInfo();
    expect(info.os).toBe("macOS");
    expect(info.os_version).toBe("10.15.7");
    expect(info.browser).toBe("Opera");
    expect(info.browser_version).toBe("98.0.4759.15");
  });

  it("correctly parses Linux and Firefox browser", async () => {
    const ua = "Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/114.0";
    vi.stubGlobal("navigator", { userAgent: ua });

    const info = await getClientInfo();
    expect(info.os).toBe("Linux");
    expect(info.os_version).toBeUndefined();
    expect(info.browser).toBe("Firefox");
    expect(info.browser_version).toBe("114.0");
  });

  it("supports userAgentData High Entropy Client Hints", async () => {
    const ua = "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36";
    const getHighEntropyValuesMock = vi.fn().mockResolvedValue({
      platform: "macOS",
      platformVersion: "13.4.0",
      architecture: "arm",
      mobile: false,
    });
    vi.stubGlobal("navigator", {
      userAgent: ua,
      userAgentData: {
        getHighEntropyValues: getHighEntropyValuesMock,
        mobile: true, // fallback check
      },
    });

    const info = await getClientInfo();
    expect(getHighEntropyValuesMock).toHaveBeenCalledWith([
      "architecture",
      "fullVersionList",
      "mobile",
      "platform",
      "platformVersion",
    ]);
    expect(info.os).toBe("macOS");
    expect(info.os_version).toBe("13.4.0");
    expect(info.architecture).toBe("arm");
    expect(info.mobile).toBe("false"); // matches highEntropy.mobile
  });

  it("correctly detects Brave browser when brave API is present", async () => {
    const ua = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36";
    vi.stubGlobal("navigator", {
      userAgent: ua,
      brave: {
        isBrave: vi.fn().mockResolvedValue(true),
      },
      userAgentData: {
        getHighEntropyValues: vi.fn().mockResolvedValue({
          fullVersionList: [
            { brand: "Chromium", version: "114.0.0.0" },
            { brand: "Brave", version: "114.0.0.0" },
          ],
        }),
      },
    });

    const info = await getClientInfo();
    expect(info.browser).toBe("Brave");
    expect(info.browser_version).toBe("114.0.0.0");
  });

  it("returns undefined browser version for unknown browser or mismatched UA", async () => {
    const ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36";
    vi.stubGlobal("navigator", { userAgent: ua });
    const info = await getClientInfo();
    expect(info.browser).toBeUndefined();
    expect(info.browser_version).toBeUndefined();
  });

  it("handles high entropy query rejection", async () => {
    const ua = "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36";
    vi.stubGlobal("navigator", {
      userAgent: ua,
      userAgentData: {
        getHighEntropyValues: vi.fn().mockRejectedValue(new Error("denied")),
      },
    });
    const info = await getClientInfo();
    expect(info.os).toBe("Android");
  });
});
