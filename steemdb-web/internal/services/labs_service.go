package services

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/steemit/steemdb/web/internal/database"
	"github.com/steemit/steemdb/web/internal/models"
	"github.com/steemit/steemdb/web/pkg/steem"
	"github.com/steemit/steemdb/web/pkg/utils"
)

// LabsService handles labs-related operations
type LabsService struct {
	db          *database.MongoDB
	steemClient *steem.Client
	logger      utils.Logger
}

// NewLabsService creates a new labs service
func NewLabsService(db *database.MongoDB, steemClient *steem.Client, logger utils.Logger) *LabsService {
	return &LabsService{
		db:          db,
		steemClient: steemClient,
		logger:      logger,
	}
}

// GetPowerUps retrieves power up statistics
func (s *LabsService) GetPowerUps(ctx context.Context, filter string) ([]models.PowerUp, error) {
	collection := s.db.Collection("vesting_deposit")

	days := 30
	switch filter {
	case "week":
		days = 7
	case "day":
		days = 1
	}

	startTime := time.Now().AddDate(0, 0, -days)

	pipeline := []bson.M{
		{
			"$match": bson.M{
				"_ts": bson.M{
					"$gte": startTime,
				},
			},
		},
		{
			"$project": bson.M{
				"date": bson.M{
					"doy":   bson.M{"$dayOfYear": "$_ts"},
					"year":  bson.M{"$year": "$_ts"},
					"month": bson.M{"$month": "$_ts"},
					"day":   bson.M{"$dayOfMonth": "$_ts"},
				},
				"to":     "$to",
				"amount": "$amount",
				"from":   "$from",
			},
		},
		{
			"$group": bson.M{
				"_id": bson.M{
					"user": bson.M{
						"$cond": bson.A{
							bson.M{"$eq": bson.A{"$to", ""}},
							"$from",
							"$to",
						},
					},
				},
				"count":     bson.M{"$sum": 1},
				"instances": bson.M{"$addToSet": "$amount"},
			},
		},
		{
			"$limit": 1000,
		},
		{
			"$lookup": bson.M{
				"from":         "account",
				"localField":   "_id.user",
				"foreignField": "name",
				"as":           "account",
			},
		},
	}

	cursor, err := collection.Aggregate(ctx, pipeline, options.Aggregate().SetAllowDiskUse(true))
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate power ups: %w", err)
	}
	defer cursor.Close(ctx)

	var results []struct {
		ID        bson.M           `bson:"_id"`
		Count     int              `bson:"count"`
		Instances []string         `bson:"instances"`
		Account   []models.Account `bson:"account"`
	}

	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("failed to decode power ups: %w", err)
	}

	powerUps := make([]models.PowerUp, 0, len(results))
	for _, r := range results {
		user, _ := r.ID["user"].(string)
		total := 0.0
		for _, inst := range r.Instances {
			// Parse amount string (e.g., "100.000 STEEM")
			parts := strings.Fields(inst)
			if len(parts) > 0 {
				if val, err := strconv.ParseFloat(parts[0], 64); err == nil {
					total += val
				}
			}
		}

		var account *models.Account
		if len(r.Account) > 0 {
			account = &r.Account[0]
		}

		powerUps = append(powerUps, models.PowerUp{
			User:      user,
			Count:     r.Count,
			Total:     total,
			Instances: r.Instances,
			Account:   account,
		})
	}

	// Sort by total descending
	sort.Slice(powerUps, func(i, j int) bool {
		return powerUps[i].Total > powerUps[j].Total
	})

	return powerUps, nil
}

