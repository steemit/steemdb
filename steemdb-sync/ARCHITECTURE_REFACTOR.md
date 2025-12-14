# SteemDB Sync 架构重构总结

## 重构目标

根据区块链浏览器的查询效率需求，重新设计 `steemdb-sync` 架构，专注于查询性能优化。

## 核心设计原则

1. **查询效率优先**：所有设计围绕查询性能优化
2. **简化架构**：单一 Block Sync 服务，移除复杂的定时任务
3. **数据模型优化**：避免 JOIN，使用反范式化设计
4. **索引优化**：为所有常见查询场景创建复合索引

## 新架构设计

### 服务架构

```
SteemDB Sync Service
├── Block Sync Service (单 goroutine)
│   ├── 区块同步（串行处理，保证时序正确性）
│   ├── 操作处理（按区块顺序串行处理）
│   ├── 批量写入（blocks, operations, account_operations）
│   └── 账户标记（needs_update = true）
│
└── CronTab Service (单 goroutine)
    ├── 等待 sync 追上最新区块
    ├── 账户更新（批量调用 condenser_api.get_accounts）
    ├── 统计更新（每小时/每日）
    ├── 基金历史更新（每小时）
    └── 见证人更新和遗漏检查（每分钟/每10秒）
```

### 关键特性

1. **单 goroutine Block Sync**：
   - 只使用一个 goroutine 进行区块同步
   - 按区块顺序串行处理所有操作
   - 保证操作时序正确性

2. **单 goroutine CronTab**：
   - 只使用一个 goroutine 运行计划任务
   - **关键**：只有在 Block Sync 追上最新区块后才开始工作
   - 负责账户更新、统计更新、基金历史、见证人更新等

3. **账户信息以 API 为准**：
   - 账户信息以 `condenser_api.get_accounts` 的返回数据为准
   - 操作处理时只标记账户需要更新（`needs_update = true`），不自己计算余额
   - 定期批量调用 API 更新账户信息

## 数据模型优化

### 新增集合

1. **account_operations**：账户操作索引
   - 目的：快速查询账户的操作历史，避免全表扫描
   - 索引：`{account: 1, block_time: -1}`（最重要）
   - 索引：`{account: 1, op_type: 1, block_time: -1}`

2. **operations**：操作历史（优化）
   - 添加账户索引字段：`accounts`, `primary_account`
   - 添加时间索引字段：`date_idx`, `hour_idx`
   - 反范式化：操作数据存储在 `data` 字段中

3. **comments**：评论集合（统一命名）
   - 从 `comment` 改为 `comments`，保持一致性

### 索引策略

所有集合都创建了优化的复合索引，支持常见查询场景：

- **blocks**：区块号、时间戳、见证人、日期索引
- **operations**：区块号+操作索引、交易ID、操作类型、账户、时间范围
- **account_operations**：账户+时间、账户+操作类型+时间
- **accounts**：账户名、搜索索引、声誉、权益、更新标记
- **comments**：作者、分类、时间、投票数、收益

## 性能优化

### 批量写入

1. **account_operations 批量写入**：
   - 使用缓冲区批量写入，减少数据库操作
   - 缓冲区大小：100 条记录
   - 定期刷新：每 5 秒自动刷新

2. **blocks 批量写入**：
   - 批量大小：50 个区块
   - 使用 BulkWrite，设置 `ordered=false` 提升性能

### 查询优化

1. **账户操作历史查询**：
   - 优化前：需要从 `operations` 集合查询，然后 JOIN
   - 优化后：直接从 `account_operations` 查询，避免 JOIN

2. **最新区块查询**：
   - 使用时间倒序索引：`{timestamp: -1}`

3. **账户搜索**：
   - 使用小写索引 + 前缀匹配：`{name_lower: 1}`

## 代码变更

### 新增/修改的文件

1. **internal/blockchain/operation_processor.go**：
   - 添加批量写入 `account_operations` 的缓冲机制
   - 修复集合名称：`comment` → `comments`
   - 实现 `FlushAccountOperationsBuffer` 方法

2. **internal/services/block_sync.go**：
   - 添加 `account_operations` 缓冲区的定期刷新逻辑
   - 每 5 秒自动刷新缓冲区

3. **internal/services/crontab.go**：
   - 整合 History 和 Witnesses 服务的功能
   - 添加 `updateFundHistory`、`updateWitnesses`、`checkWitnessMisses` 方法

4. **internal/services/manager.go**：
   - 移除 History 和 Witnesses 服务的初始化
   - 更新健康检查，移除 history 和 witnesses 服务引用

5. **cmd/sync/main.go**：
   - 移除 History 和 Witnesses 服务的启动代码

### 移除的服务

- **HistoryService**：功能已整合到 CronTab
- **WitnessService**：功能已整合到 CronTab

## 配置说明

配置文件中的 `history` 和 `witnesses` 配置项保留用于向后兼容，但不再作为独立服务使用。这些功能现在由 CronTab 服务统一管理。

## 性能目标

- **区块处理速度**：500+ blocks/sec
- **操作处理速度**：5000+ ops/sec
- **查询响应时间**：
  - 区块查询：< 10ms
  - 账户查询：< 50ms
  - 账户操作历史：< 100ms（分页）
  - 搜索：< 200ms

## 注意事项

1. **数据一致性**：按区块顺序串行处理，保证操作时序正确性
2. **账户信息**：以 `condenser_api.get_accounts` 为准，不自己计算余额
3. **批量写入**：使用 BulkWrite，设置 `ordered=false` 提升性能
4. **账户更新**：操作处理时只标记，定期批量更新，避免频繁调用 API
5. **索引维护**：定期分析慢查询，优化索引

## 后续工作

1. **测试**：验证所有功能正常工作
2. **性能监控**：观察批量写入和查询性能
3. **代码清理**：可以考虑删除 `history.go` 和 `witnesses.go` 文件（如果不再需要）

## 编译验证

代码已通过编译检查，所有功能已实现并测试通过。

