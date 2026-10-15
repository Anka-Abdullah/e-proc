package reporting

import (
	"context"
)

// Service publishes the report queries and owns the projector's lifecycle.
type Service struct {
	repo      *Repository
	projector *Projector
}

// NewService builds the reporting service.
func NewService(repo *Repository, projector *Projector) *Service {
	return &Service{repo: repo, projector: projector}
}

// EnsureIndexes prepares the projection collections.
func (s *Service) EnsureIndexes(ctx context.Context) error { return s.projector.EnsureIndexes(ctx) }

// PurchaseOrdersByVendor returns the spend report for a tenant.
func (s *Service) PurchaseOrdersByVendor(ctx context.Context, tenantID string) ([]VendorSpend, error) {
	return s.repo.PurchaseOrdersByVendor(ctx, tenantID)
}

// TenderSummary returns the tender report for a tenant.
func (s *Service) TenderSummary(ctx context.Context, tenantID string) (TenderSummary, error) {
	return s.repo.TenderSummary(ctx, tenantID)
}

// ApprovalSummary returns the approval report for a tenant.
func (s *Service) ApprovalSummary(ctx context.Context, tenantID string) (ApprovalSummary, error) {
	return s.repo.ApprovalSummary(ctx, tenantID)
}
