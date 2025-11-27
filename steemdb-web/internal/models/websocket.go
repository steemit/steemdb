package models

import "time"

// WebSocketMessage represents a WebSocket message
type WebSocketMessage struct {
	Type      string      `json:"type"`
	Channel   string      `json:"channel,omitempty"`
	Data      interface{} `json:"data,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}

// SubscriptionRequest represents a subscription request
type SubscriptionRequest struct {
	Action  string `json:"action"` // "subscribe" or "unsubscribe"
	Channel string `json:"channel"`
}

// BlockData represents real-time block data
type BlockData struct {
	Number       int64     `json:"number"`
	Timestamp    time.Time `json:"timestamp"`
	Witness      string    `json:"witness"`
	Transactions int       `json:"transactions"`
	Operations   int       `json:"operations"`
}

// PropsData represents blockchain properties
type PropsData struct {
	HeadBlockNumber          int64   `json:"head_block_number"`
	HeadBlockID              string  `json:"head_block_id"`
	Time                     string  `json:"time"`
	CurrentWitness           string  `json:"current_witness"`
	TotalPow                 int64   `json:"total_pow"`
	NumPowWitnesses          int     `json:"num_pow_witnesses"`
	VirtualSupply            string  `json:"virtual_supply"`
	CurrentSupply            string  `json:"current_supply"`
	ConfidentialSupply       string  `json:"confidential_supply"`
	CurrentSBDSupply         string  `json:"current_sbd_supply"`
	ConfidentialSBDSupply    string  `json:"confidential_sbd_supply"`
	TotalVestingFundSteem    string  `json:"total_vesting_fund_steem"`
	TotalVestingShares       string  `json:"total_vesting_shares"`
	TotalRewardFundSteem     string  `json:"total_reward_fund_steem"`
	TotalRewardShares2       string  `json:"total_reward_shares2"`
	PendingRewardedVestingShares string `json:"pending_rewarded_vesting_shares"`
	PendingRewardedVestingSteem  string `json:"pending_rewarded_vesting_steem"`
	SBDInterestRate          int     `json:"sbd_interest_rate"`
	SBDPrintRate             int     `json:"sbd_print_rate"`
	MaximumBlockSize         int     `json:"maximum_block_size"`
	CurrentAslot             int64   `json:"current_aslot"`
	RecentSlotsFilled        string  `json:"recent_slots_filled"`
	ParticipationCount       int     `json:"participation_count"`
	LastIrreversibleBlockNum int64   `json:"last_irreversible_block_num"`
	VotePowerReserveRate     int     `json:"vote_power_reserve_rate"`
}

// OperationData represents an operation notification
type OperationData struct {
	Type      string      `json:"type"`
	Block     int64       `json:"block"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
	Accounts  []string    `json:"accounts,omitempty"` // Affected accounts
}

// StateData represents global state information
type StateData struct {
	Accounts    int64 `json:"accounts"`
	Comments    int64 `json:"comments"`
	Witnesses   int64 `json:"witnesses"`
	LastBlock   int64 `json:"last_block"`
	LastUpdate  time.Time `json:"last_update"`
}
