import { expect, test } from "@playwright/test";

// An owner edits a webhook (the secret stays), pauses it and resumes it; a share
// link takes a validity.
test("webhooks pause and edit; share links end", async ({ page }) => {
  const made = await page.request.post("/api/proxy/v1/collections", { data: { name: `hooks-${Date.now()}`, schema: { type: "object" } } });
  expect(made.status()).toBe(201);
  const id = (await made.json()).id as string;
  const hook = await page.request.post(`/api/proxy/v1/collections/${id}/webhooks`, { data: { url: "https://example.com/a", events: ["record.created"] } });
  expect(hook.status()).toBe(201);

  await page.goto(`/collections/${id}/integrations#webhooks`);
  const row = page.getByRole("listitem").filter({ hasText: "https://example.com/a" });
  await row.getByRole("button", { name: "Pausar" }).click();
  await expect(row.getByText("Pausado", { exact: true })).toBeVisible();
  await row.getByRole("button", { name: "Retomar" }).click();
  await expect(row.getByText("Pausado", { exact: true })).toBeHidden();

  await row.getByRole("button", { name: /^Editar/ }).click();
  const form = page.getByRole("form", { name: "Editar webhook" });
  await expect(form.getByText("O segredo de assinatura continua o mesmo.")).toBeVisible();
  await form.getByLabel("Endereço (https)").fill("https://example.com/b");
  await form.getByRole("button", { name: "Salvar" }).click();
  await expect(page.getByRole("listitem").filter({ hasText: "https://example.com/b" })).toBeVisible();

  // A link made for 7 days says when it ends.
  await page.getByRole("tab", { name: "Links públicos" }).click();
  await page.getByRole("combobox", { name: "Validade do link" }).click();
  await page.getByRole("option", { name: /7 dias/ }).click();
  await page.getByRole("button", { name: /formulário/i }).first().click();
  await expect(page.getByText(/vence /).first()).toBeVisible();
});
