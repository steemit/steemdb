package services

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/steemit/steemdb/web/internal/database"
	"github.com/steemit/steemdb/web/pkg/steem"
	"github.com/steemit/steemdb/web/pkg/utils"
)

// SearchService handles search operations across blocks, transactions, and accounts
type SearchService struct {
	db          *database.MongoDB
	steemClient *steem.Client
	logger      utils.Logger
}

// NewSearchService creates a new search service
func NewSearchService(db *database.MongoDB, steemClient *steem.Client, logger utils.Logger) *SearchService {
	return &SearchService{
		db:          db,
		steemClient: steemClient,
		logger:      logger,
	}
}

// SearchResult represents a single search result item
type SearchResult struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	URL      string `json:"url"`
}

// Search performs a unified search across blocks, transactions, and accounts.
// The query type is auto-detected:
//   - All-numeric → block search
//   - 40-character hex string → transaction search
//   - Otherwise → account name search
//
// If a type filter is provided, only that type is searched.
func (s *SearchService) Search(ctx context.Context, query, searchType string) ([]SearchResult, error) {
	results := make([]SearchResult, 0)

	// Auto-detect search type if not specified
	if searchType == "" {
		searchType = s.detectType(query)
	}

	switch searchType {
	case "block":
		result, err := s.searchBlock(query)
		if err != nil {
			s.logger.Warn("Block search failed", utils.Error(err))
		} else if result != nil {
			results = append(results, *result)
		}

	case "transaction":
		result, err := s.searchTransaction(query)
		if err != nil {
			s.logger.Warn("Transaction search failed", utils.Error(err))
		} else if result != nil {
			results = append(results, *result)
		}

	case "account":
		fallthrough
	default:
		accountResults, err := s.searchAccounts(ctx, query)
		if err != nil {
			s.logger.Warn("Account search failed", utils.Error(err))
		} else {
			results = append(results, accountResults...)
		}
	}

	return results, nil
}

// detectType auto-detects the search type based on query format
func (s *SearchService) detectType(query string) string {
	// All-numeric → block
	if _, err := strconv.ParseInt(query, 10, 64); err == nil {
		return "block"
	}
	// 40-character hex → transaction
	if len(query) == 40 && isHex(query) {
		return "transaction"
	}
	return "account"
}

// searchBlock looks up a block by number via steem RPC
func (s *SearchService) searchBlock(query string) (*SearchResult, error) {
	blockNum, err := strconv.ParseInt(query, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid block number: %s", query)
	}

	block, err := s.steemClient.GetBlock(blockNum)
	if err != nil || block == nil {
		return nil, err
	}

	return &SearchResult{
		Type:  "block",
		ID:    query,
		Title: fmt.Sprintf("Block #%d", blockNum),
		URL:   fmt.Sprintf("/block/%d", blockNum),
	}, nil
}

// searchTransaction looks up a transaction by ID via steem RPC. The block
// number is surfaced in Subtitle so the UI can link to the block page.
func (s *SearchService) searchTransaction(query string) (*SearchResult, error) {
	tx, err := s.steemClient.GetTransaction(query)
	if err != nil || tx == nil {
		return nil, err
	}

	result := &SearchResult{
		Type:  "transaction",
		ID:    query,
		Title: query,
		URL:   fmt.Sprintf("/tx/%s", query),
	}
	// JSON numbers decode as float64; format without scientific notation.
	if blockNum, ok := tx["block_num"].(float64); ok {
		result.Subtitle = strconv.FormatInt(int64(blockNum), 10)
	}

	return result, nil
}

// searchAccounts searches the account collection by name prefix
func (s *SearchService) searchAccounts(ctx context.Context, query string) ([]SearchResult, error) {
	// Escape regex special characters in the query
	escaped := regexp.QuoteMeta(query)

	filter := bson.M{
		"name": primitive.Regex{Pattern: "^" + escaped, Options: "i"},
	}

	findOptions := options.Find().
		SetSort(bson.M{"reputation": -1}).
		SetLimit(5).
		SetProjection(bson.M{"name": 1})

	cursor, err := s.db.Collection("account").Find(ctx, filter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to search accounts: %w", err)
	}
	defer cursor.Close(ctx)

	results := make([]SearchResult, 0)
	for cursor.Next(ctx) {
		var account struct {
			Name string `bson:"name"`
		}
		if err := cursor.Decode(&account); err != nil {
			s.logger.Error("Failed to decode account", utils.Error(err))
			continue
		}
		results = append(results, SearchResult{
			Type:  "account",
			ID:    account.Name,
			Title: account.Name,
			URL:   fmt.Sprintf("/@%s", account.Name),
		})
	}

	return results, nil
}

// isHex checks if a string consists only of hex characters
func isHex(s string) bool {
	for _, c := range strings.ToLower(s) {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
