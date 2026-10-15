"use client";

// Tender management: create a draft, publish it, and close it. The publish and
// close buttons follow the same state machine the service enforces.

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { ApiError, api, formatDate, formatMoney } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { can } from "@/lib/rbac";
import type { Tender } from "@/lib/types";
import {
  Button,
  Card,
  ErrorBanner,
  Field,
  Input,
  Modal,
  Pagination,
  Select,
  Spinner,
  StatusBadge,
  SuccessBanner,
  Table,
} from "@/components/ui";

const schema = z.object({
  code: z.string().min(1, "Kode wajib diisi").max(40, "Maksimal 40 karakter"),
  title: z.string().min(1, "Judul wajib diisi").max(240, "Maksimal 240 karakter"),
  description: z.string().optional(),
  itemName: z.string().min(1, "Nama item wajib diisi"),
  itemQuantity: z.coerce.number().positive("Kuantitas harus lebih dari 0"),
  itemUnit: z.string().optional(),
  budget: z.coerce.number().nonnegative("Budget tidak boleh negatif"),
  closeAt: z.string().min(1, "Deadline wajib diisi"),
});

type FormValues = z.input<typeof schema>;
type FormOutput = z.output<typeof schema>;

export default function TendersPage() {
  const { user } = useAuth();
  const queryClient = useQueryClient();

  const [page, setPage] = useState(1);
  const [status, setStatus] = useState("");
  const [search, setSearch] = useState("");
  const [creating, setCreating] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const [failure, setFailure] = useState<{ message: string; details?: Record<string, string> } | null>(null);

  const mayCreate = can(user?.role, "tender:write");
  const mayPublish = can(user?.role, "tender:publish");

  const tenders = useQuery({
    queryKey: ["tenders", page, status, search],
    queryFn: () => api.tenders.list({ page, per_page: 10, status, search }),
  });

  const createTender = useMutation({
    mutationFn: (values: FormOutput) =>
      api.tenders.create({
        code: values.code,
        title: values.title,
        description: values.description ?? "",
        items: [
          {
            name: values.itemName,
            quantity: values.itemQuantity,
            unit: values.itemUnit ?? "",
            specification: "",
          },
        ],
        budget: { amount: Math.round(values.budget), currency: "IDR" },
        // The input is datetime-local; the API expects RFC3339 in UTC.
        close_at: new Date(values.closeAt).toISOString(),
      }),
    onSuccess: async (tender) => {
      setNotice(`Tender ${tender.code} dibuat sebagai draft.`);
      setCreating(false);
      reset();
      await queryClient.invalidateQueries({ queryKey: ["tenders"] });
    },
    onError: (error: unknown) => {
      if (error instanceof ApiError) {
        setFailure({ message: error.message, details: error.details });
        return;
      }
      setFailure({ message: error instanceof Error ? error.message : "Gagal membuat tender" });
    },
  });

  const publishTender = useMutation({
    mutationFn: (id: string) => api.tenders.publish(id),
    onSuccess: async (tender) => {
      setNotice(`Tender ${tender.code} dipublikasikan.`);
      await queryClient.invalidateQueries({ queryKey: ["tenders"] });
    },
    onError: (error: unknown) => {
      setFailure({ message: error instanceof Error ? error.message : "Gagal mempublikasikan tender" });
    },
  });

  const closeTender = useMutation({
    mutationFn: (id: string) => api.tenders.close(id),
    onSuccess: async (tender) => {
      setNotice(`Tender ${tender.code} ditutup.`);
      await queryClient.invalidateQueries({ queryKey: ["tenders"] });
    },
    onError: (error: unknown) => {
      setFailure({ message: error instanceof Error ? error.message : "Gagal menutup tender" });
    },
  });

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors, isSubmitting },
  } = useForm<FormValues, unknown, FormOutput>({ resolver: zodResolver(schema) });

  const rows: Tender[] = tenders.data?.items ?? [];

  return (
    <>
      <header className="flex flex-wrap items-end justify-between gap-2">
        <div>
          <h1 className="text-lg font-semibold text-slate-900">Tender</h1>
          <p className="text-xs text-slate-500">
            Draft → published → closed. Tender yang lewat deadline ditutup otomatis oleh scheduler.
          </p>
        </div>
        {mayCreate && <Button onClick={() => setCreating(true)}>Buat tender</Button>}
      </header>

      {failure && <ErrorBanner message={failure.message} details={failure.details} />}
      {notice && <SuccessBanner message={notice} />}

      <Card>
        <div className="mb-3 flex flex-wrap gap-2">
          <Input
            placeholder="Cari kode atau judul"
            value={search}
            onChange={(event) => {
              setSearch(event.target.value);
              setPage(1);
            }}
            className="max-w-xs"
          />
          <Select
            value={status}
            onChange={(event) => {
              setStatus(event.target.value);
              setPage(1);
            }}
            className="max-w-40"
          >
            <option value="">Semua status</option>
            <option value="draft">draft</option>
            <option value="published">published</option>
            <option value="closed">closed</option>
          </Select>
        </div>

        {tenders.isLoading ? (
          <Spinner />
        ) : tenders.error ? (
          <ErrorBanner message={tenders.error.message} />
        ) : (
          <>
            <Table
              rows={rows}
              keyOf={(row) => row.id}
              empty="Belum ada tender."
              columns={[
                { header: "Kode", render: (row) => <span className="font-medium">{row.code}</span> },
                { header: "Judul", render: (row) => row.title },
                {
                  header: "Budget",
                  render: (row) => formatMoney(row.budget.amount, row.budget.currency),
                },
                {
                  header: "Deadline",
                  render: (row) => formatDate(row.close_at),
                },
                { header: "Status", render: (row) => <StatusBadge status={row.status} /> },
                {
                  header: "",
                  className: "text-right",
                  render: (row) => (
                    <div className="flex justify-end gap-1">
                      <Link href={`/bids?tender_id=${row.id}`}>
                        <Button size="sm" variant="ghost">
                          Penawaran
                        </Button>
                      </Link>
                      {mayPublish && row.status === "draft" && (
                        <Button
                          size="sm"
                          disabled={publishTender.isPending}
                          onClick={() => publishTender.mutate(row.id)}
                        >
                          Publikasikan
                        </Button>
                      )}
                      {mayPublish && row.status === "published" && (
                        <Button
                          size="sm"
                          variant="secondary"
                          disabled={closeTender.isPending}
                          onClick={() => closeTender.mutate(row.id)}
                        >
                          Tutup
                        </Button>
                      )}
                    </div>
                  ),
                },
              ]}
            />
            <Pagination
              page={tenders.data?.meta.page ?? 1}
              perPage={tenders.data?.meta.per_page ?? 10}
              total={tenders.data?.meta.total ?? 0}
              onPage={setPage}
            />
          </>
        )}
      </Card>

      <Modal open={creating} title="Buat tender" onClose={() => setCreating(false)}>
        <form
          className="space-y-3"
          onSubmit={handleSubmit((values) => {
            setFailure(null);
            setNotice(null);
            createTender.mutate(values);
          })}
        >
          <Field label="Kode" error={errors.code?.message}>
            <Input placeholder="TND-001" {...register("code")} />
          </Field>
          <Field label="Judul" error={errors.title?.message}>
            <Input placeholder="Pengadaan Laptop 2026" {...register("title")} />
          </Field>
          <Field label="Deskripsi" error={errors.description?.message}>
            <Input placeholder="Kebutuhan tim IT" {...register("description")} />
          </Field>
          <div className="grid grid-cols-3 gap-2">
            <Field label="Item" error={errors.itemName?.message}>
              <Input placeholder="Laptop 14 inch" {...register("itemName")} />
            </Field>
            <Field label="Qty" error={errors.itemQuantity?.message}>
              <Input type="number" step="0.01" placeholder="25" {...register("itemQuantity")} />
            </Field>
            <Field label="Satuan" error={errors.itemUnit?.message}>
              <Input placeholder="unit" {...register("itemUnit")} />
            </Field>
          </div>
          <Field label="Budget (IDR)" error={errors.budget?.message}>
            <Input type="number" step="1" placeholder="750000000" {...register("budget")} />
          </Field>
          <Field
            label="Deadline penawaran"
            error={errors.closeAt?.message}
            hint="Harus di masa depan, jika tidak publish akan ditolak service"
          >
            <Input type="datetime-local" {...register("closeAt")} />
          </Field>

          <div className="flex justify-end gap-2 pt-1">
            <Button type="button" variant="secondary" onClick={() => setCreating(false)}>
              Batal
            </Button>
            <Button type="submit" disabled={isSubmitting || createTender.isPending}>
              {createTender.isPending ? "Menyimpan…" : "Simpan sebagai draft"}
            </Button>
          </div>
        </form>
      </Modal>
    </>
  );
}
