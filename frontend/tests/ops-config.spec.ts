import { test, expect } from "@playwright/test";
import { parse } from "yaml";

async function installBridge(page: import("@playwright/test").Page) {
  await page.addInitScript(() => {
    const w = window as any;
    w.__configCalls = [];
    w.__configSaves = [];
    w.__savedConfigs = {};
    const project = {
      apiVersion: "devopspad.project/v1",
      name: "applications",
      environments: [
        { name: "pro", connection: "k3s", namespace: "team" },
        { name: "dev", connection: "k3s", namespace: "dev" },
      ],
      applications: [
        { name: "portal", selector: "app=portal" },
        { name: "api", selector: "app=api" },
      ],
    };
    w.go = {
      main: {
        App: {
          GetOpsProject: async () => ({
            root: "/local/project",
            revision: "r1",
            yaml: `apiVersion: devopspad.project/v1\nname: applications\nenvironments: ${JSON.stringify(project.environments)}\napplications: ${JSON.stringify(project.applications)}\n# retained comment\n`,
            project,
            artifacts: [],
            releases: [],
            runs: [],
          }),
          ListConnections: async () => [
            { id: "c1", name: "k3s", kind: "k3s", context: "local" },
          ],
          ReadOpsWorkloads: async (
            _id: string,
            namespace: string,
            selector: string,
          ) => {
            if (w.__holdWorkloads)
              await new Promise<void>((resolve) => {
                w.__finishWorkloads = resolve;
              });
            const name = selector.split("=")[1];
            return {
              workloads: [
                {
                  name,
                  namespace,
                  resource: "deployments",
                  kind: "Deployment",
                  uid: name,
                },
              ],
              warnings: [],
              truncated: false,
            };
          },
          SaveConfigMap: async (id: string, request: any) => {
            w.__configSaves.push({ id, ...request });
            if (w.__saveError) throw new Error(w.__saveError);
            const updated = {
              metadata: { uid: request.ref.uid, resourceVersion: "2" },
              data: request.data,
              binaryData: { "cert.bin": "YWJj" },
            };
            w.__savedConfigs[`${request.ref.namespace}/${request.ref.name}`] =
              updated;
            return JSON.stringify(updated);
          },
          InspectClusterResource: async (_id: string, ref: any) => {
            w.__configCalls.push(ref);
            if (ref.resource === "configmaps") {
              if (w.__denyConfig)
                throw new Error("Access denied: get configmaps");
              return {
                definition: JSON.stringify(
                  w.__savedConfigs[`${ref.namespace}/${ref.name}`] || {
                    metadata: {
                      uid: `${ref.namespace}-${ref.name}`,
                      resourceVersion: "1",
                    },
                    immutable: !!w.__immutable,
                    binaryData: { "cert.bin": "YWJj" },
                    data: {
                      "settings.json": '{"enabled":true}',
                      "settings.toml": "[server]\nport = 8080\n",
                      "application.yaml": `application: ${ref.name}\nnamespace: ${ref.namespace}\nendpoint: https://example.test/${"long-path/".repeat(90)}`,
                    },
                  },
                ),
              };
            }
            return {
              definition: JSON.stringify({
                spec: {
                  template: {
                    spec: {
                      initContainers: [
                        {
                          name: "init",
                          env: [
                            {
                              name: "INIT",
                              valueFrom: {
                                configMapKeyRef: {
                                  name: `${ref.name}-init`,
                                  key: "mode",
                                },
                              },
                            },
                          ],
                        },
                      ],
                      containers: [
                        {
                          name: "web",
                          env: [
                            { name: "APP_ENV", value: ref.namespace },
                            {
                              name: "TOKEN",
                              valueFrom: {
                                secretKeyRef: {
                                  name: "credentials",
                                  key: "token",
                                },
                              },
                            },
                          ],
                          envFrom: [
                            { configMapRef: { name: `${ref.name}-config` } },
                          ],
                          volumeMounts: [
                            { name: "config", mountPath: "/etc/app" },
                            { name: "projected", mountPath: "/etc/shared" },
                          ],
                        },
                      ],
                      volumes: [
                        {
                          name: "config",
                          configMap: { name: `${ref.name}-config` },
                        },
                        {
                          name: "projected",
                          projected: {
                            sources: [
                              { configMap: { name: "shared" } },
                              { secret: { name: "credentials" } },
                            ],
                          },
                        },
                      ],
                    },
                  },
                },
              }),
            };
          },
        },
      },
    };
  });
}

