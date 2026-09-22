package services

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/steemit/steemdb/web/internal/database"
	"github.com/steemit/steemdb/web/internal/models"
	"github.com/steemit/steemdb/web/pkg/utils"
)

// AccountService handles account-related operations
type AccountService struct {
	db     *database.MongoDB
	logger utils.Logger
}

// NewAccountService creates a new account service
func NewAccountService(db *database.MongoDB, logger utils.Logger) *AccountService {
	return &AccountService{
		db:     db,
		logger: logger,
	}
}

// GetAccount retrieves an account by name. Decoded into a generic map: the
// account documents are written by steemdb-sync from raw RPC values (e.g.
// proxied_vsf_votes mixes strings and numbers), which a rigid struct cannot
// decode — the passthrough shape matches what the frontend consumes anyway.
func (s *AccountService) GetAccount(ctx context.Context, name string) (map[string]interface{}, error) {
	collection := s.db.Collection("account")
	var account bson.M

	// The sync schema keys account documents by _id = account name; the
	// "name" field only exists on docs already populated by the refresher,
	// and querying it would also be an unindexed collection scan.
	err := collection.FindOne(ctx, bson.M{"_id": name}).Decode(&account)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("account not found: %s", name)
		}
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	return account, nil
}

// accountSortField maps the API sort_by value to a mongo sort field. Only
// index-backed fields are offered (sync's indexInventory is the index
// authority: reputation and vesting_shares have dedicated indexes; _id is
// the account name and carries the default index). Sorting on any other
// field would be an unindexed in-memory sort over the whole account
// collection, so unknown values fall back to the default.
func accountSortField(sortBy string) string {
	switch sortBy {
	case "name":
		// _id is the account name and exists on every document; the `name`
		// field only exists on refresher-populated documents.
		return "_id"
	case "vests", "vesting_shares":
		return "vesting_shares"
	case "reputation":
		return "reputation"
	default:
		return "reputation"
	}
}

// accountListFilter builds the listing filter for GetAccounts: an empty
// filter lists every account; a non-empty search narrows it to account-name
// prefixes via the escaped prefix matcher shared with SearchAccounts.
func accountListFilter(search string) bson.M {
	if search == "" {
		return bson.M{}
	}
	return accountNamePrefixFilter(search)
}

// GetAccounts retrieves multiple accounts with pagination. A non-empty
// search narrows the listing to account-name prefixes using the same
// escaped prefix matcher as SearchAccounts.
func (s *AccountService) GetAccounts(ctx context.Context, params models.PaginationParams, sortParams models.SortParams, search string) (*models.AccountSearchResult, error) {
	collection := s.db.Collection("account")

	sortField := accountSortField(sortParams.SortBy)
	sortOrder := -1
	if sortParams.SortOrder == "asc" {
		sortOrder = 1
	}

	filter := accountListFilter(search)

	// Count total documents
	total, err := collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to count accounts: %w", err)
	}

	// Calculate pagination
	skip := (params.Page - 1) * params.PageSize
	findOptions := options.Find().
		SetSort(bson.M{sortField: sortOrder}).
		SetSkip(int64(skip)).
		SetLimit(int64(params.PageSize))

	cursor, err := collection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to find accounts: %w", err)
	}
	defer cursor.Close(ctx)

	var accounts []models.AccountSummary
	for cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			s.logger.Error("Failed to decode account", utils.Error(err))
			continue
		}
		accounts = append(accounts, accountSummaryFromMap(doc))
	}

	return &models.AccountSearchResult{
		Accounts: accounts,
		Total:    total,
		Page:     params.Page,
		PageSize: params.PageSize,
	}, nil
}

// accountNamePrefixFilter builds the _id filter for account name prefix
// search. The query is user input reaching a $regex pattern, so it is
// escaped with regexp.QuoteMeta: unescaped metacharacters turn the prefix
// search into an arbitrary regex (e.g. ".*" forces a full collection
// scan) — a regex injection / cheap DoS vector.
func accountNamePrefixFilter(query string) bson.M {
	return bson.M{
		"_id": bson.M{
			"$regex":   "^" + regexp.QuoteMeta(query),
			"$options": "i",
		},
	}
}

