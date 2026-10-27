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

// AuthorLeaderboard represents author reward leaderboard entry
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
	Permlinks  []string        `json:"permlinks,omitempty"`
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

// Clients represents client statistics
type Clients struct {
	Dates   []ClientDate       `json:"dates"`
	Posts   map[string]int     `json:"posts"`
	Rewards map[string]float64 `json:"rewards"`
}

// ClientDate represents client data for a specific date
type ClientDate struct {
	Date    time.Time     `json:"date"`
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
