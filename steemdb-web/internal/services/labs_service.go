package services

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
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

// powerUpRow is one output row of the powerup aggregation ($group + $lookup).
// Kept at package level so the tolerant decoding can be unit tested. The
// joined account docs and the numeric amount values are decoded leniently
// (bson.M / interface{}) because both are written by steemdb-sync with open
// shapes: account documents carry raw RPC strings (withdrawn,
// curation_rewards, proxied_vsf_votes, last_account_update) and
// vesting_deposit.amount is numeric, not an asset string.
type powerUpRow struct {
	ID        bson.M        `bson:"_id"`
	Count     int           `bson:"count"`
	Total     float64       `bson:"total"`
	Instances []interface{} `bson:"instances"`
	Account   []bson.M      `bson:"account"`
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
				"count": bson.M{"$sum": 1},
				// total is the sum over every deposit. The former code derived
				// it by summing `instances`, which was an $addToSet of amounts:
				// two identical amounts in the window collapsed into one, so
				// the figure was systematically short while `count` stayed
				// right (worst account under-reported by 99.86%).
				"total":     bson.M{"$sum": "$amount"},
				"instances": bson.M{"$addToSet": "$amount"},
			},
		},
		{
			// Rank before truncating: $limit ahead of $sort kept an arbitrary
			// 1000 accounts instead of the top 1000 once the window exceeded
			// it. Sorting ~1k grouped rows is negligible.
			"$sort": bson.M{
				"total": -1,
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

	var results []powerUpRow
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("failed to decode power ups: %w", err)
	}

	powerUps := make([]models.PowerUp, 0, len(results))
	for _, r := range results {
		user, _ := r.ID["user"].(string)

		// `total` comes from the aggregation ($sum over every deposit), so
		// duplicate amounts are counted. The instances walk stays as a
		// fallback for rows written by an older pipeline that only produced
		// the distinct-amount set (vesting_deposit.amount is numeric; older
		// writers may still carry asset strings, hence the lenient coercion).
		total := r.Total
		instances := make([]float64, 0, len(r.Instances))
		for _, inst := range r.Instances {
			instances = append(instances, toFloat64(inst))
		}
		if total == 0 && len(instances) > 0 {
			for _, val := range instances {
				total += val
			}
		}

		powerUps = append(powerUps, models.PowerUp{
			User:      user,
			Count:     r.Count,
			Total:     total,
			Instances: instances,
			Account:   accountSummaryFromLookup(r.Account),
		})
	}

	// Sort by total descending
	sort.Slice(powerUps, func(i, j int) bool {
		return powerUps[i].Total > powerUps[j].Total
	})

	return powerUps, nil
}

