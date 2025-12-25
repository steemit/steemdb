# SteemDB Sync 架构重构与查询优化计划

## 1. 设计目标

### 1.1 核心目标

- **查询效率优先**：所有设计围绕查询性能优化
- **简化架构**：单一 Block Sync 服务，移除复杂的定时任务
- **数据模型优化**：避免 JOIN，使用反范式化设计
- **索引优化**：为所有常见查询场景创建复合索引

### 1.2 查询场景分析

#### 高频查询场景

1. **区块查询**：

- 按区块号查询（最频繁）
- 最新区块列表（时间倒序）
- 按见证人查询区块

2. **账户查询**：

- 账户详情（按账户名）
- 账户操作历史（按账户名 + 时间范围）
- 账户创建的评论/帖子

3. **操作查询**：

- 按操作类型查询（转账、投票等）
- 按账户查询操作（账户 + 操作类型）
- 按时间范围查询

4. **评论查询**：

- 按作者查询
- 按分类查询
- 最新/热门评论

5. **搜索功能**：

- 账户名搜索（前缀匹配）
- 区块号搜索
- 交易ID搜索

## 2. 新架构设计

### 2.1 整体架构

```javascript
Steem RPC Nodes
    │
    ├─► condenser_api.get_accounts (账户信息)
    └─► get_ops_in_block (区块操作)
    │
    ▼
Block Sync Service (单 goroutine)
    │
    ├─► Block Fetcher (批量获取区块)
    ├─► Operation Processor (串行处理操作)
    │   └─► 标记账户 needs_update = true
    └─► Batch Writer (批量写入)
    │
    ▼
MongoDB (优化的数据模型)
    │
    ├─► blocks (区块数据)
    ├─► operations (操作历史)
    ├─► accounts (账户状态，完整匹配 get_accounts)
    ├─► comments (评论状态)
    ├─► account_operations (账户操作索引)
    ├─► operation_stats (操作统计)
    └─► global_stats (全局统计)
    │
    ▼
CronTab Service (单 goroutine)
    │
    ├─► 等待 Sync 追上最新区块
    ├─► Account Updater (批量调用 get_accounts 更新账户)
    ├─► Stats Updater (30天聚合数据计算)
    └─► Materialized View Builder (构建物化视图)
```



### 2.2 核心设计原则

1. **单 goroutine Block Sync**：

- 只使用一个 goroutine 进行区块同步
- 按区块顺序串行处理所有操作
- 保证操作时序正确性（避免下一个 transaction 的 operation 依赖上一个 transaction 的 operation 时出现时序问题）
- 批量获取区块，批量写入

2. **单 goroutine CronTab**：

- 只使用一个 goroutine 运行计划任务
- **关键**：只有在 Block Sync 追上最新区块后才开始工作
- 负责账户更新、30天聚合数据计算、物化视图更新等

3. **账户信息以 API 为准**：

- 账户信息以 `condenser_api.get_accounts` 的返回数据为准
- 操作处理时只标记账户需要更新（`needs_update = true`），不自己计算余额
- 定期批量调用 API 更新账户信息

4. **状态与历史分离**：

- 状态集合：当前状态（account, comment）
- 历史集合：操作历史（operations, account_operations）

5. **物化视图**：

- 预计算常用查询结果
- 定期更新，减少实时计算

6. **反范式化**：

- 避免 JOIN 操作
- 冗余关键字段，提升查询速度

## 3. 数据模型设计

### 3.1 区块集合 (blocks)

```go
type Block struct {
    ID          int64     `bson:"_id"`           // 区块号
    Number      int64     `bson:"number"`        // 区块号（冗余）
    Hash        string    `bson:"hash"`          // 区块哈希
    Previous    string    `bson:"previous"`      // 前一区块哈希
    Timestamp   time.Time `bson:"timestamp"`     // 时间戳
    Witness     string    `bson:"witness"`       // 见证人
    TransactionCount int  `bson:"tx_count"`      // 交易数量
    OperationCount   int  `bson:"op_count"`      // 操作数量
    
    // 索引字段
    WitnessIndex string   `bson:"witness_idx"`   // 见证人（用于索引）
    DateIndex    string   `bson:"date_idx"`      // 日期索引 (YYYY-MM-DD)
    
    // 统计字段（物化）
    TransferCount    int   `bson:"transfer_count"`
    VoteCount        int   `bson:"vote_count"`
    CommentCount     int   `bson:"comment_count"`
}
```

