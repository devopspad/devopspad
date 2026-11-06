import { test, expect, openWorkbench } from "./fixtures/workspace";
test.beforeEach(async ({ page }) => {
  await page.goto("/");
  await openWorkbench(page);
  await expect(page.getByText("Olivia Rhye", { exact: true })).toBeVisible();
});

test("database navigation preserves each query tab scope", async ({ page }) => {
  const picker = page.getByRole("combobox", {
    name: "Database for Commerce database",
  });
  await picker.selectOption("sandbox");
  await page.getByRole("button", { name: "experiments", exact: true }).click();
  await page.getByRole("button", { name: "Run query" }).first().click();
  await expect(page.locator("tbody")).toContainText("Checkout redesign");
  await expect(page.locator(".query-toolbar")).toContainText("sandbox");
  await page.getByRole("tab", { name: "orders Close orders" }).click();
  await expect(picker).toHaveValue("commerce");
  await expect(page.locator("tbody")).toContainText("Olivia Rhye");
  await page
    .getByRole("tab", { name: "experiments Close experiments" })
    .click();
  await expect(picker).toHaveValue("sandbox");
  await expect(page.locator("tbody")).toContainText("Checkout redesign");
});

test("table inspector exposes columns, indexes and native DDL", async ({
  page,
}) => {
  await page.getByRole("button", { name: "Structure", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "orders", exact: true });
  await expect(
    dialog.getByRole("cell", { name: "email", exact: true }),
  ).toBeVisible();
  await dialog.getByRole("tab", { name: "Indexes", exact: true }).click();
  await expect(
    dialog.getByRole("cell", { name: "idx_orders_status", exact: true }),
  ).toBeVisible();
  await dialog.getByRole("tab", { name: "DDL", exact: true }).click();
  await expect(dialog.locator("pre")).toContainText("CREATE TABLE orders");
  await expect(dialog).not.toContainText("SQLite demo");
  await dialog.getByRole("button", { name: "Open query", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(
    page.getByRole("tab", { name: "orders Close orders" }),
  ).toHaveCount(1);
});

test("Redis key pages can be loaded, filtered and inspected", async ({
  page,
}) => {
  await page
    .getByRole("button", { name: "Session cache", exact: false })
    .click();
  await page.getByRole("button", { name: "Load more keys" }).click();
  await expect(
    page.getByRole("button", { name: "rate-limit:api", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Load more keys" }),
  ).toHaveCount(0);
  await page
    .getByRole("textbox", { name: "Key pattern for Session cache" })
    .fill("rate-*");
  await page.getByRole("button", { name: "Apply key pattern" }).click();
  await expect(page.locator(".resource-entry")).toHaveCount(1);
  await page
    .getByRole("button", { name: "rate-limit:api", exact: true })
    .hover();
  await page
    .getByRole("button", { name: "Inspect rate-limit:api", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "rate-limit:api",
    exact: true,
  });
  await expect(
    dialog.getByRole("cell", { name: "42", exact: true }),
  ).toBeVisible();
  await expect(dialog).toContainText("No expiration (sample)");
});

test("engine capability dialog clearly distinguishes supported operations", async ({
  page,
}) => {
  await page.getByRole("button", { name: "Connections 5" }).click();
  const card = page
    .locator(".connection-card")
    .filter({ has: page.getByRole("heading", { name: "Event pipeline" }) });
  await card.getByRole("button", { name: "Topics · Topic name" }).click();
  const dialog = page.getByRole("dialog", { name: "Kafka capabilities" });
  await expect(dialog).toContainText("Partition leaders");
  await expect(dialog).toContainText(
    "Messages, consumer groups and SCRAM authentication are not available yet.",
  );
});
