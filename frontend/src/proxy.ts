import { NextRequest, NextResponse } from "next/server";

const PUBLIC = ["/login", "/register", "/activate", "/forgot-password", "/reset-password", "/f", "/s"];

// Coarse gate by cookie presence; the API validates the session. A present but
// invalid cookie is cleared by the BFF on 401, so /login never loops.
export function proxy(req: NextRequest) {
  const { pathname } = req.nextUrl;
  const isPublic = PUBLIC.some((p) => pathname === p || pathname.startsWith(p + "/"));
  const signedIn = req.cookies.has("dataset_session");
  if (signedIn && (pathname === "/login" || pathname === "/register")) {
    return NextResponse.redirect(new URL("/", req.url));
  }
  if (!isPublic && !signedIn) {
    const url = req.nextUrl.clone();
    url.pathname = "/login";
    url.search = "";
    if (pathname !== "/") url.searchParams.set("next", pathname);
    return NextResponse.redirect(url);
  }
  return NextResponse.next();
}

export const config = {
  matcher: ["/((?!api|_next/static|_next/image|favicon.svg|fonts).*)"],
};
