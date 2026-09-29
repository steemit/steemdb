package main

import (
	"context"
	"fmt"
	"log"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// discover-accounts is a ONE-OFF repair mode: it inserts stub documents
// ({_id, _dirty: true}) for accounts that were created on chain (per the
// local op stream) but never entered the account collection — the creation
// ops had no handler before PR #90, so accounts whose only activity was
// being created stayed invisible. The AccountRefresher fills the stubs with
// full snapshots on its normal schedule. Additive only (never deletes), so
// it runs without the destructive dry-run guard; -dry-run still reports
// without writing.
//
// Side report: account documents that have full fields but no creation op
// in the local op stream (possible sign of locally missing creation ops) —
// logged as a data-quality signal, never acted on.

// discoverSource is the narrow data surface discover-accounts needs.
type discoverSource interface {
	// StreamCreatedNames invokes fn for every account name created by a
	// creation op in the operations collection.
	StreamCreatedNames(ctx context.Context, fn func(name string) error) error
	// StreamAccountIDs invokes fn for every account _id, reporting whether
	// the document is an unrefreshed stub (name field absent).
	StreamAccountIDs(ctx context.Context, fn func(id string, isStub bool) error) error
	// InsertAccountStubs upserts {_id, _dirty: true} stubs, returning the
	// number of newly inserted documents.
	InsertAccountStubs(ctx context.Context, ids []string) (int64, error)
}

// discoverResult summarizes one discover-accounts run.
type discoverResult struct {
	CreatedN       int
	ExistingN      int
	Missing        int
	Inserted       int64
	AnomalyCount   int      // full docs with no creation op in the local op stream
	AnomalySamples []string // capped at 20
}

// discoverAccounts backfills missing account stubs from the local
// created-accounts set. dryRun=true reports without inserting.
func discoverAccounts(ctx context.Context, src discoverSource, batchSize int, dryRun bool, logger *log.Logger) (*discoverResult, error) {
	if batchSize <= 0 {
		batchSize = 500
	}

	// 1. Created set from the local op stream (creation ops + genesis).
	created := make(map[string]struct{}, 2_000_000)
	for _, g := range genesisAccounts {
		created[g] = struct{}{}
	}
	if err := src.StreamCreatedNames(ctx, func(name string) error {
		if name != "" {
			created[name] = struct{}{}
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to build created-accounts set: %w", err)
	}
	logger.Printf("Created-accounts set built: %d names (creation ops + genesis)", len(created))

	// 2. Existing set from the account collection; collect anomalies
	// (full docs without a creation op) as a data-quality side report.
	existing := make(map[string]struct{}, 2_000_000)
	res := &discoverResult{CreatedN: len(created)}
	err := src.StreamAccountIDs(ctx, func(id string, isStub bool) error {
		existing[id] = struct{}{}
		res.ExistingN++
		if _, ok := created[id]; !ok && !isStub {
			res.AnomalyCount++
			if len(res.AnomalySamples) < 20 {
				res.AnomalySamples = append(res.AnomalySamples, id)
			}
		}
		if res.ExistingN%100000 == 0 {
			logger.Printf("Progress: scanned %d account ids", res.ExistingN)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed during account scan: %w", err)
	}

	// 3. Diff and insert missing stubs in batches.
	batch := make([]string, 0, batchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if !dryRun {
			n, err := src.InsertAccountStubs(ctx, batch)
			if err != nil {
				return fmt.Errorf("failed to insert stubs: %w", err)
			}
			res.Inserted += n
		}
		batch = batch[:0]
		return nil
	}
	for name := range created {
		if _, ok := existing[name]; !ok {
			res.Missing++
			batch = append(batch, name)
			if len(batch) >= batchSize {
				if err := flush(); err != nil {
					return nil, err
				}
			}
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return res, nil
}

// --- mongoDiscoverSource: discoverSource backed by MongoDB ---

type mongoDiscoverSource struct {
	db *mongo.Database
}

func newMongoDiscoverSource(db *mongo.Database) *mongoDiscoverSource {
	return &mongoDiscoverSource{db: db}
}

// StreamCreatedNames reuses the verify-accounts implementation (same scan).
func (s *mongoDiscoverSource) StreamCreatedNames(ctx context.Context, fn func(name string) error) error {
	return newMongoVerifySource(s.db).StreamCreatedNames(ctx, fn)
}

// StreamAccountIDs implements discoverSource.
func (s *mongoDiscoverSource) StreamAccountIDs(ctx context.Context, fn func(id string, isStub bool) error) error {
	cur, err := s.db.Collection("account").Find(ctx, bson.M{},
		options.Find().SetProjection(bson.M{"name": 1}).SetNoCursorTimeout(true))
	if err != nil {
		return err
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var doc struct {
			ID   string  `bson:"_id"`
			Name *string `bson:"name"`
		}
		if err := cur.Decode(&doc); err != nil {
			return err
		}
		if err := fn(doc.ID, doc.Name == nil); err != nil {
			return err
		}
	}
	return cur.Err()
}

// InsertAccountStubs implements discoverSource. Unordered bulk upsert;
// already-existing documents are only re-marked dirty (idempotent).
func (s *mongoDiscoverSource) InsertAccountStubs(ctx context.Context, ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	models := make([]mongo.WriteModel, 0, len(ids))
	for _, id := range ids {
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": id}).
			SetUpdate(bson.M{"$set": bson.M{"_dirty": true}}).
			SetUpsert(true))
	}
	res, err := s.db.Collection("account").BulkWrite(ctx, models, options.BulkWrite().SetOrdered(false))
	if err != nil {
		return 0, err
	}
	return res.UpsertedCount, nil
}