// GetPowerDowns retrieves power down statistics
func (s *LabsService) GetPowerDowns(ctx context.Context) (*models.PowerDown, error) {
	// Get blockchain properties
	props, err := s.steemClient.GetDynamicGlobalProperties()
	if err != nil {
		return nil, fmt.Errorf("failed to get props: %w", err)
	}

	// Parse amounts
	current := parseAmount(props.CurrentSupply)
	vesting := parseAmount(props.TotalVestingFundSteem)
	liquid := current - vesting

	powerDownProps := models.PowerDownProps{
		Current: current,
		Vesting: vesting,
		Liquid:  liquid,
	}

	// Get upcoming power downs from accounts
	accountCollection := s.db.Collection("account")
	today := time.Now().Truncate(24 * time.Hour)

	upcomingPipeline := []bson.M{
		{
			"$match": bson.M{
				"next_vesting_withdrawal": bson.M{
					"$gte": today,
				},
				"vesting_withdraw_rate": bson.M{
					"$gt": 0,
				},
			},
		},
		{
			"$group": bson.M{
				"_id": bson.M{
					"doy":   bson.M{"$dayOfYear": "$next_vesting_withdrawal"},
					"year":  bson.M{"$year": "$next_vesting_withdrawal"},
					"month": bson.M{"$month": "$next_vesting_withdrawal"},
					"day":   bson.M{"$dayOfMonth": "$next_vesting_withdrawal"},
					"dow":   bson.M{"$dayOfWeek": "$next_vesting_withdrawal"},
				},
				"count":     bson.M{"$sum": 1},
				"withdrawn": bson.M{"$sum": "$vesting_withdraw_rate"},
			},
		},
		{
			"$sort": bson.M{
				"_id.year": 1,
				"_id.doy":  1,
			},
		},
		{
			"$limit": 7,
		},
	}

	cursor, err := accountCollection.Aggregate(ctx, upcomingPipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate upcoming power downs: %w", err)
	}
	defer cursor.Close(ctx)

	var upcomingResults []struct {
		ID        bson.M  `bson:"_id"`
		Count     int     `bson:"count"`
		Withdrawn float64 `bson:"withdrawn"`
	}
	if err := cursor.All(ctx, &upcomingResults); err != nil {
		return nil, fmt.Errorf("failed to decode upcoming: %w", err)
	}

	upcoming := make([]models.PowerDownDay, 0, len(upcomingResults))
	upcomingTotal := 0.0
	for _, r := range upcomingResults {
		id := r.ID
		upcoming = append(upcoming, models.PowerDownDay{
			DayOfYear: getInt(id["doy"]),
			Year:      getInt(id["year"]),
			Month:     getInt(id["month"]),
			Day:       getInt(id["day"]),
			DayOfWeek: getInt(id["dow"]),
			Count:     r.Count,
			Withdrawn: r.Withdrawn,
		})
		upcomingTotal += r.Withdrawn
	}

	// Get previous power downs from vesting_withdraw collection
	withdrawCollection := s.db.Collection("vesting_withdraw")
	sevenDaysAgo := time.Now().AddDate(0, 0, -7).Truncate(24 * time.Hour)

	previousPipeline := []bson.M{
		{
			"$match": bson.M{
				"_ts": bson.M{
					"$gte": sevenDaysAgo,
				},
			},
		},
		{
			"$group": bson.M{
				"_id": bson.M{
					"doy":   bson.M{"$dayOfYear": "$_ts"},
					"year":  bson.M{"$year": "$_ts"},
					"month": bson.M{"$month": "$_ts"},
					"day":   bson.M{"$dayOfMonth": "$_ts"},
					"dow":   bson.M{"$dayOfWeek": "$_ts"},
				},
				"count":     bson.M{"$sum": 1},
				"withdrawn": bson.M{"$sum": "$withdrawn"},
				"deposited": bson.M{"$sum": "$deposited"},
			},
		},
		{
			"$sort": bson.M{
				"_id.year": 1,
				"_id.doy":  1,
			},
		},
		{
			"$limit": 8,
		},
	}

	cursor, err = withdrawCollection.Aggregate(ctx, previousPipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate previous power downs: %w", err)
	}
	defer cursor.Close(ctx)

	var previousResults []struct {
		ID        bson.M  `bson:"_id"`
		Count     int     `bson:"count"`
		Withdrawn float64 `bson:"withdrawn"`
		Deposited float64 `bson:"deposited"`
	}
	if err := cursor.All(ctx, &previousResults); err != nil {
		return nil, fmt.Errorf("failed to decode previous: %w", err)
	}

	previous := make([]models.PowerDownDay, 0, len(previousResults))
	previousTotal := 0.0
	for _, r := range previousResults {
		id := r.ID
		previous = append(previous, models.PowerDownDay{
			DayOfYear: getInt(id["doy"]),
			Year:      getInt(id["year"]),
			Month:     getInt(id["month"]),
			Day:       getInt(id["day"]),
			DayOfWeek: getInt(id["dow"]),
			Count:     r.Count,
			Withdrawn: r.Withdrawn,
			Deposited: r.Deposited,
		})
		previousTotal += r.Withdrawn
	}

	// Get top power down users
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30).Truncate(24 * time.Hour)
	midnight := time.Now().Truncate(24 * time.Hour)

	userPipeline := []bson.M{
		{
			"$match": bson.M{
				"_ts": bson.M{
					"$gte": thirtyDaysAgo,
					"$lte": midnight,
				},
			},
		},
		{
			"$group": bson.M{
				"_id": bson.M{
					"user": "$from_account",
				},
				"count":        bson.M{"$sum": 1},
				"withdrawn":    bson.M{"$sum": "$withdrawn"},
				"deposited":    bson.M{"$sum": "$deposited"},
				"deposited_to": bson.M{"$addToSet": "$to_account"},
			},
		},
		{
			"$lookup": bson.M{
				"from":         "account",
				"localField":   "_id.user",
				"foreignField": "name",
				"as":           "account",
			},
		},
		{
			"$sort": bson.M{
				"withdrawn": -1,
			},
		},
		{
			"$limit": 100,
		},
	}

	cursor, err = withdrawCollection.Aggregate(ctx, userPipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate power down users: %w", err)
	}
	defer cursor.Close(ctx)

	var userResults []struct {
		ID          bson.M           `bson:"_id"`
		Count       int              `bson:"count"`
		Withdrawn   float64          `bson:"withdrawn"`
		Deposited   float64          `bson:"deposited"`
		DepositedTo []string         `bson:"deposited_to"`
		Account     []models.Account `bson:"account"`
	}
	if err := cursor.All(ctx, &userResults); err != nil {
		return nil, fmt.Errorf("failed to decode power down users: %w", err)
	}

	powerDowns := make([]models.PowerDownUser, 0, len(userResults))
	for _, r := range userResults {
		userID := r.ID["user"]
		user, _ := userID.(string)

		var account *models.Account
		if len(r.Account) > 0 {
			account = &r.Account[0]
		}

		powerDowns = append(powerDowns, models.PowerDownUser{
			User:        user,
			Count:       r.Count,
			Withdrawn:   r.Withdrawn,
			Deposited:   r.Deposited,
			DepositedTo: r.DepositedTo,
			Account:     account,
		})
	}

	return &models.PowerDown{
		UpcomingTotal: upcomingTotal,
		Upcoming:      upcoming,
		PreviousTotal: previousTotal,
		Previous:      previous,
		PowerDowns:    powerDowns,
		Props:         powerDownProps,
	}, nil
}

