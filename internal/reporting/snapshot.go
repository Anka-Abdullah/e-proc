package reporting

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/ank/e-proc/pkg/db"
)

// Snapshot is a periodic record of the numbers, so "what did the report say last
// week?" has an answer instead of being recomputed from whatever the projection
// happens to hold now.
type Snapshot struct {
	ID              string          `bson:"_id" json:"id"`
	TenantID        string          `bson:"tenant_id" json:"tenant_id"`
	GeneratedAt     time.Time       `bson:"generated_at" json:"generated_at"`
	VendorSpend     []VendorSpend   `bson:"vendor_spend" json:"vendor_spend"`
	TenderSummary   TenderSummary   `bson:"tender_summary" json:"tender_summary"`
	ApprovalSummary ApprovalSummary `bson:"approval_summary" json:"approval_summary"`
}

// EnsureSnapshotIndexes prepares the snapshot collection.
func (r *Repository) EnsureSnapshotIndexes(ctx context.Context) error {
	return db.EnsureIndexes(ctx, r.snapshots,
		mongo.IndexModel{
			Keys:    bson.D{{Key: "tenant_id", Value: 1}, {Key: "generated_at", Value: -1}},
			Options: options.Index().SetName("tenant_generated"),
		},
	)
}

// SaveSnapshot stores one snapshot.
func (r *Repository) SaveSnapshot(ctx context.Context, snapshot Snapshot) error {
	if _, err := r.snapshots.InsertOne(ctx, snapshot); err != nil {
		return fmt.Errorf("insert report snapshot: %w", err)
	}
	return nil
}

// LatestSnapshot returns the newest snapshot for a tenant.
func (r *Repository) LatestSnapshot(ctx context.Context, tenantID string) (*Snapshot, error) {
	var snapshot Snapshot
	err := r.snapshots.FindOne(ctx,
		bson.M{"tenant_id": tenantID},
		options.FindOne().SetSort(bson.D{{Key: "generated_at", Value: -1}})).Decode(&snapshot)
	if err != nil {
		return nil, err
	}
	return &snapshot, nil
}

// ListTenants reports which tenants the projections know about. The reporting
// service has no tenant registry of its own, so it derives them from the data it
// already projected. Aggregation is used rather than Distinct, whose driver v2
// API returns a result wrapper instead of (values, error).
func (r *Repository) ListTenants(ctx context.Context) ([]string, error) {
	seen := map[string]struct{}{}
	tenants := make([]string, 0, 8)

	for _, collection := range []*mongo.Collection{r.tenders, r.pos, r.approvals} {
		cursor, err := collection.Aggregate(ctx, mongo.Pipeline{
			{{Key: "$group", Value: bson.M{"_id": "$tenant_id"}}},
		})
		if err != nil {
			return nil, fmt.Errorf("list tenants from %s: %w", collection.Name(), err)
		}

		var rows []struct {
			TenantID string `bson:"_id"`
		}
		if err := cursor.All(ctx, &rows); err != nil {
			_ = cursor.Close(ctx)
			return nil, fmt.Errorf("decode tenants from %s: %w", collection.Name(), err)
		}
		_ = cursor.Close(ctx)

		for _, row := range rows {
			if row.TenantID == "" {
				continue
			}
			if _, exists := seen[row.TenantID]; exists {
				continue
			}
			seen[row.TenantID] = struct{}{}
			tenants = append(tenants, row.TenantID)
		}
	}

	return tenants, nil
}

// SnapshotAll writes one snapshot per known tenant and returns how many were
// written. It is driven by a scheduler.
func (s *Service) SnapshotAll(ctx context.Context) (int, error) {
	tenants, err := s.repo.ListTenants(ctx)
	if err != nil {
		return 0, err
	}

	written := 0
	for _, tenant := range tenants {
		snapshot, err := s.BuildSnapshot(ctx, tenant)
		if err != nil {
			return written, err
		}
		if err := s.repo.SaveSnapshot(ctx, snapshot); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

// BuildSnapshot assembles the numbers for one tenant without storing them.
func (s *Service) BuildSnapshot(ctx context.Context, tenantID string) (Snapshot, error) {
	spend, err := s.repo.PurchaseOrdersByVendor(ctx, tenantID)
	if err != nil {
		return Snapshot{}, err
	}
	tenders, err := s.repo.TenderSummary(ctx, tenantID)
	if err != nil {
		return Snapshot{}, err
	}
	approvals, err := s.repo.ApprovalSummary(ctx, tenantID)
	if err != nil {
		return Snapshot{}, err
	}

	return Snapshot{
		ID:              fmt.Sprintf("%s-%d", tenantID, time.Now().UTC().UnixNano()),
		TenantID:        tenantID,
		GeneratedAt:     time.Now().UTC(),
		VendorSpend:     spend,
		TenderSummary:   tenders,
		ApprovalSummary: approvals,
	}, nil
}

// LatestSnapshot returns the most recent stored snapshot for a tenant.
func (s *Service) LatestSnapshot(ctx context.Context, tenantID string) (*Snapshot, error) {
	return s.repo.LatestSnapshot(ctx, tenantID)
}

// LogSnapshot writes the headline numbers to the service log, which is the
// cheapest useful form of a scheduled report before an email integration exists.
func LogSnapshot(snapshot Snapshot, log *slog.Logger) {
	log.Info("scheduled report snapshot",
		slog.String("tenant", snapshot.TenantID),
		slog.Int("tenders", int(snapshot.TenderSummary.Total)),
		slog.Int("open_tenders", int(snapshot.TenderSummary.OpenTenders)),
		slog.Int("vendors_with_spend", len(snapshot.VendorSpend)),
		slog.Int("approvals", int(snapshot.ApprovalSummary.Total)),
		slog.Int("approvals_pending", int(snapshot.ApprovalSummary.ByStatus["pending"])),
	)
}
