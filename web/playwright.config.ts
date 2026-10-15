import { defineConfig, devices } from "@playwright/test";

// The UI tests run against a live stack:
//
//   ./scripts/dev-up.sh                 # gateway + services (from the repo root)
//   cd web && NEXT_PUBLIC_API_URL=http://localhost:8000 pnpm dev
//   pnpm exec playwright test
//
// They are not parallel: the stack is shared and its rate limiter counts per IP,
// so parallel workers would throttle each other and produce false failures.
export default defineConfig({
  testDir: "./e2e",
  timeout: 60_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [["list"]],
  use: {
    baseURL: process.env.E2E_BASE_URL ?? "http://localhost:3000",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
