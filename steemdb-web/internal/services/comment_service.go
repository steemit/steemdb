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

// CommentService handles comment/post-related operations
type CommentService struct {
	db     *database.MongoDB
	redis  *database.Redis
	logger utils.Logger
}

// NewCommentService creates a new comment service
func NewCommentService(db *database.MongoDB, redis *database.Redis, logger utils.Logger) *CommentService {
	return &CommentService{
		db:     db,
		redis:  redis,
		logger: logger,
	}
}

// GetPosts retrieves posts (depth=0) with pagination
func (s *CommentService) GetPosts(ctx context.Context, params models.PaginationParams, sortParams models.SortParams) (*models.CommentSearchResult, error) {
	collection := s.db.Collection("comment")

	query := bson.M{"depth": 0}

	// Build sort options
	sort := bson.M{}
	if sortParams.SortBy != "" {
		sortField := sortParams.SortBy
		if sortParams.SortOrder == "desc" {
			sort[sortField] = -1
		} else {
			sort[sortField] = 1
		}
	} else {
		sort["created"] = -1 // Default: newest first
	}

	// Count total
	total, err := collection.CountDocuments(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to count posts: %w", err)
	}

	// Find with pagination
	findOptions := options.Find().
		SetSort(sort).
		SetSkip(int64((params.Page - 1) * params.PageSize)).
		SetLimit(int64(params.PageSize))

	cursor, err := collection.Find(ctx, query, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to find posts: %w", err)
	}
	defer cursor.Close(ctx)

	var posts []models.Comment
	if err := cursor.All(ctx, &posts); err != nil {
		return nil, fmt.Errorf("failed to decode posts: %w", err)
	}

	totalPages := int((total + int64(params.PageSize) - 1) / int64(params.PageSize))

	return &models.CommentSearchResult{
		Data:       posts,
		Total:      total,
		Page:       params.Page,
		PageSize:   params.PageSize,
		TotalPages: totalPages,
	}, nil
}

// GetPost retrieves a single post by author and permlink
func (s *CommentService) GetPost(ctx context.Context, author, permlink string) (*models.Comment, error) {
	collection := s.db.Collection("comment")
	id := author + "/" + permlink

	var comment models.Comment
	err := collection.FindOne(ctx, bson.M{"_id": id}).Decode(&comment)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("post not found: %s/%s", author, permlink)
		}
		return nil, fmt.Errorf("failed to get post: %w", err)
	}

	return &comment, nil
}

// GetPostsByDate retrieves posts by date and optional tag
func (s *CommentService) GetPostsByDate(ctx context.Context, date time.Time, tag string, sortBy string) ([]models.Comment, error) {
	collection := s.db.Collection("comment")

	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	matchStage := bson.M{
		"depth": 0,
		"created": bson.M{
			"$gte": startOfDay,
			"$lt":  endOfDay,
		},
	}

	if tag != "" && tag != "all" {
		matchStage["category"] = tag
	}

	// Build aggregation pipeline
	pipeline := []bson.M{
		{"$match": matchStage},
		{
			"$project": bson.M{
				"_id":                        1,
				"author":                     1,
				"permlink":                   1,
				"title":                      1,
				"body":                       1,
				"json_metadata":              1,
				"parent_author":              1,
				"parent_permlink":            1,
				"category":                   1,
				"depth":                      1,
				"children":                   1,
				"created":                    1,
				"last_update":                1,
				"cashout_time":               1,
				"pending_payout_value":       1,
				"total_payout_value":         1,
				"net_votes":                  1,
				"block_num":                  1,
				"scanned":                    1,
				"author_lower":               1,
				"category_lower":             1,
				"date_idx":                   1,
				"url":                        1,
				"author_reputation":          1,
				"total_pending_payout_value": 1,
				"active_votes":               1,
				"combined_payout": bson.M{
					"$add": []interface{}{"$total_payout_value", "$pending_payout_value"},
				},
			},
		},
	}

	// Build sort stage
	sortStage := bson.M{}
	switch sortBy {
	case "votes":
		sortStage["net_votes"] = -1
	default:
		sortStage["combined_payout"] = -1
	}
	pipeline = append(pipeline, bson.M{"$sort": sortStage})
	pipeline = append(pipeline, bson.M{"$limit": 100})

	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate posts by date: %w", err)
	}
	defer cursor.Close(ctx)

	var posts []models.Comment
	if err := cursor.All(ctx, &posts); err != nil {
		return nil, fmt.Errorf("failed to decode posts: %w", err)
	}

	return posts, nil
}

// GetPostReplies retrieves replies to a post
func (s *CommentService) GetPostReplies(ctx context.Context, author, permlink string) ([]models.Comment, error) {
	collection := s.db.Collection("comment")

	query := bson.M{
		"parent_author":   author,
		"parent_permlink": permlink,
	}

	findOptions := options.Find().
		SetSort(bson.M{"created": -1})

	cursor, err := collection.Find(ctx, query, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to find replies: %w", err)
	}
	defer cursor.Close(ctx)

	var replies []models.Comment
	if err := cursor.All(ctx, &replies); err != nil {
		return nil, fmt.Errorf("failed to decode replies: %w", err)
	}

	return replies, nil
}

// GetPostVotes retrieves votes for a post
func (s *CommentService) GetPostVotes(ctx context.Context, author, permlink string) ([]models.Vote, error) {
	collection := s.db.Collection("vote")

	query := bson.M{
		"author":   author,
		"permlink": permlink,
	}

	findOptions := options.Find().
		SetSort(bson.M{"_ts": 1}) // Sort by time ascending

	cursor, err := collection.Find(ctx, query, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to find votes: %w", err)
	}
	defer cursor.Close(ctx)

	var votes []models.Vote
	if err := cursor.All(ctx, &votes); err != nil {
		return nil, fmt.Errorf("failed to decode votes: %w", err)
	}

	return votes, nil
}

// GetPostReblogs retrieves reblogs for a post
func (s *CommentService) GetPostReblogs(ctx context.Context, author, permlink string) ([]models.Reblog, error) {
	collection := s.db.Collection("reblog")

	query := bson.M{
		"author":   author,
		"permlink": permlink,
	}

	findOptions := options.Find().
		SetSort(bson.M{"_ts": 1}) // Sort by time ascending

	cursor, err := collection.Find(ctx, query, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to find reblogs: %w", err)
	}
	defer cursor.Close(ctx)

	var reblogs []models.Reblog
	if err := cursor.All(ctx, &reblogs); err != nil {
		return nil, fmt.Errorf("failed to decode reblogs: %w", err)
	}

	return reblogs, nil
}
