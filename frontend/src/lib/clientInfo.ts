export type ClientInfo = {
  browser?: string;
  browser_version?: string;
  os?: string;
  os_version?: string;
  architecture?: string;
  mobile?: string;
};

function parseClientOS(ua: string): Pick<ClientInfo, "os" | "os_version"> {
  if (/iPhone|iPad|iPod/.test(ua)) {
    const version = ua.match(/OS ([\d_]+)/)?.[1]?.replaceAll("_", ".");
    return { os: "iOS", os_version: version };
  }
  if (/Android/.test(ua)) {
    return { os: "Android", os_version: ua.match(/Android ([\d.]+)/)?.[1] };
  }
  if (/Windows NT/.test(ua)) {
    const ntVersion = ua.match(/Windows NT ([\d.]+)/)?.[1];
    const version = ntVersion === "10.0" ? "10/11" : ntVersion;
    return { os: "Windows", os_version: version };
  }
  if (/Macintosh|Mac OS X/.test(ua)) {
    const version = ua.match(/Mac OS X ([\d_]+)/)?.[1]?.replaceAll("_", ".");
    return { os: "macOS", os_version: version };
  }
  if (/Linux/.test(ua)) return { os: "Linux" };

  return {};
}

function browserVersionFromUA(ua: string, browser?: string): string | undefined {
  if (browser === "Opera") return ua.match(/OPR\/([\d.]+)/)?.[1];
  if (browser === "Edge") return ua.match(/Edg\/([\d.]+)/)?.[1];
  if (browser === "Firefox") return ua.match(/Firefox\/([\d.]+)/)?.[1];
  if (browser === "Safari") return ua.match(/Version\/([\d.]+)/)?.[1];
  if (browser === "Brave" || browser === "Chrome") return ua.match(/Chrome\/([\d.]+)/)?.[1];
  return undefined;
}

export async function getClientInfo(): Promise<ClientInfo> {
  if (typeof navigator === "undefined") return {};

  const nav = navigator as Navigator & {
    brave?: { isBrave?: () => Promise<boolean> };
    userAgentData?: {
      brands?: Array<{ brand: string; version: string }>;
      mobile?: boolean;
      platform?: string;
      getHighEntropyValues?: (hints: string[]) => Promise<{
        architecture?: string;
        brands?: Array<{ brand: string; version: string }>;
        fullVersionList?: Array<{ brand: string; version: string }>;
        mobile?: boolean;
        platform?: string;
        platformVersion?: string;
      }>;
    };
  };

  const ua = navigator.userAgent;
  const info: ClientInfo = parseClientOS(ua);

  const highEntropy = await nav.userAgentData
    ?.getHighEntropyValues?.(["architecture", "fullVersionList", "mobile", "platform", "platformVersion"])
    .catch(() => undefined);
  if (highEntropy?.platform) info.os = highEntropy.platform;
  if (highEntropy?.platformVersion) info.os_version = highEntropy.platformVersion;
  if (highEntropy?.architecture) info.architecture = highEntropy.architecture;
  const mobile = highEntropy?.mobile ?? nav.userAgentData?.mobile;
  if (typeof mobile === "boolean") info.mobile = mobile ? "true" : "false";

  if (await nav.brave?.isBrave?.().catch(() => false)) {
    info.browser = "Brave";
    info.browser_version =
      highEntropy?.fullVersionList?.find((item) => item.brand === "Chromium")?.version ||
      highEntropy?.fullVersionList?.find((item) => item.brand === "Google Chrome")?.version ||
      browserVersionFromUA(ua, info.browser);
    return info;
  }

  if (/OPR\//.test(ua)) info.browser = "Opera";
  else if (/Edg\//.test(ua)) info.browser = "Edge";
  else if (/Firefox\//.test(ua)) info.browser = "Firefox";
  else if (/Safari\//.test(ua) && !/Chrome\//.test(ua)) info.browser = "Safari";
  else if (/Chrome\//.test(ua) && !/Chromium\//.test(ua)) info.browser = "Chrome";

  if (info.browser === "Chrome") {
    info.browser_version =
      highEntropy?.fullVersionList?.find((item) => item.brand === "Google Chrome")?.version ||
      highEntropy?.fullVersionList?.find((item) => item.brand === "Chromium")?.version ||
      browserVersionFromUA(ua, info.browser);
  } else if (info.browser) {
    info.browser_version = browserVersionFromUA(ua, info.browser);
  }

  return info;
}