// GetPowerDowns retrieves power down statistics.
//
// Unit contract: `vesting_withdraw.withdrawn` and
// `account.vesting_withdraw_rate` are VESTS on chain, `deposited` is STEEM.
// Every figure this endpoint exposes as "withdrawn" is converted to STEEM at
// the current global-properties rate, because the question the page answers is
// how much STEEM is being redeemed (showing raw VESTS as STEEM overstated the
// page by ~1610x). `deposited` needs no conversion. Past days carry the exact
// per-op `deposited` sum, so a historical "withdrawn" is the VESTS total valued
// at today's rate, not at the withdrawal moment (the collection holds no
// per-op rate); the two differ by <0.05% per the 2026-10 audit.
func (s *LabsService) GetPowerDowns(ctx context.Context) (*models.PowerDown, error) {
	// Get blockchain properties
	props, err := s.steemClient.GetDynamicGlobalProperties()
	if err != nil {
		return nil, fmt.Errorf("failed to get props: %w", err)
	}

	// Parse amounts
	current := parseAmount(props.CurrentSupply)
	vesting := parseAmount(props.TotalVestingFundSteem)
	// current_supply = balances + savings + escrow + vesting fund + reward fund
	// + pending rewarded vesting. Subtracting only the vesting fund leaves the
	// reward fund (currently empty) and the already-earmarked pending vesting
	// rewards in the figure, so subtract both: what remains is the spendable
	// upper bound (savings/escrow are still counted — they are liquid to their
	// owners, just not free-floating).
	pendingRewarded := parseAmount(props.PendingRewardedVestingSteem)
	liquid := current - vesting - pendingRewarded

	vestsRatio := vestsToSteemRatio(props.TotalVestingFundSteem, props.TotalVestingShares)

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
		withdrawn := vestsToSteem(r.Withdrawn, vestsRatio)
		upcoming = append(upcoming, models.PowerDownDay{
			DayOfYear: getInt(id["doy"]),
			Year:      getInt(id["year"]),
			Month:     getInt(id["month"]),
			Day:       getInt(id["day"]),
			DayOfWeek: getInt(id["dow"]),
			Count:     r.Count,
			Withdrawn: withdrawn,
		})
		upcomingTotal += withdrawn
	}

	// Get previous power downs from vesting_withdraw collection
	withdrawCollection := s.db.Collection("vesting_withdraw")
	// Exact 7x24h window, not a day-aligned one: day alignment ran from the
	// midnight 7 days back, i.e. 8 daily buckets, and because a power down
	// batch repeats every 7 days the same batch landed in both the first and
	// the last bucket — the "(Last 7 Days)" total counted one full weekly batch
	// twice. The first bucket is now partial, which is the honest reading of a
	// rolling window.
	sevenDaysAgo := previousPowerDownWindow(time.Now())

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
		withdrawn := vestsToSteem(r.Withdrawn, vestsRatio)
		previous = append(previous, models.PowerDownDay{
			DayOfYear: getInt(id["doy"]),
			Year:      getInt(id["year"]),
			Month:     getInt(id["month"]),
			Day:       getInt(id["day"]),
			DayOfWeek: getInt(id["dow"]),
			Count:     r.Count,
			Withdrawn: withdrawn,
			Deposited: r.Deposited,
		})
		previousTotal += withdrawn
	}

	// Get top power down users (rolling 30 days, today included — the former
	// upper bound at today's midnight dropped the whole current day).
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30).Truncate(24 * time.Hour)

	userPipeline := []bson.M{
		{
			"$match": bson.M{
				"_ts": bson.M{
					"$gte": thirtyDaysAgo,
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
		ID          bson.M   `bson:"_id"`
		Count       int      `bson:"count"`
		Withdrawn   float64  `bson:"withdrawn"`
		Deposited   float64  `bson:"deposited"`
		DepositedTo []string `bson:"deposited_to"`
		Account     []bson.M `bson:"account"`
	}
	if err := cursor.All(ctx, &userResults); err != nil {
		return nil, fmt.Errorf("failed to decode power down users: %w", err)
	}

	powerDowns := make([]models.PowerDownUser, 0, len(userResults))
	for _, r := range userResults {
		userID := r.ID["user"]
		user, _ := userID.(string)

		powerDowns = append(powerDowns, models.PowerDownUser{
			User:        user,
			Count:       r.Count,
			Withdrawn:   vestsToSteem(r.Withdrawn, vestsRatio),
			Deposited:   r.Deposited,
			DepositedTo: r.DepositedTo,
			Account:     accountSummaryFromLookup(r.Account),
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
		ID      bson.M   `bson:"_id"`
		Votes   int      `bson:"votes"`
		Rshares int64    `bson:"rshares"`
		Account []bson.M `bson:"account"`
	}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("failed to decode rshares: %w", err)
	}

	allocations := make([]models.RsharesAllocation, 0, len(results))
	for _, r := range results {
		voterID := r.ID["voter"]
		voter, _ := voterID.(string)

		allocations = append(allocations, models.RsharesAllocation{
			Voter:   voter,
			Votes:   r.Votes,
			Rshares: r.Rshares,
			Account: accountSummaryFromLookup(r.Account),
		})
	}

	return allocations, nil
}

// curatedPermlinksPerCurator bounds how many of a curator's posts the
// leaderboard returns per row.
const curatedPermlinksPerCurator = 20

// GetCurationLeaderboard retrieves curation reward leaderboard for one UTC day
// or month.
//
// Unit contract: `total` is the sum of curation_reward.reward, which is VESTS
// on chain — the SPA renders it with formatVests. `count` is the number of
// reward ops and `authors` the distinct rewarded authors.
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
			// The permlink set dominates the response (96.9-99.8% of the
			// bytes; a monthly board was ~4.7MB) and nothing renders it. Cap
			// it per curator instead of returning every post, which also
			// bounds the document size far from the 16MB limit.
			"$project": bson.M{
				"count":     1,
				"total":     1,
				"authors":   1,
				"permlinks": bson.M{"$slice": bson.A{"$permlinks", curatedPermlinksPerCurator}},
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
		return nil, fmt.Errorf("failed to aggregate curation leaderboard: %w", err)
	}
	defer cursor.Close(ctx)

	var results []struct {
		ID        string   `bson:"_id"`
		Count     int      `bson:"count"`
		Total     float64  `bson:"total"`
		Authors   []string `bson:"authors"`
		Permlinks []string `bson:"permlinks"`
		Account   []bson.M `bson:"account"`
	}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("failed to decode curation leaderboard: %w", err)
	}

	leaderboard := make([]models.CurationLeaderboard, 0, len(results))
	for _, r := range results {
		leaderboard = append(leaderboard, models.CurationLeaderboard{
			Curator:   r.ID,
			Count:     r.Count,
			Total:     r.Total,
			Authors:   r.Authors,
			Permlinks: r.Permlinks,
			Account:   accountSummaryFromLookup(r.Account),
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
		ID         string   `bson:"_id"`
		Count      int      `bson:"count"`
		Posts      int      `bson:"posts"`
		Replies    int      `bson:"replies"`
		PostVest   float64  `bson:"post_vest"`
		PostSbd    float64  `bson:"post_sbd"`
		PostSteem  float64  `bson:"post_steem"`
		ReplyVest  float64  `bson:"reply_vest"`
		ReplySbd   float64  `bson:"reply_sbd"`
		ReplySteem float64  `bson:"reply_steem"`
		Sbd        float64  `bson:"sbd"`
		Steem      float64  `bson:"steem"`
		Vest       float64  `bson:"vest"`
		Permlinks  []string `bson:"permlinks"`
		Account    []bson.M `bson:"account"`
	}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("failed to decode author leaderboard: %w", err)
	}

	leaderboard := make([]models.AuthorLeaderboard, 0, len(results))
	for _, r := range results {
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
			Account:    accountSummaryFromLookup(r.Account),
		})
	}

	return leaderboard, nil
}

