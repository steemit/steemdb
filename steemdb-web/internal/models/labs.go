package models

import "time"

// PowerUp represents a power up (vesting deposit) entry. Total is the STEEM
// sum over every deposit in the window; Instances is the *distinct* amount set
// of that window (legacy shape, unused by the SPA) — never derive Total from
// it. Account is a tolerant summary projection: account documents written by
// steemdb-sync keep raw RPC values (string ints/dates) that a rigid struct
// cannot decode.
type PowerUp struct {
	User      string          `json:"user"`
	Count     int             `json:"count"`
	Total     float64         `json:"total"`
	Instances []float64       `json:"instances,omitempty"`
	Account   *AccountSummary `json:"account,omitempty"`
}

// PowerDown represents power down statistics
type PowerDown struct {
	UpcomingTotal float64         `json:"upcoming_total"`
	Upcoming      []PowerDownDay  `json:"upcoming"`
	PreviousTotal float64         `json:"previous_total"`
	Previous      []PowerDownDay  `json:"previous"`
	PowerDowns    []PowerDownUser `json:"powerdowns"`
	Props         PowerDownProps  `json:"props"`
}

// PowerDownDay represents daily power down statistics. Withdrawn is STEEM
// (converted from the on-chain VESTS rate), Deposited is the exact STEEM paid
// out by the operations themselves.
type PowerDownDay struct {
	DayOfYear int     `json:"doy"`
	Year      int     `json:"year"`
	Month     int     `json:"month"`
	Day       int     `json:"day"`
	DayOfWeek int     `json:"dow"`
	Count     int     `json:"count"`
	Withdrawn float64 `json:"withdrawn"`
	Deposited float64 `json:"deposited,omitempty"`
}

// PowerDownUser represents user power down statistics. Withdrawn is STEEM
// (converted from VESTS), Deposited is the exact STEEM paid out.
type PowerDownUser struct {
	User        string          `json:"user"`
	Count       int             `json:"count"`
	Withdrawn   float64         `json:"withdrawn"`
	Deposited   float64         `json:"deposited"`
	DepositedTo []string        `json:"deposited_to,omitempty"`
	Account     *AccountSummary `json:"account,omitempty"`
}

// PowerDownProps represents blockchain properties for power down. All three
// are STEEM: Vesting is total_vesting_fund_steem (the STEEM locked in the
// vesting fund, not total_vesting_shares), Liquid is current_supply minus the
// vesting fund and the pending (already earmarked) vesting rewards.
type PowerDownProps struct {
	Current float64 `json:"current"`
	Vesting float64 `json:"vesting"`
	Liquid  float64 `json:"liquid"`
}

// RsharesAllocation represents rshares allocation data
type RsharesAllocation struct {
	Voter   string          `json:"voter"`
	Votes   int             `json:"votes"`
	Rshares int64           `json:"rshares"`
	Account *AccountSummary `json:"account,omitempty"`
}

// RsharesBoard is the /labs/rshares response.
//
// The totals are computed before the row cap is applied: the SPA derives each
// row's share of the day from TotalRshares, and computing that from a
// truncated array would make the shares of a "top 200" board add up to far more
// than 100%.
//
// VotesRefreshedAt is the newest `comment.rescanned_at` among the day's posts:
// the instant the comment_rescanner last refreshed active_votes for this set of
// posts. It is deliberately NOT `comment.scanned` — that field is also written
// by the comment handler on every comment op (edits included), so its maximum
// is "the newest write to the day's documents", not a snapshot time, and an
// ongoing day would claim to be fresh while its last hours of votes were still
// missing. nil when no post of that day has been rescanned yet; the SPA must
// then make no freshness claim at all.
type RsharesBoard struct {
	Allocations      []RsharesAllocation `json:"allocations"`
	TotalRshares     int64               `json:"total_rshares"`
	TotalVotes       int64               `json:"total_votes"`
	Voters           int                 `json:"voters"`
	Limit            int                 `json:"limit"`
	VotesRefreshedAt *time.Time          `json:"votes_refreshed_at,omitempty"`
}

// CurationLeaderboard represents curation reward leaderboard entry. Total is
// VESTS (curation_reward.reward); Permlinks is capped per curator by the
// service and is not rendered by the SPA.
type CurationLeaderboard struct {
	Curator   string          `json:"curator"`
	Count     int             `json:"count"`
	Total     float64         `json:"total"`
	Authors   []string        `json:"authors,omitempty"`
	Permlinks []string        `json:"permlinks,omitempty"`
	Account   *AccountSummary `json:"account,omitempty"`
}

