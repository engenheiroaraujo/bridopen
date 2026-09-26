// @ts-check
const { defineConfig, devices } = require("@playwright/test");

// O servidor de pré-visualização (cmd/e2e-preview) renderiza as telas de
// autenticação com os templates e assets reais, sem banco de dados.
const PORT = 5511;
const BASE_URL = `http://127.0.0.1:${PORT}`;

module.exports = defineConfig({
  testDir: "./tests/e2e",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL: BASE_URL,
    locale: "pt-BR",
    trace: "retain-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: {
    command: `go run ./cmd/e2e-preview -addr 127.0.0.1:${PORT}`,
    url: `${BASE_URL}/user/signin`,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
});
