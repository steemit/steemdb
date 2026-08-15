package services

import (
	"context"
	"fmt"
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
	redis  *database.Redis
	logger utils.Logger
}

// NewAccountService creates a new account service
func NewAccountService(db *database.MongoDB, redis *database.Redis, logger utils.Logger) *AccountService {
	return &AccountService{
		db:     db,
		redis:  redis,
		logger: logger,
	}
}

// GetAccount retrieves an account by name
func (s *AccountService) GetAccount(ctx context.Context, name string) (*models.Account, error) {
	collection := s.db.Collection("account")
	var account models.Account

	err := collection.FindOne(ctx, bson.M{"name": name}).Decode(&account)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("account not found: %s", name)
		}
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	return &account, nil
}

// GetAccounts retrieves multiple accounts with pagination
func (s *AccountService) GetAccounts(ctx context.Context, params models.PaginationParams, sortParams models.SortParams) (*models.AccountSearchResult, error) {
	collection := s.db.Collection("account")

	// Build sort options
	sortField := "reputation"
	sortOrder := -1
	if sortParams.SortBy != "" {
		sortField = sortParams.SortBy
	}
	if sortParams.SortOrder == "asc" {
		sortOrder = 1
	}

	// Count total documents
	total, err := collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to count accounts: %w", err)
	}

	// Calculate pagination
	skip := (params.Page - 1) * params.PageSize
	findOptions := options.Find().
		SetSort(bson.M{sortField: sortOrder}).
		SetSkip(int64(skip)).
		SetLimit(int64(params.PageSize))

	cursor, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to find accounts: %w", err)
	}
	defer cursor.Close(ctx)

	var accounts []models.AccountSummary
	for cursor.Next(ctx) {
		var account models.Account
		if err := cursor.Decode(&account); err != nil {
			s.logger.Error("Failed to decode account", utils.Error(err))
			continue
		}

		summary := models.AccountSummary{
			Name:          account.Name,
			Reputation:    account.Reputation,
			VestingShares: account.VestingShares,
			Balance:       account.Balance,
			SBDBalance:    account.SBDBalance,
			PostCount:     account.PostCount,
			LastPost:      account.LastPost,
			Created:       account.Created,
		}
		accounts = append(accounts, summary)
	}

	return &models.AccountSearchResult{
		Accounts: accounts,
		Total:    total,
		Page:     params.Page,
		PageSize: params.PageSize,
	}, nil
}

// SearchAccounts searches for accounts by name pattern
func (s *AccountService) SearchAccounts(ctx context.Context, query string, limit int) (*models.AccountSearchResult, error) {
	collection := s.db.Collection("account")

	// Build search filter
	filter := bson.M{
		"name": bson.M{
			"$regex":   fmt.Sprintf("^%s", query),
			"$options": "i",
		},
	}

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
		var account models.Account
		if err := cursor.Decode(&account); err != nil {
			s.logger.Error("Failed to decode account", utils.Error(err))
			continue
		}

		summary := models.AccountSummary{
			Name:          account.Name,
			Reputation:    account.Reputation,
			VestingShares: account.VestingShares,
			Balance:       account.Balance,
			SBDBalance:    account.SBDBalance,
			PostCount:     account.PostCount,
			LastPost:      account.LastPost,
			Created:       account.Created,
		}
		accounts = append(accounts, summary)
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

	cursor, err := s.db.Collection("blocks").Find(ctx,
		bson.M{"block_num": bson.M{"$in": nums}},
		options.Find().SetProjection(bson.M{"block_num": 1, "timestamp": 1}),
	)
	if err != nil {
		s.logger.Warn("Failed to fetch block times", utils.Error(err))
		return result
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var block struct {
			BlockNum  int64     `bson:"block_num"`
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
		var account models.Account
		if err := cursor.Decode(&account); err != nil {
			s.logger.Error("Failed to decode account", utils.Error(err))
			continue
		}

		summary := models.AccountSummary{
			Name:          account.Name,
			Reputation:    account.Reputation,
			VestingShares: account.VestingShares,
			Balance:       account.Balance,
			SBDBalance:    account.SBDBalance,
			PostCount:     account.PostCount,
			LastPost:      account.LastPost,
			Created:       account.Created,
		}
		accounts = append(accounts, summary)
	}

	return accounts, nil
}
