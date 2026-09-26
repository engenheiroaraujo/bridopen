const { test, expect } = require("@playwright/test");
const { openAppPage, shellKept } = require("./helpers");

test("reading pane selects real content and retains routes for edit and attachments", async ({ page }) => {
  const errors = await openAppPage(page, "/");
  await expect(page.locator(".bear-document h2")).toHaveText("Reunião de planejamento");
  const link = page.locator('[data-note-select="2"]');
  await expect(link).toHaveAttribute("href", "/note/2");
  await link.click();
  await expect(page).toHaveURL(/\/$/);
  await expect(page.locator(".bear-document h2")).toHaveText("Lista de compras");
  await expect(page.locator(".bear-document-content")).toContainText("Café");
  await expect(link).toHaveAttribute("aria-current", "true");
  await expect(page.locator(".bear-open-note")).toHaveAttribute("href", "/note/2");
  await page.locator(".bear-document").getByRole("link", {name:"Editar",exact:true}).click();
  await expect(page).toHaveURL(/\/note\/2\/edit$/);
  expect(await shellKept(page)).toBe(true);
  expect(errors).toEqual([]);
});

test("mobile switches between list and reader and restores keyboard focus", async ({ page }) => {
  await page.setViewportSize({width:390,height:844});
  await openAppPage(page, "/");
  await expect(page.locator(".bear-reading")).not.toBeVisible();
  await page.locator('[data-note-select="2"]').click();
  await expect(page.locator(".bear-reading")).toBeVisible();
  await expect(page.locator(".bear-list")).not.toBeVisible();
  await expect(page.locator(".bear-document h2")).toBeFocused();
  await page.locator(".bear-back").click();
  await expect(page.locator('[data-note-select="2"]')).toBeFocused();
  await expect(page.locator(".bear-list")).toBeVisible();
  expect(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth)).toBe(false);
});

test("reader remounts after filtered partial navigation and treats text as text", async ({ page }) => {
  await openAppPage(page, "/");
  await page.locator("#note-search-input").fill("compras");
  await expect(page).toHaveURL(/q=compras/);
  await expect(page.locator(".bear-document h2")).toHaveText("Lista de compras");
  await page.evaluate(()=>{
    const template=document.querySelector('template[data-note-document="2"]');
    template.content.querySelector('.bear-document-content').textContent='<img src=x onerror=alert(1)>';
  });
  await page.locator('[data-note-select="2"]').click();
  await expect(page.locator(".bear-document-content")).toHaveText('<img src=x onerror=alert(1)>');
  await expect(page.locator(".bear-document-content img")).toHaveCount(0);
});