**索引**：

- `{number: 1}` - 主键索引
- `{timestamp: -1}` - 时间倒序（最新区块）
- `{witness: 1, timestamp: -1}` - 见证人区块查询
- `{date_idx: 1, timestamp: -1}` - 按日期查询

### 3.2 操作集合 (operations)

```go
type Operation struct {
    ID          primitive.ObjectID `bson:"_id"`
    BlockNum    int64              `bson:"block_num"`
    BlockTime   time.Time          `bson:"block_time"`
    TrxID       string             `bson:"trx_id"`
    OpType      string             `bson:"op_type"`      // comment, vote, transfer等
    OpIndex     int                `bson:"op_index"`     // 操作在交易中的索引
    
    // 操作数据（反范式化）
    Data        bson.M             `bson:"data"`         // 操作具体数据
    
    // 账户索引（用于快速查询账户操作）
    Accounts    []string           `bson:"accounts"`     // 涉及的所有账户
    PrimaryAccount string          `bson:"primary_account"` // 主要账户
    
    // 时间索引
    DateIndex   string             `bson:"date_idx"`     // YYYY-MM-DD
    HourIndex   int                `bson:"hour_idx"`     // 0-23
}
```

**索引**：

- `{block_num: 1, op_index: 1}` - 区块+操作索引
- `{op_type: 1, block_time: -1}` - 按操作类型查询
- `{primary_account: 1, block_time: -1}` - 账户操作查询
- `{accounts: 1, block_time: -1}` - 多账户操作查询
- `{date_idx: 1, hour_idx: 1}` - 时间范围查询
- `{trx_id: 1}` - 交易ID查询

### 3.3 账户操作索引 (account_operations)

**目的**：快速查询账户的操作历史，避免全表扫描

```go
type AccountOperation struct {
    ID          primitive.ObjectID `bson:"_id"`
    Account     string             `bson:"account"`
    BlockNum    int64              `bson:"block_num"`
    BlockTime   time.Time          `bson:"block_time"`
    OpType      string             `bson:"op_type"`
    OpID        primitive.ObjectID `bson:"op_id"`        // 指向 operations._id
    TrxID       string             `bson:"trx_id"`
    
    // 操作摘要（避免JOIN）
    Summary     bson.M             `bson:"summary"`      // 操作关键信息
}
```

**索引**：

- `{account: 1, block_time: -1}` - 账户操作历史（最重要）
- `{account: 1, op_type: 1, block_time: -1}` - 账户+操作类型
- `{account: 1, block_num: -1}` - 账户+区块号

### 3.4 账户集合 (accounts)

**数据来源**：以 `condenser_api.get_accounts` 的返回数据为准，完整存储所有字段。

