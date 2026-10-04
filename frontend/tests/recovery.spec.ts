import { test, expect, type Page } from "@playwright/test";

const key = "infradex-workspace-v1";
async function enableRecovery(page: Page) {
  await page.getByRole("button", { name: "Settings", exact: true }).click();
  await page
    .getByRole("checkbox", { name: "Restore query drafts on startup" })
    .check();
  await page.getByRole("button", { name: "Close dialog" }).click();
}

test("workspace switches preserve drafts without enabling disk recovery", async ({
  page,
}) => {
  await page.goto("/");
  const editor = page.getByRole("textbox", { name: "Query editor" });
  await editor.fill("SELECT 'unsaved draft';");
  await page
    .getByRole("button", { name: "Switch workspace", exact: true })
    .click();
  await expect(
    page.getByText("Personal workspace", { exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Switch workspace", exact: true })
    .click();
  await expect(editor).toContainText("unsaved draft");
  expect(await page.evaluate((k) => localStorage.getItem(k), key)).toBeNull();
});

test("opt-in recovery restores active drafts and scopes without results or automatic execution", async ({
  page,
}) => {
  await page.goto("/");
  await expect(page.getByText("Olivia Rhye", { exact: true })).toBeVisible();
  await enableRecovery(page);
  await page
    .getByRole("textbox", { name: "Query editor" })
    .fill("SELECT 'recovered draft';");
  await page.getByRole("button", { name: "New query tab" }).click();
  await page
    .getByRole("textbox", { name: "Query editor" })
    .fill("SELECT 'active second draft';");
  await page.reload(); // Flush pending debounce on unload.
  await expect(
    page.getByRole("textbox", { name: "Query editor" }),
  ).toContainText("active second draft");
  await expect(page.locator("tbody tr")).toHaveCount(0);
  const stored = await page.evaluate(
    (k) => JSON.parse(localStorage.getItem(k)!),
    key,
  );
  expect(stored.workspaces.demo.tabs).toHaveLength(2);
  expect(stored.workspaces.demo.tabs[0].database).toBe("commerce");
  expect(JSON.stringify(stored)).not.toContain("Olivia");
  expect(stored.workspaces.demo.tabs[0]).not.toHaveProperty("result");
  await page.getByRole("tab", { name: "orders", exact: false }).click();
  await expect(
    page.getByRole("textbox", { name: "Query editor" }),
  ).toContainText("recovered draft");
  await page.getByRole("button", { name: "Close Query 2" }).click();
  await page.reload();
  await expect(page.getByRole("tab")).toHaveCount(1);
  await page.getByRole("button", { name: "Settings", exact: true }).click();
  await page
    .getByRole("checkbox", { name: "Restore query drafts on startup" })
    .uncheck();
  expect(await page.evaluate((k) => localStorage.getItem(k), key)).toBeNull();
  await page.getByRole("button", { name: "Close dialog" }).click();
  await expect(
    page.getByRole("textbox", { name: "Query editor" }),
  ).toContainText("recovered draft");
});

test("invalid recovery data stays intact and storage failures remain visible", async ({
  page,
}) => {
  await page.addInitScript(
    (k) => localStorage.setItem(k, '{"version":99}'),
    key,
  );
  await page.goto("/");
  await expect(page.getByRole("alert")).toContainText("could not be restored");
  expect(await page.evaluate((k) => localStorage.getItem(k), key)).toBe(
    '{"version":99}',
  );
  await page.evaluate(() => {
    Storage.prototype.setItem = () => {
      throw new DOMException("full", "QuotaExceededError");
    };
  });
  await enableRecovery(page);
  await expect(page.getByRole("alert")).toContainText("could not be saved");
  await page
    .getByRole("textbox", { name: "Query editor" })
    .fill("SELECT 'still editable';");
  await expect(
    page.getByRole("textbox", { name: "Query editor" }),
  ).toContainText("still editable");
  await expect(page.getByRole("alert")).toContainText("could not be saved");
});

test("personal recovery preserves context and drops deleted connections after metadata loads", async ({
  page,
}) => {
  await page.addInitScript((k) => {
    const c = {
      id: "live-cluster",
      kind: "k8s",
      name: "Saved cluster",
      environment: "Development",
      database: "default",
      context: "new-context",
    };
    (window as any).__executions = 0;
    (window as any).go = {
      main: {
        App: {
          ListConnections: async () => {
            await new Promise((r) => setTimeout(r, 300));
            (window as any).__ready = true;
            return [c];
          },
          ListDatabases: async () => ["default", "team"],
          BrowseResources: async () => ({
            resources: [],
            hasMore: false,
            nextCursor: "",
            message: "",
          }),
          ExecuteInContext: async (
            _id: string,
            database: string,
            context: string,
          ) => {
            (window as any).__executions++;
            (window as any).__scope = { database, context };
            return {
              columns: [],
              rows: [],
              duration: 1,
              truncated: false,
              message: "",
            };
          },
        },
      },
    };
    if (!localStorage.getItem(k))
      localStorage.setItem(
        k,
        JSON.stringify({
          version: 1,
          enabled: true,
          demo: false,
          workspaces: {
            demo: { tabs: [], activeId: "", selected: "" },
            personal: {
              activeId: "kept",
              selected: c.id,
              tabs: [
                {
                  id: "kept",
                  connectionId: c.id,
                  title: "Pods draft",
                  query: "get pods",
                  database: "team",
                  context: "original-context",
                },
                {
                  id: "removed",
                  connectionId: "deleted",
                  title: "Deleted draft",
                  query: "SELECT 1",
                },
              ],
            },
          },
        }),
      );
  }, key);
  await page.goto("/");
  await expect(page.getByRole("tab", { name: "Pods draft" })).toBeVisible();
  await expect(page.getByRole("tab")).toHaveCount(1);
  expect(await page.evaluate(() => (window as any).__executions)).toBe(0);
  await page.getByRole("button", { name: "Run query" }).first().click();
  await expect
    .poll(() => page.evaluate(() => (window as any).__scope))
    .toEqual({ database: "team", context: "original-context" });
  await page
    .getByRole("button", { name: "Switch workspace", exact: true })
    .click();
  await page.reload();
  await expect(
    page.getByRole("button", { name: "Demo workspace Sample" }),
  ).toBeVisible();
  await expect
    .poll(() => page.evaluate(() => (window as any).__ready))
    .toBe(true);
  await page
    .getByRole("button", { name: "Switch workspace", exact: true })
    .click();
  await expect(page.getByRole("tab", { name: "Pods draft" })).toBeVisible();
});

test("recovery settings remain usable at the minimum desktop size", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1000, height: 680 });
  await page.goto("/");
  await page.getByRole("button", { name: "Settings", exact: true }).click();
  await expect(
    page.getByRole("checkbox", { name: "Restore query drafts on startup" }),
  ).toBeVisible();
  await page.screenshot({
    path: "../artifacts/workspace-recovery-settings.png",
  });
  await page
    .getByRole("button", { name: "Save provider" })
    .scrollIntoViewIfNeeded();
  await expect(
    page.getByRole("button", { name: "Save provider" }),
  ).toBeInViewport();
});
