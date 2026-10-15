// Typed client for the platform API. Every call goes through the gateway, which
// terminates authentication; this module only carries the token and unwraps the
// shared response envelope.

import type {
  Approval,
  AuditEntry,
  Bid,
  BidComparison,
  Envelope,
  LoginResult,
  Money,
  Page,
  PageMeta,
  PurchaseOrder,
  Tender,
  TenderSummary,
  ApprovalSummary,
  User,
  Vendor,
  VendorSpend,
} from "./types";

export const API_BASE =
  process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8000";

const TOKEN_KEY = "eproc.token";

/** ApiError carries the server's error envelope plus the HTTP status. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details?: Record<string, string>;

  constructor(status: number, code: string, message: string, details?: Record<string, string>) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.details = details;
  }

  /** isAuth reports whether the caller must log in again. */
  get isAuth(): boolean {
    return this.status === 401;
  }
}

// The token lives in memory for the session and is mirrored to localStorage so a
// reload keeps the user signed in. An httpOnly cookie set by the gateway is the
// proper hardening step; this is the MVP trade-off.
let inMemoryToken: string | null = null;

export function setToken(token: string | null): void {
  inMemoryToken = token;
  if (typeof window === "undefined") return;
  if (token) {
    window.localStorage.setItem(TOKEN_KEY, token);
  } else {
    window.localStorage.removeItem(TOKEN_KEY);
  }
}

export function getToken(): string | null {
  if (inMemoryToken) return inMemoryToken;
  if (typeof window === "undefined") return null;
  inMemoryToken = window.localStorage.getItem(TOKEN_KEY);
  return inMemoryToken;
}

interface RequestOptions {
  method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  body?: unknown;
  query?: Record<string, string | number | undefined | null>;
  signal?: AbortSignal;
}

function buildUrl(path: string, query?: RequestOptions["query"]): string {
  const url = new URL(API_BASE + path);
  if (query) {
    for (const [key, value] of Object.entries(query)) {
      if (value === undefined || value === null || value === "") continue;
      url.searchParams.set(key, String(value));
    }
  }
  return url.toString();
}

/** request performs one API call and returns the decoded envelope. */
async function request<T>(path: string, options: RequestOptions = {}): Promise<Envelope<T>> {
  const token = getToken();
  const headers: Record<string, string> = { Accept: "application/json" };
  if (options.body !== undefined) headers["Content-Type"] = "application/json";
  if (token) headers.Authorization = `Bearer ${token}`;

  const response = await fetch(buildUrl(path, options.query), {
    method: options.method ?? "GET",
    headers,
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
    signal: options.signal,
    cache: "no-store",
  });

  if (response.status === 204) {
    return {};
  }

  const text = await response.text();
  let parsed: Envelope<T> = {};
  if (text) {
    try {
      parsed = JSON.parse(text) as Envelope<T>;
    } catch {
      throw new ApiError(response.status, "invalid_response", "server returned a non-JSON response");
    }
  }

  if (!response.ok) {
    const error = parsed.error;
    throw new ApiError(
      response.status,
      error?.code ?? "request_failed",
      error?.message ?? `request failed with status ${response.status}`,
      error?.details,
    );
  }
  return parsed;
}

/** data performs a call and returns its payload, requiring one to exist. */
async function data<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const envelope = await request<T>(path, options);
  if (envelope.data === undefined) {
    throw new ApiError(500, "empty_response", "server returned no data for " + path);
  }
  return envelope.data;
}

/** list performs a call and returns items plus pagination meta. */
async function list<T>(path: string, options: RequestOptions = {}): Promise<Page<T>> {
  const envelope = await request<T[]>(path, options);
  return {
    items: envelope.data ?? [],
    meta: envelope.meta ?? { page: 1, per_page: 20, total: envelope.data?.length ?? 0 },
  };
}

export interface ListParams {
  page?: number;
  per_page?: number;
  search?: string;
  status?: string;
  [key: string]: string | number | undefined;
}

