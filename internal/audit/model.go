// Package audit records every important action as an append-only log. Entries
// arrive from domain events, never from client input.
package audit

import (
	"strings"
	"time"

	"github.com/ank/e-proc/pkg/validator"
)

// Entry is one immutable audit record.
type Entry struct {
	ID         string         `bson:"_id" json:"id"`
	TenantID   string         `bson:"tenant_id" json:"tenant_id"`
	ActorID    string         `bson:"actor_id" json:"actor_id"`
	ActorRole  string         `bson:"actor_role,omitempty" json:"actor_role,omitempty"`
	Action     string         `bson:"action" json:"action"`
	EntityType string         `bson:"entity_type" json:"entity_type"`
	EntityID   string         `bson:"entity_id" json:"entity_id"`
	Summary    string         `bson:"summary,omitempty" json:"summary,omitempty"`
	Payload    map[string]any `bson:"payload,omitempty" json:"payload,omitempty"`
	EventID    string         `bson:"event_id" json:"event_id"`
	RequestID  string         `bson:"request_id,omitempty" json:"request_id,omitempty"`
	IP         string         `bson:"ip,omitempty" json:"ip,omitempty"`
	CreatedAt  time.Time      `bson:"created_at" json:"created_at"`
}

// Filter narrows an audit query.
type Filter struct {
	TenantID   string
	ActorID    string
	EntityType string
	EntityID   string
	Action     string
	From       *time.Time
	To         *time.Time
	Page       int
	PerPage    int
}

// Validate checks a query filter; an unbounded query over an append-only log is
// refused so the API cannot accidentally dump everything.
func (f Filter) Validate() validator.Errors {
	errs := validator.Errors{}
	errs.Required("tenant_id", f.TenantID)
	return errs
}

// Normalize trims filter fields.
func (f *Filter) Normalize() {
	f.TenantID = strings.TrimSpace(f.TenantID)
	f.ActorID = strings.TrimSpace(f.ActorID)
	f.EntityType = strings.ToLower(strings.TrimSpace(f.EntityType))
	f.EntityID = strings.TrimSpace(f.EntityID)
	f.Action = strings.TrimSpace(f.Action)
}
