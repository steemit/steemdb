package mongo

import (
	"context"
	"log"
	"time"

	"github.com/pkg/errors"
	"github.com/steemit/steemdb-sync/internal/config"
	"github.com/steemit/steemdb-sync/internal/metrics"
	"github.com/steemit/steemdb-sync/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Client wraps MongoDB client and collections
type Client struct {
	client       *mongo.Client
	db           *mongo.Database
	blocks       *mongo.Collection
	transactions *mongo.Collection
	operations   *mongo.Collection
	meta         *mongo.Collection
}

// NewClient creates a new MongoDB client
func NewClient(cfg *config.Config) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOptions := options.Client().
		ApplyURI(cfg.Mongo.URI).
		SetMinPoolSize(uint64(cfg.Mongo.MinPoolSize)).
		SetMaxPoolSize(uint64(cfg.Mongo.MaxPoolSize))

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to MongoDB")
	}

	// Ping to verify connection
	if err := client.Ping(ctx, nil); err != nil {
		return nil, errors.Wrap(err, "failed to ping MongoDB")
	}

	db := client.Database(cfg.Mongo.Database)

	c := &Client{
		client:       client,
		db:           db,
		blocks:       db.Collection("blocks"),
		transactions: db.Collection("transactions"),
		operations:   db.Collection("operations"),
		meta:         db.Collection("meta"),
	}

	// Create indexes. Index builds on production-size collections take
	// minutes-to-hours, so they must not inherit the 10s connect deadline:
	// with it, the first startup that has to backfill a new index on a large
	// database times out and prevents the service from starting at all.
	// Connection liveness was already verified by the Ping above.
	if err := c.createIndexes(context.Background()); err != nil {
		return nil, errors.Wrap(err, "failed to create indexes")
	}

	return c, nil
}

// createIndexes ensures every index in the inventory exists. Idempotent:
// ensuring an existing index is a cheap catalog lookup. It blocks until all
// builds finish (see the call site for why it runs without a deadline).
func (c *Client) createIndexes(ctx context.Context) error {
	specs := indexInventory()
	log.Printf("[mongo] Ensuring %d indexes (single index authority, docs/AI-driver/02-data-model.md)", len(specs))
	for _, s := range specs {
		if _, err := c.db.Collection(s.collection).Indexes().CreateOne(ctx, s.model); err != nil {
			return errors.Wrapf(err, "failed to create index on %s (%v)", s.collection, s.model.Keys)
		}
	}
	return c.dropLegacyWebIndexes(ctx)
}

// legacyWebIndexes lists index names steemdb-web's former startup builder
// created before index authority was consolidated into sync. They are either
// phantoms on fields no writer writes (vote/transfer `timestamp` — the real
// field is `_ts`; account `last_update` — the refresher writes `scanned`) or
// serve no reader in the current codebase (see docs/AI-driver/02-data-model.md
// "Index authority"). Sync, as the authority, removes them once at startup;
// index drops are online and lossless (indexes are derivable state).
var legacyWebIndexes = map[string][]string{
	"vote":     {"voter_1", "timestamp_-1", "author_1_permlink_1"},
	"transfer": {"from_1", "to_1", "timestamp_-1"},
	"account":  {"last_update_-1"},
	"blocks":   {"timestamp_-1", "witness_1"},
	"comment":  {"author_1_permlink_1"},
}

// dropLegacyWebIndexes removes the legacy web-built indexes above when (and
// only when) they are present, so existing deployments converge on the
// inventory. Absent names are skipped via a catalog listing, making the
// step a cheap no-op after the first successful startup.
func (c *Client) dropLegacyWebIndexes(ctx context.Context) error {
	for coll, names := range legacyWebIndexes {
		cursor, err := c.db.Collection(coll).Indexes().List(ctx)
		if err != nil {
			return errors.Wrapf(err, "failed to list %s indexes", coll)
		}
		var existing []struct {
			Name string `bson:"name"`
		}
		if err := cursor.All(ctx, &existing); err != nil {
			return errors.Wrapf(err, "failed to decode %s index list", coll)
		}
		present := make(map[string]bool, len(existing))
		for _, e := range existing {
			present[e.Name] = true
		}
		for _, name := range names {
			if !present[name] {
				continue
			}
			if _, err := c.db.Collection(coll).Indexes().DropOne(ctx, name); err != nil {
				return errors.Wrapf(err, "failed to drop legacy %s index %s", coll, name)
			}
			log.Printf("[mongo] Dropped legacy web-built index %s.%s", coll, name)
		}
	}
	return nil
}

