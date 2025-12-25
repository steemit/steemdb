# SteemDB Sync 三层架构重构计划

## 架构概述

参考 `sps-fund-watcher` 项目的 sync 机制，将 `steemdb-sync` 重构为三层架构：

```javascript
┌─────────────────────────────────────────────────────────────┐
│                    Layer 1: Raw Operation Sync              │
│  - 同步所有 operations（包括 virtual operations）           │
│  - 保存 operation 与 transaction id、block num、block id 的关系 │
│  - 只负责数据同步，不进行业务处理                           │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│              Layer 2: Business Logic Processing            │
│  - 从数据库按区块链时间顺序批量读取 operations（按 timestamp 字段）│
│  - 根据 legacy 需求处理各种业务逻辑                         │
│  - 生成业务数据存入数据库（comments, votes, transfers等）  │
│  - 支持恢复机制：从上次结束的区块高度继续工作               │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│                  Layer 3: Backfill Tool                     │
│  - 可指定块高范围执行业务逻辑补齐                           │
│  - 支持指定业务类型（如只补齐 comments）                    │
│  - 新增业务逻辑时，补齐上次结束区块高度之前的数据           │
│  - 执行时根据业务情况判断是否需要停止 Layer 2               │
└─────────────────────────────────────────────────────────────┘
```



## 第一部分：Raw Operation Sync

### 1.1 数据模型设计

**`operations` 集合结构**（参考 sps-fund-watcher）：

```go
type Operation struct {
    ID              primitive.ObjectID `bson:"_id"`
    BlockNum        int64             `bson:"block_num"`
    BlockID         string            `bson:"block_id"`        // 区块哈希
    TrxID           string            `bson:"trx_id"`          // 交易ID
    TrxInBlock      int               `bson:"trx_in_block"`   // 交易在区块中的索引
    OpInTrx         int               `bson:"op_in_trx"`      // 操作在交易中的索引
    OpType          string            `bson:"op_type"`        // 操作类型
    OpData          bson.M            `bson:"op_data"`         // 操作数据（完整）
    IsVirtual       bool              `bson:"is_virtual"`      // 是否为 virtual operation
    VirtualOpNum    int               `bson:"virtual_op_num"`  // virtual operation 编号（如果是 virtual）
    Timestamp       time.Time         `bson:"timestamp"`       // 区块链上 operation 的发生时间（链上时间）
    CreatedAt       time.Time         `bson:"created_at"`     // 记录创建时间（入库时间）
}
```

**`sync_state` 集合**（参考 sps-fund-watcher）：

```go
type SyncState struct {
    ID                    string    `bson:"_id"`
    LastBlock             int64     `bson:"last_block"`
    LastIrreversibleBlock int64     `bson:"last_irreversible_block"`
    UpdatedAt             time.Time `bson:"updated_at"`
}
```



### 1.2 同步服务实现

**文件**: `internal/sync/raw_syncer.go`（新建）**核心功能**：

- 使用 `GetOpsInBlocks` 获取所有操作（包括 virtual operations）
- 参考 `sps-fund-watcher/internal/sync/syncer.go` 的实现
- 批量处理区块，使用 `GetOpsInBlocks` 而不是 `GetBlocks` + `GetOpsInBlock`
- **正确存储链上时间**：
- Regular operations：使用 `block.Timestamp`（区块时间戳）
- Virtual operations：使用 `opObj.Timestamp`（操作对象的时间戳）
- 确保 `timestamp` 字段存储的是区块链上的实际发生时间，而非入库时间
- 保存所有 operations 到 `operations` 集合
- 使用 upsert 防止重复（唯一索引：`block_num + trx_id + op_in_trx + is_virtual + virtual_op_num`）

**关键代码结构**：

```go
type RawSyncer struct {
    steemAPI  *steemgosdk.API
    storage   *database.MongoDB
    logger    utils.Logger
    config    *utils.Config
}

func (s *RawSyncer) SyncBlocks(ctx context.Context, startBlock int64) error {
    // 1. 获取 latest irreversible block
    // 2. 批量获取 operations（使用 GetOpsInBlocks）
    // 3. 处理每个区块的 operations
    // 4. 保存到 operations 集合
    // 5. 更新 sync_state
}
```



### 1.3 存储层实现

**文件**: `internal/database/mongodb.go`（修改）**新增方法**：

