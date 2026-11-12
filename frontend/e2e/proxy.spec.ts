import { expect, test } from "@playwright/test";
import { admin } from "./helpers";

// The proxy keeps token routes out of reach so tokens never land in the browser.
// "%252e" reaches the route as "%2e", which URL parsing turns into "..".
test("the proxy refuses token routes reached through dot segments", async ({ page }) => {
  for (const path of ["v1/auth/login", "v1/%252e%252e/v1/auth/login", "v1/auth/%252e/login"]) {
    const res = await page.request.post(`/api/proxy/${path}`, { data: { email: admin.email, password: admin.password } });
    expect(res.status(), path).toBe(404);
    expect(await res.text(), path).not.toContain("access_token");
  }
});
