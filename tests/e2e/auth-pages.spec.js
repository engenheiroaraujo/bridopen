// @ts-check
const { test, expect } = require("@playwright/test");
const { openAuthPage } = require("./helpers");

// Contrato comum a toda tela de autenticação: carrega sem erro de JavaScript,
// tem o título esperado e o formulário aponta para a rota real do backend.
const pages = [
  { path: "/user/signin", title: "Iniciar sessão — Bridopen", action: "/user/signin" },
  { path: "/user/signup", title: "Criar conta — Bridopen", action: "/user/signup" },
  { path: "/user/forgetpassword", title: "Redefinir senha — Bridopen", action: "/user/forgetpassword" },
  { path: "/user/resend", title: "Reenviar confirmação — Bridopen", action: "/user/resend" },
  { path: "/user/password/token-e2e", title: "Nova senha — Bridopen", action: "/user/password/token-e2e" },
  { path: "/user/twofactor", title: "Verificação em duas etapas — Bridopen", action: "/user/twofactor" },
];

for (const { path, title, action } of pages) {
  test(`${path} carrega sem erros de JavaScript`, async ({ page }) => {
    const errors = await openAuthPage(page, path);

    await expect(page).toHaveTitle(title);
    await expect(page.locator(`form[action="${action}"][method="post"]`)).toBeVisible();
    await expect(page.locator('form input[name="gorilla.csrf.Token"]')).toHaveCount(1);
    expect(errors).toEqual([]);
  });
}

test("erros de campo aparecem no alerta da tela", async ({ page }) => {
  await openAuthPage(page, "/user/signin?erro=E-mail%20ou%20senha%20inv%C3%A1lidos.");

  const alert = page.getByRole("alert");
  await expect(alert).toBeVisible();
  await expect(alert).toContainText("E-mail ou senha inválidos.");
});

test("alternar tema troca a classe dark e o rótulo do botão", async ({ page }) => {
  await openAuthPage(page, "/user/signin");

  const html = page.locator("html");
  const toggle = page.locator("[data-theme-toggle]");
  const wasDark = await html.evaluate((el) => el.classList.contains("dark"));

  await toggle.click();
  await expect(html).toHaveClass(wasDark ? /^(?!.*\bdark\b)/ : /\bdark\b/);
  await expect(toggle).toHaveAttribute("aria-pressed", wasDark ? "false" : "true");
  await expect(toggle).toContainText(wasDark ? "Tema escuro" : "Tema claro");

  await toggle.click();
  await expect(toggle).toHaveAttribute("aria-pressed", wasDark ? "true" : "false");
  expect(await page.evaluate(() => localStorage.getItem("qn-theme"))).toBe(wasDark ? "dark" : "light");
});
