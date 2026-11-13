import { expect, test, type Page } from "@playwright/test";

const api = (page: Page, path: string, data: unknown) => page.request.post(`/api/proxy${path}`, { data });

// The Analysis page measures each field; the overview's dashboard holds the widgets owners pick.
test("field analysis and a dashboard", async ({ page }) => {
  await page.goto("/");
  const schema = {
    type: "object",
    properties: {
      nome: { type: "string", title: "Nome" },
      tipo: { title: "Tipo", enum: ["gatilho", "efeito"] },
      preco: { type: "number", title: "Preço" },
      quando: { type: "string", format: "date", title: "Quando" },
    },
  };
  const made = await api(page, "/v1/collections", { name: `analise-${Date.now()}`, schema });
  const id = (await made.json()).id as string;
  for (const data of [
    { nome: "A", tipo: "gatilho", preco: 10, quando: "2026-01-05" },
    { nome: "B", tipo: "gatilho", preco: 30, quando: "2026-03-01" },
    { nome: "C", tipo: "efeito" },
  ]) expect((await api(page, "/v1/records", { collection_id: id, data })).ok()).toBeTruthy();

  await page.goto(`/collections/${id}/analysis`);
  await expect(page.getByRole("status")).toHaveText("3 registros");
  const preco = page.getByRole("region", { name: "Preço" });
  await expect(preco).toContainText("2 / 3 · 67%");
  await expect(preco).toContainText("Mínimo10");
  await expect(preco).toContainText("Máximo30");
  await expect(preco).toContainText("Média20");
  const tipo = page.getByRole("region", { name: "Tipo" });
  await expect(tipo).toContainText("2 valores diferentes");
  await expect(tipo).toContainText("gatilho");
  await expect(page.getByRole("region", { name: "Quando" })).toContainText("5 de jan. de 2026");
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.screenshot({ path: process.env.SHOT_DIR ? `${process.env.SHOT_DIR}/analysis.png` : "test-results/analysis.png", fullPage: true });

  // The dashboard: a count of one kind and a chart by kind.
  await page.goto(`/collections/${id}/overview`);
  const board = page.getByRole("region", { name: "Painel" });
  await board.getByRole("button", { name: /Monte o painel/ }).click();
  let dialog = page.getByRole("dialog", { name: "Adicionar widget" });
  await dialog.getByRole("button", { name: "Condições" }).click();
  const rules = page.getByRole("dialog", { name: "Condições" });
  await rules.getByRole("combobox", { name: "Campo da condição 1" }).click();
  await page.getByRole("option", { name: "Tipo" }).click();
  await rules.getByRole("combobox", { name: "Valor 1" }).click();
  await page.getByRole("option", { name: "gatilho" }).click();
  await rules.getByRole("button", { name: "Aplicar" }).click();
  await dialog.getByLabel("Título").fill("Gatilhos");
  await dialog.getByRole("button", { name: "Adicionar widget" }).click();
  await board.getByRole("button", { name: "Adicionar widget" }).click();
  dialog = page.getByRole("dialog", { name: "Adicionar widget" });
  await dialog.getByLabel("Tipo").click();
  await page.getByRole("option", { name: "Gráfico de barras" }).click();
  await dialog.getByLabel("Campo").click();
  await page.getByRole("option", { name: "Tipo" }).click();
  await dialog.getByRole("button", { name: "Adicionar widget" }).click();
  await board.getByRole("button", { name: "Salvar" }).click();
  await expect(board.getByRole("region", { name: "Gatilhos" })).toContainText("2");
  await expect(board.getByRole("region", { name: "Gráfico de barras: Tipo" })).toContainText("efeito");
  // Saved: a reload keeps it.
  await page.reload();
  await expect(board.getByRole("region", { name: "Gatilhos" })).toContainText("2");
  await page.screenshot({ path: process.env.SHOT_DIR ? `${process.env.SHOT_DIR}/dashboard.png` : "test-results/dashboard.png", fullPage: true });
});
