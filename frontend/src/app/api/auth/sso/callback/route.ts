// The provider sends the browser back here; the backend checks the code against
// the flow cookie and answers the session tokens, kept in the session cookie.
import { NextRequest, NextResponse } from "next/server";
import { apiUrl, forwardHeaders, sessionFromTokens } from "@/lib/api/upstream";
import { SSO_FLOW_COOKIE, cookieOpts, writeSessionCookie } from "@/lib/session";
import type { TokenPair } from "@/lib/api/types";

export const dynamic = "force-dynamic";

export async function GET(req: NextRequest) {
  const q = req.nextUrl.searchParams;
  const flow = req.cookies.get(SSO_FLOW_COOKIE)?.value ?? "";
  let to = "/login?sso_error=1";
  let pair: TokenPair | null = null;
  try {
    const res = await fetch(apiUrl("/v1/auth/sso/callback"), {
      method: "POST",
      headers: { ...forwardHeaders(req), "content-type": "application/json" },
      body: JSON.stringify({ code: q.get("code") ?? "", state: q.get("state") ?? "", flow }),
      cache: "no-store",
      signal: AbortSignal.timeout(15_000),
    });
    if (res.ok) {
      pair = (await res.json()) as TokenPair;
      to = "/";
    } else if (res.status === 403) {
      to = "/login?sso_error=no_account";
    }
  } catch {
    // stays on the error redirect
  }
  const out = NextResponse.redirect(new URL(to, req.nextUrl));
  out.cookies.set(SSO_FLOW_COOKIE, "", { ...(await cookieOpts()), path: "/api/auth/sso", maxAge: 0 });
  if (pair) await writeSessionCookie(out, sessionFromTokens(pair));
  return out;
}
