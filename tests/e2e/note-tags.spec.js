// @ts-check
const { test, expect } = require("@playwright/test");
const { openAppPage } = require("./helpers");

const items = (page) => page.locator("[data-tag-item]");

test.describe("etiquetas", () => {
  test("lista as etiquetas com cor, nome e ações", async ({ page }) => {
    const errors = await openAppPage(page, "/tags");

    await expect(page.getByRole("heading", { level: 1, name: "Etiquetas" })).toBeVisible();
    await expect(items(page)).toHaveCount(3);
    const trabalho = items(page).filter({ hasText: "#Trabalho" });
    await expect(trabalho).toContainText("#2563eb");
    await expect(trabalho.locator("[data-note-color]")).toHaveCSS("background-color", "rgb(37, 99, 235)");
    await expect(items(page).filter({ hasText: "#Ideias" })).toContainText("sem cor");
    await expect(trabalho.locator('form[action="/tags/1/delete"] input[name="redirect"]')).toHaveValue("/tags");
    await expect(page.locator("#main-content").getByRole("link", { name: "Nova nota", exact: true })).toHaveAttribute("href", "/note/new");
    expect(errors).toEqual([]);
  });

  test("filtros por nome e cor escondem itens e Limpar restaura tudo", async ({ page }) => {
    await openAppPage(page, "/tags");

    const name = page.locator("[data-tag-name-filter]");
    const color = page.locator("[data-tag-color-filter]");
    const empty = page.locator("[data-tag-filter-empty]");

    await name.fill("trab");
    await expect(items(page).filter({ hasText: "#Trabalho" })).toBeVisible();
    await expect(items(page).filter({ hasText: "#Pessoal" })).toBeHidden();
    await expect(empty).toBeHidden();

    await name.fill("");
    await color.fill("16a");
    await expect(items(page).filter({ hasText: "#Pessoal" })).toBeVisible();
    await expect(items(page).filter({ hasText: "#Trabalho" })).toBeHidden();

    await name.fill("trab");
    await expect(empty).toBeVisible();
    await expect(empty).toHaveText("Nenhuma etiqueta encontrada com os filtros atuais.");

    await page.locator("[data-tag-filter-clear]").click();
    await expect(name).toHaveValue("");
    await expect(color).toHaveValue("");
    await expect(name).toBeFocused();
    for (let i = 0; i < 3; i += 1) await expect(items(page).nth(i)).toBeVisible();
  });

  test("menu de ações abre um por vez e fecha ao clicar fora ou com Escape", async ({ page }) => {
    await openAppPage(page, "/tags");

    const trigger = page.getByRole("button", { name: "Ações da etiqueta Trabalho" });
    const menu = page.locator("#tagActionsMenu1");
    await expect(menu).toBeHidden();

    await trigger.click();
    await expect(menu).toBeVisible();
    await expect(menu).toHaveAttribute("aria-hidden", "false");
    await expect(trigger).toHaveAttribute("aria-expanded", "true");

    await page.locator("h1").click();
    await expect(menu).toBeHidden();
    await expect(trigger).toHaveAttribute("aria-expanded", "false");

    // Abrir o segundo menu fecha o primeiro, mesmo se ele ainda estivesse aberto por script.
    await trigger.click();
    await page.getByRole("button", { name: "Ações da etiqueta Pessoal" }).evaluate((el) => el.click());
    await expect(menu).toBeHidden();
    await expect(page.locator("#tagActionsMenu2")).toBeVisible();

    await page.keyboard.press("Escape");
    await expect(page.locator("#tagActionsMenu2")).toBeHidden();
    await expect(page.getByRole("button", { name: "Ações da etiqueta Pessoal" })).toHaveAttribute("aria-expanded", "false");
  });

  test("Nova etiqueta abre o modal limpo, com cor padrão e sincronização do hex", async ({ page }) => {
    await openAppPage(page, "/tags");

    const modal = page.locator("#tag-create-modal");
    await expect(modal).toBeHidden();
    await page.getByRole("button", { name: "Nova etiqueta" }).click();
    await expect(modal).toBeVisible();
    await expect(modal).toHaveAttribute("aria-hidden", "false");
    await expect(page.locator("#tag-create-name")).toBeFocused();
    await expect(page.locator("#tag-create-form")).toHaveAttribute("action", "/tags");
    await expect(page.locator("#tag-create-color")).toHaveValue("#2f4538");

    const hex = page.locator("#tag-create-color-hex");
    await hex.fill("abc");
    await hex.blur();
    await expect(hex).toHaveValue("#aabbcc");
    await expect(page.locator("#tag-create-color")).toHaveValue("#aabbcc");

    await hex.fill("xyz");
    await hex.blur();
    expect(await hex.evaluate((el) => el.validationMessage)).toBe("Use #rgb ou #rrggbb (apenas 0-9 e a-f).");

    await page.locator("#tag-create-name").fill("Urgente");
    await modal.getByRole("button", { name: "Criar etiqueta" }).click();
    // A validação nativa segura o envio com o hex inválido marcado no blur.
    await expect(page).toHaveURL(/\/tags$/);
    expect(await hex.evaluate((el) => el.checkValidity())).toBe(false);
    await expect(modal).toBeVisible();

    await modal.getByRole("button", { name: "Cancelar" }).click();
    await expect(modal).toBeHidden();
  });

  test("Editar abre o modal preenchido com a etiqueta e aponta para a rota dela", async ({ page }) => {
    await openAppPage(page, "/tags");

    await page.getByRole("button", { name: "Ações da etiqueta Trabalho" }).click();
    await page.locator("#tagActionsMenu1").getByRole("menuitem", { name: "Editar" }).click();

    const modal = page.locator("#tag-edit-modal");
    await expect(modal).toBeVisible();
    await expect(page.locator("#tagActionsMenu1")).toBeHidden();
    await expect(page.locator("#tag-edit-name")).toHaveValue("Trabalho");
    await expect(page.locator("#tag-edit-name")).toBeFocused();
    await expect(page.locator("#tag-edit-color")).toHaveValue("#2563eb");
    await expect(page.locator("#tag-edit-color-hex")).toHaveValue("#2563eb");
    await expect(page.locator("#tag-edit-form")).toHaveAttribute("action", /\/tags\/1$/);

    await page.keyboard.press("Escape");
    await expect(modal).toBeHidden();
  });

  test("erros do servidor reabrem o modal certo com a mensagem", async ({ page }) => {
    await openAppPage(page, "/tags?erro=name:Nome%20obrigat%C3%B3rio.&form=create");
    const create = page.locator("#tag-create-modal");
    await expect(create).toBeVisible();
    await expect(create.getByRole("alert")).toContainText("Nome obrigatório.");
    await expect(page.locator("#tag-edit-modal")).toBeHidden();

    await openAppPage(page, "/tags?erro=name:Nome%20j%C3%A1%20existe.&form=edit&name=Trabalho&color=%232563eb");
    const edit = page.locator("#tag-edit-modal");
    await expect(edit).toBeVisible();
    await expect(edit.getByRole("alert")).toContainText("Nome já existe.");
    await expect(page.locator("#tag-edit-name")).toHaveValue("Trabalho");
    await expect(page.locator("#tag-edit-form")).toHaveAttribute("action", "/tags/1");
    await expect(create).toBeHidden();
  });

  test("sem etiquetas mostra o estado vazio e ainda permite criar", async ({ page }) => {
    await openAppPage(page, "/tags?vazio=1");
    await expect(page.getByText("Nenhuma etiqueta criada ainda.")).toBeVisible();
    await expect(items(page)).toHaveCount(0);
    await page.getByRole("button", { name: "Nova etiqueta" }).click();
    await expect(page.locator("#tag-create-modal")).toBeVisible();
  });
});
