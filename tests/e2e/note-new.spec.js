// @ts-check
const { test, expect } = require("@playwright/test");
const { openAppPage } = require("./helpers");

async function setColorPicker(page, value) {
  await page.locator("#color").evaluate((el, v) => {
    el.value = v;
    el.dispatchEvent(new Event("input", { bubbles: true }));
  }, value);
}

test.describe("nova nota", () => {
  test("formulário nasce vazio, com contador zerado e anexos bloqueados até salvar", async ({ page }) => {
    const errors = await openAppPage(page, "/note/new");

    const form = page.locator("#note-new-form");
    await expect(form).toHaveAttribute("action", "/note");
    await expect(form.locator('input[name="gorilla.csrf.Token"]')).toHaveCount(1);
    await expect(page.locator("#title")).toHaveValue("");
    await expect(page.locator("#content")).toHaveValue("");

    const max = await page.locator("#title").getAttribute("maxlength");
    await expect(page.locator("#note-title-count")).toHaveText(`0/${max}`);
    await expect(page.getByText("Salve a nota para liberar o envio de imagens e documentos.")).toBeVisible();
    await expect(page.locator('form[enctype="multipart/form-data"]')).toHaveCount(0);
    await expect(page.getByRole("link", { name: "Cancelar" })).toHaveAttribute("href", "/");
    expect(errors).toEqual([]);
  });

  test("contador de título acompanha a digitação e trava no limite", async ({ page }) => {
    await openAppPage(page, "/note/new");

    const title = page.locator("#title");
    const max = Number(await title.getAttribute("maxlength"));
    const root = page.locator("[data-note-title-counter]");

    await title.fill("Café");
    await expect(page.locator("#note-title-count")).toHaveText(`4/${max}`);
    await expect(root).not.toHaveClass(/is-near-limit/);

    await title.evaluate((el, n) => {
      el.value = "x".repeat(n + 5);
      el.dispatchEvent(new Event("input", { bubbles: true }));
    }, max);
    await expect(title).toHaveValue("x".repeat(max));
    await expect(page.locator("#note-title-count")).toHaveText(`${max}/${max}`);
    await expect(root).toHaveClass(/is-at-limit/);
  });

  test("cor do cartão sincroniza seletor e campo hex nos dois sentidos", async ({ page }) => {
    await openAppPage(page, "/note/new");

    const hex = page.locator("#color-hex");
    const picker = page.locator("#color");

    await hex.fill("abc");
    await hex.blur();
    await expect(hex).toHaveValue("#aabbcc");
    await expect(picker).toHaveValue("#aabbcc");

    await setColorPicker(page, "#123456");
    await expect(hex).toHaveValue("#123456");

    await hex.fill("zzz");
    await hex.blur();
    expect(await hex.evaluate((el) => el.validationMessage)).toBe("Use #rgb ou #rrggbb (apenas 0-9 e a-f).");

    await hex.fill("");
    await hex.blur();
    await expect(hex).toHaveValue("#123456");
  });

  test("hex inválido impede o envio e explica o formato esperado", async ({ page }) => {
    await openAppPage(page, "/note/new");

    await page.locator("#title").fill("Nota de teste");
    await page.locator("#content").fill("Conteúdo");
    const hex = page.locator("#color-hex");
    await hex.fill("#12");
    const posted = [];
    page.on("request", (req) => {
      if (req.method() === "POST") posted.push(req.url());
    });
    await page.getByRole("button", { name: "Salvar" }).click();

    // O blur ao clicar em Salvar já marca o campo como inválido, e a validação nativa segura o envio antes do submit.
    await expect(page).toHaveURL(/\/note\/new$/);
    expect(await hex.evaluate((el) => el.validationMessage)).toBe("Use #rgb ou #rrggbb (apenas 0-9 e a-f).");
    expect(await hex.evaluate((el) => el.checkValidity())).toBe(false);
    expect(posted).toEqual([]);
  });

  test("etiquetas disponíveis podem ser marcadas e pertencem ao formulário da nota", async ({ page }) => {
    await openAppPage(page, "/note/new");

    const options = page.locator('input[name="tag_ids"]');
    await expect(options).toHaveCount(3);
    await expect(options.first()).toHaveAttribute("form", "note-new-form");
    await expect(page.locator("[data-note-tag-empty]")).toBeHidden();

    await page.getByText("#Trabalho", { exact: true }).click();
    await expect(page.locator('input[name="tag_ids"][value="1"]')).toBeChecked();
    await expect(page.locator('input[name="tag_ids"][value="2"]')).not.toBeChecked();
    await expect(page.locator("[data-note-color]").first()).toHaveCSS("background-color", "rgb(37, 99, 235)");
  });

  test("erros de validação do servidor aparecem no alerta do formulário", async ({ page }) => {
    const response = await page.request.get("/note/new?erro=title:Informe%20o%20t%C3%ADtulo.");
    expect(response.status()).toBe(422);

    await openAppPage(page, "/note/new?erro=title:Informe%20o%20t%C3%ADtulo.&erro=content:Escreva%20algo.");
    const alert = page.getByRole("alert");
    await expect(alert).toContainText("Preencha os campos obrigatórios");
    await expect(alert).toContainText("Informe o título.");
    await expect(alert).toContainText("Escreva algo.");
  });

  test("salvar com dados válidos envia o formulário e abre a nota criada", async ({ page }) => {
    await openAppPage(page, "/note/new");

    await page.locator("#title").fill("Nota de teste");
    await page.locator("#content").fill("Conteúdo da nota");
    await page.locator("#color-hex").fill("#abcdef");
    await page.getByRole("button", { name: "Salvar" }).click();

    await expect(page).toHaveURL(/\/note\/1$/);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Reunião de planejamento");
  });
});
