import { expect, type Browser, type Page } from "@playwright/test";

function need(name: string): string {
  const value = process.env[name];
  if (!value) throw new Error(`${name} is not set; see e2e/README.md`);
  return value;
}

export const admin = { email: need("E2E_ADMIN_EMAIL"), password: need("E2E_ADMIN_PASSWORD") };
export const adminState = "e2e/.auth/admin.json";
export const apiUrl = process.env.E2E_API_URL ?? "http://127.0.0.1:9990";
export const strongPassword = "Quiet river lantern 42";

/** An address no earlier run used, so the flows never collide with old data. */
export const uniqueEmail = (tag: string) => `e2e-${tag}-${Date.now()}@example.test`;

export async function login(page: Page, email: string, password: string) {
  await page.goto("/login");
  await page.getByLabel("E-mail").fill(email);
  await page.getByLabel("Senha", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Entrar" }).click();
  await expect(page).toHaveURL("/");
}

/** Invites a person through the admin screen and returns the activation link the development backend hands back. */
export async function invite(page: Page, name: string, email: string): Promise<string> {
  await page.goto("/admin/users/new");
  await page.getByLabel("Nome").fill(name);
  await page.getByLabel("E-mail").fill(email);
  await page.getByRole("button", { name: "Enviar convite" }).click();
  const link = page.locator("input[readonly]");
  await expect(link).toHaveValue(/\/activate#token=/);
  return link.inputValue();
}

export async function choosePassword(page: Page, password: string) {
  await page.getByLabel("Nova senha").fill(password);
  await page.getByLabel("Confirmar senha").fill(password);
}

/** A page with no session: the config hands every new page the admin's. */
export const guestPage = (browser: Browser) => browser.newPage({ locale: "pt-BR", storageState: { cookies: [], origins: [] } });
