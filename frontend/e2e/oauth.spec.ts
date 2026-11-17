import { createHash, randomBytes } from "node:crypto";
import { expect, test } from "@playwright/test";
import { dropKey } from "./helpers";

// The ChatGPT app path: an MCP client without a key finds the OAuth server,
// registers, sends the member to approve, and trades the code for a key.
test("an assistant gets a key through OAuth", async ({ page, playwright, baseURL }) => {
  const name = `oauth-${Date.now()}`;
  const made = await page.request.post("/api/proxy/v1/collections", { data: { name, schema: { type: "object" } } });
  expect(made.status()).toBe(201);
  const collectionId = (await made.json()).id as string;

  // No session and no key: the 401 points at the resource metadata, which names this address's server.
  const anon = await playwright.request.newContext({ baseURL });
  const denied = await anon.post("/api/proxy/v1/mcp", { headers: { Accept: "application/json, text/event-stream" }, data: { jsonrpc: "2.0", id: 1, method: "tools/list" } });
  expect(denied.status()).toBe(401);
  expect(denied.headers()["www-authenticate"]).toContain("/.well-known/oauth-protected-resource/api/proxy/v1/mcp");
  const resource = await (await anon.get("/.well-known/oauth-protected-resource/api/proxy/v1/mcp")).json();
  expect(resource.resource).toBe(`${baseURL}/api/proxy/v1/mcp`);
  const server = await (await anon.get("/.well-known/oauth-authorization-server")).json();
  expect(server.code_challenge_methods_supported).toEqual(["S256"]);

  const redirect = "http://127.0.0.1:9/callback";
  const reg = await anon.post("/oauth/register", { data: { client_name: "Assistente E2E", redirect_uris: [redirect] } });
  expect(reg.status()).toBe(201);
  const clientId = (await reg.json()).client_id as string;

  const verifier = randomBytes(32).toString("base64url");
  const challenge = createHash("sha256").update(verifier).digest("base64url");
  const query = new URLSearchParams({ response_type: "code", client_id: clientId, redirect_uri: redirect, code_challenge: challenge, code_challenge_method: "S256", state: "s1" });
  // The member approves on the consent page; the browser then lands on the client's redirect.
  let landed = "";
  await page.route(`${redirect}**`, (route) => { landed = route.request().url(); return route.fulfill({ body: "ok" }); });
  await page.goto(`/oauth/authorize?${query}`);
  await expect(page.getByRole("heading", { name: "Assistente E2E quer acessar suas coleções" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Permitir" })).toBeDisabled(); // nothing picked yet
  await page.getByRole("combobox", { name, exact: true }).click();
  await page.getByRole("option", { name: "Só leitura" }).click();
  await page.getByRole("button", { name: "Permitir" }).click();
  await expect.poll(() => landed).toContain("code=");
  const back = new URL(landed);
  expect(back.searchParams.get("state")).toBe("s1");

  const token = await anon.post("/oauth/token", {
    form: { grant_type: "authorization_code", code: back.searchParams.get("code")!, client_id: clientId, redirect_uri: redirect, code_verifier: verifier },
  });
  expect(token.status()).toBe(200);
  const { access_token: secret } = await token.json();
  const call = await anon.post("/api/proxy/v1/mcp", {
    headers: { Authorization: `Bearer ${secret}`, Accept: "application/json, text/event-stream" },
    data: { jsonrpc: "2.0", id: 1, method: "tools/call", params: { name: "workspace_overview", arguments: {} } },
  });
  expect((await call.json()).result.structuredContent.collections.map((c: { id: string }) => c.id)).toEqual([collectionId]);
  await anon.dispose();
  await dropKey(page, "Assistente E2E");
});
