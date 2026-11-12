// @vitest-environment node
import { describe, expect, it, vi } from "vitest";
import { NextResponse } from "next/server";

vi.mock("server-only", () => ({}));
const jar = new Map<string, string>();
const req = new Headers();
vi.mock("next/headers", () => ({
  cookies: async () => ({ get: (name: string) => (jar.has(name) ? { value: jar.get(name)! } : undefined) }),
  headers: async () => req,
}));

const { SESSION_COOKIE, readSession, writeSessionCookie } = await import("./session");

async function issue() {
  const res = NextResponse.json({});
  await writeSessionCookie(res, { access_token: "acc", refresh_token: "ref" });
  return res.cookies.get(SESSION_COOKIE)!;
}
const issued = async () => (await issue()).value;

describe("session cookie", () => {
  it("reads back the tokens it wrote", async () => {
    jar.set(SESSION_COOKIE, await issued());
    expect(await readSession()).toEqual({ session: { access_token: "acc", refresh_token: "ref" }, present: true });
  });

  it("rejects a cookie whose payload was changed", async () => {
    const [, signature] = (await issued()).split(".");
    const forged = Buffer.from(JSON.stringify({ a: "mine", r: "mine" })).toString("base64url");
    jar.set(SESSION_COOKIE, `${forged}.${signature}`);
    expect(await readSession()).toEqual({ session: null, present: true });
  });

  it("reports no cookie as absent", async () => {
    jar.clear();
    expect(await readSession()).toEqual({ session: null, present: false });
  });

  // A production build on a LAN address over HTTP must still sign in.
  it("marks the cookie Secure only over HTTPS", async () => {
    req.set("x-forwarded-proto", "http");
    expect((await issue()).secure).toBe(false);
    req.set("x-forwarded-proto", "https");
    expect((await issue()).secure).toBe(true);
  });
});