export const api = {
  // --- auth ---
  login: (body: { tenant_id: string; email: string; password: string }) =>
    data<LoginResult>("/api/v1/auth/login", { method: "POST", body }),
  me: () => data<User>("/api/v1/auth/me"),
  logout: () => request<never>("/api/v1/auth/logout", { method: "POST" }),
  register: (body: { email: string; name: string; password: string; role: string; tenant_id?: string }) =>
    data<User>("/api/v1/auth/register", { method: "POST", body }),

  // --- vendors ---
  vendors: {
    list: (params: ListParams = {}) => list<Vendor>("/api/v1/vendors", { query: params }),
    get: (id: string) => data<Vendor>(`/api/v1/vendors/${id}`),
    create: (body: Partial<Vendor>) => data<Vendor>("/api/v1/vendors", { method: "POST", body }),
    update: (id: string, body: Partial<Vendor>) =>
      data<Vendor>(`/api/v1/vendors/${id}`, { method: "PUT", body }),
    setStatus: (id: string, status: string) =>
      data<Vendor>(`/api/v1/vendors/${id}/status`, { method: "PATCH", body: { status } }),
  },

  // --- tenders ---
  tenders: {
    list: (params: ListParams = {}) => list<Tender>("/api/v1/tenders", { query: params }),
    get: (id: string) => data<Tender>(`/api/v1/tenders/${id}`),
    create: (body: unknown) => data<Tender>("/api/v1/tenders", { method: "POST", body }),
    publish: (id: string) => data<Tender>(`/api/v1/tenders/${id}/publish`, { method: "POST" }),
    close: (id: string) => data<Tender>(`/api/v1/tenders/${id}/close`, { method: "POST" }),
  },

  // --- bids ---
  bids: {
    list: (params: ListParams = {}) => list<Bid>("/api/v1/bids", { query: params }),
    get: (id: string) => data<Bid>(`/api/v1/bids/${id}`),
    submit: (body: unknown) => data<Bid>("/api/v1/bids", { method: "POST", body }),
    compare: (tenderId: string) =>
      data<{ comparisons: BidComparison[]; lowest_total: Money }>(
        `/api/v1/bids/compare/${tenderId}`,
      ),
    select: (id: string) => data<Bid>(`/api/v1/bids/${id}/select`, { method: "POST" }),
  },

  // --- purchase orders ---
  purchaseOrders: {
    list: (params: ListParams = {}) => list<PurchaseOrder>("/api/v1/purchase-orders", { query: params }),
    get: (id: string) => data<PurchaseOrder>(`/api/v1/purchase-orders/${id}`),
    create: (body: unknown) => data<PurchaseOrder>("/api/v1/purchase-orders", { method: "POST", body }),
    setStatus: (id: string, status: string, approvalId?: string) =>
      data<PurchaseOrder>(`/api/v1/purchase-orders/${id}/status`, {
        method: "PATCH",
        body: { status, approval_id: approvalId },
      }),
    close: (id: string) => data<PurchaseOrder>(`/api/v1/purchase-orders/${id}/close`, { method: "POST" }),
  },

  // --- approvals ---
  approvals: {
    list: (params: ListParams = {}) => list<Approval>("/api/v1/approvals", { query: params }),
    get: (id: string) => data<Approval>(`/api/v1/approvals/${id}`),
    request: (body: unknown) => data<Approval>("/api/v1/approvals", { method: "POST", body }),
    decide: (id: string, body: { approve: boolean; comment: string }) =>
      data<Approval>(`/api/v1/approvals/${id}/decide`, { method: "POST", body }),
  },

  // --- audit ---
  auditLogs: {
    list: (params: ListParams = {}) => list<AuditEntry>("/api/v1/audit-logs", { query: params }),
    get: (id: string) => data<AuditEntry>(`/api/v1/audit-logs/${id}`),
  },

  // --- reports ---
  reports: {
    poByVendor: () => data<VendorSpend[]>("/api/v1/reports/po-by-vendor"),
    poByVendorCsvUrl: () => buildUrl("/api/v1/reports/po-by-vendor", { format: "csv" }),
    tenderSummary: () => data<TenderSummary>("/api/v1/reports/tender-summary"),
    approvalSummary: () => data<ApprovalSummary>("/api/v1/reports/approval-summary"),
  },
};

/** formatMoney renders an integer minor-unit amount with its currency. */
export function formatMoney(amount: number, currency = "IDR"): string {
  return new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency,
    maximumFractionDigits: 0,
  }).format(amount);
}

/** formatDate renders an ISO timestamp for the UI. */
export function formatDate(value?: string): string {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return date.toLocaleString("id-ID", { dateStyle: "medium", timeStyle: "short" });
}

export type { Page, PageMeta };