// indexSpec is one entry of the database index inventory.
type indexSpec struct {
	collection string
	model      mongo.IndexModel
}

// indexInventory returns every secondary index of the steemdb database.
//
// steemdb-sync is the single index authority (docs/AI-driver/02-data-model.md
// "Index authority"): it owns the collections, so it owns their indexes, and
// steemdb-web creates none. Every entry exists to serve either
//
//   - a sync write path: Pattern-B collections upsert by a business-field
//     filter, and that filter must be index-led or every upsert is a
//     collection scan (the benefactor_reward COLLSCAN incident), or
//   - a concrete read query in steemdb-web / steemdb-sync, cited in the
//     entry's comment.
//
// When adding an entry: verify the query's exact filter/sort fields against
// the code, cite the reader in the comment, and extend TestIndexInventory.
func indexInventory() []indexSpec {
	return []indexSpec{
		// ---- Raw layer ----

		{
			// Per-day chart aggregations match blocks by timestamp range
			// (web charts_service.go GetBlockProduction/
			// GetTransactionVolume, dashboard_service.go funds chart).
			collection: "blocks",
			model:      mongo.IndexModel{Keys: bson.D{{Key: "timestamp", Value: 1}}},
		},
		{
			// Block-scoped transaction lookups (transactions is written by
			// live_sync only).
			collection: "transactions",
			model:      mongo.IndexModel{Keys: bson.D{{Key: "block_num", Value: 1}}},
		},
		{
			// sync refresher/stats.go and web block ops: operations of one
			// block (block detail, per-day op counting).
			collection: "operations",
			model:      mongo.IndexModel{Keys: bson.D{{Key: "block_num", Value: 1}}},
		},
		{
			// Transaction-scoped operation lookups (web block detail).
			collection: "operations",
			model:      mongo.IndexModel{Keys: bson.D{{Key: "trx_id", Value: 1}}},
		},
		{
			// Web stats: counting/grouping operations by type.
			collection: "operations",
			model:      mongo.IndexModel{Keys: bson.D{{Key: "op_type", Value: 1}}},
		},
		{
			// Web stats: splitting virtual vs user operations.
			collection: "operations",
			model:      mongo.IndexModel{Keys: bson.D{{Key: "virtual", Value: 1}}},
		},
		{
			// Serves per-account history queries: equality on the accounts
			// multikey field + descending sort on block_num (monotonic in time).
			collection: "operations",
			model: mongo.IndexModel{
				Keys: bson.D{{Key: "accounts", Value: 1}, {Key: "block_num", Value: -1}},
			},
		},

		// ---- Pattern-B write-path indexes ----
		// These collections are written by multi-field filter upserts
		// (legacy sync.py Pattern B: dedup by business fields). Without an
		// index on the filter fields every upsert is a collection scan — the
		// processor ground at minutes-per-thousand-blocks until these were
		// added.

		{
			collection: "follow",
			model: mongo.IndexModel{Keys: bson.D{
				{Key: "_block", Value: 1}, {Key: "follower", Value: 1}, {Key: "following", Value: 1},
			}},
		},
		{
			collection: "reblog",
			model: mongo.IndexModel{Keys: bson.D{
				{Key: "_block", Value: 1}, {Key: "permlink", Value: 1}, {Key: "account", Value: 1},
			}},
		},
		{
			collection: "witness_vote",
			model: mongo.IndexModel{Keys: bson.D{
				{Key: "_ts", Value: 1}, {Key: "account", Value: 1}, {Key: "witness", Value: 1},
			}},
		},
		{
			// benefactor_reward needs both indexes: the upsert filter is
			// {_block, benefactor, permlink, author} (see BenefactorRewardHandler),
			// while the _ts-led one only serves the web-layer $sort on _ts
			// (LabsService.GetBenefactors). Without the _block-led index every
			// upsert is a collection scan.
			collection: "benefactor_reward",
			model: mongo.IndexModel{Keys: bson.D{
				{Key: "_ts", Value: 1}, {Key: "benefactor", Value: 1},
				{Key: "permlink", Value: 1}, {Key: "author", Value: 1},
			}},
		},
		{
			collection: "benefactor_reward",
			model: mongo.IndexModel{Keys: bson.D{
				{Key: "_block", Value: 1}, {Key: "benefactor", Value: 1},
				{Key: "permlink", Value: 1}, {Key: "author", Value: 1},
			}},
		},

		// ---- Read-side indexes (labs, posts, rescanner) ----
		// Derived-layer documents carry the event time in _ts (written by
		// handlers/helpers.go), never in `timestamp`; every labs time-range
		// aggregation below matches on _ts.

		{
			// web labs_service.go GetPowerUps: $match _ts >= now-30d.
			collection: "vesting_deposit",
			model:      mongo.IndexModel{Keys: bson.D{{Key: "_ts", Value: 1}}},
		},
		{
			// web labs_service.go GetPowerDowns: previous-days and top-user
			// pipelines match _ts >= now-7d / 30d windows.
			collection: "vesting_withdraw",
			model:      mongo.IndexModel{Keys: bson.D{{Key: "_ts", Value: 1}}},
		},
		{
			// web labs_service.go GetCurationLeaderboard: $match _ts in
			// [day|month start, end).
			collection: "curation_reward",
			model:      mongo.IndexModel{Keys: bson.D{{Key: "_ts", Value: 1}}},
		},
		{
			// web labs_service.go GetAuthorLeaderboard: $match _ts in
			// [day|month start, end).
			collection: "author_reward",
			model:      mongo.IndexModel{Keys: bson.D{{Key: "_ts", Value: 1}}},
		},
		{
			// web comment_service.go GetPostVotes: equality {author, permlink}
			// + sort _ts asc.
			collection: "vote",
			model: mongo.IndexModel{Keys: bson.D{
				{Key: "author", Value: 1}, {Key: "permlink", Value: 1}, {Key: "_ts", Value: 1},
			}},
		},
		{
			// web labs_service.go GetFlags: $match weight < 0 over the whole
			// vote collection. Partial on weight < 0: negative votes (flags)
			// are a tiny minority of all votes, a full {weight: 1} over every
			// vote ever cast would be pure write amplification for this single
			// query, and the query predicate is identical to the filter
			// expression so the planner can use the index.
			collection: "vote",
			model: mongo.IndexModel{
				Keys:    bson.D{{Key: "weight", Value: 1}},
				Options: options.Index().SetPartialFilterExpression(bson.M{"weight": bson.M{"$lt": 0}}),
			},
		},
		{
			// web comment_service.go GetPostReblogs: equality {author, permlink}
			// + sort _ts asc (author is preserved from the reblog custom_json
			// payload by CustomJSONHandler.handleReblog).
			collection: "reblog",
			model: mongo.IndexModel{Keys: bson.D{
				{Key: "author", Value: 1}, {Key: "permlink", Value: 1}, {Key: "_ts", Value: 1},
			}},
		},
		{
			// Labs $lookup joins on account.name (GetPowerUps/GetPowerDowns/
			// GetCurationLeaderboard/GetAuthorLeaderboard/GetRsharesAllocation,
			// foreignField "name") and web search_service.go matches a
			// name-prefix regex. `name` exists only on refreshed documents —
			// that is the same population the joins and the search target.
			collection: "account",
			model:      mongo.IndexModel{Keys: bson.D{{Key: "name", Value: 1}}},
		},
		{
			// web account_service.go GetAccounts: default sort reputation desc.
			collection: "account",
			model:      mongo.IndexModel{Keys: bson.D{{Key: "reputation", Value: -1}}},
		},
		{
			// web account_service.go GetAccounts: sort_by=vesting_shares
			// passthrough (accounts page vests column).
			collection: "account",
			model:      mongo.IndexModel{Keys: bson.D{{Key: "vesting_shares", Value: -1}}},
		},
		{
			// web labs_service.go GetPowerDowns upcoming-withdrawals pipeline:
			// $match next_vesting_withdrawal >= today (+ vesting_withdraw_rate
			// residual). The field is parsed to a date by
			// account_refresher.processAccount dateFields.
			collection: "account",
			model:      mongo.IndexModel{Keys: bson.D{{Key: "next_vesting_withdrawal", Value: 1}}},
		},
		{
			// sync refresher/witness.go DeleteMany {owner: $nin} stale-witness
			// cleanup + web witness_service.go GetWitnesses sort by owner.
			collection: "witness",
			model:      mongo.IndexModel{Keys: bson.D{{Key: "owner", Value: 1}}},
		},
		{
			// web witness_service.go GetWitnesses/GetTopWitnesses: default
			// sort votes desc (votes stored as float by the refresher).
			collection: "witness",
			model:      mongo.IndexModel{Keys: bson.D{{Key: "votes", Value: -1}}},
		},
		{
			// web witness_service.go GetWitnesses: sort by total_misses.
			collection: "witness",
			model:      mongo.IndexModel{Keys: bson.D{{Key: "total_missed", Value: 1}}},
		},

		// comment is the largest collection (posts + comments). Each index
		// below serves a specific hot path; think twice before adding more —
		// every comment op rewrites `scanned` and updates all of these.

		{
			// web comment_service.go GetPosts (/api/v1/posts): equality
			// depth=0 for the total count + default created desc sort; also
			// GetPostsByDate {depth: 0, created range} and comment_rescanner
			// queue3 {depth: $exists: false} (missing values live under the
			// index' null key).
			collection: "comment",
			model: mongo.IndexModel{Keys: bson.D{
				{Key: "depth", Value: 1}, {Key: "created", Value: -1},
			}},
		},
		{
			// web comment_service.go GetPostReplies: equality
			// {parent_author, parent_permlink} + sort created desc.
			collection: "comment",
			model: mongo.IndexModel{Keys: bson.D{
				{Key: "parent_author", Value: 1},
				{Key: "parent_permlink", Value: 1},
				{Key: "created", Value: -1},
			}},
		},
		{
			// sync comment_rescanner queue1: {created > window_days ago,
			// scanned < stale_hours ago} with sort {scanned: 1} + limit.
			// `scanned` leads so the sort is index-ordered; `created` prunes
			// the scan. Note the comment handler rewrites `scanned` on every
			// comment op (handlers/comment.go) — this index churns by design.
			collection: "comment",
			model: mongo.IndexModel{Keys: bson.D{
				{Key: "scanned", Value: 1}, {Key: "created", Value: 1},
			}},
		},
		{
			// sync comment_rescanner queue2: {depth: 0,
			// pending_payout_value > 0, cashout_time < now,
			// mode $in [first_payout, second_payout]}. depth + pending bound
			// the scan to posts still awaiting payout; cashout_time and mode
			// are residuals.
			collection: "comment",
			model: mongo.IndexModel{Keys: bson.D{
				{Key: "depth", Value: 1}, {Key: "pending_payout_value", Value: 1},
			}},
		},
		{
			// web comment_service.go GetPostsByDate: optional category
			// equality filter (tag pages).
			collection: "comment",
			model:      mongo.IndexModel{Keys: bson.D{{Key: "category", Value: 1}}},
		},
		{
			// web labs_service.go GetRsharesAllocation/GetPendingPosts:
			// created-range matches; sync refresher/clients.go 90-day window;
			// comment_rescanner queue1 created bound.
			collection: "comment",
			model:      mongo.IndexModel{Keys: bson.D{{Key: "created", Value: -1}}},
		},
		{
			// web comment_service.go GetPosts sort_by=net_votes
			// (posts page votes column).
			collection: "comment",
			model:      mongo.IndexModel{Keys: bson.D{{Key: "net_votes", Value: -1}}},
		},
		{
			// web comment_service.go GetPosts sort_by=pending_payout_value
			// and labs_service.go GetPendingPosts sort pending desc.
			collection: "comment",
			model:      mongo.IndexModel{Keys: bson.D{{Key: "pending_payout_value", Value: -1}}},
		},
		{
			// web comment_service.go GetPosts sort_by=total_payout_value
			// (posts page payout column).
			collection: "comment",
			model:      mongo.IndexModel{Keys: bson.D{{Key: "total_payout_value", Value: -1}}},
		},
	}
}

