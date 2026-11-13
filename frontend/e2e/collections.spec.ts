import { expect, test, type Page } from "@playwright/test";

async function addField(page: Page, name: string, opts: { required?: boolean; options?: string; help?: string; type?: string } = {}) {
  const form = page.getByRole("group", { name: "Adicionar campo" });
  // The form opens from its button and stays open for the next field.
  if (!(await form.isVisible())) await page.getByRole("button", { name: "Adicionar campo", exact: true }).click();
  await form.getByLabel("Nome do campo").fill(name);
  const type = opts.options ? "Uma opção da lista" : opts.type;
  if (type) {
    await form.getByRole("combobox", { name: "Tipo" }).click();
    await page.getByRole("option", { name: type }).click();
  }
  if (opts.options) await form.getByLabel("Opções, separadas por vírgula").fill(opts.options);
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
  await expect(page.getByRole("columnheader")).toHaveText(["", "Nome", "Categoria", "Nota", "Atualizado em"]);
  const row = page.getByRole("row").filter({ hasText: "Ao clicar" });
  await expect(row.getByRole("cell")).toHaveText(["", "Ao clicar", "gatilho", "—", /\d/]);
  // A choice field filters through a select inside the filter popover.
  await page.getByRole("button", { name: "Filtros" }).click();
  await page.getByRole("combobox", { name: "Categoria" }).click();
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
  // A short list has no search box; a long one narrows to this collection.
  await expect(page.getByRole("button", { name, exact: true })).toBeVisible();
  const search = page.getByRole("textbox", { name: "Buscar por nome" });
  if (await search.isVisible()) await search.fill(name);
  const mine = page.getByRole("list", { name: "Minhas coleções" }).getByRole("listitem").filter({ hasText: name });
  await expect(mine).toContainText(["1 registro"]);
  await expect(mine).toContainText(["3 campos"]);
  await expect(mine).toContainText(["2 membros"]);
  await mine.getByRole("button", { name: `Fixar no topo: ${name}` }).click();
  await expect(page.getByRole("list", { name: "Favoritas" }).getByRole("button", { name, exact: true })).toBeVisible();
});

