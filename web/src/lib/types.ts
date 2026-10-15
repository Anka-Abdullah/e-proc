// Domain types. They mirror the JSON the Go services return, so a change on the
// server shows up here as a type error rather than a runtime surprise.

export type Role = "admin" | "procurement_officer" | "approver" | "vendor";

export interface Money {
  amount: number;
  currency: string;
}

export interface PageMeta {
  page: number;
  per_page: number;
  total: number;
}

export interface ApiErrorBody {
  code: string;
  message: string;
  details?: Record<string, string>;
}

export interface Envelope<T> {
  data?: T;
  meta?: PageMeta;
  error?: ApiErrorBody;
}

export interface Page<T> {
  items: T[];
  meta: PageMeta;
}

export interface User {
  id: string;
  tenant_id: string;
  email: string;
  name: string;
  role: Role;
  status: string;
  last_login_at?: string;
  created_at: string;
}

export interface LoginResult {
  token: string;
  expires_at: string;
  user: User;
}

export type VendorStatus = "pending" | "active" | "suspended";

export interface Vendor {
  id: string;
  tenant_id: string;
  code: string;
  name: string;
  email: string;
  phone: string;
  address: string;
  status: VendorStatus;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface TenderItem {
  name: string;
  quantity: number;
  unit: string;
  specification: string;
}

export type TenderStatus = "draft" | "published" | "closed" | "cancelled" | "awarded";

export interface Tender {
  id: string;
  tenant_id: string;
  code: string;
  title: string;
  description: string;
  items: TenderItem[];
  budget: Money;
  status: TenderStatus;
  open_at: string;
  close_at: string;
  invited_vendor_ids: string[];
  created_by: string;
  created_at: string;
  updated_at: string;
  published_at?: string;
  closed_at?: string;
}

export interface BidItem {
  name: string;
  quantity: number;
  unit_price: Money;
  total_price: Money;
}

export type BidStatus = "submitted" | "shortlisted" | "selected" | "rejected";

export interface Bid {
  id: string;
  tenant_id: string;
  tender_id: string;
  vendor_id: string;
  items: BidItem[];
  total: Money;
  delivery_days: number;
  notes: string;
  status: BidStatus;
  submitted_by: string;
  submitted_at: string;
  updated_at: string;
}

export interface BidComparison {
  bid_id: string;
  vendor_id: string;
  total: Money;
  delivery_days: number;
  status: BidStatus;
  lowest_price: boolean;
  fastest_delivery: boolean;
  rank: number;
  price_delta_pct: number;
}

export interface PurchaseOrderLine {
  name: string;
  quantity: number;
  unit_price: Money;
  total_price: Money;
}

export type POStatus =
  | "draft"
  | "pending_approval"
  | "approved"
  | "rejected"
  | "issued"
  | "closed";

export interface PurchaseOrder {
  id: string;
  tenant_id: string;
  number: string;
  tender_id: string;
  bid_id: string;
  vendor_id: string;
  lines: PurchaseOrderLine[];
  total: Money;
  status: POStatus;
  approval_id?: string;
  notes: string;
  created_by: string;
  created_at: string;
  updated_at: string;
  issued_at?: string;
  closed_at?: string;
}

export type ApprovalStatus = "pending" | "approved" | "rejected" | "cancelled";

export interface ApprovalStep {
  actor_id: string;
  decision: ApprovalStatus;
  comment: string;
  decided_at: string;
}

export interface Approval {
  id: string;
  tenant_id: string;
  entity_type: string;
  entity_id: string;
  amount: Money;
  status: ApprovalStatus;
  requested_by: string;
  assigned_to: string;
  comment: string;
  history: ApprovalStep[];
  created_at: string;
  updated_at: string;
  decided_at?: string;
}

export interface AuditEntry {
  id: string;
  tenant_id: string;
  actor_id: string;
  actor_role?: string;
  action: string;
  entity_type: string;
  entity_id: string;
  summary?: string;
  payload?: Record<string, unknown>;
  event_id: string;
  created_at: string;
}

export interface VendorSpend {
  vendor_id: string;
  order_count: number;
  total: number;
  currency: string;
}

export interface TenderSummary {
  total: number;
  by_status: Record<string, number>;
  total_budget: number;
  total_bids: number;
  currency: string;
  open_tenders: number;
  closed_tenders: number;
}

export interface ApprovalSummary {
  total: number;
  by_status: Record<string, number>;
  pending_value: number;
  approved_value: number;
  rejected_value: number;
  currency: string;
}