// GetFlags retrieves the flagged-accounts leaderboard from
// status.flags-snapshot (maintained by steemdb-sync's flags refresher).
//
// This used to aggregate the vote collection per request: `vote` holds ~941M
// documents (~7.7M with weight<0) and the only index on that slice is the
// partial {weight:1}, which has no author/voter/permlink — so every request
// FETCHed all ~7.7M negative-vote documents and the $group needed >100MB, which
// MongoDB rejected with code 292
// (QueryExceededMemoryLimitNoDiskUseAllowed), surfacing as this endpoint's
// permanent 500. With allowDiskUse the same aggregation took 145-293s on
// production, so it cannot be the request path at all.
//
// The snapshot also trims the payload: the old response shipped every voter
// name and every flagged permlink (~40MB for the top 200 accounts, none of
// which the SPA read).
func (s *LabsService) GetFlags(ctx context.Context) ([]models.Flags, error) {
	var snapshot struct {
		ID   string              `bson:"_id"`
		Data []flagSnapshotEntry `bson:"data"`
	}

	err := s.db.Collection("status").FindOne(ctx, bson.M{"_id": flagsSnapshotID}).Decode(&snapshot)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("flags snapshot %q not available yet: %w", flagsSnapshotID, err)
		}
		return nil, fmt.Errorf("failed to get flags snapshot: %w", err)
	}

	return flagsFromSnapshot(snapshot.Data), nil
}

