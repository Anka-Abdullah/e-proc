// Package bid owns vendor quotations: submission, price comparison and
// selection.
package bid

import (
	"strings"
	"time"

	"github.com/ank/e-proc/pkg/auth"
	"github.com/ank/e-proc/pkg/validator"
)

// Status is where a quotation stands in the selection process.
type Status string

// Bid states.
const (
	StatusSubmitted   Status = "submitted"
	StatusShortlisted Status = "shortlisted"
	StatusSelected    Status = "selected"
	StatusRejected    Status = "rejected"
)

// Money is an integer amount in the smallest currency unit.
type Money struct {
	Amount   int64  `bson:"amount" json:"amount"`
	Currency string `bson:"currency" json:"currency"`
}

// Item is the price offered for one tender line. TotalPrice is always computed
// by the service, never taken from the client.
type Item struct {
	Name       string  `bson:"name" json:"name"`
	Quantity   float64 `bson:"quantity" json:"quantity"`
	UnitPrice  Money   `bson:"unit_price" json:"unit_price"`
	TotalPrice Money   `bson:"total_price" json:"total_price"`
}

// Bid is a vendor's quotation for a tender.
type Bid struct {
	ID           string    `bson:"_id" json:"id"`
	TenantID     string    `bson:"tenant_id" json:"tenant_id"`
	TenderID     string    `bson:"tender_id" json:"tender_id"`
	VendorID     string    `bson:"vendor_id" json:"vendor_id"`
	Items        []Item    `bson:"items" json:"items"`
	Total        Money     `bson:"total" json:"total"`
	DeliveryDays int       `bson:"delivery_days" json:"delivery_days"`
	Notes        string    `bson:"notes" json:"notes"`
	Status       Status    `bson:"status" json:"status"`
	SubmittedBy  string    `bson:"submitted_by" json:"submitted_by"`
	SubmittedAt  time.Time `bson:"submitted_at" json:"submitted_at"`
	UpdatedAt    time.Time `bson:"updated_at" json:"updated_at"`
}

// SubmitInput is the payload for POST /bids.
type SubmitInput struct {
	TenantID     string `json:"tenant_id"`
	TenderID     string `json:"tender_id"`
	VendorID     string `json:"vendor_id"`
	Items        []Item `json:"items"`
	DeliveryDays int    `json:"delivery_days"`
	Notes        string `json:"notes"`
	Currency     string `json:"currency"`
}

// ListFilter narrows bid list queries.
type ListFilter struct {
	TenantID string
	TenderID string
	VendorID string
	Status   Status
	Page     int
	PerPage  int
}

// Validate checks a submission payload.
func (in SubmitInput) Validate() validator.Errors {
	errs := validator.Errors{}
	errs.Required("tenant_id", in.TenantID)
	errs.Required("tender_id", in.TenderID)
	errs.Required("vendor_id", in.VendorID)
	if len(in.Items) == 0 {
		errs.Add("items", "items must contain at least one line")
	}
	for i, item := range in.Items {
		if strings.TrimSpace(item.Name) == "" {
			errs.Add("items", "item "+idx(i)+" must have a name")
			break
		}
		if item.Quantity <= 0 {
			errs.Add("items", "item "+idx(i)+" must have a positive quantity")
			break
		}
		if item.UnitPrice.Amount <= 0 {
			errs.Add("items", "item "+idx(i)+" must have a positive unit price")
			break
		}
	}
	errs.NonNegative("delivery_days", float64(in.DeliveryDays))
	errs.MaxLen("notes", in.Notes, 2000)
	return errs
}

// Normalize trims whitespace and defaults the currency.
func (in *SubmitInput) Normalize() {
	in.TenantID = strings.TrimSpace(in.TenantID)
	in.TenderID = strings.TrimSpace(in.TenderID)
	in.VendorID = strings.TrimSpace(in.VendorID)
	in.Notes = strings.TrimSpace(in.Notes)
	if in.Currency == "" {
		in.Currency = "IDR"
	}
	for i := range in.Items {
		in.Items[i].Name = strings.TrimSpace(in.Items[i].Name)
		if in.Items[i].UnitPrice.Currency == "" {
			in.Items[i].UnitPrice.Currency = in.Currency
		}
	}
}

// PriceItems recomputes per-line totals and the grand total from quantities and
// unit prices, so a client cannot submit an inconsistent total.
func (in *SubmitInput) PriceItems() ([]Item, Money) {
	items := make([]Item, 0, len(in.Items))
	total := Money{Currency: in.Currency}

	for _, item := range in.Items {
		lineTotal := int64(item.Quantity * float64(item.UnitPrice.Amount))
		currency := item.UnitPrice.Currency
		if currency == "" {
			currency = in.Currency
		}
		items = append(items, Item{
			Name:       item.Name,
			Quantity:   item.Quantity,
			UnitPrice:  Money{Amount: item.UnitPrice.Amount, Currency: currency},
			TotalPrice: Money{Amount: lineTotal, Currency: currency},
		})
		total.Amount += lineTotal
	}
	return items, total
}

// IsValidStatus reports whether s is a known bid status.
func IsValidStatus(s Status) bool {
	switch s {
	case StatusSubmitted, StatusShortlisted, StatusSelected, StatusRejected:
		return true
	default:
		return false
	}
}

// StatusFromString parses a status from a query parameter.
func StatusFromString(s string) Status { return Status(strings.ToLower(strings.TrimSpace(s))) }

// Comparison is one row of the price comparison report.
type Comparison struct {
	BidID           string  `json:"bid_id"`
	VendorID        string  `json:"vendor_id"`
	Total           Money   `json:"total"`
	DeliveryDays    int     `json:"delivery_days"`
	Status          Status  `json:"status"`
	LowestPrice     bool    `json:"lowest_price"`
	FastestDelivery bool    `json:"fastest_delivery"`
	Rank            int     `json:"rank"`
	PriceDeltaPct   float64 `json:"price_delta_pct"`
}

// CanVendorSubmit reports whether the vendor role may submit a bid.
func CanVendorSubmit(role auth.Role) bool {
	return role == auth.RoleVendor || role == auth.RoleProcurement || role == auth.RoleAdmin
}

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
