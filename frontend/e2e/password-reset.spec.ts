import { expect, test } from "@playwright/test";
import { apiUrl, choosePassword, invite, login, strongPassword, uniqueEmail , guestPage } from "./helpers";

test("a person resets a forgotten password", async ({ page, browser, request }) => {
  const email = uniqueEmail("reset");
  const link = await invite(page, "Pessoa Esquecida", email);
  const guest = await guestPage(browser);
  await guest.goto(link);
  await choosePassword(guest, strongPassword);
  await guest.getByRole("button", { name: "Ativar conta" }).click();
  await expect(guest.getByText("Conta ativada")).toBeVisible();

  // The screen never shows the code; the development backend returns it to a direct call instead.
  const res = await request.post(`${apiUrl}/v1/auth/password-reset/request`, { data: { email } });
  expect(res.ok()).toBeTruthy();
  const { reset_token } = (await res.json()) as { reset_token?: string };
  expect(reset_token, "the backend needs AUTH_MANUAL_TOKEN_DELIVERY=1").toBeTruthy();

  const fresh = "Brand new orchard 77";
  await guest.goto(`/reset-password#token=${encodeURIComponent(reset_token!)}`);
  await choosePassword(guest, fresh);
  await guest.getByRole("button", { name: "Salvar nova senha" }).click();
  await expect(guest.getByText("Senha alterada")).toBeVisible();

  await login(guest, email, fresh);
  await guest.close();
});