// flagsFromSnapshot maps status.flags-snapshot rows to API models.
func flagsFromSnapshot(entries []flagSnapshotEntry) []models.Flags {
	flags := make([]models.Flags, 0, len(entries))
	for _, entry := range entries {
		voters := make(map[string]int, len(entry.Voters))
		for _, v := range entry.Voters {
			voters[v.Voter] = v.Count
		}
		flags = append(flags, models.Flags{
			Author:     entry.Author,
			Count:      entry.Count,
			VoterCount: entry.VoterCount,
			PostCount:  entry.PostCount,
			Voters:     voters,
		})
	}
	return flags
}

// flagsSnapshotID is the status document holding the precomputed flags
// leaderboard (written by steemdb-sync's flags refresher).
const flagsSnapshotID = "flags-snapshot"

// flagSnapshotEntry mirrors one status.flags-snapshot row.
type flagSnapshotEntry struct {
	Author     string `bson:"author"`
	Count      int    `bson:"count"`
	VoterCount int    `bson:"voter_count"`
	PostCount  int    `bson:"post_count"`
	Voters     []struct {
		Voter string `bson:"voter"`
		Count int    `bson:"count"`
	} `bson:"voters"`
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

// GetBenefactors retrieves benefactor reward statistics for the last 10 days.
//
// reward is VESTS (comment_benefactor_reward.vesting_payout), never STEEM —
// see docs/AI-driver/02-data-model.md; the SPA renders it with formatVests.
func (s *LabsService) GetBenefactors(ctx context.Context) (*models.Benefactors, error) {
	collection := s.db.Collection("benefactor_reward")

	windowStart := benefactorWindowStart(time.Now())

	pipeline := []bson.M{
		{
			// Bounded scan: `_ts` leads the benefactor_reward index, so this
			// is a range scan over ~6.5k documents instead of a 7.1M-document
			// full-collection $group that exceeds MongoDB's $group memory cap.
			"$match": bson.M{
				"_ts": bson.M{"$gte": windowStart},
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
			// Order the per-day benefactor array by reward before folding it
			// into the day document, so the SPA's first 10 rows (and its
			// "+N more benefactors") are the day's top benefactors. The former
			// `$sort {_ts:-1}` on the raw collection only ordered that array by
			// event time.
			"$sort": bson.M{
				"_id.year": -1,
				"_id.doy":  -1,
				"reward":   -1,
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
			// Newest day first (was ascending, which together with $limit 10
			// returned the oldest 10 days of the collection's history).
			"$sort": bson.M{
				"_id.year": -1,
				"_id.doy":  -1,
			},
		},
		{
			"$limit": 10,
		},
	}

	cursor, err := collection.Aggregate(ctx, pipeline, options.Aggregate().
		SetAllowDiskUse(true).
		SetMaxTime(10*time.Second))
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

// GetPendingPosts retrieves posts whose payout is still pending.
//
// Scope: top-level posts only (`depth: 0`) — replies were 14.5% of the rows on
// a page titled "Posts" — with a strictly positive pending payout, ranked by
// pending_payout_value, capped at 200 rows.
func (s *LabsService) GetPendingPosts(ctx context.Context) ([]models.PendingPost, error) {
	collection := s.db.Collection("comment")

	start, end := pendingPostsWindow(time.Now())

	query := bson.M{
		"created": bson.M{
			"$gte": start,
			"$lte": end,
		},
		"depth":                0,
		"pending_payout_value": bson.M{"$gt": 0},
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

// pendingPostsWindow returns the [start, end] created-time bounds for the
// pending payout review: posts created between 7 days ago and 156 hours
// (6.5 days) ago — i.e. in their final 12 hours before the 7-day cashout.
// This mirrors legacy LabsController::pendingAction exactly
// ($gte strtotime("-7 days"), $lte strtotime("-156 hours")).
func pendingPostsWindow(now time.Time) (time.Time, time.Time) {
	sevenDaysAgo := now.AddDate(0, 0, -7)
	hundredFiftySixHoursAgo := now.Add(-156 * time.Hour)
	return sevenDaysAgo, hundredFiftySixHoursAgo
}

// previousPowerDownWindow returns the lower bound of the "(Last 7 Days)"
// power-down window: exactly 7x24h before now.
//
// It used to be day-aligned (`now.AddDate(0,0,-7).Truncate(24h)`), which spans
// 8 UTC calendar buckets; since a power down batch repeats every 7 days, the
// same batch appeared in both the first and the last bucket and the total
// counted it twice. A rolling window whose first bucket is partial is the
// correct reading of "last 7 days".
func previousPowerDownWindow(now time.Time) time.Time {
	return now.Add(-7 * 24 * time.Hour)
}

// vestsToSteemRatio returns the STEEM value of one VEST at the given global
// properties snapshot (total_vesting_fund_steem / total_vesting_shares).
// Returns 0 when either side is missing; vestsToSteem then passes the amount
// through unchanged rather than fabricating a rate.
func vestsToSteemRatio(totalVestingFundSteem, totalVestingShares string) float64 {
	fund := parseAmount(totalVestingFundSteem)
	shares := parseAmount(totalVestingShares)
	if fund <= 0 || shares <= 0 {
		return 0
	}
	return fund / shares
}

// vestsToSteem converts a VESTS amount to STEEM with the given ratio. A zero
// ratio (missing props) leaves the value as-is — never a silent zero.
func vestsToSteem(vests, ratio float64) float64 {
	if ratio <= 0 {
		return vests
	}
	return vests * ratio
}

// benefactorWindowStart returns the $match lower bound for the benefactor
// leaderboard: UTC midnight 9 days back, i.e. the 10 daily buckets ending
// today.
//
// The endpoint used to aggregate benefactor_reward with no $match at all
// (~7.1M documents, 413k (benefactor, day) groups). That $group needed
// >100MB — MongoDB code 292 QueryExceededMemoryLimitNoDiskUseAllowed, the
// endpoint's 500 — and the eventual `$sort {year:1, doy:1}` + `$limit 10`
// returned the OLDEST 10 days (April 2017) instead of the recent ones.
// The window also drops the stray `_ts = 0001-01-01` documents written by a
// cold-start replay with an uninitialised block timestamp.
func benefactorWindowStart(now time.Time) time.Time {
	midnight := now.UTC().Truncate(24 * time.Hour)
	return midnight.AddDate(0, 0, -9)
}

// accountSummaryFromLookup projects the account array produced by a $lookup
// join into the tolerant AccountSummary view. Account documents are written
// by steemdb-sync from raw RPC values (withdrawn, curation_rewards,
// posting_rewards, proxied_vsf_votes and last_account_update stay strings,
// and their BSON numeric types vary between int32/int64/float64), so the
// join result must be decoded as bson.M and coerced field by field instead
// of into a rigid struct (P0-1, architecture constraint C1 — see
// AccountService.GetAccount / accountSummaryFromMap).
func accountSummaryFromLookup(docs []bson.M) *models.AccountSummary {
	if len(docs) == 0 {
		return nil
	}
	summary := accountSummaryFromMap(docs[0])
	return &summary
}

// toFloat64 coerces an aggregation output value (numeric, or an asset string
// like "100.000 STEEM" from older writers) to float64.
func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case string:
		return parseAmount(val)
	default:
		return getFloat64(v)
	}
}

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
	case int32:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		return parseAmount(val)
	default:
		return 0
	}
}
