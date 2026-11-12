// Server-only helpers to talk to the Go API: base URL, forwarded headers and
// the refresh-token rotation shared by the BFF routes.
import "server-only";
import { randomUUID } from "node:crypto";
import type { NextRequest } from "next/server";
import type { Session } from "@/lib/session";
import type { TokenPair } from "@/lib/api/types";

export function apiUrl(path: string): string {
  return (process.env.API_URL ?? `http://localhost:${process.env.API_PORT ?? 9977}`).replace(/\/$/, "") + path;
}

// shortcut: CLIENT_IP_HEADER is trusted as sent. Without a reverse proxy in front
// of the web app that overwrites it, a client picks its own IP and escapes the
// per-IP limits; put one there (and set x-real-ip) before going public.
function clientIp(req: NextRequest): string | undefined {
  const header = process.env.CLIENT_IP_HEADER || "x-forwarded-for";
  return req.headers.get(header)?.split(",")[0]?.trim() || undefined;
}

/** Headers every upstream call carries: a correlation id and the client IP. */
export function forwardHeaders(req: NextRequest): Record<string, string> {
  const headers: Record<string, string> = { "x-request-id": randomUUID() };
  const ip = clientIp(req);
  if (ip) headers["x-real-ip"] = ip;
  // The session list names the device by the browser, not by this server.
  const agent = req.headers.get("user-agent");
  if (agent) headers["user-agent"] = agent;
  return headers;
}

export function sessionFromTokens(pair: TokenPair): Session {
  return { access_token: pair.access_token, refresh_token: pair.refresh_token };
}

// Only an explicit rejection ends the session. A network error, a 5xx or a 429
// is transient: clearing the cookie then would log people out for nothing.
export type RefreshOutcome = { ok: true; session: Session } | { ok: false; dead: boolean };

const pending = new Map<string, Promise<RefreshOutcome>>();
// A request sent with the old cookie can reach us just after the rotation ended,
// before the browser stored the new cookie; it gets the same answer instead of a
// 409 from the backend. Shorter than the backend's race window.
const REUSE_MS = 5_000;

/** Refresh tokens are single use: concurrent and just-late requests share one rotation. */
export function refreshSession(s: Session, headers: Record<string, string>): Promise<RefreshOutcome> {
  const running = pending.get(s.refresh_token);
  if (running) return running;
  const request = rotate(s, headers);
  pending.set(s.refresh_token, request);
  void request.then((r) => setTimeout(() => pending.delete(s.refresh_token), r.ok ? REUSE_MS : 0));
  return request;
}

async function rotate(s: Session, headers: Record<string, string>): Promise<RefreshOutcome> {
  let res: Response;
  try {
    res = await fetch(apiUrl("/v1/auth/refresh"), {
      method: "POST",
      headers: { ...headers, "content-type": "application/json" },
      body: JSON.stringify({ refresh_token: s.refresh_token }),
      cache: "no-store",
      signal: AbortSignal.timeout(15_000),
    });
  } catch {
    return { ok: false, dead: false };
  }
  if (!res.ok) return { ok: false, dead: res.status === 400 || res.status === 401 || res.status === 403 };
  try {
    const pair = (await res.json()) as TokenPair;
    if (!pair.access_token || !pair.refresh_token) return { ok: false, dead: false };
    return { ok: true, session: sessionFromTokens(pair) };
  } catch {
    return { ok: false, dead: false };
  }
}