// GetRsharesAllocation retrieves rshares allocation data
func (s *LabsService) GetRsharesAllocation(ctx context.Context, date time.Time) ([]models.RsharesAllocation, error) {
	commentCollection := s.db.Collection("comment")

	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	pipeline := []bson.M{
		{
			"$match": bson.M{
				"created": bson.M{
					"$gte": startOfDay,
					"$lt":  endOfDay,
				},
			},
		},
		{
			"$project": bson.M{
				"author":       1,
				"active_votes": 1,
				"created":      1,
			},
		},
		{
			"$unwind": "$active_votes",
		},
		{
			"$match": bson.M{
				"active_votes.weight": bson.M{"$ne": 0},
			},
		},
		{
			"$group": bson.M{
				"_id": bson.M{
					"voter": "$active_votes.voter",
					"doy":   bson.M{"$dayOfYear": "$created"},
					"year":  bson.M{"$year": "$created"},
					"month": bson.M{"$month": "$created"},
					"day":   bson.M{"$dayOfMonth": "$created"},
				},
				"votes":   bson.M{"$sum": 1},
				"rshares": bson.M{"$sum": "$active_votes.rshares"},
			},
		},
		{
			"$sort": bson.M{
				"rshares": -1,
			},
		},
		{
			"$lookup": bson.M{
				"from":         "account",
				"localField":   "_id.voter",
				"foreignField": "name",
				"as":           "account",
			},
		},
	}

	cursor, err := commentCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate rshares: %w", err)
	}
	defer cursor.Close(ctx)

	var results []struct {
		ID      bson.M           `bson:"_id"`
		Votes   int              `bson:"votes"`
		Rshares int64            `bson:"rshares"`
		Account []models.Account `bson:"account"`
	}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("failed to decode rshares: %w", err)
	}

	allocations := make([]models.RsharesAllocation, 0, len(results))
	for _, r := range results {
		voterID := r.ID["voter"]
		voter, _ := voterID.(string)

		var account *models.Account
		if len(r.Account) > 0 {
			account = &r.Account[0]
		}

		allocations = append(allocations, models.RsharesAllocation{
			Voter:   voter,
			Votes:   r.Votes,
			Rshares: r.Rshares,
			Account: account,
		})
	}

	return allocations, nil
}

