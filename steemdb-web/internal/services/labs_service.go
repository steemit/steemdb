package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/sync/singleflight"

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

	// Reward valuation cache (author leaderboard ranking). valMu guards the
	// cached snapshot; valGroup merges concurrent refreshes so a burst of
	// requests issues one props + price-feed round trip instead of one each.
	valMu     sync.Mutex
	val       rewardValuation
	valAt     time.Time
	valFailAt time.Time
	valGroup  singleflight.Group
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

	// The aggregation already returns the top 1000 by total; re-sorting stably
	// keeps equal totals in that order instead of depending on the Go map walk.
	sort.SliceStable(powerUps, func(i, j int) bool {
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
			// bson.D, not bson.M: MongoDB reads the field order of a $sort
			// document as precedence, while bson.M is an unordered map. With
			// a map the day-of-year key can end up primary, which misorders a
			// window that spans a year boundary (Dec 29..Jan 04).
			"$sort": bson.D{
				{Key: "_id.year", Value: 1},
				{Key: "_id.doy", Value: 1},
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
			// bson.D for the same reason as the upcoming pipeline above: a
			// two-key sort in a map has undefined precedence.
			"$sort": bson.D{
				{Key: "_id.year", Value: 1},
				{Key: "_id.doy", Value: 1},
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

// rsharesDefaultLimit / rsharesMaxLimit bound the returned voter rows. The
// board used to return every voter of the day (1,359-1,901 rows, 290-405KB)
// while top-10 concentrated 65% and top-100 95.8% of the rshares — truncation
// loses almost no signal, but the percentages must then be computed from
// server-side totals rather than from the truncated array.
const (
	rsharesDefaultLimit = 200
	rsharesMaxLimit     = 2000
)

// GetRsharesAllocation retrieves the rshares allocation of one UTC day.
//
// Unit/semantics contract: the figure is the rshares accumulated by posts
// CREATED on that day, including votes cast on later days (a post's payout
// window is 7 days, so the same historical day keeps growing). It is not
// "votes cast that day" — the SPA copy says so.
//
// Only positive rshares are summed. The previous filter was
// `active_votes.weight != 0`, which dropped downvotes only by accident:
// `weight` is a derived, scaled field, so every negative vote observed had
// weight 0 (and ~480 positive dust votes per day were dropped with it).
// Filtering on `rshares > 0` states the intent and keeps `votes` counting the
// same rows as the sum.
//
// `DataAsOf` is the newest `comment.scanned` among that day's posts: the
// comment_rescanner refreshes active_votes on a ~6h cycle, so the newest
// hours of an ongoing day are systematically missing (audit measured 36% of
// votes / 42% of rshares on the day of observation) and the SPA has to say so.
func (s *LabsService) GetRsharesAllocation(ctx context.Context, date time.Time, limit int) (*models.RsharesBoard, error) {
	commentCollection := s.db.Collection("comment")

	limit = clampLabsLimit(limit, rsharesDefaultLimit, rsharesMaxLimit)
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
			// Explicit positive filter: a downvote must not silently cancel
			// out an upvote in the total, and the vote count must describe the
			// same rows the rshares sum covers.
			"$match": bson.M{
				"active_votes.rshares": bson.M{"$gt": 0},
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
			// One pass, two outputs: the truncated top rows and the totals the
			// percentages are computed from. Slicing without carrying the
			// totals would make the SPA report every percentage against the
			// truncated sum (top-200 of 1,400: the shares would no longer add
			// up to 100%).
			"$facet": bson.M{
				"rows": []bson.M{
					// bson.D: a two-key $sort in an unordered map made the
					// tie-breaker (_id.voter) primary in some runs, so the
					// board intermittently showed voters in name order.
					{"$sort": bson.D{{Key: "rshares", Value: -1}, {Key: "_id.voter", Value: 1}}},
					{"$limit": limit},
					{
						"$lookup": bson.M{
							"from":         "account",
							"localField":   "_id.voter",
							"foreignField": "name",
							"as":           "account",
						},
					},
				},
				"meta": []bson.M{
					{
						"$group": bson.M{
							"_id":           nil,
							"total_rshares": bson.M{"$sum": "$rshares"},
							"total_votes":   bson.M{"$sum": "$votes"},
							"voters":        bson.M{"$sum": 1},
						},
					},
				},
			},
		},
	}

	cursor, err := commentCollection.Aggregate(ctx, pipeline, options.Aggregate().SetAllowDiskUse(true))
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate rshares: %w", err)
	}
	defer cursor.Close(ctx)

	// Decoded leniently (float64): active_votes[].rshares is written by
	// steemdb-sync, which stores it as a BSON double, while `$sum` over
	// integer-typed junk returns int64. Decoding both as float64 keeps the
	// rounding in our hands instead of the codec's (a fractional double would
	// fail a rigid int64 decode outright); rshares are whole numbers, so the
	// round on the way out is lossless.
	var facets []struct {
		Rows []struct {
			ID      bson.M   `bson:"_id"`
			Votes   int      `bson:"votes"`
			Rshares float64  `bson:"rshares"`
			Account []bson.M `bson:"account"`
		} `bson:"rows"`
		Meta []struct {
			TotalRshares float64 `bson:"total_rshares"`
			TotalVotes   int64   `bson:"total_votes"`
			Voters       int     `bson:"voters"`
		} `bson:"meta"`
	}
	if err := cursor.All(ctx, &facets); err != nil {
		return nil, fmt.Errorf("failed to decode rshares: %w", err)
	}

	board := &models.RsharesBoard{
		Allocations: make([]models.RsharesAllocation, 0, limit),
		Limit:       limit,
	}
	if len(facets) > 0 {
		if len(facets[0].Meta) > 0 {
			board.TotalRshares = int64(math.Round(facets[0].Meta[0].TotalRshares))
			board.TotalVotes = facets[0].Meta[0].TotalVotes
			board.Voters = facets[0].Meta[0].Voters
		}
		for _, r := range facets[0].Rows {
			voter, _ := r.ID["voter"].(string)
			board.Allocations = append(board.Allocations, models.RsharesAllocation{
				Voter:   voter,
				Votes:   r.Votes,
				Rshares: int64(math.Round(r.Rshares)),
				Account: accountSummaryFromLookup(r.Account),
			})
		}
	}

	board.DataAsOf = s.rsharesDataAsOf(ctx, startOfDay, endOfDay)

	return board, nil
}

// rsharesDataAsOf returns the newest comment.scanned timestamp among the posts
// created in the window — the instant the vote snapshot behind the numbers was
// taken. A nil result means no post of that day carries a snapshot yet.
func (s *LabsService) rsharesDataAsOf(ctx context.Context, start, end time.Time) *time.Time {
	var doc struct {
		Scanned time.Time `bson:"scanned"`
	}

	err := s.db.Collection("comment").FindOne(ctx,
		bson.M{
			"created": bson.M{"$gte": start, "$lt": end},
			"scanned": bson.M{"$exists": true, "$type": "date"},
		},
		options.FindOne().SetSort(bson.M{"scanned": -1}).SetProjection(bson.M{"scanned": 1}),
	).Decode(&doc)
	if err != nil || doc.Scanned.IsZero() {
		return nil
	}

	scanned := doc.Scanned.UTC()
	return &scanned
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

// Author leaderboard limits. The board used to return every rewarded account
// (622 rows for one day, 2,064 for a month) and carried a permlink array the
// SPA never rendered (65% of a 2.3MB monthly response).
const (
	authorDefaultLimit = 200
	authorMaxLimit     = 500
)

// GetAuthorLeaderboard retrieves the author reward leaderboard for one UTC day
// or month.
//
// Reward accounting: `vest`/`steem`/`sbd` are the three author_reward payout
// legs as written by steemdb-sync (VESTS / STEEM / SBD). `total_steem` is the
// STEEM-equivalent used for ranking — see rewardValuation — because ranking by
// VESTS alone while displaying only steem+sbd inverted the top of the board
// and showed "0.000 STEEM / 0.000 SBD" for 100% power-up authors.
func (s *LabsService) GetAuthorLeaderboard(ctx context.Context, date time.Time, grouping string, limit int) ([]models.AuthorLeaderboard, error) {
	valuation, err := s.rewardValuation(ctx)
	if err != nil {
		return nil, err
	}
	return s.authorLeaderboard(ctx, date, grouping, limit, valuation)
}

// authorLeaderboard is GetAuthorLeaderboard with the chain valuation supplied
// by the caller, so the aggregation can be exercised without a node.
//
// Reply classification: a payout is a reply when its comment document has a
// non-empty parent_author. The board used to test whether the permlink began
// with "re-", which is a client convention rather than a protocol rule: on
// 2026-10-26 that judged 164 of 876 payouts (18.7%) wrong — all replies
// counted as posts — under-reporting replies by 87% and mis-splitting 88 of
// 622 authors. parent_author is written by the comment handler for every
// comment op (no rescan needed) and comment._id is author/permlink, so the
// join is a primary-key lookup per payout.
func (s *LabsService) authorLeaderboard(ctx context.Context, date time.Time, grouping string, limit int, valuation rewardValuation) ([]models.AuthorLeaderboard, error) {
	collection := s.db.Collection("author_reward")

	limit = clampLabsLimit(limit, authorDefaultLimit, authorMaxLimit)

	var startTime, endTime time.Time
	if grouping == "monthly" {
		startTime = time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, date.Location())
		endTime = startTime.AddDate(0, 1, 0)
	} else {
		startTime = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
		endTime = startTime.Add(24 * time.Hour)
	}

	// splitByReply builds the two halves of a split metric: whenReply applies
	// to payouts classified as replies, whenPost to everything else.
	splitByReply := func(whenReply, whenPost interface{}) bson.M {
		return bson.M{"$sum": bson.M{"$cond": bson.A{"$is_reply", whenReply, whenPost}}}
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
				"author":         1,
				"permlink":       1,
				"steem_payout":   1,
				"vesting_payout": 1,
				"sbd_payout":     1,
			},
		},
		{
			// Only parent_author is fetched: the comment body would otherwise
			// be carried through a join that runs ~900 times a day and ~34k
			// times a month.
			"$lookup": bson.M{
				"from": "comment",
				"let":  bson.M{"cid": bson.M{"$concat": bson.A{"$author", "/", "$permlink"}}},
				"pipeline": []bson.M{
					{"$match": bson.M{"$expr": bson.M{"$eq": bson.A{"$_id", "$$cid"}}}},
					{"$project": bson.M{"parent_author": 1}},
				},
				"as": "comment",
			},
		},
		{
			// A payout whose comment document is missing (or not yet written)
			// counts as a post, which is the pre-fix behaviour for unknown
			// documents rather than a new error path.
			"$addFields": bson.M{
				"is_reply": bson.M{"$gt": bson.A{
					bson.M{"$strLenCP": bson.M{"$ifNull": bson.A{
						bson.M{"$arrayElemAt": bson.A{"$comment.parent_author", 0}},
						"",
					}}},
					0,
				}},
			},
		},
		{
			"$group": bson.M{
				"_id":     "$author",
				"count":   bson.M{"$sum": 1},
				"posts":   splitByReply(0, 1),
				"replies": splitByReply(1, 0),
				"post_vest": bson.M{"$sum": bson.M{"$cond": bson.A{
					"$is_reply", 0, "$vesting_payout",
				}}},
				"post_sbd": bson.M{"$sum": bson.M{"$cond": bson.A{
					"$is_reply", 0, "$sbd_payout",
				}}},
				"post_steem": bson.M{"$sum": bson.M{"$cond": bson.A{
					"$is_reply", 0, "$steem_payout",
				}}},
				"reply_vest": bson.M{"$sum": bson.M{"$cond": bson.A{
					"$is_reply", "$vesting_payout", 0,
				}}},
				"reply_sbd": bson.M{"$sum": bson.M{"$cond": bson.A{
					"$is_reply", "$sbd_payout", 0,
				}}},
				"reply_steem": bson.M{"$sum": bson.M{"$cond": bson.A{
					"$is_reply", "$steem_payout", 0,
				}}},
				"sbd":   bson.M{"$sum": "$sbd_payout"},
				"steem": bson.M{"$sum": "$steem_payout"},
				"vest":  bson.M{"$sum": "$vesting_payout"},
			},
		},
		{
			// The ranking metric: all three payout legs, in STEEM at the
			// current vests ratio and median SBD price.
			"$addFields": bson.M{
				"total_steem": valuation.steemEquivalentExpr("$vest", "$steem", "$sbd"),
			},
		},
		{
			// Accounts with an all-zero payout (817 of web2.support's monthly
			// rows carried no reward at all) are noise on a reward board.
			"$match": bson.M{"total_steem": bson.M{"$gt": 0}},
		},
		{
			// bson.D: with a map the _id tie-breaker can become the primary
			// key, which silently replaced "rank by STEEM equivalent" with
			// "alphabetical by author" in roughly one run out of five.
			"$sort": bson.D{
				{Key: "total_steem", Value: -1},
				{Key: "_id", Value: 1},
			},
		},
		{
			"$limit": limit,
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
		TotalSteem float64  `bson:"total_steem"`
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
			TotalSteem: r.TotalSteem,
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
// Snapshot shape: {_id: "clients-snapshot", updated_at, window_from, window_to,
// posts_scanned, posts_with_app, data: [{_id: {doy, year, month, day, dow},
// clients: [{client, count, reward}], reward, total}, ...]} — one data entry
// per day with apps seen in comment json_metadata.app over the last 90 days.
//
// includeDaily controls the per-day series. It is off by default because the
// series is ~98% of the response and the SPA renders only the per-client
// totals; callers that want to chart the timeline can ask for it.
func (s *LabsService) GetClients(ctx context.Context, includeDaily bool) (*models.Clients, error) {
	statusCollection := s.db.Collection("status")

	var status struct {
		ID           string          `bson:"_id"`
		Data         []clientDayData `bson:"data"`
		UpdatedAt    *time.Time      `bson:"updated_at"`
		WindowFrom   *time.Time      `bson:"window_from"`
		WindowTo     *time.Time      `bson:"window_to"`
		PostsScanned int             `bson:"posts_scanned"`
		PostsWithApp int             `bson:"posts_with_app"`
	}

	err := statusCollection.FindOne(ctx, bson.M{"_id": "clients-snapshot"}).Decode(&status)
	if err != nil {
		return nil, fmt.Errorf("failed to get clients snapshot: %w", err)
	}

	clients := &models.Clients{
		Posts:        make(map[string]int),
		Rewards:      make(map[string]float64),
		UpdatedAt:    utcPtr(status.UpdatedAt),
		WindowFrom:   utcPtr(status.WindowFrom),
		WindowTo:     utcPtr(status.WindowTo),
		PostsScanned: status.PostsScanned,
		PostsWithApp: status.PostsWithApp,
	}
	if includeDaily {
		clients.Dates = make([]models.ClientDate, 0, len(status.Data))
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
		if includeDaily {
			clients.Dates = append(clients.Dates, models.ClientDate{
				Date:    time.Date(day.ID.Year, time.Month(day.ID.Month), day.ID.Day, 0, 0, 0, 0, time.UTC),
				Total:   day.Total,
				Reward:  day.Reward,
				Clients: entries,
			})
		}
	}

	return clients, nil
}

// utcPtr normalises a snapshot timestamp to UTC (the snapshot window and its
// day buckets are UTC), passing nil through.
func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	utc := t.UTC()
	return &utc
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
			// bson.D: the day keys must stay ahead of `reward` in precedence.
			// In a map the order is random, and with `reward` primary the
			// per-day array would be ordered across days instead of within
			// each day.
			"$sort": bson.D{
				{Key: "_id.year", Value: -1},
				{Key: "_id.doy", Value: -1},
				{Key: "reward", Value: -1},
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
			// bson.D for ordering precedence across the two day keys.
			"$sort": bson.D{
				{Key: "_id.year", Value: -1},
				{Key: "_id.doy", Value: -1},
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

// labsValuationTTL bounds how often the author leaderboard asks the chain for
// its reward valuation (global props + median price feed). Conversions drift
// by <0.05% over a day, so a minute-old valuation ranks identically, while an
// uncached pair would put two public-node round trips inside every request.
const labsValuationTTL = 60 * time.Second

// labsValuationRetryTTL is how long a failed refresh suppresses the next
// attempt. Without it, a sick node turns every request into a fresh
// two-RPC call that takes up to 46s each (see steem.rpcMaxRetries): the
// endpoint would emit a steady stream of doomed requests instead of a few.
const labsValuationRetryTTL = 15 * time.Second

// labsValuationWait bounds how long a request waits on a refresh that is
// already in flight. The refresh itself is never abandoned (it warms the
// cache for later requests), but no request queues behind it: past this
// budget the caller is served the last good valuation, or fails.
const labsValuationWait = 3 * time.Second

var (
	// errValuationInFlight reports that a refresh was already running and this
	// request declined to queue behind it.
	errValuationInFlight = errors.New("labs reward valuation refresh already in flight")
	// errValuationRecentFailure reports that the last refresh failed less than
	// labsValuationRetryTTL ago.
	errValuationRecentFailure = errors.New("labs reward valuation refresh failed recently")
)

// rewardValuation is everything needed to state an author_reward payout in one
// unit. VESTS → STEEM uses the props ratio; SBD → STEEM uses the median price
// feed, because on Steem SBD does not trade at parity (2026-10: 1 SBD ≈ 9.6
// STEEM). Collapsing the two at 1:1 would misrank every author whose payout is
// mostly SBD.
type rewardValuation struct {
	// VestsRatio is STEEM per 1 VEST (total_vesting_fund_steem /
	// total_vesting_shares).
	VestsRatio float64
	// SteemPerSBD is STEEM per 1 SBD (the feed's quote/base).
	SteemPerSBD float64
}

// steemEquivalent adds the three payout legs in STEEM.
func (v rewardValuation) steemEquivalent(vests, steem, sbd float64) float64 {
	return vests*v.VestsRatio + steem + sbd*v.SteemPerSBD
}

// steemEquivalentExpr is steemEquivalent as an aggregation expression. Each
// operand is $ifNull-guarded: `$add` returns null if any operand is missing or
// non-numeric, and a null sort key would silently sink an author to the bottom
// of the board.
func (v rewardValuation) steemEquivalentExpr(vests, steem, sbd string) bson.M {
	guard := func(field string, factor float64) bson.M {
		return bson.M{"$ifNull": bson.A{
			bson.M{"$multiply": bson.A{field, factor}},
			0,
		}}
	}
	return bson.M{"$add": bson.A{
		guard(vests, v.VestsRatio),
		guard(steem, 1),
		guard(sbd, v.SteemPerSBD),
	}}
}

// rewardValuation returns the current reward valuation, refreshing at most once
// per labsValuationTTL.
//
// Failure behaviour is what keeps a sick node from becoming an endpoint-wide
// pile-up (the lock-then-RPC shape this replaced made every concurrent request
// wait behind the same refresh, and a hung node makes one refresh take up to
// 46s per RPC):
//
//   - concurrent refreshes are merged, so a burst costs one round trip;
//   - a request never queues behind an in-flight refresh for longer than
//     labsValuationWait — it is served the last good valuation instead;
//   - a failed refresh suppresses the next attempt for labsValuationRetryTTL.
//
// A last-good valuation is always preferred over an error (a minute-stale rate
// is within 0.05% of the truth); only a cold cache surfaces the error, so the
// board fails instead of ranking on a fabricated rate.
func (s *LabsService) rewardValuation(ctx context.Context) (rewardValuation, error) {
	if v, ok := s.freshValuation(); ok {
		return v, nil
	}

	if failedAt := s.lastValuationFailure(); !failedAt.IsZero() && time.Since(failedAt) < labsValuationRetryTTL {
		return s.cachedValuation(errValuationRecentFailure)
	}

	// DoChan (not Do) so the wait can be bounded: the refresh keeps running in
	// its own goroutine and warms the cache even when every waiter gives up.
	refreshed := s.valGroup.DoChan("rewardValuation", func() (interface{}, error) {
		return s.fetchValuation()
	})

	timer := time.NewTimer(labsValuationWait)
	defer timer.Stop()

	select {
	case res := <-refreshed:
		if res.Err != nil {
			return s.cachedValuation(res.Err)
		}
		v, _ := res.Val.(rewardValuation)
		return v, nil
	case <-timer.C:
		return s.cachedValuation(errValuationInFlight)
	case <-ctx.Done():
		return s.cachedValuation(ctx.Err())
	}
}

// freshValuation returns the cached valuation while it is still within
// labsValuationTTL.
func (s *LabsService) freshValuation() (rewardValuation, bool) {
	s.valMu.Lock()
	defer s.valMu.Unlock()
	if s.val.VestsRatio > 0 && s.val.SteemPerSBD > 0 && time.Since(s.valAt) < labsValuationTTL {
		return s.val, true
	}
	return rewardValuation{}, false
}

// lastValuationFailure returns when the last refresh failed (zero if it did
// not).
func (s *LabsService) lastValuationFailure() time.Time {
	s.valMu.Lock()
	defer s.valMu.Unlock()
	return s.valFailAt
}

// fetchValuation performs one refresh and records its outcome. Runs at most
// once per merge group, never holding valMu across the RPC.
func (s *LabsService) fetchValuation() (rewardValuation, error) {
	val, err := s.readValuationFromChain()
	if err != nil {
		s.valMu.Lock()
		s.valFailAt = time.Now()
		s.valMu.Unlock()
		return rewardValuation{}, err
	}

	s.valMu.Lock()
	s.val = val
	s.valAt = time.Now()
	s.valFailAt = time.Time{}
	s.valMu.Unlock()

	return val, nil
}

// readValuationFromChain is the two-call RPC half of a refresh.
func (s *LabsService) readValuationFromChain() (rewardValuation, error) {
	if s.steemClient == nil {
		return rewardValuation{}, errors.New("no steem client configured")
	}

	props, err := s.steemClient.GetDynamicGlobalProperties()
	if err != nil {
		return rewardValuation{}, fmt.Errorf("failed to get props: %w", err)
	}
	ratio := vestsToSteemRatio(props.TotalVestingFundSteem, props.TotalVestingShares)
	if ratio <= 0 {
		return rewardValuation{}, errors.New("props carry no vesting ratio")
	}

	steemPerSBD, err := s.steemClient.GetMedianHistoryPrice()
	if err != nil {
		return rewardValuation{}, fmt.Errorf("failed to get median price: %w", err)
	}

	return rewardValuation{VestsRatio: ratio, SteemPerSBD: steemPerSBD}, nil
}

// cachedValuation returns the last good valuation when a refresh failed,
// otherwise the refresh error.
func (s *LabsService) cachedValuation(cause error) (rewardValuation, error) {
	s.valMu.Lock()
	val := s.val
	s.valMu.Unlock()

	if val.VestsRatio > 0 && val.SteemPerSBD > 0 {
		if s.logger != nil {
			s.logger.Warn("Using stale labs reward valuation", utils.Error(cause))
		}
		return val, nil
	}
	return rewardValuation{}, cause
}

// clampLabsLimit applies a caller-supplied row cap with a default and a hard
// maximum, so `?limit=` can neither omit truncation nor request an unbounded
// response.
func clampLabsLimit(requested, def, max int) int {
	switch {
	case requested <= 0:
		return def
	case requested > max:
		return max
	default:
		return requested
	}
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
