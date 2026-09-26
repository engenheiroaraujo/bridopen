// @ts-check
const { test, expect } = require("@playwright/test");
const { openAuthPage, solveCaptcha } = require("./helpers");

// Telas de um único campo cujo envio depende só do captcha.
const captchaPages = [
  { path: "/user/forgetpassword", button: "Enviar instruções" },
  { path: "/user/resend", button: "Reenviar link" },
];

for (const { path, button } of captchaPages) {
  test(`${path}: captcha trava o envio até ser respondido`, async ({ page }) => {
    const errors = await openAuthPage(page, path);

    const submit = page.getByRole("button", { name: button });
    await expect(page.locator("#email")).toBeVisible();
    await expect(page.locator("[data-captcha-panel]")).toBeHidden();
    await expect(submit).toBeDisabled();

    await page.getByLabel("Não sou um robô").check();
    await expect(page.locator("[data-captcha-panel]")).toBeVisible();
    await expect(submit).toBeDisabled();

    await page.locator("#captcha_answer").fill("1234");
    await expect(submit).toBeEnabled();
    expect(errors).toEqual([]);
  });

  test(`${path}: e-mail preenchido pelo servidor é preservado`, async ({ page }) => {
    await openAuthPage(page, `${path}?email=ana%40exemplo.com`);
    await expect(page.locator("#email")).toHaveValue("ana@exemplo.com");
  });
}

test.describe("verificação em duas etapas", () => {
  test("sem captcha o botão Validar fica liberado e o método aparece no texto", async ({ page }) => {
    const errors = await openAuthPage(page, "/user/twofactor");

    await expect(page.locator("[data-auth-captcha]")).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Validar código" })).toBeEnabled();
    await expect(page.locator("#code")).toHaveAttribute("aria-invalid", "false");
    await expect(page.locator("#code")).toHaveAttribute("autocomplete", "one-time-code");
    await expect(page.locator("main")).toContainText("Informe o ");
    expect(errors).toEqual([]);
  });

  test("com captcha adaptativo o envio só libera depois de respondido", async ({ page }) => {
    await openAuthPage(page, "/user/twofactor?captcha=1");

    const submit = page.getByRole("button", { name: "Validar código" });
    await expect(submit).toBeDisabled();

    await solveCaptcha(page);
    await expect(submit).toBeEnabled();
  });

  test("erro no código marca o campo como inválido e mostra o alerta", async ({ page }) => {
    await openAuthPage(page, "/user/twofactor?erro=code:C%C3%B3digo%20inv%C3%A1lido.");

    await expect(page.locator("#code")).toHaveAttribute("aria-invalid", "true");
    await expect(page.getByRole("alert")).toContainText("Código inválido.");
  });
});
