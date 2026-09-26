// @ts-check
const { test, expect } = require("@playwright/test");
const { openAppPage, shellKept, toast } = require("./helpers");

test.describe("editar nota", () => {
  test("campos vêm preenchidos com a nota, contador reflete o título e etiquetas marcadas", async ({ page }) => {
    const errors = await openAppPage(page, "/note/1/edit");

    const form = page.locator("#note-new-form");
    await expect(form).toHaveAttribute("action", "/note");
    await expect(form.locator('input[name="id"]')).toHaveValue("1");
    await expect(page.locator("#title")).toHaveValue("Reunião de planejamento");
    await expect(page.locator("#content")).toHaveValue("Definir metas do trimestre e revisar o orçamento.");
    await expect(page.locator("#color-hex")).toHaveValue("#fef3c7");
    await expect(page.locator("#color")).toHaveValue("#fef3c7");

    const max = await page.locator("#title").getAttribute("maxlength");
    await expect(page.locator("#note-title-count")).toHaveText(`23/${max}`);

    await expect(page.locator('input[name="tag_ids"][value="1"]')).toBeChecked();
    await expect(page.locator('input[name="tag_ids"][value="2"]')).not.toBeChecked();
    await expect(page.getByRole("link", { name: "Voltar" })).toHaveAttribute("href", "/note/1");
    await expect(page.getByRole("button", { name: "Atualizar" })).toBeVisible();
    expect(errors).toEqual([]);
  });

  test("anexos existentes são listados e o envio aponta para a nota", async ({ page }) => {
    await openAppPage(page, "/note/1/edit");

    const list = page.getByLabel("Anexos da nota");
    await expect(list.getByRole("listitem")).toHaveCount(2);
    // A imagem tem dois links (miniatura e nome); o PDF só o do nome.
    await expect(list.getByRole("link", { name: "planta.png" })).toHaveCount(2);
    await expect(list.getByText("planta.png")).toHaveAttribute("href", "/note/1/attachments/10");
    await expect(list.locator("img")).toHaveAttribute("src", "/note/1/attachments/10");
    await expect(list.getByRole("link", { name: "contrato.pdf" })).toHaveAttribute("href", "/note/1/attachments/11");

    const upload = page.locator('form[action="/note/1/attachments"]');
    await expect(upload).toHaveAttribute("enctype", "multipart/form-data");
    await expect(upload.locator('input[name="redirect"]')).toHaveValue("/note/1/edit");
    await expect(upload.locator('input[type="file"]')).toHaveAttribute("required", "");
    await expect(page.getByText("JPG, PNG, WEBP, PDF, TXT ou DOCX até 10 MB.")).toBeVisible();
  });

  test("remover anexo confirma no modal, envia DELETE e recarrega a mesma tela", async ({ page }) => {
    await openAppPage(page, "/note/1/edit");

    await page.getByRole("button", { name: "Remover anexo" }).first().click();
    const modal = page.locator("#note-delete-modal");
    await expect(modal).toBeVisible();
    await expect(modal.getByRole("heading")).toHaveText("Remover anexo?");
    await expect(modal.locator("#note-delete-modal-desc")).toHaveText("O arquivo será removido desta nota.");

    const deletion = page.waitForRequest((req) => req.method() === "DELETE" && req.url().endsWith("/note/1/attachments/10"));
    await modal.locator("#note-delete-confirm").click();
    await deletion;

    await expect(modal).toBeHidden();
    await expect(toast(page)).toContainText("Anexo removido.");
    await expect(page).toHaveURL(/\/note\/1\/edit$/);
    expect(await shellKept(page)).toBe(true);
  });

  test("erros de validação do servidor aparecem sem perder os valores", async ({ page }) => {
    await openAppPage(page, "/note/1/edit?erro=content:O%20conte%C3%BAdo%20%C3%A9%20obrigat%C3%B3rio.");

    await expect(page.getByRole("alert")).toContainText("O conteúdo é obrigatório.");
    await expect(page.locator("#title")).toHaveValue("Reunião de planejamento");
  });

  test("atualizar envia o formulário e volta para a nota", async ({ page }) => {
    await openAppPage(page, "/note/1/edit");

    await page.locator("#title").fill("Reunião revisada");
    await page.getByRole("button", { name: "Atualizar" }).click();
    await expect(page).toHaveURL(/\/note\/1$/);
  });

  test("nota inexistente cai na página 404", async ({ page }) => {
    const response = await page.goto("/note/999/edit");
    expect(response?.status()).toBe(404);
    await expect(page.getByRole("heading", { name: "Página não encontrada" })).toBeVisible();
  });
});
