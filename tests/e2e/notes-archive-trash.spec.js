// @ts-check
const { test, expect } = require("@playwright/test");
const { openAppPage, shellKept, toast } = require("./helpers");

test.describe("arquivo", () => {
  test("lista as notas arquivadas com desarquivar e lixeira", async ({ page }) => {
    const errors = await openAppPage(page, "/notes/archive");

    await expect(page.getByRole("heading", { level: 1, name: "Arquivo" })).toBeVisible();
    await expect(page.getByText("Notas arquivadas ficam fora da home, mas continuam disponíveis aqui.")).toBeVisible();
    const cards = page.locator("#note-grid > li");
    await expect(cards).toHaveCount(1);
    await expect(cards).toContainText("Relatório antigo");
    await expect(cards.getByRole("link", { name: "#Trabalho" })).toBeVisible();
    await expect(cards.getByRole("link", { name: "#Ideias" })).toBeVisible();
    await expect(cards.locator('form[action="/note/4/unarchive"]').getByRole("button", { name: "Desarquivar nota" })).toBeVisible();

    const trash = cards.getByRole("button", { name: "Mover nota para lixeira" });
    await expect(trash).toHaveAttribute("data-note-delete-url", "/note/4");
    await expect(trash).toHaveAttribute("data-note-delete-redirect", "/notes/archive");
    expect(errors).toEqual([]);
  });

  test("desarquivar posta pela navegação parcial e permanece no arquivo", async ({ page }) => {
    await openAppPage(page, "/notes/archive");

    const request = page.waitForRequest((req) => req.method() === "POST" && req.url().endsWith("/note/4/unarchive"));
    await page.getByRole("button", { name: "Desarquivar nota" }).click();
    expect((await request).headers()["x-nav-mode"]).toBe("partial");
    await expect(page.locator("#main-content")).not.toHaveAttribute("aria-busy", "true");
    expect(await shellKept(page)).toBe(true);
  });

  test("arquivo vazio explica que não há notas", async ({ page }) => {
    await openAppPage(page, "/notes/archive?vazio=1");
    await expect(page.getByText("Nenhuma nota arquivada.")).toBeVisible();
    await expect(page.locator("#note-grid")).toHaveCount(0);
  });
});

test.describe("lixeira", () => {
  test("lista as notas excluídas com restaurar e exclusão definitiva", async ({ page }) => {
    const errors = await openAppPage(page, "/notes/trash");

    await expect(page.getByRole("heading", { level: 1, name: "Lixeira" })).toBeVisible();
    const cards = page.locator("#note-grid > li");
    await expect(cards).toHaveCount(1);
    await expect(cards).toContainText("Ideia descartada");
    await expect(cards.locator('form[action="/note/5/restore"]').getByRole("button", { name: "Restaurar nota" })).toBeVisible();

    const destroy = cards.getByRole("button", { name: "Excluir nota definitivamente" });
    await expect(destroy).toHaveAttribute("data-note-delete-url", "/note/5/destroy");
    await expect(destroy).toHaveAttribute("data-note-delete-redirect", "/notes/trash");
    expect(errors).toEqual([]);
  });

  test("excluir definitivamente avisa que não há volta e confirma por DELETE", async ({ page }) => {
    await openAppPage(page, "/notes/trash");

    await page.getByRole("button", { name: "Excluir nota definitivamente" }).click();
    const modal = page.locator("#note-delete-modal");
    await expect(modal.getByRole("heading")).toHaveText("Excluir definitivamente?");
    await expect(modal.locator("#note-delete-modal-desc")).toContainText("Esta ação não pode ser desfeita.");
    await expect(modal.locator("#note-delete-confirm")).toHaveText("Excluir definitivamente");

    const deletion = page.waitForRequest((req) => req.method() === "DELETE" && req.url().endsWith("/note/5/destroy"));
    await modal.locator("#note-delete-confirm").click();
    await deletion;
    await expect(toast(page)).toContainText("Nota excluída definitivamente.");
    await expect(page).toHaveURL(/\/notes\/trash$/);
    expect(await shellKept(page)).toBe(true);
  });

  test("restaurar posta pela navegação parcial", async ({ page }) => {
    await openAppPage(page, "/notes/trash");

    const request = page.waitForRequest((req) => req.method() === "POST" && req.url().endsWith("/note/5/restore"));
    await page.getByRole("button", { name: "Restaurar nota" }).click();
    expect((await request).headers()["x-nav-mode"]).toBe("partial");
    expect(await shellKept(page)).toBe(true);
  });

  test("lixeira vazia explica que não há notas", async ({ page }) => {
    await openAppPage(page, "/notes/trash?vazio=1");
    await expect(page.getByText("A lixeira está vazia.")).toBeVisible();
  });
});
