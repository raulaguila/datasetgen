import { expect, test } from "@playwright/test";
import { admin, formAlert, login } from "./helpers";

// These start signed out; signing out here must not end the shared admin session.
test.use({ storageState: { cookies: [], origins: [] } });

test("a wrong password keeps the person on the login page", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("E-mail").fill(admin.email);
  await page.getByLabel("Senha", { exact: true }).fill("not the password at all");
  await page.getByRole("button", { name: "Entrar" }).click();
  await expect(page.locator(formAlert)).toBeVisible();
  await expect(page).toHaveURL(/\/login/);
});

test("the admin signs in and out", async ({ page }) => {
  await login(page, admin.email, admin.password);
  // With no collection yet, the admin lands on the collection list.
  await expect(page).toHaveURL(/\/collections/);
  await page.getByRole("button", { name: "Menu da conta" }).click();
  await page.getByRole("menuitem", { name: "Sair" }).click();
  await expect(page).toHaveURL(/\/login/);
  await page.goto("/admin/users");
  await expect(page).toHaveURL(/\/login/);
});
