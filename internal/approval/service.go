package approval

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/ank/e-proc/pkg/apperr"
	"github.com/ank/e-proc/pkg/auth"
	"github.com/ank/e-proc/pkg/broker"
	"github.com/ank/e-proc/pkg/cache"
)

// cacheTTL is how long a single approval document is cached.
const cacheTTL = 2 * time.Minute

// Service applies approval workflow rules on top of the repository.
type Service struct {
	repo      *Repository
	cache     *cache.Client
	publisher *broker.Publisher
	log       *slog.Logger
	// assignee is who an auto-created approval is routed to.
	assignee string
}

// NewService builds the approval service. assignee is the default approver used
// when an approval is opened automatically from an event.
func NewService(repo *Repository, cacheClient *cache.Client, publisher *broker.Publisher, assignee string, log *slog.Logger) *Service {
	return &Service{repo: repo, cache: cacheClient, publisher: publisher, assignee: assignee, log: log}
}

// EnsureIndexes is called once at boot.
func (s *Service) EnsureIndexes(ctx context.Context) error { return s.repo.EnsureIndexes(ctx) }

// CacheKey is the Redis key for one approval document.
func CacheKey(id string) string { return "cache:approval:" + id }

// Request opens an approval request for a business document.
func (s *Service) Request(ctx context.Context, in RequestInput, actorID string) (*Approval, error) {
	in.Normalize()
	if errs := in.Validate(); errs.Any() {
		return nil, apperr.NewValidation(errs)
	}

	a := &Approval{
		TenantID:    in.TenantID,
		EntityType:  EntityType(in.EntityType),
		EntityID:    in.EntityID,
		Amount:      Money{Amount: in.Amount, Currency: in.Currency},
		Status:      StatusPending,
		RequestedBy: actorID,
		AssignedTo:  in.AssignedTo,
		Comment:     in.Comment,
		History:     []Step{},
	}
	if err := s.repo.Create(ctx, a); err != nil {
		return nil, err
	}

	s.emit(ctx, broker.EventApprovalRequested, a.TenantID, actorID, map[string]any{
		"approval_id": a.ID,
		"entity_type": a.EntityType,
		"entity_id":   a.EntityID,
		"amount":      a.Amount,
		"assigned_to": a.AssignedTo,
	})
	return a, nil
}

// Decide records an approve/reject decision, enforcing that only the assignee
// (or an admin) may decide, and that a rejection carries a reason.
func (s *Service) Decide(ctx context.Context, tenantID, id string, in DecideInput, claims *auth.Claims) (*Approval, error) {
	in.Normalize()
	if errs := in.Validate(); errs.Any() {
		return nil, apperr.NewValidation(errs)
	}

	current, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if !current.IsPending() {
		return nil, apperr.NewConflict("approval is %s and can no longer be decided", current.Status)
	}
	if !current.CanBeDecidedBy(claims.UserID, claims.IsAdmin()) {
		return nil, apperr.NewForbidden("approval is assigned to another approver")
	}

	decision := StatusRejected
	if in.Approve {
		decision = StatusApproved
	}
	step := Step{
		ActorID:   claims.UserID,
		Decision:  decision,
		Comment:   in.Comment,
		DecidedAt: time.Now().UTC(),
	}

	updated, err := s.repo.AppendDecision(ctx, tenantID, id, step, in.Comment)
	if err != nil {
		return nil, err
	}
	s.invalidate(ctx, updated.ID)

	// Every decision is announced, so projections and the audit log see it.
	s.emit(ctx, broker.EventApprovalDecided, updated.TenantID, claims.UserID, map[string]any{
		"approval_id": updated.ID,
		"entity_type": updated.EntityType,
		"entity_id":   updated.EntityID,
		"decision":    decision,
		"amount":      updated.Amount,
		"comment":     in.Comment,
	})

	// The purchase order service reacts to these events to move its own state.
	switch updated.EntityType {
	case EntityPurchaseOrder:
		name := broker.EventPOApproved
		if decision == StatusRejected {
			name = broker.EventPORejected
		}
		s.emit(ctx, name, updated.TenantID, claims.UserID, map[string]any{
			"po_id":       updated.EntityID,
			"approval_id": updated.ID,
			"amount":      updated.Amount,
			"comment":     in.Comment,
		})
	}

	s.log.InfoContext(ctx, "approval decided",
		slog.String("approval_id", updated.ID),
		slog.String("decision", string(decision)),
		slog.String("entity_id", updated.EntityID))
	return updated, nil
}

// Get returns one approval, serving it from cache when possible.
func (s *Service) Get(ctx context.Context, tenantID, id string) (*Approval, error) {
	var cached Approval
	if err := s.cache.GetJSON(ctx, CacheKey(id), &cached); err == nil && cached.TenantID == tenantID {
		return &cached, nil
	}

	a, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if err := s.cache.SetJSON(ctx, CacheKey(a.ID), a, cacheTTL); err != nil {
		s.log.WarnContext(ctx, "cache write failed", slog.String("error", err.Error()))
	}
	return a, nil
}

// List returns a page of approvals.
func (s *Service) List(ctx context.Context, f ListFilter) ([]Approval, int64, error) {
	if f.Status != "" && !IsValidStatus(f.Status) {
		return nil, 0, apperr.NewValidation(map[string]string{"status": "unknown approval status"})
	}
	return s.repo.List(ctx, f)
}

// FindByEntity returns the approval governing a business document.
func (s *Service) FindByEntity(ctx context.Context, tenantID string, entityType EntityType, entityID string) (*Approval, error) {
	return s.repo.FindByEntity(ctx, tenantID, entityType, entityID)
}

// StatsByStatus returns approval counts per status for the reporting service.
func (s *Service) StatsByStatus(ctx context.Context, tenantID string) (map[string]int64, error) {
	return s.repo.StatsByStatus(ctx, tenantID)
}

func (s *Service) invalidate(ctx context.Context, id string) {
	if err := s.cache.Delete(ctx, CacheKey(id)); err != nil {
		s.log.WarnContext(ctx, "cache invalidate failed", slog.String("error", err.Error()))
	}
}

func (s *Service) emit(ctx context.Context, name, tenantID, actorID string, payload any) {
	if s.publisher == nil {
		return
	}
	evt, err := broker.NewEvent(name, tenantID, actorID, payload)
	if err != nil {
		s.log.ErrorContext(ctx, "build event failed", slog.String("event", name), slog.String("error", err.Error()))
		return
	}
	if err := s.publisher.Publish(ctx, evt); err != nil && !errors.Is(err, context.Canceled) {
		s.log.ErrorContext(ctx, "publish event failed", slog.String("event", name), slog.String("error", err.Error()))
	}
}
