package reporting

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/ank/e-proc/pkg/db"
)

// Repository stores and queries the reporting projections.
type Repository struct {
	pos       *mongo.Collection
	tenders   *mongo.Collection
	approvals *mongo.Collection
}

// NewRepository binds the three projection collections.
func NewRepository(database *mongo.Database) *Repository {
	return &Repository{
		pos:       database.Collection("reporting_purchase_orders"),
		tenders:   database.Collection("reporting_tenders"),
		approvals: database.Collection("reporting_approvals"),
	}
}

// EnsureIndexes creates the projection indexes.
func (r *Repository) EnsureIndexes(ctx context.Context) error {
	if err := db.EnsureIndexes(ctx, r.pos,
		mongo.IndexModel{
			Keys:    bson.D{{Key: "tenant_id", Value: 1}, {Key: "vendor_id", Value: 1}},
			Options: options.Index().SetName("tenant_vendor"),
		},
		mongo.IndexModel{
			Keys:    bson.D{{Key: "tenant_id", Value: 1}, {Key: "status", Value: 1}},
			Options: options.Index().SetName("tenant_status"),
		},
	); err != nil {
		return err
	}

	if err := db.EnsureIndexes(ctx, r.tenders,
		mongo.IndexModel{
			Keys:    bson.D{{Key: "tenant_id", Value: 1}, {Key: "status", Value: 1}},
			Options: options.Index().SetName("tenant_status"),
		},
	); err != nil {
		return err
	}

	return db.EnsureIndexes(ctx, r.approvals,
		mongo.IndexModel{
			Keys:    bson.D{{Key: "tenant_id", Value: 1}, {Key: "status", Value: 1}},
			Options: options.Index().SetName("tenant_status"),
		},
		mongo.IndexModel{
			Keys:    bson.D{{Key: "entity_id", Value: 1}},
			Options: options.Index().SetName("entity"),
		},
	)
}

// UpsertPO inserts or replaces a purchase order projection row.
func (r *Repository) UpsertPO(ctx context.Context, row PORow) error {
	return r.upsert(ctx, r.pos, row.ID, row)
}

// UpdatePOStatus applies a status change coming from an approval or closure.
func (r *Repository) UpdatePOStatus(ctx context.Context, id, status string, at time.Time) error {
	_, err := r.pos.UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"status": status, "updated_at": at}})
	if err != nil {
		return fmt.Errorf("update po projection: %w", err)
	}
	return nil
}

// UpsertTender inserts or replaces a tender projection row.
func (r *Repository) UpsertTender(ctx context.Context, row TenderRow) error {
	return r.upsert(ctx, r.tenders, row.ID, row)
}

// UpdateTenderStatus applies a tender status change.
func (r *Repository) UpdateTenderStatus(ctx context.Context, id, status string, at time.Time, closed bool) error {
	set := bson.M{"status": status, "updated_at": at}
	if closed {
		set["closed_at"] = at
	}
	_, err := r.tenders.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": set})
	if err != nil {
		return fmt.Errorf("update tender projection: %w", err)
	}
	return nil
}

// IncrementTenderBids counts a submitted bid on the tender projection.
func (r *Repository) IncrementTenderBids(ctx context.Context, tenderID string) error {
	_, err := r.tenders.UpdateOne(ctx, bson.M{"_id": tenderID}, bson.M{"$inc": bson.M{"bids": 1}})
	if err != nil {
		return fmt.Errorf("increment tender bids: %w", err)
	}
	return nil
}

// UpsertApproval inserts or replaces an approval projection row.
func (r *Repository) UpsertApproval(ctx context.Context, row ApprovalRow) error {
	return r.upsert(ctx, r.approvals, row.ID, row)
}

// UpdateApprovalStatus applies a decision to the approval projection.
func (r *Repository) UpdateApprovalStatus(ctx context.Context, id, status string, at time.Time) error {
	_, err := r.approvals.UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"status": status, "decided_at": at}})
	if err != nil {
		return fmt.Errorf("update approval projection: %w", err)
	}
	return nil
}

func (r *Repository) upsert(ctx context.Context, coll *mongo.Collection, id string, row any) error {
	opts := options.Replace().SetUpsert(true)
	if _, err := coll.ReplaceOne(ctx, bson.M{"_id": id}, row, opts); err != nil {
		return fmt.Errorf("upsert into %s: %w", coll.Name(), err)
	}
	return nil
}