```go
type Account struct {
    ID              string    `bson:"_id"`              // 账户名
    Name            string    `bson:"name"`
    
    // 权限信息
    Owner           bson.M   `bson:"owner"`            // owner权限
    Active          bson.M   `bson:"active"`           // active权限
    Posting         bson.M   `bson:"posting"`          // posting权限
    MemoKey         string   `bson:"memo_key"`         // memo key
    JsonMetadata    string   `bson:"json_metadata"`    // JSON元数据
    
    // 代理和恢复
    Proxy           string   `bson:"proxy"`            // 代理账户
    RecoveryAccount string   `bson:"recovery_account"` // 恢复账户
    ResetAccount   string   `bson:"reset_account"`    // 重置账户
    
    // 余额信息（从API获取）
    Balance         string   `bson:"balance"`           // STEEM余额 (格式: "100.000 STEEM")
    SavingsBalance  string   `bson:"savings_balance"`   // 储蓄余额
    SBDBalance      string   `bson:"sbd_balance"`       // SBD余额
    SavingsSBDBalance string `bson:"savings_sbd_balance"` // 储蓄SBD余额
    RewardSBDBalance string `bson:"reward_sbd_balance"`   // 奖励SBD余额
    RewardSteemBalance string `bson:"reward_steem_balance"` // 奖励STEEM余额
    RewardVestingBalance string `bson:"reward_vesting_balance"` // 奖励VESTS余额
    RewardVestingSteem string `bson:"reward_vesting_steem"`   // 奖励VESTS对应的STEEM
    
    // VESTS信息
    VestingShares   string   `bson:"vesting_shares"`    // VESTS (格式: "100.000000 VESTS")
    DelegatedVestingShares string `bson:"delegated_vesting_shares"` // 委托的VESTS
    ReceivedVestingShares string `bson:"received_vesting_shares"`  // 接收的VESTS
    VestingWithdrawRate string `bson:"vesting_withdraw_rate"`      // VESTS提取速率
    NextVestingWithdrawal time.Time `bson:"next_vesting_withdrawal"` // 下次VESTS提取时间
    Withdrawn       int64    `bson:"withdrawn"`         // 已提取
    ToWithdraw      int64    `bson:"to_withdraw"`      // 待提取
    
    // SBD利息相关
    SBDSeconds      string   `bson:"sbd_seconds"`       // SBD秒数
    SBDSecondsLastUpdate time.Time `bson:"sbd_seconds_last_update"` // SBD秒数最后更新时间
    SBDLastInterestPayment time.Time `bson:"sbd_last_interest_payment"` // SBD最后利息支付时间
    SavingsSBDSeconds string `bson:"savings_sbd_seconds"` // 储蓄SBD秒数
    SavingsSBDSecondsLastUpdate time.Time `bson:"savings_sbd_seconds_last_update"`
    SavingsSBDLastInterestPayment time.Time `bson:"savings_sbd_last_interest_payment"`
    SavingsWithdrawRequests int `bson:"savings_withdraw_requests"` // 储蓄提取请求数
    
    // 投票和奖励
    VotingPower     int      `bson:"voting_power"`      // 投票权重 (0-10000)
    LastVoteTime    time.Time `bson:"last_vote_time"`   // 最后投票时间
    CanVote         bool     `bson:"can_vote"`          // 是否可以投票
    CurationRewards int64    `bson:"curation_rewards"`  // 策展奖励
    PostingRewards  int64    `bson:"posting_rewards"`   // 发帖奖励
    ProxiedVSFVotes []int64  `bson:"proxied_vsf_votes"` // 代理VESTS投票
    
    // 见证人相关
    WitnessesVotedFor int     `bson:"witnesses_voted_for"` // 投票的见证人数量
    WitnessVotes      []string `bson:"witness_votes"`      // 见证人投票列表
    WithdrawRoutes    int     `bson:"withdraw_routes"`    // 提取路由数
    
    // 统计字段（从API获取）
    PostCount       int       `bson:"post_count"`        // 帖子数
    CommentCount    int       `bson:"comment_count"`    // 评论数
    LifetimeVoteCount int    `bson:"lifetime_vote_count"` // 终身投票数
    
    // 时间字段
    Created         time.Time `bson:"created"`           // 创建时间
    LastOwnerUpdate time.Time `bson:"last_owner_update"` // 最后owner更新
    LastAccountUpdate time.Time `bson:"last_account_update"` // 最后账户更新
    LastAccountRecovery time.Time `bson:"last_account_recovery"` // 最后账户恢复
    LastPost        time.Time `bson:"last_post"`        // 最后发帖时间
    LastRootPost    time.Time `bson:"last_root_post"`    // 最后根帖时间
    
    // 其他字段
    Mined           bool     `bson:"mined"`             // 是否挖矿账户
    Reputation      string   `bson:"reputation"`        // 声誉（字符串格式）
    VestingBalance  string   `bson:"vesting_balance"`   // VESTS余额对应的STEEM
    
    // 历史数据（可选，用于快速查询）
    TransferHistory []bson.M `bson:"transfer_history,omitempty"` // 转账历史（可选）
    MarketHistory   []bson.M `bson:"market_history,omitempty"`   // 市场历史（可选）
    PostHistory     []bson.M `bson:"post_history,omitempty"`     // 帖子历史（可选）
    VoteHistory     []bson.M `bson:"vote_history,omitempty"`     // 投票历史（可选）
    OtherHistory    []bson.M `bson:"other_history,omitempty"`    // 其他历史（可选）
    TagsUsage       []bson.M `bson:"tags_usage,omitempty"`       // 标签使用（可选）
    GuestBloggers   []string `bson:"guest_bloggers,omitempty"`   // 客座博主（可选）
    
    // 索引字段
    NameLower       string   `bson:"name_lower"`        // 小写账户名（搜索用）
    
    // 更新标记
    NeedsUpdate     bool     `bson:"needs_update"`     // 标记需要更新（操作处理时设置）
    LastUpdated     time.Time `bson:"last_updated"`     // 最后更新时间
}
```

