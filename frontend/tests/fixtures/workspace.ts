import { test as base, expect, type Page } from "@playwright/test";
import {
  fixtureConnections,
  fixtureDatabases,
  fixtureResources,
  fixtureExecute,
  fixtureInspect,
} from "./engine";
import { demoClusterDetails, demoClusterEvents, demoPodLogs } from "./cluster";

export async function installWorkspace(page: Page) {
  let connections = structuredClone(fixtureConnections);
  await page.exposeFunction(
    "__fixtureBridge",
    async (method: string, ...args: any[]) => {
      const c = connections.find((c) => c.id === args[0]);
      switch (method) {
        case "ListConnections":
          return connections;
        case "ListDatabases":
          return fixtureDatabases(c!.kind);
        case "BrowseResources": {
          const request = args[1];
          let items = fixtureResources(c!.kind, request.database);
          if (request.filter && request.filter !== "*")
            items = items.filter((r) =>
              r.name.startsWith(request.filter.replaceAll("*", "")),
            );
          const offset = Number(request.cursor || 0),
            count = c!.kind === "redis" ? 2 : items.length;
          return {
            resources: items.slice(offset, offset + count),
            hasMore: offset + count < items.length,
            nextCursor: String(offset + count),
            message: "",
          };
        }
        case "ExecuteScoped":
          return fixtureExecute(c!.kind, args[2], args[1]);
        case "ExecuteInContext":
          return fixtureExecute(c!.kind, args[3], args[1]);
        case "InspectResource":
          return fixtureInspect(c!.kind, args[1], args[2]);
        case "InspectClusterResource":
          return demoClusterDetails(args[1]);
        case "ClusterEvents":
          return demoClusterEvents();
        case "ReadPodLogs":
          return demoPodLogs(args[1]);
        case "CancelQuery":
          return;
        case "GetAISettings":
          return {
            endpoint: "https://fixture.test/v1",
            model: "fixture",
            hasKey: false,
          };
        case "SaveAISettings":
          return;
        case "AskAI":
          return "Fixture provider response\n```sql\nSELECT status, COUNT(*) AS orders, SUM(total) AS revenue FROM orders GROUP BY status ORDER BY revenue DESC;\n```";
        case "SaveConnection": {
          const next = { ...args[0], id: args[0].id || "saved-connection" };
          connections = [...connections.filter((c) => c.id !== next.id), next];
          return next;
        }
        case "DeleteConnection":
          connections = connections.filter((c) => c.id !== args[0]);
          return;
        case "TestConnection":
          return 5;
        case "GetOpsProject":
          return {
            root: "/fixture/project",
            yaml: "",
            revision: "1",
            project: { name: "Fixture", environments: [], applications: [] },
            artifacts: [],
            releases: [],
            runs: [],
          };
        default:
          throw new Error(`Missing fixture for ${method}`);
      }
    },
  );
  await page.addInitScript(() => {
    const w = window as any;
    if (w.go) return;
    w.go = {
      main: {
        App: new Proxy(
          {},
          {
            get: (_, method) => {
              if (method === "ExportCSV")
                return async (content: string) => {
                  const url = URL.createObjectURL(
                    new Blob([content], { type: "text/csv" }),
                  );
                  const a = document.createElement("a");
                  a.href = url;
                  a.download = "devopspad-results.csv";
                  a.click();
                  return true;
                };
              return (...args: unknown[]) => w.__fixtureBridge(method, ...args);
            },
          },
        ),
      },
    };
  });
}
export async function openWorkbench(page: Page) {
  await page.getByRole("button", { name: "orders", exact: true }).click();
  await page
    .getByRole("button", { name: "Run query", exact: true })
    .first()
    .click();
  await expect(page.getByText("Olivia Rhye", { exact: true })).toBeVisible();
}
export const test = base.extend({
  page: async ({ page }, use) => {
    await installWorkspace(page);
    await use(page);
  },
});
export { expect };
