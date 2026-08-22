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

// WitnessVotingPoint represents the daily total voting weight across all
// snapshotted witnesses (from witness_history)
type WitnessVotingPoint struct {
	Date      string  `json:"date"`
	Votes     float64 `json:"votes"`
	Witnesses int     `json:"witnesses"`
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
// The operations collection has no timestamp field, so this runs in two
// phases: first aggregate per-day block_num boundaries from the blocks
// collection (blocks are strictly sequential in time), then count distinct
// non-empty trx_id values per block range. Virtual operations carry an empty
// trx_id and are excluded.
func (s *ChartsService) GetTransactionVolume(ctx context.Context, days int) ([]TransactionVolumePoint, error) {
	if days <= 0 {
		days = 30
	}

	cutoff := time.Now().AddDate(0, 0, -days)

	// Phase 1: per-day block_num boundaries from blocks
	blockPipeline := []bson.M{
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
				"first_block": bson.M{"$min": "$block_num"},
				"last_block":  bson.M{"$max": "$block_num"},
			},
		},
		{
			"$sort": bson.M{"_id": 1},
		},
	}

	cursor, err := s.db.Collection("blocks").Aggregate(ctx, blockPipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate block day ranges: %w", err)
	}
	defer cursor.Close(ctx)

	// Phase 2: distinct real transactions per block range
	results := make([]TransactionVolumePoint, 0)
	for cursor.Next(ctx) {
		var dayRange struct {
			ID         string `bson:"_id"`
			FirstBlock int64  `bson:"first_block"`
			LastBlock  int64  `bson:"last_block"`
		}
		if err := cursor.Decode(&dayRange); err != nil {
			s.logger.Error("Failed to decode block day range", utils.Error(err))
			continue
		}

		filter := bson.M{
			"block_num": bson.M{"$gte": dayRange.FirstBlock, "$lte": dayRange.LastBlock},
			"trx_id":    bson.M{"$ne": ""},
		}
		trxIDs, err := s.db.Collection("operations").Distinct(ctx, "trx_id", filter)
		if err != nil {
			s.logger.Error("Failed to count transactions for day", utils.String("date", dayRange.ID), utils.Error(err))
			continue
		}

		results = append(results, TransactionVolumePoint{
			Date:         dayRange.ID,
			Transactions: int64(len(trxIDs)),
		})
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate block day ranges: %w", err)
	}

	return results, nil
}

// GetWitnessVoting returns the daily total witness voting weight from the
// witness_history snapshots maintained by steemdb-sync's refresher process.
// Snapshot ids are "owner|YYYYMMDD", so the day bucket is extracted by
// splitting on "|" — the YYYYMMDD strings sort lexicographically.
func (s *ChartsService) GetWitnessVoting(ctx context.Context, days int) ([]WitnessVotingPoint, error) {
	if days <= 0 {
		days = 30
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format("20060102")

	pipeline := []bson.M{
		{
			"$project": bson.M{
				"parts": bson.M{"$split": []interface{}{"$_id", "|"}},
				"votes": 1,
			},
		},
		{
			"$project": bson.M{
				"date":  bson.M{"$arrayElemAt": []interface{}{"$parts", 1}},
				"votes": 1,
			},
		},
		{
			"$match": bson.M{
				"date": bson.M{"$gte": cutoff},
			},
		},
		{
			"$group": bson.M{
				"_id":       "$date",
				"votes":     bson.M{"$sum": "$votes"},
				"witnesses": bson.M{"$sum": 1},
			},
		},
		{
			"$sort": bson.M{"_id": 1},
		},
	}

	cursor, err := s.db.Collection("witness_history").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate witness voting: %w", err)
	}
	defer cursor.Close(ctx)

	results := make([]WitnessVotingPoint, 0)
	for cursor.Next(ctx) {
		var row struct {
			ID        string  `bson:"_id"`
			Votes     float64 `bson:"votes"`
			Witnesses int     `bson:"witnesses"`
		}
		if err := cursor.Decode(&row); err != nil {
			s.logger.Error("Failed to decode witness voting row", utils.Error(err))
			continue
		}
		results = append(results, WitnessVotingPoint{
			Date:      row.ID,
			Votes:     row.Votes,
			Witnesses: row.Witnesses,
		})
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate witness voting rows: %w", err)
	}

	return results, nil
}
