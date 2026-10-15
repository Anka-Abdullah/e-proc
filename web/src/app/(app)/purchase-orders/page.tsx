"use client";

// Purchase orders: create from a selected bid, walk the status machine, and
// request approval. The threshold rule is applied by the service, so the UI only
// reports what came back.

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { ApiError, api, formatDate, formatMoney } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { can } from "@/lib/rbac";
import type { POStatus, PurchaseOrder } from "@/lib/types";
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

const createSchema = z.object({
  tenderId: z.string().min(1, "Tender wajib diisi"),
  bidId: z.string().min(1, "Bid wajib diisi"),
  vendorId: z.string().min(1, "Vendor wajib diisi"),
  lineName: z.string().min(1, "Nama item wajib diisi"),
  quantity: z.coerce.number().positive("Kuantitas harus lebih dari 0"),
  unitPrice: z.coerce.number().positive("Harga satuan harus lebih dari 0"),
  notes: z.string().optional(),
});

type CreateValues = z.input<typeof createSchema>;
type CreateOutput = z.output<typeof createSchema>;

const approvalSchema = z.object({
  amount: z.coerce.number().positive("Nominal harus lebih dari 0"),
  assignedTo: z.string().min(1, "Approver wajib diisi"),
  comment: z.string().optional(),
});

type ApprovalValues = z.input<typeof approvalSchema>;
type ApprovalOutput = z.output<typeof approvalSchema>;

