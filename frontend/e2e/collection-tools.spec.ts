import { expect, test } from "@playwright/test";

// CSV import, sort by a field, bulk edit, a saved view, the trash, an API key
// used through the web address, and the field history.
test("a collection's tools: CSV, bulk edit, views, trash, API key, field history", async ({ page }) => {
  const name = `kit-${Date.now()}`;
  await page.goto("/collections/new");
  await page.locator("#c-name").fill(name);
  // The starter fields: Título (title, required) and Notas (notes).
  await page.getByRole("region", { name: "Nova coleção" }).getByRole("button", { name: "Criar" }).click();
  await expect(page).toHaveURL(/\/collections\/[0-9a-f-]{36}$/);
  const records = page.url();
  const collectionId = records.split("/").pop()!;

  // CSV: columns by label or key.
  await page.getByRole("button", { name: "Importar" }).click();
  await page.locator("#import-file").setInputFiles({ name: "dados.csv", mimeType: "text/csv", buffer: Buffer.from("Título;notes\nBeta;b\nAlfa;a\nGama;c\n") });
  const dialog = page.getByRole("dialog", { name: "Importar" });
  await expect(dialog.getByText("3 registro(s) válido(s), 0 com problema.")).toBeVisible();
  await dialog.getByRole("button", { name: "Importar 3" }).click();
  await expect(dialog).toBeHidden();

  // Sort by a field's column.
  await page.getByRole("button", { name: "Título" }).click();
  await expect(page.locator("tbody tr td[data-cell=name]")).toHaveText(["Alfa", "Beta", "Gama"]);
  await page.getByRole("button", { name: "Título" }).click();
  await expect(page.locator("tbody tr td[data-cell=name]")).toHaveText(["Gama", "Beta", "Alfa"]);

  // A saved view keeps that order.
  await page.getByRole("button", { name: "Visões" }).click();
  const views = page.getByRole("dialog", { name: "Visões" });
  await views.getByLabel("Salvar a lista atual como visão").fill("Z-A");
  await views.getByRole("button", { name: "Salvar" }).click();
  await expect(views.getByRole("button", { name: "Z-A", exact: true })).toBeVisible();
  await page.keyboard.press("Escape");
  await page.goto(records);
  await page.getByRole("button", { name: "Visões" }).click();
  await views.getByRole("button", { name: "Z-A", exact: true }).click();
  await expect(page.locator("tbody tr td[data-cell=name]")).toHaveText(["Gama", "Beta", "Alfa"]);

  // Bulk edit: the notes of two records at once.
  await page.getByRole("checkbox", { name: "Gama" }).click();
  await page.getByRole("checkbox", { name: "Beta" }).click();
  await page.getByRole("button", { name: "Editar em lote" }).click();
  const bulk = page.getByRole("dialog", { name: "Editar em lote" });
  await bulk.getByLabel("Campo").click();
  await page.getByRole("option", { name: "Notas" }).click();
  await bulk.locator("#bulk_notes").fill("lote");
  await bulk.getByRole("button", { name: "Aplicar em 2" }).click();
  await expect(page.getByText("2 registro(s) atualizado(s)")).toBeVisible();
  await expect(page.getByRole("row").filter({ hasText: "lote" })).toHaveCount(2);

  // Delete goes to the trash; restore brings it back.
  await page.getByRole("checkbox", { name: "Alfa" }).click();
  // The delete waits out the undo window; leaving the page before it would keep the record.
  const deleted = page.waitForResponse((r) => r.url().endsWith("/v1/records/delete") && r.ok(), { timeout: 15_000 });
  await page.getByRole("button", { name: "Excluir selecionados" }).click();
  await expect(page.getByText("1 registro(s) movido(s) para a lixeira.")).toBeVisible();
  await deleted;
  await page.goto(`${records}/trash`);
  await page.getByRole("row").filter({ hasText: "Alfa" }).getByRole("button", { name: "Restaurar" }).click();
  await expect(page.getByText("1 registro(s) restaurado(s)")).toBeVisible();
  await page.goto(records);
  await expect(page.getByRole("row").filter({ hasText: "Alfa" })).toBeVisible();

  // An API key reads the records through the web address; it never sees another collection.
  await page.goto(`${records}/integrations`);
  await page.getByLabel("Nome da chave").fill("leitura");
  await page.getByRole("button", { name: "Criar chave" }).click();
  const secret = await page.locator("code").filter({ hasText: /^dsk_/ }).innerText();
  const res = await page.request.get(`/api/proxy/v1/records?collection_id=${collectionId}&page=1&limit=10`, { headers: { Authorization: `Bearer ${secret}`, Cookie: "" } });
  expect(res.status()).toBe(200);
  expect((await res.json()).pagination.total_items).toBe(3);
  const write = await page.request.post("/api/proxy/v1/records", { headers: { "X-API-Key": secret, Cookie: "" }, data: { collection_id: collectionId, data: { title: "x" } } });
  expect(write.status()).toBe(403);

  // A field change is kept; loading the old version brings the field back.
  await page.goto(`${records}/fields`);
  await page.getByRole("button", { name: "Remover campo: Notas" }).click();
  await page.getByRole("button", { name: "Salvar", exact: true }).click();
  // Records hold notes: removing the field asks first.
  await page.getByRole("dialog").getByRole("button", { name: "Salvar" }).click();
  await expect(page.getByText("Alterações salvas.")).toBeVisible();
  await page.getByText("Histórico dos campos").click();
  await expect(page.getByText("Depois: Campo removido: Notas")).toBeVisible();
  await page.getByRole("button", { name: "Carregar no editor" }).click();
  await expect(page.getByRole("button", { name: "Remover campo: Notas" })).toBeVisible();
});

