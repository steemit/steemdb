package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/steemit/steemdb/web/internal/database"
	"github.com/steemit/steemdb/web/pkg/steem"
	"github.com/steemit/steemdb/web/pkg/utils"
)

// WitnessService handles witness data operations. Reads the local `witness`
// collection maintained by steemdb-sync's refresher process first and falls
// back to live RPC when the collection is empty (e.g. refresher not deployed).
type WitnessService struct {
	db          *database.MongoDB
	steemClient *steem.Client
	logger      utils.Logger
}

// NewWitnessService creates a new witness service
func NewWitnessService(db *database.MongoDB, steemClient *steem.Client, logger utils.Logger) *WitnessService {
	return &WitnessService{
		db:          db,
		steemClient: steemClient,
		logger:      logger,
	}
}

// WitnessListResult represents a paginated list of witnesses
type WitnessListResult struct {
	Witnesses  []map[string]interface{} `json:"witnesses"`
	Total      int64                    `json:"total"`
	Page       int                      `json:"page"`
	PageSize   int                      `json:"page_size"`
	TotalPages int                      `json:"total_pages"`
}

// GetWitnesses retrieves a paginated, sorted list of witnesses from the local
// collection (fallback: steem RPC top-100 with in-memory paging).
func (s *WitnessService) GetWitnesses(ctx context.Context, page, pageSize int, sortBy, sortOrder string) (*WitnessListResult, error) {
	// MongoDB-first path: the refresher keeps the top-100 witnesses sorted by
	// votes as float64, so all sort fields are directly indexable/sortable.
	col := s.db.Collection("witness")
	total, err := col.CountDocuments(ctx, bson.M{})
	if err == nil && total > 0 {
		sortField := witnessSortField(sortBy)
		direction := -1
		if sortOrder == "asc" {
			direction = 1
		}

		findOptions := options.Find().
			SetSort(bson.M{sortField: direction}).
			SetSkip(int64((page - 1) * pageSize)).
			SetLimit(int64(pageSize))

		cursor, err := col.Find(ctx, bson.M{}, findOptions)
		if err == nil {
			defer cursor.Close(ctx)
			witnesses := make([]map[string]interface{}, 0, pageSize)
			if err := cursor.All(ctx, &witnesses); err == nil {
				return &WitnessListResult{
					Witnesses:  witnesses,
					Total:      total,
					Page:       page,
					PageSize:   pageSize,
					TotalPages: int((total + int64(pageSize) - 1) / int64(pageSize)),
				}, nil
			}
			s.logger.Warn("Failed to query witness collection, falling back to RPC", utils.Error(err))
		} else {
			s.logger.Warn("Failed to query witness collection, falling back to RPC", utils.Error(err))
		}
	}

	// RPC fallback: condenser_api.get_witnesses_by_vote returns at most 100
	// witnesses, so pagination happens in-memory on the top-100 set.
	witnesses, err := s.steemClient.GetWitnessesByVote("", 100)
	if err != nil {
		return nil, fmt.Errorf("failed to get witnesses: %w", err)
	}

	maps := rpcWitnessesToMaps(witnesses)
	total64 := int64(len(maps))
	sortWitnessMaps(maps, sortBy, sortOrder)

	start := (page - 1) * pageSize
	if start >= len(maps) {
		return &WitnessListResult{
			Witnesses:  []map[string]interface{}{},
			Total:      total64,
			Page:       page,
			PageSize:   pageSize,
			TotalPages: int((total64 + int64(pageSize) - 1) / int64(pageSize)),
		}, nil
	}

	end := start + pageSize
	if end > len(maps) {
		end = len(maps)
	}

	return &WitnessListResult{
		Witnesses:  maps[start:end],
		Total:      total64,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: int((total64 + int64(pageSize) - 1) / int64(pageSize)),
	}, nil
}

