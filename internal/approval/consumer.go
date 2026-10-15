package approval

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/ank/e-proc/pkg/broker"
	"github.com/ank/e-proc/pkg/cache"
)

// poCreatedRoutingKey is the event that may open an approval automatically.
const poCreatedRoutingKey = broker.EventPOCreated

// eventIdempotencyTTL bounds how long a consumed event id is remembered.
const eventIdempotencyTTL = 24 * time.Hour

// RoutingKeys lists the events the approval service consumes.
func RoutingKeys() []string { return []string{poCreatedRoutingKey} }

// HandleEvent turns an inbound domain event into an approval request when the
// originating service asked for one. The event carries requires_approval so the
// rule (threshold) stays in po-service, the service that owns it.
func (s *Service) HandleEvent(ctx context.Context, evt broker.Event) error {
	if evt.Name != poCreatedRoutingKey {
		return nil
	}

	payload := decodePayload(evt.Payload)
	if !boolField(payload, "requires_approval") {
		// Below threshold: nothing to approve.
		return nil
	}

	entityID := stringField(payload, "po_id")
	if entityID == "" {
		return fmt.Errorf("po.created without po_id")
	}

	// Replayed events must not open a second approval.
	claimed, err := s.cache.ClaimIdempotency(ctx, "approval-from-event:"+evt.ID, eventIdempotencyTTL)
	if err != nil {
		return fmt.Errorf("claim approval event: %w", err)
	}
	if !claimed {
		s.log.DebugContext(ctx, "approval event already handled", slog.String("event_id", evt.ID))
		return nil
	}

	// The unique (entity_type, entity_id) index is the second guard.
	if existing, err := s.repo.FindByEntity(ctx, evt.TenantID, EntityPurchaseOrder, entityID); err == nil {
		s.log.InfoContext(ctx, "approval already exists for purchase order",
			slog.String("approval_id", existing.ID), slog.String("po_id", entityID))
		return nil
	}

	amount, currency := moneyField(payload, "total")
	if currency == "" {
		currency = "IDR"
	}

	created, err := s.Request(ctx, RequestInput{
		TenantID:   evt.TenantID,
		EntityType: string(EntityPurchaseOrder),
		EntityID:   entityID,
		Amount:     amount,
		Currency:   currency,
		AssignedTo: s.assignee,
		Comment:    "otomatis dibuat dari event po.created (nominal di atas threshold)",
	}, evt.ActorID)
	if err != nil {
		return fmt.Errorf("auto-create approval for %s: %w", entityID, err)
	}

	s.log.InfoContext(ctx, "approval auto-created",
		slog.String("approval_id", created.ID),
		slog.String("po_id", entityID),
		slog.String("assigned_to", created.AssignedTo),
		slog.Int64("amount", amount))
	return nil
}

// Handler adapts HandleEvent to the broker consumer signature.
func (s *Service) Handler() broker.Handler {
	return func(ctx context.Context, evt broker.Event) error { return s.HandleEvent(ctx, evt) }
}

// RemindersDue announces pending approvals that have waited longer than the
// reminder window. It walks the tenants the service knows about, because the
// repository is tenant-scoped and an empty tenant filter matches nothing.
func (s *Service) RemindersDue(ctx context.Context, olderThan time.Duration, limit int64) (int, error) {
	tenants, err := s.repo.ListTenants(ctx)
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().UTC().Add(-olderThan)
	reminded := 0

	for _, tenant := range tenants {
		pending, err := s.repo.ListPendingOlderThan(ctx, tenant, cutoff, limit)
		if err != nil {
			return reminded, err
		}

		for i := range pending {
			item := pending[i]
			// A reminder must not be sent twice for the same wait window.
			claimed, err := s.cache.ClaimIdempotency(ctx,
				fmt.Sprintf("approval-reminder:%s:%s", item.ID, item.CreatedAt.UTC().Format(time.RFC3339)), 48*time.Hour)
			if err != nil || !claimed {
				continue
			}

			s.emit(ctx, broker.EventApprovalReminder, item.TenantID, "scheduler", map[string]any{
				"approval_id": item.ID,
				"entity_type": item.EntityType,
				"entity_id":   item.EntityID,
				"assigned_to": item.AssignedTo,
				"amount":      item.Amount,
				"waiting_for": time.Since(item.CreatedAt).Round(time.Minute).String(),
			})
			reminded++
		}
	}

	if reminded > 0 {
		s.log.InfoContext(ctx, "approval reminders sent", slog.Int("count", reminded))
	}
	return reminded, nil
}

func decodePayload(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func stringField(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	if value, ok := payload[key].(string); ok {
		return value
	}
	return ""
}

func boolField(payload map[string]any, key string) bool {
	if payload == nil {
		return false
	}
	value, ok := payload[key].(bool)
	return ok && value
}

func moneyField(payload map[string]any, key string) (int64, string) {
	if payload == nil {
		return 0, ""
	}
	nested, ok := payload[key].(map[string]any)
	if !ok {
		return 0, ""
	}
	amount, _ := nested["amount"].(float64)
	currency, _ := nested["currency"].(string)
	return int64(amount), currency
}

// ensure the cache import stays meaningful if idempotency moves out of Service.
var _ = cache.ErrMiss