// A reference field picks a record of another collection; a file field uploads at once.
test("reference and file fields", async ({ page }) => {
  const target = `alvo-${Date.now()}`;
  await page.goto("/collections/new");
  await page.locator("#c-name").fill(target);
  await page.getByRole("region", { name: "Nova coleção" }).getByRole("button", { name: "Criar" }).click();
  await expect(page).toHaveURL(/\/collections\/[0-9a-f-]{36}$/);
  await page.getByRole("button", { name: "Novo registro" }).click();
  await page.locator("#rec_title").fill("Alvo 1");
  await page.getByRole("region", { name: "Novo registro" }).getByRole("button", { name: "Criar", exact: true }).click();
  await expect(page.getByRole("row").filter({ hasText: "Alvo 1" })).toBeVisible();

  await page.goto("/collections/new");
  await page.locator("#c-name").fill(`links-${Date.now()}`);
  const form = page.getByRole("group", { name: "Adicionar campo" });
  await page.getByRole("button", { name: "Adicionar campo", exact: true }).click();
  await form.getByLabel("Nome do campo").fill("Dono");
  await form.getByRole("combobox", { name: "Tipo" }).click();
  await page.getByRole("option", { name: /^Referência/ }).click();
  await form.getByRole("combobox", { name: "Coleção referenciada" }).click();
  await page.getByRole("option", { name: target }).click();
  await form.getByRole("button", { name: "Adicionar campo" }).click();
  await form.getByLabel("Nome do campo").fill("Anexo");
  await form.getByRole("combobox", { name: "Tipo" }).click();
  await page.getByRole("option", { name: /^Arquivo/ }).click();
  await form.getByRole("button", { name: "Adicionar campo" }).click();
  await page.getByRole("region", { name: "Nova coleção" }).getByRole("button", { name: "Criar" }).click();
  await expect(page).toHaveURL(/\/collections\/[0-9a-f-]{36}$/);

  await page.getByRole("button", { name: "Novo registro" }).click();
  await page.locator("#rec_title").fill("Com ligação");
  await page.getByRole("button", { name: "Dono Escolher registro" }).click();
  await page.getByPlaceholder("Buscar registro").fill("Alvo");
  await page.getByRole("button", { name: "Alvo 1" }).click();
  await expect(page.getByRole("link", { name: "Alvo 1" })).toBeVisible();
  await page.locator(".rjsf input[type=file]").setInputFiles({ name: "nota.txt", mimeType: "text/plain", buffer: Buffer.from("olá") });
  await expect(page.getByRole("link", { name: /nota\.txt/ })).toBeVisible();
  await page.getByRole("region", { name: "Novo registro" }).getByRole("button", { name: "Criar", exact: true }).click();
  await expect(page).toHaveURL(/\/collections\/[0-9a-f-]{36}$/);
  const row = page.getByRole("row").filter({ hasText: "Com ligação" });
  await expect(row).toContainText("Alvo 1");
  const file = row.getByRole("link", { name: /nota\.txt/ });
  const res = await page.request.get(await file.getAttribute("href") as string);
  expect(res.headers()["content-disposition"]).toContain("attachment");
  expect(await res.text()).toBe("olá");
});
