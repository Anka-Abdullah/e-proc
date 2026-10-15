"use client";

// Vendor management: search, filter by status, create, and activate/suspend.
// Write actions are hidden when the signed-in role lacks vendor:write.

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { ApiError, api, formatDate } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { can } from "@/lib/rbac";
import type { Vendor } from "@/lib/types";
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
  code: z.string().min(1, "Kode wajib diisi").max(32, "Maksimal 32 karakter"),
  name: z.string().min(1, "Nama wajib diisi"),
  email: z.string().min(1, "Email wajib diisi").email("Format email tidak valid"),
  phone: z.string().optional(),
  address: z.string().optional(),
});

type FormValues = z.infer<typeof schema>;

export default function VendorsPage() {
  const { user } = useAuth();
  const queryClient = useQueryClient();

  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("");
  const [creating, setCreating] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const [failure, setFailure] = useState<{ message: string; details?: Record<string, string> } | null>(null);

  const mayWrite = can(user?.role, "vendor:write");

  const vendors = useQuery({
    queryKey: ["vendors", page, search, status],
    queryFn: () => api.vendors.list({ page, per_page: 10, search, status }),
  });

  const createVendor = useMutation({
    mutationFn: (values: FormValues) => api.vendors.create(values),
    onSuccess: async (vendor) => {
      setNotice(`Vendor ${vendor.code} dibuat.`);
      setCreating(false);
      reset();
      await queryClient.invalidateQueries({ queryKey: ["vendors"] });
    },
    onError: (error: unknown) => {
      if (error instanceof ApiError) {
        setFailure({ message: error.message, details: error.details });
        return;
      }
      setFailure({ message: error instanceof Error ? error.message : "Gagal menyimpan vendor" });
    },
  });

  const setVendorStatus = useMutation({
    mutationFn: ({ id, next }: { id: string; next: string }) => api.vendors.setStatus(id, next),
    onSuccess: async (vendor) => {
      setNotice(`Status ${vendor.code} menjadi ${vendor.status}.`);
      await queryClient.invalidateQueries({ queryKey: ["vendors"] });
    },
    onError: (error: unknown) => {
      setFailure({ message: error instanceof Error ? error.message : "Gagal mengubah status" });
    },
  });

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({ resolver: zodResolver(schema) });

  const rows: Vendor[] = vendors.data?.items ?? [];

  return (
    <>
      <header className="flex flex-wrap items-end justify-between gap-2">
        <div>
          <h1 className="text-lg font-semibold text-slate-900">Vendor</h1>
          <p className="text-xs text-slate-500">Master data supplier dan status keaktifannya.</p>
        </div>
        {mayWrite && <Button onClick={() => setCreating(true)}>Tambah vendor</Button>}
      </header>

      {failure && <ErrorBanner message={failure.message} details={failure.details} />}
      {notice && <SuccessBanner message={notice} />}

      <Card>
        <div className="mb-3 flex flex-wrap gap-2">
          <Input
            placeholder="Cari nama, kode, atau email"
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
            <option value="pending">pending</option>
            <option value="active">active</option>
            <option value="suspended">suspended</option>
          </Select>
        </div>

        {vendors.isLoading ? (
          <Spinner />
        ) : vendors.error ? (
          <ErrorBanner message={vendors.error.message} />
        ) : (
          <>
            <Table
              rows={rows}
              keyOf={(row) => row.id}
              empty="Belum ada vendor."
              columns={[
                { header: "Kode", render: (row) => <span className="font-medium">{row.code}</span> },
                { header: "Nama", render: (row) => row.name },
                { header: "Email", render: (row) => row.email },
                { header: "Status", render: (row) => <StatusBadge status={row.status} /> },
                { header: "Diperbarui", render: (row) => formatDate(row.updated_at) },
                {
                  header: "",
                  className: "text-right",
                  render: (row) =>
                    mayWrite ? (
                      <div className="flex justify-end gap-1">
                        {row.status !== "active" && (
                          <Button
                            size="sm"
                            variant="secondary"
                            disabled={setVendorStatus.isPending}
                            onClick={() => setVendorStatus.mutate({ id: row.id, next: "active" })}
                          >
                            Aktifkan
                          </Button>
                        )}
                        {row.status === "active" && (
                          <Button
                            size="sm"
                            variant="ghost"
                            disabled={setVendorStatus.isPending}
                            onClick={() => setVendorStatus.mutate({ id: row.id, next: "suspended" })}
                          >
                            Suspend
                          </Button>
                        )}
                      </div>
                    ) : null,
                },
              ]}
            />
            <Pagination
              page={vendors.data?.meta.page ?? 1}
              perPage={vendors.data?.meta.per_page ?? 10}
              total={vendors.data?.meta.total ?? 0}
              onPage={setPage}
            />
          </>
        )}
      </Card>

      <Modal open={creating} title="Tambah vendor" onClose={() => setCreating(false)}>
        <form
          className="space-y-3"
          onSubmit={handleSubmit((values) => {
            setFailure(null);
            setNotice(null);
            createVendor.mutate(values);
          })}
        >
          <Field label="Kode vendor" error={errors.code?.message} hint="Huruf besar, maksimal 32 karakter">
            <Input placeholder="VND-001" {...register("code")} />
          </Field>
          <Field label="Nama" error={errors.name?.message}>
            <Input placeholder="PT Contoh Sejahtera" {...register("name")} />
          </Field>
          <Field label="Email" error={errors.email?.message}>
            <Input placeholder="sales@contoh.co.id" {...register("email")} />
          </Field>
          <Field label="Telepon" error={errors.phone?.message}>
            <Input placeholder="021-5551234" {...register("phone")} />
          </Field>
          <Field label="Alamat" error={errors.address?.message}>
            <Input placeholder="Jl. Contoh No. 1, Jakarta" {...register("address")} />
          </Field>

          <div className="flex justify-end gap-2 pt-1">
            <Button type="button" variant="secondary" onClick={() => setCreating(false)}>
              Batal
            </Button>
            <Button type="submit" disabled={isSubmitting || createVendor.isPending}>
              {createVendor.isPending ? "Menyimpan…" : "Simpan"}
            </Button>
          </div>
        </form>
      </Modal>
    </>
  );
}
