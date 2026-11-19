package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/steemit/steemdb/web/internal/database"
	"github.com/steemit/steemdb/web/internal/models"
	"github.com/steemit/steemdb/web/pkg/steem"
	"github.com/steemit/steemdb/web/pkg/utils"
)

// Steem produces one block per 3-second slot, and a skipped slot still consumes
// a block number, so a block_num delta and elapsed time are interchangeable:
// N blocks == 3N seconds. Both stats endpoints use that identity to turn a
// duration window into an index-servable block_num range.
const steemBlockInterval = 3 * time.Second

const (
	defaultStatsRange = 24 * time.Hour
	minStatsRange     = time.Hour
	maxStatsRange     = 7 * 24 * time.Hour
	// statsAggregateTimeout bounds the on-demand operation histogram. The
	// supported windows cost tens to a few hundred milliseconds on production
	// (23ms per 2000 blocks, ~120k ops/day), so 10s only ever fires when
	// something pathological happens (cold cache, replica stall) — failing fast
	// beats pinning a request handler.
	statsAggregateTimeout = 10 * time.Second
)

// ErrInvalidStatsRange marks a /operations/stats range parameter that cannot be
// parsed or is outside the supported bounds. Handlers map it to HTTP 400; every
// other error is a 500.
var ErrInvalidStatsRange = errors.New("invalid stats range")

// BlockService handles block-related operations
type BlockService struct {
	db          *database.MongoDB
	steemClient *steem.Client
	logger      utils.Logger
}

// NewBlockService creates a new block service
func NewBlockService(db *database.MongoDB, steemClient *steem.Client, logger utils.Logger) *BlockService {
	return &BlockService{
		db:          db,
		steemClient: steemClient,
		logger:      logger,
	}
}

// GetBlock retrieves a block by number. The local blocks collection does not
// store transaction details (the sync batcher only keeps block headers), so
// after a local hit the transaction list is enriched from the steem RPC.
// When the RPC call fails the block is still returned with local fields only.
func (s *BlockService) GetBlock(ctx context.Context, blockNum int64) (*models.Block, error) {
	collection := s.db.Collection("blocks")
	var block models.Block

	err := collection.FindOne(ctx, bson.M{"_id": blockNum}).Decode(&block)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("block not found: %d", blockNum)
		}
		return nil, fmt.Errorf("failed to get block: %w", err)
	}

	s.enrichTransactions(&block, blockNum)

	return &block, nil
}

// enrichTransactions fills Transactions (and TransactionCount when missing)
// from the steem RPC block. Failures are logged and ignored — the block is
// still useful without the transaction list.
func (s *BlockService) enrichTransactions(block *models.Block, blockNum int64) {
	if len(block.Transactions) > 0 {
		return
	}

	rpcBlock, err := s.steemClient.GetBlock(blockNum)
	if err != nil || rpcBlock == nil {
		s.logger.Warn("Failed to enrich block transactions from RPC",
			utils.Int64("block_num", blockNum), utils.Error(err))
		return
	}

	block.Transactions = make([]models.Transaction, 0, len(rpcBlock.Transactions))
	for i := range rpcBlock.Transactions {
		tx := &rpcBlock.Transactions[i]
		ops := make([]models.Operation, 0, len(tx.Operations))
		for j := range tx.Operations {
			ops = append(ops, models.Operation{
				ID:       fmt.Sprintf("%d:%d:%d", blockNum, i, j),
				BlockNum: uint32(blockNum),
				TrxID:    tx.TransactionID,
				TrxIndex: int32(i),
				OpIndex:  int32(j),
				OpType:   tx.Operations[j].Type,
				OpValue:  opToMap(tx.Operations[j].Value),
			})
		}
		block.Transactions = append(block.Transactions, models.Transaction{
			ID:             tx.TransactionID,
			RefBlockNum:    tx.RefBlockNum,
			RefBlockPrefix: tx.RefBlockPrefix,
			Expiration:     tx.Expiration,
			Operations:     ops,
			Extensions:     tx.Extensions,
			Signatures:     tx.Signatures,
			TransactionID:  tx.TransactionID,
		})
	}

	if block.TransactionCount == 0 {
		block.TransactionCount = len(block.Transactions)
	}
	if len(block.TransactionIDs) == 0 {
		block.TransactionIDs = rpcBlock.TransactionIDs
	}
	// Cold-ingested historical blocks only persist the block number, so the
	// remaining header fields are enriched from the RPC block when empty.
	if block.BlockID == "" {
		block.BlockID = rpcBlock.BlockID
	}
	if block.Previous == "" {
		block.Previous = rpcBlock.Previous
	}
	if block.Witness == "" {
		block.Witness = rpcBlock.Witness
	}
	if block.TransactionMerkleRoot == "" {
		block.TransactionMerkleRoot = rpcBlock.TransactionRoot
	}
	if block.WitnessSignature == "" {
		block.WitnessSignature = rpcBlock.WitnessSignature
	}
	if len(block.Extensions) == 0 {
		block.Extensions = rpcBlock.Extensions
	}
	if block.OperationCount == 0 {
		opCount := 0
		for _, tx := range block.Transactions {
			opCount += len(tx.Operations)
		}
		block.OperationCount = opCount
	}
}