// A group keeps its own fields, drawn together in the record form; a free
// object takes any JSON and refuses text that does not parse.
test("groups hold subfields and free objects take JSON", async ({ page }) => {
  const name = `e2e-group-${Date.now()}`;
  await page.goto("/collections/new");
  await page.locator("#c-name").fill(name);
  await page.getByRole("button", { name: "Remover campo: Título" }).click();
  await page.getByRole("button", { name: "Remover campo: Notas" }).click();
  await addField(page, "Nome", { required: true });
  await addField(page, "Endereço", { type: "Grupo de campos" });
  await expect(page.getByRole("list", { name: "Campos", exact: true }).getByRole("listitem").filter({ hasText: "Endereço" }).first()).toContainText("0 subcampos");
  await page.getByRole("button", { name: "Adicionar subcampo", exact: true }).click();
  const sub = page.getByRole("group", { name: "Adicionar subcampo: Endereço" });
  await sub.getByLabel("Nome do campo").fill("Cidade");
  await sub.getByRole("checkbox", { name: "Obrigatório" }).click();
  await sub.getByRole("button", { name: "Adicionar subcampo" }).click();
  await expect(page.getByRole("list", { name: "Subcampos: Endereço" }).getByRole("listitem")).toHaveText([/^Cidade\*/]);
  // The group folds its subfields away.
  await page.getByRole("button", { name: "Recolher subcampos: Endereço" }).click();
  await expect(page.getByRole("list", { name: "Subcampos: Endereço" })).toHaveCount(0);
  await page.getByRole("button", { name: "Mostrar subcampos: Endereço" }).click();
  await addField(page, "Extra", { type: "Objeto livre (JSON)" });
  await page.getByRole("region", { name: "Nova coleção" }).getByRole("button", { name: "Criar" }).click();
  await expect(page).toHaveURL(/\/collections\/[0-9a-f-]{36}$/);

  await page.getByRole("button", { name: "Novo registro" }).click();
  // A new record starts in its first field.
  await expect(page.locator("#rec_nome")).toBeFocused();
  await page.locator("#rec_nome").fill("Casa");
  await page.locator("#rec_endereco_cidade").fill("Recife");
  const extra = page.locator("#rec_extra");
  await extra.fill("{oops");
  await expect(page.getByText("JSON inválido.")).toBeVisible();
  const save = page.getByRole("region", { name: "Novo registro" }).getByRole("button", { name: "Criar", exact: true });
  await save.click();
  await expect(page).toHaveURL(/\/records\/new/);
  await extra.fill('{"andar": 2, "bloco": "B"}');
  await expect(page.getByText("JSON inválido.")).toHaveCount(0);
  // Saving with the data kept leaves the form filled for a near-copy.
  await page.getByRole("button", { name: "Mais opções de salvar" }).click();
  await page.getByRole("menuitem", { name: /Salvar e criar outro com estes dados/ }).click();
  await expect(page.getByText(/Os dados continuam no formulário/)).toBeVisible();
  await expect(page).toHaveURL(/\/records\/new/);
  await expect(page.locator("#rec_endereco_cidade")).toHaveValue("Recife");
  await page.locator("#rec_nome").fill("Casa 2");
  // Ctrl+Enter saves from inside a text area.
  await extra.press("Control+Enter");
  await expect(page).toHaveURL(/\/collections\/[0-9a-f-]{36}$/);
  await expect(page.getByRole("row").filter({ hasText: "Casa 2" })).toHaveCount(1);
  const row = page.getByRole("row").filter({ hasText: "Casa" }).filter({ hasNotText: "Casa 2" });
  // The list carries plain values only; the record keeps the rest.
  await expect(row.getByRole("cell")).toHaveText(["", "Casa", /\d/]);
  await row.getByRole("link", { name: "Casa", exact: true }).click();
  await expect(page.locator("#rec_endereco_cidade")).toHaveValue("Recife");
  await expect(page.locator("#rec_extra")).toHaveValue(/"bloco": "B"/);
});

// A template fills the fields; a unique one refuses a value another record has,
// and the collection's log names each record by its title.
test("templates start collections and unique fields hold", async ({ page }) => {
  const name = `e2e-contacts-${Date.now()}`;
  await page.goto("/collections/new");
  // Create stays live: without a name it says what is missing.
  const create = page.getByRole("region", { name: "Nova coleção" }).getByRole("button", { name: "Criar" });
  await create.click();
  await expect(page.getByText("Dê um nome à coleção.").first()).toBeVisible();
  await page.getByRole("radio", { name: "Contatos" }).click();
  await expect(fieldsList(page).getByRole("listitem").filter({ hasText: "E-mail" })).toContainText(/único/i);
  await page.locator("#c-name").fill(name);
  await create.click();
  await expect(page).toHaveURL(/\/collections\/[0-9a-f-]{36}$/);
  const records = page.url();

  for (const who of ["Ana", "Bia"]) {
    await page.goto(records.replace(/\/collections\/([^/]+)$/, "/records/new?collection=$1"));
    await page.locator("#rec_nome").fill(who);
    await page.locator("#rec_e_mail").fill("mesmo@example.test");
    await page.getByRole("region", { name: "Novo registro" }).getByRole("button", { name: "Criar", exact: true }).click();
    if (who === "Ana") await expect(page).toHaveURL(/\/collections\/[0-9a-f-]{36}$/);
  }
  await expect(page.getByText("Outro registro já usa este valor.").first()).toBeVisible();

  await page.goto(`${records}/audit`);
  await expect(page.getByRole("row").filter({ hasText: "Hoje" })).toBeVisible();
  await expect(page.getByRole("row").filter({ hasText: "Registro criado" })).toContainText("Ana");
});
