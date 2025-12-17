package utils

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/steemit/steemutil/protocol"
	protocolapi "github.com/steemit/steemutil/protocol/api"
)

// ToTime safely converts protocol.Time to time.Time
func ToTime(pt protocol.Time) time.Time {
	if pt.Time != nil {
		return *pt.Time
	}
	return time.Time{}
}

// Block represents a blockchain block (simplified from protocolapi.Block)
type Block struct {
	Number           int64
	Previous         string
	Timestamp        time.Time
	Witness          string
	TransactionRoot  string
	Extensions       []interface{}
	WitnessSignature string
	Transactions     []Transaction
	BlockID          string
	SigningKey       string
	TransactionIDs   []string
}

// Transaction represents a blockchain transaction
type Transaction struct {
	RefBlockNum    int
	RefBlockPrefix int64
	Expiration     time.Time
	Operations     [][]interface{} // Format: [op_type, op_data]
	Extensions     []interface{}
	Signatures     []string
	TransactionID  string
	BlockNum       int64
	TransactionNum int
}

// Operation represents a blockchain operation
type Operation struct {
	TrxID      string
	Block      int64
	TrxInBlock int
	OpInTrx    int
	VirtualOp  int
	Timestamp  time.Time
	Op         []interface{} // Format: [op_type, op_data]
}

// ConvertBlock converts protocolapi.Block to our Block type
func ConvertBlock(block *protocolapi.Block, blockNum int64) *Block {
	if block == nil {
		return nil
	}

	// Convert transactions
	transactions := make([]Transaction, len(block.Transactions))
	for i, tx := range block.Transactions {
		transactions[i] = ConvertTransaction(&tx, blockNum, i)
	}

	// Convert time
	var timestamp time.Time
	if block.Timestamp != nil && block.Timestamp.Time != nil {
		timestamp = *block.Timestamp.Time
	}

	return &Block{
		Number:           blockNum,
		Previous:         block.Previous,
		Timestamp:        timestamp,
		Witness:          block.Witness,
		TransactionRoot:  block.TransactionMerkleRoot,
		Extensions:       block.Extensions,
		WitnessSignature: block.WitnessSignature,
		Transactions:     transactions,
		BlockID:          block.BlockId,
		SigningKey:       block.SigningKey,
		TransactionIDs:   block.TransactionIds,
	}
}

// ConvertTransaction converts protocolapi.Transaction to our Transaction type
func ConvertTransaction(tx *protocolapi.Transaction, blockNum int64, txNum int) Transaction {
	var expiration time.Time
	if tx.Expiration != nil && tx.Expiration.Time != nil {
		expiration = *tx.Expiration.Time
	}

	// Convert operations to [][]interface{}
	ops := make([][]interface{}, len(tx.Operations))
	for i, op := range tx.Operations {
		opTypeStr := string(op.Type())
		ops[i] = []interface{}{opTypeStr, op.Data()}
	}

	return Transaction{
		RefBlockNum:    int(tx.RefBlockNum),
		RefBlockPrefix: int64(tx.RefBlockPrefix),
		Expiration:     expiration,
		Operations:     ops,
		Extensions:     tx.Extensions,
		Signatures:     tx.Signatures,
		TransactionID:  tx.TransactionId,
		BlockNum:       blockNum,
		TransactionNum: txNum,
	}
}

// Account, Witness, RewardFund, Content types for RPC calls
// These will be unmarshaled from JSON responses

