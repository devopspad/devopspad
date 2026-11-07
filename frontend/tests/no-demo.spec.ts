import { test, expect } from "@playwright/test";

test("first launch has no seeded connections or demo entry, including old stored demo data", async ({
  page,
}) => {
  await page.addInitScript(() => {
    localStorage.setItem(
      "devopspad-ops-demo-v1",
      JSON.stringify({
        project: { name: "Old fake project" },
        artifacts: [{ version: "fake-version" }],
      }),
    );
    localStorage.setItem(
      "devopspad-workspace-v1",
      JSON.stringify({
        version: 1,
        demo: true,
        enabled: true,
        workspaces: {
          demo: {
            activeId: "old",
            selected: "demo-mysql",
            tabs: [
              {
                id: "old",
                connectionId: "demo-mysql",
                title: "Fake orders",
                query: "SELECT * FROM fake",
              },
            ],
          },
          personal: { tabs: [], activeId: "", selected: "" },
        },
      }),
    );
  });
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "No open connections yet" }),
  ).toBeVisible();
  await expect(page.locator(".tree-connection")).toHaveCount(0);
  await expect(page.getByRole("textbox", { name: "Query editor" })).toHaveCount(
    0,
  );
  await expect(
    page.getByText(
      /Demo workspace|Sample connections|View demo|Switch local project|查看演示|Fake orders|Olivia/,
    ),
  ).toHaveCount(0);
  await expect(
    page.getByRole("group", { name: "Workspace navigation" }).getByRole("button"),
  ).toHaveCount(2);
  await page
    .getByRole("group", { name: "Workspace navigation" })
    .getByRole("button", { name: "Infra", exact: true })
    .click();
  await expect(
    page.getByText("Connect your infrastructure", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText(/Old fake project|fake-version|View demo|Demo project|查看演示|演示项目/),
  ).toHaveCount(0);
});

test("old application service routes reopen Infra without an extra history entry", async ({
  page,
}) => {
  await page.goto("/#apps");
  await expect(page).toHaveURL(/#infra$/);
  await expect(
    page
      .getByRole("group", { name: "Workspace navigation" })
      .getByRole("button", { name: "Infra", exact: true }),
  ).toHaveAttribute("aria-current", "page");
  await expect(
    page.getByRole("button", { name: "Application services", exact: true }),
  ).toHaveCount(0);
  await page.reload();
  await expect(page).toHaveURL(/#infra$/);
  await expect(
    page.getByText("Connect your infrastructure", { exact: true }),
  ).toBeVisible();
});