- `InsertOperations(ctx, ops []*Operation) error` - 批量插入 operations（支持分表，自动路由到对应集合，自动创建集合和索引）
- `EnsureCollectionIndexes(ctx, collectionName string) error` - 确保指定集合的索引存在（自动创建）
- `GetOperationsCollection(blockNum int64) *mongo.Collection` - 根据区块高度获取对应的集合
- `QueryOperations(ctx, startBlock, endBlock int64, filter bson.M) ([]*Operation, error)` - 查询 operations（自动处理单集合或跨集合查询）
- `QueryOperationsAcrossCollections(ctx, startBlock, endBlock int64, filter bson.M) ([]*Operation, error)` - 跨集合查询 operations（仅用于特殊情况，如 backfill）
- `GetSyncState(ctx) (*SyncState, error)` - 获取同步状态
- `UpdateSyncState(ctx, lastBlock, lastIrreversibleBlock int64) error` - 更新同步状态（使用 `$max` 确保只增不减）

**分表策略**（详见 `steemdb-sync_Layer1存储空间评估.md`）：

- **分表方案**: 按区块高度范围分表
- **分表粒度**: 每 1000 万区块一个集合（`operations_0_10000000`, `operations_10000000_20000000`, ...）
- **理由**: 
- 总 operations 数量约 24 亿条，总存储空间约 1.6 TB
- 单集合 24 亿条文档会导致查询性能下降和维护困难
- 分表后每个集合约 2.4 亿条文档，160 GB，性能和维护性更好
- **查询设计**: 
- Layer 2 的 batch size 设计应确保每次查询只涉及单个集合
- 正常情况下不会出现跨集合查询（batch size 远小于 1000 万区块范围）
- 跨集合查询仅用于特殊情况（如 backfill 工具处理大范围数据）

**索引**（每个集合都有相同的索引结构）：

- 唯一索引：`{block_num: 1, trx_id: 1, op_in_trx: 1, is_virtual: 1, virtual_op_num: 1}`
- 查询索引：
- `{block_num: 1, timestamp: -1}` - 按区块号和链上时间排序（用于 Layer 2 按时间顺序处理）
- `{timestamp: 1}` - 按链上时间排序（用于按区块链时间顺序查询）
- `{trx_id: 1}`, `{op_type: 1}` - 其他查询场景

### 1.4 入口程序

**文件**: `cmd/sync/main.go`（重构）**功能**：

- 只启动 Raw Syncer
- 移除所有业务处理逻辑
- 参考 `sps-fund-watcher/cmd/sync/main.go` 的实现

## 第二部分：Business Logic Processing

### 2.1 业务处理器设计

**文件**: `internal/processor/business_processor.go`（新建）**核心功能**：

- 从 `operations` 集合**按区块链时间顺序**（`timestamp` 字段）批量读取 operations
- 根据操作类型调用对应的业务处理器
- 生成业务数据存入对应的集合（comments, votes, transfers, etc.）
- **支持恢复机制**：记录每个业务类型的最后处理区块高度，重启后从上次结束的区块继续

**处理流程**：

```go
type BusinessProcessor struct {
    db     *database.MongoDB
    logger utils.Logger
    config *utils.Config
    
    // 业务处理器映射
    handlers map[string]BusinessHandler
}

type BusinessHandler func(ctx context.Context, op *Operation) error

func (p *BusinessProcessor) ProcessOperations(ctx context.Context, startBlock, endBlock int64) error {
    // 1. 从 operations 集合查询指定范围的 operations
    // 2. 按区块链时间顺序排序（使用 timestamp 字段，确保按链上发生时间顺序处理）
    // 3. 批量处理（可配置批量大小）
    // 4. 调用对应的业务处理器
    // 5. 更新处理进度（每个业务类型单独记录）
}
```



### 2.2 业务处理器实现

**文件**: `internal/processor/handlers/`（新建目录）根据 `legacy/docker/sync/sync.py` 的需求，实现以下处理器：

- `comment_handler.go` - 处理 comment 操作，保存到 comments 集合
- `vote_handler.go` - 处理 vote 操作，保存到 votes 集合
- `transfer_handler.go` - 处理 transfer 操作，保存到 transfers 集合
- `convert_handler.go` - 处理 convert 操作
- `curation_reward_handler.go` - 处理 curation_reward
- `author_reward_handler.go` - 处理 author_reward
- `vesting_handler.go` - 处理 transfer_to_vesting 和 fill_vesting_withdraw
- `account_handler.go` - 标记账户需要更新
- 等等...