// GetCurationLeaderboard retrieves curation reward leaderboard
func (s *LabsService) GetCurationLeaderboard(ctx context.Context, date time.Time, grouping string) ([]models.CurationLeaderboard, error) {
	collection := s.db.Collection("curation_reward")

	var startTime, endTime time.Time
	if grouping == "monthly" {
		startTime = time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, date.Location())
		endTime = startTime.AddDate(0, 1, 0)
	} else {
		startTime = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
		endTime = startTime.Add(24 * time.Hour)
	}

	pipeline := []bson.M{
		{
			"$match": bson.M{
				"_ts": bson.M{
					"$gte": startTime,
					"$lt":  endTime,
				},
			},
		},
		{
			"$group": bson.M{
				"_id":     "$curator",
				"count":   bson.M{"$sum": 1},
				"total":   bson.M{"$sum": "$reward"},
				"authors": bson.M{"$addToSet": "$comment_author"},
				"permlinks": bson.M{
					"$addToSet": bson.M{
						"$concat": bson.A{"$comment_author", "/", "$comment_permlink"},
					},
				},
			},
		},
		{
			"$sort": bson.M{
				"total": -1,
			},
		},
		{
			"$limit": 100,
		},
		{
			"$lookup": bson.M{
				"from":         "account",
				"localField":   "_id",
				"foreignField": "name",
				"as":           "account",
			},
		},
	}

	cursor, err := collection.Aggregate(ctx, pipeline, options.Aggregate().SetAllowDiskUse(true))
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate curation leaderboard: %w", err)
	}
	defer cursor.Close(ctx)

	var results []struct {
		ID        string           `bson:"_id"`
		Count     int              `bson:"count"`
		Total     float64          `bson:"total"`
		Authors   []string         `bson:"authors"`
		Permlinks []string         `bson:"permlinks"`
		Account   []models.Account `bson:"account"`
	}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("failed to decode curation leaderboard: %w", err)
	}

	leaderboard := make([]models.CurationLeaderboard, 0, len(results))
	for _, r := range results {
		var account *models.Account
		if len(r.Account) > 0 {
			account = &r.Account[0]
		}

		leaderboard = append(leaderboard, models.CurationLeaderboard{
			Curator:   r.ID,
			Count:     r.Count,
			Total:     r.Total,
			Authors:   r.Authors,
			Permlinks: r.Permlinks,
			Account:   account,
		})
	}

	return leaderboard, nil
}

