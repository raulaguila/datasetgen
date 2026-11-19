import { expect, test, type Page } from "@playwright/test";
import { collectionAt, colOf, pageOf } from "./helpers";

const api = (page: Page, path: string, data: unknown) => page.request.post(`/api/proxy${path}`, { data });

// Moving a field and saving keeps the new order after a reload, and leaves nothing unsaved.
test("field order survives a save and a reload", async ({ page }) => {
  await page.goto("/");
  const id = (await (await api(page, "/v1/collections", {
    name: `ordem-${Date.now()}`,
    schema: { type: "object", properties: { primeiro: { type: "string", title: "Primeiro" }, segundo: { type: "string", title: "Segundo" } } },
  })).json()).id as string;
  await api(page, `${await colOf(page, id)}/records`, { data: { primeiro: "a", segundo: "b" } });
  await page.goto(`${await pageOf(page, id)}/fields`);
  const rows = page.getByRole("list", { name: "Campos" }).getByRole("listitem");
  await rows.nth(0).locator("[draggable=true]").dragTo(rows.nth(1).locator("[draggable=true]"));
  await expect(rows.first()).toContainText("Segundo");
  await page.getByRole("button", { name: "Salvar", exact: true }).click();
  await expect(page.getByText("Alterações salvas.").first()).toBeVisible();
  const saved = await (await page.request.get(`/api/proxy${await colOf(page, id)}`)).json();
  expect(Object.keys(saved.schema.properties)).toEqual(["segundo", "primeiro"]);
  // Saved means clean at once: leaving asks nothing, without a reload first.
  await expect(page.getByRole("button", { name: "Descartar" })).toHaveCount(0);
  await page.reload();
  await expect(page.getByRole("list", { name: "Campos" }).getByRole("listitem").first()).toContainText("Segundo");
  // Nothing left to save: the bar is gone and leaving asks nothing.
  await expect(page.getByRole("button", { name: "Salvar", exact: true })).toHaveCount(0);
});

// The same inside a group: its subfields keep their new order.
test("subfield order survives a save", async ({ page }) => {
  await page.goto("/");
  const id = (await (await api(page, "/v1/collections", {
    name: `ordem-grupo-${Date.now()}`,
    schema: { type: "object", required: ["title"], properties: {
      title: { type: "string", title: "Título" },
      endereco: { title: "Endereço", type: "object", required: ["rua", "numero"], properties: { rua: { title: "Rua", type: "string" }, numero: { title: "Número", type: "integer" } } },
    } },
  })).json()).id as string;
  await page.goto(`${await pageOf(page, id)}/fields`);
  await page.getByRole("button", { name: "Subir: Número" }).click();
  await page.getByRole("button", { name: "Subir: Endereço" }).click();
  await page.getByRole("button", { name: "Salvar", exact: true }).click();
  await expect(page.getByText("Alterações salvas.").first()).toBeVisible();
  const saved = await (await page.request.get(`/api/proxy${await colOf(page, id)}`)).json();
  expect(Object.keys(saved.schema.properties)).toEqual(["endereco", "title"]);
  expect(Object.keys(saved.schema.properties.endereco.properties)).toEqual(["numero", "rua"]);
  await expect(page.getByRole("button", { name: "Descartar" })).toHaveCount(0);
  await page.reload();
  await expect(page.getByRole("button", { name: "Salvar", exact: true })).toHaveCount(0);
});

// The side panel: a ready-made Status with colors, a conditional field in its own
// section, a duplicate; the record form then shows the section and the condition,
// and the record shows the option's color.
test("ready-made fields, conditions, sections and option colors", async ({ page }) => {
  await page.goto("/");
  const id = (await (await api(page, "/v1/collections", {
    name: `painel-${Date.now()}`,
    schema: { type: "object", required: ["title"], properties: { title: { type: "string", title: "Título" } } },
  })).json()).id as string;
  await page.goto(`${await pageOf(page, id)}/fields`);

  await page.getByRole("button", { name: "Adicionar campo", exact: true }).click();
  const panel = page.getByRole("dialog");
  await panel.getByRole("button", { name: "Status", exact: true }).click();
  await expect(panel).toBeHidden();
  const list = page.getByRole("list", { name: "Campos" });
  await expect(list.getByRole("listitem").filter({ hasText: "Status" })).toContainText("A fazer");

  await page.getByRole("button", { name: "Adicionar campo", exact: true }).click();
  await panel.getByRole("button", { name: "Texto curto", exact: true }).click();
  await panel.getByLabel("Nome do campo").fill("Motivo");
  // The panel previews the field as the form will show it.
  await expect(panel.locator("#field-preview_motivo")).toBeVisible();
  await panel.getByLabel("Seção do formulário").fill("Encerramento");
  await panel.getByRole("checkbox", { name: "Mostrar só quando outro campo tiver um valor" }).click();
  await panel.getByRole("combobox", { name: "Campo" }).click();
  await page.getByRole("option", { name: "Status" }).click();
  await panel.getByRole("combobox", { name: "Valor" }).click();
  await page.getByRole("option", { name: "Concluído" }).click();
  await panel.getByRole("button", { name: "Adicionar campo", exact: true }).click();
  await expect(panel).toBeHidden();
  await expect(list.getByRole("listitem").filter({ hasText: "Motivo" })).toContainText("Condicional");
  await expect(list.getByText("Encerramento")).toBeVisible();

  await page.getByRole("button", { name: "Duplicar campo: Motivo" }).click();
  await expect(list.getByText("Motivo (cópia)")).toBeVisible();
  await page.getByRole("button", { name: "Remover campo: Motivo (cópia)" }).click();

  await page.getByRole("button", { name: "Salvar", exact: true }).click();
  await expect(page.getByText("Alterações salvas.").first()).toBeVisible();
  const saved = (await (await page.request.get(`/api/proxy${await colOf(page, id)}`)).json()).schema;
  expect(saved.properties.status["x-colors"]).toEqual({ "A fazer": "gray", "Em andamento": "blue", "Concluído": "green" });
  expect(saved.properties.motivo).toMatchObject({ "x-section": "Encerramento", "x-show-when": { field: "status", op: "eq", value: "Concluído" } });

  // The record form: Motivo only once Status is Concluído, under its section.
  await page.goto(`${await pageOf(page, id)}`);
  await page.getByRole("button", { name: "Novo registro" }).click();
  await page.locator("#rec_title").fill("Pedido 1");
  await expect(page.locator("#rec_motivo")).toHaveCount(0);
  await page.locator("#rec_status").click();
  await page.getByRole("option", { name: "Concluído" }).click();
  await expect(page.locator("#rec_motivo")).toBeVisible();
  await expect(page.locator(".rjsf-section", { hasText: "Encerramento" })).toBeVisible();
  await page.locator("#rec_motivo").fill("Entregue");
  await page.getByRole("region", { name: "Novo registro" }).getByRole("button", { name: "Criar", exact: true }).click();
  await expect(page).toHaveURL(await collectionAt(page, id));
  await page.getByRole("link", { name: "Pedido 1" }).click();
  await expect(page.getByText("Concluído").first()).toHaveClass(/emerald/);
});