// Close closes the MongoDB connection
func (c *Client) Close(ctx context.Context) error {
	return c.client.Disconnect(ctx)
}

// BulkUpsertOperations performs bulk upsert of operations
func (c *Client) BulkUpsertOperations(ctx context.Context, ops []*model.Operation) error {
	if len(ops) == 0 {
		return nil
	}

	startTime := time.Now()
	models := make([]mongo.WriteModel, 0, len(ops))
	for _, op := range ops {
		// Derive involved accounts at the single write choke point so every
		// source (batcher, repair, live_sync) populates the field.
		op.Accounts = model.ExtractAccounts(op.OpValue)
		filter := bson.M{"_id": op.ID}
		update := bson.M{"$set": op}
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(filter).
			SetUpdate(update).
			SetUpsert(true))
	}

	opts := options.BulkWrite().SetOrdered(false)
	_, err := c.operations.BulkWrite(ctx, models, opts)
	duration := time.Since(startTime)

	// Record metrics
	metrics.RecordMongoWrite("operations", "bulk_upsert", duration, err)

	if err != nil {
		return errors.Wrap(err, "failed to bulk upsert operations")
	}

	return nil
}

// BackfillOperationAccounts scans operations that lack the accounts field,
// derives involved accounts, and writes them back in batches. It is the
// one-time migration path for data ingested before the field existed; newly
// ingested operations are populated automatically by BulkUpsertOperations.
// Already-backfilled documents no longer match the scan filter, so the loop
// terminates without a separate cursor bookmark.
func (c *Client) BackfillOperationAccounts(ctx context.Context, batchSize int) (int64, error) {
	if batchSize <= 0 {
		batchSize = 1000
	}

	var total int64
	for {
		findOptions := options.Find().
			SetLimit(int64(batchSize)).
			SetProjection(bson.M{"op_type": 1, "op_value": 1})

		cursor, err := c.operations.Find(ctx, bson.M{"accounts": bson.M{"$exists": false}}, findOptions)
		if err != nil {
			return total, errors.Wrap(err, "failed to find operations without accounts")
		}

		type opRef struct {
			ID      string                 `bson:"_id"`
			OpValue map[string]interface{} `bson:"op_value"`
		}
		var batch []opRef
		if err := cursor.All(ctx, &batch); err != nil {
			cursor.Close(ctx)
			return total, errors.Wrap(err, "failed to decode operations without accounts")
		}
		cursor.Close(ctx)

		if len(batch) == 0 {
			return total, nil
		}

		models := make([]mongo.WriteModel, 0, len(batch))
		for _, op := range batch {
			models = append(models, mongo.NewUpdateOneModel().
				SetFilter(bson.M{"_id": op.ID}).
				SetUpdate(bson.M{"$set": bson.M{"accounts": model.ExtractAccounts(op.OpValue)}}))
		}

		opts := options.BulkWrite().SetOrdered(false)
		res, err := c.operations.BulkWrite(ctx, models, opts)
		if err != nil {
			return total, errors.Wrap(err, "failed to backfill accounts")
		}
		total += res.ModifiedCount

		if len(batch) < batchSize {
			return total, nil
		}
	}
}