test("application config reads references directly and isolates applications, namespaces and failures", async ({
  page,
}) => {
  await installBridge(page);
  await page.goto("/#apps");
  await page.getByRole("button", { name: "App config", exact: true }).click();
  await expect(
    page.getByLabel("portal-config / application.yaml"),
  ).toContainText("namespace: team");
  await expect(page.getByLabel("portal-init / application.yaml")).toBeVisible();
  await expect(page.getByLabel("shared / application.yaml")).toBeVisible();
  await expect(
    page.getByRole("cell", {
      name: "Secret (reference only) / credentials / token",
      exact: true,
    }),
  ).toBeVisible();
  const calls = await page.evaluate(() => (window as any).__configCalls);
  expect(
    calls.filter(
      (c: any) => c.resource === "configmaps" && c.name === "portal-config",
    ),
  ).toHaveLength(1);
  expect(calls.some((c: any) => c.resource === "secrets")).toBe(false);
  expect(
    calls.every((c: any) => c.namespace === "team" && c.context === "local"),
  ).toBe(true);
  await page.getByLabel("Application", { exact: true }).selectOption("api");
  await expect(page.getByLabel("api-config / application.yaml")).toBeVisible();
  await expect(page.getByLabel("portal-config / application.yaml")).toHaveCount(
    0,
  );
  await page.getByLabel("Environment", { exact: true }).selectOption("dev");
  await expect(page.getByLabel("api-config / application.yaml")).toContainText(
    "namespace: dev",
  );
  await page.evaluate(() => {
    (window as any).__denyConfig = true;
  });
  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(page.getByRole("alert").first()).toContainText("Access denied");
  await expect(page.getByLabel("api-config / application.yaml")).toHaveCount(0);
  await page.evaluate(() => {
    (window as any).__denyConfig = false;
  });
  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(page.getByLabel("api-config / application.yaml")).toBeVisible();
});

test("pending reads cannot replace the selected application's config", async ({
  page,
}) => {
  await installBridge(page);
  await page.goto("/#apps");
  await page.evaluate(() => {
    (window as any).__holdWorkloads = true;
  });
  await page.getByRole("button", { name: "App config", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("Loading app config");
  await expect
    .poll(() => page.evaluate(() => typeof (window as any).__finishWorkloads))
    .toBe("function");
  await page.evaluate(() => {
    (window as any).__holdWorkloads = false;
  });
  await page.getByLabel("Application", { exact: true }).selectOption("api");
  await expect(page.getByLabel("api-config / application.yaml")).toBeVisible();
  await page.evaluate(() => (window as any).__finishWorkloads());
  await expect(page.getByLabel("portal-config / application.yaml")).toHaveCount(
    0,
  );
  expect(
    await page.evaluate(() =>
      (window as any).__configCalls.some((ref: any) => ref.name === "portal"),
    ),
  ).toBe(false);
});

test("config fits the window and restores the selected page without a workbench detour", async ({
  page,
}, testInfo) => {
  await installBridge(page);
  await page.goto("/#apps");
  await page.getByLabel("Application", { exact: true }).selectOption("api");
  await page.getByRole("button", { name: "App config", exact: true }).click();
  await expect(page.getByLabel("api-config / application.yaml")).toBeVisible();
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "App config", exact: true }),
  ).toBeVisible();
  await expect(page.getByLabel("Application", { exact: true })).toHaveValue("api");
  await expect(page.locator(".app-shell")).toHaveCount(0);
  for (const width of [1228, 1000]) {
    await page.setViewportSize({ width, height: 761 });
    await expect(
      page.getByRole("button", { name: "Refresh", exact: true }),
    ).toBeInViewport();
    expect(
      await page
        .locator(".ops-main")
        .evaluate((el) => el.scrollWidth <= el.clientWidth),
    ).toBe(true);
  }
  await page.screenshot({
    path: testInfo.outputPath("ops-application-config.png"),
  });
  await page.getByRole("button", { name: "Project mapping", exact: true }).click();
  await expect(page.getByLabel("Project YAML")).toContainText(
    "# retained comment",
  );
  const original = await page.getByLabel("Project YAML").inputValue();
  expect(
    await page
      .locator(".ops-main")
      .evaluate((el) => el.scrollWidth <= el.clientWidth),
  ).toBe(true);
  await page.getByRole("button", { name: "Format YAML" }).click();
  const yaml = await page.getByLabel("Project YAML").inputValue();
  expect(yaml).toContain("# retained comment");
  expect(yaml).toContain("applications:\n  - ");
  expect(parse(yaml)).toEqual(parse(original));
  for (const width of [1228, 1000]) {
    await page.setViewportSize({ width, height: 761 });
    await expect(
      page.getByRole("button", { name: "Save config" }),
    ).toBeInViewport();
    expect(
      await page
        .locator(".ops-main")
        .evaluate((el) => el.scrollWidth <= el.clientWidth),
    ).toBe(true);
  }
});