**索引**：

- `{name: 1}` - 主键索引
- `{name_lower: 1}` - 搜索索引
- `{reputation: -1}` - 按声誉排序
- `{vesting_shares: -1}` - 按权益排序
- `{last_post: -1}` - 按最后发帖时间排序
- `{needs_update: 1, last_updated: 1}` - 需要更新的账户查询

### 3.5 评论集合 (comments)

```go
type Comment struct {
    ID              string    `bson:"_id"`              // author/permlink
    Author          string    `bson:"author"`
    Permlink        string    `bson:"permlink"`
    Title           string    `bson:"title"`
    Body            string    `bson:"body"`
    
    // 关系字段
    ParentAuthor    string    `bson:"parent_author"`
    ParentPermlink  string    `bson:"parent_permlink"`
    Category        string    `bson:"category"`
    
    // 状态字段
    NetVotes        int       `bson:"net_votes"`
    TotalPayout     float64   `bson:"total_payout"`
    PendingPayout   float64   `bson:"pending_payout"`
    
    // 时间字段
    Created         time.Time `bson:"created"`
    LastUpdate      time.Time `bson:"last_update"`
    CashoutTime     time.Time `bson:"cashout_time"`
    
    // 索引字段
    AuthorLower     string    `bson:"author_lower"`
    CategoryLower   string    `bson:"category_lower"`
    DateIndex       string    `bson:"date_idx"`
}
```

**索引**：

- `{author: 1, created: -1}` - 作者评论列表
- `{category: 1, created: -1}` - 分类评论列表
- `{created: -1}` - 最新评论
- `{net_votes: -1, created: -1}` - 热门评论
- `{total_payout: -1}` - 高收益评论

### 3.6 操作统计集合 (operation_stats)

**目的**：预计算操作统计，避免实时聚合

```go
type OperationStats struct {
    ID          string    `bson:"_id"`              // op_type + date_idx
    OpType      string    `bson:"op_type"`
    DateIndex   string    `bson:"date_idx"`         // YYYY-MM-DD
    HourIndex   int       `bson:"hour_idx"`         // 0-23
    
    Count       int64     `bson:"count"`
    UniqueAccounts int64  `bson:"unique_accounts"`
    
    UpdatedAt   time.Time `bson:"updated_at"`
}
```

**索引**：

- `{op_type: 1, date_idx: 1, hour_idx: 1}` - 操作统计查询

### 3.7 全局统计集合 (global_stats)

```go
type GlobalStats struct {
    ID              string    `bson:"_id"`              // "current"
    TotalAccounts   int64     `bson:"total_accounts"`
    TotalPosts      int64     `bson:"total_posts"`
    TotalComments   int64     `bson:"total_comments"`
    TotalTransfers  int64     `bson:"total_transfers"`
    TotalVotes      int64     `bson:"total_votes"`
    
    LastBlockNum    int64     `bson:"last_block_num"`
    LastBlockTime   time.Time `bson:"last_block_time"`
    
    UpdatedAt       time.Time `bson:"updated_at"`
}
```



## 4. 服务架构设计

### 4.1 Block Sync Service

