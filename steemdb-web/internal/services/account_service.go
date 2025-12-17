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

// GetAccountHistory retrieves account history
func (s *AccountService) GetAccountHistory(ctx context.Context, name string, params models.PaginationParams) (*models.AccountSearchResult, error) {
	// For now, return empty result as placeholder
	return &models.AccountSearchResult{
		Accounts: []models.AccountSummary{},
		Total:    0,
		Page:     params.Page,
		PageSize: params.PageSize,
	}, nil
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
