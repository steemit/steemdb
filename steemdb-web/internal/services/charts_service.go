package services

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/steemit/steemdb/web/internal/database"
	"github.com/steemit/steemdb/web/pkg/utils"
)

// ChartsService handles chart data aggregations
type ChartsService struct {
	db     *database.MongoDB
	logger utils.Logger
}

// NewChartsService creates a new charts service
func NewChartsService(db *database.MongoDB, logger utils.Logger) *ChartsService {
	return &ChartsService{
		db:     db,
		logger: logger,
	}
}

// ChartPoint represents a single data point in a time-series chart
type ChartPoint struct {
	Date string `json:"date"`
}

// AccountGrowthPoint represents daily account creation count
type AccountGrowthPoint struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

// BlockProductionPoint represents daily block production count
type BlockProductionPoint struct {
	Date   string `json:"date"`
	Blocks int64  `json:"blocks"`
}

// TransactionVolumePoint represents daily transaction count
type TransactionVolumePoint struct {
	Date         string `json:"date"`
	Transactions int64  `json:"transactions"`
}

// GetAccountGrowth returns daily new account creation counts for the past N days
func (s *ChartsService) GetAccountGrowth(ctx context.Context, days int) ([]AccountGrowthPoint, error) {
	if days <= 0 {
		days = 30
	}

	cutoff := time.Now().AddDate(0, 0, -days)

	pipeline := []bson.M{
		{
			"$match": bson.M{
				"created": bson.M{"$gte": cutoff},
			},
		},
		{
			"$group": bson.M{
				"_id": bson.M{
					"$dateToString": bson.M{
						"format": "%Y-%m-%d",
						"date":   "$created",
					},
				},
				"count": bson.M{"$sum": 1},
			},
		},
		{
			"$sort": bson.M{"_id": 1},
		},
	}

	cursor, err := s.db.Collection("account").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate account growth: %w", err)
	}
	defer cursor.Close(ctx)

	results := make([]AccountGrowthPoint, 0)
	for cursor.Next(ctx) {
		var row struct {
			ID    string `bson:"_id"`
			Count int64  `bson:"count"`
		}
		if err := cursor.Decode(&row); err != nil {
			s.logger.Error("Failed to decode account growth row", utils.Error(err))
			continue
		}
		results = append(results, AccountGrowthPoint{
			Date:  row.ID,
			Count: row.Count,
		})
	}

	return results, nil
}

// GetBlockProduction returns daily block production counts for the past N days
func (s *ChartsService) GetBlockProduction(ctx context.Context, days int) ([]BlockProductionPoint, error) {
	if days <= 0 {
		days = 7
	}

	cutoff := time.Now().AddDate(0, 0, -days)

	pipeline := []bson.M{
		{
			"$match": bson.M{
				"timestamp": bson.M{"$gte": cutoff},
			},
		},
		{
			"$group": bson.M{
				"_id": bson.M{
					"$dateToString": bson.M{
						"format": "%Y-%m-%d",
						"date":   "$timestamp",
					},
				},
				"blocks": bson.M{"$sum": 1},
			},
		},
		{
			"$sort": bson.M{"_id": 1},
		},
	}

	cursor, err := s.db.Collection("blocks").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate block production: %w", err)
	}
	defer cursor.Close(ctx)

	results := make([]BlockProductionPoint, 0)
	for cursor.Next(ctx) {
		var row struct {
			ID     string `bson:"_id"`
			Blocks int64  `bson:"blocks"`
		}
		if err := cursor.Decode(&row); err != nil {
			s.logger.Error("Failed to decode block production row", utils.Error(err))
			continue
		}
		results = append(results, BlockProductionPoint{
			Date:   row.ID,
			Blocks: row.Blocks,
		})
	}

	return results, nil
}

// GetTransactionVolume returns daily transaction counts for the past N days.
// NOTE: blocks.transaction_count is currently hardcoded to 0 by the sync
// batcher, so we aggregate from the operations collection by counting distinct
// trx_id values per day.
func (s *ChartsService) GetTransactionVolume(ctx context.Context, days int) ([]TransactionVolumePoint, error) {
	if days <= 0 {
		days = 30
	}

	cutoff := time.Now().AddDate(0, 0, -days)

	pipeline := []bson.M{
		{
			"$match": bson.M{
				"timestamp": bson.M{"$gte": cutoff},
			},
		},
		{
			"$group": bson.M{
				"_id": bson.M{
					"$dateToString": bson.M{
						"format": "%Y-%m-%d",
						"date":   "$timestamp",
					},
				},
				"transactions": bson.M{"$addToSet": "$trx_id"},
			},
		},
		{
			"$project": bson.M{
				"_id":          1,
				"transactions": bson.M{"$size": "$transactions"},
			},
		},
		{
			"$sort": bson.M{"_id": 1},
		},
	}

	cursor, err := s.db.Collection("operations").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate transaction volume: %w", err)
	}
	defer cursor.Close(ctx)

	results := make([]TransactionVolumePoint, 0)
	for cursor.Next(ctx) {
		var row struct {
			ID           string `bson:"_id"`
			Transactions int64  `bson:"transactions"`
		}
		if err := cursor.Decode(&row); err != nil {
			s.logger.Error("Failed to decode transaction volume row", utils.Error(err))
			continue
		}
		results = append(results, TransactionVolumePoint{
			Date:         row.ID,
			Transactions: row.Transactions,
		})
	}

	return results, nil
}

// GetWitnessVoting returns witness voting distribution data.
// TODO: The blocks collection does not store witness information (the sync
// batcher does not populate the witness field), so witness voting aggregation
// is not currently possible. This returns an empty array until a witness
// refresher is added to steemdb-sync.
func (s *ChartsService) GetWitnessVoting(ctx context.Context) ([]interface{}, error) {
	return []interface{}{}, nil
}
