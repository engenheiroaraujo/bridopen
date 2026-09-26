// @ts-check
const { expect } = require("@playwright/test");

/**
 * Coleta erros de JavaScript da página: exceções não tratadas e console.error.
 * Falhas de rede do CDN do Tailwind são ignoradas para o teste não depender
 * de internet; o comportamento das telas não depende desse CSS.
 */
function watchConsole(page) {
  const errors = [];
  page.on("pageerror", (err) => errors.push(`pageerror: ${err.message}`));
  page.on("console", (msg) => {
    if (msg.type() !== "error") return;
    const url = msg.location().url || "";
    const text = msg.text();
    if (url.includes("cdn.tailwindcss.com")) return;
    if (/Failed to load resource/.test(text)) {
      // Sem URL é falha de rede fora do nosso controle (ex.: buffer do socket); a própria página com status 4xx/5xx é esperada
      // nas telas de erro. Um asset nosso com 404 continua acusando, porque vem com a URL dele.
      if (!url || url === page.url()) return;
    }
    errors.push(`console.error: ${text}${url ? ` (${url})` : ""}`);
  });
  return errors;
}

/** Abre a rota, espera os scripts carregarem e garante que nada quebrou. */
async function openAuthPage(page, path) {
  const errors = watchConsole(page);
  await page.goto(path);
  await page.waitForLoadState("load");
  // O ciclo de vida das páginas precisa existir: sem ele index.js e toast.js
  // param na primeira linha e nada abaixo funciona.
  await expect
    .poll(() => page.evaluate(() => typeof window.BridopenPage))
    .toBe("object");
  return errors;
}

/**
 * Abre uma tela do app logado (shell com sidebar) e marca a janela para os
 * testes de navegação parcial provarem que não houve recarregamento completo.
 */
async function openAppPage(page, path) {
  const errors = await openAuthPage(page, path);
  await expect(page.locator("#app-sidebar")).toBeAttached();
  await page.evaluate(() => {
    window.__shellMarker = true;
  });
  return errors;
}

/** Verdadeiro enquanto o shell não foi recarregado desde openAppPage. */
function shellKept(page) {
  return page.evaluate(() => window.__shellMarker === true);
}

/**
 * Preenche um campo de senha. Alguns deles nascem readonly e só liberam a
 * edição no foco (defesa contra autofill), então é preciso clicar antes.
 */
async function fillPassword(locator, value) {
  await locator.click();
  await locator.fill(value);
}

/** Estado observável do botão de mostrar/ocultar senha. */
async function toggleState(button, input) {
  return {
    type: await input.getAttribute("type"),
    pressed: await button.getAttribute("aria-pressed"),
    label: await button.getAttribute("aria-label"),
    eyeHidden: await button.locator(".icon-eye").evaluate((el) => el.classList.contains("hidden")),
    eyeOffHidden: await button.locator(".icon-eye-off").evaluate((el) => el.classList.contains("hidden")),
  };
}

/** Marca "Não sou um robô" e responde o captcha do formulário. */
async function solveCaptcha(page, answer = "1234") {
  await page.getByLabel("Não sou um robô").check();
  await page.locator("#captcha_answer").fill(answer);
}

/** Último toast exibido pelo shell. */
function toast(page) {
  return page.locator(".qn-toast").last();
}

/** Responde uma rota JSON com o corpo dado, no formato que o backend usa. */
function fulfillJSON(status, body) {
  return (route) =>
    route.fulfill({
      status,
      contentType: "application/json; charset=utf-8",
      body: JSON.stringify(body),
    });
}

/** Dispara paste/beforeinput/drop sintéticos e informa quais foram bloqueados. */
function pasteBlocked(el) {
  const paste = new ClipboardEvent("paste", { bubbles: true, cancelable: true });
  el.dispatchEvent(paste);
  const beforeInput = new InputEvent("beforeinput", { bubbles: true, cancelable: true, inputType: "insertFromPaste" });
  el.dispatchEvent(beforeInput);
  const drop = new DragEvent("drop", { bubbles: true, cancelable: true });
  el.dispatchEvent(drop);
  return { paste: paste.defaultPrevented, beforeInput: beforeInput.defaultPrevented, drop: drop.defaultPrevented };
}

module.exports = {
  watchConsole,
  openAuthPage,
  openAppPage,
  shellKept,
  fillPassword,
  toggleState,
  solveCaptcha,
  toast,
  fulfillJSON,
  pasteBlocked,
};
