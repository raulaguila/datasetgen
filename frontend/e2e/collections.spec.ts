import { expect, test, type Page } from "@playwright/test";
import { addField, newCollection, atCollection } from "./helpers";

const fieldsList = (page: Page) => page.getByRole("list", { name: "Campos" });

// The whole path in a real browser: fields built without JSON, their order, the
// save of a schema with required fields, errors under each field, the record
// table, the sidebar sections, the warning when records already exist and members.
test("a collection's records follow its fields", async ({ page }) => {
  const name = `e2e-${Date.now()}`;
  const records = await newCollection(page, name, "Em branco");

  // Fields are built on the new collection's Fields page, with the list instead of JSON.
  const fields = page.getByRole("list", { name: "Campos" });
  await addField(page, "Nome", { required: true });
  await addField(page, "Nota", { help: "De 0 a 10" });
  await addField(page, "Categoria", { required: true, options: "gatilho, efeito" });
  await page.getByRole("button", { name: "Subir: Categoria" }).click();
  await expect(fields.getByRole("listitem")).toHaveText([/^NomeObrigatório/, /^CategoriaObrigatório/, /^Nota/]);

  // The preview's required fields stay empty: they must not block this save.
  await page.getByRole("button", { name: "Salvar", exact: true }).click();
  await expect(page.getByText("Alterações salvas.").first()).toBeVisible();
  await page.goto(records);
  // The sidebar lists it and opens its sections.
  await expect(page.getByRole("button", { name: `${name}: Trocar de coleção` })).toBeVisible();

  await page.getByRole("button", { name: "Novo registro" }).click();
  await expect(page).toHaveURL(atCollection("r/new"));
  await expect(page.locator(".rjsf label.control-label")).toHaveText([/Nome/, /Categoria/, /Nota/]);
  // Keys come from the labels; the help text shows under its field.
  await expect(page.getByText("De 0 a 10")).toBeVisible();

  // The browser checks before the server: the error lands under the field.
  await page.locator("#rec_nome").fill("Ao clicar");
  const save = page.getByRole("region", { name: "Novo registro" }).getByRole("button", { name: "Criar", exact: true });
  await save.click();
  await expect(page.getByText("Campo obrigatório.")).toBeVisible();

  await page.locator("#rec_categoria").click();
  await page.getByRole("option", { name: "gatilho" }).click();
  await expect(page.getByText("Campo obrigatório.")).toHaveCount(0);
  await save.click();
  await expect(page).toHaveURL(atCollection());
  // One column per field, named after it, after the selection box.
  await expect(page.getByRole("columnheader")).toHaveText(["", "Nome", "Categoria", "Nota", "Atualizado em"]);
  const row = page.getByRole("row").filter({ hasText: "Ao clicar" });
  await expect(row.getByRole("cell")).toHaveText(["", "Ao clicar", "gatilho", "—", /\S/]);
  // A choice field: its condition offers the options.
  await page.getByRole("button", { name: "Condições" }).click();
  const rules = page.getByRole("dialog", { name: "Condições" });
  await rules.getByRole("combobox", { name: "Campo da condição 1" }).click();
  await page.getByRole("option", { name: "Categoria" }).click();
  await rules.getByRole("combobox", { name: "Valor 1" }).click();
  await page.getByRole("option", { name: "gatilho" }).click();
  await rules.getByRole("button", { name: "Aplicar" }).click();
  await expect(row).toBeVisible();
  await expect(page).toHaveURL(/where=/);
  await page.getByRole("button", { name: /^Remover condição: Categoria/ }).click();
  // A field filter from an older link or saved view reads as a condition.
  await page.goto(`${page.url().split("?")[0]}?filter=${encodeURIComponent('{"categoria":"gatilho"}')}`);
  await expect(page.getByRole("button", { name: /^Remover condição: Categoria/ })).toBeVisible();
  await expect(row).toBeVisible();
  await page.getByRole("button", { name: /^Remover condição: Categoria/ }).click();
  await expect(page).not.toHaveURL(/filter=/);

  // Fields live in their own section; changing them with records stored warns.
  await page.getByRole("link", { name: "Campos" }).click();
  await expect(page).toHaveURL(/\/fields$/);
  // With records stored, keys stay fixed; the edit happens in the row.
  await page.getByRole("button", { name: "Editar campo: Nota" }).click();
  const edit = page.getByRole("dialog", { name: "Editando: Nota" });
  await expect(edit.getByLabel("Chave")).toBeDisabled();
  await edit.getByRole("checkbox", { name: "Obrigatório" }).click();
  await edit.getByRole("button", { name: "Salvar campo" }).click();
  await expect(fieldsList(page).getByRole("listitem").filter({ hasText: "Nota" })).toContainText("Alterado");
  const summary = page.getByRole("status").filter({ hasText: "Esta coleção tem 1 registro." });
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
  await page.getByRole("group", { name: "Gestão" }).getByRole("link", { name: "Membros" }).click();
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
  await expect(page.getByRole("main").getByRole("link", { name, exact: true })).toBeVisible();
  const search = page.getByRole("textbox", { name: "Buscar por nome" });
  if (await search.isVisible()) await search.fill(name);
  const mine = page.getByRole("list", { name: "Minhas coleções" }).getByRole("listitem").filter({ hasText: name });
  await expect(mine).toContainText(["1 registro"]);
  await expect(mine).toContainText(["3 campos"]);
  await expect(mine).toContainText(["2 membros"]);
  await mine.getByRole("button", { name: `Fixar no topo: ${name}` }).click();
  await expect(page.getByRole("list", { name: "Favoritas" }).getByRole("link", { name, exact: true })).toBeVisible();
});

