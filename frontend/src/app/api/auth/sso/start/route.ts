// Sends the browser to the sign-in provider. The sealed flow stays in an
// httpOnly cookie so only this browser can finish the sign-in.
import { NextRequest, NextResponse } from "next/server";
import { apiUrl, forwardHeaders } from "@/lib/api/upstream";
import { SSO_FLOW_COOKIE, cookieOpts } from "@/lib/session";

export const dynamic = "force-dynamic";

export async function GET(req: NextRequest) {
  try {
    const res = await fetch(apiUrl("/v1/auth/sso/start"), {
      method: "POST",
      headers: forwardHeaders(req),
      cache: "no-store",
      signal: AbortSignal.timeout(15_000),
    });
    if (res.ok) {
      const { url, flow } = (await res.json()) as { url: string; flow: string };
      const out = NextResponse.redirect(url);
      out.cookies.set(SSO_FLOW_COOKIE, flow, { ...(await cookieOpts()), path: "/api/auth/sso", maxAge: 600 });
      return out;
    }
  } catch {
    // falls through to the login page with an error
  }
  return NextResponse.redirect(new URL("/login?sso_error=1", req.nextUrl));
}