test("all configuration content remains reachable by scrolling at minimum window height", async ({
  page,
}, testInfo) => {
  await installBridge(page);
  await page.setViewportSize({ width: 1000, height: 680 });
  await page.goto("/#apps");
  await page.getByRole("button", { name: "App config", exact: true }).click();
  await expect(
    page.getByLabel("portal-config / application.yaml"),
  ).toBeVisible();
  const main = page.locator(".ops-main");
  expect(await main.evaluate((el) => el.scrollHeight > el.clientHeight)).toBe(
    true,
  );
  await main.hover();
  await page.mouse.wheel(0, 5000);
  const last = page
    .getByRole("cell", { name: "/etc/shared", exact: true })
    .last();
  await expect(last).toBeInViewport();
  await expect(
    page
      .getByRole("group", { name: "Workspace navigation" })
      .getByRole("button", { name: "Infra", exact: true }),
  ).toBeInViewport();
  await page.screenshot({
    path: testInfo.outputPath("ops-config-scrolled.png"),
  });
  await page.getByRole("button", { name: "Sharing & sync", exact: true }).click();
  await page
    .getByRole("heading", { name: "Credentials stay on each device" })
    .scrollIntoViewIfNeeded();
  await expect(
    page.getByRole("heading", { name: "Credentials stay on each device" }),
  ).toBeInViewport();
});

test("config files highlight, retain drafts across applications, and save to the selected cluster", async ({
  page,
}, testInfo) => {
  await installBridge(page);
  await page.goto("/#infra");
  await page.getByRole("button", { name: "App config", exact: true }).click();
  const card = page.getByRole("region", {
    name: "ConfigMap portal-config",
    exact: true,
  });
  const editor = page.getByRole("textbox", {
    name: "portal-config / application.yaml",
    exact: true,
  });
  await expect(editor).toHaveAttribute("contenteditable", "false");
  await expect(editor.locator(".cm-line span").first()).toBeVisible();
  expect(
    await editor
      .locator(".cm-line span")
      .first()
      .evaluate((el) => getComputedStyle(el).color),
  ).not.toBe(await editor.evaluate((el) => getComputedStyle(el).color));
  await card.getByRole("button", { name: "Edit config", exact: true }).click();
  await editor.fill("# keep comment\napplication: edited\nport: 8080\n");
  await expect(card.getByText("Unsaved", { exact: true })).toBeVisible();
  await page.getByLabel("Application", { exact: true }).selectOption("api");
  await expect(
    page.getByLabel("api-config / application.yaml", { exact: true }),
  ).not.toContainText("application: edited");
  await page.getByLabel("Application", { exact: true }).selectOption("portal");
  await expect(editor).toContainText("application: edited");
  await page.screenshot({ path: testInfo.outputPath("config-editor.png") });
  await card.getByRole("button", { name: "Save to cluster", exact: true }).click();
  await expect(card.getByRole("status")).toContainText("Saved to the cluster");
  await expect(editor).toHaveAttribute("contenteditable", "false");
  const saves = await page.evaluate(() => (window as any).__configSaves);
  expect(saves).toHaveLength(1);
  expect(saves[0]).toMatchObject({
    id: "c1",
    ref: {
      context: "local",
      namespace: "team",
      resource: "configmaps",
      name: "portal-config",
      uid: "team-portal-config",
    },
    resourceVersion: "1",
  });
  expect(saves[0].data["application.yaml"]).toBe(
    "# keep comment\napplication: edited\nport: 8080\n",
  );
  expect(saves[0].data["settings.json"]).toBe('{"enabled":true}');
  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(editor).toContainText("application: edited");
  await card.getByRole("button", { name: "Edit config", exact: true }).click();
  await editor.fill("application: cancelled");
  await card.getByRole("button", { name: "Cancel editing", exact: true }).click();
  await expect(editor).toContainText("application: edited");
  await expect(editor).not.toContainText("cancelled");
  await card.getByRole("button", { name: "Edit config", exact: true }).click();
  await editor.fill("application: second");
  await card.getByRole("button", { name: "Save to cluster", exact: true }).click();
  await expect(card.getByRole("status")).toContainText("Saved to the cluster");
  expect(
    await page.evaluate(() => (window as any).__configSaves[1].resourceVersion),
  ).toBe("2");
});

