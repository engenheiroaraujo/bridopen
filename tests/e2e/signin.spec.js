// @ts-check
const { test, expect } = require("@playwright/test");
const { openAuthPage, fillPassword, toggleState, solveCaptcha } = require("./helpers");

test.describe("login", () => {
  test("olhinho mostra e volta a ocultar a senha", async ({ page }) => {
    const errors = await openAuthPage(page, "/user/signin");

    const password = page.locator("#password");
    const toggle = page.locator("#signin-password-toggle");
    await fillPassword(password, "segredo-e2e");

    expect(await toggleState(toggle, password)).toEqual({
      type: "password",
      pressed: "false",
      label: "Mostrar senha",
      eyeHidden: false,
      eyeOffHidden: true,
    });

    await toggle.click();
    expect(await toggleState(toggle, password)).toEqual({
      type: "text",
      pressed: "true",
      label: "Ocultar senha",
      eyeHidden: true,
      eyeOffHidden: false,
    });
    await expect(password).toHaveValue("segredo-e2e");

    await toggle.click();
    expect(await toggleState(toggle, password)).toMatchObject({ type: "password", pressed: "false" });
    expect(errors).toEqual([]);
  });

  test("sem captcha o botão Entrar fica liberado", async ({ page }) => {
    await openAuthPage(page, "/user/signin");

    await expect(page.locator("[data-auth-captcha]")).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Entrar" })).toBeEnabled();
  });

  test("captcha abre o painel e só libera Entrar depois de respondido", async ({ page }) => {
    await openAuthPage(page, "/user/signin?captcha=1");

    const submit = page.getByRole("button", { name: "Entrar" });
    const check = page.getByLabel("Não sou um robô");
    const panel = page.locator("[data-captcha-panel]");
    const answer = page.locator("#captcha_answer");

    await expect(panel).toBeHidden();
    await expect(answer).toBeDisabled();
    await expect(submit).toBeDisabled();
    await expect(submit).toHaveAttribute("aria-disabled", "true");

    await check.check();
    await expect(panel).toBeVisible();
    await expect(answer).toBeEnabled();
    await expect(submit).toBeDisabled();

    await answer.fill("1234");
    await expect(submit).toBeEnabled();
    await expect(submit).toHaveAttribute("aria-disabled", "false");

    await check.uncheck();
    await expect(panel).toBeHidden();
    await expect(answer).toBeDisabled();
    await expect(answer).toHaveValue("");
    await expect(submit).toBeDisabled();
  });

  test("captcha já aberto pelo servidor vem visível e habilitado", async ({ page }) => {
    await openAuthPage(page, "/user/signin?captcha=1&captcha_aberto=1");

    await expect(page.getByLabel("Não sou um robô")).toBeChecked();
    await expect(page.locator("[data-captcha-panel]")).toBeVisible();
    await expect(page.locator("#captcha_answer")).toBeEnabled();

    await solveCaptcha(page);
    await expect(page.getByRole("button", { name: "Entrar" })).toBeEnabled();
  });

  test("erro de captcha aparece junto do captcha, não no alerta geral", async ({ page }) => {
    await openAuthPage(page, "/user/signin?captcha=1&erro=captcha:C%C3%B3digo%20incorreto.");

    await expect(page.getByRole("alert")).toHaveCount(0);
    await expect(page.locator("[data-auth-captcha]")).toContainText("Código incorreto.");
  });
});
