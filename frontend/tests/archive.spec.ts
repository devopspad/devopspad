import { test, expect, type Page } from "@playwright/test";

async function install(page: Page, opts: { empty?: boolean } = {}) {
  await page.addInitScript((empty) => {
    const w = window as any;
    w.__calls = [];
    const record = (name: string, args: unknown[]) => w.__calls.push([name, ...args]);
    let opened = !empty;
    const resources = [
      { ref: "product:heartie", kind: "Product", name: "heartie", title: "Heartie", file: "products/heartie/product.yaml", labels: { line: "consumer" }, header: ["NAME", "STAGE"], row: ["heartie", "live"], usedBy: [] },
      { ref: "product:toolbox", kind: "Product", name: "toolbox", title: "Toolbox", file: "products/toolbox/product.yaml", labels: { line: "tools" }, header: ["NAME", "STAGE"], row: ["toolbox", "building"], usedBy: [] },
      { ref: "service:heartie/admin", kind: "Service", name: "admin", title: "", file: "products/heartie/services/admin.yaml", product: "heartie", header: ["REF", "TYPE", "DEPLOYMENTS"], row: ["service:heartie/admin", "web", "pro:k8s"], usedBy: ["product:heartie"] },
      { ref: "host:hk-1", kind: "Host", name: "hk-1", title: "Hong Kong 1", file: "inventory/hosts/hk-1.yaml", header: ["NAME", "STATE", "PUBLIC", "ROLES", "PROVIDER"], row: ["hk-1", "active", "203.0.113.10", "k3s-server,nginx", "aliyun"], usedBy: ["product:heartie", "product:toolbox"] },
      { ref: "domain:heartie.example", kind: "Domain", name: "heartie.example", title: "", file: "inventory/domains/heartie.example.yaml", header: ["NAME", "REGISTRAR", "DNS", "EXPIRES"], row: ["heartie.example", "cloudflare", "cloudflare", "2027-05-01"], usedBy: ["product:heartie"] },
    ];
    const summary = () => ({
      dir: "/Users/me/opc-archive",
      name: "my-opc",
      errors: [],
      warnings: [],
      resources,
      matrix: [
        { product: "heartie", title: "Heartie", stage: "live", services: ["admin"], domains: ["heartie.example"], environments: ["pro→hk-k3s/default"], hosts: ["hk-1"] },
        { product: "toolbox", title: "Toolbox", stage: "building", services: [], domains: [], environments: [], hosts: ["hk-1"] },
      ],
      renewals: [{ due: "2026-11-20", days: 13, what: "certificate admin.heartie.example (observed)", ref: "domain:heartie.example", usedBy: ["product:heartie"] }],
      drift: [{ ref: "domain:heartie.example", severity: "warn", field: "spec.renewal.expiresAt", archive: "2027-05-01", observed: "2027-06-01", message: "expiry differs (cloudflare-registrar); update the archive" }],
      git: { dir: "/Users/me/opc-archive", branch: "main", upstream: "origin/main", ahead: 1, behind: 0, fetched: false, changes: [" M products/heartie/services/admin.yaml"] },
    });
    w.go = {
      main: {
        App: {
          ListConnections: async () => [],
          GetArchive: async () => {
            if (!opened) throw new Error("No archive selected. Create one, or open an existing archive folder.");
            return summary();
          },
          SelectArchive: async (create: boolean) => {
            record("SelectArchive", [create]);
            opened = true;
            return summary();
          },
          DescribeArchiveResource: async (ref: string) => ({
            description: {
              ref,
              kind: "Service",
              file: "products/heartie/services/admin.yaml",
              deployments: [{ environment: "pro", runtime: "k8s", cluster: "cluster:hk-k3s", namespace: "default", workload: { kind: "Deployment", name: "heartie-admin" } }],
              releaseNotes: "products/heartie/notes/release-admin.md",
              references: [{ from: ref, field: "spec.configs[0].target.host", to: "host:hk-1", file: "x" }],
              referencedBy: [],
              usedByProducts: ["product:heartie"],
              secrets: [],
              notes: [],
              recentRecords: [{ id: "01M", action: "manual", status: "succeeded", message: "restarted", startedAt: "2026-11-07T10:00:00Z", actor: { type: "ai", name: "claude-code" }, targets: [ref] }],
            },
            text: "apiVersion: devopspad.io/v1alpha1\nkind: Service\n",
            revision: "rev1",
          }),
          SaveArchiveFile: async (file: string, text: string, revision: string) => {
            record("SaveArchiveFile", [file, text, revision]);
            return { revision: "rev2", issues: [{ file, line: 3, code: "field.required", message: "type is required" }] };
          },
          NewArchiveResource: async (kind: string, name: string, product: string) => {
            record("NewArchiveResource", [kind, name, product]);
            return `products/${name}/product.yaml`;
          },
          ArchiveGit: async (action: string, message: string) => {
            record("ArchiveGit", [action, message]);
            return action === "commit" ? "Committed abc123" : "Pushed";
          },
          ScanArchive: async (kind: string) => {
            record("ScanArchive", [kind]);
            return { observations: [{}, {}], unregisteredZones: [], notes: [] };
          },
          SyncArchiveConnections: async (dryRun: boolean) => {
            record("SyncArchiveConnections", [dryRun]);
            return [{ ref: "cluster:hk-k3s", name: "hk-k3s", action: dryRun ? "create" : "create" }, { ref: "datastore:main-mysql", name: "main-mysql", action: "unchanged" }];
          },
        },
      },
    };
  }, Boolean(opts.empty));
}

