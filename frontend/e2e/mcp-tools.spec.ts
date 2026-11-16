import { expect, test } from "@playwright/test";

// An owner publishes an MCP tool on the Integrations page; an assistant with a
// key to the collection sees it and calls it through the web address.
test("an owner publishes an MCP tool and an assistant calls it", async ({ page }) => {
  const name = `mcp-${Date.now()}`;
  await page.goto("/collections/new");
  await page.locator("#c-name").fill(name);
  await page.getByRole("region", { name: "Nova coleção" }).getByRole("button", { name: "Criar" }).click();
  await expect(page).toHaveURL(/\/collections\/[0-9a-f-]{36}$/);
  const records = page.url();
  const collectionId = records.split("/").pop()!;
  for (const title of ["Alfa", "Beta"]) {
    const res = await page.request.post("/api/proxy/v1/records", { data: { collection_id: collectionId, data: { title, notes: `nota ${title}` } } });
    expect(res.status()).toBe(201);
  }

  await page.goto(`${records}/integrations`);
  await page.getByRole("button", { name: "Nova tool" }).click();
  const form = page.getByRole("form", { name: "Nova tool" });
  await form.getByLabel("Nome da tool").fill("nota_por_titulo");
  await form.getByLabel("Descrição para o assistente").fill("A nota de um registro pelo título exato.");
  await form.getByRole("button", { name: "Adicionar parâmetro" }).click();
  await form.getByLabel("Nome do parâmetro").fill("titulo");
  await form.getByText("Obrigatório").click();
  await form.getByText("Notas", { exact: true }).click(); // returns only the notes
  await form.getByRole("button", { name: "Salvar" }).click();
  await expect(page.getByRole("list", { name: "Tools do assistente (MCP)" }).getByText("nota_por_titulo")).toBeVisible();
  await expect(page.getByText("(titulo)")).toBeVisible();

  await page.goto("/profile#keys");
  await page.getByRole("button", { name: "Nova chave" }).click();
  await page.getByLabel("Nome da chave").fill(`assistente-${name}`);
  await page.getByRole("combobox", { name, exact: true }).click();
  await page.getByRole("option", { name: "Só leitura" }).click();
  await page.getByRole("button", { name: "Criar chave" }).click();
  const secret = await page.locator("code").filter({ hasText: /^dsk_/ }).innerText();

  const mcp = (body: object) =>
    page.request.post("/api/proxy/v1/mcp", {
      headers: { Authorization: `Bearer ${secret}`, Cookie: "", Accept: "application/json, text/event-stream" },
      data: { jsonrpc: "2.0", id: 1, ...body },
    });
  const list = await (await mcp({ method: "tools/list", params: {} })).json();
  expect(list.result.tools.map((x: { name: string }) => x.name)).toContain("nota_por_titulo");
  const call = await (await mcp({ method: "tools/call", params: { name: "nota_por_titulo", arguments: { titulo: "Beta" } } })).json();
  expect(call.result.structuredContent.total).toBe(1);
  expect(call.result.structuredContent.records[0].data).toEqual({ notes: "nota Beta" });
});