// A group keeps its own fields, drawn together in the record form; a free
// object takes any JSON and refuses text that does not parse.
test("groups hold subfields and free objects take JSON", async ({ page }) => {
  const name = `e2e-group-${Date.now()}`;
  const records = await newCollection(page, name, "Em branco");
  await addField(page, "Nome", { required: true });
  await addField(page, "Endereço", { type: "Grupo de campos" });
  await expect(page.getByRole("list", { name: "Campos", exact: true }).getByRole("listitem").filter({ hasText: "Endereço" }).first()).toContainText("0 subcampos");
  await addField(page, "Cidade", { required: true, sub: true });
  await expect(page.getByRole("list", { name: "Subcampos: Endereço" }).getByRole("listitem")).toHaveText([/^CidadeObrigatório/]);
  // The group folds its subfields away.
  await page.getByRole("button", { name: "Recolher subcampos: Endereço" }).click();
  await expect(page.getByRole("list", { name: "Subcampos: Endereço" })).toHaveCount(0);
  await page.getByRole("button", { name: "Mostrar subcampos: Endereço" }).click();
  await addField(page, "Extra", { type: "Objeto livre (JSON)" });
  await page.getByRole("button", { name: "Salvar", exact: true }).click();
  await expect(page.getByText("Alterações salvas.").first()).toBeVisible();
  await page.goto(records);

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
  await expect(page).toHaveURL(atCollection("r/new"));
  await extra.fill('{"andar": 2, "bloco": "B"}');
  await expect(page.getByText("JSON inválido.")).toHaveCount(0);
  // Saving with the data kept leaves the form filled for a near-copy.
  await page.getByRole("button", { name: "Mais opções de salvar" }).click();
  await page.getByRole("menuitem", { name: /Salvar e criar outro com estes dados/ }).click();
  await expect(page.getByText(/Os dados continuam no formulário/)).toBeVisible();
  await expect(page).toHaveURL(atCollection("r/new"));
  await expect(page.locator("#rec_endereco_cidade")).toHaveValue("Recife");
  await page.locator("#rec_nome").fill("Casa 2");
  // Ctrl+Enter saves from inside a text area.
  await extra.press("Control+Enter");
  await expect(page).toHaveURL(atCollection());
  await expect(page.getByRole("row").filter({ hasText: "Casa 2" })).toHaveCount(1);
  const row = page.getByRole("row").filter({ hasText: "Casa" }).filter({ hasNotText: "Casa 2" });
  // The list carries plain values only; the record keeps the rest.
  await expect(row.getByRole("cell")).toHaveText(["", "Casa", /\S/]);
  await row.getByRole("link", { name: "Casa", exact: true }).click();
  await expect(page.getByRole("main")).toContainText("Recife");
  await page.getByRole("button", { name: /^Editar( E)?$/ }).click();
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
  await page.getByLabel("Modelo").click();
  await page.getByRole("option", { name: "Contatos" }).click();
  await page.locator("#c-name").fill(name);
  await create.click();
  await expect(page).toHaveURL(atCollection("fields"));
  await expect(fieldsList(page).getByRole("listitem").filter({ hasText: "E-mail" })).toContainText(/único/i);
  const records = page.url().replace(/\/fields$/, "");

  for (const who of ["Ana", "Bia"]) {
    await page.goto(`${records}/r/new`);
    await page.locator("#rec_nome").fill(who);
    await page.locator("#rec_e_mail").fill("mesmo@example.test");
    await page.getByRole("region", { name: "Novo registro" }).getByRole("button", { name: "Criar", exact: true }).click();
    if (who === "Ana") await expect(page).toHaveURL(atCollection());
  }
  await expect(page.getByText("Outro registro já usa este valor.").first()).toBeVisible();

  await page.goto(`${records}/audit`);
  await expect(page.getByRole("row").filter({ hasText: "Hoje" })).toBeVisible();
  await expect(page.getByRole("row").filter({ hasText: "Registro criado" })).toContainText("Ana");
});

// A whole-collection export carries name, icon, fields and records: the new-collection page recreates it in one go.
test("a whole-collection export recreates the collection", async ({ page }) => {
  const name = `e2e-bundle-${Date.now()}`;
  const records = await newCollection(page, name, "Contatos");
  await page.goto(`${records}/r/new`);
  await page.locator("#rec_nome").fill("Ana");
  await page.getByRole("region", { name: "Novo registro" }).getByRole("button", { name: "Criar", exact: true }).click();
  await expect(page).toHaveURL(atCollection());

  await page.getByRole("button", { name: "Exportar" }).click();
  const [download] = await Promise.all([page.waitForEvent("download"), page.getByRole("menuitem", { name: /Coleção completa/ }).click()]);
  expect(download.suggestedFilename()).toMatch(/\.collection\.jsonl$/);

  await page.goto("/collections/new");
  await page.getByRole("radio", { name: /Importar de um arquivo/ }).click();
  await page.locator("input[type=file]").setInputFiles(await download.path());
  await expect(page.locator("#c-name")).toHaveValue(name);
  await page.locator("#c-name").fill(`${name}-copia`);
  await page.getByRole("region", { name: "Nova coleção" }).getByRole("button", { name: "Criar" }).click();
  await expect(page).toHaveURL(atCollection());
  const dialog = page.getByRole("dialog", { name: "Importar" });
  await dialog.getByRole("button", { name: /^Importar 1/ }).click();
  await expect(page.getByRole("row").filter({ hasText: "Ana" })).toBeVisible();
  await page.goto(`${page.url()}/fields`);
  await expect(fieldsList(page).getByRole("listitem").filter({ hasText: "E-mail" })).toContainText(/único/i);
});
