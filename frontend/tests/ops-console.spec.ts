import { test, expect } from "@playwright/test";

test("desktop application bridge requires a reviewed plan and reports rollout failure", async ({
  page,
}) => {
  await page.addInitScript(() => {
    const image = "registry.local/svc-user@sha256:" + "a".repeat(64);
    const snap: any = {
      root: "/local/project",
      revision: "r1",
      yaml: "apiVersion: infradex.project/v1\nname: Team\nenvironments: []\napplications: []\n",
      project: {
        apiVersion: "infradex.project/v1",
        name: "Team",
        environments: [
          { name: "pro", connection: "local-k3s", namespace: "default" },
        ],
        applications: [{ name: "svc-user", selector: "app=svc-user" }],
      },
      artifacts: [
        {
          id: "a1",
          application: "svc-user",
          version: "2.0.0",
          image,
          source: "Release test",
          createdAt: "2026-10-23T00:00:00Z",
        },
      ],
      releases: [],
    };
    const calls: any[] = [];
    (window as any).__opsCalls = calls;
    (window as any).go = {
      main: {
        App: {
          GetOpsProject: async () => snap,
          ListConnections: async () => [
            {
              id: "k1",
              name: "local-k3s",
              kind: "k3s",
              database: "default",
              context: "local",
            },
          ],
          ReadOpsWorkloads: async (...args: any[]) => {
            calls.push(["cluster", ...args]);
            return {
              workloads: [
                {
                  name: "svc-user",
                  namespace: "default",
                  kind: "Deployment",
                  uid: "w1",
                  resource: "deployments",
                  ready: 1,
                  desired: 1,
                  image: "old:1",
                },
              ],
              pods: [],
              warnings: [],
              truncated: false,
              fetchedAt: "2026-10-23T00:00:00Z",
            };
          },
          InspectClusterResource: async () => ({
            definition: JSON.stringify({
              spec: {
                template: {
                  spec: { containers: [{ name: "web", image: "old:1" }] },
                },
              },
            }),
          }),
          PlanOpsRelease: async (...args: any[]) => {
            calls.push(["plan", ...args]);
            return {
              id: "p1",
              namespace: "default",
              workload: "svc-user",
              container: "web",
              beforeImage: "old:1",
              image,
              expiresAt: "2099-10-23T00:00:00Z",
            };
          },
          PlanOpsWorkloadAction: async (...args: any[]) => {
            calls.push(["action", ...args]);
            return {
              id: "scale1",
              action: "scale",
              namespace: "default",
              workload: "svc-user",
              container: "",
              image: "",
              beforeImage: "",
              beforeReplicas: 1,
              replicas: args[4],
              expiresAt: "2099-10-23T00:00:00Z",
            };
          },
          ApplyOpsRelease: async (id: string) => {
            calls.push(["apply", id]);
            const r = {
              id,
              action: id === "scale1" ? "scale" : "release",
              beforeReplicas: 1,
              replicas: 0,
              application: "svc-user",
              environment: "pro",
              connection: "local-k3s",
              namespace: "default",
              workload: "svc-user",
              container: "web",
              image,
              beforeImage: "old:1",
              version: "2.0.0",
              status: "failed",
              startedAt: "2026-10-23T00:00:00Z",
              finishedAt: "2026-10-23T00:03:00Z",
              message: "等待就绪超时；请核对实际状态",
              steps: ["镜像已更新"],
            };
            snap.releases = [r];
            return r;
          },
          SaveOpsProject: async () => {
            throw new Error("配置已被修改，请刷新并合并改动后再保存");
          },
        },
      },
    };
  });
  await page.goto("/#apps");
  await page.getByRole("button", { name: "发布", exact: true }).click();
  await expect(page.getByLabel("发布容器")).toHaveValue("web");
  await page.getByRole("button", { name: "生成发布计划" }).click();
  expect(
    await page.evaluate(
      () =>
        (window as any).__opsCalls.filter((c: any[]) => c[0] === "apply")
          .length,
    ),
  ).toBe(0);
  await page.getByRole("button", { name: "确认发布", exact: true }).click();
  await expect(page.getByRole("dialog")).toContainText("等待就绪超时");
  const calls = await page.evaluate(() => (window as any).__opsCalls);
  expect(calls).toContainEqual(["cluster", "k1", "default", "app=svc-user"]);
  expect(calls).toContainEqual([
    "plan",
    "pro",
    "svc-user",
    "svc-user",
    "web",
    "a1",
  ]);
  expect(calls).toContainEqual(["apply", "p1"]);
  await page.getByRole("button", { name: "Close dialog" }).click();
  await expect(page.locator(".ops-release-row")).toContainText("发布未完成");
  await page.getByRole("button", { name: "项目映射", exact: true }).click();
  await page.getByLabel("项目 YAML").fill("my unsaved draft");
  await page.getByRole("button", { name: "保存配置" }).click();
  await expect(page.getByRole("alert")).toContainText("配置已被修改");
  await expect(page.getByLabel("项目 YAML")).toHaveValue("my unsaved draft");
  await page.getByRole("button", { name: "Deployment", exact: true }).click();
  await page.getByRole("button", { name: "扩缩容", exact: true }).click();
  await page.getByLabel("目标副本数").fill("0");
  await page.getByRole("button", { name: "生成操作计划" }).click();
  await expect(page.getByRole("dialog")).toContainText("副本数：1 → 0");
  expect(
    await page.evaluate(
      () =>
        (window as any).__opsCalls.filter((c: any[]) => c[0] === "apply")
          .length,
    ),
  ).toBe(1);
  await page.getByRole("button", { name: "确认扩缩容", exact: true }).click();
  await expect(page.getByRole("dialog")).toContainText("等待就绪超时");
  expect(await page.evaluate(() => (window as any).__opsCalls)).toContainEqual([
    "action",
    "pro",
    "svc-user",
    "svc-user",
    "scale",
    0,
  ]);
  expect(await page.evaluate(() => (window as any).__opsCalls)).toContainEqual([
    "apply",
    "scale1",
  ]);
});

