// @ts-check
const { test, expect } = require("@playwright/test");
const { openAuthPage, watchConsole } = require("./helpers");

const legal = [
  { path: "/privacy", title: "Privacidade — Bridopen", heading: "Política de privacidade" },
  { path: "/terms", title: "Termos de Serviço — Bridopen", heading: "Termos de Serviço do Bridopen" },
  { path: "/cookies", title: "Cookies — Bridopen", heading: "Política de cookies" },
];

for (const { path, title, heading } of legal) {
  test(`${path} renderiza o texto legal com link de volta e sem shell do app`, async ({ page }) => {
    const errors = await openAuthPage(page, path);

    await expect(page).toHaveTitle(title);
    await expect(page.getByRole("heading", { level: 1, name: heading })).toBeVisible();
    await expect(page.getByRole("link", { name: "Voltar" })).toHaveAttribute("href", "/");
    await expect(page.locator("#app-sidebar")).toHaveCount(0);
    await expect(page.locator("footer.site-footer").getByRole("link", { name: "Privacidade" })).toHaveAttribute("href", "/privacy");
    expect(errors).toEqual([]);
  });
}

test.describe("páginas de erro", () => {
  test("404 devolve o status certo e leva para o início", async ({ page }) => {
    const errors = watchConsole(page);
    const response = await page.goto("/erro/404");
    expect(response?.status()).toBe(404);

    await expect(page).toHaveTitle("Página não encontrada — Bridopen");
    await expect(page.getByRole("heading", { level: 1, name: "Página não encontrada" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Ir para o início" })).toHaveAttribute("href", "/");
    expect(errors).toEqual([]);
  });

  test("erro genérico devolve 500 e leva para o início", async ({ page }) => {
    const response = await page.goto("/erro/generico");
    expect(response?.status()).toBe(500);
    await expect(page.getByRole("heading", { level: 1, name: "Algo deu errado" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Ir para o início" })).toHaveAttribute("href", "/");
  });

  test("sessão expirada devolve 403 e o botão volta para a origem do formulário", async ({ page }) => {
    let response = await page.goto("/erro/csrf?origem=signup");
    expect(response?.status()).toBe(403);
    await expect(page.getByRole("heading", { level: 1, name: "Sessão expirada" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Voltar para o cadastro" })).toHaveAttribute("href", "/user/signup");

    response = await page.goto("/erro/csrf?origem=forgetpassword");
    expect(response?.status()).toBe(403);
    await expect(page.getByRole("link", { name: "Redefinir senha novamente" })).toHaveAttribute("href", "/user/forgetpassword");
  });
});

test.describe("confirmação de e-mail de recuperação", () => {
  test("link válido confirma o e-mail e leva para o login", async ({ page }) => {
    const errors = await openAuthPage(page, "/recovery-email/confirmation/valido");

    await expect(page).toHaveTitle("E-mail confirmado — Bridopen");
    await expect(page.getByRole("heading", { level: 1, name: "E-mail de recuperação confirmado" })).toBeVisible();
    await expect(page.getByText("recuperacao@exemplo.com")).toBeVisible();
    await expect(page.getByRole("link", { name: "Ir para o login" })).toHaveAttribute("href", "/user/signin");
    expect(errors).toEqual([]);
  });

  test("link inválido explica o problema", async ({ page }) => {
    await openAuthPage(page, "/recovery-email/confirmation/expirado");

    await expect(page).toHaveTitle("Link inválido — Bridopen");
    await expect(page.getByRole("heading", { level: 1, name: "Link inválido ou expirado" })).toBeVisible();
    await expect(page.getByText("Este link de confirmação é inválido ou já expirou.")).toBeVisible();
    await expect(page.getByRole("link", { name: "Ir para o login" })).toHaveAttribute("href", "/user/signin");
  });
});
