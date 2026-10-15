// Package po owns purchase orders: creation from a selected bid, the status
// machine, and the approval threshold rule.
package po

import (
	"strings"
	"time"

	"github.com/ank/e-proc/pkg/validator"
)

// Status is the purchase order lifecycle, including the approval leg.
type Status string

// Purchase order states.
const (
	StatusDraft           Status = "draft"
	StatusPendingApproval Status = "pending_approval"
	StatusApproved        Status = "approved"
	StatusRejected        Status = "rejected"
	StatusIssued          Status = "issued"
	StatusClosed          Status = "closed"
)

// Money is an integer amount in the smallest currency unit.
type Money struct {
	Amount   int64  `bson:"amount" json:"amount"`
	Currency string `bson:"currency" json:"currency"`
}

// Line is one ordered item with its agreed price.
type Line struct {
	Name       string  `bson:"name" json:"name"`
	Quantity   float64 `bson:"quantity" json:"quantity"`
	UnitPrice  Money   `bson:"unit_price" json:"unit_price"`
	TotalPrice Money   `bson:"total_price" json:"total_price"`
}

// PurchaseOrder is the committed order to a vendor.
type PurchaseOrder struct {
	ID         string     `bson:"_id" json:"id"`
	TenantID   string     `bson:"tenant_id" json:"tenant_id"`
	Number     string     `bson:"number" json:"number"`
	TenderID   string     `bson:"tender_id" json:"tender_id"`
	BidID      string     `bson:"bid_id" json:"bid_id"`
	VendorID   string     `bson:"vendor_id" json:"vendor_id"`
	Lines      []Line     `bson:"lines" json:"lines"`
	Total      Money      `bson:"total" json:"total"`
	Status     Status     `bson:"status" json:"status"`
	ApprovalID string     `bson:"approval_id,omitempty" json:"approval_id,omitempty"`
	Notes      string     `bson:"notes" json:"notes"`
	CreatedBy  string     `bson:"created_by" json:"created_by"`
	CreatedAt  time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt  time.Time  `bson:"updated_at" json:"updated_at"`
	IssuedAt   *time.Time `bson:"issued_at,omitempty" json:"issued_at,omitempty"`
	ClosedAt   *time.Time `bson:"closed_at,omitempty" json:"closed_at,omitempty"`
}

// CreateInput is the payload for POST /purchase-orders.
type CreateInput struct {
	TenantID string `json:"tenant_id"`
	TenderID string `json:"tender_id"`
	BidID    string `json:"bid_id"`
	VendorID string `json:"vendor_id"`
	Lines    []Line `json:"lines"`
	Notes    string `json:"notes"`
	Currency string `json:"currency"`
}

// ListFilter narrows purchase order list queries.
type ListFilter struct {
	TenantID string
	VendorID string
	Status   Status
	Page     int
	PerPage  int
}

// Validate checks a create payload.
func (in CreateInput) Validate() validator.Errors {
	errs := validator.Errors{}
	errs.Required("tenant_id", in.TenantID)
	errs.Required("vendor_id", in.VendorID)
	if len(in.Lines) == 0 {
		errs.Add("lines", "lines must contain at least one line")
	}
	for i, line := range in.Lines {
		if strings.TrimSpace(line.Name) == "" {
			errs.Add("lines", "line "+idx(i)+" must have a name")
			break
		}
		if line.Quantity <= 0 {
			errs.Add("lines", "line "+idx(i)+" must have a positive quantity")
			break
		}
		if line.UnitPrice.Amount <= 0 {
			errs.Add("lines", "line "+idx(i)+" must have a positive unit price")
			break
		}
	}
	errs.Required("tender_id", in.TenderID)
	errs.Required("bid_id", in.BidID)
	errs.MaxLen("notes", in.Notes, 2000)
	return errs
}

// Normalize trims whitespace and defaults the currency.
func (in *CreateInput) Normalize() {
	in.TenantID = strings.TrimSpace(in.TenantID)
	in.TenderID = strings.TrimSpace(in.TenderID)
	in.BidID = strings.TrimSpace(in.BidID)
	in.VendorID = strings.TrimSpace(in.VendorID)
	in.Notes = strings.TrimSpace(in.Notes)
	if in.Currency == "" {
		in.Currency = "IDR"
	}
	for i := range in.Lines {
		in.Lines[i].Name = strings.TrimSpace(in.Lines[i].Name)
		if in.Lines[i].UnitPrice.Currency == "" {
			in.Lines[i].UnitPrice.Currency = in.Currency
		}
	}
}

// PriceLines recomputes line totals and the grand total from quantities and
// unit prices, so the client cannot dictate the total.
func (in *CreateInput) PriceLines() ([]Line, Money) {
	lines := make([]Line, 0, len(in.Lines))
	total := Money{Currency: in.Currency}

	for _, line := range in.Lines {
		lineTotal := int64(line.Quantity * float64(line.UnitPrice.Amount))
		currency := line.UnitPrice.Currency
		if currency == "" {
			currency = in.Currency
		}
		lines = append(lines, Line{
			Name:       line.Name,
			Quantity:   line.Quantity,
			UnitPrice:  Money{Amount: line.UnitPrice.Amount, Currency: currency},
			TotalPrice: Money{Amount: lineTotal, Currency: currency},
		})
		total.Amount += lineTotal
	}
	return lines, total
}

// RequiresApproval reports whether an amount is at or above the threshold.
func RequiresApproval(total Amount, threshold float64) bool {
	return float64(total) >= threshold
}

// Amount is the raw integer amount, named to keep the threshold comparison
// readable at the call site.
type Amount = int64

// IsValidStatus reports whether s is a known purchase order status.
func IsValidStatus(s Status) bool {
	switch s {
	case StatusDraft, StatusPendingApproval, StatusApproved, StatusRejected, StatusIssued, StatusClosed:
		return true
	default:
		return false
	}
}

// StatusFromString parses a status from a query parameter.
func StatusFromString(s string) Status { return Status(strings.ToLower(strings.TrimSpace(s))) }

// transitions is the purchase order state machine. Absent pairs are rejected,
// so an illegal status change cannot slip through a generic update endpoint.
var transitions = map[Status][]Status{
	StatusDraft:           {StatusPendingApproval, StatusIssued, StatusRejected},
	StatusPendingApproval: {StatusApproved, StatusRejected},
	StatusApproved:        {StatusIssued, StatusClosed},
	StatusIssued:          {StatusClosed},
	StatusRejected:        {},
	StatusClosed:          {},
}

// CanTransition reports whether from -> to is a legal move.
func CanTransition(from, to Status) bool {
	if from == to {
		return true
	}
	for _, allowed := range transitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// NextStatuses lists the legal targets from a status (used in error messages).
func NextStatuses(from Status) []Status { return transitions[from] }

func idx(i int) string {
	if i == 0 {
		return "0"
	}
	digits := make([]byte, 0, 4)
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}
