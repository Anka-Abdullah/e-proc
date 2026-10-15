"use client";

// Approval queue. Only the assignee (or an admin) may decide — enforced by the
// service, mirrored here by disabling the buttons for anyone else.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, formatDate, formatMoney } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { can } from "@/lib/rbac";
import type { Approval } from "@/lib/types";
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

export default function ApprovalsPage() {
  const { user } = useAuth();
  const queryClient = useQueryClient();

  const [page, setPage] = useState(1);
  const [status, setStatus] = useState("pending");
  const [mine, setMine] = useState(false);
  const [deciding, setDeciding] = useState<Approval | null>(null);
  const [decision, setDecision] = useState<{ approve: boolean; comment: string }>({
    approve: true,
    comment: "",
  });
  const [notice, setNotice] = useState<string | null>(null);
  const [failure, setFailure] = useState<string | null>(null);

  const mayAct = can(user?.role, "approval:act");

  const approvals = useQuery({
    queryKey: ["approvals", page, status, mine],
    queryFn: () =>
      api.approvals.list({
        page,
        per_page: 10,
        status,
        assigned_to: mine ? user?.id : undefined,
      }),
  });

  const decide = useMutation({
    mutationFn: (input: { id: string; approve: boolean; comment: string }) =>
      api.approvals.decide(input.id, { approve: input.approve, comment: input.comment }),
    onSuccess: async (approval) => {
      setNotice(`Approval ${approval.id} menjadi ${approval.status}.`);
      setDeciding(null);
      setDecision({ approve: true, comment: "" });
      await queryClient.invalidateQueries({ queryKey: ["approvals"] });
    },
    onError: (error: unknown) => {
      setFailure(error instanceof Error ? error.message : "Gagal menyimpan keputusan");
    },
  });

  const rows: Approval[] = approvals.data?.items ?? [];

  return (
    <>
      <header>
        <h1 className="text-lg font-semibold text-slate-900">Approval</h1>
        <p className="text-xs text-slate-500">
          Hanya assignee yang berhak memutuskan. Penolakan wajib disertai alasan.
        </p>
      </header>

      {failure && <ErrorBanner message={failure} />}
      {notice && <SuccessBanner message={notice} />}

      <Card>
        <div className="mb-3 flex flex-wrap items-center gap-2">
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
            <option value="approved">approved</option>
            <option value="rejected">rejected</option>
          </Select>
          <label className="flex items-center gap-2 text-xs text-slate-600">
            <input
              type="checkbox"
              checked={mine}
              onChange={(event) => {
                setMine(event.target.checked);
                setPage(1);
              }}
            />
            Hanya untuk saya
          </label>
        </div>

        {approvals.isLoading ? (
          <Spinner />
        ) : approvals.error ? (
          <ErrorBanner message={approvals.error.message} />
        ) : (
          <>
            <Table
              rows={rows}
              keyOf={(row) => row.id}
              empty="Tidak ada approval pada filter ini."
              columns={[
                {
                  header: "Dokumen",
                  render: (row) => (
                    <span className="font-medium">
                      {row.entity_type} · {row.entity_id.slice(0, 8)}…
                    </span>
                  ),
                },
                { header: "Nominal", render: (row) => formatMoney(row.amount.amount, row.amount.currency) },
                { header: "Assignee", render: (row) => row.assigned_to || "-" },
                { header: "Status", render: (row) => <StatusBadge status={row.status} /> },
                { header: "Dibuat", render: (row) => formatDate(row.created_at) },
                {
                  header: "Riwayat",
                  render: (row) =>
                    row.history.length === 0 ? (
                      <span className="text-xs text-slate-400">-</span>
                    ) : (
                      <span className="text-xs text-slate-500">
                        {row.history.map((step) => `${step.actor_id}: ${step.decision}`).join(", ")}
                      </span>
                    ),
                },
                {
                  header: "",
                  className: "text-right",
                  render: (row) => {
                    const isMine = !row.assigned_to || row.assigned_to === user?.id;
                    const allowed = mayAct && row.status === "pending" && (isMine || user?.role === "admin");
                    if (!allowed) return null;
                    return (
                      <div className="flex justify-end gap-1">
                        <Button
                          size="sm"
                          onClick={() => {
                            setFailure(null);
                            setDecision({ approve: true, comment: "" });
                            setDeciding(row);
                          }}
                        >
                          Setujui
                        </Button>
                        <Button
                          size="sm"
                          variant="danger"
                          onClick={() => {
                            setFailure(null);
                            setDecision({ approve: false, comment: "" });
                            setDeciding(row);
                          }}
                        >
                          Tolak
                        </Button>
                      </div>
                    );
                  },
                },
              ]}
            />
            <Pagination
              page={approvals.data?.meta.page ?? 1}
              perPage={approvals.data?.meta.per_page ?? 10}
              total={approvals.data?.meta.total ?? 0}
              onPage={setPage}
            />
          </>
        )}
      </Card>

      <Modal
        open={deciding !== null}
        title={decision.approve ? "Setujui permintaan" : "Tolak permintaan"}
        onClose={() => setDeciding(null)}
      >
        <form
          className="space-y-3"
          onSubmit={(event) => {
            event.preventDefault();
            if (!deciding) return;
            setFailure(null);
            setNotice(null);
            decide.mutate({ id: deciding.id, approve: decision.approve, comment: decision.comment });
          }}
        >
          <p className="text-sm text-slate-600">
            {deciding?.entity_type} <span className="font-mono">{deciding?.entity_id.slice(0, 12)}…</span> ·{" "}
            {deciding ? formatMoney(deciding.amount.amount, deciding.amount.currency) : ""}
          </p>
          <Field
            label="Komentar"
            hint={decision.approve ? "Opsional" : "Wajib, minimal 3 karakter untuk penolakan"}
          >
            <Input
              value={decision.comment}
              onChange={(event) => setDecision((prev) => ({ ...prev, comment: event.target.value }))}
              placeholder={decision.approve ? "disetujui, harga wajar" : "harga di atas anggaran"}
            />
          </Field>

          <div className="flex justify-end gap-2 pt-1">
            <Button type="button" variant="secondary" onClick={() => setDeciding(null)}>
              Batal
            </Button>
            <Button
              type="submit"
              variant={decision.approve ? "primary" : "danger"}
              disabled={decide.isPending}
            >
              {decide.isPending ? "Menyimpan…" : decision.approve ? "Setujui" : "Tolak"}
            </Button>
          </div>
        </form>
      </Modal>
    </>
  );
}
