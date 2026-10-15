package bid

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

// Repository persists bids in MongoDB.
type Repository struct {
	coll *mongo.Collection
}

// NewRepository returns a repository bound to the bids collection.
func NewRepository(database *mongo.Database) *Repository {
	return &Repository{coll: database.Collection("bids")}
}

// EnsureIndexes creates the bid indexes. The unique (tender, vendor) index is
// what enforces "one quotation per vendor per tender" under concurrency.
func (r *Repository) EnsureIndexes(ctx context.Context) error {
	return db.EnsureIndexes(ctx, r.coll,
		mongo.IndexModel{
			Keys:    bson.D{{Key: "tender_id", Value: 1}, {Key: "vendor_id", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uniq_tender_vendor"),
		},
		mongo.IndexModel{
			Keys:    bson.D{{Key: "tender_id", Value: 1}, {Key: "total.amount", Value: 1}},
			Options: options.Index().SetName("tender_total"),
		},
		mongo.IndexModel{
			Keys:    bson.D{{Key: "vendor_id", Value: 1}, {Key: "submitted_at", Value: -1}},
			Options: options.Index().SetName("vendor_submitted"),
		},
		mongo.IndexModel{
			Keys:    bson.D{{Key: "tenant_id", Value: 1}, {Key: "status", Value: 1}},
			Options: options.Index().SetName("tenant_status"),
		},
	)
}

// Create inserts a bid; a duplicate vendor/tender pair becomes a conflict.
func (r *Repository) Create(ctx context.Context, b *Bid) error {
	if b.ID == "" {
		b.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	b.SubmittedAt, b.UpdatedAt = now, now
	if b.Status == "" {
		b.Status = StatusSubmitted
	}

	if _, err := r.coll.InsertOne(ctx, b); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return apperr.NewConflict("vendor %s already submitted a bid for this tender", b.VendorID)
		}
		return fmt.Errorf("insert bid: %w", err)
	}
	return nil
}

// GetByID loads one bid scoped to its tenant.
func (r *Repository) GetByID(ctx context.Context, tenantID, id string) (*Bid, error) {
	var b Bid
	err := r.coll.FindOne(ctx, bson.M{"_id": id, "tenant_id": tenantID}).Decode(&b)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, apperr.ErrNotFound
		}
		return nil, fmt.Errorf("find bid: %w", err)
	}
	return &b, nil
}

// SetStatus updates one bid's status.
func (r *Repository) SetStatus(ctx context.Context, tenantID, id string, status Status) (*Bid, error) {
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var b Bid
	err := r.coll.FindOneAndUpdate(ctx,
		bson.M{"_id": id, "tenant_id": tenantID},
		bson.M{"$set": bson.M{"status": status, "updated_at": time.Now().UTC()}},
		opts).Decode(&b)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, apperr.ErrNotFound
		}
		return nil, fmt.Errorf("set bid status: %w", err)
	}
	return &b, nil
}

// RejectOthers marks every other bid of the tender as rejected once one is
// selected.
func (r *Repository) RejectOthers(ctx context.Context, tenantID, tenderID, keepBidID string) (int64, error) {
	res, err := r.coll.UpdateMany(ctx,
		bson.M{"tenant_id": tenantID, "tender_id": tenderID, "_id": bson.M{"$ne": keepBidID}, "status": bson.M{"$ne": StatusRejected}},
		bson.M{"$set": bson.M{"status": StatusRejected, "updated_at": time.Now().UTC()}})
	if err != nil {
		return 0, fmt.Errorf("reject other bids: %w", err)
	}
	return res.ModifiedCount, nil
}

// List returns a page of bids matching the filter.
func (r *Repository) List(ctx context.Context, f ListFilter) ([]Bid, int64, error) {
	filter := bson.M{"tenant_id": f.TenantID}
	if f.TenderID != "" {
		filter["tender_id"] = f.TenderID
	}
	if f.VendorID != "" {
		filter["vendor_id"] = f.VendorID
	}
	if f.Status != "" {
		filter["status"] = f.Status
	}

	total, err := r.coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("count bids: %w", err)
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "total.amount", Value: 1}, {Key: "submitted_at", Value: 1}}).
		SetSkip(httpx.Skip(f.Page, f.PerPage)).
		SetLimit(int64(f.PerPage))

	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("list bids: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()

	bids := make([]Bid, 0, f.PerPage)
	if err := cursor.All(ctx, &bids); err != nil {
		return nil, 0, fmt.Errorf("decode bids: %w", err)
	}
	return bids, total, nil
}

// ListForComparison returns every non-rejected bid of a tender, cheapest first.
func (r *Repository) ListForComparison(ctx context.Context, tenantID, tenderID string) ([]Bid, error) {
	cursor, err := r.coll.Find(ctx,
		bson.M{"tenant_id": tenantID, "tender_id": tenderID, "status": bson.M{"$ne": StatusRejected}},
		options.Find().SetSort(bson.D{{Key: "total.amount", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list bids for comparison: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()

	bids := make([]Bid, 0, 16)
	if err := cursor.All(ctx, &bids); err != nil {
		return nil, fmt.Errorf("decode bids for comparison: %w", err)
	}
	return bids, nil
}

// HasBidsForTender reports whether a tender already has bids, which the tender
// side uses before allowing a state change.
func (r *Repository) HasBidsForTender(ctx context.Context, tenantID, tenderID string) (bool, error) {
	count, err := r.coll.CountDocuments(ctx, bson.M{"tenant_id": tenantID, "tender_id": tenderID})
	if err != nil {
		return false, fmt.Errorf("count bids for tender: %w", err)
	}
	return count > 0, nil
}
