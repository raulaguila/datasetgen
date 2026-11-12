// BFF proxy. The browser calls /api/proxy/v1/...; this route attaches the bearer
// token from the httpOnly cookie, refreshes it once on 401, and returns the
// upstream answer. Token endpoints stay behind /api/auth, so a token never
// reaches the browser.
import { NextRequest, NextResponse } from "next/server";
import { apiUrl, forwardHeaders, refreshSession } from "@/lib/api/upstream";
import { clearSessionCookie, readSession, unauthorized, writeSessionCookie, type Session } from "@/lib/session";

export const dynamic = "force-dynamic";

const MAX_BODY = 104 * 1024; // the backend refuses more: a 100 KB picture is its largest body
const TOKEN_PATHS = /^v1\/auth\/(login|refresh|logout|register|activate|password-reset)(\/|$)/;
const PASSTHROUGH = ["content-type", "content-disposition", "retry-after", "x-request-id"];
const COMPRESSIBLE = /^(application\/json|text\/)/;
const MIN_GZIP = 1024; // below this the gzip header costs more than it saves

/** Gzips JSON and text when the browser takes it: Next compresses pages, not route handler streams. */
function compressed(req: NextRequest, upstream: Response): ReadableStream<Uint8Array> | null {
  const length = Number(upstream.headers.get("content-length") ?? Infinity);
  if (!upstream.body || length < MIN_GZIP || !COMPRESSIBLE.test(upstream.headers.get("content-type") ?? "")) return null;
  if (!/\bgzip\b/.test(req.headers.get("accept-encoding") ?? "")) return null;
  return upstream.body.pipeThrough(new CompressionStream("gzip"));
}

async function handle(req: NextRequest, ctx: { params: Promise<{ path: string[] }> }) {
  const { path } = await ctx.params;
  // Checked after URL parsing: fetch would turn "v1/%2e%2e/v1/auth/login" into a token path.
  const target = new URL(apiUrl("/" + path.join("/")) + req.nextUrl.search);
  const routed = target.pathname.slice(new URL(apiUrl("/")).pathname.length);
  if (!routed.startsWith("v1/") || TOKEN_PATHS.test(routed)) return NextResponse.json({ code: "NOT_FOUND", message: "use /api/auth" }, { status: 404 });

  const { session, present } = await readSession();
  if (!session) return unauthorized(present);

  // Bytes, not text: an image body would not survive a UTF-8 round trip.
  const raw = req.method === "GET" || req.method === "HEAD" ? undefined : await req.arrayBuffer();
  const body = raw?.byteLength ? raw : undefined;
  if (body && body.byteLength > MAX_BODY) return NextResponse.json({ code: "INVALID_INPUT", message: "request body too large" }, { status: 413 });

  const headers = forwardHeaders(req);
  const send = (token: string) =>
    fetch(target, {
      method: req.method,
      headers: {
        ...headers,
        authorization: `Bearer ${token}`,
        ...(body ? { "content-type": req.headers.get("content-type") ?? "application/json" } : {}),
      },
      body,
      cache: "no-store",
      signal: AbortSignal.any([req.signal, AbortSignal.timeout(30_000)]),
    });

  let rotated: Session | null = null;
  let upstream: Response;
  try {
    upstream = await send(session.access_token);
    if (upstream.status === 401) {
      await upstream.body?.cancel();
      const next = await refreshSession(session, headers);
      if (!next.ok) {
        if (!next.dead) return NextResponse.json({ code: "UNAVAILABLE", message: "auth temporarily unavailable" }, { status: 503 });
        return unauthorized(true);
      }
      rotated = next.session;
      upstream = await send(rotated.access_token);
    }
  } catch {
    const out = NextResponse.json({ code: "UNAVAILABLE", message: "upstream temporarily unavailable" }, { status: 503 });
    if (rotated) await writeSessionCookie(out, rotated);
    return out;
  }

  const gzip = compressed(req, upstream);
  const out = new NextResponse(gzip ?? upstream.body, { status: upstream.status });
  if (gzip) {
    out.headers.set("content-encoding", "gzip");
    out.headers.set("vary", "accept-encoding");
  }
  for (const name of PASSTHROUGH) {
    const value = upstream.headers.get(name);
    if (value) out.headers.set(name, value);
  }
  if (upstream.status === 401) await clearSessionCookie(out);
  else if (rotated) await writeSessionCookie(out, rotated);
  return out;
}

export const GET = handle;
export const POST = handle;
export const PUT = handle;
export const PATCH = handle;
export const DELETE = handle;