// ListInvalidAccountIDs scans the account collection (projection: _id only)
// and returns the _id values of every document whose name fails the given
// validity check. These are stub documents created when user-controlled
// custom_json payloads were queued for refresh without account-name
// validation; they can never resolve on chain, so they only pollute the
// accounts list. _id is decoded as interface{} because a non-string _id is
// itself invalid and must be caught too.
func (c *Client) ListInvalidAccountIDs(ctx context.Context, valid func(string) bool) ([]interface{}, error) {
	cursor, err := c.db.Collection("account").Find(ctx, bson.M{},
		options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return nil, errors.Wrap(err, "failed to scan account ids")
	}
	defer cursor.Close(ctx)

	var invalid []interface{}
	for cursor.Next(ctx) {
		var doc struct {
			ID interface{} `bson:"_id"`
		}
		if err := cursor.Decode(&doc); err != nil {
			return invalid, errors.Wrap(err, "failed to decode account id")
		}
		name, ok := doc.ID.(string)
		if !ok || !valid(name) {
			invalid = append(invalid, doc.ID)
		}
	}
	if err := cursor.Err(); err != nil {
		return invalid, errors.Wrap(err, "failed to iterate account ids")
	}
	return invalid, nil
}

// DeleteAccountsByIDs removes account documents by _id in batches and
// returns the number of deleted documents.
func (c *Client) DeleteAccountsByIDs(ctx context.Context, ids []interface{}, batchSize int) (int64, error) {
	if batchSize <= 0 {
		batchSize = 1000
	}
	var total int64
	for i := 0; i < len(ids); i += batchSize {
		end := i + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		res, err := c.db.Collection("account").DeleteMany(ctx, bson.M{"_id": bson.M{"$in": ids[i:end]}})
		if err != nil {
			return total, errors.Wrap(err, "failed to delete invalid accounts")
		}
		total += res.DeletedCount
	}
	return total, nil
}

