// @ts-check
const { test, expect } = require("@playwright/test");
const { openAppPage, shellKept } = require("./helpers");

test.describe("segurança da conta", () => {
  test("mostra o status da verificação em duas etapas e os dois métodos disponíveis", async ({ page }) => {
    const errors = await openAppPage(page, "/me/security/two-step-verification");

    await expect(page.getByRole("heading", { level: 1, name: "Segurança" })).toBeVisible();
    const section = page.locator("#two-step-verification");
    await expect(section).toContainText("Status atual: Desabilitado");
    const email = section.getByRole("link", { name: /Código por e-mail/ });
    const totp = section.getByRole("link", { name: /Aplicativo autenticador/ });
    await expect(email).toHaveAttribute("href", "/me/security/two-factor/email");
    await expect(totp).toHaveAttribute("href", "/me/security/two-factor/totp");
    await expect(email).toContainText("Disponível");
    await expect(totp).toContainText("Disponível");
    expect(errors).toEqual([]);
  });

  test("com um método ativo, os cartões indicam qual está ativo e qual está inativo", async ({ page }) => {
    await openAppPage(page, "/me/security/two-step-verification?2fa=enabled&method=totp");

    const section = page.locator("#two-step-verification");
    await expect(section).toContainText("Status atual: Habilitado por aplicativo autenticador");
    await expect(section.getByRole("link", { name: /Aplicativo autenticador/ })).toContainText("Ativo");
    await expect(section.getByRole("link", { name: /Código por e-mail/ })).toContainText("Inativo");
  });

  test("abrir um método navega parcialmente para a tela de configuração", async ({ page }) => {
    await openAppPage(page, "/me/security/two-step-verification");

    await page.locator("#two-step-verification").getByRole("link", { name: /Aplicativo autenticador/ }).click();
    await expect(page).toHaveURL(/\/me\/security\/two-factor\/totp$/);
    await expect(page.getByRole("heading", { level: 1, name: "Aplicativo autenticador" })).toBeVisible();
    expect(await shellKept(page)).toBe(true);
  });

  test("histórico lista as sessões, marca a atual e oferece encerrar as outras", async ({ page }) => {
    await openAppPage(page, "/me/security/active-sessions");

    const section = page.locator("#active-sessions");
    await expect(section.getByRole("button", { name: "Fazer logout" })).toHaveCount(3);
    await expect(section.getByText("Sessão atual")).toHaveCount(1);
    await expect(section).toContainText("Chrome 140 em Windows");
    await expect(section).toContainText("Endereço IP: 192.168.0.10");
    await expect(section.locator('form[action="/me/security/sessions/sess-celular/logout"]')).toHaveCount(1);
    await expect(section.getByRole("button", { name: "Encerrar outras" })).toBeVisible();
    await expect(section.locator('form[action="/me/security/sessions/revoke-other"]')).toHaveCount(1);
    await expect(page.locator("#active-session-limit")).toHaveValue("10");
  });

  test("com uma única sessão não há o que encerrar", async ({ page }) => {
    await openAppPage(page, "/me/security/active-sessions?sessoes=1");
    await expect(page.locator("#active-sessions").getByRole("button", { name: "Fazer logout" })).toHaveCount(1);
    await expect(page.getByRole("button", { name: "Encerrar outras" })).toHaveCount(0);

    await openAppPage(page, "/me/security/active-sessions?sessoes=0");
    await expect(page.getByText("Nenhuma sessão ativa encontrada.")).toBeVisible();
  });

  test("trocar a quantidade de sessões reenvia o filtro e mantém a opção escolhida", async ({ page }) => {
    await openAppPage(page, "/me/security/active-sessions");

    // O select usa form.submit() no onchange, que não dispara o evento submit: é navegação completa, não parcial.
    await page.locator("#active-session-limit").selectOption("100");
    await expect(page).toHaveURL(/\/me\/security\/active-sessions\?sessions=100$/);
    await expect(page.locator("#active-session-limit")).toHaveValue("100");
    await expect(page.locator("#active-sessions").getByRole("button", { name: "Fazer logout" })).toHaveCount(3);
  });

  test("encerrar uma sessão envia o formulário e volta com a lista atualizada", async ({ page }) => {
    await openAppPage(page, "/me/security/active-sessions");

    await page.locator('form[action="/me/security/sessions/sess-celular/logout"] button').click();
    await expect(page).toHaveURL(/sessoes=2/);
    await expect(page.locator("#active-sessions").getByRole("button", { name: "Fazer logout" })).toHaveCount(2);
  });

  test("a seção alvo da rota recebe o destaque", async ({ page }) => {
    await openAppPage(page, "/me/security/active-sessions");
    const section = page.locator("#active-sessions");
    await expect(section).toHaveClass(/qn-me-section--spotlight/, { timeout: 2000 });
    await expect(section).not.toHaveClass(/qn-me-section--spotlight/, { timeout: 3000 });
  });
});
