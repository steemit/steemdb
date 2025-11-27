package database

import (
	"time"
)

// Block represents a blockchain block
type Block struct {
	ID           int64                    `bson:"_id" json:"id"`
	Number       int64                    `bson:"number" json:"number"`
	Timestamp    time.Time                `bson:"_ts" json:"timestamp"`
	Previous     string                   `bson:"previous" json:"previous"`
	Witness      string                   `bson:"witness" json:"witness"`
	Transactions []map[string]interface{} `bson:"transactions" json:"transactions"`
	Extensions   []interface{}            `bson:"extensions" json:"extensions"`
}

// Account represents a Steem account
type Account struct {
	ID                    string    `bson:"_id" json:"id"`
	Name                  string    `bson:"name" json:"name"`
	Created               time.Time `bson:"created" json:"created"`
	Reputation            int64     `bson:"reputation" json:"reputation"`
	VestingShares         float64   `bson:"vesting_shares" json:"vesting_shares"`
	Balance               float64   `bson:"balance" json:"balance"`
	SBDBalance            float64   `bson:"sbd_balance" json:"sbd_balance"`
	PostCount             int       `bson:"post_count" json:"post_count"`
	CommentCount          int       `bson:"comment_count" json:"comment_count"`
	VotingPower           int       `bson:"voting_power" json:"voting_power"`
	LastPost              time.Time `bson:"last_post" json:"last_post"`
	LastVoteTime          time.Time `bson:"last_vote_time" json:"last_vote_time"`
	NextVestingWithdrawal time.Time `bson:"next_vesting_withdrawal" json:"next_vesting_withdrawal"`
	VestingWithdrawRate   float64   `bson:"vesting_withdraw_rate" json:"vesting_withdraw_rate"`
	ProxyWitness          float64   `bson:"proxy_witness" json:"proxy_witness"`
	WitnessVotes          []string  `bson:"witness_votes" json:"witness_votes"`
	JsonMetadata          string    `bson:"json_metadata" json:"json_metadata"`
	Scanned               time.Time `bson:"scanned" json:"scanned"`
}

// AccountHistory represents account historical data
type AccountHistory struct {
	ID            string    `bson:"_id" json:"id"`
	Account       string    `bson:"account" json:"account"`
	Date          time.Time `bson:"date" json:"date"`
	Reputation    int64     `bson:"reputation" json:"reputation"`
	VestingShares float64   `bson:"vesting_shares" json:"vesting_shares"`
	Balance       float64   `bson:"balance" json:"balance"`
	SBDBalance    float64   `bson:"sbd_balance" json:"sbd_balance"`
	PostCount     int       `bson:"post_count" json:"post_count"`
	CommentCount  int       `bson:"comment_count" json:"comment_count"`
	VotingPower   int       `bson:"voting_power" json:"voting_power"`
}

// Comment represents a post or comment
type Comment struct {
	ID             string                 `bson:"_id" json:"id"`
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
}

// Vote represents a vote operation
type Vote struct {
	ID        string    `bson:"_id" json:"id"`
	Voter     string    `bson:"voter" json:"voter"`
	Author    string    `bson:"author" json:"author"`
	Permlink  string    `bson:"permlink" json:"permlink"`
	Weight    int       `bson:"weight" json:"weight"`
	Timestamp time.Time `bson:"_ts" json:"timestamp"`
	BlockNum  int64     `bson:"block_num" json:"block_num"`
}

// Transfer represents a transfer operation
type Transfer struct {
	ID        string    `bson:"_id" json:"id"`
	From      string    `bson:"from" json:"from"`
	To        string    `bson:"to" json:"to"`
	Amount    float64   `bson:"amount" json:"amount"`
	Currency  string    `bson:"type" json:"currency"`
	Memo      string    `bson:"memo" json:"memo"`
	Timestamp time.Time `bson:"_ts" json:"timestamp"`
	BlockNum  int64     `bson:"block_num" json:"block_num"`
}

