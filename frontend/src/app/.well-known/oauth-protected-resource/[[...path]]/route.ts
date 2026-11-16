// RFC 9728: the MCP endpoint is a resource this address's OAuth server guards.
// Answers with or without the resource's path after it.
import { NextResponse, type NextRequest } from "next/server";
import { MCP_PATH } from "@/lib/oauth";

export const dynamic = "force-dynamic";

export function GET(req: NextRequest) {
  const origin = req.nextUrl.origin;
  return NextResponse.json({ resource: `${origin}${MCP_PATH}`, authorization_servers: [origin], bearer_methods_supported: ["header"] });
}
