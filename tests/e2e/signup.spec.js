// @ts-check
const { test, expect } = require("@playwright/test");
const { openAuthPage, fillPassword, toggleState, solveCaptcha } = require("./helpers");

const VALID = "SenhaSegura12345";

test.describe("cadastro", () => {
  test("botão Cadastrar nasce travado e só libera com senhas iguais de 12+ caracteres e captcha", async ({ page }) => {
    const errors = await openAuthPage(page, "/user/signup");

    const submit = page.locator("#reset-password-submit");
    const password = page.locator("#password");
    const confirm = page.locator("#password_confirm");
    await expect(submit).toBeDisabled();

    await fillPassword(password, VALID);
    await fillPassword(confirm, VALID);
    await expect(submit).toBeDisabled(); // captcha ainda pendente

    await solveCaptcha(page);
    await expect(submit).toBeEnabled();
    await expect(submit).toHaveAttribute("aria-disabled", "false");

    // Senha curta demais trava de novo, mesmo com confirmação igual.
    await fillPassword(password, "curta123");
    await fillPassword(confirm, "curta123");
    await expect(submit).toBeDisabled();

    expect(errors).toEqual([]);
  });

  test("senhas diferentes travam o envio e explicam o motivo", async ({ page }) => {
    await openAuthPage(page, "/user/signup");

    const submit = page.locator("#reset-password-submit");
    const password = page.locator("#password");
    const confirm = page.locator("#password_confirm");

    await solveCaptcha(page);
    await fillPassword(password, VALID);
    await fillPassword(confirm, VALID + "x");

    await expect(submit).toBeDisabled();
    expect(await confirm.evaluate((el) => el.validationMessage)).toBe("As senhas não conferem.");

    await fillPassword(confirm, VALID);
    await expect(submit).toBeEnabled();
    expect(await confirm.evaluate((el) => el.validationMessage)).toBe("");
  });

  test("os dois olhinhos funcionam de forma independente", async ({ page }) => {
    await openAuthPage(page, "/user/signup");

    const password = page.locator("#password");
    const confirm = page.locator("#password_confirm");
    const toggle = page.locator("#signup-password-toggle");
    const toggleConfirm = page.locator("#signup-password-confirm-toggle");

    await toggle.click();
    expect(await toggleState(toggle, password)).toMatchObject({ type: "text", label: "Ocultar senha" });
    expect(await toggleState(toggleConfirm, confirm)).toMatchObject({ type: "password", label: "Mostrar confirmação de senha" });

    await toggleConfirm.click();
    expect(await toggleState(toggleConfirm, confirm)).toMatchObject({
      type: "text",
      pressed: "true",
      label: "Ocultar confirmação de senha",
      eyeHidden: true,
      eyeOffHidden: false,
    });

    await toggleConfirm.click();
    expect(await toggleState(toggleConfirm, confirm)).toMatchObject({ type: "password", label: "Mostrar confirmação de senha" });
    expect(await toggleState(toggle, password)).toMatchObject({ type: "text" });
  });

  test("colar é bloqueado só no campo de confirmação", async ({ page }) => {
    await openAuthPage(page, "/user/signup");

    const pasteBlocked = (el) => {
      const paste = new ClipboardEvent("paste", { bubbles: true, cancelable: true });
      el.dispatchEvent(paste);
      const beforeInput = new InputEvent("beforeinput", { bubbles: true, cancelable: true, inputType: "insertFromPaste" });
      el.dispatchEvent(beforeInput);
      const drop = new DragEvent("drop", { bubbles: true, cancelable: true });
      el.dispatchEvent(drop);
      return { paste: paste.defaultPrevented, beforeInput: beforeInput.defaultPrevented, drop: drop.defaultPrevented };
    };

    expect(await page.locator("#password_confirm").evaluate(pasteBlocked)).toEqual({ paste: true, beforeInput: true, drop: true });
    expect(await page.locator("#password").evaluate(pasteBlocked)).toEqual({ paste: false, beforeInput: false, drop: false });
    expect(await page.locator("#email").evaluate(pasteBlocked)).toEqual({ paste: false, beforeInput: false, drop: false });
  });
});
