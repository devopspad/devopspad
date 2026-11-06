import { test, expect } from "@playwright/test";

test("Deployment loads only workloads, shares pending reads, caches navigation and forces refresh", async ({
  page,
}) => {
  await page.addInitScript(() => {
    const w = window as any;
    const now = Date.now.bind(Date);
    w.__advance = 0;
    Date.now = () => now() + w.__advance;
    w.__reads = { workloads: 0, full: 0 };
    const snapshot = {
      root: "/local/project",
      revision: "r1",
      yaml: "",
      project: {
        name: "Local",
        environments: [
          { name: "pro", connection: "k3s", namespace: "default" },
        ],
        applications: [
          { name: "svc-user", selector: "app=svc-user" },
          { name: "svc-other", selector: "app=svc-other" },
        ],
      },
      artifacts: [],
      releases: [],
      runs: [],
    };
    function result(selector: string) {
      return {
        workloads: [
          {
            name: selector.split("=")[1],
            kind: "Deployment",
            resource: "deployments",
            namespace: "default",
            uid: selector,
            desired: 1,
            ready: 1,
            image: "app:1",
            selector,
          },
        ],
        pods: [],
        warnings: [],
        truncated: false,
        fetchedAt: new Date().toISOString(),
      };
    }
    w.go = {
      main: {
        App: {
          GetOpsProject: async () => structuredClone(snapshot),
          ListConnections: async () => [
            { id: "c1", name: "k3s", kind: "k3s", database: "default" },
          ],
          ReadOpsWorkloads: async (
            _id: string,
            _ns: string,
            selector: string,
          ) => {
            w.__reads.workloads++;
            if (w.__reads.workloads === 1)
              await new Promise<void>((resolve) => {
                w.__finishRead = resolve;
              });
            if (w.__failNext) {
              w.__failNext = false;
              throw new Error("集群连接中断");
            }
            return result(selector);
          },
          ReadOpsCluster: async (
            _id: string,
            _ns: string,
            selector: string,
          ) => {
            w.__reads.full++;
            return result(selector);
          },
        },
      },
    };
  });
  const navigate = (name: string) =>
    page.getByRole("button", { name, exact: true }).click();
  const row = (name: string) => page.getByRole("button", { name, exact: true });
  const counts = () => page.evaluate(() => (window as any).__reads);
  await page.goto("/#apps");
  await navigate("Deployment");
  await expect(page.getByRole("status")).toContainText("正在读取集群负载");
  await navigate("制品库");
  await navigate("Deployment");
  expect(await counts()).toEqual({ workloads: 1, full: 0 });
  await page.evaluate(() => (window as any).__finishRead());
  await expect(row("svc-user")).toBeVisible();
  await navigate("制品库");
  await navigate("Deployment");
  await expect(row("svc-user")).toBeVisible();
  expect(await counts()).toEqual({ workloads: 1, full: 0 });

  await navigate("刷新");
  await expect(
    page.getByRole("button", { name: "刷新", exact: true }),
  ).toBeEnabled();
  expect(await counts()).toEqual({ workloads: 2, full: 0 });
  await navigate("Pod 实例");
  await expect.poll(counts).toEqual({ workloads: 2, full: 1 });
  await navigate("Deployment");
  await expect(row("svc-user")).toBeVisible();
  expect(await counts()).toEqual({ workloads: 2, full: 1 });

  await navigate("制品库");
  await page.evaluate(() => {
    (window as any).__advance += 16000;
  });
  await navigate("Deployment");
  await expect.poll(counts).toEqual({ workloads: 3, full: 1 });
  await page.getByLabel("应用", { exact: true }).selectOption("svc-other");
  await expect(row("svc-other")).toBeVisible();
  await expect(row("svc-user")).toHaveCount(0);
  expect(await counts()).toEqual({ workloads: 4, full: 1 });

  await page.evaluate(() => {
    (window as any).__failNext = true;
  });
  await navigate("刷新");
  await expect(page.getByRole("alert")).toContainText("集群连接中断");
  await expect(row("svc-other")).toHaveCount(0);
  await navigate("制品库");
  await navigate("Deployment");
  await expect(row("svc-other")).toBeVisible();
  expect(await counts()).toEqual({ workloads: 6, full: 1 });
});
