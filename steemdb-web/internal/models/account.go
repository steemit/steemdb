package models

import (
	"time"
)

// Account represents a Steem account
type Account struct {
	ID                     string                 `json:"id" bson:"_id"`
	Name                   string                 `json:"name" bson:"name"`
	Owner                  map[string]interface{} `json:"owner" bson:"owner"`
	Active                 map[string]interface{} `json:"active" bson:"active"`
	Posting                map[string]interface{} `json:"posting" bson:"posting"`
	MemoKey                string                 `json:"memo_key" bson:"memo_key"`
	JSONMetadata           string                 `json:"json_metadata" bson:"json_metadata"`
	PostingJSONMetadata    string                 `json:"posting_json_metadata" bson:"posting_json_metadata"`
	Proxy                  string                 `json:"proxy" bson:"proxy"`
	LastOwnerUpdate        time.Time              `json:"last_owner_update" bson:"last_owner_update"`
	LastAccountUpdate      time.Time              `json:"last_account_update" bson:"last_account_update"`
	Created                time.Time              `json:"created" bson:"created"`
	Mined                  bool                   `json:"mined" bson:"mined"`
	RecoveryAccount        string                 `json:"recovery_account" bson:"recovery_account"`
	LastAccountRecovery    time.Time              `json:"last_account_recovery" bson:"last_account_recovery"`
	ResetAccount           string                 `json:"reset_account" bson:"reset_account"`
	CommentCount           int                    `json:"comment_count" bson:"comment_count"`
	LifetimeVoteCount      int                    `json:"lifetime_vote_count" bson:"lifetime_vote_count"`
	PostCount              int                    `json:"post_count" bson:"post_count"`
	CanVote                bool                   `json:"can_vote" bson:"can_vote"`
	VotingManabar          map[string]interface{} `json:"voting_manabar" bson:"voting_manabar"`
	DownvoteManbar         map[string]interface{} `json:"downvote_manabar" bson:"downvote_manabar"`
	VotingPower            int                    `json:"voting_power" bson:"voting_power"`
	Balance                float64                `json:"balance" bson:"balance"`
	SBDBalance             float64                `json:"sbd_balance" bson:"sbd_balance"`
	VestingShares          float64                `json:"vesting_shares" bson:"vesting_shares"`
	DelegatedVestingShares float64                `json:"delegated_vesting_shares" bson:"delegated_vesting_shares"`
	ReceivedVestingShares  float64                `json:"received_vesting_shares" bson:"received_vesting_shares"`
	VestingWithdrawRate    float64                `json:"vesting_withdraw_rate" bson:"vesting_withdraw_rate"`
	NextVestingWithdrawal  time.Time              `json:"next_vesting_withdrawal" bson:"next_vesting_withdrawal"`
	Withdrawn              int64                  `json:"withdrawn" bson:"withdrawn"`
	ToWithdraw             int64                  `json:"to_withdraw" bson:"to_withdraw"`
	WithdrawRoutes         int                    `json:"withdraw_routes" bson:"withdraw_routes"`
	CurationRewards        int64                  `json:"curation_rewards" bson:"curation_rewards"`
	PostingRewards         int64                  `json:"posting_rewards" bson:"posting_rewards"`
	ProxiedVSFVotes        []int64                `json:"proxied_vsf_votes" bson:"proxied_vsf_votes"`
	WitnessesVotedFor      int                    `json:"witnesses_voted_for" bson:"witnesses_voted_for"`
	WitnessVotes           []string               `json:"witness_votes" bson:"witness_votes"`
	LastPost               time.Time              `json:"last_post" bson:"last_post"`
	LastRootPost           time.Time              `json:"last_root_post" bson:"last_root_post"`
	LastVoteTime           time.Time              `json:"last_vote_time" bson:"last_vote_time"`
	PostBandwidth          int64                  `json:"post_bandwidth" bson:"post_bandwidth"`
	PendingClaimedAccounts int                    `json:"pending_claimed_accounts" bson:"pending_claimed_accounts"`
	Reputation             int64                  `json:"reputation" bson:"reputation"`
	LastUpdate             time.Time              `json:"last_update" bson:"last_update"`
	Scanned                time.Time              `json:"scanned" bson:"scanned"`
}

// AccountSummary represents a simplified account view for lists
type AccountSummary struct {
	Name          string    `json:"name"`
	Reputation    int64     `json:"reputation"`
	VestingShares float64   `json:"vesting_shares"`
	Balance       float64   `json:"balance"`
	SBDBalance    float64   `json:"sbd_balance"`
	PostCount     int       `json:"post_count"`
	LastPost      time.Time `json:"last_post"`
	Created       time.Time `json:"created"`
}

// AccountStats represents account statistics
type AccountStats struct {
	TotalAccounts    int64   `json:"total_accounts"`
	ActiveAccounts   int64   `json:"active_accounts"`
	NewAccountsToday int64   `json:"new_accounts_today"`
	TotalVests       float64 `json:"total_vests"`
	TotalSteem       float64 `json:"total_steem"`
	TotalSBD         float64 `json:"total_sbd"`
}

// AccountSearchResult represents search results
type AccountSearchResult struct {
	Accounts []AccountSummary `json:"accounts"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
}

// AccountOperation represents an operations-collection document projected
// into a per-account history entry (backed by the accounts array + multikey
// index written by steemdb-sync)
type AccountOperation struct {
	ID        string                 `json:"id" bson:"_id"`
	Account   string                 `json:"account" bson:"account"`
	BlockNum  int64                  `json:"block_num" bson:"block_num"`
	BlockTime time.Time              `json:"block_time" bson:"block_time"`
	OpType    string                 `json:"op_type" bson:"op_type"`
	TrxID     string                 `json:"trx_id" bson:"trx_id"`
	Summary   map[string]interface{} `json:"summary" bson:"summary"`
}

// AccountHistoryResult represents account operation history search results
type AccountHistoryResult struct {
	Operations []AccountOperation `json:"operations"`
	Total      int64              `json:"total"`
	Page       int                `json:"page"`
	PageSize   int                `json:"page_size"`
	TotalPages int                `json:"total_pages"`
}