// Account represents a Steem account (unmarshaled from JSON)
type Account struct {
	ID                            int           `json:"id"`
	Name                          string        `json:"name"`
	Owner                         Authority     `json:"owner"`
	Active                        Authority     `json:"active"`
	Posting                       Authority     `json:"posting"`
	MemoKey                       string        `json:"memo_key"`
	JsonMetadata                  string        `json:"json_metadata"`
	Proxy                         string        `json:"proxy"`
	LastOwnerUpdate               protocol.Time `json:"last_owner_update"`
	LastAccountUpdate             protocol.Time `json:"last_account_update"`
	Created                       protocol.Time `json:"created"`
	Mined                         bool          `json:"mined"`
	RecoveryAccount               string        `json:"recovery_account"`
	LastAccountRecovery           protocol.Time `json:"last_account_recovery"`
	ResetAccount                  string        `json:"reset_account"`
	CommentCount                  int           `json:"comment_count"`
	LifetimeBandwidth             string        `json:"lifetime_bandwidth"`
	LifetimeVoteCount             int           `json:"lifetime_vote_count"`
	PostCount                     int           `json:"post_count"`
	CanVote                       bool          `json:"can_vote"`
	VotingManabar                 Manabar       `json:"voting_manabar"`
	DownvoteManabar               Manabar       `json:"downvote_manabar"`
	VotingPower                   int           `json:"voting_power"`
	Balance                       string        `json:"balance"`
	SavingsBalance                string        `json:"savings_balance"`
	SBDBalance                    string        `json:"sbd_balance"`
	SBDSeconds                    string        `json:"sbd_seconds"`
	SBDSecondsLastUpdate          protocol.Time `json:"sbd_seconds_last_update"`
	SBDLastInterestPayment        protocol.Time `json:"sbd_last_interest_payment"`
	SavingsSBDBalance             string        `json:"savings_sbd_balance"`
	SavingsSBDSecondsLastUpdate   protocol.Time `json:"savings_sbd_seconds_last_update"`
	SavingsSBDLastInterestPayment protocol.Time `json:"savings_sbd_last_interest_payment"`
	SavingsWithdrawRequests       int           `json:"savings_withdraw_requests"`
	RewardSBDBalance              string        `json:"reward_sbd_balance"`
	RewardSteemBalance            string        `json:"reward_steem_balance"`
	RewardVestingBalance          string        `json:"reward_vesting_balance"`
	RewardVestingSteem            string        `json:"reward_vesting_steem"`
	VestingShares                 string        `json:"vesting_shares"`
	DelegatedVestingShares        string        `json:"delegated_vesting_shares"`
	ReceivedVestingShares         string        `json:"received_vesting_shares"`
	VestingWithdrawRate           string        `json:"vesting_withdraw_rate"`
	NextVestingWithdrawal         protocol.Time `json:"next_vesting_withdrawal"`
	Withdrawn                     string        `json:"withdrawn"`
	ToWithdraw                    string        `json:"to_withdraw"`
	WithdrawRoutes                int           `json:"withdraw_routes"`
	CurationRewards               int64         `json:"curation_rewards"`
	PostingRewards                int64         `json:"posting_rewards"`
	ProxiedVSFVotes               []string      `json:"proxied_vsf_votes"`
	WitnessesVotedFor             int           `json:"witnesses_voted_for"`
	LastPost                      protocol.Time `json:"last_post"`
	LastRootPost                  protocol.Time `json:"last_root_post"`
	LastVoteTime                  protocol.Time `json:"last_vote_time"`
	PostBandwidth                 int           `json:"post_bandwidth"`
	PendingClaimedAccounts        int           `json:"pending_claimed_accounts"`
	Reputation                    string        `json:"reputation"`
	Transfer                      bool          `json:"transfer"`
	MarketHistory                 bool          `json:"market_history"`
	PostHistory                   bool          `json:"post_history"`
	VoteHistory                   bool          `json:"vote_history"`
	MarketBandwidth               int           `json:"market_bandwidth"`
	BlogBandwidth                 int           `json:"blog_bandwidth"`
	ForumBandwidth                int           `json:"forum_bandwidth"`
	AverageBandwidth              string        `json:"average_bandwidth"`
	LifetimeBandwidthLimit        string        `json:"lifetime_bandwidth_limit"`
	AverageMarketBandwidth        string        `json:"average_market_bandwidth"`
	LifetimeMarketBandwidth       string        `json:"lifetime_market_bandwidth"`
	WitnessVotes                  []string      `json:"witness_votes"`
}

// Authority represents account authority
type Authority struct {
	WeightThreshold int             `json:"weight_threshold"`
	AccountAuths    [][]interface{} `json:"account_auths"`
	KeyAuths        [][]interface{} `json:"key_auths"`
}

// Manabar represents voting manabar
type Manabar struct {
	CurrentMana    string `json:"current_mana"`
	LastUpdateTime int64  `json:"last_update_time"`
}