// AuthorLeaderboard represents author reward leaderboard entry. The reward
// legs are VESTS (Vest/PostVest/ReplyVest), STEEM (Steem/PostSteem/
// ReplySteem) and SBD (Sbd/PostSbd/ReplySbd); TotalSteem is their STEEM
// equivalent at the current vests ratio and median SBD price, and is the value
// the board is ranked by.
type AuthorLeaderboard struct {
	Author     string          `json:"author"`
	Count      int             `json:"count"`
	Posts      int             `json:"posts"`
	Replies    int             `json:"replies"`
	PostVest   float64         `json:"post_vest"`
	PostSbd    float64         `json:"post_sbd"`
	PostSteem  float64         `json:"post_steem"`
	ReplyVest  float64         `json:"reply_vest"`
	ReplySbd   float64         `json:"reply_sbd"`
	ReplySteem float64         `json:"reply_steem"`
	Sbd        float64         `json:"sbd"`
	Steem      float64         `json:"steem"`
	Vest       float64         `json:"vest"`
	TotalSteem float64         `json:"total_steem"`
	Account    *AccountSummary `json:"account,omitempty"`
}

// Flags represents flagged accounts statistics, served from the
// status.flags-snapshot document maintained by steemdb-sync's refresher.
// Count is the downvote op count (an op log, so one voter/one post can
// contribute several), VoterCount/PostCount are the distinct voter and post
// totals, and Voters carries the top downvoters with their op counts.
type Flags struct {
	Author     string         `json:"author"`
	Count      int            `json:"count"`
	VoterCount int            `json:"voter_count"`
	PostCount  int            `json:"post_count"`
	Voters     map[string]int `json:"voters,omitempty"`
}

// Clients represents client statistics.
//
// Posts/Rewards are the whole-window totals per client (the SPA renders the
// top 50 of each); Rewards is SBD, the sum of the three payout legs written by
// steemdb-sync (total_payout_value + pending_payout_value +
// total_pending_payout_value — see the clients refresher for why all three are
// kept). Dates carries the per-day series and is only filled when the caller
// asks for it — it was 98% of a 236KB payload that nothing rendered.
//
// The metadata fields come from the snapshot document and exist so a reader can
// tell a stale snapshot (UpdatedAt) from a fresh one and see how much of the
// window the client attribution actually covers (PostsWithApp of
// PostsScanned). Both counters count COMMENTS (posts + replies), and
// PostsWithApp uses the same attribution predicate as the per-client lists, so
// a document counted there is a document the lists contain.
type Clients struct {
	Dates   []ClientDate       `json:"dates,omitempty"`
	Posts   map[string]int     `json:"posts"`
	Rewards map[string]float64 `json:"rewards"`

	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
	WindowFrom   *time.Time `json:"window_from,omitempty"`
	WindowTo     *time.Time `json:"window_to,omitempty"`
	PostsScanned int        `json:"posts_scanned"`
	PostsWithApp int        `json:"posts_with_app"`
}

// ClientDate represents client data for a specific date. Total/Reward are the
// day's own sums (all clients), which the snapshot stores.
type ClientDate struct {
	Date    time.Time     `json:"date"`
	Total   int           `json:"total"`
	Reward  float64       `json:"reward"`
	Clients []ClientEntry `json:"clients"`
}

// ClientEntry represents a client entry
type ClientEntry struct {
	Client string  `json:"client"`
	Count  int     `json:"count"`
	Reward float64 `json:"reward"`
}

// Benefactors represents benefactor reward statistics
type Benefactors struct {
	Dates []BenefactorDate `json:"dates"`
}

// BenefactorDate represents benefactor data for a specific date
type BenefactorDate struct {
	DayOfYear   int               `json:"doy"`
	Year        int               `json:"year"`
	Month       int               `json:"month"`
	Day         int               `json:"day"`
	DayOfWeek   int               `json:"dow"`
	Benefactors []BenefactorEntry `json:"benefactors"`
	Reward      float64           `json:"reward"`
	Total       int               `json:"total"`
}

// BenefactorEntry represents a benefactor entry. Reward is VESTS
// (comment_benefactor_reward.vesting_payout), never STEEM.
type BenefactorEntry struct {
	Benefactor string  `json:"benefactor"`
	Count      int     `json:"count"`
	Reward     float64 `json:"reward"`
}

// PendingPost represents a pending payout post
type PendingPost struct {
	ID                      string    `json:"id"`
	Author                  string    `json:"author"`
	Permlink                string    `json:"permlink"`
	Title                   string    `json:"title"`
	Created                 time.Time `json:"created"`
	PendingPayoutValue      float64   `json:"pending_payout_value"`
	TotalPendingPayoutValue float64   `json:"total_pending_payout_value"`
}
