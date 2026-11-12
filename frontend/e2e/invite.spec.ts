import { expect, test } from "@playwright/test";
import { choosePassword, invite, login, strongPassword, uniqueEmail , guestPage } from "./helpers";

test("an invited person activates the account and signs in", async ({ page, browser }) => {
  const email = uniqueEmail("invite");
  const link = await invite(page, "Pessoa Convidada", email);

  const guest = await guestPage(browser);
  await guest.goto(link);
  await expect(guest).not.toHaveURL(/#token=/); // the token leaves the address bar
  await choosePassword(guest, strongPassword);
  await guest.getByRole("button", { name: "Ativar conta" }).click();
  await expect(guest.getByText("Conta ativada")).toBeVisible();

  await login(guest, email, strongPassword);
  await expect(guest.getByRole("heading", { level: 1 })).toContainText("Olá, Pessoa");
  await guest.close();
});

test("an activation link works only once", async ({ page, browser }) => {
  const link = await invite(page, "Pessoa Repetida", uniqueEmail("once"));

  for (const attempt of [1, 2]) {
    const guest = await guestPage(browser);
    await guest.goto(link);
    await choosePassword(guest, strongPassword);
    await guest.getByRole("button", { name: "Ativar conta" }).click();
    if (attempt === 1) await expect(guest.getByText("Conta ativada")).toBeVisible();
    else await expect(guest.getByRole("alert").filter({ hasText: /\S/ })).toBeVisible();
    await guest.close();
  }
});

test("an admin hands over a new activation link and the old one stops working", async ({ page, browser }) => {
  const email = uniqueEmail("link");
  const first = await invite(page, "Pessoa Link", email);
  await page.goto(`/admin/users?q=${encodeURIComponent(email)}`);
  await page.getByRole("link", { name: "Pessoa Link" }).click();
  await expect(page.getByText(/^Expira em/)).toBeVisible();

  await page.waitForTimeout(1_100); // past the test backend's 1s email cooldown
  await page.getByRole("button", { name: "Ativação" }).click();
  await page.getByRole("menuitem", { name: "Gerar link para copiar" }).click();
  await page.getByRole("button", { name: "Gerar link" }).click();
  const field = page.getByRole("dialog").getByRole("textbox", { name: "Link de ativação" });
  await expect(field).toHaveValue(/\/activate#token=/);
  const second = await field.inputValue();
  expect(second).not.toBe(first);

  const old = await guestPage(browser);
  await old.goto(first);
  await choosePassword(old, strongPassword);
  await old.getByRole("button", { name: "Ativar conta" }).click();
  await expect(old.getByRole("alert").filter({ hasText: /\S/ })).toBeVisible(); // the form error, not the empty route announcer
  await old.close();

  const guest = await guestPage(browser);
  await guest.goto(second);
  await choosePassword(guest, strongPassword);
  await guest.getByRole("button", { name: "Ativar conta" }).click();
  await expect(guest.getByText("Conta ativada")).toBeVisible();
  await guest.close();
});
