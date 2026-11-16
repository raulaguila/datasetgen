import { createHmac } from "node:crypto";
import { expect, test } from "@playwright/test";
import { choosePassword, guestPage, invite, login, strongPassword, uniqueEmail } from "./helpers";

// RFC 6238 code for the current 30-second step.
function totp(secret: string): string {
  const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
  let bits = "";
  for (const c of secret.replace(/\s/g, "").toUpperCase()) bits += alphabet.indexOf(c).toString(2).padStart(5, "0");
  const key = Buffer.from(bits.match(/.{8}/g)!.map((b) => parseInt(b, 2)));
  const counter = Buffer.alloc(8);
  counter.writeBigUInt64BE(BigInt(Math.floor(Date.now() / 30_000)));
  const h = createHmac("sha1", key).update(counter).digest();
  const o = h[h.length - 1] & 0xf;
  return String((h.readUInt32BE(o) & 0x7fffffff) % 1_000_000).padStart(6, "0");
}

test("two-step login asks for a code after the password", async ({ page, browser }) => {
  const email = uniqueEmail("mfa");
  const link = await invite(page, "Pessoa Cuidadosa", email);
  const user = await guestPage(browser);
  await user.goto(link);
  await choosePassword(user, strongPassword);
  await user.getByRole("button", { name: "Ativar conta" }).click();
  await expect(user.getByText("Conta ativada")).toBeVisible();
  await login(user, email, strongPassword);

  await user.goto("/profile");
  await user.getByRole("button", { name: "Ativar login em duas etapas" }).click();
  await expect(user.getByRole("img", { name: "QR code para o app autenticador" })).toBeVisible();
  const secret = await user.locator("code").first().innerText();
  await user.getByLabel("Código de verificação").fill(totp(secret));
  await user.getByRole("button", { name: "Confirmar e ativar" }).click();
  await expect(user.getByText("Códigos de recuperação", { exact: true })).toBeVisible();
  const recovery = await user.locator("ul.font-mono li").first().innerText();
  await user.close();

  const again = await guestPage(browser);
  await again.goto("/login");
  await again.getByLabel("E-mail").fill(email);
  await again.getByLabel("Senha", { exact: true }).fill(strongPassword);
  await again.getByRole("button", { name: "Entrar" }).click();
  await again.getByLabel("Código de verificação").fill(recovery);
  await again.getByRole("button", { name: "Entrar" }).click();
  await expect(again.getByRole("button", { name: "Menu da conta" })).toBeVisible();
  await again.close();
});
