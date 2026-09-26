// @ts-check
const { test, expect } = require("@playwright/test");
const { openAppPage, shellKept } = require("./helpers");

test.describe("shell do app: sidebar, navegação parcial e tema", () => {
  test("sidebar marca a página atual e mostra o nome do usuário", async ({ page }) => {
    const errors = await openAppPage(page, "/");

    const sidebar = page.locator("#app-sidebar");
    await expect(sidebar.getByRole("link", { name: /Home/ })).toHaveAttribute("aria-current", "page");
    await expect(sidebar.getByRole("link", { name: /Arquivo/ })).not.toHaveAttribute("aria-current", "page");
    await expect(sidebar.getByText("Ada", { exact: true })).toBeVisible();
    await expect(sidebar.locator('form[action="/user/signout"]')).toHaveAttribute("data-nav-full", "");
    expect(errors).toEqual([]);
  });

  test("grupos da conta abrem, fecham e lembram o estado entre cargas", async ({ page }) => {
    await openAppPage(page, "/");

    const group = page.locator('[data-sidebar-group="overview"]');
    const panel = page.locator("#sidebarOverviewSubnav");
    await expect(group).toHaveAttribute("aria-expanded", "false");
    await expect(panel).toBeHidden();

    await group.click();
    await expect(group).toHaveAttribute("aria-expanded", "true");
    await expect(panel).toBeVisible();
    expect(await page.evaluate(() => JSON.parse(sessionStorage.getItem("qn-sidebar-groups") || "{}"))).toEqual({ overview: true });

    await page.reload();
    await expect(page.locator("#sidebarOverviewSubnav")).toBeVisible();

    await page.locator('[data-sidebar-group="overview"]').click();
    await expect(page.locator("#sidebarOverviewSubnav")).toBeHidden();
  });

  test("link do menu troca só o conteúdo principal e sincroniza o item ativo", async ({ page }) => {
    await openAppPage(page, "/");

    await page.locator("#app-sidebar").getByRole("link", { name: /Arquivo/ }).click();
    await expect(page).toHaveURL(/\/notes\/archive$/);
    await expect(page.getByRole("heading", { level: 1, name: "Arquivo" })).toBeVisible();
    await expect(page).toHaveTitle("Arquivo — Bridopen");
    expect(await shellKept(page)).toBe(true);

    const sidebar = page.locator("#app-sidebar");
    await expect(sidebar.getByRole("link", { name: /Arquivo/ })).toHaveAttribute("aria-current", "page");
    await expect(sidebar.getByRole("link", { name: /Home/ })).not.toHaveAttribute("aria-current", "page");

    await page.goBack();
    await expect(page).toHaveURL(/\/$/);
    await expect(page.getByRole("heading", { level: 1, name: "Últimas anotações" })).toBeVisible();
    expect(await shellKept(page)).toBe(true);
  });

  test("navegar para uma subseção da conta abre o grupo correspondente", async ({ page }) => {
    await openAppPage(page, "/");

    await page.locator('[data-sidebar-group="security"]').click();
    await page.locator("#sidebarSecuritySubnav").getByRole("link", { name: "Histórico de sessões" }).click();
    await expect(page).toHaveURL(/\/me\/security\/active-sessions$/);
    await expect(page.locator('[data-sidebar-group="security"]')).toHaveAttribute("aria-expanded", "true");
    await expect(page.locator("#sidebarSecuritySubnav").getByRole("link", { name: "Histórico de sessões" })).toHaveAttribute("aria-current", "page");
    expect(await shellKept(page)).toBe(true);
  });

  test("no celular a sidebar vira um drawer com botão de abrir, fechar e Escape", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await openAppPage(page, "/");

    const open = page.locator("[data-sidebar-open]");
    await expect(open).toBeVisible();
    await open.click();
    await expect(page.locator("body")).toHaveClass(/sidebar-open/);
    await expect(open).toHaveAttribute("aria-expanded", "true");
    await expect(page.locator("#app-sidebar [data-sidebar-close]")).toBeFocused();

    await page.keyboard.press("Escape");
    await expect(page.locator("body")).not.toHaveClass(/sidebar-open/);
    await expect(open).toHaveAttribute("aria-expanded", "false");

    await open.click();
    await expect(page.locator("body")).toHaveClass(/sidebar-open/);
    // O drawer ocupa a esquerda; o fundo escurecido que fecha ao toque é o que sobra à direita dele, antes da barra de rolagem.
    const drawer = await page.locator("#app-sidebar").boundingBox();
    const backdrop = await page.locator("#app-sidebar-backdrop").boundingBox();
    expect(drawer && backdrop && drawer.width < backdrop.width).toBe(true);
    await page.mouse.click(((drawer?.width ?? 0) + (backdrop?.width ?? 0)) / 2, 700);
    await expect(page.locator("body")).not.toHaveClass(/sidebar-open/);
  });

  test("alternar tema na sidebar troca a classe dark e persiste", async ({ page }) => {
    await openAppPage(page, "/");

    const html = page.locator("html");
    const toggle = page.locator("#app-sidebar [data-theme-toggle]");
    const wasDark = await html.evaluate((el) => el.classList.contains("dark"));

    await toggle.click();
    await expect(html).toHaveClass(wasDark ? /^(?!.*\bdark\b)/ : /\bdark\b/);
    expect(await page.evaluate(() => localStorage.getItem("qn-theme"))).toBe(wasDark ? "light" : "dark");
    await expect(toggle).toContainText(wasDark ? "Tema escuro" : "Tema claro");
  });

  test("sair envia o formulário com navegação completa", async ({ page }) => {
    await openAppPage(page, "/");

    await page.locator('#app-sidebar form[action="/user/signout"] button').click();
    await expect(page).toHaveURL(/\/user\/signin$/);
    await expect(page.getByRole("heading", { name: "Iniciar sessão" })).toBeVisible();
    expect(await page.evaluate(() => window.__shellMarker)).toBeUndefined();
  });

  test("rodapé autenticado tem o link de segurança", async ({ page }) => {
    await openAppPage(page, "/");
    const footer = page.locator("footer.site-footer");
    await expect(footer.getByRole("link", { name: "Segurança" })).toHaveAttribute("href", "/me/security/two-step-verification");
    await expect(footer.getByRole("link", { name: "Termos" })).toHaveAttribute("href", "/terms");
  });
});