// GetAuthorLeaderboard retrieves author reward leaderboard
func (s *LabsService) GetAuthorLeaderboard(ctx context.Context, date time.Time, grouping string) ([]models.AuthorLeaderboard, error) {
	collection := s.db.Collection("author_reward")

	var startTime, endTime time.Time
	if grouping == "monthly" {
		startTime = time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, date.Location())
		endTime = startTime.AddDate(0, 1, 0)
	} else {
		startTime = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
		endTime = startTime.Add(24 * time.Hour)
	}

	pipeline := []bson.M{
		{
			"$match": bson.M{
				"_ts": bson.M{
					"$gte": startTime,
					"$lt":  endTime,
				},
			},
		},
		{
			"$project": bson.M{
				"prefix":         bson.M{"$substr": bson.A{"$permlink", 0, 3}},
				"author":         1,
				"permlink":       1,
				"steem_payout":   1,
				"vesting_payout": 1,
				"sbd_payout":     1,
			},
		},
		{
			"$group": bson.M{
				"_id":   "$author",
				"count": bson.M{"$sum": 1},
				"posts": bson.M{
					"$sum": bson.M{
						"$cond": bson.A{
							bson.M{"$eq": bson.A{"$prefix", "re-"}},
							0,
							1,
						},
					},
				},
				"replies": bson.M{
					"$sum": bson.M{
						"$cond": bson.A{
							bson.M{"$eq": bson.A{"$prefix", "re-"}},
							1,
							0,
						},
					},
				},
				"post_vest": bson.M{
					"$sum": bson.M{
						"$cond": bson.A{
							bson.M{"$eq": bson.A{"$prefix", "re-"}},
							0,
							"$vesting_payout",
						},
					},
				},
				"post_sbd": bson.M{
					"$sum": bson.M{
						"$cond": bson.A{
							bson.M{"$eq": bson.A{"$prefix", "re-"}},
							0,
							"$sbd_payout",
						},
					},
				},
				"post_steem": bson.M{
					"$sum": bson.M{
						"$cond": bson.A{
							bson.M{"$eq": bson.A{"$prefix", "re-"}},
							0,
							"$steem_payout",
						},
					},
				},
				"reply_vest": bson.M{
					"$sum": bson.M{
						"$cond": bson.A{
							bson.M{"$eq": bson.A{"$prefix", "re-"}},
							"$vesting_payout",
							0,
						},
					},
				},
				"reply_sbd": bson.M{
					"$sum": bson.M{
						"$cond": bson.A{
							bson.M{"$eq": bson.A{"$prefix", "re-"}},
							"$sbd_payout",
							0,
						},
					},
				},
				"reply_steem": bson.M{
					"$sum": bson.M{
						"$cond": bson.A{
							bson.M{"$eq": bson.A{"$prefix", "re-"}},
							"$steem_payout",
							0,
						},
					},
				},
				"sbd":   bson.M{"$sum": "$sbd_payout"},
				"steem": bson.M{"$sum": "$steem_payout"},
				"vest":  bson.M{"$sum": "$vesting_payout"},
				"permlinks": bson.M{
					"$addToSet": bson.M{
						"$concat": bson.A{"$author", "/", "$permlink"},
					},
				},
			},
		},
		{
			"$sort": bson.M{
				"vest": -1,
			},
		},
		{
			"$lookup": bson.M{
				"from":         "account",
				"localField":   "_id",
				"foreignField": "name",
				"as":           "account",
			},
		},
	}

	cursor, err := collection.Aggregate(ctx, pipeline, options.Aggregate().SetAllowDiskUse(true))
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate author leaderboard: %w", err)
	}
	defer cursor.Close(ctx)

	var results []struct {
		ID         string           `bson:"_id"`
		Count      int              `bson:"count"`
		Posts      int              `bson:"posts"`
		Replies    int              `bson:"replies"`
		PostVest   float64          `bson:"post_vest"`
		PostSbd    float64          `bson:"post_sbd"`
		PostSteem  float64          `bson:"post_steem"`
		ReplyVest  float64          `bson:"reply_vest"`
		ReplySbd   float64          `bson:"reply_sbd"`
		ReplySteem float64          `bson:"reply_steem"`
		Sbd        float64          `bson:"sbd"`
		Steem      float64          `bson:"steem"`
		Vest       float64          `bson:"vest"`
		Permlinks  []string         `bson:"permlinks"`
		Account    []models.Account `bson:"account"`
	}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("failed to decode author leaderboard: %w", err)
	}

	leaderboard := make([]models.AuthorLeaderboard, 0, len(results))
	for _, r := range results {
		var account *models.Account
		if len(r.Account) > 0 {
			account = &r.Account[0]
		}

		leaderboard = append(leaderboard, models.AuthorLeaderboard{
			Author:     r.ID,
			Count:      r.Count,
			Posts:      r.Posts,
			Replies:    r.Replies,
			PostVest:   r.PostVest,
			PostSbd:    r.PostSbd,
			PostSteem:  r.PostSteem,
			ReplyVest:  r.ReplyVest,
			ReplySbd:   r.ReplySbd,
			ReplySteem: r.ReplySteem,
			Sbd:        r.Sbd,
			Steem:      r.Steem,
			Vest:       r.Vest,
			Permlinks:  r.Permlinks,
			Account:    account,
		})
	}

	return leaderboard, nil
}