```go
type BlockSyncService struct {
    // 配置
    config *Config
    db     *mongo.Database
    steem  *steem.Client
    
    // 状态
    lastBlock int64
    
    // 工作队列
    blockQueue    chan int64
    operationQueue chan *Operation
    
    // 批量缓冲区
    blockBuffer      []*Block
    operationBuffer   []*Operation
    accountOpBuffer   []*AccountOperation
    
    // 统计
    stats *SyncStats
}
```



### 4.2 处理流程

```javascript
1. Block Fetcher (单 goroutine)
   └─► 批量获取区块 (get_blocks_range)
       └─► 串行处理每个区块

2. Block Processor (串行处理)
   └─► 保存区块到 blocks 集合
       └─► 获取操作 (get_ops_in_block)
           └─► 按顺序串行处理每个 operation

3. Operation Processor (串行处理)
   └─► 处理操作
       ├─► 标记相关账户需要更新 (needs_update = true)
       ├─► 更新 comments 集合
       ├─► 写入 operations 集合
       └─► 写入 account_operations 集合

4. Batch Writer
   └─► 批量写入 MongoDB
       ├─► BulkWrite blocks
       ├─► BulkWrite operations
       ├─► BulkWrite account_operations
       └─► BulkWrite comments (upsert)

5. Account Updater (单 goroutine，在 CronTab 中)
   └─► 查询需要更新的账户 (needs_update = true)
       └─► 批量调用 condenser_api.get_accounts
           └─► 更新 accounts 集合
               └─► 清除 needs_update 标记

6. Stats Updater (在 CronTab 中)
   └─► 定期更新统计
       ├─► 更新 operation_stats
       └─► 更新 global_stats
```



## 5. 索引策略

### 5.1 必须创建的索引

```go
// blocks 集合
db.blocks.createIndex({number: 1}, {unique: true})
db.blocks.createIndex({timestamp: -1})
db.blocks.createIndex({witness: 1, timestamp: -1})
db.blocks.createIndex({date_idx: 1, timestamp: -1})

// operations 集合
db.operations.createIndex({block_num: 1, op_index: 1})
db.operations.createIndex({op_type: 1, block_time: -1})
db.operations.createIndex({primary_account: 1, block_time: -1})
db.operations.createIndex({accounts: 1, block_time: -1})
db.operations.createIndex({trx_id: 1})
db.operations.createIndex({date_idx: 1, hour_idx: 1})

// account_operations 集合
db.account_operations.createIndex({account: 1, block_time: -1})
db.account_operations.createIndex({account: 1, op_type: 1, block_time: -1})
db.account_operations.createIndex({account: 1, block_num: -1})

// accounts 集合
db.accounts.createIndex({name: 1}, {unique: true})
db.accounts.createIndex({name_lower: 1})
db.accounts.createIndex({reputation: -1})
db.accounts.createIndex({vesting_shares: -1})
db.accounts.createIndex({last_active: -1})

// comments 集合
db.comments.createIndex({author: 1, created: -1})
db.comments.createIndex({category: 1, created: -1})
db.comments.createIndex({created: -1})
db.comments.createIndex({net_votes: -1, created: -1})
```



## 6. 查询优化策略

### 6.1 账户操作历史查询

**优化前**（需要JOIN）：

```go
// 需要从 operations 集合查询，然后JOIN account_operations
operations := db.operations.find({primary_account: "alice"})
```

**优化后**（直接查询）：

```go
// 直接从 account_operations 查询
ops := db.account_operations.find({
    account: "alice",
    block_time: {$gte: startTime, $lte: endTime}
}).sort({block_time: -1})
```



### 6.2 最新区块查询

**优化**：使用时间倒序索引

```go
blocks := db.blocks.find({}).sort({timestamp: -1}).limit(20)
```



### 6.3 账户搜索

**优化**：使用小写索引 + 前缀匹配

```go
accounts := db.accounts.find({
    name_lower: {$regex: "^" + query.toLowerCase()}
}).sort({reputation: -1}).limit(10)
```



## 7. 实施计划

### Phase 1: 核心 Block Sync (2-3周)

