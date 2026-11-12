// Auth BFF. Login, refresh and logout keep the tokens in the httpOnly cookie;
// the browser only learns whether they worked. The public flows (register,
// activate, password reset) pass straight through.
import { NextRequest, NextResponse } from "next/server";
import { apiUrl, forwardHeaders, refreshSession, sessionFromTokens } from "@/lib/api/upstream";
import { clearSessionCookie, readSession, unauthorized, writeSessionCookie } from "@/lib/session";
import type { TokenPair } from "@/lib/api/types";

export const dynamic = "force-dynamic";

const PUBLIC: Record<string, string> = {
  register: "/v1/auth/register",
  activate: "/v1/auth/activate",
  "password-reset-request": "/v1/auth/password-reset/request",
  "password-reset-confirm": "/v1/auth/password-reset/confirm",
};

const MAX_BODY = 16 * 1024; // the backend refuses more

function unavailable(): NextResponse {
  return NextResponse.json({ code: "UNAVAILABLE", message: "service temporarily unavailable" }, { status: 503 });
}

async function relay(res: Response): Promise<NextResponse> {
  const body = await res.text();
  const headers = new Headers({ "content-type": res.headers.get("content-type") ?? "application/json" });
  const retry = res.headers.get("retry-after");
  if (retry) headers.set("retry-after", retry);
  return new NextResponse(body || null, { status: res.status, headers });
}

export async function POST(req: NextRequest, ctx: { params: Promise<{ action: string }> }) {
  const { action } = await ctx.params;
  const body = await req.text();
  if (body.length > MAX_BODY) return NextResponse.json({ code: "INVALID_INPUT", message: "request body too large" }, { status: 413 });
  const headers = forwardHeaders(req);
  const post = (path: string, json: string, extra: Record<string, string> = {}) =>
    fetch(apiUrl(path), {
      method: "POST",
      headers: { ...headers, ...extra, "content-type": "application/json" },
      body: json,
      cache: "no-store",
      signal: AbortSignal.timeout(15_000),
    });

  try {
    if (action in PUBLIC) return await relay(await post(PUBLIC[action], body || "{}"));

    if (action === "login") {
      const res = await post("/v1/auth/login", body || "{}");
      if (!res.ok) return await relay(res);
      const pair = (await res.json()) as TokenPair;
      const out = NextResponse.json({ user: pair.user ?? null });
      await writeSessionCookie(out, sessionFromTokens(pair));
      return out;
    }

    const { session, present } = await readSession();
    if (action === "refresh") {
      if (!session) return unauthorized(present);
      const next = await refreshSession(session, headers);
      if (!next.ok) {
        if (!next.dead) return unavailable();
        return unauthorized(true);
      }
      const out = new NextResponse(null, { status: 204 });
      await writeSessionCookie(out, next.session);
      return out;
    }

    if (action === "logout") {
      // Ending the backend session is best effort: the cookie goes either way.
      if (session) await post("/v1/auth/logout", "{}", { authorization: `Bearer ${session.access_token}` }).catch(() => null);
      const out = new NextResponse(null, { status: 204 });
      await clearSessionCookie(out);
      return out;
    }
  } catch {
    return unavailable();
  }
  return NextResponse.json({ code: "NOT_FOUND", message: "unknown action" }, { status: 404 });
}
