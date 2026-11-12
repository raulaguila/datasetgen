const BROWSER_NAMES: Record<string, string> = { Edg: "Edge", OPR: "Opera", CriOS: "Chrome", SamsungBrowser: "Samsung Internet" };

/** A short name for a User-Agent, such as "Chrome 152 · Linux" or "curl 8.18". */
export function describeDevice(ua: string | null): { label: string | null; mobile: boolean } {
  if (!ua) return { label: null, mobile: false };
  const mobile = /Mobile|Android|iPhone|iPad/.test(ua);
  const tool = /^(curl|Wget|PostmanRuntime|python-requests|Go-http-client|okhttp)\/([\d.]+)/i.exec(ua);
  if (tool) return { label: `${tool[1]} ${tool[2]}`, mobile };
  // Order matters: Edge and Opera also say Chrome, and Chrome also says Safari.
  const m =
    /(Edg|OPR|Firefox|SamsungBrowser)\/(\d+)/.exec(ua) ??
    /(Chrome|CriOS)\/(\d+)/.exec(ua) ??
    /(Version)\/(\d+).*Safari\//.exec(ua);
  const browser = m && `${m[1] === "Version" ? "Safari" : (BROWSER_NAMES[m[1]] ?? m[1])} ${m[2]}`;
  const os =
    /iPhone|iPad/.test(ua) ? "iOS" :
    /Android/.test(ua) ? "Android" :
    /Windows/.test(ua) ? "Windows" :
    /CrOS/.test(ua) ? "ChromeOS" :
    /Mac OS X|Macintosh/.test(ua) ? "macOS" :
    /Linux/.test(ua) ? "Linux" : null;
  const label = [browser, os].filter(Boolean).join(" · ");
  return { label: label || ua.slice(0, 60), mobile };
}
