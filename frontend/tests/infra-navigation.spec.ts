import { test, expect } from "@playwright/test";
import { installWorkspace, openWorkbench } from "./fixtures/workspace";

async function install(page: import("@playwright/test").Page) {
  await page.addInitScript(() => {
    const w = window as any;
    w.__nodeCalls = [];
    const connections = [
      {
        id: "pro-a",
        name: "K3s production",
        kind: "k3s",
        environment: "Production",
        context: "east",
      },
      {
        id: "pro-b",
        name: "K3s backup",
        kind: "k3s",
        environment: "Production",
        context: "west",
      },
      {
        id: "dev",
        name: "K3s development",
        kind: "k3s",
        environment: "Development",
        context: "dev",
      },
      {
        id: "db",
        name: "MySQL production",
        kind: "mysql",
        environment: "Production",
        context: "",
      },
    ].map((c) => ({
      ...c,
      database: "team",
      host: "localhost",
      port: 6443,
      username: "",
      kubeconfig: "fixture",
      tls: true,
      hasPassword: false,
    }));
    w.go = {
      main: {
        App: {
          ListConnections: async () => connections,
          GetOpsProject: async () => ({
            root: "/fixture/project",
            yaml: "",
            revision: "1",
            project: {
              name: "Fixture",
              environments: connections
                .filter((c) => c.kind === "k3s")
                .map((c) => ({
                  name: c.id,
                  connection: c.name,
                  namespace: "team",
                })),
              applications: [
                { name: "portal", selector: "app=portal" },
                { name: "api", selector: "app=api" },
              ],
            },
            artifacts: [],
            releases: [],
            runs: [],
          }),
          BrowseResources: async () => ({
            resources: [
              { name: "nodes", kind: "resource", query: "get nodes" },
            ],
            hasMore: false,
            nextCursor: "",
            message: "",
          }),
          ListDatabases: async () => ["team", "default"],
          CancelQuery: async () => {},
          ListWorkloads: async () => [
            {
              name: "api",
              kind: "Deployment",
              namespace: "team",
              uid: "w1",
              resource: "deployments",
              desired: 2,
              ready: 2,
              image: "api:1",
              updatedAt: "",
              extraImages: 0,
            },
          ],
          ExecuteInContext: async (
            id: string,
            namespace: string,
            context: string,
            query: string,
          ) => {
            w.__nodeCalls.push({ id, namespace, context, query });
            if (w.__hold === id)
              await new Promise<void>((resolve) => {
                w.__release = resolve;
              });
            if (w.__deny) throw new Error("Access denied: list nodes");
            return {
              columns: ["name", "namespace", "created", "resource"],
              rows: [0, 1, 2].map((index) => {
                const name = `${id}-node-${index + 1}`;
                return [
                  name,
                  "",
                  "",
                  {
                    metadata: {
                      name,
                      uid: `${id}-${index}`,
                      labels:
                        index === 0
                          ? { "node-role.kubernetes.io/control-plane": "true" }
                          : {},
                    },
                    spec: { unschedulable: index === 2 },
                    status: {
                      conditions: [
                        {
                          type: "Ready",
                          status: index === 2 ? "False" : "True",
                        },
                      ],
                      addresses: [
                        { type: "InternalIP", address: `10.0.0.${index + 1}` },
                      ],
                      capacity: { cpu: "8", memory: "16777216Ki" },
                      nodeInfo: {
                        osImage: "Ubuntu 24.04",
                        kubeletVersion: "v1.34.0+k3s1",
                        architecture: "arm64",
                      },
                    },
                  },
                ];
              }),
              duration: 1,
              truncated: Boolean(w.__truncated),
              message: "",
            };
          },
          InspectClusterResource: async (_id: string, ref: any) => {
            w.__inspected = ref;
            return {
              ref,
              properties: [{ name: "Name", value: ref.name }],
              labels: {},
              containers: [],
              definition: JSON.stringify({
                kind: "Node",
                metadata: { name: ref.name },
              }),
            };
          },
        },
      },
    };
  });
}

