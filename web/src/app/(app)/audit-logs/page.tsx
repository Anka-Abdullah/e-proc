"use client";

// Audit log: read-only, filled by the audit service from domain events. There is
// deliberately no create/edit control here.

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, formatDate } from "@/lib/api";
import type { AuditEntry } from "@/lib/types";
import {
  Button,
  Card,
  ErrorBanner,
  Input,
  Pagination,
  Select,
  Spinner,
  Table,
} from "@/components/ui";

const ACTIONS = [
  "vendor.created",
  "vendor.updated",
  "tender.published",
  "tender.closed",
  "bid.submitted",
  "bid.selected",
  "po.created",
  "po.approved",
  "po.issued",
  "po.rejected",
  "po.closed",
  "approval.requested",
  "approval.decided",
];

export default function AuditLogsPage() {
  const [page, setPage] = useState(1);
  const [entityType, setEntityType] = useState("");
  const [action, setAction] = useState("");
  const [entityId, setEntityId] = useState("");
  const [expanded, setExpanded] = useState<string | null>(null);

  const logs = useQuery({
    queryKey: ["audit-logs", page, entityType, action, entityId],
    queryFn: () =>
      api.auditLogs.list({ page, per_page: 15, entity_type: entityType, action, entity_id: entityId }),
  });

  const rows: AuditEntry[] = logs.data?.items ?? [];

  return (
    <>
      <header>
        <h1 className="text-lg font-semibold text-slate-900">Audit Log</h1>
        <p className="text-xs text-slate-500">
          Append-only: entri dibuat audit-service dari event domain, tidak ada endpoint tulis.
        </p>
      </header>

      <Card>
        <div className="mb-3 flex flex-wrap gap-2">
          <Select
            value={entityType}
            onChange={(event) => {
              setEntityType(event.target.value);
              setPage(1);
            }}
            className="max-w-44"
          >
            <option value="">Semua entity</option>
            {["vendor", "tender", "bid", "purchase_order", "approval"].map((value) => (
              <option key={value} value={value}>
                {value}
              </option>
            ))}
          </Select>
          <Select
            value={action}
            onChange={(event) => {
              setAction(event.target.value);
              setPage(1);
            }}
            className="max-w-52"
          >
            <option value="">Semua aksi</option>
            {ACTIONS.map((value) => (
              <option key={value} value={value}>
                {value}
              </option>
            ))}
          </Select>
          <Input
            placeholder="Entity ID"
            value={entityId}
            onChange={(event) => {
              setEntityId(event.target.value);
              setPage(1);
            }}
            className="max-w-64"
          />
        </div>

        {logs.isLoading ? (
          <Spinner />
        ) : logs.error ? (
          <ErrorBanner message={logs.error.message} />
        ) : (
          <>
            <Table
              rows={rows}
              keyOf={(row) => row.id}
              empty="Belum ada entri audit pada filter ini."
              columns={[
                { header: "Waktu", render: (row) => formatDate(row.created_at) },
                { header: "Aksi", render: (row) => <span className="font-medium">{row.action}</span> },
                { header: "Entity", render: (row) => row.entity_type },
                {
                  header: "Entity ID",
                  render: (row) => <span className="font-mono text-xs">{row.entity_id.slice(0, 12)}…</span>,
                },
                { header: "Actor", render: (row) => row.actor_id || "-" },
                {
                  header: "",
                  className: "text-right",
                  render: (row) => (
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => setExpanded(expanded === row.id ? null : row.id)}
                    >
                      {expanded === row.id ? "Tutup" : "Detail"}
                    </Button>
                  ),
                },
              ]}
            />

            {expanded && (
              <div className="mt-3 rounded-md border border-slate-200 bg-slate-50 p-3">
                {(() => {
                  const entry = rows.find((row) => row.id === expanded);
                  if (!entry) return null;
                  return (
                    <div className="space-y-2 text-xs text-slate-600">
                      <p>
                        <span className="font-medium">Event ID:</span> {entry.event_id}
                      </p>
                      <p>
                        <span className="font-medium">Ringkasan:</span> {entry.summary ?? "-"}
                      </p>
                      <pre className="max-h-64 overflow-auto rounded bg-white p-2 text-[11px] leading-relaxed text-slate-700">
                        {JSON.stringify(entry.payload ?? {}, null, 2)}
                      </pre>
                    </div>
                  );
                })()}
              </div>
            )}

            <Pagination
              page={logs.data?.meta.page ?? 1}
              perPage={logs.data?.meta.per_page ?? 15}
              total={logs.data?.meta.total ?? 0}
              onPage={setPage}
            />
          </>
        )}
      </Card>
    </>
  );
}
