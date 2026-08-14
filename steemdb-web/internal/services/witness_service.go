package services

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/steemit/steemdb/web/internal/database"
	"github.com/steemit/steemdb/web/pkg/steem"
	"github.com/steemit/steemdb/web/pkg/utils"
)

// WitnessService handles witness data operations via steem RPC
type WitnessService struct {
	steemClient *steem.Client
	logger      utils.Logger
}

// NewWitnessService creates a new witness service
func NewWitnessService(db *database.MongoDB, steemClient *steem.Client, logger utils.Logger) *WitnessService {
	return &WitnessService{
		steemClient: steemClient,
		logger:      logger,
	}
}

// WitnessListResult represents a paginated list of witnesses
type WitnessListResult struct {
	Witnesses  []steem.Witness `json:"witnesses"`
	Total      int64           `json:"total"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
	TotalPages int             `json:"total_pages"`
}

// GetWitnesses retrieves a paginated, sorted list of witnesses from steem RPC.
// The condenser_api.get_witnesses_by_vote call returns at most 100 witnesses,
// so pagination is done in-memory on the top-100 set.
func (s *WitnessService) GetWitnesses(ctx context.Context, page, pageSize int, sortBy, sortOrder string) (*WitnessListResult, error) {
	// Fetch top 100 witnesses (RPC maximum)
	witnesses, err := s.steemClient.GetWitnessesByVote("", 100)
	if err != nil {
		return nil, fmt.Errorf("failed to get witnesses: %w", err)
	}

	total := len(witnesses)

	// Sort in-memory
	sortWitnesses(witnesses, sortBy, sortOrder)

	// Paginate in-memory
	start := (page - 1) * pageSize
	if start >= total {
		return &WitnessListResult{
			Witnesses:  []steem.Witness{},
			Total:      int64(total),
			Page:       page,
			PageSize:   pageSize,
			TotalPages: (total + pageSize - 1) / pageSize,
		}, nil
	}

	end := start + pageSize
	if end > total {
		end = total
	}

	return &WitnessListResult{
		Witnesses:  witnesses[start:end],
		Total:      int64(total),
		Page:       page,
		PageSize:   pageSize,
		TotalPages: (total + pageSize - 1) / pageSize,
	}, nil
}

// GetTopWitnesses retrieves the top N witnesses by vote count
func (s *WitnessService) GetTopWitnesses(ctx context.Context, limit int) ([]steem.Witness, error) {
	witnesses, err := s.steemClient.GetWitnessesByVote("", limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top witnesses: %w", err)
	}
	return witnesses, nil
}

// GetWitness retrieves a single witness by account name
func (s *WitnessService) GetWitness(ctx context.Context, account string) (*steem.Witness, error) {
	witness, err := s.steemClient.GetWitnessByAccount(account)
	if err != nil {
		return nil, fmt.Errorf("failed to get witness: %w", err)
	}
	if witness == nil {
		return nil, fmt.Errorf("witness not found: %s", account)
	}
	return witness, nil
}

// sortWitnesses sorts a slice of witnesses by the given field and order.
// Supported sort fields: votes, total_missed, running_version, owner.
// Default sort field is "votes".
func sortWitnesses(witnesses []steem.Witness, sortBy, sortOrder string) {
	desc := sortOrder != "asc"

	switch sortBy {
	case "total_missed":
		sort.SliceStable(witnesses, func(i, j int) bool {
			if desc {
				return witnesses[i].TotalMissed > witnesses[j].TotalMissed
			}
			return witnesses[i].TotalMissed < witnesses[j].TotalMissed
		})
	case "owner":
		sort.SliceStable(witnesses, func(i, j int) bool {
			if desc {
				return witnesses[i].Owner > witnesses[j].Owner
			}
			return witnesses[i].Owner < witnesses[j].Owner
		})
	case "votes":
		fallthrough
	default:
		// votes is a string representation of a large integer; compare numerically
		sort.SliceStable(witnesses, func(i, j int) bool {
			vi, _ := strconv.ParseInt(witnesses[i].Votes, 10, 64)
			vj, _ := strconv.ParseInt(witnesses[j].Votes, 10, 64)
			if desc {
				return vi > vj
			}
			return vi < vj
		})
	}
}
