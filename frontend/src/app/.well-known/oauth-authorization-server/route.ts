// RFC 8414: where an MCP client registers, sends the member to approve, and trades the code.
import { NextResponse, type NextRequest } from "next/server";

export const dynamic = "force-dynamic";

export function GET(req: NextRequest) {
  const origin = req.nextUrl.origin;
  return NextResponse.json({
    issuer: origin,
    authorization_endpoint: `${origin}/oauth/authorize`,
    token_endpoint: `${origin}/oauth/token`,
    registration_endpoint: `${origin}/oauth/register`,
    response_types_supported: ["code"],
    grant_types_supported: ["authorization_code"],
    code_challenge_methods_supported: ["S256"],
    token_endpoint_auth_methods_supported: ["none"],
  });
}