// BulkUpsertBlocks performs bulk upsert of blocks
func (c *Client) BulkUpsertBlocks(ctx context.Context, blocks []*model.Block) error {
	if len(blocks) == 0 {
		return nil
	}

	startTime := time.Now()
	models := make([]mongo.WriteModel, 0, len(blocks))
	for _, block := range blocks {
		filter := bson.M{"_id": block.BlockNum}
		update := bson.M{"$set": block}
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(filter).
			SetUpdate(update).
			SetUpsert(true))
	}

	opts := options.BulkWrite().SetOrdered(false)
	_, err := c.blocks.BulkWrite(ctx, models, opts)
	duration := time.Since(startTime)

	// Record metrics
	metrics.RecordMongoWrite("blocks", "bulk_upsert", duration, err)

	if err != nil {
		return errors.Wrap(err, "failed to bulk upsert blocks")
	}

	return nil
}

// BulkUpsertTransactions performs bulk upsert of transactions
func (c *Client) BulkUpsertTransactions(ctx context.Context, txs []*model.Transaction) error {
	if len(txs) == 0 {
		return nil
	}

	startTime := time.Now()
	models := make([]mongo.WriteModel, 0, len(txs))
	for _, tx := range txs {
		filter := bson.M{"_id": tx.ID}
		update := bson.M{"$set": tx}
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(filter).
			SetUpdate(update).
			SetUpsert(true))
	}

	opts := options.BulkWrite().SetOrdered(false)
	_, err := c.transactions.BulkWrite(ctx, models, opts)
	duration := time.Since(startTime)

	// Record metrics
	metrics.RecordMongoWrite("transactions", "bulk_upsert", duration, err)

	if err != nil {
		return errors.Wrap(err, "failed to bulk upsert transactions")
	}

	return nil
}

