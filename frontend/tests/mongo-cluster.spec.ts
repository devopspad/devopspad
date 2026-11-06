import { test, expect } from "./fixtures/workspace";

test("MongoDB batches keep sort and projection and require rerun after edits", async ({
  page,
}) => {
  await page.goto("/");
  await page
    .getByRole("button", { name: "Product catalog", exact: false })
    .click();
  await page.getByRole("button", { name: "products", exact: true }).click();
  const editor = page.getByRole("textbox", { name: "Query editor" });
  const query = JSON.stringify({
    collection: "products",
    filter: {},
    projection: { name: 1, _id: 0 },
    sort: { price: -1 },
    limit: 1,
  });
  await editor.fill(query);
  await page.getByRole("button", { name: "Run query" }).first().click();
  await expect(page.locator("tbody")).toContainText("Team workspace");
  await expect(page.locator("tbody")).not.toContainText("prd_");
  await page.getByRole("button", { name: "Next batch", exact: true }).click();
  await expect(page.locator("tbody")).toContainText("Pro workspace");
  await expect(
    page.getByRole("button", { name: "Next batch", exact: true }),
  ).toBeDisabled();
  await expect(page.locator(".batch-navigation")).toContainText(
    "Batch offset 1",
  );
  await page
    .getByRole("button", { name: "Previous batch", exact: true })
    .click();
  await expect(page.locator("tbody")).toContainText("Team workspace");
  await page.screenshot({ path: "../artifacts/mongo-batches.png" });
  await editor.fill(query + " ");
  await expect(
    page.getByRole("button", { name: "Next batch", exact: true }),
  ).toBeDisabled();
  await page.getByRole("button", { name: "Run query" }).first().click();
  await expect(
    page.getByRole("button", { name: "Next batch", exact: true }),
  ).toBeEnabled();
});

test("cluster contexts load from kubeconfig; tabs keep their context and namespace", async ({
  page,
}) => {
  await page.addInitScript(() => {
    const calls: any[] = [];
    (window as any).__calls = calls;
    let c = {
      id: "cluster",
      name: "Fixture cluster",
      kind: "k8s",
      host: "",
      port: 6443,
      username: "",
      database: "team",
      environment: "Development",
      tls: true,
      kubeconfig: "/fixture/config",
      context: "east",
      hasPassword: false,
    };
    (window as any).go = {
      main: {
        App: {
          ListConnections: async () => [c],
          ReadKubeconfig: async () => ({
            currentContext: "east",
            contexts: [
              {
                name: "east",
                cluster: "east-cluster",
                namespace: "team",
                execAuth: false,
              },
              {
                name: "west",
                cluster: "west-cluster",
                namespace: "staging",
                execAuth: false,
              },
            ],
          }),
          SaveConnection: async (value: any) => {
            c = value;
            return c;
          },
          ListDatabases: async () => ["*", "team", "staging"],
          BrowseResources: async () => ({
            resources: [{ name: "pods", kind: "resource", query: "get pods" }],
            hasMore: false,
            nextCursor: "",
            message: "",
          }),
          ExecuteInContext: async (
            id: string,
            database: string,
            context: string,
            query: string,
          ) => {
            calls.push({ id, database, context, query });
            return {
              columns: ["name", "namespace"],
              rows: [[context + "-pod", database]],
              duration: 2,
              truncated: false,
              message: "",
            };
          },
        },
      },
    };
  });
  await page.goto("/#infra/resources");
  await page.getByRole("button", { name: "pods", exact: true }).click();
  await page
    .getByRole("combobox", { name: "Namespace for Fixture cluster" })
    .selectOption("staging");
  await page.getByRole("button", { name: "Run query" }).first().click();
  await expect(page.locator("tbody")).toContainText("east-pod");
  expect(
    await page.evaluate(() => (window as any).__calls.at(-1)),
  ).toMatchObject({ context: "east", database: "team" });
  await page.getByRole("button", { name: "Connections 1" }).click();
  await page.getByRole("button", { name: "Edit Fixture cluster" }).click();
  await expect(
    page
      .getByRole("combobox", { name: "Context", exact: true })
      .locator("option"),
  ).toHaveCount(3);
  await page
    .getByRole("combobox", { name: "Context", exact: true })
    .selectOption("west");
  await expect(
    page.getByRole("textbox", { name: "Namespace", exact: true }),
  ).toHaveValue("staging");
  await page.screenshot({ path: "../artifacts/cluster-contexts.png" });
  await page
    .getByRole("button", { name: "Save connection", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Open workspace", exact: false })
    .click();
  await page.getByRole("tab", { name: "pods Close pods", exact: true }).click();
  await page.getByRole("button", { name: "Run query" }).first().click();
  expect(
    await page.evaluate(() => (window as any).__calls.at(-1)),
  ).toMatchObject({ context: "east", database: "team" });
  await expect(page.locator(".query-toolbar")).toContainText("east / team");
});

test("MongoDB authentication database is saved through the desktop bridge", async ({
  page,
}) => {
  await page.addInitScript(() => {
    (window as any).go = {
      main: {
        App: {
          ListConnections: async () => [],
          SaveConnection: async (c: any) => {
            (window as any).__saved = c;
            return { ...c, id: "mongo" };
          },
          ListDatabases: async () => ["catalog"],
          BrowseResources: async () => ({
            resources: [],
            hasMore: false,
            nextCursor: "",
            message: "",
          }),
        },
      },
    };
  });
  await page.goto("/");
  await page
    .getByRole("button", { name: "New connection", exact: true })
    .first()
    .click();
  await page.getByRole("button", { name: "Mo MongoDB", exact: true }).click();
  await page.getByRole("textbox", { name: "Connection name" }).fill("Catalog");
  await page
    .getByRole("textbox", { name: "Database", exact: true })
    .fill("catalog");
  await page
    .getByRole("textbox", { name: "Authentication database", exact: false })
    .fill("accounts");
  await page
    .getByRole("button", { name: "Save connection", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(await page.evaluate(() => (window as any).__saved)).toMatchObject({
    kind: "mongo",
    authSource: "accounts",
    database: "catalog",
  });
});
