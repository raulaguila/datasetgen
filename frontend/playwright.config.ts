import { defineConfig, devices } from "@playwright/test";

// End-to-end runs against a real backend: see e2e/README.md for what it needs.
const port = Number(process.env.E2E_PORT ?? 3101);

export default defineConfig({
  testDir: "e2e",
  fullyParallel: false, // the flows share one backend and its rate limits
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? "github" : "list",
  use: {
    baseURL: `http://localhost:${port}`,
    locale: "pt-BR",
    trace: "retain-on-failure",
  },
  projects: [
    { name: "setup", testMatch: /\.setup\.ts$/ },
    { name: "chromium", dependencies: ["setup"], use: { ...devices["Desktop Chrome"], storageState: "e2e/.auth/admin.json" } },
  ],
  webServer: {
    command: `pnpm exec next dev -p ${port}`,
    port,
    reuseExistingServer: !process.env.CI,
    env: { API_URL: process.env.E2E_API_URL ?? "http://127.0.0.1:9990" },
    timeout: 120_000,
  },
});
