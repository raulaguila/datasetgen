import { test as setup } from "@playwright/test";
import { admin, adminState, login } from "./helpers";

// One admin sign-in for the whole run: the backend allows few auth calls per minute and IP.
setup("admin session", async ({ page }) => {
  await login(page, admin.email, admin.password);
  await page.context().storageState({ path: adminState });
});
