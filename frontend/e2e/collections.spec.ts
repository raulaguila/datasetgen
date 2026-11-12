import { expect, test } from "@playwright/test";

// The whole schema-to-record path in a real browser: field order, the save of a
// schema with required fields, and errors under each field from the validator.
test("a collection's records follow its schema", async ({ page }) => {
  const name = `e2e-${Date.now()}`;
  // "nota" is shorter than "categoria": jsonb storage would have put it first.
  const schema = {
    type: "object",
    required: ["nome", "categoria"],
    properties: {
      nome: { type: "string", title: "Nome" },
      categoria: { type: "string", title: "Categoria", enum: ["gatilho", "efeito"] },
      nota: { type: "string", title: "Nota" },
    },
  };

  await page.goto("/collections/new");
  await page.locator("#c-name").fill(name);
  await page.getByRole("textbox", { name: "Schema" }).fill(JSON.stringify(schema, null, 2));
  // The preview's required fields stay empty: they must not block this save.
  await page.getByRole("region", { name: "Nova coleção" }).getByRole("button", { name: "Criar" }).click();
  await expect(page).toHaveURL(/\/collections\/[0-9a-f-]{36}$/);

  await page.getByRole("link", { name: "Novo registro" }).or(page.getByRole("button", { name: "Novo registro" })).click();
  await expect(page).toHaveURL(/\/records\/new\?collection=/);
  await expect(page.locator(".rjsf label.control-label")).toHaveText([/Nome/, /Categoria/, /Nota/]);

  // The browser checks before the server: the error lands under the field.
  await page.locator("#rec_nome").fill("Ao clicar");
  const save = page.getByRole("region", { name: "Novo registro" }).getByRole("button", { name: "Criar", exact: true });
  await save.click();
  await expect(page.getByText("Campo obrigatório.")).toBeVisible();

  await page.locator("#rec_categoria").selectOption("gatilho");
  await expect(page.getByText("Campo obrigatório.")).toHaveCount(0);
  await save.click();
  await expect(page).toHaveURL(/\/collections\/[0-9a-f-]{36}$/);
  await expect(page.getByRole("link", { name: "Ao clicar · gatilho" })).toBeVisible();
});
