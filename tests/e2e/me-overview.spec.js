// @ts-check
const { test, expect } = require("@playwright/test");
const { openAppPage, toast, toggleState, pasteBlocked } = require("./helpers");

test.describe("minha conta: visão geral", () => {
  test("/me redireciona para informações pessoais e mostra os dados da conta", async ({ page }) => {
    const errors = await openAppPage(page, "/me");

    await expect(page).toHaveURL(/\/me\/overview\/personal-information$/);
    await expect(page.getByRole("heading", { level: 1, name: "Visão geral" })).toBeVisible();
    await expect(page.locator("#personal-information")).toContainText("Ada Lovelace");
    await expect(page.locator("#personal-information")).toContainText("(11) 98765-4321");
    await expect(page.locator("#account-settings")).toContainText("ada@exemplo.com");
    await expect(page.locator("#account-settings")).toContainText("Não informado");
    await expect(page.locator("#account-settings")).toContainText("01/09/2026");
    await expect(page.locator("#privacy-data").getByRole("link", { name: "Exportar meus dados" })).toHaveAttribute("href", "/account/export");
    await expect(page.locator("#privacy-data").getByRole("link", { name: "Política de cookies" })).toHaveAttribute("href", "/cookies");
    expect(errors).toEqual([]);
  });

  test("conta sem dados pessoais avisa que faltam informações", async ({ page }) => {
    await openAppPage(page, "/me/overview/personal-information?sem-dados=1");
    await expect(page.getByText("Os dados pessoais completos ainda não foram registrados nesta conta.")).toBeVisible();
    await expect(page.locator("#personal-information dd").first()).toHaveText("Não informado");
  });

  test("menu de informações pessoais abre o modal de edição com filtros de nome e telefone", async ({ page }) => {
    await openAppPage(page, "/me/overview/personal-information");

    const trigger = page.locator("#btnOpenPersonalInfoMenu");
    const menu = page.locator("#personalInfoMenu");
    await trigger.click();
    await expect(menu).toBeVisible();
    await expect(trigger).toHaveAttribute("aria-expanded", "true");

    await page.locator("#btnOpenEditPersonalInfoModal").click();
    const modal = page.locator("#editPersonalInfoModal");
    await expect(modal).toBeVisible();
    await expect(menu).toBeHidden();
    await expect(page.locator("#first_name")).toBeFocused();
    await expect(page.locator("#first_name")).toHaveValue("Ada");
    await expect(page.locator("#last_name")).toHaveValue("Lovelace");
    await expect(page.locator("#personal_info_id")).toHaveValue("7");

    await page.locator("#first_name").fill("");
    await page.keyboard.type("Ana1 Mar-ia'2");
    await expect(page.locator("#first_name")).toHaveValue("Ana Mar-ia'");

    await page.locator("#phone_number").fill("");
    await page.keyboard.type("(11) 9abc8+7-6");
    await expect(page.locator("#phone_number")).toHaveValue("(11) 98+7-6");

    await page.locator("#btnCancelEditPersonalInfo").click();
    await expect(modal).toBeHidden();
    await expect(trigger).toBeFocused();
    await expect(page.locator("body")).not.toHaveClass(/overflow-hidden/);
  });

  test("Escape fecha primeiro o menu e depois o modal", async ({ page }) => {
    await openAppPage(page, "/me/overview/personal-information");

    await page.locator("#btnOpenPersonalInfoMenu").click();
    await page.keyboard.press("Escape");
    await expect(page.locator("#personalInfoMenu")).toBeHidden();

    await page.locator("#btnOpenPersonalInfoMenu").click();
    await page.locator("#btnOpenEditPersonalInfoModal").click();
    await expect(page.locator("#editPersonalInfoModal")).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(page.locator("#editPersonalInfoModal")).toBeHidden();
  });

  test("modal de senha abre pelo menu de configurações, com olhinhos e bloqueio de colar", async ({ page }) => {
    await openAppPage(page, "/me/overview/account-settings");

    await page.getByRole("button", { name: "Ações das configurações da conta" }).click();
    await expect(page.locator("#accountSettingsMenu")).toBeVisible();
    await page.locator("[data-password-change-open]").click();

    const modal = page.locator("#password-change-modal");
    await expect(modal).toBeVisible();
    await expect(page.locator("#accountSettingsMenu")).toBeHidden();
    await expect(page.locator("#account_current_secret")).toBeFocused();
    await expect(page.locator("#passwordChangeForm")).toHaveAttribute("action", "/me/password");

    const newSecret = page.locator("#account_new_secret");
    const toggle = page.locator('button[data-password-target="account_new_secret"]');
    await newSecret.click();
    await newSecret.fill("NovaSenha123456");
    await toggle.click();
    expect(await toggleState(toggle, newSecret)).toMatchObject({ type: "text", pressed: "true", label: "Ocultar senha" });

    expect(await page.locator("#account_new_secret_confirm").evaluate(pasteBlocked)).toEqual({ paste: true, beforeInput: true, drop: true });
    expect(await newSecret.evaluate(pasteBlocked)).toEqual({ paste: false, beforeInput: false, drop: false });

    await modal.getByRole("button", { name: "Cancelar" }).click();
    await expect(modal).toBeHidden();
  });

  test("modal de e-mail de recuperação foca o e-mail e fecha com Escape", async ({ page }) => {
    await openAppPage(page, "/me/overview/account-settings");

    await page.getByRole("button", { name: "Ações das configurações da conta" }).click();
    await page.locator("[data-recovery-email-open]").click();
    const modal = page.locator("#recovery-email-modal");
    await expect(modal).toBeVisible();
    await expect(page.locator("#recovery_email")).toBeFocused();
    await expect(page.locator("#recoveryEmailForm")).toHaveAttribute("action", "/me/recovery-email");

    await page.keyboard.press("Escape");
    await expect(modal).toBeHidden();
  });

  test("modal de exclusão foca Cancelar, bloqueia colar e envia o formulário ao confirmar", async ({ page }) => {
    await openAppPage(page, "/me/overview/privacy-data");

    await page.locator("[data-account-delete-open]").click();
    const modal = page.locator("#account-delete-modal");
    await expect(modal).toBeVisible();
    await expect(page.locator("#account-delete-cancel")).toBeFocused();
    expect(await page.locator("#account_delete_confirmation").evaluate(pasteBlocked)).toMatchObject({ paste: true });

    await page.keyboard.press("Escape");
    await expect(modal).toBeHidden();

    await page.locator("[data-account-delete-open]").click();
    await page.locator("#account_delete_confirmation").fill("excluir conta");
    await page.locator("#account_delete_password").fill("segredo123");
    const submit = page.waitForResponse((res) => res.request().method() === "POST" && res.url().endsWith("/account/delete"));
    await page.locator("#account-delete-confirm").click();
    const response = await submit;
    expect(response.request().postData()).toContain("account_delete_confirmation=excluir+conta");
    expect(response.status()).toBe(303);
    await expect(page).toHaveURL(/\/user\/signin$/);
  });

  test("modal reaberto pelo servidor mostra os erros de campo", async ({ page }) => {
    await openAppPage(page, "/me/overview/personal-information?modal=edit&erro=first_name:Nome%20inv%C3%A1lido.");

    const modal = page.locator("#editPersonalInfoModal");
    await expect(modal).toBeVisible();
    await expect(modal.getByRole("alert")).toContainText("Nome inválido.");
    await expect(page.locator("#first_name")).toHaveAttribute("aria-invalid", "true");
    await expect(page.locator("#last_name")).toHaveAttribute("aria-invalid", "false");
    await expect(page.locator("body")).toHaveClass(/overflow-hidden/);

    await openAppPage(page, "/me/overview/account-settings?modal=password&erro=current_password:Senha%20atual%20incorreta.");
    await expect(page.locator("#password-change-modal")).toBeVisible();
    await expect(page.locator("#password-change-modal").getByRole("alert")).toContainText("Senha atual incorreta.");
  });

  test("seção alvo da rota recebe o destaque e ele desaparece sozinho", async ({ page }) => {
    await openAppPage(page, "/me/overview/account-settings");
    const section = page.locator("#account-settings");
    await expect(section).toHaveClass(/qn-me-section--spotlight/, { timeout: 2000 });
    await expect(section).not.toHaveClass(/qn-me-section--spotlight/, { timeout: 3000 });
  });

  test("eventos do servidor viram toast", async ({ page }) => {
    await openAppPage(page, "/me/overview/account-settings?toast=senha");
    await expect(toast(page)).toContainText("Senha alterada com sucesso.");

    await openAppPage(page, "/me/overview/account-settings?toast=recovery");
    await expect(toast(page)).toContainText("Enviamos a confirmação para novo@exemplo.com.");
    await expect(page.locator("#account-settings")).toContainText("Aguardando confirmação: novo@exemplo.com");
  });
});
