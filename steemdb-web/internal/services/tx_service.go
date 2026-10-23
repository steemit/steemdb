package services

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/steemit/steemdb/web/internal/database"
	"github.com/steemit/steemdb/web/pkg/steem"
	"github.com/steemit/steemdb/web/pkg/utils"
)

// txCacheCollection caches full condenser_api.get_transaction documents
// fetched from the public node, keyed by transaction id (_id = txid).
// Landed transactions are immutable, so entries never need refresh or TTL;
// every transaction id costs at most one upstream RPC for the lifetime of
// the deployment. No secondary index: lookups go through _id only.
const txCacheCollection = "tx_cache"

// TxService serves transaction lookups local-first: the Mongo cache above,
// with the public Steem node as a read-through backfill source.
type TxService struct {
	db          *database.MongoDB
	steemClient *steem.Client
	logger      utils.Logger
}

// NewTxService creates a new transaction service
func NewTxService(db *database.MongoDB, steemClient *steem.Client, logger utils.Logger) *TxService {
	return &TxService{
		db:          db,
		steemClient: steemClient,
		logger:      logger,
	}
}

// cachedTx is the on-disk shape of one cache entry. The raw upstream
// document is nested under "tx" so a cache hit returns exactly the same
// shape as a fresh fetch, instead of leaking cache metadata into the API.
type cachedTx struct {
	Tx map[string]interface{} `bson:"tx"`
}

// GetTransaction returns the full transaction document for txID.
//
// Read-through: a cache hit is served locally; a miss is fetched from the
// public node once and stored, so subsequent lookups never leave the local
// database. Failures of the cache itself never fail the request — they only
// degrade to the direct-proxy behavior.
func (s *TxService) GetTransaction(ctx context.Context, txID string) (map[string]interface{}, error) {
	tx, err := s.fromCache(ctx, txID)
	if err == nil && tx != nil {
		return tx, nil
	}
	if err != nil {
		// Unreadable cache (not a plain miss): log and fall through to RPC.
		s.logger.Warn("tx cache lookup failed, fetching upstream",
			utils.String("tx_id", txID), utils.Error(err))
	}

	tx, err = s.steemClient.GetTransaction(txID)
	if err != nil {
		return nil, err
	}

	if cacheable(tx) {
		s.store(ctx, txID, tx)
	}
	return tx, nil
}

// fromCache returns the cached document for txID, or nil on a miss.
func (s *TxService) fromCache(ctx context.Context, txID string) (map[string]interface{}, error) {
	var entry cachedTx
	err := s.db.Collection(txCacheCollection).FindOne(ctx, bson.M{"_id": txID}).Decode(&entry)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if entry.Tx == nil {
		// Corrupt entry without the payload: treat as a miss and refetch.
		return nil, nil
	}
	return entry.Tx, nil
}

// store persists tx as a best-effort, fire-and-forget write. A concurrent
// fetch of the same id racing to insert is fine: the loser's duplicate-key
// error is ignored (the data is identical), and any other error only means
// the next lookup fetches upstream again.
func (s *TxService) store(ctx context.Context, txID string, tx map[string]interface{}) {
	doc := bson.M{"_id": txID, "_ts": time.Now(), "tx": tx}
	if _, err := s.db.Collection(txCacheCollection).InsertOne(ctx, doc); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return
		}
		s.logger.Warn("tx cache write failed",
			utils.String("tx_id", txID), utils.Error(err))
	}
}

// cacheable reports whether an upstream document is safe to cache forever.
// condenser_api.get_transaction only returns included transactions (it reads
// the account_history index), so a real result always carries block_num > 0.
// Anything else (missing or non-positive block_num) is an unconfirmed or
// degenerate shape from a different upstream — serve it, but never pin it.
func cacheable(tx map[string]interface{}) bool {
	blockNum, ok := tx["block_num"].(float64)
	return ok && blockNum > 0
}
