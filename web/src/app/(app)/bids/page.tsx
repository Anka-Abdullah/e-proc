"use client";

// Vendor quotations: submit, list per tender, compare prices, and select the
// winner. The comparison table is what procurement actually decides from, so it
// is a first-class panel rather than a hidden report.

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useSearchParams } from "next/navigation";
import { Suspense, useState } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { ApiError, api, formatDate, formatMoney } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { can } from "@/lib/rbac";
import type { Bid } from "@/lib/types";
import {
  Badge,
  Button,
  Card,
  ErrorBanner,
  Field,
  Input,
  Modal,
  Spinner,
  StatusBadge,
  SuccessBanner,
  Table,
  Textarea,
} from "@/components/ui";

const schema = z.object({
  tenderId: z.string().min(1, "Tender wajib diisi"),
  vendorId: z.string().min(1, "Vendor wajib diisi"),
  itemName: z.string().min(1, "Nama item wajib diisi"),
  quantity: z.coerce.number().positive("Kuantitas harus lebih dari 0"),
  unitPrice: z.coerce.number().positive("Harga satuan harus lebih dari 0"),
  deliveryDays: z.coerce.number().int().nonnegative("Tidak boleh negatif"),
  notes: z.string().optional(),
});

type FormValues = z.input<typeof schema>;
type FormOutput = z.output<typeof schema>;

