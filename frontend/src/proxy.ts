import { NextRequest, NextResponse } from "next/server";

// /.well-known and the OAuth register and token endpoints answer MCP clients, which have no session.
const PUBLIC = ["/login", "/register", "/activate", "/forgot-password", "/reset-password", "/f", "/s", "/.well-known", "/oauth/register", "/oauth/token"];

// Coarse gate by cookie presence; the API validates the session. A present but
// invalid cookie is cleared by the BFF on 401, so /login never loops.
export function proxy(req: NextRequest) {
  const { pathname } = req.nextUrl;
  const isPublic = PUBLIC.some((p) => pathname === p || pathname.startsWith(p + "/"));
  const signedIn = req.cookies.has("dataset_session");
  if (signedIn && (pathname === "/login" || pathname === "/register")) {
    return NextResponse.redirect(new URL("/", req.url));
  }
  // Signed in, the password changes in the profile; reset and activate links from mail still open.
  if (signedIn && pathname === "/forgot-password") {
    return NextResponse.redirect(new URL("/profile#security", req.url));
  }
  if (!isPublic && !signedIn) {
    const url = req.nextUrl.clone();
    url.pathname = "/login";
    url.search = "";
    // The query goes along: the OAuth consent page needs its client's parameters back.
    if (pathname !== "/") url.searchParams.set("next", pathname + req.nextUrl.search);
    return NextResponse.redirect(url);
  }
  return NextResponse.next();
}

export const config = {
  matcher: ["/((?!api|_next/static|_next/image|favicon.svg|fonts).*)"],
};
