import { resolve } from "node:path";
import type { NextConfig } from "next";
import { loadConfigDotEnv } from "./config/load-env.mjs";

loadConfigDotEnv(resolve(__dirname, "config/.env"));

// Next 16 blocks cross-origin access to /_next/webpack-hmr unless the browser
// host is listed. "*" is NOT a wildcard — use explicit hosts (LAN IP, etc.).
function allowedDevOrigins(): string[] {
  const hosts = new Set<string>(["127.0.0.1", "localhost"]);
  for (const part of (process.env.DEV_ALLOWED_ORIGINS ?? "").split(",")) {
    const h = part.trim();
    if (h) hosts.add(h);
  }
  try {
    const api = process.env.API_URL;
    if (api) {
      const host = new URL(api).hostname;
      if (host && host !== "0.0.0.0") hosts.add(host);
    }
  } catch {
    /* ignore bad API_URL */
  }
  return [...hosts];
}

const nextConfig: NextConfig = {
  output: "standalone",
  poweredByHeader: false,
  async headers() {
    const dev = process.env.NODE_ENV !== "production";
    const csp = [
      "default-src 'self'",
      `script-src 'self' 'unsafe-inline'${dev ? " 'unsafe-eval'" : ""}`,
      "style-src 'self' 'unsafe-inline'",
      "img-src 'self' data: blob:",
      "font-src 'self' data:",
      `connect-src 'self'${dev ? " ws: wss:" : ""}`,
      "worker-src 'self' blob:",
      "object-src 'none'",
      "base-uri 'self'",
      "frame-ancestors 'none'",
    ].join("; ");
    return [
      {
        source: "/:path*",
        headers: [
          { key: "Content-Security-Policy", value: csp },
          { key: "X-Content-Type-Options", value: "nosniff" },
          { key: "X-Frame-Options", value: "DENY" },
          { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
          {
            key: "Permissions-Policy",
            value: "camera=(), microphone=(), geolocation=()",
          },
        ],
      },
    ];
  },
  allowedDevOrigins: allowedDevOrigins(),
};

export default nextConfig;
