// Package db wraps the MongoDB driver with the few helpers services need:
// a pooled connection, a database handle and index management.
package db

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// Connect dials MongoDB and verifies the deployment is reachable.
func Connect(ctx context.Context, uri, database string) (*mongo.Client, *mongo.Database, error) {
	opts := options.Client().
		ApplyURI(uri).
		SetConnectTimeout(10 * time.Second).
		SetServerSelectionTimeout(10 * time.Second).
		SetMaxPoolSize(50)

	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, nil, fmt.Errorf("mongo connect: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		_ = client.Disconnect(ctx)
		return nil, nil, fmt.Errorf("mongo ping: %w", err)
	}

	return client, client.Database(database), nil
}

// EnsureIndexes creates the given indexes if they do not exist yet. It is safe
// to call on every boot: identical index specs are a no-op in MongoDB.
func EnsureIndexes(ctx context.Context, coll *mongo.Collection, models ...mongo.IndexModel) error {
	if len(models) == 0 {
		return nil
	}
	if _, err := coll.Indexes().CreateMany(ctx, models); err != nil {
		return fmt.Errorf("create indexes on %s: %w", coll.Name(), err)
	}
	return nil
}
