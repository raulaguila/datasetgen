import { expect, test, type Page } from "@playwright/test";

async function addField(page: Page, name: string, opts: { required?: boolean; options?: string; help?: string } = {}) {
  const form = page.getByRole("group", { name: "Adicionar campo" });
  await form.getByLabel("Nome do campo").fill(name);
  if (opts.options) {
    await form.getByRole("combobox", { name: "Tipo" }).click();
    await page.getByRole("option", { name: "Uma opção da lista" }).click();
    await form.getByLabel("Opções, separadas por vírgula").fill(opts.options);
  }
  if (opts.help) {
    await form.getByText("Mais regras").click();
    await form.getByLabel("Texto de ajuda").fill(opts.help);
  }
  if (opts.required) await form.getByRole("checkbox", { name: "Obrigatório" }).click();
  await form.getByRole("button", { name: "Adicionar campo" }).click();
}

const fieldsList = (page: Page) => page.getByRole("list", { name: "Campos" });

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
  await addField(page, "Nota", { help: "De 0 a 10" });
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
  // Keys come from the labels; the help text shows under its field.
  await expect(page.getByText("De 0 a 10")).toBeVisible();

  // The browser checks before the server: the error lands under the field.
  await page.locator("#rec_nome").fill("Ao clicar");
  const save = page.getByRole("region", { name: "Novo registro" }).getByRole("button", { name: "Criar", exact: true });
  await save.click();
  await expect(page.getByText("Campo obrigatório.")).toBeVisible();

  await page.locator("#rec_categoria").selectOption("gatilho");
  await expect(page.getByText("Campo obrigatório.")).toHaveCount(0);
  await save.click();
  await expect(page).toHaveURL(/\/collections\/[0-9a-f-]{36}$/);
  // One column per field, named after it, after the selection box.
  await expect(page.getByRole("columnheader")).toHaveText(["", "Nome", "Categoria", "Nota", "Criado em"]);
  const row = page.getByRole("row").filter({ hasText: "Ao clicar" });
  await expect(row.getByRole("cell")).toHaveText(["", "Ao clicar", "gatilho", "—", /\d/]);
  // A choice field filters through a select inside the filter popover.
  await page.getByRole("button", { name: "Filtros" }).click();
  await page.getByLabel("Categoria").click();
  await page.getByRole("option", { name: "gatilho" }).click();
  await page.getByRole("button", { name: "Aplicar" }).click();
  await expect(page.getByRole("button", { name: "Remover filtro: Categoria" })).toBeVisible();
  await expect(row).toBeVisible();
  await expect(page).toHaveURL(/filter=/);
  await page.getByRole("button", { name: "Remover filtro: Categoria" }).click();

  // Fields live in their own section; changing them with records stored warns.
  await page.getByRole("link", { name: "Campos" }).click();
  await expect(page).toHaveURL(/\/fields$/);
  // With records stored, keys stay fixed; the edit happens in the row.
  await page.getByRole("button", { name: "Editar campo: Nota" }).click();
  const edit = page.getByRole("group", { name: "Editando: Nota" });
  await expect(edit.getByLabel("Chave")).toBeDisabled();
  await edit.getByRole("checkbox", { name: "Obrigatório" }).click();
  await edit.getByRole("button", { name: "Salvar campo" }).click();
  await expect(fieldsList(page).getByRole("listitem").filter({ hasText: "Nota" })).toContainText("Alterado");
  const summary = page.getByRole("status").filter({ hasText: "Esta coleção tem 1 registro(s)" });
  await expect(summary).toContainText("Passa a ser obrigatório: Nota");
  // Saving a risky change asks first.
  await page.getByRole("button", { name: "Salvar", exact: true }).click();
  const confirm = page.getByRole("dialog", { name: "Salvar mudanças nos campos?" });
  await expect(confirm).toBeVisible();
  await confirm.getByRole("button", { name: "Cancelar" }).click();
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
