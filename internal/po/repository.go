package po

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/ank/e-proc/pkg/apperr"
	"github.com/ank/e-proc/pkg/db"
	"github.com/ank/e-proc/pkg/httpx"
)

// Repository persists purchase orders in MongoDB.
type Repository struct {
	coll *mongo.Collection
}

// NewRepository returns a repository bound to the purchase_orders collection.
func NewRepository(database *mongo.Database) *Repository {
	return &Repository{coll: database.Collection("purchase_orders")}
}

// EnsureIndexes creates the purchase order indexes.
func (r *Repository) EnsureIndexes(ctx context.Context) error {
	return db.EnsureIndexes(ctx, r.coll,
		mongo.IndexModel{
			Keys:    bson.D{{Key: "tenant_id", Value: 1}, {Key: "number", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uniq_tenant_number"),
		},
		mongo.IndexModel{
			Keys:    bson.D{{Key: "tenant_id", Value: 1}, {Key: "vendor_id", Value: 1}, {Key: "created_at", Value: -1}},
			Options: options.Index().SetName("tenant_vendor_created"),
		},
		mongo.IndexModel{
			Keys:    bson.D{{Key: "tenant_id", Value: 1}, {Key: "status", Value: 1}, {Key: "updated_at", Value: -1}},
			Options: options.Index().SetName("tenant_status_updated"),
		},
		mongo.IndexModel{
			Keys:    bson.D{{Key: "approval_id", Value: 1}},
			Options: options.Index().SetName("approval_id").SetSparse(true),
		},
	)
}

// Create inserts a purchase order; a duplicate number is a conflict.
func (r *Repository) Create(ctx context.Context, p *PurchaseOrder) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	p.CreatedAt, p.UpdatedAt = now, now
	if p.Status == "" {
		p.Status = StatusDraft
	}

	if _, err := r.coll.InsertOne(ctx, p); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return apperr.NewConflict("purchase order number %q already exists", p.Number)
		}
		return fmt.Errorf("insert purchase order: %w", err)
	}
	return nil
}

// GetByID loads one purchase order scoped to its tenant.
func (r *Repository) GetByID(ctx context.Context, tenantID, id string) (*PurchaseOrder, error) {
	var p PurchaseOrder
	err := r.coll.FindOne(ctx, bson.M{"_id": id, "tenant_id": tenantID}).Decode(&p)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, apperr.ErrNotFound
		}
		return nil, fmt.Errorf("find purchase order: %w", err)
	}
	return &p, nil
}

// NextSequence counts existing orders for a tenant so the number can be
// generated as PO-<year>-<seq>.
func (r *Repository) NextSequence(ctx context.Context, tenantID string) (int64, error) {
	count, err := r.coll.CountDocuments(ctx, bson.M{"tenant_id": tenantID})
	if err != nil {
		return 0, fmt.Errorf("count purchase orders: %w", err)
	}
	return count + 1, nil
}

// SetStatus applies a status change and stamps the matching timestamp.
func (r *Repository) SetStatus(ctx context.Context, tenantID, id string, status Status, approvalID string) (*PurchaseOrder, error) {
	now := time.Now().UTC()
	set := bson.M{"status": status, "updated_at": now}
	switch status {
	case StatusIssued:
		set["issued_at"] = now
	case StatusClosed:
		set["closed_at"] = now
	}
	if approvalID != "" {
		set["approval_id"] = approvalID
	}

	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var p PurchaseOrder
	err := r.coll.FindOneAndUpdate(ctx, bson.M{"_id": id, "tenant_id": tenantID}, bson.M{"$set": set}, opts).Decode(&p)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, apperr.ErrNotFound
		}
		return nil, fmt.Errorf("set purchase order status: %w", err)
	}
	return &p, nil
}

// SetApprovalID links an approval request to its purchase order.
func (r *Repository) SetApprovalID(ctx context.Context, tenantID, id, approvalID string) error {
	_, err := r.coll.UpdateOne(ctx,
		bson.M{"_id": id, "tenant_id": tenantID},
		bson.M{"$set": bson.M{"approval_id": approvalID, "updated_at": time.Now().UTC()}})
	if err != nil {
		return fmt.Errorf("link approval to purchase order: %w", err)
	}
	return nil
}

// List returns a page of purchase orders matching the filter.
func (r *Repository) List(ctx context.Context, f ListFilter) ([]PurchaseOrder, int64, error) {
	filter := bson.M{"tenant_id": f.TenantID}
	if f.VendorID != "" {
		filter["vendor_id"] = f.VendorID
	}
	if f.Status != "" {
		filter["status"] = f.Status
	}

	total, err := r.coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("count purchase orders: %w", err)
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip(httpx.Skip(f.Page, f.PerPage)).
		SetLimit(int64(f.PerPage))

	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("list purchase orders: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()

	orders := make([]PurchaseOrder, 0, f.PerPage)
	if err := cursor.All(ctx, &orders); err != nil {
		return nil, 0, fmt.Errorf("decode purchase orders: %w", err)
	}
	return orders, total, nil
}

// AggregateByVendor sums approved/issued/closed order value per vendor, which
// the reporting service consumes.
func (r *Repository) AggregateByVendor(ctx context.Context, tenantID string) ([]VendorTotal, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"tenant_id": tenantID,
			"status":    bson.M{"$in": bson.A{StatusApproved, StatusIssued, StatusClosed}},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id":         "$vendor_id",
			"total":       bson.M{"$sum": "$total.amount"},
			"order_count": bson.M{"$sum": 1},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "total", Value: -1}}}},
	}

	cursor, err := r.coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("aggregate purchase orders by vendor: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()

	var raw []struct {
		VendorID   string `bson:"_id"`
		Total      int64  `bson:"total"`
		OrderCount int64  `bson:"order_count"`
	}
	if err := cursor.All(ctx, &raw); err != nil {
		return nil, fmt.Errorf("decode vendor totals: %w", err)
	}

	out := make([]VendorTotal, 0, len(raw))
	for _, row := range raw {
		out = append(out, VendorTotal{VendorID: row.VendorID, Total: row.Total, OrderCount: row.OrderCount})
	}
	return out, nil
}

// VendorTotal is one row of the purchase-order-per-vendor report.
type VendorTotal struct {
	VendorID   string `json:"vendor_id"`
	Total      int64  `json:"total"`
	OrderCount int64  `json:"order_count"`
}
