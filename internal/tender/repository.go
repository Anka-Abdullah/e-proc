package tender

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/ank/e-proc/pkg/apperr"
	"github.com/ank/e-proc/pkg/db"
	"github.com/ank/e-proc/pkg/httpx"
)

// Repository persists tenders in MongoDB.
type Repository struct {
	coll *mongo.Collection
}

// NewRepository returns a repository bound to the tenders collection.
func NewRepository(database *mongo.Database) *Repository {
	return &Repository{coll: database.Collection("tenders")}
}

// EnsureIndexes creates the tender indexes.
func (r *Repository) EnsureIndexes(ctx context.Context) error {
	return db.EnsureIndexes(ctx, r.coll,
		mongo.IndexModel{
			Keys:    bson.D{{Key: "tenant_id", Value: 1}, {Key: "code", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uniq_tenant_code"),
		},
		mongo.IndexModel{
			Keys:    bson.D{{Key: "tenant_id", Value: 1}, {Key: "status", Value: 1}, {Key: "close_at", Value: 1}},
			Options: options.Index().SetName("tenant_status_close"),
		},
		mongo.IndexModel{
			Keys:    bson.D{{Key: "tenant_id", Value: 1}, {Key: "invited_vendor_ids", Value: 1}},
			Options: options.Index().SetName("tenant_invited"),
		},
		mongo.IndexModel{
			Keys:    bson.D{{Key: "tenant_id", Value: 1}, {Key: "updated_at", Value: -1}},
			Options: options.Index().SetName("tenant_updated"),
		},
	)
}

// Create inserts a tender; duplicate codes become a conflict error.
func (r *Repository) Create(ctx context.Context, t *Tender) error {
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	t.CreatedAt, t.UpdatedAt = now, now
	if t.OpenAt.IsZero() {
		t.OpenAt = now
	}
	if t.Status == "" {
		t.Status = StatusDraft
	}

	if _, err := r.coll.InsertOne(ctx, t); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return apperr.NewConflict("tender code %q already exists for this tenant", t.Code)
		}
		return fmt.Errorf("insert tender: %w", err)
	}
	return nil
}

// GetByID loads one tender scoped to its tenant.
func (r *Repository) GetByID(ctx context.Context, tenantID, id string) (*Tender, error) {
	var t Tender
	err := r.coll.FindOne(ctx, bson.M{"_id": id, "tenant_id": tenantID}).Decode(&t)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, apperr.ErrNotFound
		}
		return nil, fmt.Errorf("find tender: %w", err)
	}
	return &t, nil
}

// SetStatus moves a tender to a new state and stamps the matching timestamp.
func (r *Repository) SetStatus(ctx context.Context, tenantID, id string, status Status) (*Tender, error) {
	now := time.Now().UTC()
	set := bson.M{"status": status, "updated_at": now}
	switch status {
	case StatusPublished:
		set["published_at"] = now
	case StatusClosed, StatusCancelled:
		set["closed_at"] = now
	}

	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var t Tender
	err := r.coll.FindOneAndUpdate(ctx, bson.M{"_id": id, "tenant_id": tenantID}, bson.M{"$set": set}, opts).Decode(&t)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, apperr.ErrNotFound
		}
		return nil, fmt.Errorf("set tender status: %w", err)
	}
	return &t, nil
}

// List returns a page of tenders plus the total matching the filter.
func (r *Repository) List(ctx context.Context, f ListFilter) ([]Tender, int64, error) {
	filter := bson.M{"tenant_id": f.TenantID}
	if f.Status != "" {
		filter["status"] = f.Status
	}
	if f.Search != "" {
		pattern := regexp.QuoteMeta(f.Search)
		filter["$or"] = []bson.M{
			{"title": bson.M{"$regex": pattern, "$options": "i"}},
			{"code": bson.M{"$regex": pattern, "$options": "i"}},
		}
	}

	total, err := r.coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("count tenders: %w", err)
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "updated_at", Value: -1}}).
		SetSkip(httpx.Skip(f.Page, f.PerPage)).
		SetLimit(int64(f.PerPage))

	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("list tenders: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()

	tenders := make([]Tender, 0, f.PerPage)
	if err := cursor.All(ctx, &tenders); err != nil {
		return nil, 0, fmt.Errorf("decode tenders: %w", err)
	}
	return tenders, total, nil
}

// ListDueForClosing returns published tenders whose deadline has passed. The
// scheduler uses it to auto-close tenders without a human action.
func (r *Repository) ListDueForClosing(ctx context.Context, now time.Time, limit int64) ([]Tender, error) {
	filter := bson.M{
		"status":   StatusPublished,
		"close_at": bson.M{"$lte": now},
	}
	opts := options.Find().SetLimit(limit).SetSort(bson.D{{Key: "close_at", Value: 1}})

	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("find tenders due for closing: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()

	due := make([]Tender, 0, limit)
	if err := cursor.All(ctx, &due); err != nil {
		return nil, fmt.Errorf("decode due tenders: %w", err)
	}
	return due, nil
}
