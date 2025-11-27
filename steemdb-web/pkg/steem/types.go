package steem

import (
	"encoding/json"
	"fmt"
	"time"
)

// DynamicGlobalProperties represents the dynamic global properties
type DynamicGlobalProperties struct {
	HeadBlockNumber            int64  `json:"head_block_number"`
	HeadBlockID                string `json:"head_block_id"`
	Time                       string `json:"time"`
	CurrentWitness             string `json:"current_witness"`
	TotalPow                   int64  `json:"total_pow"`
	NumPowWitnesses            int    `json:"num_pow_witnesses"`
	VirtualSupply              string `json:"virtual_supply"`
	CurrentSupply              string `json:"current_supply"`
	ConfidentialSupply         string `json:"confidential_supply"`
	CurrentSBDSupply           string `json:"current_sbd_supply"`
	ConfidentialSBDSupply      string `json:"confidential_sbd_supply"`
	TotalVestingFundSteem      string `json:"total_vesting_fund_steem"`
	TotalVestingShares         string `json:"total_vesting_shares"`
	TotalRewardFundSteem       string `json:"total_reward_fund_steem"`
	TotalRewardShares2         string `json:"total_reward_shares2"`
	PendingRewardedVestingShares string `json:"pending_rewarded_vesting_shares"`
	PendingRewardedVestingSteem  string `json:"pending_rewarded_vesting_steem"`
	SBDInterestRate            int    `json:"sbd_interest_rate"`
	SBDPrintRate               int    `json:"sbd_print_rate"`
	MaximumBlockSize           int    `json:"maximum_block_size"`
	CurrentAslot               int64  `json:"current_aslot"`
	RecentSlotsFilled          string `json:"recent_slots_filled"`
	ParticipationCount         int    `json:"participation_count"`
	LastIrreversibleBlockNum   int64  `json:"last_irreversible_block_num"`
	VotePowerReserveRate       int    `json:"vote_power_reserve_rate"`
}

// Block represents a blockchain block
type Block struct {
	Number           int64         `json:"number"`
	Previous         string        `json:"previous"`
	Timestamp        time.Time     `json:"timestamp"`
	Witness          string        `json:"witness"`
	TransactionRoot  string        `json:"transaction_merkle_root"`
	Extensions       []interface{} `json:"extensions"`
	WitnessSignature string        `json:"witness_signature"`
	Transactions     []Transaction `json:"transactions"`
	BlockID          string        `json:"block_id"`
	SigningKey       string        `json:"signing_key"`
	TransactionIDs   []string      `json:"transaction_ids"`
}

// Transaction represents a blockchain transaction
type Transaction struct {
	RefBlockNum    int           `json:"ref_block_num"`
	RefBlockPrefix int64         `json:"ref_block_prefix"`
	Expiration     time.Time     `json:"expiration"`
	Operations     []Operation   `json:"operations"`
	Extensions     []interface{} `json:"extensions"`
	Signatures     []string      `json:"signatures"`
	TransactionID  string        `json:"transaction_id"`
	BlockNum       int64         `json:"block_num"`
	TransactionNum int           `json:"transaction_num"`
}

