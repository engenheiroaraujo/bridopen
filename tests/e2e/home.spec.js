// @ts-check
const { test, expect } = require("@playwright/test");
const { openAppPage, shellKept, toast, fulfillJSON } = require("./helpers");

const cards = (page) => page.locator("#note-grid > li");

test.describe("home: lista de notas", () => {
  test("lista as notas ativas com cor, título, fallback e etiquetas", async ({ page }) => {
    const errors = await openAppPage(page, "/");

    await expect(page.getByRole("heading", { level: 1, name: "Últimas anotações" })).toBeVisible();
    await expect(page.getByText("3 notas ativas")).toBeVisible();
    await expect(cards(page)).toHaveCount(3);

    const first = cards(page).filter({ hasText: "Reunião de planejamento" });
    await expect(first.locator("[data-note-color]")).toHaveCSS("border-left-color", "rgb(254, 243, 199)");
    await expect(first.getByRole("link", { name: "#Trabalho" })).toHaveAttribute("href", "/?tag=trabalho");
    await expect(first.getByRole("button", { name: "Desfixar nota" })).toHaveAttribute("aria-pressed", "true");

    await expect(cards(page).filter({ hasText: "Sem título" })).toHaveCount(1);
    await expect(page.locator("#main-content").getByRole("link", { name: "Nova nota", exact: true })).toHaveAttribute("href", "/note/new");
    expect(errors).toEqual([]);
  });

  test("busca por texto navega parcialmente, destaca o termo e ordena por relevância", async ({ page }) => {
    await openAppPage(page, "/");

    await page.locator("#note-search-input").fill("compras");
    await expect(page).toHaveURL(/\?q=compras/);
    await expect(cards(page)).toHaveCount(1);
    await expect(page.getByText("1 nota encontrada")).toBeVisible();
    await expect(cards(page).locator("mark")).toHaveText("compras");
    // A busca envia o sort atual do select; a opção de relevância só passa a existir quando há texto.
    await expect(page.locator('select[name="sort"] option[value="relevance"]')).toHaveText("Melhores resultados");
    await expect(page.locator('select[name="sort"]')).toHaveValue("pinned");
    await expect(page.getByLabel("Filtros ativos")).toContainText("Texto");
    await expect(page.getByLabel("Filtros ativos")).toContainText("compras");
    await expect(page.getByText('Filtrando por texto "compras".')).toBeVisible();
    expect(await shellKept(page)).toBe(true);
  });

  test("filtros de fixadas, cor e etiqueta enviam sozinhos e Limpar remove tudo", async ({ page }) => {
    await openAppPage(page, "/");

    // A checkbox é sr-only; o usuário clica no rótulo.
    await page.locator("label").filter({ has: page.locator('input[name="pinned"]') }).click();
    await expect(page).toHaveURL(/pinned=true/);
    await expect(cards(page)).toHaveCount(1);
    await expect(page.getByLabel("Filtros ativos")).toContainText("Fixadas");

    await page.getByRole("link", { name: "Limpar" }).click();
    await expect(page).toHaveURL(/\/$/);
    await expect(cards(page)).toHaveCount(3);

    await page.getByText("Filtros de cor e etiqueta", { exact: true }).click();
    await page.getByTitle("Filtrar por cor #dbeafe").click();
    await expect(page).toHaveURL(/color=%23dbeafe/);
    await expect(cards(page)).toHaveCount(1);
    await expect(cards(page)).toContainText("Lista de compras");

    await page.getByRole("link", { name: "Limpar" }).click();
    await page.getByText("Filtros de cor e etiqueta", { exact: true }).click();
    await page.locator("label").filter({ has: page.locator('input[name="tag"][value="pessoal"]') }).click();
    await expect(page).toHaveURL(/tag=pessoal/);
    await expect(cards(page)).toHaveCount(1);
    await expect(page.getByLabel("Filtros ativos")).toContainText("#Pessoal");
    expect(await shellKept(page)).toBe(true);
  });

  test("atalhos / e Ctrl+K focam a busca", async ({ page }) => {
    await openAppPage(page, "/");

    await page.locator("body").click({ position: { x: 5, y: 5 } });
    await page.keyboard.press("/");
    await expect(page.locator("#note-search-input")).toBeFocused();

    await page.locator("h1").click();
    await page.keyboard.press("Control+k");
    await expect(page.locator("#note-search-input")).toBeFocused();
  });

  test("fixar e arquivar postam pela navegação parcial e voltam para a lista", async ({ page }) => {
    await openAppPage(page, "/");

    const pin = page.waitForRequest((req) => req.method() === "POST" && req.url().endsWith("/note/2/pin"));
    await cards(page).filter({ hasText: "Lista de compras" }).getByRole("button", { name: "Fixar nota" }).click();
    const request = await pin;
    expect(request.headers()["x-nav-mode"]).toBe("partial");
    await expect(page).toHaveURL(/\/$/);
    await expect(cards(page)).toHaveCount(3);
    expect(await shellKept(page)).toBe(true);

    const archive = page.waitForRequest((req) => req.method() === "POST" && req.url().endsWith("/note/2/archive"));
    await cards(page).filter({ hasText: "Lista de compras" }).getByRole("button", { name: "Arquivar nota" }).click();
    await archive;
    await expect(page.locator("#main-content")).not.toHaveAttribute("aria-busy", "true");
    expect(await shellKept(page)).toBe(true);
  });

  test("modal de lixeira mostra os textos do cartão e fecha por Cancelar e Escape", async ({ page }) => {
    await openAppPage(page, "/");

    const modal = page.locator("#note-delete-modal");
    await cards(page).first().getByRole("button", { name: "Mover nota para lixeira" }).click();
    await expect(modal).toBeVisible();
    await expect(modal).toHaveAttribute("aria-hidden", "false");
    await expect(modal.getByRole("heading")).toHaveText("Mover para lixeira?");
    await expect(modal.locator("#note-delete-modal-desc")).toHaveText("A nota será movida para a lixeira.");
    await expect(modal.locator("#note-delete-confirm")).toHaveText("Mover para lixeira");
    await expect(modal.locator("#note-delete-cancel")).toBeFocused();

    await modal.locator("#note-delete-cancel").click();
    await expect(modal).toBeHidden();

    await cards(page).first().getByRole("button", { name: "Mover nota para lixeira" }).click();
    await expect(modal).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(modal).toBeHidden();
    await expect(page.locator("body")).not.toHaveClass(/overflow-hidden/);
  });

  test("confirmar a lixeira envia DELETE com CSRF, navega e avisa por toast", async ({ page }) => {
    await openAppPage(page, "/");

    await cards(page).filter({ hasText: "Lista de compras" }).getByRole("button", { name: "Mover nota para lixeira" }).click();
    const deletion = page.waitForRequest((req) => req.method() === "DELETE" && req.url().endsWith("/note/2"));
    await page.locator("#note-delete-confirm").click();
    const request = await deletion;
    expect(request.headers()["x-csrf-token"]).toBeTruthy();
    expect(request.headers()["accept"]).toBe("application/json");

    await expect(page.locator("#note-delete-modal")).toBeHidden();
    await expect(toast(page)).toContainText("Nota movida para a lixeira.");
    await expect(page).toHaveURL(/\/$/);
    expect(await shellKept(page)).toBe(true);
  });

  test("falha ao excluir mantém o modal, libera o botão e mostra o erro do servidor", async ({ page }) => {
    await openAppPage(page, "/");
    await page.route("**/note/2", fulfillJSON(500, { ok: false, message: "Banco indisponível." }));

    await cards(page).filter({ hasText: "Lista de compras" }).getByRole("button", { name: "Mover nota para lixeira" }).click();
    await page.locator("#note-delete-confirm").click();

    await expect(toast(page)).toContainText("Banco indisponível.");
    await expect(page.locator("#note-delete-modal")).toBeVisible();
    await expect(page.locator("#note-delete-confirm")).toBeEnabled();
  });

  test("estados vazios diferenciam lista nova de filtro sem resultado", async ({ page }) => {
    await openAppPage(page, "/?vazio=1");
    await expect(page.getByText("Nenhuma anotação foi criada ainda. Que tal criar uma?")).toBeVisible();
    await expect(page.locator("#note-grid")).toHaveCount(0);

    await openAppPage(page, "/?q=inexistente");
    await expect(page.getByText("Nenhuma anotação encontrada com os filtros atuais.")).toBeVisible();
    await expect(page.getByText("0 notas encontradas")).toBeVisible();
  });
});
