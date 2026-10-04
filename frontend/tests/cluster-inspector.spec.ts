import { test, expect } from "@playwright/test";

test("cluster demo exposes overview, UID events, container logs and JSON", async ({
  page,
}) => {
  await page.goto("/");
  await page
    .getByRole("button", { name: "US East cluster", exact: false })
    .click();
  await page.getByRole("button", { name: "pods", exact: true }).click();
  await page.getByRole("button", { name: "Run query" }).first().click();
  await page
    .getByRole("button", {
      name: "Open details for api-7d9f8b6c4-x2k9p",
      exact: true,
    })
    .click();
  const dialog = page.getByRole("dialog", { name: "api-7d9f8b6c4-x2k9p" });
  await expect(
    dialog.getByRole("region", { name: "Resource overview" }),
  ).toContainText("Running (sample)");
  await expect(dialog).toContainText("Init");
  await page.screenshot({ path: "../artifacts/cluster-overview.png" });
  await dialog.getByRole("tab", { name: "Events", exact: true }).click();
  await expect(
    dialog.getByRole("region", { name: "Resource events" }),
  ).toContainText("Started container app");
  await dialog.getByRole("tab", { name: "Logs", exact: true }).click();
  await expect(dialog.getByLabel("Log output")).toContainText(
    "app: starting service",
  );
  await dialog
    .getByRole("combobox", { name: "Log container" })
    .selectOption("setup");
  await expect(dialog.getByLabel("Log output")).toContainText(
    "setup: starting service",
  );
  await dialog.getByRole("checkbox", { name: "Previous instance" }).check();
  await expect(dialog.getByLabel("Log output")).toContainText(
    "Previous container",
  );
  await page.screenshot({ path: "../artifacts/pod-logs.png" });
  await dialog.getByRole("tab", { name: "JSON", exact: true }).click();
  await expect(dialog.getByLabel("Resource JSON")).toContainText(
    '"kind": "Pod"',
  );
  await dialog.getByRole("button", { name: "Done", exact: true }).click();
  await expect(dialog).toHaveCount(0);
});

test("cluster log responses preserve scope, reject stale UI responses and isolate errors", async ({
  page,
}) => {
  await page.addInitScript(() => {
    const calls: any[] = [];
    (window as any).__calls = calls;
    const c = {
      id: "cluster",
      name: "Fixture",
      kind: "k8s",
      host: "",
      port: 6443,
      username: "",
      database: "team",
      environment: "Development",
      tls: true,
      kubeconfig: "/fixture",
      context: "east",
      hasPassword: false,
    };
    (window as any).go = {
      main: {
        App: {
          ListConnections: async () => [c],
          ListDatabases: async () => ["team", "staging"],
          BrowseResources: async () => ({
            resources: [{ name: "pods", kind: "resource", query: "get pods" }],
            hasMore: false,
            nextCursor: "",
            message: "",
          }),
          ExecuteInContext: async () => ({
            columns: ["name", "namespace", "resource"],
            rows: [["api", "team", { metadata: { uid: "old-uid" } }]],
            resourceType: "pods",
            duration: 1,
            message: "",
            truncated: false,
          }),
          InspectClusterResource: async (id: string, ref: any) => {
            calls.push({ method: "detail", id, ref });
            return {
              ref,
              properties: [{ name: "Phase", value: "Running" }],
              labels: { app: "api" },
              definition: '{"kind":"Pod"}',
              containers: [
                {
                  name: "app",
                  kind: "Application",
                  image: "sample",
                  state: "running",
                  ready: true,
                  restarts: 0,
                },
                {
                  name: "init",
                  kind: "Init",
                  image: "sample",
                  state: "terminated",
                  ready: false,
                  restarts: 0,
                },
              ],
            };
          },
          ClusterEvents: async () => {
            throw new Error("Access denied: list events");
          },
          ReadPodLogs: async (id: string, request: any) => {
            calls.push({ method: "logs", id, request });
            if (request.previous)
              throw new Error("Previous terminated container was not found");
            if (request.container === "app") {
              await new Promise((resolve) => setTimeout(resolve, 500));
              (window as any).__late = true;
            }
            return {
              text: request.container + " output",
              truncated: request.container === "init",
            };
          },
        },
      },
    };
  });
  await page.goto("/");
  await page.locator(".workspace-switch").click();
  await page.getByRole("button", { name: "pods", exact: true }).click();
  await page.getByRole("button", { name: "Run query" }).first().click();
  await expect(
    page.getByRole("button", { name: "Open details for api", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("combobox", { name: "Namespace for Fixture" })
    .selectOption("staging");
  // Inspect the returned resource, even if the query text and browser scope changed.
  await page
    .getByRole("textbox", { name: "Query editor" })
    .fill("get configmaps staging");
  await page
    .getByRole("button", { name: "Open details for api", exact: true })
    .click();
  const dialog = page.getByRole("dialog", { name: "api", exact: true });
  await expect(
    dialog.getByRole("tab", { name: "Logs", exact: true }),
  ).toBeVisible();
  expect(
    await page.evaluate(() => (window as any).__calls[0].ref),
  ).toMatchObject({
    context: "east",
    namespace: "team",
    resource: "pods",
    uid: "old-uid",
  });
  await dialog.getByRole("tab", { name: "Events", exact: true }).click();
  await expect(dialog.getByRole("alert")).toContainText("Access denied");
  await dialog.getByRole("tab", { name: "Logs", exact: true }).click();
  await dialog
    .getByRole("combobox", { name: "Log container" })
    .selectOption("init");
  await expect(dialog.getByLabel("Log output")).toHaveText("init output");
  await expect
    .poll(() => page.evaluate(() => (window as any).__late))
    .toBe(true);
  await expect(dialog.getByLabel("Log output")).toHaveText("init output");
  await expect(dialog).toContainText("Log size limit reached");
  await dialog.getByRole("checkbox", { name: "Previous instance" }).check();
  await expect(dialog.getByRole("alert")).toContainText(
    "Previous terminated container",
  );
  await dialog.getByRole("tab", { name: "Overview", exact: true }).click();
  await expect(dialog).toContainText("Running");
  await expect(dialog.getByRole("alert")).toHaveCount(0);
});

test("pod inspector remains usable at the desktop minimum size", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1000, height: 680 });
  await page.goto("/");
  await page
    .getByRole("button", { name: "US East cluster", exact: false })
    .click();
  await page.getByRole("button", { name: "pods", exact: true }).click();
  await page.getByRole("button", { name: "Run query" }).first().click();
  await page
    .getByRole("button", {
      name: "Open details for api-7d9f8b6c4-x2k9p",
      exact: true,
    })
    .click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("tab", { name: "Logs", exact: true }).click();
  await expect(dialog.getByLabel("Log output")).toContainText(
    "starting service",
  );
  expect(await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(
    true,
  );
  await dialog
    .getByRole("button", { name: "Refresh logs", exact: true })
    .click();
  await dialog.getByRole("button", { name: "Done", exact: true }).click();
  await expect(dialog).toHaveCount(0);
});
