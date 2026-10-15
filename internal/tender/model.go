// Package tender owns tenders / requests for quotation: model, repository,
// publication lifecycle and both transports.
package tender

import (
	"strings"
	"time"

	"github.com/ank/e-proc/pkg/validator"
)

// Status is the lifecycle state of a tender.
type Status string

// Tender states.
const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusClosed    Status = "closed"
	StatusCancelled Status = "cancelled"
	StatusAwarded   Status = "awarded"
)

// Money is an integer amount in the smallest currency unit, so comparisons in
// approval thresholds never suffer float rounding.
type Money struct {
	Amount   int64  `bson:"amount" json:"amount"`
	Currency string `bson:"currency" json:"currency"`
}

// Item is one line of what is being procured.
type Item struct {
	Name          string  `bson:"name" json:"name"`
	Quantity      float64 `bson:"quantity" json:"quantity"`
	Unit          string  `bson:"unit" json:"unit"`
	Specification string  `bson:"specification" json:"specification"`
}

// Tender is a request for quotation published to vendors.
type Tender struct {
	ID               string     `bson:"_id" json:"id"`
	TenantID         string     `bson:"tenant_id" json:"tenant_id"`
	Code             string     `bson:"code" json:"code"`
	Title            string     `bson:"title" json:"title"`
	Description      string     `bson:"description" json:"description"`
	Items            []Item     `bson:"items" json:"items"`
	Budget           Money      `bson:"budget" json:"budget"`
	Status           Status     `bson:"status" json:"status"`
	OpenAt           time.Time  `bson:"open_at" json:"open_at"`
	CloseAt          time.Time  `bson:"close_at" json:"close_at"`
	InvitedVendorIDs []string   `bson:"invited_vendor_ids" json:"invited_vendor_ids"`
	CreatedBy        string     `bson:"created_by" json:"created_by"`
	CreatedAt        time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt        time.Time  `bson:"updated_at" json:"updated_at"`
	PublishedAt      *time.Time `bson:"published_at,omitempty" json:"published_at,omitempty"`
	ClosedAt         *time.Time `bson:"closed_at,omitempty" json:"closed_at,omitempty"`
}

// CreateInput is the payload for POST /tenders.
type CreateInput struct {
	TenantID         string    `json:"tenant_id"`
	Code             string    `json:"code"`
	Title            string    `json:"title"`
	Description      string    `json:"description"`
	Items            []Item    `json:"items"`
	Budget           Money     `json:"budget"`
	CloseAt          time.Time `json:"close_at"`
	InvitedVendorIDs []string  `json:"invited_vendor_ids"`
}

// ListFilter narrows tender list queries.
type ListFilter struct {
	TenantID string
	Search   string
	Status   Status
	Page     int
	PerPage  int
}

// Validate checks a create payload.
func (in CreateInput) Validate() validator.Errors {
	errs := validator.Errors{}
	errs.Required("tenant_id", in.TenantID)
	errs.Required("code", in.Code)
	errs.MaxLen("code", in.Code, 40)
	errs.Required("title", in.Title)
	errs.MaxLen("title", in.Title, 240)
	errs.MaxLen("description", in.Description, 4000)
	if len(in.Items) == 0 {
		errs.Add("items", "items must contain at least one line")
	}
	for i, item := range in.Items {
		if strings.TrimSpace(item.Name) == "" {
			errs.Add("items", "item "+itoa(i)+" must have a name")
			break
		}
		if item.Quantity <= 0 {
			errs.Add("items", "item "+itoa(i)+" must have a positive quantity")
			break
		}
	}
	errs.NonNegative("budget.amount", float64(in.Budget.Amount))
	if in.CloseAt.IsZero() {
		errs.Add("close_at", "close_at is required")
	}
	return errs
}

// Normalize trims whitespace and upper-cases the code.
func (in *CreateInput) Normalize() {
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	in.TenantID = strings.TrimSpace(in.TenantID)
	if in.Budget.Currency == "" {
		in.Budget.Currency = "IDR"
	}
	for i := range in.Items {
		in.Items[i].Name = strings.TrimSpace(in.Items[i].Name)
		in.Items[i].Unit = strings.TrimSpace(in.Items[i].Unit)
		in.Items[i].Specification = strings.TrimSpace(in.Items[i].Specification)
	}
}

// IsValidStatus reports whether s is a known tender status.
func IsValidStatus(s Status) bool {
	switch s {
	case StatusDraft, StatusPublished, StatusClosed, StatusCancelled, StatusAwarded:
		return true
	default:
		return false
	}
}

// StatusFromString parses a status from a query parameter.
func StatusFromString(s string) Status { return Status(strings.ToLower(strings.TrimSpace(s))) }

// CanPublish reports whether the tender is in a state that may be published.
func (t *Tender) CanPublish() bool { return t.Status == StatusDraft }

// CanClose reports whether the tender may be closed.
func (t *Tender) CanClose() bool { return t.Status == StatusPublished }

// IsOpenAt reports whether the close deadline has passed.
func (t *Tender) IsOpenAt(now time.Time) bool { return !t.CloseAt.IsZero() && !t.CloseAt.After(now) }

func itoa(i int) string {
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
