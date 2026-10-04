import { test, expect } from "@playwright/test";

test("reference desktop layout places the assistant left of Explorer and top-level query tabs", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1920, height: 1195 });
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/");
  await expect(page.getByText("Olivia Rhye", { exact: true })).toBeVisible();
  const rail = await page.locator(".app-rail").boundingBox();
  const assistant = await page.locator(".assistant").boundingBox();
  const explorer = await page.locator(".sidebar").boundingBox();
  const main = await page.locator(".main-shell").boundingBox();
  expect(assistant!.x).toBe(rail!.width);
  expect(explorer!.x).toBe(assistant!.x + assistant!.width);
  expect(main!.x).toBe(explorer!.x + explorer!.width);
  await expect(
    page.locator(".topbar").getByRole("tab", { name: "orders Close orders" }),
  ).toBeVisible();
  await page.screenshot({
    path: "../artifacts/desktop-reference-workbench.png",
  });
  await page
    .locator(".assistant")
    .screenshot({ path: "../artifacts/desktop-reference-assistant.png" });
  await page
    .locator(".main-shell")
    .screenshot({ path: "../artifacts/desktop-reference-editor.png" });
  await page
    .getByRole("button", { name: "Connections 7", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Your connections" }),
  ).toBeVisible();
  await page.screenshot({
    path: "../artifacts/desktop-reference-connections.png",
  });
  expect(errors).toEqual([]);
});

test("assistant collapse preserves query content and frees workspace width", async ({
  page,
}) => {
  await page.goto("/");
  const editor = page.getByRole("textbox", { name: "Query editor" });
  await editor.fill("SELECT 42 AS answer;");
  const before = (await page.locator(".main-shell").boundingBox())!.width;
  await page
    .getByRole("button", { name: "Toggle AI panel", exact: true })
    .click();
  await expect(page.locator(".assistant")).toHaveCount(0);
  expect(
    (await page.locator(".main-shell").boundingBox())!.width,
  ).toBeGreaterThan(before + 250);
  await expect(editor).toHaveText("SELECT 42 AS answer;");
  await page.getByRole("button", { name: "Run query" }).first().click();
  await expect(page.locator("tbody")).toContainText("42");
  await page
    .getByRole("button", { name: "Toggle AI panel", exact: true })
    .click();
  await expect(page.locator(".assistant")).toBeVisible();
});

test("desktop reference layout at working and minimum window sizes", async ({
  page,
}) => {
  await page.goto("/");
  await expect(page.getByText("Olivia Rhye", { exact: true })).toBeVisible();
  await page.screenshot({ path: "../artifacts/desktop-reference-1480.png" });
  await page
    .getByRole("button", { name: "New connection", exact: true })
    .first()
    .click();
  await expect(
    page.getByRole("dialog", { name: "New connection" }),
  ).toBeVisible();
  await page.screenshot({ path: "../artifacts/desktop-reference-dialog.png" });
  await page.getByRole("button", { name: "Close dialog", exact: true }).click();
  await page.setViewportSize({ width: 1000, height: 680 });
  const main = (await page.locator(".main-shell").boundingBox())!;
  const run = (await page
    .getByRole("button", { name: "Run query" })
    .first()
    .boundingBox())!;
  expect(main.x + main.width).toBeLessThanOrEqual(1000);
  expect(run.x + run.width).toBeLessThanOrEqual(1000);
  for (const locator of [
    page.getByRole("button", { name: "Send message", exact: true }),
    page.getByRole("button", { name: "Settings", exact: true }),
    page.locator(".statusbar"),
  ]) {
    const box = (await locator.boundingBox())!;
    expect(box.y + box.height).toBeLessThanOrEqual(680);
  }
  await page.screenshot({ path: "../artifacts/desktop-reference-1000.png" });
});
