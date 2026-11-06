import { test, expect, openWorkbench } from "./fixtures/workspace";

test.beforeEach(async ({ page }) => {
  await page.goto("/");
  await openWorkbench(page);
  await expect(page.getByText("Olivia Rhye", { exact: true })).toBeVisible();
});

test("query execution, filtering, sorting, inspection, and CSV export", async ({
  page,
}) => {
  await page.getByRole("button", { name: "Run query" }).first().click();
  await expect(page.locator("tbody tr")).toHaveCount(12);
  await page.getByRole("textbox", { name: "Filter results" }).fill("olivia");
  await expect(page.locator("tbody tr")).toHaveCount(1);
  await page.getByText("Olivia Rhye", { exact: true }).dblclick();
  await expect(
    page.getByRole("dialog", { name: "Row inspector" }),
  ).toBeVisible();
  await expect(
    page.getByRole("dialog").getByText("olivia@acme.com", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Close dialog" }).click();
  await page.getByRole("textbox", { name: "Filter results" }).fill("");
  await page.getByRole("button", { name: "total", exact: false }).click();
  await expect(page.locator("tbody tr").first()).toContainText("$79.00");
  const downloadPromise = page.waitForEvent("download");
  await page.getByRole("button", { name: "Export CSV" }).click();
  const download = await downloadPromise;
  expect(download.suggestedFilename()).toBe("devopspad-results.csv");
  const stream = await download.createReadStream();
  let csv = "";
  for await (const chunk of stream!) csv += chunk;
  expect(csv).toContain('"customer"');
  expect(csv).toContain('"Olivia Rhye"');
});

test("AI provider response inserts a query and executes through the bridge", async ({
  page,
}) => {
  await page
    .getByRole("button", { name: "Summarize the available resources" })
    .click();
  await expect(
    page.getByText("Fixture provider response", { exact: false }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Insert into editor" }).click();
  await page.getByRole("button", { name: "Run query" }).first().click();
  await expect(page.locator("thead")).toContainText("revenue");
  await expect(page.locator("tbody tr")).toHaveCount(3);
  await page
    .getByRole("button", { name: "Query history", exact: true })
    .click();
  await expect(page.locator(".history-list")).toContainText("GROUP BY status");
});

test("saved queries persist and can be reopened or deleted", async ({
  page,
}) => {
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await page.getByRole("textbox", { name: "Query name" }).fill("Recent orders");
  await page.getByRole("button", { name: "Save query", exact: true }).click();
  await page.reload();
  await page
    .getByRole("button", { name: "Saved queries", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Recent orders" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Open in editor" }).click();
  await expect(page.getByRole("tab", { name: "Recent orders" })).toBeVisible();
  await page
    .getByRole("button", { name: "Saved queries", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Delete saved query Recent orders" })
    .click();
  await expect(
    page.getByRole("heading", { name: "Good queries deserve a home." }),
  ).toBeVisible();
});

test("command palette opens another connection and tabs retain independent content", async ({
  page,
}) => {
  await page.keyboard.press("Meta+k");
  await page.getByRole("textbox", { name: "Search workspace" }).fill("session");
  await page.getByRole("button", { name: "Session cache Redis" }).click();
  await expect(page.locator(".connection-heading")).toContainText(
    "Session cache",
  );
  await page.getByRole("button", { name: "Run query" }).first().click();
  await expect(page.locator("tbody")).toContainText("session:user:1048");
  await page.getByRole("tab", { name: "orders", exact: false }).click();
  await expect(page.locator("tbody")).toContainText("Olivia Rhye");
  await expect(
    page.getByRole("textbox", { name: "Query editor" }),
  ).toContainText("FROM orders");
});

test("middleware engines can be explored through real UI interactions", async ({
  page,
}) => {
  await page.getByRole("button", { name: "Connections 5" }).click();
  await expect(page.locator(".connection-card")).toHaveCount(5);
  for (const name of [
    "Product catalog",
    "Event pipeline",
    "Product analytics",
  ]) {
    await page
      .locator(".connection-card")
      .filter({ has: page.getByRole("heading", { name, exact: true }) })
      .getByRole("button", { name: "Open workspace" })
      .click();
    if (name === "Product catalog") {
      await page.getByRole("button", { name: "products", exact: true }).click();
    }
    await page.getByRole("button", { name: "Run query" }).first().click();
    await expect(page.locator("tbody tr").first()).toBeVisible();
    await expect(page.locator(".query-error")).toHaveCount(0);
    await page.getByRole("button", { name: "Connections 5" }).click();
  }
});

test("connection dialog supports all engines through the desktop bridge", async ({
  page,
}) => {
  await page
    .getByRole("button", { name: "New connection", exact: true })
    .first()
    .click();
  const dialog = page.getByRole("dialog", { name: "New connection" });
  await expect(
    dialog.getByRole("button", { name: "Test connection" }),
  ).toBeEnabled();
  await dialog
    .getByRole("button", { name: "Kubernetes", exact: false })
    .click();
  await expect(
    dialog.getByRole("textbox", { name: "Kubeconfig file" }),
  ).toBeVisible();
  await expect(dialog.getByRole("textbox", { name: "Namespace" })).toHaveValue(
    "default",
  );
  await dialog.getByRole("button", { name: "Redis", exact: false }).click();
  await expect(dialog.getByRole("spinbutton", { name: "Port" })).toHaveValue(
    "6379",
  );
  await expect(
    dialog.getByRole("textbox", { name: "Database", exact: true }),
  ).toHaveValue("0");
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
});

test("SQL errors are explicit and read-only fixture rejects writes", async ({
  page,
}) => {
  const editor = page.getByRole("textbox", { name: "Query editor" });
  await editor.fill("SELECT * FROM missing_table;");
  await page.getByRole("button", { name: "Run query" }).first().click();
  await expect(page.getByRole("alert")).toContainText("no such table");
  await editor.fill("DELETE FROM orders;");
  await page.getByRole("button", { name: "Run query" }).first().click();
  await expect(page.getByRole("alert")).toContainText("read-only");
  await editor.fill("SELECT COUNT(*) AS count FROM orders;");
  await page.getByRole("button", { name: "Run query" }).first().click();
  await expect(page.locator("tbody tr")).toContainText("12");
});

test("desktop minimum size keeps core controls available", async ({ page }) => {
  await page.setViewportSize({ width: 1024, height: 700 });
  await expect(page.getByRole("button", { name: "Run query" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Export CSV" })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    1024,
  );
  await page.getByRole("button", { name: "Close AI assistant" }).click();
  await expect(page.locator(".assistant")).toHaveCount(0);
  await page.getByRole("button", { name: "AI assistant", exact: true }).click();
  await expect(page.locator(".assistant")).toBeVisible();
});