- [ ] 实现 Block Fetcher
- [ ] 实现 Operation Processor
- [ ] 实现批量写入逻辑
- [ ] 实现基础数据模型

### Phase 2: 数据模型优化 (1-2周)

- [ ] 实现 account_operations 集合
- [ ] 实现操作统计集合
- [ ] 创建所有索引
- [ ] 数据迁移脚本

### Phase 3: 查询优化 (1周)

- [ ] 实现物化视图更新逻辑
- [ ] 优化查询接口
- [ ] 性能测试和调优

### Phase 4: 监控和运维 (1周)

- [ ] 添加 Prometheus 指标
- [ ] 添加健康检查
- [ ] 添加错误恢复机制

## 8. 关键文件

### 需要创建/修改的文件

1. **数据模型**：

- `internal/database/models.go` - 重新设计所有模型
- `internal/database/indexes.go` - 索引定义

2. **服务层**：

- `internal/services/block_sync.go` - 核心同步服务（单 goroutine）
- `internal/services/crontab.go` - 计划任务服务（单 goroutine）
- `internal/services/account_updater.go` - 账户更新服务（调用 condenser_api.get_accounts）
- `internal/services/stats_updater.go` - 统计更新服务

3. **操作处理**：

- `internal/blockchain/operation_processor.go` - 操作处理器
- `internal/blockchain/operation_handlers.go` - 各操作类型的处理器

4. **配置**：

- `configs/config.yaml` - 简化配置

## 9. 性能目标

- **区块处理速度**：500+ blocks/sec
- **操作处理速度**：5000+ ops/sec
- **查询响应时间**：
- 区块查询：< 10ms
- 账户查询：< 50ms
- 账户操作历史：< 100ms（分页）
- 搜索：< 200ms

## 10. 账户信息更新策略

### 10.1 更新机制

**核心原则**：账户信息以 `condenser_api.get_accounts` 的返回数据为准。

1. **操作处理时标记**：

- 当处理涉及账户的操作（transfer, vote, comment等）时，标记账户 `needs_update = true`
- 不自己计算余额，只标记需要更新

2. **批量更新**：

- Account Updater 定期查询 `needs_update = true` 的账户
- 批量调用 `condenser_api.get_accounts` 获取最新账户信息
- 更新 accounts 集合，清除 `needs_update` 标记

3. **更新频率**：

- 在 CronTab 中定期执行（如每小时或每6小时）
- 批量处理，避免频繁调用 API

### 10.2 实现示例

```go
// Operation Processor 中标记账户需要更新
func (p *OperationProcessor) handleTransfer(ctx context.Context, op *Operation) error {
    // ... 保存 transfer 记录 ...
    
    // 标记账户需要更新（不自己计算余额）
    p.markAccountNeedsUpdate(ctx, opData["from"])
    p.markAccountNeedsUpdate(ctx, opData["to"])
    
    return nil
}

// Account Updater 中批量更新
func (a *AccountUpdater) UpdateAccounts(ctx context.Context) error {
    // 查询需要更新的账户
    accounts, err := a.db.FindAccountsNeedingUpdate(ctx, 100) // 批量100个
    
    if len(accounts) == 0 {
        return nil
    }
    
    // 调用 condenser_api.get_accounts
    accountNames := make([]string, len(accounts))
    for i, acc := range accounts {
        accountNames[i] = acc.Name
    }
    
    updatedAccounts, err := a.steem.GetAccounts(ctx, accountNames)
    if err != nil {
        return err
    }
    
    // 批量更新 accounts 集合
    for _, acc := range updatedAccounts {
        acc.NeedsUpdate = false
        acc.LastUpdated = time.Now()
        a.db.UpsertAccount(ctx, acc)
    }
    
    return nil
}
```



## 11. 注意事项

1. **数据一致性**：按区块顺序串行处理，保证操作时序正确性
2. **账户信息**：以 `condenser_api.get_accounts` 为准，不自己计算余额
3. **批量写入**：使用 BulkWrite，设置 ordered=false 提升性能
4. **账户更新**：操作处理时只标记，定期批量更新，避免频繁调用 API
5. **索引维护**：定期分析慢查询，优化索引