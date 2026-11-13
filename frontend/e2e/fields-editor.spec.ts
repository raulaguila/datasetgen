import { expect, test, type Page } from "@playwright/test";

const api = (page: Page, path: string, data: unknown) => page.request.post(`/api/proxy${path}`, { data });

// Moving a field and saving keeps the new order after a reload, and leaves nothing unsaved.
test("field order survives a save and a reload", async ({ page }) => {
  await page.goto("/");
  const id = (await (await api(page, "/v1/collections", {
    name: `ordem-${Date.now()}`,
    schema: { type: "object", properties: { primeiro: { type: "string", title: "Primeiro" }, segundo: { type: "string", title: "Segundo" } } },
  })).json()).id as string;
  await api(page, "/v1/records", { collection_id: id, data: { primeiro: "a", segundo: "b" } });
  await page.goto(`/collections/${id}/fields`);
  const rows = page.getByRole("list", { name: "Campos" }).getByRole("listitem");
  await rows.nth(0).locator("[draggable=true]").dragTo(rows.nth(1).locator("[draggable=true]"));
  await expect(rows.first()).toContainText("Segundo");
  await page.getByRole("button", { name: "Salvar", exact: true }).click();
  await expect(page.getByText("Salvo").first()).toBeVisible();
  const saved = await (await page.request.get(`/api/proxy/v1/collections/${id}`)).json();
  expect(Object.keys(saved.schema.properties)).toEqual(["segundo", "primeiro"]);
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
  await page.goto(`/collections/${id}/fields`);
  await page.getByRole("button", { name: "Subir: Número" }).click();
  await page.getByRole("button", { name: "Subir: Endereço" }).click();
  await page.getByRole("button", { name: "Salvar", exact: true }).click();
  await expect(page.getByText("Salvo").first()).toBeVisible();
  const saved = await (await page.request.get(`/api/proxy/v1/collections/${id}`)).json();
  expect(Object.keys(saved.schema.properties)).toEqual(["endereco", "title"]);
  expect(Object.keys(saved.schema.properties.endereco.properties)).toEqual(["numero", "rua"]);
  await page.reload();
  await expect(page.getByRole("button", { name: "Salvar", exact: true })).toHaveCount(0);
});
