import { test, expect } from "@playwright/test";

test("K3s onboarding reuses the saved connection and AI records refresh into artifacts", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.addInitScript(() => {
    const c: any = {
      id: "k3s-1",
      name: "local-k3s",
      kind: "k3s",
      database: "default",
      context: "local",
      ssh: { enabled: true },
    };
    const workload: any = {
      kind: "Deployment",
      name: "svc-api",
      namespace: "default",
      uid: "live-uid",
      resource: "deployments",
      desired: 1,
      ready: 1,
      image: "old:1",
      selector: "app=svc-api",
    };
    let snapshot: any = {
      root: "/local/team",
      revision: "r1",
      yaml: "apiVersion: devopspad.project/v1\nname: Team\nenvironments: []\napplications: []\n",
      project: {
        apiVersion: "devopspad.project/v1",
        name: "Team",
        environments: [],
        applications: [],
      },
      artifacts: [],
      releases: [],
      runs: [],
    };
    const calls: any[] = [];
    (window as any).__calls = calls;
    (window as any).__addRun = () => {
      const id = "a6b9f86e-d84b-4e3d-a5ed-7dfb4955b807",
        image = "registry.local/svc-api@sha256:" + "a".repeat(64),
        now = new Date().toISOString();
      snapshot = {
        ...snapshot,
        runs: [
          {
            apiVersion: "devopspad.run/v1",
            id,
            kind: "build",
            application: "svc-api",
            version: "2.0.0",
            actor: { kind: "ai", name: "Codex", sessionId: "task-k3s-build" },
            source: {
              repository: "https://example.com/svc-api.git",
              commit: "b".repeat(40),
              branch: "main",
              dirty: false,
            },
            recipe: "docker buildx build + push",
            status: "succeeded",
            image,
            startedAt: now,
            finishedAt: now,
            message: "构建和推送完成",
            events: [
              { at: now, event: { id: "1", type: "start" } },
              {
                at: now,
                event: {
                  id: "2",
                  type: "step",
                  step: {
                    name: "build",
                    status: "succeeded",
                    message: "构建完成",
                    logPath: "logs/build.log",
                  },
                },
              },
              {
                at: now,
                event: {
                  id: "3",
                  type: "step",
                  step: {
                    name: "push",
                    status: "succeeded",
                    message: "摘要已读取",
                  },
                },
              },
              {
                at: now,
                event: {
                  id: "4",
                  type: "finish",
                  finish: { status: "succeeded", message: "已完成" },
                },
              },
            ],
          },
        ],
        artifacts: [
          {
            id,
            runId: id,
            application: "svc-api",
            version: "2.0.0",
            image,
            source: "AI build",
            createdAt: now,
          },
        ],
      };
    };
    (window as any).go = {
      main: {
        App: {
          GetOpsProject: async () => snapshot,
          ListConnections: async () => [c],
          ReadOpsCluster: async (...args: any[]) => {
            calls.push(["read", ...args]);
            return {
              workloads: [workload],
              pods: [],
              warnings: [],
              truncated: false,
              fetchedAt: new Date().toISOString(),
            };
          },
          ReadOpsWorkloads: async (...args: any[]) => {
            calls.push(["workloads", ...args]);
            return {
              workloads: [workload],
              pods: [],
              warnings: [],
              truncated: false,
              fetchedAt: new Date().toISOString(),
            };
          },
          AttachOpsWorkload: async (r: any) => {
            calls.push(["attach", r]);
            snapshot = {
              ...snapshot,
              revision: "r2",
              project: {
                ...snapshot.project,
                environments: [
                  {
                    name: r.environment,
                    connection: c.name,
                    namespace: r.namespace,
                  },
                ],
                applications: [
                  { name: r.application, selector: workload.selector },
                ],
              },
            };
            return snapshot;
          },
        },
      },
    };
  });
  await page.goto("/#apps");
  await page.getByRole("button", { name: "接入集群应用" }).click();
  await expect(page.getByRole("dialog")).toContainText("通过已保存的 SSH 隧道");
  await expect(page.getByLabel("接入集群")).toHaveValue("k3s-1");
  await page.getByRole("button", { name: "读取负载" }).click();
  await expect(page.getByRole("dialog")).toContainText("1 个负载");
  await page.screenshot({ path: "../artifacts/ops-k3s-connect.png" });
  await page.getByRole("button", { name: "接入应用", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "svc-api", exact: true }),
  ).toBeVisible();
  expect(await page.evaluate(() => (window as any).__calls)).toContainEqual([
    "attach",
    {
      connectionId: "k3s-1",
      namespace: "default",
      workload: "svc-api",
      uid: "live-uid",
      resource: "deployments",
      environment: "local-k3s",
      application: "svc-api",
      revision: "r1",
    },
  ]);
  await page.getByRole("button", { name: "AI 执行记录", exact: true }).click();
  await expect(page.getByText("暂无执行记录", { exact: true })).toBeVisible();
  await page.evaluate(() => {
    (window as any).__addRun();
    window.dispatchEvent(new Event("focus"));
  });
  await expect(page.getByText("构建 · 2.0.0", { exact: true })).toBeVisible();
  await page.screenshot({ path: "../artifacts/ops-ai-runs.png" });
  await page.getByRole("button", { name: "详情", exact: true }).click();
  await expect(page.getByRole("dialog")).toContainText("task-k3s-build");
  await expect(page.getByRole("dialog")).toContainText("logs/build.log");
  await page.screenshot({ path: "../artifacts/ops-ai-run-detail.png" });
  await page.getByRole("button", { name: "Close dialog" }).click();
  await page.getByRole("button", { name: "制品库", exact: true }).click();
  await expect(page.getByRole("cell", { name: /2.0.0/ })).toBeVisible();
  await page.getByRole("button", { name: "查看构建记录" }).click();
  await expect(page.getByRole("dialog")).toContainText("Codex");
  await page.getByRole("button", { name: "Close dialog" }).click();
  await page.setViewportSize({ width: 1000, height: 680 });
  await expect(
    page.getByRole("button", { name: "接入 K3s", exact: true }),
  ).toBeInViewport();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBeTruthy();
  expect(errors).toEqual([]);
});