function BidsContent() {
  const params = useSearchParams();
  const tenderId = params.get("tender_id") ?? "";
  const { user } = useAuth();
  const queryClient = useQueryClient();

  const [submitting, setSubmitting] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const [failure, setFailure] = useState<{ message: string; details?: Record<string, string> } | null>(null);

  const maySubmit = can(user?.role, "bid:submit");
  const maySelect = can(user?.role, "tender:write");
  const mayCompare = can(user?.role, "bid:read");

  const bids = useQuery({
    queryKey: ["bids", tenderId],
    queryFn: () => api.bids.list({ tender_id: tenderId, per_page: 50 }),
  });

  const comparison = useQuery({
    queryKey: ["bids", "compare", tenderId],
    queryFn: () => api.bids.compare(tenderId),
    enabled: Boolean(tenderId) && mayCompare,
  });

  const submitBid = useMutation({
    mutationFn: (values: FormOutput) =>
      api.bids.submit({
        tender_id: values.tenderId,
        vendor_id: values.vendorId,
        delivery_days: values.deliveryDays,
        notes: values.notes ?? "",
        currency: "IDR",
        items: [
          {
            name: values.itemName,
            quantity: values.quantity,
            unit_price: { amount: Math.round(values.unitPrice), currency: "IDR" },
          },
        ],
      }),
    onSuccess: async (bid) => {
      setNotice(`Penawaran tersimpan. Total dihitung server: ${formatMoney(bid.total.amount)}.`);
      setSubmitting(false);
      reset();
      await queryClient.invalidateQueries({ queryKey: ["bids"] });
    },
    onError: (error: unknown) => {
      if (error instanceof ApiError) {
        setFailure({ message: error.message, details: error.details });
        return;
      }
      setFailure({ message: error instanceof Error ? error.message : "Gagal mengirim penawaran" });
    },
  });

  const selectBid = useMutation({
    mutationFn: (id: string) => api.bids.select(id),
    onSuccess: async (bid) => {
      setNotice(`Penawaran ${bid.vendor_id} dipilih; penawaran lain otomatis ditolak.`);
      await queryClient.invalidateQueries({ queryKey: ["bids"] });
    },
    onError: (error: unknown) => {
      setFailure({ message: error instanceof Error ? error.message : "Gagal memilih penawaran" });
    },
  });

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors, isSubmitting },
  } = useForm<FormValues, unknown, FormOutput>({
    resolver: zodResolver(schema),
    defaultValues: { tenderId, deliveryDays: 14, quantity: 1 },
  });

  const rows: Bid[] = bids.data?.items ?? [];

  return (
    <>
      <header className="flex flex-wrap items-end justify-between gap-2">
        <div>
          <h1 className="text-lg font-semibold text-slate-900">Penawaran</h1>
          <p className="text-xs text-slate-500">
            {tenderId ? (
              <>
                Tender <span className="font-mono">{tenderId}</span>
              </>
            ) : (
              "Pilih tender dari halaman Tender untuk melihat penawarannya."
            )}
          </p>
        </div>
        {maySubmit && tenderId && <Button onClick={() => setSubmitting(true)}>Kirim penawaran</Button>}
      </header>

      {failure && <ErrorBanner message={failure.message} details={failure.details} />}
      {notice && <SuccessBanner message={notice} />}

      {mayCompare && tenderId && (
        <Card title="Perbandingan harga" subtitle="Diurutkan dari total terendah; dihitung service, bukan klien">
          {comparison.isLoading ? (
            <Spinner />
          ) : (comparison.data?.comparisons ?? []).length === 0 ? (
            <p className="rounded-md bg-slate-50 px-3 py-6 text-center text-sm text-slate-500">
              Belum ada penawaran yang bisa dibandingkan.
            </p>
          ) : (
            <Table
              rows={comparison.data?.comparisons ?? []}
              keyOf={(row) => row.bid_id}
              columns={[
                { header: "#", render: (row) => row.rank },
                { header: "Vendor", render: (row) => <span className="font-medium">{row.vendor_id}</span> },
                { header: "Total", render: (row) => formatMoney(row.total.amount, row.total.currency) },
                { header: "Kirim (hari)", render: (row) => row.delivery_days },
                {
                  header: "Selisih",
                  render: (row) => (row.price_delta_pct === 0 ? "-" : `+${row.price_delta_pct}%`),
                },
                {
                  header: "Tanda",
                  render: (row) => (
                    <div className="flex gap-1">
                      {row.lowest_price && <Badge tone="green">termurah</Badge>}
                      {row.fastest_delivery && <Badge tone="blue">tercepat</Badge>}
                    </div>
                  ),
                },
                {
                  header: "",
                  className: "text-right",
                  render: (row) =>
                    maySelect && row.status !== "selected" ? (
                      <Button
                        size="sm"
                        disabled={selectBid.isPending}
                        onClick={() => selectBid.mutate(row.bid_id)}
                      >
                        Pilih
                      </Button>
                    ) : (
                      <StatusBadge status={row.status} />
                    ),
                },
              ]}
            />
          )}
        </Card>
      )}

      <Card title="Daftar penawaran">
        {!tenderId ? (
          <p className="rounded-md bg-slate-50 px-3 py-6 text-center text-sm text-slate-500">
            Tidak ada tender yang dipilih.
          </p>
        ) : bids.isLoading ? (
          <Spinner />
        ) : bids.error ? (
          <ErrorBanner message={bids.error.message} />
        ) : (
          <Table
            rows={rows}
            keyOf={(row) => row.id}
            empty="Belum ada penawaran untuk tender ini."
            columns={[
              { header: "Vendor", render: (row) => <span className="font-medium">{row.vendor_id}</span> },
              { header: "Total", render: (row) => formatMoney(row.total.amount, row.total.currency) },
              { header: "Kirim (hari)", render: (row) => row.delivery_days },
              { header: "Status", render: (row) => <StatusBadge status={row.status} /> },
              { header: "Dikirim", render: (row) => formatDate(row.submitted_at) },
              { header: "Catatan", render: (row) => row.notes || "-" },
            ]}
          />
        )}
      </Card>

      <Modal open={submitting} title="Kirim penawaran" onClose={() => setSubmitting(false)}>
        <form
          className="space-y-3"
          onSubmit={handleSubmit((values) => {
            setFailure(null);
            setNotice(null);
            submitBid.mutate(values);
          })}
        >
          <Field label="Tender ID" error={errors.tenderId?.message}>
            <Input readOnly {...register("tenderId")} />
          </Field>
          <Field
            label="Vendor ID"
            error={errors.vendorId?.message}
            hint="Satu vendor hanya boleh mengirim satu penawaran per tender"
          >
            <Input placeholder="v-alpha" {...register("vendorId")} />
          </Field>
          <div className="grid grid-cols-3 gap-2">
            <Field label="Item" error={errors.itemName?.message}>
              <Input placeholder="Laptop 14 inch" {...register("itemName")} />
            </Field>
            <Field label="Qty" error={errors.quantity?.message}>
              <Input type="number" step="0.01" {...register("quantity")} />
            </Field>
            <Field label="Harga satuan" error={errors.unitPrice?.message}>
              <Input type="number" step="1" placeholder="11000000" {...register("unitPrice")} />
            </Field>
          </div>
          <Field label="Estimasi kirim (hari)" error={errors.deliveryDays?.message}>
            <Input type="number" step="1" {...register("deliveryDays")} />
          </Field>
          <Field label="Catatan" error={errors.notes?.message}>
            <Textarea rows={2} placeholder="garansi 3 tahun" {...register("notes")} />
          </Field>

          <div className="flex justify-end gap-2 pt-1">
            <Button type="button" variant="secondary" onClick={() => setSubmitting(false)}>
              Batal
            </Button>
            <Button type="submit" disabled={isSubmitting || submitBid.isPending}>
              {submitBid.isPending ? "Mengirim…" : "Kirim"}
            </Button>
          </div>
        </form>
      </Modal>
    </>
  );
}

export default function BidsPage() {
  // useSearchParams requires a Suspense boundary when the page is prerendered.
  return (
    <Suspense fallback={<Spinner />}>
      <BidsContent />
    </Suspense>
  );
}