// SearchAccounts searches for accounts by name pattern
func (s *AccountService) SearchAccounts(ctx context.Context, query string, limit int) (*models.AccountSearchResult, error) {
	collection := s.db.Collection("account")

	// Build search filter — _id is the account name (see GetAccount)
	filter := accountNamePrefixFilter(query)

	// Count total matches
	total, err := collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to count search results: %w", err)
	}

	// Find matching accounts
	findOptions := options.Find().
		SetSort(bson.M{"reputation": -1}).
		SetLimit(int64(limit))

	cursor, err := collection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to search accounts: %w", err)
	}
	defer cursor.Close(ctx)

	var accounts []models.AccountSummary
	for cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			s.logger.Error("Failed to decode account", utils.Error(err))
			continue
		}
		accounts = append(accounts, accountSummaryFromMap(doc))
	}

	return &models.AccountSearchResult{
		Accounts: accounts,
		Total:    total,
		Page:     1,
		PageSize: limit,
	}, nil
}

// GetAccountHistory retrieves account operation history by querying the
// operations collection on the denormalized accounts array (populated by
// steemdb-sync at ingest time). block_num is used for sorting because it is
// strictly monotonic in time and the operations collection has no timestamp
// field; block_time is enriched afterwards from the blocks collection.
func (s *AccountService) GetAccountHistory(ctx context.Context, name string, params models.PaginationParams) (*models.AccountHistoryResult, error) {
	collection := s.db.Collection("operations")

	// Build filter
	filter := bson.M{
		"accounts": name,
	}

	// Count total documents
	total, err := collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to count account operations: %w", err)
	}

	// Calculate pagination
	skip := (params.Page - 1) * params.PageSize
	totalPages := int((total + int64(params.PageSize) - 1) / int64(params.PageSize))

	// Build find options
	findOptions := options.Find().
		SetSort(bson.M{"block_num": -1}). // Monotonic in time; operations has no timestamp
		SetSkip(int64(skip)).
		SetLimit(int64(params.PageSize))

	// Query operations
	cursor, err := collection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to find account operations: %w", err)
	}
	defer cursor.Close(ctx)

	type opDoc struct {
		ID       string                 `bson:"_id"`
		BlockNum int64                  `bson:"block_num"`
		TrxID    string                 `bson:"trx_id"`
		OpType   string                 `bson:"op_type"`
		OpValue  map[string]interface{} `bson:"op_value"`
	}

	var operations []models.AccountOperation
	blockNums := make(map[int64]bool)
	for cursor.Next(ctx) {
		var doc opDoc
		if err := cursor.Decode(&doc); err != nil {
			s.logger.Error("Failed to decode operation", utils.Error(err))
			continue
		}
		operations = append(operations, models.AccountOperation{
			ID:       doc.ID,
			Account:  name,
			BlockNum: doc.BlockNum,
			OpType:   doc.OpType,
			TrxID:    doc.TrxID,
			Summary:  buildOpSummary(doc.OpType, doc.OpValue),
		})
		blockNums[doc.BlockNum] = true
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	// Enrich block_time from the blocks collection (page-sized lookup)
	blockTimes := s.fetchBlockTimes(ctx, blockNums)
	for i := range operations {
		if ts, ok := blockTimes[operations[i].BlockNum]; ok {
			operations[i].BlockTime = ts
		}
	}

	return &models.AccountHistoryResult{
		Operations: operations,
		Total:      total,
		Page:       params.Page,
		PageSize:   params.PageSize,
		TotalPages: totalPages,
	}, nil
}

// fetchBlockTimes loads timestamps for the given block numbers in one query.
func (s *AccountService) fetchBlockTimes(ctx context.Context, blockNums map[int64]bool) map[int64]time.Time {
	result := make(map[int64]time.Time, len(blockNums))
	if len(blockNums) == 0 {
		return result
	}

	nums := make([]int64, 0, len(blockNums))
	for n := range blockNums {
		nums = append(nums, n)
	}

	// Query by _id: blocks key their documents by _id = block number (same
	// convention as BlockService.GetBlocks), which is always index-backed.
	// The old query on the `block_num` field had no index anywhere and forced
	// a collection scan per enrichment.
	cursor, err := s.db.Collection("blocks").Find(ctx,
		bson.M{"_id": bson.M{"$in": nums}},
		options.Find().SetProjection(bson.M{"timestamp": 1}),
	)
	if err != nil {
		s.logger.Warn("Failed to fetch block times", utils.Error(err))
		return result
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var block struct {
			BlockNum  int64     `bson:"_id"`
			Timestamp time.Time `bson:"timestamp"`
		}
		if err := cursor.Decode(&block); err != nil {
			continue
		}
		result[block.BlockNum] = block.Timestamp
	}

	return result
}

