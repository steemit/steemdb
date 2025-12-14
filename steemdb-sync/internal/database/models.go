package database

import (
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Block represents a blockchain block
type Block struct {
	ID               int64                    `bson:"_id" json:"id"`                    // 区块号
	Number           int64                    `bson:"number" json:"number"`             // 区块号（冗余）
	Hash             string                   `bson:"hash" json:"hash"`                 // 区块哈希（block_id）
	Previous         string                   `bson:"previous" json:"previous"`         // 前一区块哈希
	Timestamp        time.Time                `bson:"timestamp" json:"timestamp"`       // 时间戳
	Witness          string                   `bson:"witness" json:"witness"`           // 见证人
	TransactionCount int                      `bson:"tx_count" json:"tx_count"`         // 交易数量
	OperationCount   int                      `bson:"op_count" json:"op_count"`         // 操作数量
	Transactions     []map[string]interface{} `bson:"transactions" json:"transactions"` // 交易列表（用于 tx_id 查询）
	Extensions       []interface{}            `bson:"extensions" json:"extensions"`

	// 索引字段
	DateIndex string `bson:"date_idx" json:"date_idx"` // 日期索引 (YYYY-MM-DD)

	// 统计字段（物化）
	TransferCount int `bson:"transfer_count" json:"transfer_count"`
	VoteCount     int `bson:"vote_count" json:"vote_count"`
	CommentCount  int `bson:"comment_count" json:"comment_count"`
}

// Account represents a Steem account (完整匹配 condenser_api.get_accounts)
type Account struct {
	ID   string `bson:"_id" json:"id"` // 账户名
	Name string `bson:"name" json:"name"`

	// 权限信息
	Owner        bson.M `bson:"owner" json:"owner"`                 // owner权限
	Active       bson.M `bson:"active" json:"active"`               // active权限
	Posting      bson.M `bson:"posting" json:"posting"`             // posting权限
	MemoKey      string `bson:"memo_key" json:"memo_key"`           // memo key
	JsonMetadata string `bson:"json_metadata" json:"json_metadata"` // JSON元数据

	// 代理和恢复
	Proxy           string `bson:"proxy" json:"proxy"`                       // 代理账户
	RecoveryAccount string `bson:"recovery_account" json:"recovery_account"` // 恢复账户
	ResetAccount    string `bson:"reset_account" json:"reset_account"`       // 重置账户

	// 余额信息（从API获取，字符串格式）
	Balance              string `bson:"balance" json:"balance"`                               // STEEM余额 (格式: "100.000 STEEM")
	SavingsBalance       string `bson:"savings_balance" json:"savings_balance"`               // 储蓄余额
	SBDBalance           string `bson:"sbd_balance" json:"sbd_balance"`                       // SBD余额
	SavingsSBDBalance    string `bson:"savings_sbd_balance" json:"savings_sbd_balance"`       // 储蓄SBD余额
	RewardSBDBalance     string `bson:"reward_sbd_balance" json:"reward_sbd_balance"`         // 奖励SBD余额
	RewardSteemBalance   string `bson:"reward_steem_balance" json:"reward_steem_balance"`     // 奖励STEEM余额
	RewardVestingBalance string `bson:"reward_vesting_balance" json:"reward_vesting_balance"` // 奖励VESTS余额
	RewardVestingSteem   string `bson:"reward_vesting_steem" json:"reward_vesting_steem"`     // 奖励VESTS对应的STEEM

	// VESTS信息
	VestingShares          string    `bson:"vesting_shares" json:"vesting_shares"`                     // VESTS (格式: "100.000000 VESTS")
	DelegatedVestingShares string    `bson:"delegated_vesting_shares" json:"delegated_vesting_shares"` // 委托的VESTS
	ReceivedVestingShares  string    `bson:"received_vesting_shares" json:"received_vesting_shares"`   // 接收的VESTS
	VestingWithdrawRate    string    `bson:"vesting_withdraw_rate" json:"vesting_withdraw_rate"`       // VESTS提取速率
	NextVestingWithdrawal  time.Time `bson:"next_vesting_withdrawal" json:"next_vesting_withdrawal"`   // 下次VESTS提取时间
	Withdrawn              string    `bson:"withdrawn" json:"withdrawn"`                               // 已提取（字符串格式）
	ToWithdraw             string    `bson:"to_withdraw" json:"to_withdraw"`                           // 待提取（字符串格式）

	// SBD利息相关
	SBDSeconds                    string    `bson:"sbd_seconds" json:"sbd_seconds"`                             // SBD秒数
	SBDSecondsLastUpdate          time.Time `bson:"sbd_seconds_last_update" json:"sbd_seconds_last_update"`     // SBD秒数最后更新时间
	SBDLastInterestPayment        time.Time `bson:"sbd_last_interest_payment" json:"sbd_last_interest_payment"` // SBD最后利息支付时间
	SavingsSBDSeconds             string    `bson:"savings_sbd_seconds" json:"savings_sbd_seconds"`             // 储蓄SBD秒数
	SavingsSBDSecondsLastUpdate   time.Time `bson:"savings_sbd_seconds_last_update" json:"savings_sbd_seconds_last_update"`
	SavingsSBDLastInterestPayment time.Time `bson:"savings_sbd_last_interest_payment" json:"savings_sbd_last_interest_payment"`
	SavingsWithdrawRequests       int       `bson:"savings_withdraw_requests" json:"savings_withdraw_requests"` // 储蓄提取请求数

	// 投票和奖励
	VotingPower     int       `bson:"voting_power" json:"voting_power"`           // 投票权重 (0-10000)
	LastVoteTime    time.Time `bson:"last_vote_time" json:"last_vote_time"`       // 最后投票时间
	CanVote         bool      `bson:"can_vote" json:"can_vote"`                   // 是否可以投票
	CurationRewards int64     `bson:"curation_rewards" json:"curation_rewards"`   // 策展奖励
	PostingRewards  int64     `bson:"posting_rewards" json:"posting_rewards"`     // 发帖奖励
	ProxiedVSFVotes []string  `bson:"proxied_vsf_votes" json:"proxied_vsf_votes"` // 代理VESTS投票（字符串数组）

	// 见证人相关
	WitnessesVotedFor int      `bson:"witnesses_voted_for" json:"witnesses_voted_for"` // 投票的见证人数量
	WitnessVotes      []string `bson:"witness_votes" json:"witness_votes"`             // 见证人投票列表
	WithdrawRoutes    int      `bson:"withdraw_routes" json:"withdraw_routes"`         // 提取路由数

	// 统计字段（从API获取）
	PostCount         int `bson:"post_count" json:"post_count"`                   // 帖子数
	CommentCount      int `bson:"comment_count" json:"comment_count"`             // 评论数
	LifetimeVoteCount int `bson:"lifetime_vote_count" json:"lifetime_vote_count"` // 终身投票数

	// 时间字段
	Created             time.Time `bson:"created" json:"created"`                             // 创建时间
	LastOwnerUpdate     time.Time `bson:"last_owner_update" json:"last_owner_update"`         // 最后owner更新
	LastAccountUpdate   time.Time `bson:"last_account_update" json:"last_account_update"`     // 最后账户更新
	LastAccountRecovery time.Time `bson:"last_account_recovery" json:"last_account_recovery"` // 最后账户恢复
	LastPost            time.Time `bson:"last_post" json:"last_post"`                         // 最后发帖时间
	LastRootPost        time.Time `bson:"last_root_post" json:"last_root_post"`               // 最后根帖时间

	// 其他字段
	Mined          bool   `bson:"mined" json:"mined"`                     // 是否挖矿账户
	Reputation     string `bson:"reputation" json:"reputation"`           // 声誉（字符串格式）
	VestingBalance string `bson:"vesting_balance" json:"vesting_balance"` // VESTS余额对应的STEEM

	// 历史数据（可选，用于快速查询）
	TransferHistory []bson.M `bson:"transfer_history,omitempty" json:"transfer_history,omitempty"` // 转账历史（可选）
	MarketHistory   []bson.M `bson:"market_history,omitempty" json:"market_history,omitempty"`     // 市场历史（可选）
	PostHistory     []bson.M `bson:"post_history,omitempty" json:"post_history,omitempty"`         // 帖子历史（可选）
	VoteHistory     []bson.M `bson:"vote_history,omitempty" json:"vote_history,omitempty"`         // 投票历史（可选）
	OtherHistory    []bson.M `bson:"other_history,omitempty" json:"other_history,omitempty"`       // 其他历史（可选）
	TagsUsage       []bson.M `bson:"tags_usage,omitempty" json:"tags_usage,omitempty"`             // 标签使用（可选）
	GuestBloggers   []string `bson:"guest_bloggers,omitempty" json:"guest_bloggers,omitempty"`     // 客座博主（可选）

	// 索引字段
	NameLower string `bson:"name_lower" json:"name_lower"` // 小写账户名（搜索用）

	// 更新标记
	NeedsUpdate bool      `bson:"needs_update" json:"needs_update"` // 标记需要更新（操作处理时设置）
	LastUpdated time.Time `bson:"last_updated" json:"last_updated"` // 最后更新时间
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

	// 索引字段
	AuthorLower   string `bson:"author_lower" json:"author_lower"`
	CategoryLower string `bson:"category_lower" json:"category_lower"`
	DateIndex     string `bson:"date_idx" json:"date_idx"`
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

// BenefactorReward represents a benefactor reward
type BenefactorReward struct {
	ID            string    `bson:"_id" json:"id"`
	Benefactor    string    `bson:"benefactor" json:"benefactor"`
	Author        string    `bson:"author" json:"author"`
	Permlink      string    `bson:"permlink" json:"permlink"`
	SBDPayout     float64   `bson:"sbd_payout" json:"sbd_payout"`
	SteemPayout   float64   `bson:"steem_payout" json:"steem_payout"`
	VestingPayout float64   `bson:"vesting_payout" json:"vesting_payout"`
	Timestamp     time.Time `bson:"_ts" json:"timestamp"`
	BlockNum      int64     `bson:"block_num" json:"block_num"`
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
	ID                    string                 `bson:"_id" json:"id"`
	Owner                 string                 `bson:"owner" json:"owner"`
	CreatedTime           time.Time              `bson:"created" json:"created"`
	URL                   string                 `bson:"url" json:"url"`
	Votes                 float64                `bson:"votes" json:"votes"`
	VirtualLastUpdate     float64                `bson:"virtual_last_update" json:"virtual_last_update"`
	VirtualPosition       float64                `bson:"virtual_position" json:"virtual_position"`
	VirtualScheduledTime  float64                `bson:"virtual_scheduled_time" json:"virtual_scheduled_time"`
	TotalMissed           int                    `bson:"total_missed" json:"total_missed"`
	LastAslot             int                    `bson:"last_aslot" json:"last_aslot"`
	LastConfirmedBlockNum int                    `bson:"last_confirmed_block_num" json:"last_confirmed_block_num"`
	SigningKey            string                 `bson:"signing_key" json:"signing_key"`
	Props                 map[string]interface{} `bson:"props" json:"props"`
	SBDExchangeRate       map[string]interface{} `bson:"sbd_exchange_rate" json:"sbd_exchange_rate"`
	LastSBDExchangeUpdate time.Time              `bson:"last_sbd_exchange_update" json:"last_sbd_exchange_update"`
	LastWork              string                 `bson:"last_work" json:"last_work"`
	RunningVersion        string                 `bson:"running_version" json:"running_version"`
	HardforkVersionVote   string                 `bson:"hardfork_version_vote" json:"hardfork_version_vote"`
	HardforkTimeVote      time.Time              `bson:"hardfork_time_vote" json:"hardfork_time_vote"`
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
	ID                  string    `bson:"_id" json:"id"`
	Name                string    `bson:"name" json:"name"`
	RewardBalance       float64   `bson:"reward_balance" json:"reward_balance"`
	RecentClaims        float64   `bson:"recent_claims" json:"recent_claims"`
	ContentConstant     float64   `bson:"content_constant" json:"content_constant"`
	PercentCuration     int       `bson:"percent_curation_rewards" json:"percent_curation_rewards"`
	PercentContent      int       `bson:"percent_content_rewards" json:"percent_content_rewards"`
	LastUpdate          time.Time `bson:"last_update" json:"last_update"`
	AuthorRewardCurve   string    `bson:"author_reward_curve" json:"author_reward_curve"`
	CurationRewardCurve string    `bson:"curation_reward_curve" json:"curation_reward_curve"`
}

// Status represents system status
type Status struct {
	ID        string      `bson:"_id" json:"id"`
	Data      interface{} `bson:"data" json:"data"`
	UpdatedAt time.Time   `bson:"updated_at" json:"updated_at"`
}

// Operation represents a blockchain operation
type Operation struct {
	ID        primitive.ObjectID `bson:"_id" json:"id"`
	BlockNum  int64              `bson:"block_num" json:"block_num"`
	BlockTime time.Time          `bson:"block_time" json:"block_time"`
	TrxID     string             `bson:"trx_id" json:"trx_id"`     // 交易ID（用于 tx_id 查询）
	OpType    string             `bson:"op_type" json:"op_type"`   // comment, vote, transfer等
	OpIndex   int                `bson:"op_index" json:"op_index"` // 操作在交易中的索引

	// 操作数据（反范式化）
	Data bson.M `bson:"data" json:"data"` // 操作具体数据

	// 账户索引（用于快速查询账户操作）
	Accounts       []string `bson:"accounts" json:"accounts"`               // 涉及的所有账户
	PrimaryAccount string   `bson:"primary_account" json:"primary_account"` // 主要账户

	// 时间索引
	DateIndex string `bson:"date_idx" json:"date_idx"` // YYYY-MM-DD
	HourIndex int    `bson:"hour_idx" json:"hour_idx"` // 0-23
}

// AccountOperation represents an account operation index for fast querying
type AccountOperation struct {
	ID        primitive.ObjectID `bson:"_id" json:"id"`
	Account   string             `bson:"account" json:"account"`
	BlockNum  int64              `bson:"block_num" json:"block_num"`
	BlockTime time.Time          `bson:"block_time" json:"block_time"`
	OpType    string             `bson:"op_type" json:"op_type"`
	OpID      primitive.ObjectID `bson:"op_id" json:"op_id"` // 指向 operations._id
	TrxID     string             `bson:"trx_id" json:"trx_id"`

	// 操作摘要（避免JOIN）
	Summary bson.M `bson:"summary" json:"summary"` // 操作关键信息
}

// OperationStats represents operation statistics (pre-computed)
type OperationStats struct {
	ID        string `bson:"_id" json:"id"` // op_type + date_idx + hour_idx
	OpType    string `bson:"op_type" json:"op_type"`
	DateIndex string `bson:"date_idx" json:"date_idx"` // YYYY-MM-DD
	HourIndex int    `bson:"hour_idx" json:"hour_idx"` // 0-23

	Count          int64 `bson:"count" json:"count"`
	UniqueAccounts int64 `bson:"unique_accounts" json:"unique_accounts"`

	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
}

// GlobalStats represents global statistics
type GlobalStats struct {
	ID             string `bson:"_id" json:"id"` // "current"
	TotalAccounts  int64  `bson:"total_accounts" json:"total_accounts"`
	TotalPosts     int64  `bson:"total_posts" json:"total_posts"`
	TotalComments  int64  `bson:"total_comments" json:"total_comments"`
	TotalTransfers int64  `bson:"total_transfers" json:"total_transfers"`
	TotalVotes     int64  `bson:"total_votes" json:"total_votes"`

	LastBlockNum  int64     `bson:"last_block_num" json:"last_block_num"`
	LastBlockTime time.Time `bson:"last_block_time" json:"last_block_time"`

	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
}
