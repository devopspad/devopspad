import { test, expect } from "@playwright/test";

test("Secrets page plans and applies a Secret without echoing values, then patches the Deployment", async ({ page }) => {
  await page.addInitScript(() => {
    const calls: any[] = [];
    (window as any).__calls = calls;
    const snap = {
      root: "/local/project",
      revision: "r1",
      yaml: "",
      project: {
        apiVersion: "devopspad.project/v1",
        name: "Team",
        environments: [{ name: "pro", connection: "local-k3s", namespace: "default" }],
        applications: [{ name: "svc-user", selector: "app=svc-user" }],
      },
      artifacts: [],
      releases: [],
      runs: [],
    };
    const record = (id: string, action: string) => ({
      id, action, application: "svc-user", environment: "pro", connection: "local-k3s", namespace: "default", workload: "svc-user",
      container: "", image: "", beforeImage: "", version: "", status: "succeeded", startedAt: "2026-11-07T00:00:00Z", finishedAt: "2026-11-07T00:00:01Z", message: "ok", steps: [],
    });
    (window as any).go = {
      main: {
        App: {
          GetOpsProject: async () => snap,
          ListConnections: async () => [{ id: "k1", name: "local-k3s", kind: "k3s", database: "default", context: "local" }],
          ReadOpsWorkloads: async () => ({ workloads: [], pods: [], warnings: [], truncated: false, fetchedAt: "2026-11-07T00:00:00Z" }),
          DescribeOpsSecret: async (...args: any[]) => {
            calls.push(["describe", ...args]);
            return { name: "app-keys", type: "Opaque", keys: ["OWNER"], updatedAt: "2026-11-01T00:00:00Z" };
          },
          PlanOpsSecret: async (...args: any[]) => {
            calls.push(["planSecret", ...args]);
            return { id: "s1", namespace: "default", name: "app-keys", type: "Opaque", create: false, contentSha256: "abc123def456",
              keys: [{ key: "API_KEY", change: "added" }, { key: "OWNER", change: "removed" }] };
          },
          ApplyOpsSecret: async (id: string) => {
            calls.push(["applySecret", id]);
            return record(id, "secret");
          },
          PlanOpsPatch: async (...args: any[]) => {
            calls.push(["planPatch", ...args]);
            return { id: "p1", action: "patch", namespace: "default", workload: "svc-user", container: "app", image: "", beforeImage: "", expiresAt: "2099-01-01T00:00:00Z",
              changes: ["add imagePullSecret ghcr-pull", "add envFrom secret app-keys to app"] };
          },
          ApplyOpsRelease: async (id: string) => {
            calls.push(["apply", id]);
            return record(id, "patch");
          },
        },
      },
    };
  });
  await page.goto("/#infra");
  await page.getByRole("button", { name: "Secrets", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Secrets", exact: true })).toBeVisible();

  await page.getByLabel("Secret name").fill("app-keys");
  await page.getByRole("button", { name: "Show current keys" }).click();
  await expect(page.getByText("app-keys (Opaque): OWNER")).toBeVisible();

  await page.getByLabel("Key 1", { exact: true }).fill("API_KEY");
  await page.getByLabel("Value 1", { exact: true }).fill("sk-very-secret");
  await page.getByRole("button", { name: "Plan" }).first().click();
  const plan = page.getByRole("list", { name: "Secret plan" });
  await expect(plan).toContainText("API_KEY added");
  await expect(plan).toContainText("OWNER removed");
  await page.getByRole("button", { name: "Apply to default" }).click();
  await expect(page.getByText("Secret updated.")).toBeVisible();
  // The value is cleared from the form after applying and never rendered elsewhere.
  await expect(page.getByLabel("Value 1", { exact: true })).toHaveValue("");
  await expect(page.locator("body")).not.toContainText("sk-very-secret");

  await page.getByLabel("Add imagePullSecret").fill("ghcr-pull");
  await page.getByLabel("Container", { exact: true }).fill("app");
  await page.getByLabel("Add envFrom Secret").fill("app-keys");
  await page.getByRole("button", { name: "Plan" }).nth(1).click();
  await expect(page.getByRole("list", { name: "Deployment plan" })).toContainText("add envFrom secret app-keys to app");
  await page.getByRole("button", { name: "Apply and roll the pods" }).click();
  await expect(page.getByText("Deployment updated and ready.")).toBeVisible();

  const calls = await page.evaluate(() => (window as any).__calls);
  expect(calls).toContainEqual(["planSecret", "pro", "svc-user", { name: "app-keys", type: "generic", data: { API_KEY: "sk-very-secret" }, server: "ghcr.io", username: "", password: "" }]);
  expect(calls).toContainEqual(["applySecret", "s1"]);
  expect(calls).toContainEqual(["planPatch", "pro", "svc-user", "svc-user", { container: "app", addImagePullSecrets: ["ghcr-pull"], addEnvFromSecrets: ["app-keys"], setEnvSecretRefs: [] }]);
  expect(calls).toContainEqual(["apply", "p1"]);
});
