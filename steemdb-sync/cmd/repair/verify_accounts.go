package main

import (
	"context"
	"fmt"
	"log"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/steemit/steemdb-sync/internal/processor/handlers"
)

// verify-accounts is a ONE-OFF repair mode: it deletes account documents
// whose names were never created on chain (phantom stubs queued from
// user-controlled custom_json payloads before the account-doc-creation rule
// was enforced — docs/rules/account-doc-creation.md). After PR #90 the
// processor cannot create new phantoms, so once this cleanup has run and the
// phantom-account-stubs health check is green, this mode is not expected to
// be needed again on that database.
//
// Verification is fully LOCAL (no RPC): an account exists on chain iff it
// was created by one of the creation ops (account_create,
// account_create_with_delegation, create_claimed_account, pow/pow2 mining)
// or is a genesis account — and the operations collection holds the complete
// op stream. Steem accounts are never deleted, so the created set equals the
// existing set exactly.
//
// Loop shape: the outer loop streams account _ids in alphabetical order up
// to a landmark (the last _id snapshot taken at startup, so accounts created
// while this runs are left for a future pass); the inner loop processes
// batchSize ids (default 500, -batch flag).

// genesisAccounts are created by init_genesis with no op (mainnet).
// discover-accounts seeds the same set.
var genesisAccounts = []string{"miners", "null", "temp", "initminer"}

// verifySource is the narrow data surface verify-accounts needs, so the
// logic can be unit tested without MongoDB.
type verifySource interface {
	// StreamCreatedNames invokes fn for every account name created by a
	// creation op in the operations collection.
	StreamCreatedNames(ctx context.Context, fn func(name string) error) error
	// LastAccountID returns the alphabetically last account _id (startup
	// landmark).
	LastAccountID(ctx context.Context) (string, error)
	// StreamAccountIDs invokes fn for every account _id <= landmark in
	// ascending order.
	StreamAccountIDs(ctx context.Context, landmark string, fn func(id string) error) error
	// DeleteAccounts deletes account documents by _id, returning the count.
	DeleteAccounts(ctx context.Context, ids []string) (int64, error)
}

// verifyResult summarizes one verify-accounts run.
type verifyResult struct {
	Checked  int
	Phantom  int
	Deleted  int64
	Samples  []string
	CreatedN int
}

// verifyAccounts runs the cleanup. dryRun=true reports without deleting.
func verifyAccounts(ctx context.Context, src verifySource, batchSize int, dryRun bool, logger *log.Logger) (*verifyResult, error) {
	if batchSize <= 0 {
		batchSize = 500
	}

	// 1. Build the created-accounts set from the local op stream.
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

	// 2. Startup landmark: the alphabetically last account _id.
	landmark, err := src.LastAccountID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read startup landmark: %w", err)
	}
	logger.Printf("Startup landmark (last account _id): %q", landmark)

	// 3. Outer loop: stream ids alphabetically; inner loop: batches.
	res := &verifyResult{CreatedN: len(created)}
	batch := make([]string, 0, batchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if !dryRun {
			n, err := src.DeleteAccounts(ctx, batch)
			if err != nil {
				return fmt.Errorf("failed to delete batch: %w", err)
			}
			res.Deleted += n
		}
		batch = batch[:0]
		return nil
	}

	err = src.StreamAccountIDs(ctx, landmark, func(id string) error {
		res.Checked++
		if _, ok := created[id]; !ok {
			res.Phantom++
			if len(res.Samples) < 20 {
				res.Samples = append(res.Samples, id)
			}
			batch = append(batch, id)
			if len(batch) >= batchSize {
				return flush()
			}
		}
		if res.Checked%50000 == 0 {
			logger.Printf("Progress: checked %d account ids, phantom %d", res.Checked, res.Phantom)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed during account scan: %w", err)
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return res, nil
}

// --- mongoVerifySource: verifySource backed by MongoDB ---

type mongoVerifySource struct {
	db *mongo.Database
}

func newMongoVerifySource(db *mongo.Database) *mongoVerifySource {
	return &mongoVerifySource{db: db}
}

// StreamCreatedNames implements verifySource. Creation-op names come from
// new_account_name; mined accounts come from the pow/pow2 worker field
// (extracted with the processor's own parser so both op shapes are covered).
func (s *mongoVerifySource) StreamCreatedNames(ctx context.Context, fn func(name string) error) error {
	ops := s.db.Collection("operations")

	creationTypes := []string{"account_create", "account_create_with_delegation", "create_claimed_account"}
	cur, err := ops.Find(ctx,
		bson.M{"op_type": bson.M{"$in": creationTypes}},
		options.Find().SetProjection(bson.M{"op_value.new_account_name": 1}).SetNoCursorTimeout(true))
	if err != nil {
		return err
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var doc struct {
			OpValue struct {
				Name string `bson:"new_account_name"`
			} `bson:"op_value"`
		}
		if err := cur.Decode(&doc); err != nil {
			return err
		}
		if err := fn(doc.OpValue.Name); err != nil {
			return err
		}
	}
	if err := cur.Err(); err != nil {
		return err
	}

	cur2, err := ops.Find(ctx,
		bson.M{"op_type": bson.M{"$in": []string{"pow", "pow2"}}},
		options.Find().SetProjection(bson.M{"op_value": 1}).SetNoCursorTimeout(true))
	if err != nil {
		return err
	}
	defer cur2.Close(ctx)
	for cur2.Next(ctx) {
		var doc struct {
			OpValue map[string]interface{} `bson:"op_value"`
		}
		if err := cur2.Decode(&doc); err != nil {
			return err
		}
		if err := fn(handlers.ExtractWorkerAccount(doc.OpValue)); err != nil {
			return err
		}
	}
	return cur2.Err()
}

// LastAccountID implements verifySource.
func (s *mongoVerifySource) LastAccountID(ctx context.Context) (string, error) {
	var doc struct {
		ID string `bson:"_id"`
	}
	err := s.db.Collection("account").FindOne(ctx, bson.M{},
		options.FindOne().SetSort(bson.M{"_id": -1}).SetProjection(bson.M{"_id": 1})).Decode(&doc)
	if err != nil {
		return "", err
	}
	return doc.ID, nil
}

// StreamAccountIDs implements verifySource.
func (s *mongoVerifySource) StreamAccountIDs(ctx context.Context, landmark string, fn func(id string) error) error {
	cur, err := s.db.Collection("account").Find(ctx,
		bson.M{"_id": bson.M{"$lte": landmark}},
		options.Find().SetSort(bson.M{"_id": 1}).SetProjection(bson.M{"_id": 1}).SetNoCursorTimeout(true))
	if err != nil {
		return err
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var doc struct {
			ID string `bson:"_id"`
		}
		if err := cur.Decode(&doc); err != nil {
			return err
		}
		if err := fn(doc.ID); err != nil {
			return err
		}
	}
	return cur.Err()
}

// DeleteAccounts implements verifySource.
func (s *mongoVerifySource) DeleteAccounts(ctx context.Context, ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res, err := s.db.Collection("account").DeleteMany(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}
