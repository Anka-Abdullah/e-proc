package po

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/ank/e-proc/pkg/broker"
	"github.com/ank/e-proc/pkg/cache"
)

// approvalEventTTL bounds how long a consumed approval event id is remembered.
const approvalEventTTL = 24 * time.Hour

// RoutingKeys lists the events the purchase order service consumes.
func RoutingKeys() []string {
	return []string{broker.EventPOApproved, broker.EventPORejected}
}

// HandleEvent applies an approval decision to its purchase order. This is what
// makes the approval step automatic: the approver decides once, and the PO moves
// without anyone patching its status from the UI.
func (s *Service) HandleEvent(ctx context.Context, evt broker.Event) error {
	if evt.Name != broker.EventPOApproved && evt.Name != broker.EventPORejected {
		return nil
	}

	payload := decodeApprovalPayload(evt.Payload)
	poID := stringFromPayload(payload, "po_id")
	approvalID := stringFromPayload(payload, "approval_id")
	if poID == "" {
		return fmt.Errorf("%s without po_id", evt.Name)
	}

	// Replayed events must not re-apply a decision.
	claimed, err := s.cache.ClaimIdempotency(ctx, "po-approval-event:"+evt.ID, approvalEventTTL)
	if err != nil {
		return fmt.Errorf("claim po approval event: %w", err)
	}
	if !claimed {
		s.log.DebugContext(ctx, "po approval event already applied", slog.String("event_id", evt.ID))
		return nil
	}

	approved := evt.Name == broker.EventPOApproved
	if err := s.ApplyApprovalDecision(ctx, evt.TenantID, poID, approved, approvalID, evt.ActorID); err != nil {
		return err
	}
	return nil
}

// Handler adapts HandleEvent to the broker consumer signature.
func (s *Service) Handler() broker.Handler {
	return func(ctx context.Context, evt broker.Event) error { return s.HandleEvent(ctx, evt) }
}

// ApplyApprovalDecision moves a purchase order according to the decision. It is
// deliberately separate from UpdateStatus: this path must NOT re-emit
// po.approved/po.rejected, or the two services would publish the same event
// forever.
func (s *Service) ApplyApprovalDecision(ctx context.Context, tenantID, poID string, approved bool, approvalID, actorID string) error {
	target := StatusRejected
	if approved {
		target = StatusApproved
	}

	current, err := s.repo.GetByID(ctx, tenantID, poID)
	if err != nil {
		return err
	}
	if current.Status == target {
		// Already applied (for example by an explicit API call); nothing to do.
		return nil
	}
	if !CanTransition(current.Status, target) {
		return fmt.Errorf("purchase order %s is %s and cannot become %s", current.Number, current.Status, target)
	}

	order, err := s.repo.SetStatus(ctx, tenantID, poID, target, approvalID)
	if err != nil {
		return err
	}
	s.invalidate(ctx, order.ID)

	s.log.InfoContext(ctx, "purchase order moved by approval decision",
		slog.String("po_id", order.ID),
		slog.String("number", order.Number),
		slog.String("status", string(order.Status)),
		slog.String("approval_id", approvalID),
		slog.String("actor_id", actorID))
	return nil
}

func decodeApprovalPayload(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func stringFromPayload(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	if value, ok := payload[key].(string); ok {
		return value
	}
	return ""
}

// keep the cache package referenced for the idempotency TTL constant.
var _ = cache.ErrMiss
