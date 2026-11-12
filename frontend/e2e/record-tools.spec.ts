import { readFile } from "node:fs/promises";
import { expect, test } from "@playwright/test";

// Import with a check first, field filters, export, a record's history and
// restore, the background quality check, a bulk fix and duplicating.
test("records come in, go out and keep their history", async ({ page }) => {
  const name = `tools-${Date.now()}`;
  await page.goto("/collections/new");
  await page.locator("#c-name").fill(name);
  // The starter fields: Título (title, required) and Notas (notes).
  await page.getByRole("region", { name: "Nova coleção" }).getByRole("button", { name: "Criar" }).click();
  await expect(page).toHaveURL(/\/collections\/[0-9a-f-]{36}$/);
  const records = page.url();

  // Import: one valid line, one missing the required title, one not JSON.
  await page.getByRole("button", { name: "Importar" }).click();
  await page.locator("#import-file").setInputFiles({
    name: "dados.jsonl",
    mimeType: "application/x-ndjson",
    buffer: Buffer.from('{"title":"Primeiro","notes":"a"}\n{"notes":"sem título"}\nnão é json\n{"title":"Segundo","notes":"b"}\n'),
  });
  const dialog = page.getByRole("dialog", { name: "Importar" });
  await expect(dialog.getByText("2 registro(s) válido(s), 2 com problema.")).toBeVisible();
  await expect(dialog.getByRole("list", { name: "Problemas encontrados" }).getByRole("listitem")).toHaveCount(2);
  await expect(dialog.getByRole("button", { name: "Importar 2" })).toBeDisabled();
  await dialog.getByRole("checkbox").click();
  await dialog.getByRole("button", { name: "Importar 2" }).click();
  await expect(dialog).toBeHidden();
  await expect(page.getByRole("row").filter({ hasText: "Primeiro" })).toBeVisible();
  await expect(page.getByRole("row").filter({ hasText: "Segundo" })).toBeVisible();

  // An exact-value filter, shown as a removable chip.
  await page.getByRole("button", { name: "Filtros" }).click();
  await page.getByLabel("Notas").fill("b");
  await page.getByRole("button", { name: "Aplicar" }).click();
  await expect(page.getByRole("row").filter({ hasText: "Segundo" })).toBeVisible();
  await expect(page.getByRole("row").filter({ hasText: "Primeiro" })).toHaveCount(0);
  await page.getByRole("button", { name: "Remover filtro: Notas" }).click();
  await expect(page.getByRole("row").filter({ hasText: "Primeiro" })).toBeVisible();

  // Export as JSON Lines.
  const [download] = await Promise.all([
    page.waitForEvent("download"),
    page.getByRole("button", { name: "Exportar" }).click().then(() => page.getByRole("menuitem", { name: "JSON Lines (.jsonl)" }).click()),
  ]);
  expect(download.suggestedFilename()).toMatch(/\.jsonl$/);
  const lines = (await readFile(await download.path(), "utf8")).trim().split("\n");
  expect(lines.map((l) => JSON.parse(l).title).sort()).toEqual(["Primeiro", "Segundo"]);

  // An edit keeps the old version; restoring brings it back.
  await page.getByRole("link", { name: "Primeiro" }).click();
  await page.locator("#rec_title").fill("Primeiro editado");
  await page.getByRole("button", { name: "Salvar" }).click();
  await expect(page).toHaveURL(records);
  await page.getByRole("link", { name: "Primeiro editado" }).click();
  const history = page.getByRole("list", { name: "Histórico" });
  await expect(history.getByText(/^Versão \d+$/)).toHaveText(["Versão 1"]);
  await expect(history).toContainText("Primeiro → Primeiro editado");
  await history.getByRole("button", { name: "Restaurar" }).click();
  await page.getByRole("dialog", { name: "Restaurar" }).getByRole("button", { name: "Restaurar" }).click();
  await expect(page.locator("#rec_title")).toHaveValue("Primeiro");
  await expect(history.getByText(/^Versão \d+$/)).toHaveText(["Versão 2", "Versão 1"]);

  // Quality runs in the background; a bulk fix then a new check.
  await page.goto(records.replace(/$/, "/quality"));
  await expect(page.getByText("Todos os registros seguem os campos atuais.")).toBeVisible({ timeout: 15_000 });
  await page.getByLabel("O que fazer").click();
  await page.getByRole("option", { name: "Renomear o campo" }).click();
  await page.getByLabel("Nome atual").fill("title");
  await page.getByLabel("Novo nome").fill("titulo");
  await page.getByRole("button", { name: "Aplicar" }).click();
  await page.getByRole("dialog", { name: "Aplicar a todos os registros?" }).getByRole("button", { name: "Aplicar" }).click();
  await expect(page.getByText("2 registro(s) alterado(s)")).toBeVisible();
  await expect(page.getByRole("heading", { name: "2 registro(s) fora do formato" })).toBeVisible({ timeout: 15_000 });

  // Duplicating copies fields and, when asked, records.
  await page.goto("/collections");
  await page.getByRole("textbox", { name: "Buscar por nome" }).fill(name);
  await page.getByRole("button", { name: `Ações: ${name}` }).click();
  await page.getByRole("menuitem", { name: "Duplicar" }).click();
  const dup = page.getByRole("dialog", { name: "Duplicar" });
  await expect(dup.getByLabel("Nome")).toHaveValue(`${name} (cópia)`);
  await dup.getByRole("checkbox").click();
  await dup.getByRole("button", { name: "Duplicar" }).click();
  await expect(page).toHaveURL(/\/overview$/);
  await expect(page.getByRole("heading", { name: "Visão geral" })).toBeVisible();
  await expect(page.getByText(`${name} (cópia)`).first()).toBeVisible();
});
