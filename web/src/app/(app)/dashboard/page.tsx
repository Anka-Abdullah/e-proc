"use client";

// Dashboard: headline numbers pulled from the reporting service, which builds
// them from domain events.

import { useQuery } from "@tanstack/react-query";

import { api, formatMoney } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { Card, ErrorBanner, Spinner, StatCard } from "@/components/ui";

export default function DashboardPage() {
  const { user } = useAuth();

  const tenderSummary = useQuery({
    queryKey: ["reports", "tender-summary"],
    queryFn: api.reports.tenderSummary,
  });
  const approvalSummary = useQuery({
    queryKey: ["reports", "approval-summary"],
    queryFn: api.reports.approvalSummary,
  });
  const poByVendor = useQuery({
    queryKey: ["reports", "po-by-vendor"],
    queryFn: api.reports.poByVendor,
  });

  const failure = tenderSummary.error ?? approvalSummary.error ?? poByVendor.error;

  return (
    <>
      <header>
        <h1 className="text-lg font-semibold text-slate-900">Dashboard</h1>
        <p className="text-xs text-slate-500">
          Ringkasan pengadaan untuk tenant <span className="font-medium">{user?.tenant_id}</span>.
          Angka berasal dari proyeksi event, jadi bisa tertinggal beberapa detik.
        </p>
      </header>

      {failure && <ErrorBanner message={failure.message} />}

      {tenderSummary.isLoading || approvalSummary.isLoading ? (
        <Spinner />
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <StatCard
            label="Tender"
            value={tenderSummary.data?.total ?? 0}
            hint={`${tenderSummary.data?.open_tenders ?? 0} berjalan`}
            tone="blue"
          />
          <StatCard
            label="Nilai tender"
            value={formatMoney(tenderSummary.data?.total_budget ?? 0, tenderSummary.data?.currency)}
          />
          <StatCard
            label="Approval menunggu"
            value={approvalSummary.data?.by_status?.pending ?? 0}
            hint={
              approvalSummary.data?.pending_value
                ? formatMoney(approvalSummary.data.pending_value, approvalSummary.data.currency)
                : undefined
            }
            tone="amber"
          />
          <StatCard
            label="Nilai disetujui"
            value={formatMoney(approvalSummary.data?.approved_value ?? 0, approvalSummary.data?.currency)}
            tone="green"
          />
        </div>
      )}

      <Card title="Belanja per vendor" subtitle="Purchase order berstatus approved, issued, atau closed">
        {poByVendor.isLoading ? (
          <Spinner />
        ) : (poByVendor.data ?? []).length === 0 ? (
          <p className="rounded-md bg-slate-50 px-3 py-6 text-center text-sm text-slate-500">
            Belum ada purchase order yang disetujui. Buat PO terlebih dahulu di halaman Purchase Order.
          </p>
        ) : (
          <ul className="divide-y divide-slate-100">
            {(poByVendor.data ?? []).map((row) => (
              <li key={row.vendor_id} className="flex items-center justify-between py-2 text-sm">
                <span className="text-slate-700">{row.vendor_id}</span>
                <span className="text-slate-500">
                  {row.order_count} PO ·{" "}
                  <span className="font-medium text-slate-900">
                    {formatMoney(row.total, row.currency)}
                  </span>
                </span>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </>
  );
}