// opToMap normalizes an RPC operation payload to a generic map so it can be
// serialized as op_value. Known operation types decode to typed structs (e.g.
// *protocol.VoteOperation) and unknown ones to *json.RawMessage, so round-trip
// through JSON to get the generic map shape in every case.
func opToMap(value interface{}) map[string]interface{} {
	switch v := value.(type) {
	case nil:
		return nil
	case map[string]interface{}:
		return v
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		var out map[string]interface{}
		if err := json.Unmarshal(raw, &out); err != nil {
			// Payloads that are not JSON objects (rare) have no map form.
			return nil
		}
		return out
	}
}

// GetVirtualOps retrieves the virtual operations of a block. Virtual
// operations are not persisted locally (the sync batcher only stores block
// headers and user operations), so they are always fetched from the steem RPC,
// mirroring the legacy steemd.getVirtualOpsInBlock call.
func (s *BlockService) GetVirtualOps(ctx context.Context, blockNum int64) ([]models.VirtualOperation, error) {
	ops, err := s.steemClient.GetOpsInBlock(blockNum, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get virtual ops: %w", err)
	}

	result := make([]models.VirtualOperation, 0, len(ops))
	for _, op := range ops {
		if op == nil {
			continue
		}
		opTimestamp := time.Time{}
		if op.Timestamp != nil {
			opTimestamp = steem.ToTime(*op.Timestamp)
		}
		result = append(result, models.VirtualOperation{
			BlockNum:   int64(op.BlockNumber),
			TrxID:      op.TransactionID,
			TrxInBlock: int64(op.TransactionInBlock),
			OpInTrx:    int64(op.OperationInTransaction),
			VirtualOp:  int64(op.VirtualOperation),
			Timestamp:  opTimestamp,
			OpType:     string(op.Operation.Type()),
			OpValue:    opToMap(op.Operation.Data()),
		})
	}

	return result, nil
}