test("invalid YAML, JSON and TOML stay editable without writing to the cluster", async ({
  page,
}) => {
  await installBridge(page);
  await page.goto("/#infra");
  await page.getByRole("button", { name: "App config", exact: true }).click();
  const card = page.getByRole("region", {
    name: "ConfigMap portal-config",
    exact: true,
  });
  for (const [filename, invalid] of [
    ["application.yaml", "key: [broken"],
    ["settings.json", '{"key":'],
    ["settings.toml", 'key = "broken'],
  ]) {
    await card.getByRole("button", { name: "Edit config", exact: true }).click();
    const editor = page.getByRole("textbox", {
      name: `portal-config / ${filename}`,
      exact: true,
    });
    await editor.fill(invalid);
    await card.getByRole("button", { name: "Save to cluster", exact: true }).click();
    await expect(card.getByRole("alert")).toContainText(filename);
    await expect(editor).toContainText(invalid);
    await card.getByRole("button", { name: "Cancel editing", exact: true }).click();
  }
  expect(await page.evaluate(() => (window as any).__configSaves)).toHaveLength(
    0,
  );
});

test("save errors preserve edits and never claim success", async ({ page }) => {
  await installBridge(page);
  await page.goto("/#infra");
  await page.getByRole("button", { name: "App config", exact: true }).click();
  await page.getByLabel("Environment", { exact: true }).selectOption("dev");
  const card = page.getByRole("region", {
    name: "ConfigMap portal-config",
    exact: true,
  });
  const editor = page.getByRole("textbox", {
    name: "portal-config / application.yaml",
    exact: true,
  });
  await card.getByRole("button", { name: "Edit config", exact: true }).click();
  await editor.fill("application: retained");
  for (const error of [
    "The cluster config changed; refresh and try again",
    "No permission to update this ConfigMap",
  ]) {
    await page.evaluate((error) => {
      (window as any).__saveError = error;
    }, error);
    await card.getByRole("button", { name: "Save to cluster", exact: true }).click();
    await expect(card.getByRole("alert")).toHaveText(error);
    await expect(editor).toContainText("application: retained");
    await expect(editor).toHaveAttribute("contenteditable", "true");
    await expect(card.getByRole("status")).toHaveCount(0);
    await expect(page.getByLabel("Application", { exact: true })).toBeEnabled();
  }
  const saves = await page.evaluate(() => (window as any).__configSaves);
  expect(saves.every((call: any) => call.ref.namespace === "dev")).toBe(true);
  await card.getByRole("button", { name: "Cancel editing", exact: true }).click();
});

test("immutable ConfigMaps remain highlighted and read-only", async ({
  page,
}) => {
  await installBridge(page);
  await page.goto("/#infra");
  await page.evaluate(() => {
    (window as any).__immutable = true;
  });
  await page.getByRole("button", { name: "App config", exact: true }).click();
  const card = page.getByRole("region", {
    name: "ConfigMap portal-config",
    exact: true,
  });
  await expect(
    card.getByRole("button", { name: "Edit config", exact: true }),
  ).toBeDisabled();
  await expect(
    card.getByText("This ConfigMap is immutable."),
  ).toBeVisible();
});
