// @ts-check
const { test, expect } = require("@playwright/test");
const { openAuthPage, fillPassword, toggleState } = require("./helpers");

const VALID = "NovaSenhaForte123";

test.describe("redefinição de senha", () => {
  test("o token do link vai para a action e para o campo oculto", async ({ page }) => {
    const errors = await openAuthPage(page, "/user/password/token-e2e");

    await expect(page.locator('form[action="/user/password/token-e2e"]')).toBeVisible();
    await expect(page.locator('input[name="token"]')).toHaveValue("token-e2e");
    expect(errors).toEqual([]);
  });

  test("botão Gravar nasce travado e libera com senhas iguais de 12+ caracteres", async ({ page }) => {
    await openAuthPage(page, "/user/password/token-e2e");

    const submit = page.locator("#reset-password-submit");
    const password = page.locator("#password");
    const confirm = page.locator("#password_confirm");

    await expect(submit).toBeDisabled();
    await expect(page.locator("[data-auth-captcha]")).toHaveCount(0);

    await fillPassword(password, VALID);
    await expect(submit).toBeDisabled();

    await fillPassword(confirm, VALID);
    await expect(submit).toBeEnabled();

    await fillPassword(confirm, "outra-coisa-12345");
    await expect(submit).toBeDisabled();
    expect(await confirm.evaluate((el) => el.validationMessage)).toBe("As senhas não conferem.");
  });

  test("olhinhos revelam a nova senha e a confirmação", async ({ page }) => {
    await openAuthPage(page, "/user/password/token-e2e");

    const password = page.locator("#password");
    const confirm = page.locator("#password_confirm");
    const toggle = page.locator("#reset-password-toggle");
    const toggleConfirm = page.locator("#reset-password-confirm-toggle");

    await fillPassword(password, VALID);
    await toggle.click();
    expect(await toggleState(toggle, password)).toMatchObject({ type: "text", pressed: "true", label: "Ocultar senha" });
    await expect(password).toHaveValue(VALID);

    await toggleConfirm.click();
    expect(await toggleState(toggleConfirm, confirm)).toMatchObject({ type: "text", label: "Ocultar confirmação de senha" });
  });
});