// GetBlocks retrieves multiple blocks with pagination
func (s *BlockService) GetBlocks(ctx context.Context, params models.PaginationParams, sortParams models.SortParams) (*models.BlockSearchResult, error) {
	collection := s.db.Collection("blocks")

	// Build sort options. Blocks store the block number as _id, so sorting by
	// _id uses the index; block_num has no index and forces an in-memory sort
	// over the whole collection.
	sortField := "_id"
	sortOrder := -1
	if sortParams.SortBy != "" {
		sortField = sortParams.SortBy
	}
	if sortField == "block_num" || sortField == "number" || sortField == "id" {
		sortField = "_id"
	}
	if sortParams.SortOrder == "asc" {
		sortOrder = 1
	}

	// Count total documents. An exact count scans the whole _id index of a
	// 20M-block collection (~10s); the explorer list page only needs an
	// approximate total for pagination, so use the collection metadata count.
	total, err := collection.EstimatedDocumentCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count blocks: %w", err)
	}

	// Calculate pagination
	skip := (params.Page - 1) * params.PageSize
	findOptions := options.Find().
		SetSort(bson.M{sortField: sortOrder}).
		SetSkip(int64(skip)).
		SetLimit(int64(params.PageSize))

	cursor, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to find blocks: %w", err)
	}
	defer cursor.Close(ctx)

	var blocks []models.BlockSummary
	for cursor.Next(ctx) {
		var block models.Block
		if err := cursor.Decode(&block); err != nil {
			s.logger.Error("Failed to decode block", utils.Error(err))
			continue
		}

		summary := models.BlockSummary{
			Number:           block.BlockNum,
			BlockNum:         block.BlockNum,
			Timestamp:        block.Timestamp,
			Witness:          block.Witness,
			TransactionCount: block.TransactionCount,
			OperationCount:   block.OperationCount,
			Previous:         block.Previous,
		}
		blocks = append(blocks, summary)
	}

	return &models.BlockSearchResult{
		Blocks:   blocks,
		Total:    total,
		Page:     params.Page,
		PageSize: params.PageSize,
	}, nil
}

// GetLatestBlocks retrieves the most recent blocks
func (s *BlockService) GetLatestBlocks(ctx context.Context, limit int) ([]models.BlockSummary, error) {
	collection := s.db.Collection("blocks")

	findOptions := options.Find().
		SetSort(bson.M{"_id": -1}).
		SetLimit(int64(limit))

	cursor, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to find latest blocks: %w", err)
	}
	defer cursor.Close(ctx)

	var blocks []models.BlockSummary
	for cursor.Next(ctx) {
		var block models.Block
		if err := cursor.Decode(&block); err != nil {
			s.logger.Error("Failed to decode block", utils.Error(err))
			continue
		}

		summary := models.BlockSummary{
			Number:           block.BlockNum,
			BlockNum:         block.BlockNum,
			Timestamp:        block.Timestamp,
			Witness:          block.Witness,
			TransactionCount: block.TransactionCount,
			OperationCount:   block.OperationCount,
			Previous:         block.Previous,
		}
		blocks = append(blocks, summary)
	}

	return blocks, nil
}

// GetBlocksByWitness retrieves blocks produced by a specific witness
func (s *BlockService) GetBlocksByWitness(ctx context.Context, witness string, params models.PaginationParams) (*models.BlockSearchResult, error) {
	collection := s.db.Collection("blocks")

	filter := bson.M{"witness": witness}

	// Count total documents
	total, err := collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to count blocks by witness: %w", err)
	}

	// Calculate pagination
	skip := (params.Page - 1) * params.PageSize
	findOptions := options.Find().
		SetSort(bson.M{"_id": -1}).
		SetSkip(int64(skip)).
		SetLimit(int64(params.PageSize))

	cursor, err := collection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to find blocks by witness: %w", err)
	}
	defer cursor.Close(ctx)

	var blocks []models.BlockSummary
	for cursor.Next(ctx) {
		var block models.Block
		if err := cursor.Decode(&block); err != nil {
			s.logger.Error("Failed to decode block", utils.Error(err))
			continue
		}

		summary := models.BlockSummary{
			Number:           block.BlockNum,
			BlockNum:         block.BlockNum,
			Timestamp:        block.Timestamp,
			Witness:          block.Witness,
			TransactionCount: block.TransactionCount,
			OperationCount:   block.OperationCount,
			Previous:         block.Previous,
		}
		blocks = append(blocks, summary)
	}

	return &models.BlockSearchResult{
		Blocks:   blocks,
		Total:    total,
		Page:     params.Page,
		PageSize: params.PageSize,
	}, nil
}

