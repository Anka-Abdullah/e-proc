package audit

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

// Repository stores audit entries. It deliberately exposes no update or delete
// method: the log is append-only by construction, not by convention.
type Repository struct {
	coll *mongo.Collection
}

// NewRepository returns a repository bound to the audit_logs collection.
func NewRepository(database *mongo.Database) *Repository {
	return &Repository{coll: database.Collection("audit_logs")}
}

// EnsureIndexes creates the audit query indexes.
func (r *Repository) EnsureIndexes(ctx context.Context) error {
	return db.EnsureIndexes(ctx, r.coll,
		mongo.IndexModel{
			Keys:    bson.D{{Key: "tenant_id", Value: 1}, {Key: "created_at", Value: -1}},
			Options: options.Index().SetName("tenant_created"),
		},
		mongo.IndexModel{
			Keys:    bson.D{{Key: "entity_type", Value: 1}, {Key: "entity_id", Value: 1}, {Key: "created_at", Value: -1}},
			Options: options.Index().SetName("entity_created"),
		},
		mongo.IndexModel{
			Keys:    bson.D{{Key: "actor_id", Value: 1}, {Key: "created_at", Value: -1}},
			Options: options.Index().SetName("actor_created"),
		},
		mongo.IndexModel{
			Keys:    bson.D{{Key: "event_id", Value: 1}},
			Options: options.Index().SetName("uniq_event").SetUnique(true),
		},
	)
}

// Insert appends one audit entry. The unique event_id index makes a redelivered
// event a no-op rather than a duplicate record.
func (r *Repository) Insert(ctx context.Context, e *Entry) error {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}

	if _, err := r.coll.InsertOne(ctx, e); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return apperr.NewConflict("audit entry for event %s already exists", e.EventID)
		}
		return fmt.Errorf("insert audit entry: %w", err)
	}
	return nil
}

// GetByID loads one entry scoped to its tenant.
func (r *Repository) GetByID(ctx context.Context, tenantID, id string) (*Entry, error) {
	var e Entry
	err := r.coll.FindOne(ctx, bson.M{"_id": id, "tenant_id": tenantID}).Decode(&e)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, apperr.ErrNotFound
		}
		return nil, fmt.Errorf("find audit entry: %w", err)
	}
	return &e, nil
}

// List returns a page of audit entries matching the filter.
func (r *Repository) List(ctx context.Context, f Filter) ([]Entry, int64, error) {
	filter := bson.M{"tenant_id": f.TenantID}
	if f.ActorID != "" {
		filter["actor_id"] = f.ActorID
	}
	if f.EntityType != "" {
		filter["entity_type"] = f.EntityType
	}
	if f.EntityID != "" {
		filter["entity_id"] = f.EntityID
	}
	if f.Action != "" {
		filter["action"] = f.Action
	}
	if f.From != nil || f.To != nil {
		window := bson.M{}
		if f.From != nil {
			window["$gte"] = *f.From
		}
		if f.To != nil {
			window["$lte"] = *f.To
		}
		filter["created_at"] = window
	}

	total, err := r.coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("count audit entries: %w", err)
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip(httpx.Skip(f.Page, f.PerPage)).
		SetLimit(int64(f.PerPage))

	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("list audit entries: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()

	entries := make([]Entry, 0, f.PerPage)
	if err := cursor.All(ctx, &entries); err != nil {
		return nil, 0, fmt.Errorf("decode audit entries: %w", err)
	}
	return entries, total, nil
}

// CountByAction returns how many entries exist per action, used by reports.
func (r *Repository) CountByAction(ctx context.Context, tenantID string, since time.Time) (map[string]int64, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"tenant_id": tenantID, "created_at": bson.M{"$gte": since}}}},
		{{Key: "$group", Value: bson.M{"_id": "$action", "count": bson.M{"$sum": 1}}}},
	}
	cursor, err := r.coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("aggregate audit actions: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()

	var rows []struct {
		Action string `bson:"_id"`
		Count  int64  `bson:"count"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, fmt.Errorf("decode audit action counts: %w", err)
	}

	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.Action] = row.Count
	}
	return counts, nil
}
