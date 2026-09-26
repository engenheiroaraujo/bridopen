// @ts-check
const { test, expect } = require("@playwright/test");
const { openAppPage, toast, fulfillJSON } = require("./helpers");

const enabledStatus = {
  active: true,
  enabled: true,
  method: "totp",
  status_label: "Habilitado por aplicativo autenticador",
  action_label: "Desabilitar",
};
const disabledStatus = { active: true, enabled: false, method: "", status_label: "Desabilitado", action_label: "Habilitar" };
const QR = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==";

test.describe("verificação em duas etapas: aplicativo autenticador", () => {
  test("tela nasce desabilitada, com Habilitar, sem código nem QR e sem gerar códigos", async ({ page }) => {
    const errors = await openAppPage(page, "/me/security/two-factor/totp");

    await expect(page.getByRole("heading", { level: 1, name: "Aplicativo autenticador" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Voltar para métodos" })).toHaveAttribute("href", "/me/security/two-step-verification");
    await expect(page.locator("#twoFactorCurrentStatus")).toHaveText("Desabilitado");
    await expect(page.locator("#twoFactorPrimaryBtn")).toHaveText("Habilitar");
    await expect(page.locator("#twoFactorPrimaryBtn")).toBeEnabled();
    await expect(page.locator("#twoFactorVerificationArea")).toBeHidden();
    await expect(page.locator("#twoFactorQRCodeWrap")).toBeHidden();
    await expect(page.locator("#twoFactorRegenerateRecoveryCodes")).toBeHidden();
    await expect(page.locator("#twoFactorRecoveryCodes")).toBeHidden();
    await expect(page.locator("#twoFactorTOTPApps")).toBeVisible();
    await expect(page.locator("#twoFactorTOTPApps")).toContainText("Google Authenticator");
    expect(errors).toEqual([]);
  });

  test("habilitar mostra QR e chave, valida ao completar 6 dígitos e exibe os códigos de recuperação", async ({ page, context }) => {
    await context.grantPermissions(["clipboard-read", "clipboard-write"]);
    await openAppPage(page, "/me/security/two-factor/totp");

    let startBody = "";
    await page.route("**/me/twofactor/start", (route) => {
      startBody = route.request().postData() || "";
      return fulfillJSON(200, {
        ok: true,
        message: "Informe o código para confirmar.",
        setup: { method: "totp", qr_code_data: QR, manual_secret: "JBSWY3DPEHPK3PXP" },
      })(route);
    });
    let verifyBody = "";
    await page.route("**/me/twofactor/verify", (route) => {
      verifyBody = route.request().postData() || "";
      return fulfillJSON(200, {
        ok: true,
        message: "Verificação em duas etapas ativada.",
        status: enabledStatus,
        recovery_codes: ["AAAA-1111", "BBBB-2222", "CCCC-3333"],
      })(route);
    });

    await page.locator("#twoFactorPrimaryBtn").click();
    await expect(toast(page)).toContainText("Informe o código para confirmar.");
    expect(startBody).toContain("method=totp");
    await expect(page.locator("#twoFactorVerificationArea")).toBeVisible();
    await expect(page.locator("#twoFactorQRCode")).toHaveAttribute("src", QR);
    await expect(page.locator("#twoFactorManualSecret")).toHaveText("JBSWY3DPEHPK3PXP");
    await expect(page.locator("#twoFactorCode")).toBeFocused();
    await expect(page.locator("#twoFactorPrimaryBtn")).toHaveText("Cancelar configuração");

    // Letras são descartadas; o sexto dígito dispara a verificação e, no sucesso, o campo é limpo.
    await page.locator("#twoFactorCode").pressSequentially("12a3456");
    await expect(page.locator("#twoFactorCurrentStatus")).toHaveText("Habilitado por aplicativo autenticador");
    expect(verifyBody).toContain("code=123456");
    expect(verifyBody).toContain("method=totp");
    await expect(page.locator("#twoFactorCode")).toHaveValue("");
    await expect(page.locator("#twoFactorPrimaryBtn")).toHaveText("Desabilitar");
    await expect(page.locator("#twoFactorRecoveryCodes")).toBeVisible();
    await expect(page.locator("#twoFactorRecoveryCodeList li")).toHaveCount(3);
    await expect(page.locator("#twoFactorRecoveryCodeList")).toContainText("BBBB-2222");
    await expect(page.locator("#twoFactorRegenerateRecoveryCodes")).toBeVisible();
    await expect(page.locator("#twoFactorVerificationArea")).toBeHidden();

    await page.locator("#twoFactorCopyRecoveryCodes").click();
    await expect(toast(page)).toContainText(/Códigos copiados|Não foi possível copiar/);
  });

  test("código curto ou rejeitado mantém a configuração pendente e avisa", async ({ page }) => {
    await openAppPage(page, "/me/security/two-factor/totp");
    await page.route("**/me/twofactor/start", fulfillJSON(200, { ok: true, message: "Informe o código.", setup: { method: "totp" } }));
    await page.route("**/me/twofactor/verify", fulfillJSON(422, { ok: false, message: "Código inválido." }));

    await page.locator("#twoFactorPrimaryBtn").click();
    await expect(page.locator("#twoFactorVerificationArea")).toBeVisible();
    await expect(page.locator("#twoFactorQRCodeWrap")).toBeHidden();

    await page.locator("#twoFactorCode").fill("123");
    await page.keyboard.press("Enter");
    await expect(toast(page)).toContainText("Informe um código de 6 dígitos.");

    await page.locator("#twoFactorCode").fill("999999");
    await expect(toast(page)).toContainText("Código inválido.");
    await expect(page.locator("#twoFactorPrimaryBtn")).toHaveText("Cancelar configuração");
    await expect(page.locator("#twoFactorCurrentStatus")).toHaveText("Desabilitado");

    await page.locator("#twoFactorPrimaryBtn").click();
    await expect(page.locator("#twoFactorPrimaryBtn")).toHaveText("Habilitar");
    await expect(page.locator("#twoFactorVerificationArea")).toBeHidden();
    await expect(page.locator("#twoFactorCode")).toHaveValue("");
  });

  test("falha ao iniciar mostra a mensagem do servidor e continua desabilitado", async ({ page }) => {
    await openAppPage(page, "/me/security/two-factor/totp");
    await page.route("**/me/twofactor/start", fulfillJSON(429, { ok: false, message: "Muitas tentativas. Aguarde." }));

    await page.locator("#twoFactorPrimaryBtn").click();
    await expect(toast(page)).toContainText("Muitas tentativas. Aguarde.");
    await expect(page.locator("#twoFactorPrimaryBtn")).toHaveText("Habilitar");
    await expect(page.locator("#twoFactorPrimaryBtn")).toBeEnabled();
    await expect(page.locator("#twoFactorVerificationArea")).toBeHidden();
  });

  test("conta não confirmada não pode habilitar", async ({ page }) => {
    await openAppPage(page, "/me/security/two-factor/totp?ativa=0");
    let called = false;
    await page.route("**/me/twofactor/start", (route) => {
      called = true;
      return route.abort();
    });
    await page.locator("#twoFactorPrimaryBtn").click();
    await expect(toast(page)).toContainText("Confirme sua conta antes de habilitar a verificação em duas etapas.");
    expect(called).toBe(false);
  });

  test("com o método ativo é possível desabilitar mediante código e gerar novos códigos", async ({ page }) => {
    await openAppPage(page, "/me/security/two-factor/totp?2fa=enabled&method=totp");

    await expect(page.locator("#twoFactorCurrentStatus")).toHaveText("Habilitado por aplicativo autenticador");
    await expect(page.locator("#twoFactorPrimaryBtn")).toHaveText("Desabilitar");
    await expect(page.locator("#twoFactorRegenerateRecoveryCodes")).toBeVisible();
    await expect(page.locator("#twoFactorTOTPApps")).toBeHidden();

    await page.route("**/me/twofactor/recovery-codes/regenerate", fulfillJSON(200, {
      ok: true,
      message: "Novos códigos de recuperação gerados.",
      status: enabledStatus,
      recovery_codes: ["NEW1-0001", "NEW2-0002"],
    }));
    await page.locator("#twoFactorRegenerateRecoveryCodes").click();
    await expect(toast(page)).toContainText("Novos códigos de recuperação gerados.");
    await expect(page.locator("#twoFactorRecoveryCodeList li")).toHaveCount(2);
    await expect(page.locator("#twoFactorRecoveryCodeList")).toContainText("NEW2-0002");

    await page.route("**/me/twofactor/disable/start", fulfillJSON(200, { ok: true, message: "Informe o código para confirmar a desativação.", setup: { method: "totp" } }));
    await page.route("**/me/twofactor/disable/verify", fulfillJSON(200, { ok: true, message: "Verificação em duas etapas desativada.", status: disabledStatus }));

    await page.locator("#twoFactorPrimaryBtn").click();
    await expect(toast(page)).toContainText("Informe o código para confirmar a desativação.");
    await expect(page.locator("#twoFactorPrimaryBtn")).toHaveText("Cancelar desativação");
    await expect(page.locator("#twoFactorVerificationArea")).toBeVisible();
    await expect(page.locator("#twoFactorQRCodeWrap")).toBeHidden();
    await expect(page.locator("#twoFactorRegenerateRecoveryCodes")).toBeDisabled();

    await page.locator("#twoFactorCode").fill("654321");
    await expect(toast(page)).toContainText("Verificação em duas etapas desativada.");
    await expect(page.locator("#twoFactorCurrentStatus")).toHaveText("Desabilitado");
    await expect(page.locator("#twoFactorPrimaryBtn")).toHaveText("Habilitar");
    await expect(page.locator("#twoFactorRegenerateRecoveryCodes")).toBeHidden();
    await expect(page.locator("#twoFactorTOTPApps")).toBeVisible();
  });

  test("outro método já ativo bloqueia este e explica o motivo", async ({ page }) => {
    await openAppPage(page, "/me/security/two-factor/email?2fa=enabled&method=totp");

    await expect(page.getByRole("heading", { level: 1, name: "Código por e-mail" })).toBeVisible();
    await expect(page.locator("#twoFactorPrimaryBtn")).toHaveText("Método indisponível");
    await expect(page.locator("#twoFactorPrimaryBtn")).toBeDisabled();
    await expect(toast(page)).toContainText("Sua conta já usa aplicativo autenticador. Para configurar código por e-mail, desative o método ativo primeiro.");
  });

  test("método por e-mail não tem QR nem lista de aplicativos", async ({ page }) => {
    await openAppPage(page, "/me/security/two-factor/email");
    await page.route("**/me/twofactor/start", fulfillJSON(200, { ok: true, message: "Enviamos um código para o seu e-mail.", setup: { method: "email" } }));

    await expect(page.locator("#twoFactorTOTPApps")).toHaveCount(0);
    await expect(page.locator("#twoFactorQRCodeWrap")).toHaveCount(0);
    await page.locator("#twoFactorPrimaryBtn").click();
    await expect(toast(page)).toContainText("Enviamos um código para o seu e-mail.");
    await expect(page.locator("#twoFactorVerificationArea")).toBeVisible();
    await expect(page.locator("#twoFactorCode")).toBeFocused();
  });

  test("método desconhecido cai na página 404", async ({ page }) => {
    const response = await page.goto("/me/security/two-factor/sms");
    expect(response?.status()).toBe(404);
  });
});