// GetBlockOperations retrieves operations from a specific block
func (s *BlockService) GetBlockOperations(ctx context.Context, blockNum int64, params models.PaginationParams) ([]models.Operation, error) {
	collection := s.db.Collection("operations")

	filter := bson.M{"block_num": blockNum}

	// Calculate pagination
	skip := (params.Page - 1) * params.PageSize
	findOptions := options.Find().
		SetSort(bson.D{{Key: "trx_index", Value: 1}, {Key: "op_index", Value: 1}}).
		SetSkip(int64(skip)).
		SetLimit(int64(params.PageSize))

	cursor, err := collection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to find block operations: %w", err)
	}
	defer cursor.Close(ctx)

	var operations []models.Operation
	for cursor.Next(ctx) {
		var operation models.Operation
		if err := cursor.Decode(&operation); err != nil {
			s.logger.Error("Failed to decode operation", utils.Error(err))
			continue
		}
		operations = append(operations, operation)
	}

	return operations, nil
}

// GetBlockStats retrieves blockchain statistics.
//
// The two totals are estimates, not exact counts: operations holds ~2.2B
// documents and transactions ~3.4M, so an exact countDocuments would scan the
// whole collection on every request. EstimatedDocumentCount reads the
// collection metadata instead (instant) and is exact for small collections; the
// response carries totals_estimated: true so clients can label the number.
//
// The per-hour rates come from the last 1200 blocks (one hour at 3s/slot) and
// are real counts over that index-backed window.
func (s *BlockService) GetBlockStats(ctx context.Context) (*models.BlockStats, error) {
	head, headTime, err := s.latestBlockHeader(ctx)
	if err != nil {
		return nil, err
	}

	totalTransactions, err := s.db.Collection("transactions").EstimatedDocumentCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to estimate transactions: %w", err)
	}
	totalOperations, err := s.db.Collection("operations").EstimatedDocumentCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to estimate operations: %w", err)
	}

	from, to := blockNumWindow(head, time.Hour)
	window := bson.M{"block_num": bson.M{"$gte": from, "$lte": to}}
	transactionsPerHour, err := s.db.Collection("transactions").CountDocuments(ctx, window)
	if err != nil {
		return nil, fmt.Errorf("failed to count transactions in the last hour: %w", err)
	}
	operationsPerHour, err := s.db.Collection("operations").CountDocuments(ctx, window)
	if err != nil {
		return nil, fmt.Errorf("failed to count operations in the last hour: %w", err)
	}

	// last_irreversible_num is a fixed-depth approximation (the chain's LIB sits
	// ~20 blocks behind the head); /api/v1/stats/props carries the authoritative
	// value from get_dynamic_global_properties for callers that need it.
	lastIrreversible := head - 20
	if lastIrreversible < 0 {
		lastIrreversible = 0
	}

	return &models.BlockStats{
		LatestBlockNum:      head,
		LastIrreversibleNum: lastIrreversible,
		HeadBlockTime:       headTime,
		TotalTransactions:   totalTransactions,
		TotalOperations:     totalOperations,
		TotalsEstimated:     true,
		AverageBlockTime:    steemBlockInterval.Seconds(),
		TransactionsPerHour: transactionsPerHour,
		OperationsPerHour:   operationsPerHour,
	}, nil
}