const calls = (page: Page) => page.evaluate(() => (window as any).__calls as unknown[][]);

test("archive empty state creates an archive", async ({ page }) => {
  await install(page, { empty: true });
  await page.goto("/#archive");
  await expect(page.getByText("No archive open")).toBeVisible();
  await page.getByRole("button", { name: "Create archive" }).click();
  await expect(page.getByRole("heading", { name: "Due soon" })).toBeVisible();
  expect(await calls(page)).toContainEqual(["SelectArchive", true]);
});

test("archive overview, products, search and editing", async ({ page }) => {
  await install(page);
  await page.goto("/#archive");
  const rail = page.getByRole("group", { name: "Workspace navigation" });
  await expect(rail.getByRole("button", { name: "Archive" })).toHaveAttribute("aria-current", "page");
  await expect(page.getByText("certificate admin.heartie.example (observed)")).toBeVisible();
  await expect(page.getByText("expiry differs (cloudflare-registrar)")).toBeVisible();
  await expect(page.getByRole("button", { name: "Git status" })).toContainText("main ↑1 · 1 changed");

  await page.getByRole("button", { name: /^Products/ }).click();
  await page.getByLabel("Stage").selectOption("live");
  await expect(page.getByRole("cell", { name: "Heartie", exact: true })).toBeVisible();
  await expect(page.getByRole("cell", { name: "Toolbox", exact: true })).toHaveCount(0);

  await page.getByLabel("Search the archive").fill("line=tools");
  await expect(page.getByRole("cell", { name: "product:toolbox" })).toBeVisible();
  await expect(page.getByRole("cell", { name: "product:heartie" })).toHaveCount(0);
  await page.getByLabel("Search the archive").fill("");

  await page.getByRole("button", { name: /^Services/ }).click();
  await page.getByRole("cell", { name: "service:heartie/admin" }).click();
  await expect(page.getByRole("heading", { name: "service:heartie/admin" })).toBeVisible();
  await expect(page.getByText("Deployment/heartie-admin")).toBeVisible();
  await expect(page.getByText("Release notes: products/heartie/notes/release-admin.md")).toBeVisible();
  await expect(page.getByRole("button", { name: "host:hk-1" })).toBeVisible();
  await expect(page.getByText("restarted")).toBeVisible();

  await page.locator(".cm-content").click();
  await page.keyboard.press("End");
  await page.keyboard.type("metadata: {}");
  await page.getByRole("button", { name: "Save" }).click();
  await expect(page.getByText("Line 3: type is required")).toBeVisible();
  const save = (await calls(page)).find((c) => c[0] === "SaveArchiveFile")!;
  expect(save[1]).toBe("products/heartie/services/admin.yaml");
  expect(save[3]).toBe("rev1");
});

test("archive git, scan and connection sync", async ({ page }) => {
  await install(page);
  await page.goto("/#archive");
  await page.getByRole("button", { name: "Git status" }).click();
  await page.getByLabel("Commit message").fill("archive(product:heartie): update admin");
  await page.getByRole("button", { name: "Commit", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("Committed abc123");

  await page.getByRole("button", { name: "Scan" }).click();
  await expect(page.getByRole("status")).toContainText("Scanned 2 resources.");

  await page.getByRole("button", { name: "Sync connections" }).click();
  await expect(page.getByRole("cell", { name: "cluster:hk-k3s" })).toBeVisible();
  await page.getByRole("button", { name: "Apply 1 changes" }).click();
  await expect(page.getByRole("status")).toContainText("Desktop connections updated (1 changed).");
  expect(await calls(page)).toEqual(
    expect.arrayContaining([
      ["ArchiveGit", "commit", "archive(product:heartie): update admin"],
      ["ScanArchive", ""],
      ["SyncArchiveConnections", true],
      ["SyncArchiveConnections", false],
    ]),
  );
});