// AuthorReward represents an author reward
type AuthorReward struct {
	ID            string    `bson:"_id" json:"id"`
	Author        string    `bson:"author" json:"author"`
	Permlink      string    `bson:"permlink" json:"permlink"`
	SBDPayout     float64   `bson:"sbd_payout" json:"sbd_payout"`
	SteemPayout   float64   `bson:"steem_payout" json:"steem_payout"`
	VestingPayout float64   `bson:"vesting_payout" json:"vesting_payout"`
	AppName       string    `bson:"app_name" json:"app_name"`
	AppVersion    string    `bson:"app_version" json:"app_version"`
	Timestamp     time.Time `bson:"_ts" json:"timestamp"`
	BlockNum      int64     `bson:"block_num" json:"block_num"`
}

// CurationReward represents a curation reward
type CurationReward struct {
	ID              string    `bson:"_id" json:"id"`
	Curator         string    `bson:"curator" json:"curator"`
	CommentAuthor   string    `bson:"comment_author" json:"comment_author"`
	CommentPermlink string    `bson:"comment_permlink" json:"comment_permlink"`
	Reward          float64   `bson:"reward" json:"reward"`
	Timestamp       time.Time `bson:"_ts" json:"timestamp"`
	BlockNum        int64     `bson:"block_num" json:"block_num"`
}

// VestingDeposit represents a vesting deposit operation
type VestingDeposit struct {
	ID        string    `bson:"_id" json:"id"`
	From      string    `bson:"from" json:"from"`
	To        string    `bson:"to" json:"to"`
	Amount    float64   `bson:"amount" json:"amount"`
	Timestamp time.Time `bson:"_ts" json:"timestamp"`
	BlockNum  int64     `bson:"block_num" json:"block_num"`
}

// VestingWithdraw represents a vesting withdraw operation
type VestingWithdraw struct {
	ID          string    `bson:"_id" json:"id"`
	FromAccount string    `bson:"from_account" json:"from_account"`
	ToAccount   string    `bson:"to_account" json:"to_account"`
	Deposited   float64   `bson:"deposited" json:"deposited"`
	Withdrawn   float64   `bson:"withdrawn" json:"withdrawn"`
	Timestamp   time.Time `bson:"_ts" json:"timestamp"`
	BlockNum    int64     `bson:"block_num" json:"block_num"`
}

// Convert represents a convert operation
type Convert struct {
	ID        string    `bson:"_id" json:"id"`
	Owner     string    `bson:"owner" json:"owner"`
	RequestID int       `bson:"requestid" json:"requestid"`
	Amount    float64   `bson:"amount" json:"amount"`
	Type      string    `bson:"type" json:"type"`
	Timestamp time.Time `bson:"_ts" json:"timestamp"`
	BlockNum  int64     `bson:"block_num" json:"block_num"`
}

// FeedPublish represents a feed publish operation
type FeedPublish struct {
	ID           string                 `bson:"_id" json:"id"`
	Publisher    string                 `bson:"publisher" json:"publisher"`
	ExchangeRate map[string]interface{} `bson:"exchange_rate" json:"exchange_rate"`
	Timestamp    time.Time              `bson:"_ts" json:"timestamp"`
	BlockNum     int64                  `bson:"block_num" json:"block_num"`
}

// WitnessVote represents a witness vote operation
type WitnessVote struct {
	ID        string    `bson:"_id" json:"id"`
	Account   string    `bson:"account" json:"account"`
	Witness   string    `bson:"witness" json:"witness"`
	Approve   bool      `bson:"approve" json:"approve"`
	Timestamp time.Time `bson:"_ts" json:"timestamp"`
	BlockNum  int64     `bson:"block_num" json:"block_num"`
}

