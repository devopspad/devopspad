import { test, expect } from "@playwright/test";
import { installWorkspace } from "./fixtures/workspace";

const update = {
  current: "0.1.1",
  latest: "0.1.2",
  available: true,
  url: "https://github.com/devopspad/devopspad/releases/tag/v0.1.2",
  downloadUrl:
    "https://github.com/devopspad/devopspad/releases/download/v0.1.2/DevOpsPad-0.1.2-macos-arm64.dmg",
  notes: "",
};

test("a newer release prompts to download and can be postponed or skipped", async ({
  page,
}) => {
  const bridge = await installWorkspace(page, { update });
  await page.goto("/");
  const prompt = page.getByRole("dialog", { name: "DevOpsPad update" });
  await expect(prompt).toContainText("DevOpsPad v0.1.2 is available");
  await expect(prompt).toContainText("You have v0.1.1");

  await prompt.getByRole("button", { name: "Download update" }).click();
  await prompt.getByRole("button", { name: "Release notes" }).click();
  await expect
    .poll(() => bridge.opened)
    .toEqual([update.downloadUrl, update.url]);

  await prompt.getByRole("button", { name: "Remind me later" }).first().click();
  await expect(prompt).toBeHidden();
  await page.reload();
  await expect(prompt).toBeVisible();

  await prompt.getByRole("button", { name: "Skip this version" }).click();
  await expect(prompt).toBeHidden();
  await page.reload();
  await expect(
    page.getByRole("button", { name: "orders", exact: true }),
  ).toBeVisible();
  await expect(prompt).toBeHidden();
});

test("no prompt when the running version is current", async ({ page }) => {
  await installWorkspace(page);
  await page.goto("/");
  await expect(
    page.getByRole("button", { name: "orders", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("dialog", { name: "DevOpsPad update" }),
  ).toHaveCount(0);
});
