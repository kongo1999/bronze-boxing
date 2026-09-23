package db

import (
	"context"
	"errors"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
)

// Store wraps the Mongo client and the app database. Handlers receive *Store
// and pull collections via Coll(). One client, pooled, lives for the process.
type Store struct {
	Client *mongo.Client
	DB     *mongo.Database
	// SupportsTx is true when the server is a replica-set member (or mongos),
	// the only deployments that accept multi-document transactions.
	SupportsTx bool
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
	s := &Store{Client: client, DB: client.Database(dbName)}
	s.SupportsTx = detectTx(ctx, client)
	return s, nil
}

// detectTx asks the server whether it is part of a replica set. A standalone
// mongod answers without setName and rejects transactions outright.
func detectTx(ctx context.Context, client *mongo.Client) bool {
	var hello struct {
		SetName string `bson:"setName"`
		Msg     string `bson:"msg"` // "isdbgrid" on mongos
	}
	if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
		return false
	}
	return hello.SetName != "" || hello.Msg == "isdbgrid"
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

// ErrNoTx is returned by WithTx when transactions are unavailable and the
// store was not explicitly allowed to run degraded.
var ErrNoTx = errors.New("mongo: transactions unavailable (server is not a replica set)")

// AllowDegraded lets WithTx run callbacks without a transaction on a
// standalone server. Only for emergencies (MONGO_ALLOW_STANDALONE=true): the
// individual writes keep their conditional guards, but a failure halfway
// through a multi-document change is no longer rolled back.
var AllowDegraded bool

var warnedDegraded bool

// WithTx runs fn inside a multi-document transaction: every read and write
// made with the ctx fn receives commits together or not at all. Transient
// conflicts (e.g. two requests updating the same balance document) are
// retried by the driver, so fn must only touch the database through its ctx
// and must be safe to run more than once.
func (s *Store) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if !s.SupportsTx {
		if !AllowDegraded {
			return ErrNoTx
		}
		if !warnedDegraded {
			warnedDegraded = true
			log.Println("WARNING: running multi-document writes WITHOUT transactions (standalone MongoDB)")
		}
		return fn(ctx)
	}
	sess, err := s.Client.StartSession()
	if err != nil {
		return err
	}
	defer sess.EndSession(context.Background())
	txOpts := options.Transaction().
		SetReadConcern(readconcern.Snapshot()).
		SetWriteConcern(writeconcern.Majority())
	_, err = sess.WithTransaction(ctx, func(sc mongo.SessionContext) (any, error) {
		return nil, fn(sc)
	}, txOpts)
	return err
}