// GetTopWitnesses retrieves the top N witnesses by vote count (local
// collection first, RPC fallback).
func (s *WitnessService) GetTopWitnesses(ctx context.Context, limit int) ([]map[string]interface{}, error) {
	col := s.db.Collection("witness")
	findOptions := options.Find().
		SetSort(bson.M{"votes": -1}).
		SetLimit(int64(limit))

	cursor, err := col.Find(ctx, bson.M{}, findOptions)
	if err == nil {
		defer cursor.Close(ctx)
		witnesses := make([]map[string]interface{}, 0, limit)
		if err := cursor.All(ctx, &witnesses); err == nil && len(witnesses) > 0 {
			return witnesses, nil
		}
		if err != nil {
			s.logger.Warn("Failed to query witness collection, falling back to RPC", utils.Error(err))
		}
	} else {
		s.logger.Warn("Failed to query witness collection, falling back to RPC", utils.Error(err))
	}

	witnesses, err := s.steemClient.GetWitnessesByVote("", limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top witnesses: %w", err)
	}
	return rpcWitnessesToMaps(witnesses), nil
}

// GetWitness retrieves a single witness by account name (local collection
// first, RPC fallback).
func (s *WitnessService) GetWitness(ctx context.Context, account string) (map[string]interface{}, error) {
	var witness map[string]interface{}
	err := s.db.Collection("witness").FindOne(ctx, bson.M{"_id": account}).Decode(&witness)
	if err == nil {
		return witness, nil
	}

	// Not found locally (or query error) — try live RPC
	rpcWitness, err := s.steemClient.GetWitnessByAccount(account)
	if err != nil {
		return nil, fmt.Errorf("failed to get witness: %w", err)
	}
	if rpcWitness == nil {
		return nil, fmt.Errorf("witness not found: %s", account)
	}
	return rpcWitnessToMap(rpcWitness), nil
}

// witnessSortField maps the API sort parameter to a mongo sort field.
func witnessSortField(sortBy string) string {
	switch sortBy {
	case "total_missed":
		return "total_missed"
	case "owner":
		return "owner"
	default:
		return "votes"
	}
}

// sortWitnessMaps sorts RPC-fallback witness maps by the given field.
func sortWitnessMaps(witnesses []map[string]interface{}, sortBy, sortOrder string) {
	desc := sortOrder != "asc"

	switch sortBy {
	case "total_missed":
		sort.SliceStable(witnesses, func(i, j int) bool {
			vi, _ := witnesses[i]["total_missed"].(float64)
			vj, _ := witnesses[j]["total_missed"].(float64)
			if desc {
				return vi > vj
			}
			return vi < vj
		})
	case "owner":
		sort.SliceStable(witnesses, func(i, j int) bool {
			vi, _ := witnesses[i]["owner"].(string)
			vj, _ := witnesses[j]["owner"].(string)
			if desc {
				return vi > vj
			}
			return vi < vj
		})
	default: // votes
		sort.SliceStable(witnesses, func(i, j int) bool {
			vi := toFloat(witnesses[i]["votes"])
			vj := toFloat(witnesses[j]["votes"])
			if desc {
				return vi > vj
			}
			return vi < vj
		})
	}
}

// toFloat coerces a JSON value (string or number) to float64.
func toFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case string:
		var f float64
		if _, err := fmt.Sscanf(val, "%g", &f); err == nil {
			return f
		}
	}
	return 0
}

// rpcWitnessesToMaps converts typed RPC witnesses into generic maps so both
// the MongoDB and RPC paths return the same document shape.
func rpcWitnessesToMaps(witnesses []steem.Witness) []map[string]interface{} {
	maps := make([]map[string]interface{}, 0, len(witnesses))
	for i := range witnesses {
		if m := rpcWitnessToMap(&witnesses[i]); m != nil {
			maps = append(maps, m)
		}
	}
	return maps
}

// rpcWitnessToMap converts one typed RPC witness via a JSON round-trip.
func rpcWitnessToMap(w *steem.Witness) map[string]interface{} {
	raw, err := json.Marshal(w)
	if err != nil {
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}
