package models

import "time"

// PowerUp represents a power up (vesting deposit) entry
type PowerUp struct {
	User      string   `json:"user"`
	Count     int      `json:"count"`
	Total     float64  `json:"total"`
	Instances []string `json:"instances,omitempty"`
	Account   *Account `json:"account,omitempty"`
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

// PowerDownDay represents daily power down statistics
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

// PowerDownUser represents user power down statistics
type PowerDownUser struct {
	User        string   `json:"user"`
	Count       int      `json:"count"`
	Withdrawn   float64  `json:"withdrawn"`
	Deposited   float64  `json:"deposited"`
	DepositedTo []string `json:"deposited_to,omitempty"`
	Account     *Account `json:"account,omitempty"`
}

// PowerDownProps represents blockchain properties for power down
type PowerDownProps struct {
	Current float64 `json:"current"`
	Vesting float64 `json:"vesting"`
	Liquid  float64 `json:"liquid"`
}

// RsharesAllocation represents rshares allocation data
type RsharesAllocation struct {
	Voter   string   `json:"voter"`
	Votes   int      `json:"votes"`
	Rshares int64    `json:"rshares"`
	Account *Account `json:"account,omitempty"`
}

// CurationLeaderboard represents curation reward leaderboard entry
type CurationLeaderboard struct {
	Curator   string   `json:"curator"`
	Count     int      `json:"count"`
	Total     float64  `json:"total"`
	Authors   []string `json:"authors,omitempty"`
	Permlinks []string `json:"permlinks,omitempty"`
	Account   *Account `json:"account,omitempty"`
}

// AuthorLeaderboard represents author reward leaderboard entry
type AuthorLeaderboard struct {
	Author     string   `json:"author"`
	Count      int      `json:"count"`
	Posts      int      `json:"posts"`
	Replies    int      `json:"replies"`
	PostVest   float64  `json:"post_vest"`
	PostSbd    float64  `json:"post_sbd"`
	PostSteem  float64  `json:"post_steem"`
	ReplyVest  float64  `json:"reply_vest"`
	ReplySbd   float64  `json:"reply_sbd"`
	ReplySteem float64  `json:"reply_steem"`
	Sbd        float64  `json:"sbd"`
	Steem      float64  `json:"steem"`
	Vest       float64  `json:"vest"`
	Permlinks  []string `json:"permlinks,omitempty"`
	Account    *Account `json:"account,omitempty"`
}

// Flags represents flagged accounts statistics
type Flags struct {
	Author   string         `json:"author"`
	Count    int            `json:"count"`
	Flaggers []string       `json:"flaggers,omitempty"`
	Posts    []string       `json:"posts,omitempty"`
	Voters   map[string]int `json:"voters,omitempty"`
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

// BenefactorEntry represents a benefactor entry
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
