import { test, expect } from "@playwright/test";

test("CLI config updates show in releases without contents or rollback", async ({
  page,
}) => {
  await page.addInitScript(() => {
    localStorage.setItem(
      "devopspad-ops-navigation-local",
      JSON.stringify({
        environment: "pro",
        application: "thumbspark",
        page: "releases",
      }),
    );
    const record = {
      id: "6f1c1d8e-7a51-4c33-9f8e-0d5d1f4c2b10",
      action: "config",
      application: "thumbspark",
      environment: "pro",
      connection: "k3s",
      namespace: "default",
      workload: "thumbspark",
      container: "",
      image: "",
      beforeImage: "",
      version: "thumbspark-config-toml/config.toml",
      beforeReplicas: 1,
      replicas: 1,
      status: "succeeded",
      startedAt: "2026-11-07T00:00:00Z",
      finishedAt: "2026-11-07T00:01:00Z",
      message: "Deployment is ready",
      steps: [
        "Checked ConfigMap revision 41",
        "Updated thumbspark-config-toml key config.toml: 1 line(s) added, 1 removed (sha256 aaaaaaaaaaaa → bbbbbbbbbbbb, revision 41 → 42)",
        "Deployment is ready",
      ],
    };
    (window as any).go = {
      main: {
        App: {
          GetOpsProject: async () => ({
            root: "/test",
            revision: "p1",
            yaml: "",
            project: {
              name: "test",
              environments: [
                { name: "pro", connection: "k3s", namespace: "default" },
              ],
              applications: [
                { name: "thumbspark", selector: "app=thumbspark" },
              ],
            },
            artifacts: [],
            releases: [record],
            runs: [],
          }),
          ListConnections: async () => [{ id: "c1", name: "k3s", kind: "k3s" }],
          ReadOpsWorkloads: async () => ({
            workloads: [],
            warnings: [],
            truncated: false,
          }),
        },
      },
    };
  });
  await page.goto("/#infra");
  await page.getByRole("button", { name: "Releases", exact: true }).click();
  const row = page.locator(".ops-release-row");
  await expect(row).toContainText(
    "Config update thumbspark-config-toml/config.toml",
  );
  await expect(row).toContainText("ConfigMap · restarted thumbspark");
  await row.click();
  await expect(
    page.getByText(
      /ConfigMap key thumbspark-config-toml\/config.toml · Deployment thumbspark restarted/,
    ),
  ).toBeVisible();
  await expect(page.getByText(/1 line\(s\) added, 1 removed/)).toBeVisible();
  await expect(
    page.getByRole("button", {
      name: "Roll back to this version",
      exact: true,
    }),
  ).toHaveCount(0);
});
