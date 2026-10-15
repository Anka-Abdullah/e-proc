package tender

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/ank/e-proc/pkg/apperr"
	"github.com/ank/e-proc/pkg/broker"
	"github.com/ank/e-proc/pkg/cache"
)

// cacheTTL is how long a single tender document is cached.
const cacheTTL = 5 * time.Minute

// Service applies tender business rules on top of the repository.
type Service struct {
	repo      *Repository
	cache     *cache.Client
	publisher *broker.Publisher
	log       *slog.Logger
}

// NewService builds the tender service.
func NewService(repo *Repository, cacheClient *cache.Client, publisher *broker.Publisher, log *slog.Logger) *Service {
	return &Service{repo: repo, cache: cacheClient, publisher: publisher, log: log}
}

// EnsureIndexes is called once at boot.
func (s *Service) EnsureIndexes(ctx context.Context) error { return s.repo.EnsureIndexes(ctx) }

// CacheKey is the Redis key for one tender document.
func CacheKey(id string) string { return "cache:tender:" + id }

// Create validates and stores a tender in draft state.
func (s *Service) Create(ctx context.Context, in CreateInput, actorID string) (*Tender, error) {
	in.Normalize()
	if errs := in.Validate(); errs.Any() {
		return nil, apperr.NewValidation(errs)
	}

	t := &Tender{
		TenantID:         in.TenantID,
		Code:             in.Code,
		Title:            in.Title,
		Description:      in.Description,
		Items:            in.Items,
		Budget:           in.Budget,
		Status:           StatusDraft,
		CloseAt:          in.CloseAt.UTC(),
		InvitedVendorIDs: in.InvitedVendorIDs,
		CreatedBy:        actorID,
	}
	if err := s.repo.Create(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// Get returns a tender, serving it from cache when possible.
func (s *Service) Get(ctx context.Context, tenantID, id string) (*Tender, error) {
	var cached Tender
	if err := s.cache.GetJSON(ctx, CacheKey(id), &cached); err == nil && cached.TenantID == tenantID {
		return &cached, nil
	}

	t, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if err := s.cache.SetJSON(ctx, CacheKey(t.ID), t, cacheTTL); err != nil {
		s.log.WarnContext(ctx, "cache write failed", slog.String("error", err.Error()))
	}
	return t, nil
}

// Publish moves a draft tender to published and announces it to vendors.
func (s *Service) Publish(ctx context.Context, tenantID, id, actorID string) (*Tender, error) {
	current, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if !current.CanPublish() {
		return nil, apperr.NewConflict("tender %s is %s and cannot be published", current.Code, current.Status)
	}
	if len(current.Items) == 0 {
		return nil, apperr.NewValidation(map[string]string{"items": "a tender needs at least one item before it can be published"})
	}
	if current.IsOpenAt(time.Now().UTC()) {
		return nil, apperr.NewValidation(map[string]string{"close_at": "close_at must be in the future"})
	}

	t, err := s.repo.SetStatus(ctx, tenantID, id, StatusPublished)
	if err != nil {
		return nil, err
	}
	s.invalidate(ctx, t.ID)
	s.emit(ctx, broker.EventTenderPublished, t.TenantID, actorID, map[string]any{
		"tender_id":          t.ID,
		"code":               t.Code,
		"title":              t.Title,
		"close_at":           t.CloseAt,
		"invited_vendor_ids": t.InvitedVendorIDs,
	})
	return t, nil
}

// Close moves a published tender to closed.
func (s *Service) Close(ctx context.Context, tenantID, id, actorID string) (*Tender, error) {
	current, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if !current.CanClose() {
		return nil, apperr.NewConflict("tender %s is %s and cannot be closed", current.Code, current.Status)
	}

	t, err := s.repo.SetStatus(ctx, tenantID, id, StatusClosed)
	if err != nil {
		return nil, err
	}
	s.invalidate(ctx, t.ID)
	s.emit(ctx, broker.EventTenderClosed, t.TenantID, actorID, map[string]any{
		"tender_id": t.ID,
		"code":      t.Code,
		"closed_by": actorID,
	})
	return t, nil
}

// List returns a page of tenders.
func (s *Service) List(ctx context.Context, f ListFilter) ([]Tender, int64, error) {
	if f.Status != "" && !IsValidStatus(f.Status) {
		return nil, 0, apperr.NewValidation(map[string]string{"status": "unknown tender status"})
	}
	return s.repo.List(ctx, f)
}

// AutoClose closes every published tender whose deadline has passed. It is
// driven by a scheduler in the service entrypoint.
func (s *Service) AutoClose(ctx context.Context) (int, error) {
	due, err := s.repo.ListDueForClosing(ctx, time.Now().UTC(), 100)
	if err != nil {
		return 0, err
	}

	closed := 0
	for i := range due {
		t, err := s.repo.SetStatus(ctx, due[i].TenantID, due[i].ID, StatusClosed)
		if err != nil {
			if errors.Is(err, apperr.ErrNotFound) {
				continue
			}
			s.log.ErrorContext(ctx, "auto-close failed",
				slog.String("tender_id", due[i].ID), slog.String("error", err.Error()))
			continue
		}
		s.invalidate(ctx, t.ID)
		s.emit(ctx, broker.EventTenderClosed, t.TenantID, "scheduler", map[string]any{
			"tender_id": t.ID,
			"code":      t.Code,
			"reason":    "close_at reached",
		})
		closed++
	}
	if closed > 0 {
		s.log.InfoContext(ctx, "tenders auto-closed", slog.Int("count", closed))
	}
	return closed, nil
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