export default function PurchaseOrdersPage() {
  const { user } = useAuth();
  const queryClient = useQueryClient();

  const [page, setPage] = useState(1);
  const [status, setStatus] = useState("");
  const [creating, setCreating] = useState(false);
  const [approvalFor, setApprovalFor] = useState<PurchaseOrder | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [failure, setFailure] = useState<{ message: string; details?: Record<string, string> } | null>(null);

  const mayWrite = can(user?.role, "po:write");

  const orders = useQuery({
    queryKey: ["purchase-orders", page, status],
    queryFn: () => api.purchaseOrders.list({ page, per_page: 10, status }),
  });

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ["purchase-orders"] });

  const createOrder = useMutation({
    mutationFn: (values: CreateOutput) =>
      api.purchaseOrders.create({
        tender_id: values.tenderId,
        bid_id: values.bidId,
        vendor_id: values.vendorId,
        notes: values.notes ?? "",
        currency: "IDR",
        lines: [
          {
            name: values.lineName,
            quantity: values.quantity,
            unit_price: { amount: Math.round(values.unitPrice), currency: "IDR" },
          },
        ],
      }),
    onSuccess: async (order) => {
      setNotice(
        `PO ${order.number} dibuat (${formatMoney(order.total.amount)}), status ${order.status}.`,
      );
      setCreating(false);
      resetCreate();
      await invalidate();
    },
    onError: (error: unknown) => {
      if (error instanceof ApiError) {
        setFailure({ message: error.message, details: error.details });
        return;
      }
      setFailure({ message: error instanceof Error ? error.message : "Gagal membuat PO" });
    },
  });

  const updateStatus = useMutation({
    mutationFn: ({ id, next }: { id: string; next: POStatus }) => api.purchaseOrders.setStatus(id, next),
    onSuccess: async (order, variables) => {
      setNotice(`PO ${order.number} -> ${variables.next}.`);
      await invalidate();
    },
    onError: (error: unknown) => {
      setFailure({ message: error instanceof Error ? error.message : "Transisi status ditolak" });
    },
  });

  const requestApproval = useMutation({
    mutationFn: async ({ order, values }: { order: PurchaseOrder; values: ApprovalOutput }) => {
      const approval = await api.approvals.request({
        entity_type: "purchase_order",
        entity_id: order.id,
        amount: Math.round(values.amount),
        currency: order.total.currency,
        assigned_to: values.assignedTo,
        comment: values.comment ?? "",
      });
      // Link the approval to the order so the PO carries its approval id.
      await api.purchaseOrders.setStatus(order.id, "pending_approval", approval.id);
      return approval;
    },
    onSuccess: async (approval) => {
      setNotice(`Permintaan approval dikirim (id ${approval.id}, status ${approval.status}).`);
      setApprovalFor(null);
      await invalidate();
    },
    onError: (error: unknown) => {
      setFailure({ message: error instanceof Error ? error.message : "Gagal meminta approval" });
    },
  });

  const {
    register: registerCreate,
    handleSubmit: handleCreateSubmit,
    reset: resetCreate,
    formState: { errors: createErrors, isSubmitting: creatingSubmitting },
  } = useForm<CreateValues, unknown, CreateOutput>({ resolver: zodResolver(createSchema) });

  const {
    register: registerApproval,
    handleSubmit: handleApprovalSubmit,
    formState: { errors: approvalErrors },
  } = useForm<ApprovalValues, unknown, ApprovalOutput>({ resolver: zodResolver(approvalSchema) });

  const rows: PurchaseOrder[] = orders.data?.items ?? [];

  return (
    <>
      <header className="flex flex-wrap items-end justify-between gap-2">
        <div>
          <h1 className="text-lg font-semibold text-slate-900">Purchase Order</h1>
          <p className="text-xs text-slate-500">
            PO di atas threshold otomatis berstatus pending_approval; sisanya draft.
          </p>
        </div>
        {mayWrite && <Button onClick={() => setCreating(true)}>Buat PO</Button>}
      </header>

      {failure && <ErrorBanner message={failure.message} details={failure.details} />}
      {notice && <SuccessBanner message={notice} />}

      <Card>
        <div className="mb-3">
          <Select
            value={status}
            onChange={(event) => {
              setStatus(event.target.value);
              setPage(1);
            }}
            className="max-w-48"
          >
            <option value="">Semua status</option>
            {["draft", "pending_approval", "approved", "rejected", "issued", "closed"].map((value) => (
              <option key={value} value={value}>
                {value}
              </option>
            ))}
          </Select>
        </div>

        {orders.isLoading ? (
          <Spinner />
        ) : orders.error ? (
          <ErrorBanner message={orders.error.message} />
        ) : (
          <>
            <Table
              rows={rows}
              keyOf={(row) => row.id}
              empty="Belum ada purchase order."
              columns={[
                { header: "Nomor", render: (row) => <span className="font-medium">{row.number}</span> },
                { header: "Vendor", render: (row) => row.vendor_id },
                { header: "Total", render: (row) => formatMoney(row.total.amount, row.total.currency) },
                { header: "Status", render: (row) => <StatusBadge status={row.status} /> },
                { header: "Dibuat", render: (row) => formatDate(row.created_at) },
                {
                  header: "",
                  className: "text-right",
                  render: (row) =>
                    mayWrite ? (
                      <div className="flex justify-end gap-1">
                        {row.status === "draft" && (
                          <>
                            <Button size="sm" variant="secondary" onClick={() => setApprovalFor(row)}>
                              Minta approval
                            </Button>
                            <Button
                              size="sm"
                              disabled={updateStatus.isPending}
                              onClick={() => updateStatus.mutate({ id: row.id, next: "issued" })}
                            >
                              Terbitkan
                            </Button>
                          </>
                        )}
                        {row.status === "approved" && (
                          <Button
                            size="sm"
                            disabled={updateStatus.isPending}
                            onClick={() => updateStatus.mutate({ id: row.id, next: "issued" })}
                          >
                            Terbitkan
                          </Button>
                        )}
                        {(row.status === "issued" || row.status === "approved") && (
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => api.purchaseOrders.close(row.id).then(invalidate)}
                          >
                            Tutup
                          </Button>
                        )}
                      </div>
                    ) : null,
                },
              ]}
            />
            <Pagination
              page={orders.data?.meta.page ?? 1}
              perPage={orders.data?.meta.per_page ?? 10}
              total={orders.data?.meta.total ?? 0}
              onPage={setPage}
            />
          </>
        )}
      </Card>

      <Modal open={creating} title="Buat purchase order" onClose={() => setCreating(false)}>
        <form
          className="space-y-3"
          onSubmit={handleCreateSubmit((values) => {
            setFailure(null);
            setNotice(null);
            createOrder.mutate(values);
          })}
        >
          <Field label="Tender ID" error={createErrors.tenderId?.message}>
            <Input placeholder="tender id dari halaman Tender" {...registerCreate("tenderId")} />
          </Field>
          <Field label="Bid ID" error={createErrors.bidId?.message} hint="Ambil dari penawaran yang dipilih">
            <Input placeholder="bid id" {...registerCreate("bidId")} />
          </Field>
          <Field label="Vendor ID" error={createErrors.vendorId?.message}>
            <Input placeholder="v-beta" {...registerCreate("vendorId")} />
          </Field>
          <div className="grid grid-cols-3 gap-2">
            <Field label="Item" error={createErrors.lineName?.message}>
              <Input placeholder="Laptop 14 inch" {...registerCreate("lineName")} />
            </Field>
            <Field label="Qty" error={createErrors.quantity?.message}>
              <Input type="number" step="0.01" {...registerCreate("quantity")} />
            </Field>
            <Field label="Harga satuan" error={createErrors.unitPrice?.message}>
              <Input type="number" step="1" placeholder="9950000" {...registerCreate("unitPrice")} />
            </Field>
          </div>
          <Field label="Catatan" error={createErrors.notes?.message}>
            <Input placeholder="pengadaan laptop" {...registerCreate("notes")} />
          </Field>

          <div className="flex justify-end gap-2 pt-1">
            <Button type="button" variant="secondary" onClick={() => setCreating(false)}>
              Batal
            </Button>
            <Button type="submit" disabled={creatingSubmitting || createOrder.isPending}>
              {createOrder.isPending ? "Menyimpan…" : "Simpan"}
            </Button>
          </div>
        </form>
      </Modal>

      <Modal
        open={approvalFor !== null}
        title={`Minta approval ${approvalFor?.number ?? ""}`}
        onClose={() => setApprovalFor(null)}
      >
        <form
          className="space-y-3"
          onSubmit={handleApprovalSubmit((values) => {
            if (!approvalFor) return;
            setFailure(null);
            setNotice(null);
            requestApproval.mutate({ order: approvalFor, values });
          })}
        >
          <Field
            label="Nominal (IDR)"
            error={approvalErrors.amount?.message}
            hint={`Total PO: ${approvalFor ? formatMoney(approvalFor.total.amount) : "-"}`}
          >
            <Input
              type="number"
              step="1"
              defaultValue={approvalFor?.total.amount}
              {...registerApproval("amount")}
            />
          </Field>
          <Field label="Assignee approver" error={approvalErrors.assignedTo?.message}>
            <Input placeholder="u-approver" {...registerApproval("assignedTo")} />
          </Field>
          <Field label="Catatan" error={approvalErrors.comment?.message}>
            <Input placeholder="mohon persetujuan pengadaan" {...registerApproval("comment")} />
          </Field>

          <div className="flex justify-end gap-2 pt-1">
            <Button type="button" variant="secondary" onClick={() => setApprovalFor(null)}>
              Batal
            </Button>
            <Button type="submit" disabled={requestApproval.isPending}>
              {requestApproval.isPending ? "Mengirim…" : "Kirim permintaan"}
            </Button>
          </div>
        </form>
      </Modal>
    </>
  );
}