// Operation represents a blockchain operation
type Operation struct {
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

// UnmarshalJSON custom unmarshaling for Operation
func (op *Operation) UnmarshalJSON(data []byte) error {
	var raw []interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if len(raw) != 2 {
		return fmt.Errorf("operation must have exactly 2 elements")
	}

	if opType, ok := raw[0].(string); ok {
		op.Type = opType
	} else {
		return fmt.Errorf("operation type must be string")
	}

	op.Value = raw[1]
	return nil
}

// Account represents a Steem account
type Account struct {
	ID                        int       `json:"id"`
	Name                      string    `json:"name"`
	Owner                     Authority `json:"owner"`
	Active                    Authority `json:"active"`
	Posting                   Authority `json:"posting"`
	MemoKey                   string    `json:"memo_key"`
	JsonMetadata              string    `json:"json_metadata"`
	Proxy                     string    `json:"proxy"`
	LastOwnerUpdate           time.Time `json:"last_owner_update"`
	LastAccountUpdate         time.Time `json:"last_account_update"`
	Created                   time.Time `json:"created"`
	Mined                     bool      `json:"mined"`
	RecoveryAccount           string    `json:"recovery_account"`
	LastAccountRecovery       time.Time `json:"last_account_recovery"`
	ResetAccount              string    `json:"reset_account"`
	CommentCount              int       `json:"comment_count"`
	LifetimeBandwidth         string    `json:"lifetime_bandwidth"`
	LifetimeVoteCount         int       `json:"lifetime_vote_count"`
	PostCount                 int       `json:"post_count"`
	CanVote                   bool      `json:"can_vote"`
	VotingManabar             Manabar   `json:"voting_manabar"`
	DownvoteManabar           Manabar   `json:"downvote_manabar"`
	VotingPower               int       `json:"voting_power"`
	Balance                   string    `json:"balance"`
	SavingsBalance            string    `json:"savings_balance"`
	SBDBalance                string    `json:"sbd_balance"`
	SBDSeconds                string    `json:"sbd_seconds"`
	SBDSecondsLastUpdate      time.Time `json:"sbd_seconds_last_update"`
	SBDLastInterestPayment    time.Time `json:"sbd_last_interest_payment"`
	SavingsSBDBalance         string    `json:"savings_sbd_balance"`
	SavingsSBDSecondsLastUpdate time.Time `json:"savings_sbd_seconds_last_update"`
	SavingsSBDLastInterestPayment time.Time `json:"savings_sbd_last_interest_payment"`
	SavingsWithdrawRequests   int       `json:"savings_withdraw_requests"`
	RewardSBDBalance          string    `json:"reward_sbd_balance"`
	RewardSteemBalance        string    `json:"reward_steem_balance"`
	RewardVestingBalance      string    `json:"reward_vesting_balance"`
	RewardVestingSteem        string    `json:"reward_vesting_steem"`
	VestingShares             string    `json:"vesting_shares"`
	DelegatedVestingShares    string    `json:"delegated_vesting_shares"`
	ReceivedVestingShares     string    `json:"received_vesting_shares"`
	VestingWithdrawRate       string    `json:"vesting_withdraw_rate"`
	NextVestingWithdrawal     time.Time `json:"next_vesting_withdrawal"`
	Withdrawn                 string    `json:"withdrawn"`
	ToWithdraw                string    `json:"to_withdraw"`
	WithdrawRoutes            int       `json:"withdraw_routes"`
	CurationRewards           string    `json:"curation_rewards"`
	PostingRewards            string    `json:"posting_rewards"`
	ProxiedVSFVotes           []string  `json:"proxied_vsf_votes"`
	WitnessesVotedFor         int       `json:"witnesses_voted_for"`
	LastPost                  time.Time `json:"last_post"`
	LastRootPost              time.Time `json:"last_root_post"`
	LastVoteTime              time.Time `json:"last_vote_time"`
	PostBandwidth             int       `json:"post_bandwidth"`
	PendingClaimedAccounts    int       `json:"pending_claimed_accounts"`
	Reputation                string    `json:"reputation"`
	Transfer                  bool      `json:"transfer"`
	MarketHistory             bool      `json:"market_history"`
	PostHistory               bool      `json:"post_history"`
	VoteHistory               bool      `json:"vote_history"`
	MarketBandwidth           int       `json:"market_bandwidth"`
	BlogBandwidth             int       `json:"blog_bandwidth"`
	ForumBandwidth            int       `json:"forum_bandwidth"`
	AverageBandwidth          string    `json:"average_bandwidth"`
	LifetimeBandwidthLimit    string    `json:"lifetime_bandwidth_limit"`
	AverageMarketBandwidth    string    `json:"average_market_bandwidth"`
	LifetimeMarketBandwidth   string    `json:"lifetime_market_bandwidth"`
	WitnessVotes              []string  `json:"witness_votes"`
}

// Authority represents account authority
type Authority struct {
	WeightThreshold int                    `json:"weight_threshold"`
	AccountAuths    [][]interface{}        `json:"account_auths"`
	KeyAuths        [][]interface{}        `json:"key_auths"`
}

// Manabar represents voting manabar
type Manabar struct {
	CurrentMana string `json:"current_mana"`
	LastUpdateTime int64 `json:"last_update_time"`
}

// Witness represents a witness
type Witness struct {
	ID                        int                    `json:"id"`
	Owner                     string                 `json:"owner"`
	CreatedTime               time.Time              `json:"created"`
	URL                       string                 `json:"url"`
	Votes                     string                 `json:"votes"`
	VirtualLastUpdate         string                 `json:"virtual_last_update"`
	VirtualPosition           string                 `json:"virtual_position"`
	VirtualScheduledTime      string                 `json:"virtual_scheduled_time"`
	TotalMissed               int                    `json:"total_missed"`
	LastAslot                 int                    `json:"last_aslot"`
	LastConfirmedBlockNum     int                    `json:"last_confirmed_block_num"`
	PowWorker                 int                    `json:"pow_worker"`
	SigningKey                string                 `json:"signing_key"`
	Props                     WitnessProps           `json:"props"`
	SBDExchangeRate           ExchangeRate           `json:"sbd_exchange_rate"`
	LastSBDExchangeUpdate     time.Time              `json:"last_sbd_exchange_update"`
	LastWork                  string                 `json:"last_work"`
	RunningVersion            string                 `json:"running_version"`
	HardforkVersionVote       string                 `json:"hardfork_version_vote"`
	HardforkTimeVote          time.Time              `json:"hardfork_time_vote"`
	AvailableWitnessSignatures int                   `json:"available_witness_signatures"`
}

// WitnessProps represents witness properties
type WitnessProps struct {
	AccountCreationFee string `json:"account_creation_fee"`
	MaximumBlockSize   int    `json:"maximum_block_size"`
	SBDInterestRate    int    `json:"sbd_interest_rate"`
}

// ExchangeRate represents exchange rate
type ExchangeRate struct {
	Base  string `json:"base"`
	Quote string `json:"quote"`
}