// GetAccountStats retrieves account statistics
func (s *AccountService) GetAccountStats(ctx context.Context) (*models.AccountStats, error) {
	collection := s.db.Collection("account")

	// Get total accounts
	totalAccounts, err := collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to count total accounts: %w", err)
	}

	// Get active accounts (posted in last 30 days)
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)
	activeAccounts, err := collection.CountDocuments(ctx, bson.M{
		"last_post": bson.M{"$gte": thirtyDaysAgo},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to count active accounts: %w", err)
	}

	// Get new accounts today
	today := time.Now().Truncate(24 * time.Hour)
	newAccountsToday, err := collection.CountDocuments(ctx, bson.M{
		"created": bson.M{"$gte": today},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to count new accounts today: %w", err)
	}

	return &models.AccountStats{
		TotalAccounts:    totalAccounts,
		ActiveAccounts:   activeAccounts,
		NewAccountsToday: newAccountsToday,
		TotalVests:       0, // Placeholder
		TotalSteem:       0, // Placeholder
		TotalSBD:         0, // Placeholder
	}, nil
}

// GetTopAccounts retrieves top accounts by various criteria
func (s *AccountService) GetTopAccounts(ctx context.Context, criteria string, limit int) ([]models.AccountSummary, error) {
	collection := s.db.Collection("account")

	sortField := "reputation"
	switch criteria {
	case "reputation":
		sortField = "reputation"
	case "vests":
		sortField = "vesting_shares"
	case "balance":
		sortField = "balance"
	case "posts":
		sortField = "post_count"
	default:
		sortField = "reputation"
	}

	findOptions := options.Find().
		SetSort(bson.M{sortField: -1}).
		SetLimit(int64(limit))

	cursor, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to find top accounts: %w", err)
	}
	defer cursor.Close(ctx)

	var accounts []models.AccountSummary
	for cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			s.logger.Error("Failed to decode account", utils.Error(err))
			continue
		}
		accounts = append(accounts, accountSummaryFromMap(doc))
	}

	return accounts, nil
}

// accountSummaryFromMap projects a raw account document (decoded as a generic
// map — sync writes raw RPC values that a rigid struct cannot decode) into an
// AccountSummary with tolerant type coercion.
func accountSummaryFromMap(m bson.M) models.AccountSummary {
	// Stubs not yet populated by the refresher carry no "name" field; _id is
	// always the account name.
	name := mapString(m, "name")
	if name == "" {
		name = mapString(m, "_id")
	}
	return models.AccountSummary{
		Name:          name,
		Reputation:    int64(mapFloat(m, "reputation")),
		VestingShares: mapFloat(m, "vesting_shares"),
		Balance:       mapFloat(m, "balance"),
		SBDBalance:    mapFloat(m, "sbd_balance"),
		PostCount:     int(mapFloat(m, "post_count")),
		// The refresher round-trips RPC values through JSON, so integer
		// counts land as BSON doubles; mapFloat coerces them tolerantly.
		CommentCount: int(mapFloat(m, "comment_count")),
		LastPost:     mapTime(m, "last_post"),
		Created:      mapTime(m, "created"),
	}
}

// mapString extracts a string field from a raw document
func mapString(m bson.M, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// mapFloat coerces a numeric field (BSON int32/int64/float64 or numeric string)
// to float64
func mapFloat(m bson.M, key string) float64 {
	switch v := m[key].(type) {
	case float64:
		return v
	case int32:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		var f float64
		if _, err := fmt.Sscanf(v, "%g", &f); err == nil {
			return f
		}
	}
	return 0
}

// mapTime extracts a datetime field, falling back to the zero time
func mapTime(m bson.M, key string) time.Time {
	if t, ok := m[key].(time.Time); ok {
		return t
	}
	return time.Time{}
}
