import { expect, test, type Page } from "@playwright/test";

async function addField(page: Page, name: string, opts: { required?: boolean; options?: string } = {}) {
  await page.locator("#fb-name").fill(name);
  if (opts.options) {
    await page.getByRole("combobox", { name: "Tipo" }).click();
    await page.getByRole("option", { name: "Uma opção da lista" }).click();
    await page.locator("#fb-options").fill(opts.options);
  }
  if (opts.required) await page.getByRole("checkbox", { name: "Obrigatório" }).click();
  await page.getByRole("button", { name: "Adicionar campo" }).click();
}

// The whole path in a real browser: fields built without JSON, their order, the
// save of a schema with required fields, errors under each field, the record
// table, the sidebar sections and the warning when records already exist.
test("a collection's records follow its fields", async ({ page }) => {
  const name = `e2e-${Date.now()}`;
  await page.goto("/collections/new");
  await page.locator("#c-name").fill(name);

  // The starter fields go; three are built with the list instead of JSON.
  const fields = page.getByRole("list", { name: "Campos" });
  await page.getByRole("button", { name: "Remover campo: Título" }).click();
  await page.getByRole("button", { name: "Remover campo: Notas" }).click();
  await addField(page, "Nome", { required: true });
  await addField(page, "Nota");
  await addField(page, "Categoria", { required: true, options: "gatilho, efeito" });
  await page.getByRole("button", { name: "Subir: Categoria" }).click();
  await expect(fields.getByRole("listitem")).toHaveText([/^Nome\*/, /^Categoria\*/, /^Nota/]);

  // The preview's required fields stay empty: they must not block this save.
  await page.getByRole("region", { name: "Nova coleção" }).getByRole("button", { name: "Criar" }).click();
  await expect(page).toHaveURL(/\/collections\/[0-9a-f-]{36}$/);
  await expect(page.getByRole("button", { name: `Coleção: ${name}` })).toBeVisible();

  await page.getByRole("button", { name: "Novo registro" }).click();
  await expect(page).toHaveURL(/\/records\/new\?collection=/);
  await expect(page.locator(".rjsf label.control-label")).toHaveText([/Nome/, /Categoria/, /Nota/]);

  // The browser checks before the server: the error lands under the field.
  await page.locator("#rec_Nome").fill("Ao clicar");
  const save = page.getByRole("region", { name: "Novo registro" }).getByRole("button", { name: "Criar", exact: true });
  await save.click();
  await expect(page.getByText("Campo obrigatório.")).toBeVisible();

  await page.locator("#rec_Categoria").selectOption("gatilho");
  await expect(page.getByText("Campo obrigatório.")).toHaveCount(0);
  await save.click();
  await expect(page).toHaveURL(/\/collections\/[0-9a-f-]{36}$/);
  // One column per field, named after it.
  await expect(page.getByRole("columnheader")).toHaveText(["Nome", "Categoria", "Nota", /Atualizado/]);
  const row = page.getByRole("row").filter({ hasText: "Ao clicar" });
  await expect(row.getByRole("cell")).toHaveText(["Ao clicar", "gatilho", "—", /\d/]);

  // Fields live in their own section; changing them with records stored warns.
  await page.getByRole("link", { name: "Campos" }).click();
  await expect(page).toHaveURL(/\/fields$/);
  await page.getByRole("button", { name: "Editar campo: Nota" }).click();
  await page.getByRole("checkbox", { name: "Obrigatório" }).click();
  await page.getByRole("button", { name: "Salvar campo" }).click();
  await expect(page.getByRole("status").filter({ hasText: "Esta coleção tem 1 registro(s)" })).toBeVisible();
  await page.getByRole("button", { name: "Descartar" }).click();
  await expect(page.getByRole("status").filter({ hasText: "Esta coleção tem" })).toHaveCount(0);
});
