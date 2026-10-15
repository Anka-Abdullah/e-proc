package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ank/e-proc/pkg/apperr"
	"github.com/ank/e-proc/pkg/broker"
	"github.com/ank/e-proc/pkg/cache"
)

// idempotencyTTL bounds how long a processed event id is remembered.
const idempotencyTTL = 24 * time.Hour

// entityByEvent maps an event name to the entity it concerns, and picks the id
// out of the payload so the log can be queried by entity.
var entityByEvent = map[string]struct {
	EntityType string
	IDField    string
}{
	broker.EventVendorCreated:     {"vendor", "vendor_id"},
	broker.EventVendorUpdated:     {"vendor", "vendor_id"},
	broker.EventTenderPublished:   {"tender", "tender_id"},
	broker.EventTenderClosed:      {"tender", "tender_id"},
	broker.EventBidSubmitted:      {"bid", "bid_id"},
	broker.EventBidSelected:       {"bid", "bid_id"},
	broker.EventPOCreated:         {"purchase_order", "po_id"},
	broker.EventPOApproved:        {"purchase_order", "po_id"},
	broker.EventPOIssued:          {"purchase_order", "po_id"},
	broker.EventPORejected:        {"purchase_order", "po_id"},
	broker.EventPOClosed:          {"purchase_order", "po_id"},
	broker.EventApprovalRequested: {"approval", "approval_id"},
	broker.EventApprovalDecided:   {"approval", "approval_id"},
}

// RoutingKeys lists every event the audit service subscribes to.
func RoutingKeys() []string {
	keys := make([]string, 0, len(entityByEvent))
	for name := range entityByEvent {
		keys = append(keys, name)
	}
	return keys
}

// Service turns domain events into audit entries.
type Service struct {
	repo  *Repository
	cache *cache.Client
	log   *slog.Logger
}

// NewService builds the audit service.
func NewService(repo *Repository, cacheClient *cache.Client, log *slog.Logger) *Service {
	return &Service{repo: repo, cache: cacheClient, log: log}
}

// EnsureIndexes is called once at boot.
func (s *Service) EnsureIndexes(ctx context.Context) error { return s.repo.EnsureIndexes(ctx) }

// Record appends an audit entry for a domain event. Redelivered events are
// ignored through an idempotency claim, so the log stays duplicate-free even
// when RabbitMQ delivers twice.
func (s *Service) Record(ctx context.Context, evt broker.Event) error {
	claimed, err := s.cache.ClaimIdempotency(ctx, "audit:"+evt.ID, idempotencyTTL)
	if err != nil {
		return fmt.Errorf("claim audit event: %w", err)
	}
	if !claimed {
		s.log.DebugContext(ctx, "audit event already recorded", slog.String("event_id", evt.ID))
		return nil
	}

	entry := Entry{
		TenantID:  evt.TenantID,
		ActorID:   evt.ActorID,
		Action:    evt.Name,
		EventID:   evt.ID,
		CreatedAt: evt.OccurredAt.UTC(),
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}

	if meta, ok := entityByEvent[evt.Name]; ok {
		entry.EntityType = meta.EntityType
		entry.EntityID = payloadString(evt.Payload, meta.IDField)
	}
	entry.Summary = summarise(evt.Name, entry.EntityID)
	entry.Payload = payloadMap(evt.Payload)

	if err := s.repo.Insert(ctx, &entry); err != nil {
		if _, conflict := apperr.AsConflict(err); conflict {
			// The unique event_id index caught a concurrent duplicate.
			return nil
		}
		return err
	}
	return nil
}

// Handler adapts Record to the broker consumer signature.
func (s *Service) Handler() broker.Handler {
	return func(ctx context.Context, evt broker.Event) error { return s.Record(ctx, evt) }
}

// Get returns one audit entry.
func (s *Service) Get(ctx context.Context, tenantID, id string) (*Entry, error) {
	return s.repo.GetByID(ctx, tenantID, id)
}

// List returns a page of audit entries.
func (s *Service) List(ctx context.Context, f Filter) ([]Entry, int64, error) {
	f.Normalize()
	if errs := f.Validate(); errs.Any() {
		return nil, 0, apperr.NewValidation(errs)
	}
	return s.repo.List(ctx, f)
}

// CountByAction returns action counts since a point in time.
func (s *Service) CountByAction(ctx context.Context, tenantID string, since time.Time) (map[string]int64, error) {
	return s.repo.CountByAction(ctx, tenantID, since)
}

func payloadMap(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func payloadString(raw json.RawMessage, field string) string {
	m := payloadMap(raw)
	if m == nil {
		return ""
	}
	value, ok := m[field]
	if !ok {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
}

func summarise(name, entityID string) string {
	action := strings.ReplaceAll(name, ".", " ")
	if entityID == "" {
		return action
	}
	return action + " " + entityID
}

// ErrUnsupportedEvent reports an event with no audit mapping.
var ErrUnsupportedEvent = errors.New("unsupported event")