// PurchaseOrdersByVendor returns the spend report rows for a tenant.
func (r *Repository) PurchaseOrdersByVendor(ctx context.Context, tenantID string) ([]VendorSpend, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"tenant_id": tenantID, "status": bson.M{"$in": bson.A{"approved", "issued", "closed"}}}}},
		{{Key: "$group", Value: bson.M{
			"_id":         "$vendor_id",
			"order_count": bson.M{"$sum": 1},
			"total":       bson.M{"$sum": "$total"},
			"currency":    bson.M{"$first": "$currency"},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "total", Value: -1}}}},
	}

	cursor, err := r.pos.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("aggregate purchase orders by vendor: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()

	var raw []struct {
		VendorID   string `bson:"_id"`
		OrderCount int64  `bson:"order_count"`
		Total      int64  `bson:"total"`
		Currency   string `bson:"currency"`
	}
	if err := cursor.All(ctx, &raw); err != nil {
		return nil, fmt.Errorf("decode vendor spend: %w", err)
	}

	rows := make([]VendorSpend, 0, len(raw))
	for _, row := range raw {
		rows = append(rows, VendorSpend{VendorID: row.VendorID, OrderCount: row.OrderCount, Total: row.Total, Currency: row.Currency})
	}
	return rows, nil
}

// TenderSummary builds the tender report.
func (r *Repository) TenderSummary(ctx context.Context, tenantID string) (TenderSummary, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"tenant_id": tenantID}}},
		{{Key: "$group", Value: bson.M{
			"_id":      "$status",
			"count":    bson.M{"$sum": 1},
			"budget":   bson.M{"$sum": "$budget"},
			"bids":     bson.M{"$sum": "$bids"},
			"currency": bson.M{"$first": "$currency"},
		}}},
	}

	cursor, err := r.tenders.Aggregate(ctx, pipeline)
	if err != nil {
		return TenderSummary{}, fmt.Errorf("aggregate tenders: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()

	var raw []struct {
		Status   string `bson:"_id"`
		Count    int64  `bson:"count"`
		Budget   int64  `bson:"budget"`
		Bids     int    `bson:"bids"`
		Currency string `bson:"currency"`
	}
	if err := cursor.All(ctx, &raw); err != nil {
		return TenderSummary{}, fmt.Errorf("decode tender summary: %w", err)
	}

	summary := TenderSummary{ByStatus: map[string]int64{}, Currency: "IDR"}
	for _, row := range raw {
		summary.Total += row.Count
		summary.ByStatus[row.Status] = row.Count
		summary.TotalBudget += row.Budget
		summary.TotalBids += row.Bids
		if row.Currency != "" {
			summary.Currency = row.Currency
		}
	}
	summary.OpenTenders = summary.ByStatus["published"]
	summary.ClosedTenders = summary.ByStatus["closed"]
	return summary, nil
}

// ApprovalSummary builds the approval report.
func (r *Repository) ApprovalSummary(ctx context.Context, tenantID string) (ApprovalSummary, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"tenant_id": tenantID}}},
		{{Key: "$group", Value: bson.M{
			"_id":      "$status",
			"count":    bson.M{"$sum": 1},
			"value":    bson.M{"$sum": "$amount"},
			"currency": bson.M{"$first": "$currency"},
		}}},
	}

	cursor, err := r.approvals.Aggregate(ctx, pipeline)
	if err != nil {
		return ApprovalSummary{}, fmt.Errorf("aggregate approvals: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()

	var raw []struct {
		Status   string `bson:"_id"`
		Count    int64  `bson:"count"`
		Value    int64  `bson:"value"`
		Currency string `bson:"currency"`
	}
	if err := cursor.All(ctx, &raw); err != nil {
		return ApprovalSummary{}, fmt.Errorf("decode approval summary: %w", err)
	}

	summary := ApprovalSummary{ByStatus: map[string]int64{}, Currency: "IDR"}
	for _, row := range raw {
		summary.Total += row.Count
		summary.ByStatus[row.Status] = row.Count
		switch row.Status {
		case "pending":
			summary.PendingValue += row.Value
		case "approved":
			summary.ApprovedValue += row.Value
		case "rejected":
			summary.RejectedValue += row.Value
		}
		if row.Currency != "" {
			summary.Currency = row.Currency
		}
	}
	return summary, nil
}
