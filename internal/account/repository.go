package account

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
)

// Repository persists users in MongoDB.
type Repository struct {
	coll *mongo.Collection
}

// NewRepository returns a repository bound to the users collection.
func NewRepository(database *mongo.Database) *Repository {
	return &Repository{coll: database.Collection("users")}
}

// EnsureIndexes creates the user indexes; the unique email per tenant is what
// makes registration races safe.
func (r *Repository) EnsureIndexes(ctx context.Context) error {
	return db.EnsureIndexes(ctx, r.coll,
		mongo.IndexModel{
			Keys:    bson.D{{Key: "tenant_id", Value: 1}, {Key: "email", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uniq_tenant_email"),
		},
		mongo.IndexModel{
			Keys:    bson.D{{Key: "tenant_id", Value: 1}, {Key: "role", Value: 1}},
			Options: options.Index().SetName("tenant_role"),
		},
	)
}

// Create inserts a user, mapping a duplicate email to a conflict error.
func (r *Repository) Create(ctx context.Context, u *User) error {
	if u.ID == "" {
		u.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	u.CreatedAt, u.UpdatedAt = now, now
	if u.Status == "" {
		u.Status = StatusActive
	}

	if _, err := r.coll.InsertOne(ctx, u); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return apperr.NewConflict("email %q is already registered for this tenant", u.Email)
		}
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

// GetByEmail loads a user by tenant and email (used by login).
func (r *Repository) GetByEmail(ctx context.Context, tenantID, email string) (*User, error) {
	var u User
	err := r.coll.FindOne(ctx, bson.M{"tenant_id": tenantID, "email": email}).Decode(&u)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, apperr.ErrNotFound
		}
		return nil, fmt.Errorf("find user by email: %w", err)
	}
	return &u, nil
}

// GetByID loads a user by id within a tenant.
func (r *Repository) GetByID(ctx context.Context, tenantID, id string) (*User, error) {
	var u User
	err := r.coll.FindOne(ctx, bson.M{"tenant_id": tenantID, "_id": id}).Decode(&u)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, apperr.ErrNotFound
		}
		return nil, fmt.Errorf("find user: %w", err)
	}
	return &u, nil
}

// TouchLastLogin records a successful login.
func (r *Repository) TouchLastLogin(ctx context.Context, tenantID, id string) error {
	now := time.Now().UTC()
	_, err := r.coll.UpdateOne(ctx,
		bson.M{"tenant_id": tenantID, "_id": id},
		bson.M{"$set": bson.M{"last_login_at": now, "updated_at": now}})
	if err != nil {
		return fmt.Errorf("update last login: %w", err)
	}
	return nil
}
