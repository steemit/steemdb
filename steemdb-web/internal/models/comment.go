package models

import "time"

// Comment represents a post or comment (aligned with steemdb-sync Comment model)
type Comment struct {
	ID             string                 `bson:"_id" json:"id"` // author/permlink
	Author         string                 `bson:"author" json:"author"`
	Permlink       string                 `bson:"permlink" json:"permlink"`
	Title          string                 `bson:"title" json:"title"`
	Body           string                 `bson:"body" json:"body"`
	JsonMetadata   map[string]interface{} `bson:"json_metadata" json:"json_metadata"`
	ParentAuthor   string                 `bson:"parent_author" json:"parent_author"`
	ParentPermlink string                 `bson:"parent_permlink" json:"parent_permlink"`
	Category       string                 `bson:"category" json:"category"`
	Depth          int                    `bson:"depth" json:"depth"`
	Children       int                    `bson:"children" json:"children"`
	Created        time.Time              `bson:"created" json:"created"`
	LastUpdate     time.Time              `bson:"last_update" json:"last_update"`
	CashoutTime    time.Time              `bson:"cashout_time" json:"cashout_time"`
	PendingPayout  float64                `bson:"pending_payout_value" json:"pending_payout_value"`
	TotalPayout    float64                `bson:"total_payout_value" json:"total_payout_value"`
	NetVotes       int                    `bson:"net_votes" json:"net_votes"`
	BlockNum       int64                  `bson:"block_num" json:"block_num"`
	Scanned        time.Time              `bson:"scanned" json:"scanned"`

	// Index fields
	AuthorLower   string `bson:"author_lower" json:"author_lower"`
	CategoryLower string `bson:"category_lower" json:"category_lower"`
	DateIndex     string `bson:"date_idx" json:"date_idx"`

	// Additional fields from legacy
	URL                     string  `bson:"url" json:"url,omitempty"`
	AuthorReputation        int64   `bson:"author_reputation" json:"author_reputation,omitempty"`
	TotalPendingPayoutValue float64 `bson:"total_pending_payout_value" json:"total_pending_payout_value,omitempty"`
	ActiveVotes             []Vote  `bson:"active_votes" json:"active_votes,omitempty"`
}

// Vote represents a vote on a comment
type Vote struct {
	Voter   string    `bson:"voter" json:"voter"`
	Weight  int64     `bson:"weight" json:"weight"`
	Rshares int64     `bson:"rshares" json:"rshares"`
	Percent int       `bson:"percent" json:"percent"`
	Time    time.Time `bson:"time" json:"time"`
}

// Reblog represents a reblog operation
type Reblog struct {
	ID        string    `bson:"_id" json:"id"`
	Account   string    `bson:"account" json:"account"`
	Author    string    `bson:"author" json:"author"`
	Permlink  string    `bson:"permlink" json:"permlink"`
	Timestamp time.Time `bson:"_ts" json:"timestamp"`
	BlockNum  int64     `bson:"block_num" json:"block_num"`
}

// CommentSearchResult represents search results for comments/posts
type CommentSearchResult struct {
	Data       []Comment `json:"data"`
	Total      int64     `json:"total"`
	Page       int       `json:"page"`
	PageSize   int       `json:"page_size"`
	TotalPages int       `json:"total_pages"`
}
