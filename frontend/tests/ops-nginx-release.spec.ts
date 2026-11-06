import { test, expect } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    const w = window as any;
    localStorage.setItem(
      "devopspad-ops-navigation-local",
      JSON.stringify({
        environment: "pro",
        application: "portal",
        page: "config",
      }),
    );
    w.__nginxSaves = [];
    w.__nginxError = "";
    const build = {
      id: "build-1",
      runId: "build-1",
      application: "portal",
      version: "v2",
      image: "registry.test/app@sha256:" + "a".repeat(64),
      createdAt: "2026-10-27T00:00:00Z",
    };
    const run = {
      id: "release-1",
      kind: "release",
      application: "portal",
      version: "v2",
      target: {
        environment: "pro",
        workload: "portal",
        container: "app",
        buildId: "build-1",
      },
      namespace: "default",
      connection: "k3s",
      status: "succeeded",
      image: build.image,
      startedAt: "2026-10-27T00:00:00Z",
      finishedAt: "2026-10-27T00:01:00Z",
      message: "Public HTTPS verified",
      actor: { name: "Codex", kind: "ai" },
      source: { commit: "a".repeat(40), branch: "main" },
      events: [
        {
          at: "2026-10-27T00:01:00Z",
          event: {
            type: "step",
            step: {
              name: "rollout",
              status: "succeeded",
              message: "1/1 ready",
            },
          },
        },
      ],
    };
    w.go = {
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
                {
                  name: "portal",
                  selector: "app=portal",
                  nginxPath: "/etc/nginx/conf.d/portal.conf",
                },
              ],
            },
            artifacts: [build],
            releases: [],
            runs: [run],
          }),
          ListConnections: async () => [{ id: "c1", name: "k3s", kind: "k3s" }],
          ReadOpsWorkloads: async () => ({
            workloads: [],
            warnings: [],
            truncated: false,
          }),
          ReadOpsNginx: async () => ({
            path: "/etc/nginx/conf.d/portal.conf",
            revision: "a".repeat(64),
            text: "server { listen 80; }\n",
          }),
          SaveOpsNginx: async (
            environment: string,
            application: string,
            update: any,
          ) => {
            w.__nginxSaves.push({ environment, application, update });
            if (w.__nginxError) throw new Error(w.__nginxError);
            return {
              path: "/etc/nginx/conf.d/portal.conf",
              text: update.text,
              revision: "b".repeat(64),
              message: "配置已保存，nginx -t 通过并已重载",
            };
          },
        },
      },
    };
  });
});

test("Nginx config reads, preserves failed draft and saves in selected environment", async ({
  page,
}) => {
  await page.goto("/#infra");
  await expect(page.getByText("Nginx · 宿主机", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "编辑 Nginx", exact: true }).click();
  const editor = page.getByRole("textbox", {
    name: "Nginx 配置内容",
    exact: true,
  });
  await editor.fill("server { listen 8080; }\n");
  await page.getByRole("button", { name: "Nginx 配置", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Nginx 配置", exact: true })).toBeVisible();
  await expect(editor).toContainText("8080");
  await page.evaluate(() => {
    (window as any).__nginxError = "Nginx 配置校验失败，已恢复原文件";
  });
  await page
    .getByRole("button", { name: "保存并重载 Nginx", exact: true })
    .click();
  await expect(page.getByRole("alert")).toContainText("已恢复原文件");
  await expect(editor).toContainText("8080");
  await page.evaluate(() => {
    (window as any).__nginxError = "";
  });
  await page
    .getByRole("button", { name: "保存并重载 Nginx", exact: true })
    .click();
  await expect(page.getByRole("status")).toContainText("nginx -t 通过并已重载");
  expect(
    await page.evaluate(() => (window as any).__nginxSaves[1]),
  ).toMatchObject({
    environment: "pro",
    application: "portal",
    update: { revision: "a".repeat(64) },
  });
});

test("external release appears in releases with provenance and execution details", async ({
  page,
}) => {
  await page.goto("/#infra");
  await page.getByRole("button", { name: "制品发布", exact: true }).click();
  await expect(page.locator(".ops-release-time")).toContainText("外部发布");
  await expect(page.getByText("上报成功", { exact: true })).toBeVisible();
  await page.locator(".ops-release-row").click();
  await expect(
    page.getByText("Public HTTPS verified", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "回退到此版本", exact: true }),
  ).toHaveCount(0);
  await page
    .getByRole("button", { name: "查看发布执行记录", exact: true })
    .click();
  await expect(
    page.getByText("Public HTTPS verified", { exact: true }),
  ).toBeVisible();
});
