// Server-only session handling. The tokens live in an httpOnly cookie signed
// with HMAC, so browser JavaScript never reads them. The user object is not
// stored: permissions change at runtime and come from /v1/me.
import "server-only";
import { createHmac, timingSafeEqual } from "node:crypto";
import { cookies, headers } from "next/headers";
import { NextResponse } from "next/server";

export const SESSION_COOKIE = "dataset_session";
const MAX_AGE = 60 * 60 * 24 * 30; // the refresh token lifetime caps real validity

export interface Session {
  access_token: string;
  refresh_token: string;
}

function sessionSecret(): string {
  const secret = process.env.SESSION_SECRET;
  if (secret) return secret;
  if (process.env.NODE_ENV === "production") throw new Error("SESSION_SECRET must be set in production");
  return "development-only-session-secret";
}

function sign(payload: string): string {
  return createHmac("sha256", sessionSecret()).update(payload).digest("base64url");
}

function encode(s: Session): string {
  const payload = Buffer.from(JSON.stringify({ a: s.access_token, r: s.refresh_token })).toString("base64url");
  return `${payload}.${sign(payload)}`;
}

function decode(raw: string): Session | null {
  const [payload, signature, ...extra] = raw.split(".");
  if (!payload || !signature || extra.length) return null;
  const actual = Buffer.from(signature);
  const expected = Buffer.from(sign(payload));
  if (actual.length !== expected.length || !timingSafeEqual(actual, expected)) return null;
  try {
    const { a, r } = JSON.parse(Buffer.from(payload, "base64url").toString("utf8"));
    return typeof a === "string" && typeof r === "string" ? { access_token: a, refresh_token: r } : null;
  } catch {
    return null;
  }
}

// COOKIE_SECURE forces the flag either way. Otherwise the cookie is Secure only
// when the page was served over HTTPS: browsers drop a Secure cookie sent over
// plain HTTP, so a production build on a LAN address could never sign in.
// Next fills x-forwarded-proto from the socket; a TLS proxy in front sends it.
async function cookieOpts() {
  const v = (process.env.COOKIE_SECURE ?? "").trim().toLowerCase();
  let secure: boolean;
  if (["0", "false", "no"].includes(v)) secure = false;
  else if (["1", "true", "yes"].includes(v)) secure = true;
  else {
    const proto = (await headers()).get("x-forwarded-proto")?.split(",")[0].trim();
    secure = proto ? proto === "https" : process.env.NODE_ENV === "production";
  }
  return { httpOnly: true, secure, sameSite: "lax" as const, path: "/" };
}

/** The session and whether a cookie was present at all (even an invalid one). */
export async function readSession(): Promise<{ session: Session | null; present: boolean }> {
  const raw = (await cookies()).get(SESSION_COOKIE)?.value;
  return { session: raw ? decode(raw) : null, present: Boolean(raw) };
}

// Set on the returned response: the next/headers store is not reliably merged
// onto a streamed NextResponse, and a lost rotated refresh token ends the session.
export async function writeSessionCookie(res: NextResponse, s: Session): Promise<void> {
  res.cookies.set(SESSION_COOKIE, encode(s), { ...(await cookieOpts()), maxAge: MAX_AGE });
}

export async function clearSessionCookie(res: NextResponse): Promise<void> {
  res.cookies.set(SESSION_COOKIE, "", { ...(await cookieOpts()), maxAge: 0 });
}

/** 401 that also clears a present-but-invalid cookie, so /login stays reachable. */
export async function unauthorized(clear: boolean): Promise<NextResponse> {
  const res = NextResponse.json({ code: "UNAUTHORIZED", message: "not authenticated" }, { status: 401 });
  if (clear) await clearSessionCookie(res);
  return res;
}
