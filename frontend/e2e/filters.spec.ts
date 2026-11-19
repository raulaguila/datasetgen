import { expect, test, type Page } from "@playwright/test";
import { colOf, pageOf } from "./helpers";

const api = (page: Page, path: string, data: unknown) => page.request.post(`/api/proxy${path}`, { data });
const day = (n: number) => { const d = new Date(); d.setDate(d.getDate() + n); return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`; };

// Dates relative to today, a record picked by name, options toggled, and a cell's quick filter.
test("richer conditions and the cell filter", async ({ page }) => {
  await page.goto("/");
  const people = (await (await api(page, "/v1/collections", { name: `pessoas-${Date.now()}`, schema: { type: "object", properties: { nome: { type: "string", title: "Nome" } } } })).json()).id as string;
  const ana = (await (await api(page, `${await colOf(page, people)}/records`, { data: { nome: "Ana Prado" } })).json()).id as string;
  const bia = (await (await api(page, `${await colOf(page, people)}/records`, { data: { nome: "Bia Lopes" } })).json()).id as string;
  const schema = {
    type: "object",
    properties: {
      titulo: { type: "string", title: "Título" },
      venc: { type: "string", format: "date", title: "Vencimento" },
      status: { title: "Status", enum: ["aberto", "pago", "atrasado"] },
      cliente: { type: "string", format: "uuid", "x-ref": people, title: "Cliente" },
    },
  };
  const id = (await (await api(page, "/v1/collections", { name: `contas-${Date.now()}`, schema })).json()).id as string;
  for (const data of [
    { titulo: "Luz", venc: day(-3), status: "atrasado", cliente: ana },
    { titulo: "Água", venc: day(2), status: "aberto", cliente: ana },
    { titulo: "Gás", venc: day(40), status: "pago", cliente: bia },
  ]) expect((await api(page, `${await colOf(page, id)}/records`, { data })).ok()).toBeTruthy();
  await page.goto(`${await pageOf(page, id)}?cols=titulo,venc,status,cliente`);
  await expect(page.locator("tbody tr")).toHaveCount(3);

  const rules = page.getByRole("dialog", { name: "Condições" });
  const pick = async (label: string, option: string) => {
    await rules.getByRole("combobox", { name: label }).click();
    await page.getByRole("option", { name: option, exact: true }).click();
  };
  // Due before today: only the late bill, and it stays right tomorrow too.
  await page.getByRole("button", { name: "Condições" }).click();
  await pick("Campo da condição 1", "Vencimento");
  await pick("Operador da condição 1", "antes de");
  await pick("Tipo de data 1", "hoje");
  await rules.getByRole("button", { name: "Aplicar" }).click();
  await expect(page.locator("tbody tr")).toHaveCount(1);
  await expect(page.getByRole("list", { name: "Condições ativas" })).toContainText("Vencimento antes de hoje");
  // A chip edits its own rule in place.
  await page.getByRole("button", { name: "Editar condição: Vencimento antes de hoje" }).click();
  const quick = page.getByRole("dialog", { name: "Editar condição" });
  await quick.getByRole("combobox", { name: "Operador da condição 1" }).click();
  await page.getByRole("option", { name: "depois de", exact: true }).click();
  await quick.getByRole("button", { name: /^Aplicar/ }).click();
  await expect(page.locator("tbody tr")).toHaveCount(2);
  await expect(page.getByRole("list", { name: "Condições ativas" })).toContainText("Vencimento depois de hoje");
  await page.getByRole("button", { name: /^Remover condição/ }).click();

  // Due in the next 7 days.
  await page.getByRole("button", { name: "Condições" }).click();
  await pick("Campo da condição 1", "Vencimento");
  await pick("Operador da condição 1", "nos próximos");
  await rules.getByLabel("Valor 1").fill("7");
  await rules.getByRole("button", { name: "Aplicar" }).click();
  await expect(page.locator("tbody tr")).toHaveCount(1);
  await expect(page.getByRole("row").filter({ hasText: "Água" })).toBeVisible();
  await page.getByRole("button", { name: /^Remover condição/ }).click();

  // A client picked by name, not by id; the chip shows the name.
  await page.getByRole("button", { name: "Condições" }).click();
  await pick("Campo da condição 1", "Cliente");
  await pick("Operador da condição 1", "é");
  await rules.getByRole("button", { name: "Valor 1" }).click();
  await rules.getByRole("searchbox").fill("Ana");
  await rules.getByRole("button", { name: "Ana Prado" }).click();
  await rules.getByRole("button", { name: "Aplicar" }).click();
  await expect(page.locator("tbody tr")).toHaveCount(2);
  await expect(page.getByRole("list", { name: "Condições ativas" })).toContainText("Cliente = Ana Prado");
  await page.getByRole("button", { name: /^Remover condição/ }).click();

  // Status is none of two options, toggled.
  await page.getByRole("button", { name: "Condições" }).click();
  await pick("Campo da condição 1", "Status");
  await pick("Operador da condição 1", "nenhum de");
  await rules.getByRole("button", { name: "aberto" }).click();
  await rules.getByRole("button", { name: "pago" }).click();
  await rules.getByRole("button", { name: "Aplicar" }).click();
  await expect(page.locator("tbody tr")).toHaveCount(1);
  await expect(page.getByRole("row").filter({ hasText: "Luz" })).toBeVisible();
  await page.getByRole("button", { name: /^Remover condição/ }).click();
  await expect(page.locator("tbody tr")).toHaveCount(3);

  // Written as text: a record by name, an option and a relative date, read into the rows.
  await page.getByRole("button", { name: "Condições" }).click();
  await rules.getByRole("button", { name: "Editar como texto" }).click();
  const box = rules.getByRole("textbox", { name: "Condições em texto" });
  await box.fill("status = fechado");
  await expect(rules.getByRole("alert")).toContainText("fechado não é opção de Status");
  await box.fill("status = pago");
  await expect(rules.getByRole("button", { name: "Aplicar · 1 registro" })).toBeVisible();
  await box.fill('cliente = "Ana Prado" e (status = atrasado ou vencimento nos próximos 7 dias)');
  await rules.getByRole("button", { name: "Editar visualmente" }).click();
  await expect(rules.getByRole("combobox", { name: "Campo da condição 1" })).toHaveText("Cliente");
  await expect(rules.getByRole("region", { name: "Grupo 2" })).toBeVisible();
  await rules.getByRole("button", { name: /^Aplicar/ }).click();
  await expect(page.locator("tbody tr")).toHaveCount(2);
  await expect(page.getByRole("status").filter({ hasText: "2 de 3 registros" })).toBeVisible();
  // Back as text, the record by its name.
  await page.getByRole("button", { name: "Condições" }).click();
  await rules.getByRole("button", { name: "Editar como texto" }).click();
  await expect(box).toHaveValue('Cliente = "Ana Prado" e (Status = "atrasado" ou Vencimento nos próximos 7 dias)');
  // Leaving with edits asks first.
  await box.fill("status = pago");
  await page.keyboard.press("Escape");
  await rules.getByRole("button", { name: "Descartar" }).click();
  await expect(rules).toBeHidden();
  await page.getByRole("button", { name: "Limpar filtro avançado" }).click();
  await expect(page.locator("tbody tr")).toHaveCount(3);

  // Right-click a cell: hide its value.
  await page.getByRole("row").filter({ hasText: "Gás" }).locator("td[data-label=Status]").click({ button: "right" });
  await page.getByRole("menuitem", { name: "Ocultar este valor" }).click();
  await expect(page.locator("tbody tr")).toHaveCount(2);
  await expect(page.getByRole("list", { name: "Condições ativas" })).toContainText("Status ≠ pago");
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.getByRole("button", { name: "Condições" }).click();
  await page.screenshot({ path: process.env.SHOT_DIR ? `${process.env.SHOT_DIR}/filters.png` : "test-results/filters.png" });
});