// GetOperationStats retrieves the operation-type distribution over a bounded
// window (default 24h, supported 1h..7d).
//
// Cost: the window is expressed as a block_num range and operations has
// {op_type:1, block_num:-1}, so the $group is index-servable — 23ms per 2000
// blocks on production, i.e. a few hundred ms for a full day. An unbounded
// histogram would be a 2.2B-document collection scan, which is why the window
// is mandatory (the default silently supplies it).
//
// Coverage: every document in operations, including virtual ops
// (producer_reward, curation_reward, …), matching the totals reported by
// /blocks/stats. Percentages are of the window's own total, rounded to two
// decimals, so they can drift a hundredth from 100.
func (s *BlockService) GetOperationStats(ctx context.Context, timeRange string) ([]models.OperationStats, error) {
	window, err := parseStatsRange(timeRange)
	if err != nil {
		return nil, err
	}

	head, _, err := s.latestBlockHeader(ctx)
	if err != nil {
		return nil, err
	}
	from, to := blockNumWindow(head, window)

	ctx, cancel := context.WithTimeout(ctx, statsAggregateTimeout)
	defer cancel()

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.D{{Key: "block_num", Value: bson.D{
			{Key: "$gte", Value: from},
			{Key: "$lte", Value: to},
		}}}}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$op_type"},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
		// Multi-key sorts must be bson.D: $sort reads document order as
		// priority, and an unordered map would silently randomise it.
		{{Key: "$sort", Value: bson.D{
			{Key: "count", Value: -1},
			{Key: "_id", Value: 1},
		}}},
	}

	cursor, err := s.db.Collection("operations").Aggregate(ctx, pipeline,
		options.Aggregate().SetAllowDiskUse(true))
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate operation stats: %w", err)
	}
	defer cursor.Close(ctx)

	var rows []struct {
		Type  string `bson:"_id"`
		Count int64  `bson:"count"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, fmt.Errorf("failed to read operation stats: %w", err)
	}

	var total int64
	for _, row := range rows {
		total += row.Count
	}

	stats := make([]models.OperationStats, 0, len(rows))
	for _, row := range rows {
		var percentage float64
		if total > 0 {
			percentage = math.Round(float64(row.Count)*10000/float64(total)) / 100
		}
		stats = append(stats, models.OperationStats{
			Type:       row.Type,
			Count:      row.Count,
			Percentage: percentage,
		})
	}

	return stats, nil
}

// latestBlockHeader returns the local chain head: the highest block_num in the
// blocks collection and its timestamp. It is the upper bound of every stats
// window — the local database is the authority for what has been ingested.
func (s *BlockService) latestBlockHeader(ctx context.Context) (int64, time.Time, error) {
	var latest models.Block
	err := s.db.Collection("blocks").
		FindOne(ctx, bson.M{}, options.FindOne().SetSort(bson.D{{Key: "_id", Value: -1}})).
		Decode(&latest)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return 0, time.Time{}, fmt.Errorf("blocks collection is empty")
		}
		return 0, time.Time{}, fmt.Errorf("failed to get latest block: %w", err)
	}
	return int64(latest.BlockNum), latest.Timestamp, nil
}

// blockNumWindow maps a duration ending at head onto an inclusive block_num
// range. See steemBlockInterval for why the conversion is exact.
func blockNumWindow(head int64, d time.Duration) (from, to int64) {
	span := int64(d / steemBlockInterval)
	if span < 1 {
		span = 1
	}
	from = head - span
	if from < 0 {
		from = 0
	}
	return from, head
}

// parseStatsRange parses the range query parameter: Go duration syntax plus a
// "d" (days) suffix, e.g. 1h, 6h, 24h, 7d. Empty means the 24h default; values
// outside [1h, 7d] are rejected rather than clamped so a caller asking for an
// unsupported window gets a clear 400 instead of a silently different answer.
func parseStatsRange(raw string) (time.Duration, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return defaultStatsRange, nil
	}

	var window time.Duration
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, fmt.Errorf("%w: %q (use e.g. 6h, 24h, 7d)", ErrInvalidStatsRange, raw)
		}
		window = time.Duration(days) * 24 * time.Hour
	} else {
		parsed, err := time.ParseDuration(s)
		if err != nil {
			return 0, fmt.Errorf("%w: %q (use e.g. 6h, 24h, 7d)", ErrInvalidStatsRange, raw)
		}
		window = parsed
	}

	if window < minStatsRange || window > maxStatsRange {
		return 0, fmt.Errorf("%w: %q must be between 1h and 7d", ErrInvalidStatsRange, raw)
	}
	return window, nil
}
