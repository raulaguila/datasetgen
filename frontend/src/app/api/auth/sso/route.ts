// Whether single sign-on is on, for the login button.
import { NextResponse } from "next/server";
import { apiUrl } from "@/lib/api/upstream";

export const dynamic = "force-dynamic";

export async function GET() {
  try {
    const res = await fetch(apiUrl("/v1/auth/sso"), { cache: "no-store", signal: AbortSignal.timeout(5_000) });
    if (res.ok) return NextResponse.json(await res.json());
  } catch {
    // the button just stays hidden
  }
  return NextResponse.json({ enabled: false });
}
