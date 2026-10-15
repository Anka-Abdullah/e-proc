// UI tests: the flows a person actually performs. These cover what the earlier
// verification could not — clicking, typing and submitting in a real browser.

import { expect, test } from "@playwright/test";

import { apiCall, createAccount, loginThroughUi, mintToken } from "./helpers";

test.describe("konsol e-procurement", () => {
  test("halaman login tampil dan menolak kredensial salah", async ({ page }) => {
    await page.goto("/login");
    await expect(page.getByRole("heading", { name: "E-Procurement" })).toBeVisible();

    await page.getByLabel("Tenant").fill("demo");
    await page.getByLabel("Email").fill("tidak-ada@contoh.co.id");
    await page.getByLabel("Password").fill("salah-sekali");
    await page.getByRole("button", { name: "Masuk" }).click();

    // The API message is shown, and the user stays on the login page.
    await expect(page.getByText(/invalid email or password/i)).toBeVisible();
    await expect(page).toHaveURL(/\/login$/);
  });

  test("login berhasil mengarahkan ke dashboard", async ({ page }) => {
    const account = await createAccount("procurement_officer");
    await loginThroughUi(page, account);

    await expect(page.getByRole("heading", { name: "Dashboard" })).toBeVisible();
    await expect(page.getByText(account.email)).toBeVisible();
  });

  test("menu disaring sesuai role", async ({ page }) => {
    const account = await createAccount("vendor");
    await loginThroughUi(page, account);

    // A vendor may read tenders and submit bids, so those entries must be there.
    await expect(page.getByRole("link", { name: "Tender" }).first()).toBeVisible();
    await expect(page.getByRole("link", { name: "Penawaran" }).first()).toBeVisible();

    // Vendor permissions do not include vendor master data, approvals, audit or
    // reports, so those entries must be absent.
    await expect(page.getByRole("link", { name: "Vendor" })).toHaveCount(0);
    await expect(page.getByRole("link", { name: "Approval" })).toHaveCount(0);
    await expect(page.getByRole("link", { name: "Audit Log" })).toHaveCount(0);
    await expect(page.getByRole("link", { name: "Laporan" })).toHaveCount(0);
  });

  test("vendor dapat dibuat lewat UI dan muncul di tabel", async ({ page }) => {
    const account = await createAccount("procurement_officer");
    await loginThroughUi(page, account);

    await page.getByRole("link", { name: "Vendor" }).first().click();
    await page.waitForURL("**/vendors");

    const code = `UI-${Date.now().toString().slice(-6)}`;
    await page.getByRole("button", { name: "Tambah vendor" }).click();
    const dialog = page.getByRole("dialog", { name: "Tambah vendor" });
    await dialog.getByLabel("Kode vendor").fill(code);
    await dialog.getByLabel("Nama").fill("PT Dari UI");
    await dialog.getByLabel("Email").fill(`ui${Date.now()}@contoh.co.id`);
    await dialog.getByRole("button", { name: "Simpan" }).click();

    await expect(page.getByText(new RegExp(`Vendor ${code} dibuat`))).toBeVisible();
    await expect(page.getByRole("cell", { name: code })).toBeVisible();
  });

  test("validasi form menampilkan error per field", async ({ page }) => {
    const account = await createAccount("procurement_officer");
    await loginThroughUi(page, account);

    await page.goto("/vendors");
    await page.getByRole("button", { name: "Tambah vendor" }).click();
    const dialog = page.getByRole("dialog", { name: "Tambah vendor" });
    await dialog.getByLabel("Email").fill("bukan-email");
    await dialog.getByRole("button", { name: "Simpan" }).click();

    // Client-side validation fires before any request is sent.
    await expect(dialog.getByText("Kode wajib diisi")).toBeVisible();
    await expect(dialog.getByText("Format email tidak valid")).toBeVisible();
  });

  test("po di atas threshold masuk approval dan statusnya berubah otomatis", async ({ page }) => {
    const account = await createAccount("procurement_officer");
    await loginThroughUi(page, account);

    // Seed the chain through the API: tender, bid, PO. The UI assertions below
    // then watch the automatic approval and PO status change.
    const admin = mintToken("admin", "demo", "u-e2e-admin");
    const stamp = Date.now();
    const closeAt = new Date(Date.now() + 3 * 24 * 3600 * 1000).toISOString().replace(/\.\d+Z$/, "Z");

    const tender = await apiCall<{ data: { id: string } }>(
      "POST",
      "/api/v1/tenders",
      {
        code: `TND-UI-${stamp}`,
        title: `Tender UI ${stamp}`,
        items: [{ name: "Laptop", quantity: 10, unit: "unit" }],
        budget: { amount: 150000000, currency: "IDR" },
        close_at: closeAt,
      },
      admin,
    );
    const tenderId = tender.body.data?.id;
    expect(tender.status, JSON.stringify(tender.body)).toBe(201);
    expect(tenderId).toBeTruthy();

    await apiCall("POST", `/api/v1/tenders/${tenderId}/publish`, undefined, admin);

    const bid = await apiCall<{ data: { id: string } }>(
      "POST",
      "/api/v1/bids",
      {
        tender_id: tenderId,
        vendor_id: `v-ui-${stamp}`,
        delivery_days: 14,
        currency: "IDR",
        items: [{ name: "Laptop", quantity: 10, unit_price: { amount: 11000000, currency: "IDR" } }],
      },
      admin,
    );
    const bidId = bid.body.data?.id;
    expect(bid.status, JSON.stringify(bid.body)).toBe(201);

    const po = await apiCall<{ data: { purchase_order: { id: string; number: string } } }>(
      "POST",
      "/api/v1/purchase-orders",
      {
        tender_id: tenderId,
        bid_id: bidId,
        vendor_id: `v-ui-${stamp}`,
        currency: "IDR",
        lines: [{ name: "Laptop", quantity: 10, unit_price: { amount: 11000000, currency: "IDR" } }],
      },
      admin,
    );
    expect(po.status, JSON.stringify(po.body)).toBe(201);
    const poNumber = po.body.data?.purchase_order?.number;
    expect(poNumber).toBeTruthy();

    // The UI must show the PO as pending approval without anyone patching it.
    await page.goto("/purchase-orders");
    const poRow = page.getByRole("row", { name: new RegExp(poNumber!) });
    await expect(poRow).toBeVisible();
    await expect(poRow.getByText("pending approval")).toBeVisible();

    // And the approval queue must contain the request the consumer created.
    await page.getByRole("link", { name: "Approval" }).first().click();
    await page.waitForURL("**/approvals");
    const approvalsTable = page.getByRole("table");
    await expect(approvalsTable.getByText("pending").first()).toBeVisible();
  });

  test("logout mengembalikan ke halaman login", async ({ page }) => {
    const account = await createAccount("approver");
    await loginThroughUi(page, account);

    await page.getByRole("button", { name: "Keluar" }).click();
    await page.waitForURL("**/login");
    await expect(page.getByRole("heading", { name: "E-Procurement" })).toBeVisible();
  });
});