test("Infra groups environments and shows every K3s node with scoped inspection", async ({
  page,
}) => {
  await install(page);
  await page.goto("/#infra/resources");
  await expect(
    page.getByRole("button", { name: "pro-a-node-3", exact: true }),
  ).toBeVisible();
  await expect(page.getByText("3 nodes", { exact: true })).toBeVisible();
  await expect(
    page.getByRole("cell", { name: "control-plane", exact: true }),
  ).toBeVisible();
  await expect(page.getByText("16 GiB").first()).toBeVisible();
  await expect(page.getByText("NotReady", { exact: true })).toBeVisible();
  await expect(page.locator(".tree-connection")).toHaveCount(3);
  await page.getByLabel("Infra environment").selectOption("Production");
  await expect(page.locator(".tree-connection")).toHaveCount(2);
  await page.getByRole("button", { name: "K3s backup", exact: false }).click();
  await expect(
    page.getByRole("button", { name: "pro-b-node-1", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "pro-a-node-1", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "pro-b-node-1", exact: true }).click();
  await expect(
    page.getByRole("dialog", { name: "pro-b-node-1", exact: true }),
  ).toBeVisible();
  expect(await page.evaluate(() => (window as any).__inspected)).toMatchObject({
    context: "west",
    namespace: "",
    resource: "nodes",
    name: "pro-b-node-1",
    uid: "pro-b-0",
  });
  await page.getByRole("button", { name: "Done", exact: true }).click();
  await page.getByLabel("Search nodes").fill("10.0.0.2");
  await expect(page.locator(".infra-node-table tbody tr")).toHaveCount(1);
  await page.getByLabel("Search nodes").fill("");
  await page.getByRole("tab", { name: "Workloads", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "api", exact: true }),
  ).toBeVisible();
  await page.getByRole("tab", { name: "Nodes", exact: true }).click();
  await page.reload();
  await expect(page.getByLabel("Infra environment")).toHaveValue("Production");
  await expect(
    page.getByRole("button", { name: "pro-b-node-1", exact: true }),
  ).toBeVisible();
  expect(await page.evaluate(() => (window as any).__nodeCalls.at(-1))).toEqual(
    { id: "pro-b", namespace: "", context: "west", query: "get nodes" },
  );
  await page.screenshot({ path: "../artifacts/infra-nodes.png" });
});

test("node reads isolate pending requests, show permission failures, and support retry", async ({
  page,
}) => {
  await install(page);
  await page.goto("/#infra/resources");
  await expect(page.getByText("3 nodes", { exact: true })).toBeVisible();
  await page.evaluate(() => {
    (window as any).__hold = "pro-b";
  });
  await page.getByRole("button", { name: "K3s backup", exact: false }).click();
  await expect
    .poll(() => page.evaluate(() => typeof (window as any).__release))
    .toBe("function");
  await page.getByLabel("Infra environment").selectOption("Development");
  await expect(
    page.getByRole("button", { name: "dev-node-1", exact: true }),
  ).toBeVisible();
  await page.evaluate(() => (window as any).__release());
  await expect(
    page.getByRole("button", { name: "pro-b-node-1", exact: true }),
  ).toHaveCount(0);
  await page.evaluate(() => {
    (window as any).__deny = true;
  });
  await page.getByRole("button", { name: "Refresh nodes", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText(
    "Access denied: list nodes",
  );
  await expect(
    page.getByRole("button", { name: "dev-node-1", exact: true }),
  ).toHaveCount(0);
  await page.evaluate(() => {
    (window as any).__deny = false;
    (window as any).__truncated = true;
  });
  await page.getByRole("button", { name: "Refresh nodes", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("not every node in the cluster");
});

test("Infra is the application console and the rail has Data, Infra and Archive", async ({
  page,
}) => {
  await installWorkspace(page);
  await page.setViewportSize({ width: 1000, height: 680 });
  await page.goto("/");
  await openWorkbench(page);
  const editor = page.getByRole("textbox", { name: "Query editor" });
  await editor.fill("SELECT 42 AS retained;");
  const nav = () =>
    page.getByRole("group", { name: "Workspace navigation", exact: true });
  await expect(nav().getByRole("button")).toHaveText(["Data", "Infra", "Archive"]);
  await nav().getByRole("button", { name: "Infra", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Artifacts", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Application services", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Nodes", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Sharing & sync", exact: true }).click();
  await expect(
    nav().getByRole("button", { name: "Infra", exact: true }),
  ).toBeInViewport();
  await nav().getByRole("button", { name: "Data", exact: true }).click();
  await expect(editor).toHaveText("SELECT 42 AS retained;");
  await expect(
    page.getByRole("button", { name: "Settings", exact: true }),
  ).toBeInViewport();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
});

test("the unified Infra console lists nodes for its selected application environment", async ({
  page,
}, testInfo) => {
  await install(page);
  await page.goto("/#infra");
  await expect(
    page.getByRole("heading", { name: "Artifacts", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Nodes", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "pro-a-node-1", exact: true }),
  ).toBeVisible();
  const application = page.getByLabel("Application", { exact: true });
  await expect(application).toBeEnabled();
  await application.selectOption("api");
  await expect(application).toHaveValue("api");
  await page.getByRole("button", { name: "Artifacts", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Artifacts", exact: true }),
  ).toBeVisible();
  await expect(application).toHaveValue("api");
  await page.getByRole("button", { name: "Nodes", exact: true }).click();
  await expect(application).toHaveValue("api");
  await page.getByLabel("Environment", { exact: true }).selectOption("dev");
  await expect(
    page.getByRole("button", { name: "dev-node-1", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "pro-a-node-1", exact: true }),
  ).toHaveCount(0);
  await expect(application).toHaveValue("api");
  await page.screenshot({ path: testInfo.outputPath("infra-unified-nodes.png") });
});