// GetMaxBlock returns the maximum block number from meta collection
func (c *Client) GetMaxBlock(ctx context.Context) (uint32, error) {
	var meta model.Meta
	filter := bson.M{"_id": "sync_state"}
	err := c.meta.FindOne(ctx, filter).Decode(&meta)
	if err == mongo.ErrNoDocuments {
		return 0, nil
	}
	if err != nil {
		return 0, errors.Wrap(err, "failed to get max block")
	}
	return meta.MaxBlock, nil
}

// UpdateMaxBlock updates the max block in meta collection
func (c *Client) UpdateMaxBlock(ctx context.Context, blockNum uint32) error {
	filter := bson.M{"_id": "sync_state"}
	update := bson.M{
		"$set": bson.M{
			"max_block":  blockNum,
			"updated_at": time.Now(),
		},
		"$setOnInsert": bson.M{
			"_id":             "sync_state",
			"cold_start_done": false,
		},
	}
	opts := options.Update().SetUpsert(true)

	_, err := c.meta.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return errors.Wrap(err, "failed to update max block")
	}

	return nil
}

// SetColdStartDone marks cold start as completed
func (c *Client) SetColdStartDone(ctx context.Context) error {
	filter := bson.M{"_id": "sync_state"}
	update := bson.M{
		"$set": bson.M{
			"cold_start_done": true,
			"updated_at":      time.Now(),
		},
	}
	opts := options.Update().SetUpsert(true)

	_, err := c.meta.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return errors.Wrap(err, "failed to set cold start done")
	}

	return nil
}

// GetBlockByNumber retrieves a block by block number
func (c *Client) GetBlockByNumber(ctx context.Context, blockNum uint32) (*model.Block, error) {
	var block model.Block
	filter := bson.M{"_id": blockNum}
	err := c.blocks.FindOne(ctx, filter).Decode(&block)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to get block")
	}
	return &block, nil
}

// GetOperationsByBlock retrieves operations for a specific block
func (c *Client) GetOperationsByBlock(ctx context.Context, blockNum uint32) ([]*model.Operation, error) {
	filter := bson.M{"block_num": blockNum}
	cursor, err := c.operations.Find(ctx, filter)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find operations")
	}
	defer cursor.Close(ctx)

	var ops []*model.Operation
	if err := cursor.All(ctx, &ops); err != nil {
		return nil, errors.Wrap(err, "failed to decode operations")
	}

	return ops, nil
}

// CheckBlockExists checks if a block exists
func (c *Client) CheckBlockExists(ctx context.Context, blockNum uint32) (bool, error) {
	filter := bson.M{"_id": blockNum}
	count, err := c.blocks.CountDocuments(ctx, filter)
	if err != nil {
		return false, errors.Wrap(err, "failed to check block existence")
	}
	return count > 0, nil
}

// Database returns the underlying *mongo.Database for direct collection access.
// Used by the processor package to write to business collections (account, comment, vote, ...).
func (c *Client) Database() *mongo.Database {
	return c.db
}