### 2.3 业务处理服务

**文件**: `internal/services/business_processor_service.go`（新建）**功能**：

- 定期从 `operations` 集合读取未处理的 operations
- 调用 `BusinessProcessor` 进行处理
- **恢复机制**：从 `business_processing_state` 集合读取每个业务类型的最后处理区块高度
- **进度跟踪**：每个业务类型单独记录处理进度，支持独立恢复
- 支持暂停/恢复机制

**处理状态集合**：

```go
type BusinessProcessingState struct {
    ID            string    `bson:"_id"`            // 业务类型（如 "comments", "votes"）
    LastBlock     int64     `bson:"last_block"`     // 最后处理的区块高度
    UpdatedAt     time.Time `bson:"updated_at"`    // 最后更新时间
}
```

**恢复机制**：

- 服务启动时，从 `business_processing_state` 集合读取每个业务类型的 `last_block`
- 从 `last_block + 1` 继续处理新的 operations
- 如果业务类型不存在（新增业务），从配置的 `start_block` 开始处理
- 处理完成后，更新 `last_block` 为当前处理的区块高度

**配置**：

```yaml
business_processor:
  enabled: true
  batch_size: 1000        # 每次处理的 operations 数量（确保不会跨集合查询）
  batch_block_range: 10000 # 每次处理的区块范围上限（可选，用于限制查询范围）
  interval: 10s           # 处理间隔
  start_block: 1          # 起始区块（用于初始化新业务类型）
```

**Batch Size 设计原则**：

- `batch_size` 按 operations 数量控制，每次处理 1000 条 operations
- 1000 条 operations 约对应 40-50 个区块（按 24 operations/区块计算）
- 远小于单个集合的 1000 万区块范围，确保不会跨集合查询
- 如果设置了 `batch_block_range`，则同时限制区块范围，进一步确保单集合查询

**恢复机制实现**：

```go
func (s *BusinessProcessorService) getLastProcessedBlock(ctx context.Context, businessType string) (int64, error) {
    // 从 business_processing_state 集合读取
    // 如果不存在，返回配置的 start_block
}

func (s *BusinessProcessorService) updateLastProcessedBlock(ctx context.Context, businessType string, blockNum int64) error {
    // 更新 business_processing_state 集合
}
```

**新增业务逻辑处理流程**：

1. 添加新的业务处理器（如 `new_feature_handler.go`）
2. 在 `business_processing_state` 集合中创建记录，`last_block` 初始值为配置的 `start_block`
3. 业务处理服务会从 `start_block` 开始处理，但只处理新产生的 operations（即从当前区块开始）
4. **历史数据补齐**：使用 backfill 工具补齐该业务类型在 `start_block` 到当前 `last_block` 之间的历史数据
5. 补齐完成后，backfill 工具更新 `business_processing_state` 的 `last_block` 为补齐的结束区块
6. 业务处理服务会从补齐的结束区块继续处理新的 operations

### 2.4 入口程序

**文件**: `cmd/processor/main.go`（新建）**功能**：

- 启动业务处理服务
- 支持从上次结束的区块高度继续工作

## 第三部分：Backfill Tool

### 3.1 补齐工具设计

**文件**: `cmd/backfill/main.go`（新建）**功能**：

- 命令行工具，可指定块高范围
- 支持指定业务逻辑类型（如：只补齐 comments，或只补齐 votes）
- 执行时检查 Layer 2 是否在运行，根据业务情况决定是否停止
- **新增业务逻辑时**：补齐该业务类型在上次结束区块高度之前的数据

**命令行参数**：

```bash
./backfill \
  --start-block 1000000 \
  --end-block 2000000 \
  --business-type comments \
  --stop-processor true
```

**新增业务逻辑补齐流程**：

1. 查询 `business_processing_state` 获取该业务类型的 `last_block`（如果不存在，则为 0）
2. 如果 `last_block > 0`，从 `start_block` 到 `last_block` 进行补齐
3. 补齐过程中，从 `operations` 集合**按区块链时间顺序**（`timestamp` 字段）读取该范围的 operations
4. 只执行指定的业务类型处理器（通过 `--business-type` 参数指定）
5. 补齐完成后，更新 `business_processing_state` 的 `last_block` 为补齐的结束区块
6. 业务处理服务会从补齐的结束区块继续处理新的 operations

