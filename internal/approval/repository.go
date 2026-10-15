package approval

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

// Repository persists approvals in MongoDB.
type Repository struct {
	coll *mongo.Collection
}

// NewRepository returns a repository bound to the approvals collection.
func NewRepository(database *mongo.Database) *Repository {
	return &Repository{coll: database.Collection("approvals")}
}

// EnsureIndexes creates the approval indexes. The unique (entity) index keeps
// one approval per document per tenant.
func (r *Repository) EnsureIndexes(ctx context.Context) error {
	return db.EnsureIndexes(ctx, r.coll,
		mongo.IndexModel{
			Keys:    bson.D{{Key: "tenant_id", Value: 1}, {Key: "entity_type", Value: 1}, {Key: "entity_id", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uniq_tenant_entity"),
		},
		mongo.IndexModel{
			Keys:    bson.D{{Key: "tenant_id", Value: 1}, {Key: "status", Value: 1}, {Key: "assigned_to", Value: 1}},
			Options: options.Index().SetName("tenant_status_assignee"),
		},
		mongo.IndexModel{
			Keys:    bson.D{{Key: "tenant_id", Value: 1}, {Key: "created_at", Value: -1}},
			Options: options.Index().SetName("tenant_created"),
		},
	)
}

// Create inserts an approval request.
func (r *Repository) Create(ctx context.Context, a *Approval) error {
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	a.CreatedAt, a.UpdatedAt = now, now
	if a.Status == "" {
		a.Status = StatusPending
	}

	if _, err := r.coll.InsertOne(ctx, a); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return apperr.NewConflict("approval for %s %s already exists", a.EntityType, a.EntityID)
		}
		return fmt.Errorf("insert approval: %w", err)
	}
	return nil
}

// GetByID loads one approval scoped to its tenant.
func (r *Repository) GetByID(ctx context.Context, tenantID, id string) (*Approval, error) {
	var a Approval
	err := r.coll.FindOne(ctx, bson.M{"_id": id, "tenant_id": tenantID}).Decode(&a)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, apperr.ErrNotFound
		}
		return nil, fmt.Errorf("find approval: %w", err)
	}
	return &a, nil
}

// FindByEntity loads the approval governing a business document.
func (r *Repository) FindByEntity(ctx context.Context, tenantID string, entityType EntityType, entityID string) (*Approval, error) {
	var a Approval
	err := r.coll.FindOne(ctx, bson.M{"tenant_id": tenantID, "entity_type": entityType, "entity_id": entityID}).Decode(&a)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, apperr.ErrNotFound
		}
		return nil, fmt.Errorf("find approval by entity: %w", err)
	}
	return &a, nil
}

// AppendDecision atomically records a decision: it only matches while the
// approval is still pending, so two approvers cannot both decide.
func (r *Repository) AppendDecision(ctx context.Context, tenantID, id string, step Step, comment string) (*Approval, error) {
	now := time.Now().UTC()

	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var a Approval
	err := r.coll.FindOneAndUpdate(ctx,
		bson.M{"_id": id, "tenant_id": tenantID, "status": StatusPending},
		bson.M{
			"$set": bson.M{
				"status":     step.Decision,
				"comment":    comment,
				"updated_at": now,
				"decided_at": now,
			},
			"$push": bson.M{"history": step},
		},
		opts).Decode(&a)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			// Either it does not exist, or it is no longer pending.
			current, getErr := r.GetByID(ctx, tenantID, id)
			if getErr != nil {
				return nil, getErr
			}
			return nil, apperr.NewConflict("approval is %s and can no longer be decided", current.Status)
		}
		return nil, fmt.Errorf("append approval decision: %w", err)
	}
	return &a, nil
}

// List returns a page of approvals matching the filter.
func (r *Repository) List(ctx context.Context, f ListFilter) ([]Approval, int64, error) {
	filter := bson.M{"tenant_id": f.TenantID}
	if f.Status != "" {
		filter["status"] = f.Status
	}
	if f.AssignedTo != "" {
		filter["assigned_to"] = f.AssignedTo
	}
	if f.EntityID != "" {
		filter["entity_id"] = f.EntityID
	}

	total, err := r.coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("count approvals: %w", err)
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip(httpx.Skip(f.Page, f.PerPage)).
		SetLimit(int64(f.PerPage))

	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("list approvals: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()

	approvals := make([]Approval, 0, f.PerPage)
	if err := cursor.All(ctx, &approvals); err != nil {
		return nil, 0, fmt.Errorf("decode approvals: %w", err)
	}
	return approvals, total, nil
}

// StatsByStatus counts approvals per status, used by the reporting service.
func (r *Repository) StatsByStatus(ctx context.Context, tenantID string) (map[string]int64, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"tenant_id": tenantID}}},
		{{Key: "$group", Value: bson.M{"_id": "$status", "count": bson.M{"$sum": 1}}}},
	}
	cursor, err := r.coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("aggregate approvals by status: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()

	var rows []struct {
		Status string `bson:"_id"`
		Count  int64  `bson:"count"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, fmt.Errorf("decode approval stats: %w", err)
	}

	stats := make(map[string]int64, len(rows))
	for _, row := range rows {
		stats[row.Status] = row.Count
	}
	return stats, nil
}
