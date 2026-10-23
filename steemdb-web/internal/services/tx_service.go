package services

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/steemit/steemdb/web/internal/database"
	"github.com/steemit/steemdb/web/pkg/steem"
	"github.com/steemit/steemdb/web/pkg/utils"
)

// txCacheCollection caches full condenser_api.get_transaction documents
// fetched from the public node, keyed by transaction id (_id = txid).
// Landed transactions are immutable, so entries never need refresh or TTL.
// Only transactions the chain itself reports as irreversible are admitted
// (see lastIrreversibleBlockNum), so a fork can never pin wrong data
// forever. No secondary index: lookups go through _id only.
const txCacheCollection = "tx_cache"

// txStoreTimeout bounds the detached cache write-back (see store).
const txStoreTimeout = 3 * time.Second

// txLibCacheTTL bounds how often the public node is asked for the chain's
// last irreversible block number. Short enough that the gate tracks the
// chain closely, long enough that poll loops (exchanges checking a fresh
// txid every few seconds) share one get_dynamic_global_properties call.
const txLibCacheTTL = 3 * time.Second

// ErrTxNotIrreversible reports that the transaction exists on chain but its
// block is still inside the reversible window: the API answers with 425 and
// does not serve the document — the site only displays irreversible data,
// because a reversible transaction can still be forked out or move blocks.
var ErrTxNotIrreversible = errors.New("transaction is not yet irreversible")

// ErrTxNotFound reports an upstream answer that is not shaped like an
// included transaction (empty document, or a missing/non-positive/
// non-integer block_num). condenser_api.get_transaction reads the account
// history index and only returns included transactions, so this is a
// degenerate shape — surfaced as the same 404 unknown ids map to.
var ErrTxNotFound = errors.New("transaction not found")

// steemRPC is the subset of the Steem client TxService depends on. It
// exists so tests can stub the upstream node.
type steemRPC interface {
	GetTransaction(txID string) (map[string]interface{}, error)
	GetDynamicGlobalProperties() (*steem.DynamicGlobalProperties, error)
}

// TxService serves transaction lookups local-first: the Mongo cache above,
// with the public Steem node as a read-through backfill source.
type TxService struct {
	db          *database.MongoDB
	steemClient steemRPC
	logger      utils.Logger

	// libMu guards lib and libAt (see lastIrreversibleBlockNum).
	libMu sync.Mutex
	lib   int64
	libAt time.Time
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

// GetTransaction returns the full transaction document for txID, but only
// once the chain itself declares it irreversible — reversible data is never
// displayed, so a fork cannot show a user a transaction that later moves or
// vanishes.
//
// Read-through: a cache hit is served locally (entries were irreversible
// when admitted and LIB only advances, so they stay irreversible); a miss
// is fetched from the public node and, once past LIB, stored, so subsequent
// lookups never leave the local database. Failures of the cache itself
// never fail the request — they only degrade to the direct-proxy behavior.
// A transaction queried before it passes LIB is re-fetched upstream on
// every request (no negative caching): the price of never showing, or
// pinning, reversible data. When LIB cannot be proven at all the call fails
// closed instead of serving unprovable data.
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
	blockNum, ok := includedBlockNum(tx)
	if !ok {
		return nil, ErrTxNotFound
	}

	lib, err := s.lastIrreversibleBlockNum()
	if err != nil {
		return nil, err
	}
	if blockNum > float64(lib) {
		return nil, ErrTxNotIrreversible
	}

	s.store(txID, tx)
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

// store persists tx as a best-effort, fire-and-forget write on a detached
// context: once the upstream RPC has been paid for, a client disconnecting
// must not cancel the write-back. A concurrent fetch of the same id racing
// to insert is fine: the loser's duplicate-key error is ignored (the data
// is identical), and any other error only means the next lookup fetches
// upstream again.
func (s *TxService) store(txID string, tx map[string]interface{}) {
	ctx, cancel := context.WithTimeout(context.Background(), txStoreTimeout)
	defer cancel()
	doc := bson.M{"_id": txID, "_ts": time.Now(), "tx": tx}
	if _, err := s.db.Collection(txCacheCollection).InsertOne(ctx, doc); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return
		}
		s.logger.Warn("tx cache write failed",
			utils.String("tx_id", txID), utils.Error(err))
	}
}

// lastIrreversibleBlockNum returns the chain's last irreversible block
// number, fetched from the public node (get_dynamic_global_properties) and
// cached in memory for txLibCacheTTL. The authoritative LIB comes from the
// chain itself rather than a local depth heuristic, so the gate holds even
// during chain-level trouble: whatever the chain has not finalized is not
// served, let alone pinned.
//
// LIB only ever advances, so when a refresh fails the last cached value
// remains a valid lower bound of the true LIB — serving it keeps the gate
// strict (it can only delay admitting a transaction, never admit a
// reversible one). Only when no value was ever fetched does the error
// propagate: irreversibility then cannot be proven and the caller fails
// closed. The fetch runs under the mutex so concurrent cold lookups
// coalesce into one RPC instead of stampeding the node.
func (s *TxService) lastIrreversibleBlockNum() (int64, error) {
	s.libMu.Lock()
	defer s.libMu.Unlock()
	if s.lib > 0 && time.Since(s.libAt) < txLibCacheTTL {
		return s.lib, nil
	}
	dgp, err := s.steemClient.GetDynamicGlobalProperties()
	if err != nil || dgp == nil {
		if s.lib > 0 {
			if err != nil {
				s.logger.Warn("lib refresh failed, keeping last known value",
					utils.Int64("lib", s.lib), utils.Error(err))
			} else {
				s.logger.Warn("lib refresh returned no properties, keeping last known value",
					utils.Int64("lib", s.lib))
			}
			return s.lib, nil
		}
		if err == nil {
			err = errors.New("empty dynamic global properties")
		}
		return 0, err
	}
	s.lib = dgp.LastIrreversibleBlockNum
	s.libAt = time.Now()
	return s.lib, nil
}

// includedBlockNum extracts the block number from an upstream document.
// condenser_api.get_transaction only returns included transactions (it
// reads the account_history index), so a real result always carries a
// positive integer block_num. Anything else (missing, non-positive,
// fractional, wrong-typed, or out of range) is an unconfirmed or degenerate
// shape from a different upstream — neither displayed nor pinned. JSON
// numbers decode as float64, which is the shape the RPC client produces.
func includedBlockNum(tx map[string]interface{}) (float64, bool) {
	blockNum, ok := tx["block_num"].(float64)
	if !ok || blockNum <= 0 || blockNum != math.Trunc(blockNum) || blockNum > math.MaxUint32 {
		return 0, false
	}
	return blockNum, true
}