// UnmarshalJSON implements custom JSON unmarshaling for Manabar
func (m *Manabar) UnmarshalJSON(data []byte) error {
	type Alias Manabar
	aux := &struct {
		CurrentMana interface{} `json:"current_mana"`
		*Alias
	}{
		Alias: (*Alias)(m),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if aux.CurrentMana != nil {
		switch v := aux.CurrentMana.(type) {
		case string:
			m.CurrentMana = v
		case float64:
			m.CurrentMana = fmt.Sprintf("%.0f", v)
		case int64:
			m.CurrentMana = fmt.Sprintf("%d", v)
		case int:
			m.CurrentMana = fmt.Sprintf("%d", v)
		default:
			m.CurrentMana = fmt.Sprintf("%v", v)
		}
	}

	return nil
}

// Witness represents a witness
type Witness struct {
	ID                         int           `json:"id"`
	Owner                      string        `json:"owner"`
	CreatedTime                protocol.Time `json:"created"`
	URL                        string        `json:"url"`
	Votes                      string        `json:"votes"`
	VirtualLastUpdate          string        `json:"virtual_last_update"`
	VirtualPosition            string        `json:"virtual_position"`
	VirtualScheduledTime       string        `json:"virtual_scheduled_time"`
	TotalMissed                int           `json:"total_missed"`
	LastAslot                  int           `json:"last_aslot"`
	LastConfirmedBlockNum      int           `json:"last_confirmed_block_num"`
	PowWorker                  int           `json:"pow_worker"`
	SigningKey                 string        `json:"signing_key"`
	Props                      WitnessProps  `json:"props"`
	SBDExchangeRate            ExchangeRate  `json:"sbd_exchange_rate"`
	LastSBDExchangeUpdate      protocol.Time `json:"last_sbd_exchange_update"`
	LastWork                   string        `json:"last_work"`
	RunningVersion             string        `json:"running_version"`
	HardforkVersionVote        string        `json:"hardfork_version_vote"`
	HardforkTimeVote           protocol.Time `json:"hardfork_time_vote"`
	AvailableWitnessSignatures int           `json:"available_witness_signatures"`
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

// RewardFund represents reward fund information
type RewardFund struct {
	ID                     int           `json:"id"`
	Name                   string        `json:"name"`
	RewardBalance          string        `json:"reward_balance"`
	RecentClaims           string        `json:"recent_claims"`
	LastUpdate             protocol.Time `json:"last_update"`
	ContentConstant        string        `json:"content_constant"`
	PercentCurationRewards int           `json:"percent_curation_rewards"`
	PercentContentRewards  int           `json:"percent_content_rewards"`
	AuthorRewardCurve      string        `json:"author_reward_curve"`
	CurationRewardCurve    string        `json:"curation_reward_curve"`
}

// Content represents post/comment content
type Content struct {
	ID                      int             `json:"id"`
	Author                  string          `json:"author"`
	Permlink                string          `json:"permlink"`
	Category                string          `json:"category"`
	ParentAuthor            string          `json:"parent_author"`
	ParentPermlink          string          `json:"parent_permlink"`
	Title                   string          `json:"title"`
	Body                    string          `json:"body"`
	JsonMetadata            json.RawMessage `json:"json_metadata"`
	LastUpdate              time.Time       `json:"last_update"`
	Created                 time.Time       `json:"created"`
	Active                  time.Time       `json:"active"`
	LastPayout              time.Time       `json:"last_payout"`
	Depth                   int             `json:"depth"`
	Children                int             `json:"children"`
	NetRshares              string          `json:"net_rshares"`
	AbsRshares              string          `json:"abs_rshares"`
	VoteRshares             string          `json:"vote_rshares"`
	ChildrenAbsRshares      string          `json:"children_abs_rshares"`
	CashoutTime             time.Time       `json:"cashout_time"`
	MaxCashoutTime          time.Time       `json:"max_cashout_time"`
	TotalVoteWeight         string          `json:"total_vote_weight"`
	RewardWeight            int             `json:"reward_weight"`
	TotalPayoutValue        string          `json:"total_payout_value"`
	CuratorPayoutValue      string          `json:"curator_payout_value"`
	AuthorRewards           string          `json:"author_rewards"`
	NetVotes                int             `json:"net_votes"`
	RootComment             int             `json:"root_comment"`
	MaxAcceptedPayout       string          `json:"max_accepted_payout"`
	PercentSteemDollars     int             `json:"percent_steem_dollars"`
	AllowReplies            bool            `json:"allow_replies"`
	AllowVotes              bool            `json:"allow_votes"`
	AllowCurationRewards    bool            `json:"allow_curation_rewards"`
	Beneficiaries           []Beneficiary   `json:"beneficiaries"`
	URL                     string          `json:"url"`
	RootTitle               string          `json:"root_title"`
	PendingPayoutValue      string          `json:"pending_payout_value"`
	TotalPendingPayoutValue string          `json:"total_pending_payout_value"`
	ActiveVotes             []Vote          `json:"active_votes"`
	Replies                 []string        `json:"replies"`
	AuthorReputation        string          `json:"author_reputation"`
	Promoted                string          `json:"promoted"`
	BodyLength              int             `json:"body_length"`
	RebloggedBy            []string          `json:"reblogged_by"`
}

// Beneficiary represents a beneficiary
type Beneficiary struct {
	Account string `json:"account"`
	Weight  int    `json:"weight"`
}

// Vote represents a vote
type Vote struct {
	Voter   string    `json:"voter"`
	Weight  string    `json:"weight"`
	Rshares string    `json:"rshares"`
	Percent int       `json:"percent"`
	Time    time.Time `json:"time"`
}

