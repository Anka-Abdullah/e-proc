package reporting

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/ank/e-proc/pkg/broker"
)

// routeByEvent lists every event the reporting projector consumes.
var routeByEvent = []string{
	broker.EventPOCreated,
	broker.EventPOApproved,
	broker.EventPOIssued,
	broker.EventPORejected,
	broker.EventPOClosed,
	broker.EventTenderPublished,
	broker.EventTenderClosed,
	broker.EventBidSubmitted,
	broker.EventApprovalRequested,
	broker.EventApprovalDecided,
}

// RoutingKeys lists the events the projector subscribes to.
func RoutingKeys() []string { return routeByEvent }

// Projector turns domain events into reporting rows. Every handler is an upsert
// keyed by entity id, so replaying an event is harmless.
type Projector struct {
	repo *Repository
	log  *slog.Logger
}

// NewProjector builds the projector.
func NewProjector(repo *Repository, log *slog.Logger) *Projector {
	return &Projector{repo: repo, log: log}
}

// EnsureIndexes prepares the projection collections.
func (p *Projector) EnsureIndexes(ctx context.Context) error { return p.repo.EnsureIndexes(ctx) }

// Handler adapts Apply to the broker consumer signature.
func (p *Projector) Handler() broker.Handler {
	return func(ctx context.Context, evt broker.Event) error { return p.Apply(ctx, evt) }
}

// Apply projects one event into the read models.
func (p *Projector) Apply(ctx context.Context, evt broker.Event) error {
	payload := decode(evt.Payload)
	at := evt.OccurredAt.UTC()
	if at.IsZero() {
		at = time.Now().UTC()
	}

	switch evt.Name {
	case broker.EventPOCreated:
		row := PORow{
			ID:        str(payload, "po_id"),
			TenantID:  evt.TenantID,
			Number:    str(payload, "number"),
			VendorID:  str(payload, "vendor_id"),
			Total:     num(payload, "total"),
			Currency:  money(payload, "total"),
			Status:    str(payload, "status"),
			CreatedAt: at,
			UpdatedAt: at,
		}
		if row.ID == "" {
			return fmt.Errorf("po.created without po_id")
		}
		return p.repo.UpsertPO(ctx, row)

	case broker.EventPOApproved:
		return p.repo.UpdatePOStatus(ctx, str(payload, "po_id"), "approved", at)
	case broker.EventPOIssued:
		return p.repo.UpdatePOStatus(ctx, str(payload, "po_id"), "issued", at)
	case broker.EventPORejected:
		return p.repo.UpdatePOStatus(ctx, str(payload, "po_id"), "rejected", at)
	case broker.EventPOClosed:
		return p.repo.UpdatePOStatus(ctx, str(payload, "po_id"), "closed", at)

	case broker.EventTenderPublished:
		row := TenderRow{
			ID:        str(payload, "tender_id"),
			TenantID:  evt.TenantID,
			Code:      str(payload, "code"),
			Title:     str(payload, "title"),
			Status:    "published",
			Budget:    num(payload, "budget"),
			Currency:  money(payload, "budget"),
			CreatedAt: at,
			UpdatedAt: at,
		}
		if row.ID == "" {
			return fmt.Errorf("tender.published without tender_id")
		}
		return p.repo.UpsertTender(ctx, row)

	case broker.EventTenderClosed:
		return p.repo.UpdateTenderStatus(ctx, str(payload, "tender_id"), "closed", at, true)

	case broker.EventBidSubmitted:
		return p.repo.IncrementTenderBids(ctx, str(payload, "tender_id"))

	case broker.EventApprovalRequested:
		row := ApprovalRow{
			ID:         str(payload, "approval_id"),
			TenantID:   evt.TenantID,
			EntityType: str(payload, "entity_type"),
			EntityID:   str(payload, "entity_id"),
			Amount:     num(payload, "amount"),
			Currency:   money(payload, "amount"),
			Status:     "pending",
			AssignedTo: str(payload, "assigned_to"),
			CreatedAt:  at,
		}
		if row.ID == "" {
			return fmt.Errorf("approval.requested without approval_id")
		}
		return p.repo.UpsertApproval(ctx, row)

	case broker.EventApprovalDecided:
		return p.repo.UpdateApprovalStatus(ctx, str(payload, "approval_id"), str(payload, "decision"), at)

	default:
		p.log.DebugContext(ctx, "reporting projector ignored event", slog.String("event", evt.Name))
		return nil
	}
}

func decode(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func str(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	if v, ok := payload[key].(string); ok {
		return v
	}
	return ""
}

func num(payload map[string]any, key string) int64 {
	if payload == nil {
		return 0
	}
	value, ok := payload[key]
	if !ok {
		return 0
	}
	switch v := value.(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case map[string]any:
		if amount, ok := v["amount"].(float64); ok {
			return int64(amount)
		}
	}
	return 0
}

func money(payload map[string]any, key string) string {
	if payload == nil {
		return "IDR"
	}
	if nested, ok := payload[key].(map[string]any); ok {
		if currency, ok := nested["currency"].(string); ok && currency != "" {
			return currency
		}
	}
	return "IDR"
}