// GetFlags retrieves flagged accounts statistics
func (s *LabsService) GetFlags(ctx context.Context) ([]models.Flags, error) {
	voteCollection := s.db.Collection("vote")

	pipeline := []bson.M{
		{
			"$match": bson.M{
				"weight": bson.M{"$lt": 0},
			},
		},
		{
			"$group": bson.M{
				"_id":      "$author",
				"count":    bson.M{"$sum": 1},
				"flaggers": bson.M{"$push": "$voter"},
				"posts":    bson.M{"$addToSet": "$permlink"},
			},
		},
		{
			"$sort": bson.M{
				"count": -1,
			},
		},
		{
			"$limit": 200,
		},
	}

	cursor, err := voteCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate flags: %w", err)
	}
	defer cursor.Close(ctx)

	var results []struct {
		ID       string   `bson:"_id"`
		Count    int      `bson:"count"`
		Flaggers []string `bson:"flaggers"`
		Posts    []string `bson:"posts"`
	}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("failed to decode flags: %w", err)
	}

	flags := make([]models.Flags, 0, len(results))
	for _, r := range results {
		// Count flaggers
		voterCounts := make(map[string]int)
		for _, voter := range r.Flaggers {
			voterCounts[voter]++
		}

		// Sort by count and take top 10
		type voterCount struct {
			voter string
			count int
		}
		voters := make([]voterCount, 0, len(voterCounts))
		for voter, count := range voterCounts {
			voters = append(voters, voterCount{voter: voter, count: count})
		}
		sort.Slice(voters, func(i, j int) bool {
			return voters[i].count > voters[j].count
		})

		topVoters := make(map[string]int)
		for i, v := range voters {
			if i >= 10 {
				break
			}
			topVoters[v.voter] = v.count
		}

		flags = append(flags, models.Flags{
			Author:   r.ID,
			Count:    r.Count,
			Flaggers: r.Flaggers,
			Posts:    r.Posts,
			Voters:   topVoters,
		})
	}

	return flags, nil
}

// GetClients retrieves client statistics from the clients-snapshot maintained
// by steemdb-sync's refresher (legacy history.py update_clients pipeline).
// Snapshot shape: [{_id: {doy, year, month, day, dow}, clients: [{client,
// count, reward}], reward, total}, ...] — one entry per day with apps seen in
// comment json_metadata.app over the last 90 days.
func (s *LabsService) GetClients(ctx context.Context) (*models.Clients, error) {
	statusCollection := s.db.Collection("status")

	var status struct {
		ID   string          `bson:"_id"`
		Data []clientDayData `bson:"data"`
	}

	err := statusCollection.FindOne(ctx, bson.M{"_id": "clients-snapshot"}).Decode(&status)
	if err != nil {
		return nil, fmt.Errorf("failed to get clients snapshot: %w", err)
	}

	clients := &models.Clients{
		Dates:   make([]models.ClientDate, 0, len(status.Data)),
		Posts:   make(map[string]int),
		Rewards: make(map[string]float64),
	}

	for _, day := range status.Data {
		entries := make([]models.ClientEntry, 0, len(day.Clients))
		for _, c := range day.Clients {
			entries = append(entries, models.ClientEntry{
				Client: c.Client,
				Count:  c.Count,
				Reward: c.Reward,
			})
			clients.Posts[c.Client] += c.Count
			clients.Rewards[c.Client] += c.Reward
		}
		clients.Dates = append(clients.Dates, models.ClientDate{
			Date:    time.Date(day.ID.Year, time.Month(day.ID.Month), day.ID.Day, 0, 0, 0, 0, time.UTC),
			Clients: entries,
		})
	}

	return clients, nil
}

// clientDayData mirrors one clients-snapshot aggregation entry.
type clientDayData struct {
	ID struct {
		Doy   int `bson:"doy"`
		Year  int `bson:"year"`
		Month int `bson:"month"`
		Day   int `bson:"day"`
		Dow   int `bson:"dow"`
	} `bson:"_id"`
	Clients []struct {
		Client string  `bson:"client"`
		Count  int     `bson:"count"`
		Reward float64 `bson:"reward"`
	} `bson:"clients"`
	Reward float64 `bson:"reward"`
	Total  int     `bson:"total"`
}

