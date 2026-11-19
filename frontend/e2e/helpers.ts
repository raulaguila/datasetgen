import { expect, type Browser, type Locator, type Page } from "@playwright/test";

function need(name: string): string {
  const value = process.env[name];
  if (!value) throw new Error(`${name} is not set; see e2e/README.md`);
  return value;
}

export const admin = { email: need("E2E_ADMIN_EMAIL"), password: need("E2E_ADMIN_PASSWORD") };
export const adminState = "e2e/.auth/admin.json";
export const apiUrl = process.env.E2E_API_URL ?? "http://127.0.0.1:9990";
export const strongPassword = "Quiet river lantern 42";

/** A collection's own address, /<owner>/<slug>[/<section>], not an old /collections/<id> one. */
export const atCollection = (section?: string) =>
  new RegExp(`^https?://[^/]+/(?!collections/|records/)[a-z0-9-]+/[a-z0-9-]+${section ? `/${section}` : ""}$`);

/** The collection id behind a collection address. */
export async function collectionIdOf(page: Page, url: string): Promise<string> {
  const [owner, slug] = new URL(url).pathname.split("/").slice(1, 3);
  const res = await page.request.get(`/api/proxy/v1/collections/path/${owner}/${slug}`);
  expect(res.ok()).toBeTruthy();
  return (await res.json()).id as string;
}

/** Matches the address of the collection with this id, once it is open. */
export async function collectionAt(page: Page, id: string): Promise<RegExp> {
  const c = await (await page.request.get(`/api/proxy/v1/collections/${id}`)).json();
  return new RegExp(`/${c.owner}/${c.slug}$`);
}

/** An address no earlier run used, so the flows never collide with old data. */
export const uniqueEmail = (tag: string) => `e2e-${tag}-${Date.now()}@example.test`;

/**
 * Creates a collection from a template ("Título e notas" when none is named) and
 * returns its records URL; the page is left on the new collection's Fields.
 */
export async function newCollection(page: Page, name: string, template?: string) {
  await page.goto("/collections/new");
  await page.locator("#c-name").fill(name);
  if (template) {
    await page.getByLabel("Modelo").click();
    await page.getByRole("option", { name: template }).click();
  }
  await page.getByRole("region", { name: "Nova coleção" }).getByRole("button", { name: "Criar" }).click();
  await expect(page).toHaveURL(atCollection("fields"));
  return page.url().replace(/\/fields$/, "");
}

export async function login(page: Page, email: string, password: string) {
  await page.goto("/login");
  await page.getByLabel("E-mail").fill(email);
  await page.getByLabel("Senha", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Entrar" }).click();
  // "/" forwards into the app (a collection, the user list or the profile).
  await expect(page).not.toHaveURL(/\/login/);
  await expect(page.getByRole("button", { name: "Menu da conta" })).toBeVisible();
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

/** The page's own error, not Next's route announcer (also role="alert", and titled in production builds). */
export const formAlert = '[role="alert"]:not(#__next-route-announcer__)';

/** Deletes the caller's key by name, so runs never pile up to the 20-key cap. */
export async function dropKey(page: Page, keyName: string) {
  const keys = (await (await page.request.get("/api/proxy/v1/me/keys")).json()).items as { id: string; name: string }[];
  const key = keys.find((k) => k.name === keyName);
  expect(key).toBeTruthy();
  expect((await page.request.delete(`/api/proxy/v1/me/keys/${key!.id}`)).status()).toBe(204);
}

/**
 * Adds a field through the side panel: the type from the gallery, then the form.
 * `sub` adds a subfield with the "Adicionar subcampo" button inside `scope`.
 */
export async function addField(page: Page, name: string, opts: { required?: boolean; options?: string; help?: string; type?: string; sub?: boolean; scope?: Locator } = {}) {
  const label = opts.sub ? "Adicionar subcampo" : "Adicionar campo";
  await (opts.scope ?? page).getByRole("button", { name: label, exact: true }).click();
  const panel = page.getByRole("dialog");
  await panel.getByRole("button", { name: opts.options ? "Uma opção da lista" : opts.type ?? "Texto curto", exact: true }).click();
  await panel.getByLabel("Nome do campo").fill(name);
  for (const o of (opts.options ?? "").split(",").map((x) => x.trim()).filter(Boolean)) {
    await panel.getByLabel("Nova opção").fill(o);
    await panel.getByLabel("Nova opção").press("Enter");
  }
  if (opts.help) await panel.getByLabel("Texto de ajuda").fill(opts.help);
  if (opts.required) await panel.getByRole("checkbox", { name: "Obrigatório" }).click();
  await panel.getByRole("button", { name: label, exact: true }).click();
  await expect(panel).toBeHidden();
}
