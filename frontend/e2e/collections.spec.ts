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
// table, the sidebar sections, the warning when records already exist and members.
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
  // The sidebar lists it and opens its sections.
  await expect(page.getByRole("list", { name: "Coleções" }).getByRole("link", { name })).toHaveAttribute("aria-current", "true");

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

  // The creator owns it: its settings group lists the members, and an email
  // without an account is invited and joins at once, pending.
  await page.getByRole("button", { name: "Configurações" }).click();
  await page.getByRole("navigation", { name: "Configurações" }).getByRole("link", { name: "Membros" }).click();
  await expect(page).toHaveURL(/\/members$/);
  const helper = `${name}@example.test`;
  await page.getByLabel("E-mail").fill(helper);
  await page.getByRole("button", { name: "Adicionar membro" }).click();
  await page.getByLabel("Nome").fill("Ajudante");
  await page.getByRole("button", { name: "Convidar" }).click();
  const members = page.getByRole("list", { name: "Membros" });
  await expect(members.getByRole("listitem").filter({ hasText: helper })).toContainText("Convite pendente");
  await expect(members.getByRole("combobox", { name: "Papel: Ajudante" })).toContainText("Editor");

  // The list groups it with the caller's own collections, with its counts; a star moves it to favorites.
  await page.goto("/collections");
  await page.getByRole("textbox", { name: "Buscar por nome" }).fill(name);
  const mine = page.getByRole("list", { name: "Minhas coleções" }).getByRole("listitem");
  await expect(mine).toContainText(["1 registro"]);
  await expect(mine).toContainText(["3 campos"]);
  await expect(mine).toContainText(["2 membros"]);
  await mine.getByRole("button", { name: `Fixar no topo: ${name}` }).click();
  await expect(page.getByRole("list", { name: "Favoritas" }).getByRole("button", { name, exact: true })).toBeVisible();
});