// Witness represents a witness
type Witness struct {
	ID                     string    `bson:"_id" json:"id"`
	Owner                  string    `bson:"owner" json:"owner"`
	CreatedTime            time.Time `bson:"created" json:"created"`
	URL                    string    `bson:"url" json:"url"`
	Votes                  float64   `bson:"votes" json:"votes"`
	VirtualLastUpdate      float64   `bson:"virtual_last_update" json:"virtual_last_update"`
	VirtualPosition        float64   `bson:"virtual_position" json:"virtual_position"`
	VirtualScheduledTime   float64   `bson:"virtual_scheduled_time" json:"virtual_scheduled_time"`
	TotalMissed            int       `bson:"total_missed" json:"total_missed"`
	LastAslot              int       `bson:"last_aslot" json:"last_aslot"`
	LastConfirmedBlockNum  int       `bson:"last_confirmed_block_num" json:"last_confirmed_block_num"`
	SigningKey             string    `bson:"signing_key" json:"signing_key"`
	Props                  map[string]interface{} `bson:"props" json:"props"`
	SBDExchangeRate        map[string]interface{} `bson:"sbd_exchange_rate" json:"sbd_exchange_rate"`
	LastSBDExchangeUpdate  time.Time `bson:"last_sbd_exchange_update" json:"last_sbd_exchange_update"`
	LastWork               string    `bson:"last_work" json:"last_work"`
	RunningVersion         string    `bson:"running_version" json:"running_version"`
	HardforkVersionVote    string    `bson:"hardfork_version_vote" json:"hardfork_version_vote"`
	HardforkTimeVote       time.Time `bson:"hardfork_time_vote" json:"hardfork_time_vote"`
}

// WitnessHistory represents witness historical data
type WitnessHistory struct {
	ID      string    `bson:"_id" json:"id"`
	Owner   string    `bson:"owner" json:"owner"`
	Date    time.Time `bson:"date" json:"date"`
	Votes   float64   `bson:"votes" json:"votes"`
	Missed  int       `bson:"total_missed" json:"total_missed"`
	Created time.Time `bson:"created" json:"created"`
}

// WitnessMiss represents a witness miss event
type WitnessMiss struct {
	ID       string    `bson:"_id" json:"id"`
	Date     time.Time `bson:"date" json:"date"`
	Witness  string    `bson:"witness" json:"witness"`
	Increase int       `bson:"increase" json:"increase"`
	Total    int       `bson:"total" json:"total"`
}

// Follow represents a follow operation
type Follow struct {
	ID        string                 `bson:"_id" json:"id"`
	Follower  string                 `bson:"follower" json:"follower"`
	Following string                 `bson:"following" json:"following"`
	What      []string               `bson:"what" json:"what"`
	Data      map[string]interface{} `bson:"data" json:"data"`
	Timestamp time.Time              `bson:"_ts" json:"timestamp"`
	BlockNum  int64                  `bson:"block_num" json:"block_num"`
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

// Pow represents a proof of work operation
type Pow struct {
	ID        string                 `bson:"_id" json:"id"`
	Worker    string                 `bson:"worker" json:"worker"`
	Input     map[string]interface{} `bson:"input" json:"input"`
	Signature string                 `bson:"signature" json:"signature"`
	Work      map[string]interface{} `bson:"work" json:"work"`
	Timestamp time.Time              `bson:"_ts" json:"timestamp"`
	BlockNum  int64                  `bson:"block_num" json:"block_num"`
}

// FundsHistory represents reward fund history
type FundsHistory struct {
	ID               string    `bson:"_id" json:"id"`
	Name             string    `bson:"name" json:"name"`
	RewardBalance    float64   `bson:"reward_balance" json:"reward_balance"`
	RecentClaims     float64   `bson:"recent_claims" json:"recent_claims"`
	ContentConstant  float64   `bson:"content_constant" json:"content_constant"`
	PercentCuration  int       `bson:"percent_curation_rewards" json:"percent_curation_rewards"`
	PercentContent   int       `bson:"percent_content_rewards" json:"percent_content_rewards"`
	LastUpdate       time.Time `bson:"last_update" json:"last_update"`
	AuthorRewardCurve string    `bson:"author_reward_curve" json:"author_reward_curve"`
	CurationRewardCurve string  `bson:"curation_reward_curve" json:"curation_reward_curve"`
}

// Status represents system status
type Status struct {
	ID        string      `bson:"_id" json:"id"`
	Data      interface{} `bson:"data" json:"data"`
	UpdatedAt time.Time   `bson:"updated_at" json:"updated_at"`
}
