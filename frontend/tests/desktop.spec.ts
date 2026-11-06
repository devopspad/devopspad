import { test, expect } from "@playwright/test";

test("desktop bridge: save, test, query, cancel, AI, and delete a connection", async ({
  page,
}) => {
  await page.addInitScript(() => {
    const calls: { method: string; value: unknown }[] = [];
    (window as any).__calls = calls;
    let connections: any[] = [];
    let ai = {
      endpoint: "http://localhost:11434/v1",
      model: "local-model",
      hasKey: false,
    };
    (window as any).go = {
      main: {
        App: {
          ListConnections: async () => connections,
          SaveConnection: async (c: any) => {
            calls.push({ method: "save", value: c });
            const saved = {
              ...c,
              id: "live-mysql",
              password: "",
              hasPassword: !!c.password,
            };
            connections = [saved];
            return saved;
          },
          TestConnection: async (c: any) => {
            calls.push({ method: "test", value: c });
            return 12;
          },
          DeleteConnection: async (id: string) => {
            calls.push({ method: "delete", value: id });
            connections = [];
          },
          Browse: async () => [
            { name: "widgets", kind: "table", query: "SELECT * FROM widgets;" },
          ],
          Execute: async (_id: string, query: string, requestID: string) => {
            calls.push({ method: "execute", value: { query, requestID } });
            await new Promise((resolve) => setTimeout(resolve, 500));
            return {
              columns: ["name"],
              rows: [["Live widget"]],
              duration: 20,
              truncated: false,
              message: "",
            };
          },
          CancelQuery: async (id: string) => {
            calls.push({ method: "cancel", value: id });
          },
          GetAISettings: async () => ai,
          SaveAISettings: async (c: any) => {
            calls.push({ method: "settings", value: c });
            ai = { ...c, apiKey: "", hasKey: true };
          },
          AskAI: async (r: any) => {
            calls.push({ method: "ai", value: r });
            return "Here is a suggestion.\n```sql\nSELECT name FROM widgets;\n```";
          },
        },
      },
    };
    const bridge = (window as any).go.main.App;
    bridge.ListDatabases = async () => ["test", "other"];
    bridge.BrowseResources = async (id: string) => ({
      resources: await bridge.Browse(id),
      nextCursor: "",
      hasMore: false,
      message: "",
    });
    bridge.ExecuteScoped = async (
      id: string,
      database: string,
      query: string,
      request: string,
    ) => bridge.Execute(id, query, request);
  });
  await page.goto("/");
  await page
    .getByRole("button", { name: "New connection", exact: true })
    .first()
    .click();
  await page
    .getByRole("textbox", { name: "Connection name" })
    .fill("Local MySQL");
  await page.getByRole("textbox", { name: "Username" }).fill("reader");
  await page.getByLabel("Password", { exact: true }).fill("secret-value");
  await page.getByRole("button", { name: "Test connection" }).click();
  await expect(page.getByRole("status")).toContainText(
    "Connection verified in 12 ms",
  );
  await page.getByRole("button", { name: "Save connection" }).click();
  await expect(
    page.getByRole("heading", { name: "Local MySQL" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Open workspace" }).click();
  await page.getByRole("button", { name: "widgets", exact: true }).click();
  await page.getByRole("button", { name: "Run query" }).first().click();
  await expect(page.getByText("Live widget", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Run query" }).first().click();
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("Query cancelled.");
  await page.waitForTimeout(600);
  await expect(page.getByRole("alert")).toContainText("Query cancelled.");
  await page
    .getByRole("textbox", { name: "Message AI assistant" })
    .fill("Read widgets");
  await page.getByRole("button", { name: "Send message" }).click();
  await page.getByRole("button", { name: "Insert into editor" }).click();
  await expect(
    page.getByRole("textbox", { name: "Query editor" }),
  ).toContainText("SELECT name FROM widgets;");
  expect(
    await page.evaluate(
      () =>
        (window as any).__calls.find((c: any) => c.method === "ai").value
          .schema,
    ),
  ).toEqual([]);
  await page.getByRole("button", { name: "AI provider settings" }).click();
  await page
    .getByRole("textbox", { name: "Model", exact: true })
    .fill("new-model");
  await page.getByRole("button", { name: "Save provider" }).click();
  await page.getByRole("button", { name: "Connections 1" }).click();
  await page.getByRole("button", { name: "Delete Local MySQL" }).click();
  await page
    .getByRole("button", { name: "Delete connection", exact: true })
    .click();
  await expect(page.locator(".connection-card")).toHaveCount(0);
  const methods = await page.evaluate(() =>
    (window as any).__calls.map((c: any) => c.method),
  );
  for (const method of [
    "save",
    "test",
    "execute",
    "cancel",
    "ai",
    "settings",
    "delete",
  ])
    expect(methods).toContain(method);
});

test("slow resource responses cannot overwrite a newer database selection", async ({
  page,
}) => {
  await page.addInitScript(() => {
    const c = {
      id: "live-scope",
      name: "Scope fixture",
      kind: "mysql",
      host: "localhost",
      port: 3306,
      username: "reader",
      database: "commerce",
      environment: "Development",
      tls: false,
      kubeconfig: "",
      context: "",
      hasPassword: false,
    };
    (window as any).go = {
      main: {
        App: {
          ListConnections: async () => [c],
          ListDatabases: async () => ["commerce", "slow", "fast"],
          BrowseResources: async (
            _id: string,
            request: { database: string },
          ) => {
            if ((window as any).__fail)
              throw new Error("Fixture connection unavailable");
            if (request.database === "slow")
              await new Promise((r) => setTimeout(r, 350));
            return {
              resources: [
                {
                  name: request.database + "_table",
                  kind: "table",
                  query: "SELECT * FROM " + request.database + "_table",
                },
              ],
              nextCursor: "",
              hasMore: false,
              message: "",
            };
          },
          ExecuteScoped: async (_id: string, database: string) => {
            (window as any).__queryDatabase = database;
            return {
              columns: ["database"],
              rows: [[database]],
              duration: 1,
              truncated: false,
              message: "",
            };
          },
        },
      },
    };
  });
  await page.goto("/");
  const picker = page.getByRole("combobox", {
    name: "Database for Scope fixture",
  });
  await picker.selectOption("slow");
  await picker.selectOption("fast");
  await expect(
    page.getByRole("button", { name: "fast_table", exact: true }),
  ).toBeVisible();
  await page.waitForTimeout(450);
  await expect(
    page.getByRole("button", { name: "slow_table", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "fast_table", exact: true }).click();
  await picker.selectOption("commerce");
  await page.getByRole("button", { name: "Run query" }).first().click();
  expect(await page.evaluate(() => (window as any).__queryDatabase)).toBe(
    "fast",
  );
  await page.evaluate(() => {
    (window as any).__fail = true;
  });
  await page.getByRole("button", { name: "Refresh resources" }).click();
  await expect(page.locator(".tree-error")).toContainText(
    "Fixture connection unavailable",
  );
  await expect(page.getByTitle("Last request failed")).toBeVisible();
});
