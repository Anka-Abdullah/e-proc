package po

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ank/e-proc/pkg/apperr"
	"github.com/ank/e-proc/pkg/broker"
	"github.com/ank/e-proc/pkg/cache"
)

// cacheTTL is how long a single purchase order document is cached.
const cacheTTL = 5 * time.Minute

// Service applies purchase order business rules on top of the repository.
type Service struct {
	repo      *Repository
	cache     *cache.Client
	publisher *broker.Publisher
	threshold float64
	log       *slog.Logger
}

// NewService builds the purchase order service.
func NewService(repo *Repository, cacheClient *cache.Client, publisher *broker.Publisher, threshold float64, log *slog.Logger) *Service {
	return &Service{repo: repo, cache: cacheClient, publisher: publisher, threshold: threshold, log: log}
}

// EnsureIndexes is called once at boot.
func (s *Service) EnsureIndexes(ctx context.Context) error { return s.repo.EnsureIndexes(ctx) }

// CacheKey is the Redis key for one purchase order document.
func CacheKey(id string) string { return "cache:po:" + id }

// Create prices the order server-side, generates its number and routes it to
// approval when the total reaches the configured threshold.
func (s *Service) Create(ctx context.Context, in CreateInput, actorID string) (*PurchaseOrder, bool, error) {
	in.Normalize()
	if errs := in.Validate(); errs.Any() {
		return nil, false, apperr.NewValidation(errs)
	}

	lines, total := in.PriceLines()
	requiresApproval := RequiresApproval(total.Amount, s.threshold)

	seq, err := s.repo.NextSequence(ctx, in.TenantID)
	if err != nil {
		return nil, false, err
	}

	order := &PurchaseOrder{
		TenantID:  in.TenantID,
		Number:    fmt.Sprintf("PO-%d-%04d", time.Now().UTC().Year(), seq),
		TenderID:  in.TenderID,
		BidID:     in.BidID,
		VendorID:  in.VendorID,
		Lines:     lines,
		Total:     total,
		Status:    StatusDraft,
		Notes:     in.Notes,
		CreatedBy: actorID,
	}
	if requiresApproval {
		order.Status = StatusPendingApproval
	}

	if err := s.repo.Create(ctx, order); err != nil {
		return nil, false, err
	}

	s.emit(ctx, broker.EventPOCreated, order.TenantID, actorID, map[string]any{
		"po_id":             order.ID,
		"number":            order.Number,
		"tender_id":         order.TenderID,
		"bid_id":            order.BidID,
		"vendor_id":         order.VendorID,
		"total":             order.Total,
		"requires_approval": requiresApproval,
		"status":            order.Status,
	})
	return order, requiresApproval, nil
}

// Get returns a purchase order, serving it from cache when possible.
func (s *Service) Get(ctx context.Context, tenantID, id string) (*PurchaseOrder, error) {
	var cached PurchaseOrder
	if err := s.cache.GetJSON(ctx, CacheKey(id), &cached); err == nil && cached.TenantID == tenantID {
		return &cached, nil
	}

	order, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if err := s.cache.SetJSON(ctx, CacheKey(order.ID), order, cacheTTL); err != nil {
		s.log.WarnContext(ctx, "cache write failed", slog.String("error", err.Error()))
	}
	return order, nil
}

// List returns a page of purchase orders.
func (s *Service) List(ctx context.Context, f ListFilter) ([]PurchaseOrder, int64, error) {
	if f.Status != "" && !IsValidStatus(f.Status) {
		return nil, 0, apperr.NewValidation(map[string]string{"status": "unknown purchase order status"})
	}
	return s.repo.List(ctx, f)
}

// UpdateStatus applies a status change only when the state machine allows it.
func (s *Service) UpdateStatus(ctx context.Context, tenantID, id string, status Status, actorID, approvalID string) (*PurchaseOrder, error) {
	if !IsValidStatus(status) {
		return nil, apperr.NewValidation(map[string]string{"status": "unknown purchase order status"})
	}

	current, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if !CanTransition(current.Status, status) {
		return nil, apperr.NewConflict("purchase order %s cannot move from %s to %s (allowed: %s)",
			current.Number, current.Status, status, joinStatuses(NextStatuses(current.Status)))
	}

	order, err := s.repo.SetStatus(ctx, tenantID, id, status, approvalID)
	if err != nil {
		return nil, err
	}
	s.invalidate(ctx, order.ID)

	switch status {
	case StatusApproved:
		s.emit(ctx, broker.EventPOApproved, order.TenantID, actorID, map[string]any{
			"po_id": order.ID, "number": order.Number, "approval_id": order.ApprovalID, "total": order.Total,
		})
	case StatusIssued:
		s.emit(ctx, broker.EventPOIssued, order.TenantID, actorID, map[string]any{
			"po_id": order.ID, "number": order.Number, "vendor_id": order.VendorID, "total": order.Total,
		})
	case StatusRejected:
		s.emit(ctx, broker.EventPORejected, order.TenantID, actorID, map[string]any{
			"po_id": order.ID, "number": order.Number, "approval_id": order.ApprovalID,
		})
	}
	return order, nil
}

// Close moves an approved or issued order to closed.
func (s *Service) Close(ctx context.Context, tenantID, id, actorID string) (*PurchaseOrder, error) {
	order, err := s.UpdateStatus(ctx, tenantID, id, StatusClosed, actorID, "")
	if err != nil {
		return nil, err
	}
	s.emit(ctx, broker.EventPOClosed, order.TenantID, actorID, map[string]any{
		"po_id": order.ID, "number": order.Number, "closed_by": actorID,
	})
	return order, nil
}

// LinkApproval records which approval request governs this order.
func (s *Service) LinkApproval(ctx context.Context, tenantID, id, approvalID string) error {
	if err := s.repo.SetApprovalID(ctx, tenantID, id, approvalID); err != nil {
		return err
	}
	s.invalidate(ctx, id)
	return nil
}

// ByVendor returns the purchase-order-per-vendor report rows.
func (s *Service) ByVendor(ctx context.Context, tenantID string) ([]VendorTotal, error) {
	return s.repo.AggregateByVendor(ctx, tenantID)
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

func joinStatuses(statuses []Status) string {
	parts := make([]string, 0, len(statuses))
	for _, s := range statuses {
		parts = append(parts, string(s))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}
