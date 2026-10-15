"use client";

// Reusable UI primitives. They keep the pages short and the look consistent:
// each page composes these instead of restating Tailwind classes.

import { clsx } from "clsx";
import type { InputHTMLAttributes, ReactNode, SelectHTMLAttributes, TextareaHTMLAttributes } from "react";

export function Card({
  title,
  subtitle,
  actions,
  children,
  className,
}: {
  title?: string;
  subtitle?: string;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={clsx("rounded-lg border border-slate-200 bg-white shadow-sm", className)}>
      {(title || actions) && (
        <header className="flex items-start justify-between gap-4 border-b border-slate-100 px-5 py-4">
          <div>
            {title && <h2 className="text-sm font-semibold text-slate-900">{title}</h2>}
            {subtitle && <p className="mt-0.5 text-xs text-slate-500">{subtitle}</p>}
          </div>
          {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
        </header>
      )}
      <div className="px-5 py-4">{children}</div>
    </section>
  );
}

export function Button({
  children,
  variant = "primary",
  size = "md",
  ...props
}: {
  children: ReactNode;
  variant?: "primary" | "secondary" | "danger" | "ghost";
  size?: "sm" | "md";
} & React.ButtonHTMLAttributes<HTMLButtonElement>) {
  const variants = {
    primary: "bg-slate-900 text-white hover:bg-slate-700 disabled:bg-slate-300",
    secondary: "border border-slate-300 bg-white text-slate-700 hover:bg-slate-50",
    danger: "bg-rose-600 text-white hover:bg-rose-500 disabled:bg-rose-300",
    ghost: "text-slate-600 hover:bg-slate-100",
  };
  const sizes = { sm: "px-2.5 py-1 text-xs", md: "px-3.5 py-2 text-sm" };
  return (
    <button
      {...props}
      className={clsx(
        "inline-flex items-center justify-center gap-1.5 rounded-md font-medium transition disabled:cursor-not-allowed",
        variants[variant],
        sizes[size],
        props.className,
      )}
    >
      {children}
    </button>
  );
}

/** tone maps a status string to a colour, so every table badge agrees. */
const tones = {
  green: "bg-emerald-50 text-emerald-700 ring-emerald-200",
  amber: "bg-amber-50 text-amber-700 ring-amber-200",
  red: "bg-rose-50 text-rose-700 ring-rose-200",
  slate: "bg-slate-100 text-slate-600 ring-slate-200",
  blue: "bg-sky-50 text-sky-700 ring-sky-200",
  violet: "bg-violet-50 text-violet-700 ring-violet-200",
} as const;

export type Tone = keyof typeof tones;

export function Badge({ children, tone = "slate" }: { children: ReactNode; tone?: Tone }) {
  return (
    <span
      className={clsx(
        "inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium ring-1 ring-inset",
        tones[tone],
      )}
    >
      {children}
    </span>
  );
}

const statusTones: Record<string, Tone> = {
  active: "green",
  approved: "green",
  selected: "green",
  issued: "blue",
  published: "blue",
  submitted: "blue",
  shortlisted: "violet",
  pending: "amber",
  pending_approval: "amber",
  draft: "slate",
  closed: "slate",
  suspended: "red",
  rejected: "red",
  cancelled: "red",
};

/** StatusBadge renders a domain status with a consistent colour. */
export function StatusBadge({ status }: { status: string }) {
  const tone = statusTones[status] ?? "slate";
  return <Badge tone={tone}>{status.replace(/_/g, " ")}</Badge>;
}

export function Table<T>({
  rows,
  columns,
  empty = "Belum ada data.",
  keyOf,
}: {
  rows: T[];
  columns: { header: string; render: (row: T) => ReactNode; className?: string }[];
  empty?: string;
  keyOf: (row: T) => string;
}) {
  if (rows.length === 0) {
    return <EmptyState message={empty} />;
  }
  return (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse text-sm">
        <thead>
          <tr className="border-b border-slate-200 text-left text-xs uppercase tracking-wide text-slate-500">
            {columns.map((column) => (
              <th key={column.header} className={clsx("px-3 py-2 font-medium", column.className)}>
                {column.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={keyOf(row)} className="border-b border-slate-100 last:border-0 hover:bg-slate-50">
              {columns.map((column) => (
                <td key={column.header} className={clsx("px-3 py-2 text-slate-700", column.className)}>
                  {column.render(row)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function Pagination({
  page,
  perPage,
  total,
  onPage,
}: {
  page: number;
  perPage: number;
  total: number;
  onPage: (page: number) => void;
}) {
  const lastPage = Math.max(1, Math.ceil(total / perPage));
  return (
    <div className="mt-3 flex items-center justify-between text-xs text-slate-500">
      <span>
        {total} baris · halaman {page} dari {lastPage}
      </span>
      <div className="flex gap-1">
        <Button variant="secondary" size="sm" disabled={page <= 1} onClick={() => onPage(page - 1)}>
          Sebelumnya
        </Button>
        <Button variant="secondary" size="sm" disabled={page >= lastPage} onClick={() => onPage(page + 1)}>
          Berikutnya
        </Button>
      </div>
    </div>
  );
}

export function EmptyState({ message }: { message: string }) {
  return <p className="rounded-md bg-slate-50 px-3 py-6 text-center text-sm text-slate-500">{message}</p>;
}

export function ErrorBanner({ message, details }: { message: string; details?: Record<string, string> }) {
  return (
    <div className="rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">
      <p className="font-medium">{message}</p>
      {details && (
        <ul className="mt-1 list-inside list-disc text-xs">
          {Object.entries(details).map(([field, detail]) => (
            <li key={field}>
              <span className="font-medium">{field}</span>: {detail}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

export function SuccessBanner({ message }: { message: string }) {
  return (
    <div className="rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700">
      {message}
    </div>
  );
}

export function Spinner({ label = "Memuat…" }: { label?: string }) {
  return (
    <div className="flex items-center gap-2 px-1 py-6 text-sm text-slate-500">
      <span className="h-3 w-3 animate-spin rounded-full border-2 border-slate-300 border-t-slate-600" />
      {label}
    </div>
  );
}

export function Field({
  label,
  error,
  children,
  hint,
}: {
  label: string;
  error?: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <label className="block">
      <span className="mb-1 block text-xs font-medium text-slate-600">{label}</span>
      {children}
      {hint && !error && <span className="mt-1 block text-[11px] text-slate-400">{hint}</span>}
      {error && <span className="mt-1 block text-[11px] text-rose-600">{error}</span>}
    </label>
  );
}

const inputClass =
  "w-full rounded-md border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none focus:border-slate-500 focus:ring-1 focus:ring-slate-500 disabled:bg-slate-50";

export function Input(props: InputHTMLAttributes<HTMLInputElement>) {
  return <input {...props} className={clsx(inputClass, props.className)} />;
}

export function Select(props: SelectHTMLAttributes<HTMLSelectElement>) {
  return <select {...props} className={clsx(inputClass, props.className)} />;
}

export function Textarea(props: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <textarea {...props} className={clsx(inputClass, props.className)} />;
}

export function Modal({
  open,
  title,
  onClose,
  children,
}: {
  open: boolean;
  title: string;
  onClose: () => void;
  children: ReactNode;
}) {
  if (!open) return null;
  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-slate-900/40 p-4 pt-16">
      <div className="w-full max-w-lg rounded-lg bg-white shadow-xl">
        <header className="flex items-center justify-between border-b border-slate-100 px-5 py-3">
          <h2 className="text-sm font-semibold text-slate-900">{title}</h2>
          <Button variant="ghost" size="sm" onClick={onClose} aria-label="Tutup">
            Tutup
          </Button>
        </header>
        <div className="px-5 py-4">{children}</div>
      </div>
    </div>
  );
}

/** StatCard shows one headline number on the dashboard and report pages. */
export function StatCard({
  label,
  value,
  hint,
  tone = "slate",
}: {
  label: string;
  value: string | number;
  hint?: string;
  tone?: Tone;
}) {
  return (
    <div className="rounded-lg border border-slate-200 bg-white px-4 py-3 shadow-sm">
      <p className="text-xs text-slate-500">{label}</p>
      <p className="mt-1 text-xl font-semibold text-slate-900">{value}</p>
      {hint && (
        <p className="mt-1 text-[11px] text-slate-500">
          <Badge tone={tone}>{hint}</Badge>
        </p>
      )}
    </div>
  );
}
