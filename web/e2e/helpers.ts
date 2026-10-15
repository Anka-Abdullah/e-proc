// Helpers for the UI tests. Accounts are created through the API so the browser
// flow can log in with real credentials instead of a token injected by a test.

import { execSync } from "node:child_process";

export const API = process.env.E2E_API_URL ?? "http://localhost:8000";

/** mintToken runs the repo's dev token minter (used only to bootstrap accounts). */
export function mintToken(role: string, tenant = "demo", user = `u-e2e-${role}`): string {
  return execSync(
    `go run ./scripts/mint-token -role ${role} -tenant ${tenant} -user ${user}`,
    { cwd: "..", encoding: "utf8" },
  ).trim();
}

interface ApiResult<T> {
  status: number;
  body: T;
}

/** apiCall performs a JSON request against the gateway. */
export async function apiCall<T = unknown>(
  method: string,
  path: string,
  body?: unknown,
  token?: string,
): Promise<ApiResult<T>> {
  const response = await fetch(API + path, {
    method,
    headers: {
      Accept: "application/json",
      ...(body === undefined ? {} : { "Content-Type": "application/json" }),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });

  const text = await response.text();
  let parsed: T = {} as T;
  if (text) {
    try {
      parsed = JSON.parse(text) as T;
    } catch {
      parsed = {} as T;
    }
  }
  return { status: response.status, body: parsed };
}

export interface TestAccount {
  email: string;
  password: string;
  role: string;
}

/** createAccount registers an account and returns its credentials. */
export async function createAccount(role: string): Promise<TestAccount> {
  const adminToken = mintToken("admin", "demo", "u-e2e-admin");
  const email = `ui-${role}-${Date.now()}@contoh.co.id`;
  const password = "rahasia-kuat-123";

  const result = await apiCall(
    "POST",
    "/api/v1/auth/register",
    { email, name: `UI ${role}`, password, role },
    adminToken,
  );
  if (result.status !== 201) {
    throw new Error(`register ${role} failed: ${result.status} ${JSON.stringify(result.body)}`);
  }
  return { email, password, role };
}

/** loginThroughUi fills the login form and waits for the dashboard. */
export async function loginThroughUi(
  page: import("@playwright/test").Page,
  account: TestAccount,
): Promise<void> {
  await page.goto("/login");
  await page.getByLabel("Tenant").fill("demo");
  await page.getByLabel("Email").fill(account.email);
  await page.getByLabel("Password").fill(account.password);
  await page.getByRole("button", { name: "Masuk" }).click();
  await page.waitForURL("**/dashboard");
}