### 3.2 业务逻辑选择器

**文件**: `internal/processor/selector.go`（新建）**功能**：

- 根据 `--business-type` 参数选择需要执行的业务处理器
- 支持多个业务类型：`--business-type comments,votes,transfers`
- 支持 `all` 执行所有业务逻辑

### 3.3 进程协调机制

**文件**: `internal/processor/coordinator.go`（新建）**功能**：

- 检查 Layer 2 是否在运行
- 根据业务情况决定是否需要停止 Layer 2（避免冲突）
- 补齐完成后可选择是否重启 Layer 2
- 使用文件锁或数据库标志位来协调

## 文件结构变更

### 新增文件

```javascript
steemdb-sync/
├── cmd/
│   ├── sync/           # Layer 1: Raw Operation Sync
│   │   └── main.go
│   ├── processor/      # Layer 2: Business Logic Processing
│   │   └── main.go
│   └── backfill/       # Layer 3: Backfill Tool
│       └── main.go
├── internal/
│   ├── sync/           # Raw sync 相关（参考 sps-fund-watcher）
│   │   ├── syncer.go
│   │   └── block_processor.go
│   ├── processor/      # Business processor 相关
│   │   ├── business_processor.go
│   │   ├── selector.go
│   │   ├── coordinator.go
│   │   └── handlers/
│   │       ├── comment_handler.go
│   │       ├── vote_handler.go
│   │       └── ...
│   └── database/
│       └── mongodb.go   # 新增 operations、sync_state、business_processing_state 相关方法
```



### 修改文件

- `internal/database/models.go` - 添加 `Operation`、`SyncState`、`BusinessProcessingState` 模型
- `internal/database/mongodb.go` - 添加 operations、sync_state、business_processing_state 的 CRUD 方法
- `internal/utils/steem_client.go` - 添加 `GetOpsInBlocks` 方法（如果不存在）

### 废弃/重构文件

- `internal/services/block_sync.go` - 重构为 `internal/sync/raw_syncer.go`
- `internal/blockchain/operation_processor.go` - 业务处理逻辑移到 `internal/processor/`
- `internal/services/crontab.go` - 部分功能移到 `internal/services/business_processor_service.go`

## 实现细节

### 0. 分表实现

**参考**: `steemdb-sync_Layer1存储空间评估.md`**集合命名规则**：

```go
func GetCollectionName(blockNum int64) string {
    rangeSize := int64(10_000_000) // 每 1000 万区块一个集合
    startBlock := (blockNum / rangeSize) * rangeSize
    endBlock := startBlock + rangeSize
    return fmt.Sprintf("operations_%d_%d", startBlock, endBlock)
}
```

**集合和索引创建策略**：

- **集合创建**：自动创建（MongoDB 默认行为，第一次插入数据时自动创建）
- **索引创建**：自动创建（在第一次插入数据到新集合前，确保索引存在）
- **实现方式**：
  ```go
      func (m *MongoDB) EnsureCollectionIndexes(ctx context.Context, collectionName string) error {
          collection := m.db.Collection(collectionName)
          
          // 检查索引是否已存在（通过尝试创建，如果已存在则忽略错误）
          // 或者先检查集合是否存在，如果不存在则创建索引
          indexes := []mongo.IndexModel{
              // 唯一索引
              {Keys: bson.D{{Key: "block_num", Value: 1}, {Key: "trx_id", Value: 1}, 
                           {Key: "op_in_trx", Value: 1}, {Key: "is_virtual", Value: 1}, 
                           {Key: "virtual_op_num", Value: 1}}, 
               Options: options.Index().SetUnique(true)},
              // 查询索引
              {Keys: bson.D{{Key: "block_num", Value: 1}, {Key: "timestamp", Value: -1}}},
              {Keys: bson.D{{Key: "timestamp", Value: 1}}},
              {Keys: bson.D{{Key: "trx_id", Value: 1}}},
              {Keys: bson.D{{Key: "op_type", Value: 1}}},
          }
          
          _, err := collection.Indexes().CreateMany(ctx, indexes)
          // 如果索引已存在，MongoDB 会返回错误，可以忽略
          if err != nil && !strings.Contains(err.Error(), "already exists") {
              return err
          }
          return nil
      }
  ```




