// @ts-check
const { test, expect } = require("@playwright/test");
const { openAppPage, shellKept, toast } = require("./helpers");

test.describe("visualizar nota", () => {
  test("nota ativa mostra conteúdo, etiquetas, anexos e as ações de editar, desfixar, arquivar e lixeira", async ({ page }) => {
    const errors = await openAppPage(page, "/note/1");

    await expect(page).toHaveTitle("Nota Reunião de planejamento — Bridopen");
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Reunião de planejamento");
    await expect(page.getByText("Definir metas do trimestre e revisar o orçamento.")).toBeVisible();
    await expect(page.getByRole("link", { name: "#Trabalho" })).toHaveAttribute("href", "/?tag=trabalho");
    await expect(page.getByRole("link", { name: "Voltar" })).toHaveAttribute("href", "/");
    await expect(page.getByRole("link", { name: "Editar" })).toHaveAttribute("href", "/note/1/edit");

    await expect(page.locator('form[action="/note/1/unpin"] button')).toHaveText("Desfixar");
    await expect(page.locator('form[action="/note/1/archive"] input[name="redirect"]')).toHaveValue("/notes/archive");
    await expect(page.getByRole("button", { name: "Lixeira" })).toHaveAttribute("data-note-delete-redirect", "/");
    await expect(page.getByLabel("Anexos da nota").getByRole("listitem")).toHaveCount(2);
    expect(errors).toEqual([]);
  });

  test("nota não fixada oferece Fixar", async ({ page }) => {
    await openAppPage(page, "/note/2");
    await expect(page.locator('form[action="/note/2/pin"] button')).toHaveText("Fixar");
    await expect(page.getByText("Nenhum anexo ainda.")).toBeVisible();
  });

  test("desfixar posta pela navegação parcial e permanece na nota", async ({ page }) => {
    await openAppPage(page, "/note/1");

    const request = page.waitForRequest((req) => req.method() === "POST" && req.url().endsWith("/note/1/unpin"));
    await page.locator('form[action="/note/1/unpin"] button').click();
    expect((await request).headers()["x-nav-mode"]).toBe("partial");

    await expect(page).toHaveURL(/\/note\/1$/);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Reunião de planejamento");
    expect(await shellKept(page)).toBe(true);
  });

  test("nota arquivada volta para o arquivo e oferece Desarquivar", async ({ page }) => {
    await openAppPage(page, "/note/4");

    await expect(page.getByRole("link", { name: "Voltar" })).toHaveAttribute("href", "/notes/archive");
    await expect(page.locator('form[action="/note/4/unarchive"] button')).toHaveText("Desarquivar");
    await expect(page.locator('form[action="/note/4/pin"]')).toHaveCount(0);
    await expect(page.locator('form[action="/note/4/archive"]')).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Lixeira" })).toHaveAttribute("data-note-delete-redirect", "/notes/archive");
  });

  test("nota na lixeira só permite restaurar ou excluir definitivamente", async ({ page }) => {
    await openAppPage(page, "/note/5");

    await expect(page.getByRole("link", { name: "Voltar" })).toHaveAttribute("href", "/notes/trash");
    await expect(page.locator('form[action="/note/5/restore"] button')).toHaveText("Restaurar");
    await expect(page.getByRole("link", { name: "Editar" })).toHaveCount(0);
    await expect(page.getByText("Restaure a nota para adicionar ou remover anexos.")).toBeVisible();

    await page.getByRole("button", { name: "Excluir" }).click();
    const modal = page.locator("#note-delete-modal");
    await expect(modal.getByRole("heading")).toHaveText("Excluir definitivamente?");
    await expect(modal.locator("#note-delete-confirm")).toHaveText("Excluir definitivamente");

    const deletion = page.waitForRequest((req) => req.method() === "DELETE" && req.url().endsWith("/note/5/destroy"));
    await modal.locator("#note-delete-confirm").click();
    await deletion;
    await expect(toast(page)).toContainText("Nota excluída definitivamente.");
    await expect(page).toHaveURL(/\/notes\/trash$/);
    await expect(page.getByRole("heading", { level: 1, name: "Lixeira" })).toBeVisible();
    expect(await shellKept(page)).toBe(true);
  });

  test("mover para a lixeira a partir da nota leva de volta à home", async ({ page }) => {
    await openAppPage(page, "/note/1");

    await page.getByRole("button", { name: "Lixeira" }).click();
    await expect(page.locator("#note-delete-modal")).toBeVisible();
    await page.locator("#note-delete-confirm").click();

    await expect(toast(page)).toContainText("Nota movida para a lixeira.");
    await expect(page).toHaveURL(/\/$/);
    await expect(page.getByRole("heading", { level: 1, name: "Últimas anotações" })).toBeVisible();
    expect(await shellKept(page)).toBe(true);
  });
});
