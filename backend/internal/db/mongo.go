package db

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// Store wraps the Mongo client and the app database. Handlers receive *Store
// and pull collections via Coll(). One client, pooled, lives for the process.
type Store struct {
	Client *mongo.Client
	DB     *mongo.Database
}

func Connect(uri, dbName string) (*Store, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := options.Client().
		ApplyURI(uri).
		SetServerSelectionTimeout(10 * time.Second).
		SetMaxPoolSize(10)

	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return nil, err
	}
	return &Store{Client: client, DB: client.Database(dbName)}, nil
}

// Coll returns a named collection from the app database.
func (s *Store) Coll(name string) *mongo.Collection {
	return s.DB.Collection(name)
}

// Ping checks connectivity (used by /api/health).
func (s *Store) Ping(ctx context.Context) error {
	return s.Client.Ping(ctx, readpref.Primary())
}

func (s *Store) Disconnect(ctx context.Context) error {
	return s.Client.Disconnect(ctx)
}