- **调用时机**：在 `InsertOperations` 中，插入数据前调用 `EnsureCollectionIndexes` 确保索引存在

**查询设计**：

- **Layer 2 正常处理**：batch size 设计为 1000 条 operations（约 40-50 个区块），远小于单个集合的 1000 万区块范围，确保只查询单个集合
- **跨集合查询**：仅在以下情况使用：

1. Backfill 工具处理大范围数据（如补齐 1000 万区块范围）
2. 特殊查询需求（如按时间范围查询跨集合的数据）

- **查询实现**：
- 先判断查询范围是否跨多个集合
- 如果只涉及单个集合，直接查询该集合
- 如果跨多个集合，并行查询所有相关集合，合并结果并按 `timestamp` 排序

### 1. GetOpsInBlocks 方法

**参考**: `sps-fund-watcher/internal/sync/syncer.go:171`需要在 `steemgosdk` 或 `steemutil` 中找到 `GetOpsInBlocks` 方法，如果不存在，需要：

- 检查 `steemgosdk` 是否支持
- 如果不支持，使用 `GetOpsInBlock` 循环调用（效率较低但可用）

### 2. Virtual Operation 处理

**参考**: `sps-fund-watcher/internal/sync/block_processor.go:190-200`

- Virtual operations 的 `TrxID` 通常为空
- 使用 `virtual_<block_num>_<virtual_op_num>` 作为唯一标识
- 在 `operations` 集合中用 `is_virtual` 和 `virtual_op_num` 字段标识
- **Timestamp 存储**：
- Regular operations：使用 `block.Timestamp`（区块时间戳）
- Virtual operations：使用 `opObj.Timestamp`（操作对象的时间戳）
- 确保 `timestamp` 字段存储的是区块链上的实际发生时间，而非入库时间（`created_at`）

### 3. 唯一索引设计

**参考**: `sps-fund-watcher/internal/storage/mongodb.go:305-313`唯一索引组合：

- Regular operations: `block_num + trx_id + op_in_trx`
- Virtual operations: `block_num + is_virtual + virtual_op_num`

### 4. 业务处理顺序

**参考**: `legacy/docker/sync/sync.py:46-93`按 legacy 的处理顺序：

1. comment → comments 集合
2. vote → votes 集合
3. transfer → transfers 集合
4. convert → converts 集合
5. 等等...

### 5. 账户更新标记

**参考**: `legacy/docker/sync/sync.py:380-403`

- 业务处理时标记账户需要更新（`needs_update = true`）
- 单独的账户更新服务从 API 获取最新账户信息
- 不自己计算余额，以 API 为准

### 6. 业务处理状态管理

**新增集合**: `business_processing_state`

```go
type BusinessProcessingState struct {
    ID        string    `bson:"_id"`         // 业务类型（如 "comments", "votes", "transfers"）
    LastBlock int64     `bson:"last_block"`  // 最后处理的区块高度
    UpdatedAt time.Time `bson:"updated_at"`  // 最后更新时间
}
```

**使用场景**：

- Layer 2 启动时，读取每个业务类型的 `last_block`，从该区块继续处理
- 新增业务类型时，`last_block` 初始化为配置的 `start_block`
- Backfill 补齐完成后，更新 `last_block` 为补齐的结束区块
- 支持多个业务类型独立处理，互不影响

## 配置变更

### 新增配置项

```yaml
sync:
  # Raw sync 配置
  batch_size: 100
  block_interval: 3s
  
business_processor:
  enabled: true
  batch_size: 1000
  interval: 10s
  start_block: 1          # 新业务类型的起始区块
  
backfill:
  # 默认配置（命令行参数会覆盖）
  default_batch_size: 1000
```



## 测试策略

1. **单元测试**：测试各个 handler 的业务逻辑
2. **集成测试**：测试三层架构的协作
3. **恢复机制测试**：测试业务处理服务重启后能否正确恢复
4. **性能测试**：确保同步速度满足要求

## 实施步骤

1. **Phase 1**：实现 Layer 1，开始同步所有 operations
2. **Phase 2**：实现 Layer 2，开始业务处理，支持恢复机制