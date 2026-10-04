import { test, expect } from "@playwright/test";

test("all seven engines retain SSH settings and expose appropriate authentication controls", async ({
  page,
}) => {
  await page.goto("/");
  await page
    .getByRole("button", { name: "New connection", exact: true })
    .first()
    .click();
  const dialog = page.getByRole("dialog", { name: "New connection" });
  await dialog.getByLabel("Connect through SSH tunnel").check();
  await dialog
    .getByLabel("SSH host", { exact: true })
    .fill("bastion.example.com");
  await dialog.getByLabel("SSH username", { exact: true }).fill("ubuntu");
  await dialog
    .getByLabel("Private key file", { exact: true })
    .fill("~/.ssh/id_ed25519");
  for (const engine of [
    "MySQL",
    "Redis",
    "MongoDB",
    "Kafka",
    "ClickHouse",
    "Kubernetes",
    "K3s",
  ]) {
    await dialog
      .locator(".engine-picker")
      .getByRole("button", { name: engine, exact: false })
      .click();
    await expect(dialog.getByLabel("Connect through SSH tunnel")).toBeChecked();
    await expect(dialog.getByLabel("SSH host", { exact: true })).toHaveValue(
      "bastion.example.com",
    );
    await expect(
      dialog.getByLabel("Private key file", { exact: true }),
    ).toHaveValue("~/.ssh/id_ed25519");
    if (engine === "Kubernetes" || engine === "K3s")
      await expect(
        dialog.getByText("The selected kubeconfig API server", {
          exact: false,
        }),
      ).toBeVisible();
  }
  await dialog.getByLabel("SSH authentication").selectOption("password");
  await expect(
    dialog.getByLabel("Private key file", { exact: true }),
  ).toHaveCount(0);
  await dialog
    .getByLabel("SSH password", { exact: true })
    .fill("test-draft-secret");
  await dialog.getByLabel("SSH authentication").selectOption("agent");
  await expect(dialog.getByLabel("SSH password", { exact: true })).toHaveCount(
    0,
  );
  await dialog.getByLabel("SSH authentication").selectOption("password");
  await expect(dialog.getByLabel("SSH password", { exact: true })).toHaveValue(
    "",
  );
  await dialog
    .getByLabel("Host key fingerprint")
    .fill("SHA256:" + "A".repeat(43));
  await expect(
    dialog.getByLabel("Known hosts file", { exact: true }),
  ).toBeDisabled();
  await dialog.getByLabel("Host key fingerprint").fill("");
  await expect(
    dialog.getByLabel("Known hosts file", { exact: true }),
  ).toBeEnabled();
  await dialog.getByLabel("SSH authentication").selectOption("key");
  await dialog.locator(".ssh-settings").scrollIntoViewIfNeeded();
  await page.screenshot({ path: "../artifacts/ssh-connection-settings.png" });
  await page.setViewportSize({ width: 1000, height: 680 });
  await dialog.getByLabel("Host key fingerprint").scrollIntoViewIfNeeded();
  const input = (await dialog
    .getByLabel("Host key fingerprint")
    .boundingBox())!;
  expect(input.y).toBeGreaterThanOrEqual(0);
  expect(input.y + input.height).toBeLessThanOrEqual(680);
  await dialog
    .getByRole("button", { name: "Save connection", exact: true })
    .scrollIntoViewIfNeeded();
  const footer = (await dialog
    .getByRole("button", { name: "Save connection", exact: true })
    .boundingBox())!;
  expect(footer.y + footer.height).toBeLessThanOrEqual(680);
});

test("SSH test and save send the current config through the desktop bridge", async ({
  page,
}) => {
  await page.addInitScript(() => {
    const captures: unknown[] = [];
    (window as unknown as { sshCaptures: unknown[] }).sshCaptures = captures;
    window.go = {
      main: {
        App: {
          ListConnections: async () => [],
          TestConnection: async (c: unknown) => {
            captures.push(c);
            return 15;
          },
          SaveConnection: async (c: Record<string, unknown>) => {
            captures.push(c);
            return {
              ...c,
              id: "ssh-saved",
              password: "",
              ssh: { ...(c.ssh as object), password: "", hasPassword: true },
            };
          },
          SelectSSHFile: async () => "/tmp/fixture-private-key",
        } as never,
      },
    };
  });
  await page.goto("/");
  await page
    .getByRole("button", { name: "New connection", exact: true })
    .first()
    .click();
  const dialog = page.getByRole("dialog", { name: "New connection" });
  await dialog.getByLabel("Connection name").fill("Private MySQL");
  await dialog.getByLabel("Connect through SSH tunnel").check();
  await dialog
    .getByLabel("SSH host", { exact: true })
    .fill("bastion.example.com");
  await dialog.getByLabel("SSH username", { exact: true }).fill("ubuntu");
  await dialog
    .getByRole("button", { name: "Choose SSH private key", exact: true })
    .click();
  await expect(
    dialog.getByLabel("Private key file", { exact: true }),
  ).toHaveValue("/tmp/fixture-private-key");
  await dialog.getByLabel("SSH authentication").selectOption("password");
  await dialog
    .getByLabel("SSH password", { exact: true })
    .fill("test-ssh-secret");
  await dialog
    .getByRole("button", { name: "Test connection", exact: true })
    .click();
  await expect(dialog.getByRole("status")).toContainText(
    "Connection verified in 15 ms",
  );
  await dialog
    .getByRole("button", { name: "Save connection", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  const calls = await page.evaluate(
    () =>
      (window as unknown as { sshCaptures: { ssh: unknown }[] }).sshCaptures,
  );
  expect(calls).toHaveLength(2);
  expect(calls[0].ssh).toMatchObject({
    enabled: true,
    host: "bastion.example.com",
    port: 22,
    username: "ubuntu",
    authMethod: "password",
    password: "test-ssh-secret",
  });
  expect(calls[1].ssh).toEqual(calls[0].ssh);
});
