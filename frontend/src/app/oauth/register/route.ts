import type { NextRequest } from "next/server";
import { forwardOAuth } from "@/lib/oauth";

export const dynamic = "force-dynamic";

export const POST = (req: NextRequest) => forwardOAuth(req, "register");
