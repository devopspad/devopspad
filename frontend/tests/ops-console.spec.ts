import { test, expect } from "@playwright/test";

test("desktop application bridge requires a reviewed plan and reports rollout failure", async ({
  page,
}) => {
  await page.addInitScript(() => {
    const image = "registry.local/svc-user@sha256:" + "a".repeat(64);
    const snap: any = {
      root: "/local/project",
      revision: "r1",
      yaml: "apiVersion: devopspad.project/v1\nname: Team\nenvironments: []\napplications: []\n",
      project: {
        apiVersion: "devopspad.project/v1",
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
              message: "Timed out waiting for the rollout; check the actual state",
              steps: ["Image updated"],
            };
            snap.releases = [r];
            return r;
          },
          SaveOpsProject: async () => {
            throw new Error("The project was modified; refresh and merge the changes before saving");
          },
        },
      },
    };
  });
  await page.goto("/#apps");
  await page.getByRole("button", { name: "Release", exact: true }).click();
  await expect(page.getByLabel("Release container")).toHaveValue("web");
  await page.getByRole("button", { name: "Create release plan" }).click();
  expect(
    await page.evaluate(
      () =>
        (window as any).__opsCalls.filter((c: any[]) => c[0] === "apply")
          .length,
    ),
  ).toBe(0);
  await page.getByRole("button", { name: "Confirm release", exact: true }).click();
  await expect(page.getByRole("dialog")).toContainText("Timed out waiting for the rollout");
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
  await expect(page.locator(".ops-release-row")).toContainText("Release incomplete");
  await page.getByRole("button", { name: "Project mapping", exact: true }).click();
  await page.getByLabel("Project YAML").fill("my unsaved draft");
  await page.getByRole("button", { name: "Save config" }).click();
  await expect(page.getByRole("alert")).toContainText("The project was modified");
  await expect(page.getByLabel("Project YAML")).toHaveValue("my unsaved draft");
  await page.getByRole("button", { name: "Deployment", exact: true }).click();
  await page.getByRole("button", { name: "Scale", exact: true }).click();
  await page.getByLabel("Target replicas").fill("0");
  await page.getByRole("button", { name: "Create plan" }).click();
  await expect(page.getByRole("dialog")).toContainText("Replicas: 1 → 0");
  expect(
    await page.evaluate(
      () =>
        (window as any).__opsCalls.filter((c: any[]) => c[0] === "apply")
          .length,
    ),
  ).toBe(1);
  await page.getByRole("button", { name: "Confirm scale", exact: true }).click();
  await expect(page.getByRole("dialog")).toContainText("Timed out waiting for the rollout");
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

