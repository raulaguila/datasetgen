// OAuth for MCP clients that cannot take a pasted key (the ChatGPT app): the
// discovery documents and the two endpoints they call live on this web
// address; the backend (/v1/oauth) does the work.
import "server-only";
import { NextResponse, type NextRequest } from "next/server";
import { apiUrl, forwardHeaders } from "@/lib/api/upstream";

export const MCP_PATH = "/api/proxy/v1/mcp";

/** Where a 401 from the MCP endpoint points the client (RFC 9728). */
export const resourceMetadata = (origin: string) => `${origin}/.well-known/oauth-protected-resource${MCP_PATH}`;

/** Passes a client's call (register or token) to the backend as it came. */
export async function forwardOAuth(req: NextRequest, path: string): Promise<NextResponse> {
  try {
    const upstream = await fetch(apiUrl(`/v1/oauth/${path}`), {
      method: "POST",
      headers: { ...forwardHeaders(req), "content-type": req.headers.get("content-type") ?? "application/json" },
      body: await req.arrayBuffer(),
      cache: "no-store",
      signal: AbortSignal.timeout(15_000),
    });
    return new NextResponse(upstream.body, {
      status: upstream.status,
      headers: { "content-type": upstream.headers.get("content-type") ?? "application/json", "cache-control": "no-store" },
    });
  } catch {
    return NextResponse.json({ error: "temporarily_unavailable" }, { status: 503 });
  }
}
