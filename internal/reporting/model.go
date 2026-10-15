// Package reporting maintains read-model projections built from domain events
// and serves the platform's reports. It never writes to another service's data;
// it only projects events it receives.
package reporting

import "time"

// PORow is the purchase-order projection row.
type PORow struct {
	ID        string    `bson:"_id" json:"id"`
	TenantID  string    `bson:"tenant_id" json:"tenant_id"`
	Number    string    `bson:"number" json:"number"`
	VendorID  string    `bson:"vendor_id" json:"vendor_id"`
	Total     int64     `bson:"total" json:"total"`
	Currency  string    `bson:"currency" json:"currency"`
	Status    string    `bson:"status" json:"status"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
}

// TenderRow is the tender projection row.
type TenderRow struct {
	ID        string     `bson:"_id" json:"id"`
	TenantID  string     `bson:"tenant_id" json:"tenant_id"`
	Code      string     `bson:"code" json:"code"`
	Title     string     `bson:"title" json:"title"`
	Status    string     `bson:"status" json:"status"`
	Budget    int64      `bson:"budget" json:"budget"`
	Currency  string     `bson:"currency" json:"currency"`
	Bids      int        `bson:"bids" json:"bids"`
	CreatedAt time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time  `bson:"updated_at" json:"updated_at"`
	ClosedAt  *time.Time `bson:"closed_at,omitempty" json:"closed_at,omitempty"`
}

// ApprovalRow is the approval projection row.
type ApprovalRow struct {
	ID         string     `bson:"_id" json:"id"`
	TenantID   string     `bson:"tenant_id" json:"tenant_id"`
	EntityType string     `bson:"entity_type" json:"entity_type"`
	EntityID   string     `bson:"entity_id" json:"entity_id"`
	Amount     int64      `bson:"amount" json:"amount"`
	Currency   string     `bson:"currency" json:"currency"`
	Status     string     `bson:"status" json:"status"`
	AssignedTo string     `bson:"assigned_to" json:"assigned_to"`
	CreatedAt  time.Time  `bson:"created_at" json:"created_at"`
	DecidedAt  *time.Time `bson:"decided_at,omitempty" json:"decided_at,omitempty"`
}

// VendorSpend is one row of the purchase-order-per-vendor report.
type VendorSpend struct {
	VendorID   string `json:"vendor_id"`
	OrderCount int64  `json:"order_count"`
	Total      int64  `json:"total"`
	Currency   string `json:"currency"`
}

// TenderSummary is the tender report body.
type TenderSummary struct {
	Total         int64            `json:"total"`
	ByStatus      map[string]int64 `json:"by_status"`
	TotalBudget   int64            `json:"total_budget"`
	TotalBids     int              `json:"total_bids"`
	Currency      string           `json:"currency"`
	OpenTenders   int64            `json:"open_tenders"`
	ClosedTenders int64            `json:"closed_tenders"`
}

// ApprovalSummary is the approval report body.
type ApprovalSummary struct {
	Total         int64            `json:"total"`
	ByStatus      map[string]int64 `json:"by_status"`
	PendingValue  int64            `json:"pending_value"`
	ApprovedValue int64            `json:"approved_value"`
	RejectedValue int64            `json:"rejected_value"`
	Currency      string           `json:"currency"`
}
