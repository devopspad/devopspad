import { defineConfig } from "@playwright/test";
// Readiness checks must reach the local Vite server even on proxy-enabled hosts.
const bypass = [
  process.env.no_proxy || process.env.NO_PROXY || "",
  "127.0.0.1",
  "localhost",
  "[::1]",
]
  .filter(Boolean)
  .join(",");
process.env.no_proxy = bypass;
process.env.NO_PROXY = bypass;
export default defineConfig({
  testDir: "./tests",
  fullyParallel: true,
  workers: 2,
  use: {
    baseURL: "http://127.0.0.1:3415",
    channel: process.env.CI ? undefined : "chrome",
    headless: true,
    viewport: { width: 1480, height: 940 },
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
  },
  webServer: {
    command: "npm run dev",
    url: "http://127.0.0.1:3415",
    reuseExistingServer: !process.env.CI,
  },
});
