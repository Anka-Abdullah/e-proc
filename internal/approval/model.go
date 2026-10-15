// Package approval runs the approval workflow: requests, decisions and the
// immutable decision history attached to a business document.
package approval

import (
	"strings"
	"time"

	"github.com/ank/e-proc/pkg/validator"
)

// Status is the state of one approval request.
type Status string

// Approval states.
const (
	StatusPending   Status = "pending"
	StatusApproved  Status = "approved"
	StatusRejected  Status = "rejected"
	StatusCancelled Status = "cancelled"
)

// EntityType names the kind of document an approval governs.
type EntityType string

// Supported entity types.
const (
	EntityPurchaseOrder EntityType = "purchase_order"
	EntityTender        EntityType = "tender"
)

// Money is an integer amount in the smallest currency unit.
type Money struct {
	Amount   int64  `bson:"amount" json:"amount"`
	Currency string `bson:"currency" json:"currency"`
}

// Step records one actor's decision.
type Step struct {
	ActorID   string    `bson:"actor_id" json:"actor_id"`
	Decision  Status    `bson:"decision" json:"decision"`
	Comment   string    `bson:"comment" json:"comment"`
	DecidedAt time.Time `bson:"decided_at" json:"decided_at"`
}

// Approval is the workflow instance attached to a business document.
type Approval struct {
	ID          string     `bson:"_id" json:"id"`
	TenantID    string     `bson:"tenant_id" json:"tenant_id"`
	EntityType  EntityType `bson:"entity_type" json:"entity_type"`
	EntityID    string     `bson:"entity_id" json:"entity_id"`
	Amount      Money      `bson:"amount" json:"amount"`
	Status      Status     `bson:"status" json:"status"`
	RequestedBy string     `bson:"requested_by" json:"requested_by"`
	AssignedTo  string     `bson:"assigned_to" json:"assigned_to"`
	Comment     string     `bson:"comment" json:"comment"`
	History     []Step     `bson:"history" json:"history"`
	CreatedAt   time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time  `bson:"updated_at" json:"updated_at"`
	DecidedAt   *time.Time `bson:"decided_at,omitempty" json:"decided_at,omitempty"`
}

// RequestInput is the payload for POST /approvals.
type RequestInput struct {
	TenantID   string `json:"tenant_id"`
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	Amount     int64  `json:"amount"`
	Currency   string `json:"currency"`
	AssignedTo string `json:"assigned_to"`
	Comment    string `json:"comment"`
}

// DecideInput is the payload for POST /approvals/{id}/decide.
type DecideInput struct {
	Approve bool   `json:"approve"`
	Comment string `json:"comment"`
}

// ListFilter narrows approval list queries.
type ListFilter struct {
	TenantID   string
	Status     Status
	AssignedTo string
	EntityID   string
	Page       int
	PerPage    int
}

// Validate checks a request payload.
func (in RequestInput) Validate() validator.Errors {
	errs := validator.Errors{}
	errs.Required("tenant_id", in.TenantID)
	errs.Required("entity_type", in.EntityType)
	errs.OneOf("entity_type", in.EntityType, string(EntityPurchaseOrder), string(EntityTender))
	errs.Required("entity_id", in.EntityID)
	errs.Positive("amount", float64(in.Amount))
	errs.MaxLen("comment", in.Comment, 2000)
	return errs
}

// Validate checks a decision payload. A rejection must say why.
func (in DecideInput) Validate() validator.Errors {
	errs := validator.Errors{}
	if !in.Approve {
		errs.MinLen("comment", in.Comment, 3)
	}
	errs.MaxLen("comment", in.Comment, 2000)
	return errs
}

// Normalize trims whitespace and defaults the currency.
func (in *RequestInput) Normalize() {
	in.TenantID = strings.TrimSpace(in.TenantID)
	in.EntityType = strings.ToLower(strings.TrimSpace(in.EntityType))
	in.EntityID = strings.TrimSpace(in.EntityID)
	in.AssignedTo = strings.TrimSpace(in.AssignedTo)
	in.Comment = strings.TrimSpace(in.Comment)
	if in.Currency == "" {
		in.Currency = "IDR"
	}
}

// Normalize trims the decision comment.
func (in *DecideInput) Normalize() { in.Comment = strings.TrimSpace(in.Comment) }

// IsValidStatus reports whether s is a known approval status.
func IsValidStatus(s Status) bool {
	switch s {
	case StatusPending, StatusApproved, StatusRejected, StatusCancelled:
		return true
	default:
		return false
	}
}

// StatusFromString parses a status from a query parameter.
func StatusFromString(s string) Status { return Status(strings.ToLower(strings.TrimSpace(s))) }

// IsPending reports whether the approval is still awaiting a decision.
func (a *Approval) IsPending() bool { return a != nil && a.Status == StatusPending }

// CanBeDecidedBy reports whether actor may decide this approval: the assignee,
// or anyone when the request was left unassigned.
func (a *Approval) CanBeDecidedBy(actorID string, isAdmin bool) bool {
	if a == nil {
		return false
	}
	if isAdmin {
		return true
	}
	return a.AssignedTo == "" || a.AssignedTo == actorID
}
