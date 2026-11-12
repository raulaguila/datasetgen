import { expect, test } from "@playwright/test";

test("a role keeps the permissions it was saved with", async ({ page }) => {
  await page.goto("/admin/roles/new");
  const name = `e2e-${Date.now()}`;
  await page.getByLabel("Nome").fill(name);

  // Audit is read-only: the matrix offers no other box for it.
  await expect(page.getByRole("checkbox", { name: "Auditoria: Ler" })).toBeVisible();
  await expect(page.getByRole("checkbox", { name: "Auditoria: Criar" })).toHaveCount(0);

  await page.getByRole("checkbox", { name: "Produtos: Ler" }).click();
  await page.getByRole("checkbox", { name: "Produtos: Editar" }).click();
  await page.getByRole("region", { name: /Alterações não salvas|Novo papel/ }).getByRole("button", { name: /Salvar|Criar/ }).click();
  await expect(page).toHaveURL(/\/admin\/roles$/); // a new role goes back to the list

  await page.getByRole("link", { name }).click();
  await expect(page).toHaveURL(/\/admin\/roles\/[0-9a-f-]{36}$/);
  await expect(page.getByRole("checkbox", { name: "Produtos: Ler" })).toBeChecked();
  await expect(page.getByRole("checkbox", { name: "Produtos: Editar" })).toBeChecked();
  await expect(page.getByRole("checkbox", { name: "Produtos: Excluir" })).not.toBeChecked();
});