// GetBenefactors retrieves benefactor reward statistics
func (s *LabsService) GetBenefactors(ctx context.Context) (*models.Benefactors, error) {
	collection := s.db.Collection("benefactor_reward")

	pipeline := []bson.M{
		{
			"$sort": bson.M{
				"_ts": -1,
			},
		},
		{
			"$group": bson.M{
				"_id": bson.M{
					"benefactor": "$benefactor",
					"doy":        bson.M{"$dayOfYear": "$_ts"},
					"year":       bson.M{"$year": "$_ts"},
					"month":      bson.M{"$month": "$_ts"},
					"day":        bson.M{"$dayOfMonth": "$_ts"},
					"dow":        bson.M{"$dayOfWeek": "$_ts"},
				},
				"reward": bson.M{"$sum": "$reward"},
				"count":  bson.M{"$sum": 1},
			},
		},
		{
			"$group": bson.M{
				"_id": bson.M{
					"doy":   "$_id.doy",
					"year":  "$_id.year",
					"month": "$_id.month",
					"day":   "$_id.day",
					"dow":   "$_id.dow",
				},
				"benefactors": bson.M{
					"$push": bson.M{
						"benefactor": "$_id.benefactor",
						"count":      "$count",
						"reward":     "$reward",
					},
				},
				"reward": bson.M{"$sum": "$reward"},
				"total":  bson.M{"$sum": "$count"},
			},
		},
		{
			"$sort": bson.M{
				"_id.year": 1,
				"_id.doy":  1,
				"reward":   -1,
			},
		},
		{
			"$limit": 10,
		},
	}

	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate benefactors: %w", err)
	}
	defer cursor.Close(ctx)

	var results []struct {
		ID          bson.M   `bson:"_id"`
		Benefactors []bson.M `bson:"benefactors"`
		Reward      float64  `bson:"reward"`
		Total       int      `bson:"total"`
	}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("failed to decode benefactors: %w", err)
	}

	dates := make([]models.BenefactorDate, 0, len(results))
	for _, r := range results {
		id := r.ID
		benefactorEntries := make([]models.BenefactorEntry, 0, len(r.Benefactors))
		for _, b := range r.Benefactors {
			benefactor, _ := b["benefactor"].(string)
			count := getInt(b["count"])
			reward := getFloat64(b["reward"])

			benefactorEntries = append(benefactorEntries, models.BenefactorEntry{
				Benefactor: benefactor,
				Count:      count,
				Reward:     reward,
			})
		}

		dates = append(dates, models.BenefactorDate{
			DayOfYear:   getInt(id["doy"]),
			Year:        getInt(id["year"]),
			Month:       getInt(id["month"]),
			Day:         getInt(id["day"]),
			DayOfWeek:   getInt(id["dow"]),
			Benefactors: benefactorEntries,
			Reward:      r.Reward,
			Total:       r.Total,
		})
	}

	return &models.Benefactors{
		Dates: dates,
	}, nil
}

// GetPendingPosts retrieves pending payout posts
func (s *LabsService) GetPendingPosts(ctx context.Context) ([]models.PendingPost, error) {
	collection := s.db.Collection("comment")

	sevenDaysAgo := time.Now().AddDate(0, 0, -7)
	sixDaysAgo := time.Now().AddDate(0, 0, -6).Add(-156 * time.Hour)

	query := bson.M{
		"created": bson.M{
			"$gte": sevenDaysAgo,
			"$lte": sixDaysAgo,
		},
	}

	findOptions := options.Find().
		SetSort(bson.M{"pending_payout_value": -1}).
		SetLimit(200)

	cursor, err := collection.Find(ctx, query, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to find pending posts: %w", err)
	}
	defer cursor.Close(ctx)

	var comments []struct {
		ID                      string    `bson:"_id"`
		Author                  string    `bson:"author"`
		Permlink                string    `bson:"permlink"`
		Title                   string    `bson:"title"`
		Created                 time.Time `bson:"created"`
		PendingPayoutValue      float64   `bson:"pending_payout_value"`
		TotalPendingPayoutValue float64   `bson:"total_pending_payout_value"`
	}
	if err := cursor.All(ctx, &comments); err != nil {
		return nil, fmt.Errorf("failed to decode pending posts: %w", err)
	}

	pendingPosts := make([]models.PendingPost, 0, len(comments))
	for _, c := range comments {
		pendingPosts = append(pendingPosts, models.PendingPost{
			ID:                      c.ID,
			Author:                  c.Author,
			Permlink:                c.Permlink,
			Title:                   c.Title,
			Created:                 c.Created,
			PendingPayoutValue:      c.PendingPayoutValue,
			TotalPendingPayoutValue: c.TotalPendingPayoutValue,
		})
	}

	return pendingPosts, nil
}

// Helper functions
func parseAmount(amountStr string) float64 {
	parts := strings.Fields(amountStr)
	if len(parts) > 0 {
		if val, err := strconv.ParseFloat(parts[0], 64); err == nil {
			return val
		}
	}
	return 0
}

func getInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case int32:
		return int(val)
	case int64:
		return int(val)
	case float64:
		return int(val)
	default:
		return 0
	}
}

func getFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	default:
		return 0
	}
}
